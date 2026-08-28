// Package repository handles DB access for the discount module (§28).
package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/rajaku-printing/backend/internal/discount/model"
)

var (
	ErrNotFound = errors.New("discount/repository: not found")
	// ErrCodeConflict — unique index idx_discounts_code_active violated
	// (kode sudah dipakai diskon lain yang masih aktif/belum dihapus).
	ErrCodeConflict = errors.New("discount/repository: code unique conflict")
	// ErrProductNotFound — satu atau lebih product_id di cakupan diskon
	// (discount_products) tidak ada di tabel products (temuan review #3).
	// Ditegakkan DUA lapis: bulk existence check SEBELUM insert (jalur
	// normal, menghasilkan error ini langsung), DAN FK violation (23503)
	// sebagai jaring pengaman kalau produk terhapus di antara pengecekan
	// dan insert (race) — keduanya dipetakan ke sentinel yang sama supaya
	// caller tidak perlu tahu bedanya.
	ErrProductNotFound = errors.New("discount/repository: one or more product ids not found")
	// ErrCustomerNotFound — satu atau lebih customer_id di cakupan member
	// diskon (discount_customers, §30.3) tidak ada di tabel users. Mirror
	// ErrProductNotFound persis — sama dua lapis penegakannya.
	ErrCustomerNotFound = errors.New("discount/repository: one or more customer ids not found")
)

const (
	pgUniqueViolationCode     = "23505"
	pgForeignKeyViolationCode = "23503"
)

type DiscountRepository struct {
	db *gorm.DB
}

func NewDiscountRepository(db *gorm.DB) *DiscountRepository { return &DiscountRepository{db: db} }

// Create inserts a new discount row plus its initial product scope
// (discount_products, §28.9 — empty productIDs is fine, e.g. applies_to=
// "all") AND its initial member scope (discount_customers, §30.3 — empty
// customerIDs is fine, e.g. audience_scope="all" or member_scope=
// "all_members") atomically in one transaction. Returns ErrCodeConflict
// specifically when the partial unique index on `code` is violated.
func (r *DiscountRepository) Create(ctx context.Context, d *model.Discount, productIDs, customerIDs []uuid.UUID) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(d).Error; err != nil {
			return err
		}
		if err := insertDiscountProducts(tx, d.ID, productIDs); err != nil {
			return err
		}
		return insertDiscountCustomers(tx, d.ID, customerIDs)
	})
	if err != nil {
		if isUniqueViolation(err) {
			return ErrCodeConflict
		}
		if sentinel, ok := classifyScopeInsertErr(err); ok {
			return sentinel
		}
		return fmt.Errorf("create discount: %w", err)
	}
	return nil
}

// FindByID returns the discount or ErrNotFound. Soft-deleted rows are
// treated as not found — matches order.repository.FindByID pattern.
func (r *DiscountRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.Discount, error) {
	var d model.Discount
	err := r.db.WithContext(ctx).Where("deleted_at IS NULL").First(&d, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find discount by id %s: %w", id, err)
	}
	return &d, nil
}

// List returns every non-deleted discount matching a free-text search on
// code/name (case-insensitive), newest first. No pagination here — dataset
// (jumlah program diskon sebuah toko) kecil, service yang menghitung status
// turunan (butuh usage count per row) & memaginasi di memori.
func (r *DiscountRepository) List(ctx context.Context, q string) ([]model.Discount, error) {
	query := r.db.WithContext(ctx).Where("deleted_at IS NULL")
	if q != "" {
		like := "%" + q + "%"
		query = query.Where("code ILIKE ? OR name ILIKE ?", like, like)
	}
	var items []model.Discount
	if err := query.Order("created_at DESC").Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list discounts: %w", err)
	}
	return items, nil
}

// ListActiveForChannel returns non-deleted, is_active, in-date-range,
// channel-compatible, min-subtotal-satisfied discounts — the DB-level
// prefilter for GET /admin/discounts/applicable. Quota (needs a usage count
// per row) is filtered afterwards by the service.
func (r *DiscountRepository) ListActiveForChannel(ctx context.Context, channel string, subtotal int64, now time.Time) ([]model.Discount, error) {
	var items []model.Discount
	err := r.db.WithContext(ctx).
		Where("deleted_at IS NULL").
		Where("is_active = true").
		Where("channel_scope = ? OR channel_scope = ?", model.ChannelScopeAll, channel).
		Where("min_subtotal <= ?", subtotal).
		Where("starts_at IS NULL OR starts_at <= ?", now).
		Where("ends_at IS NULL OR ends_at >= ?", now).
		Order("created_at DESC").
		Find(&items).Error
	if err != nil {
		return nil, fmt.Errorf("list active discounts for channel %q: %w", channel, err)
	}
	return items, nil
}

// CountUsage returns, for each discount id in `ids`, how many non-deleted
// orders reference it (§28.4 — kuota DIHITUNG dari orders, bukan disimpan
// sebagai counter). Raw SQL against the `orders` table — allowed per §28:
// this is a direct query to another module's TABLE, not an import of its Go
// package, which stays forbidden (§22).
func (r *DiscountRepository) CountUsage(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]int64, error) {
	out := make(map[uuid.UUID]int64, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var rows []struct {
		DiscountID uuid.UUID
		Count      int64
	}
	err := r.db.WithContext(ctx).
		Table("orders").
		Select("discount_id, COUNT(*) as count").
		Where("discount_id IN ? AND deleted_at IS NULL", ids).
		Group("discount_id").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("count discount usage: %w", err)
	}
	for _, row := range rows {
		out[row.DiscountID] = row.Count
	}
	return out, nil
}

// CountUsageOne — single-discount variant of CountUsage, dipakai path
// ResolveForOrder (validasi kuota saat SATU order akan dibuat).
func (r *DiscountRepository) CountUsageOne(ctx context.Context, id uuid.UUID) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Table("orders").
		Where("discount_id = ? AND deleted_at IS NULL", id).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("count discount usage for %s: %w", id, err)
	}
	return count, nil
}

// ScopeReplace — instruksi "ganti SELURUH daftar cakupan ini" untuk satu
// sumbu (produk §28.9 atau member §30.3) dalam satu panggilan
// UpdateWithScopes. Replace=false berarti "jangan sentuh cakupan sumbu ini
// sama sekali" — IDs diabaikan.
type ScopeReplace struct {
	Replace bool
	IDs     []uuid.UUID
}

// UpdateWithScopes applies a partial field update AND (kalau Replace=true di
// salah satu/kedua ScopeReplace) mengganti SELURUH cakupan produk dan/atau
// member diskon, DALAM SATU TRANSAKSI (temuan review #1, extended §30.3 —
// dulu bernama UpdateWithProducts, hanya menangani cakupan produk). Sebelum
// pola transaksi tunggal ini ada, Update dan ganti-cakupan adalah panggilan
// terpisah — kalau salah satu gagal belakangan, field diskon (mis.
// applies_to='selected') sudah ter-commit duluan TANPA cakupannya, persis
// state yang §28.9/§30.3 larang keras. Field, cakupan produk, dan cakupan
// member sekarang hidup/mati bersama, sama seperti Create.
//
// Mengembalikan ErrNotFound kalau baris tidak ada/sudah di-soft-delete
// (hanya dicek kalau fields tidak kosong — kalau caller cuma mengganti
// cakupan, PK/FK constraint discount_products/discount_customers sudah
// menegakkan keberadaan discountID). ErrProductNotFound /
// ErrCustomerNotFound kalau salah satu id di cakupan terkait tidak
// ditemukan.
func (r *DiscountRepository) UpdateWithScopes(ctx context.Context, id uuid.UUID, fields map[string]any, products, customers ScopeReplace) error {
	if len(fields) == 0 && !products.Replace && !customers.Replace {
		return nil
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(fields) > 0 {
			updates := make(map[string]any, len(fields)+1)
			for k, v := range fields {
				updates[k] = v
			}
			updates["updated_at"] = gorm.Expr("NOW()")
			res := tx.Model(&model.Discount{}).
				Where("id = ? AND deleted_at IS NULL", id).
				Updates(updates)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return ErrNotFound
			}
		}
		if products.Replace {
			if err := tx.Exec("DELETE FROM discount_products WHERE discount_id = ?", id).Error; err != nil {
				return fmt.Errorf("clear discount product scope: %w", err)
			}
			if err := insertDiscountProducts(tx, id, products.IDs); err != nil {
				return err
			}
		}
		if customers.Replace {
			if err := tx.Exec("DELETE FROM discount_customers WHERE discount_id = ?", id).Error; err != nil {
				return fmt.Errorf("clear discount member scope: %w", err)
			}
			if err := insertDiscountCustomers(tx, id, customers.IDs); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrNotFound
		}
		if isUniqueViolation(err) {
			return ErrCodeConflict
		}
		if sentinel, ok := classifyScopeInsertErr(err); ok {
			return sentinel
		}
		return fmt.Errorf("update discount %s: %w", id, err)
	}
	return nil
}

// SoftDeleteParams — payload for soft-deleting a discount.
type SoftDeleteParams struct {
	DiscountID uuid.UUID
	ActorID    uuid.UUID
	Reason     string
}

// SoftDelete stamps deleted_at/deleted_by/delete_reason. NEVER removes the
// row — order.discount_id keeps pointing at it forever (§28.2 lapis 2).
func (r *DiscountRepository) SoftDelete(ctx context.Context, p SoftDeleteParams) error {
	res := r.db.WithContext(ctx).Model(&model.Discount{}).
		Where("id = ? AND deleted_at IS NULL", p.DiscountID).
		Updates(map[string]any{
			"deleted_at":    gorm.Expr("NOW()"),
			"deleted_by":    p.ActorID,
			"delete_reason": p.Reason,
			"updated_at":    gorm.Expr("NOW()"),
		})
	if res.Error != nil {
		return fmt.Errorf("soft delete discount %s: %w", p.DiscountID, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ProductIDs returns, for each discount id in `discountIDs`, the list of
// product ids currently scoped to it (discount_products rows) — bulk
// variant used both for single lookups (Get, ResolveForOrder) and for
// listing pages (List, Applicable) to avoid N+1 queries (§28.9 brief).
func (r *DiscountRepository) ProductIDs(ctx context.Context, discountIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error) {
	out := make(map[uuid.UUID][]uuid.UUID, len(discountIDs))
	if len(discountIDs) == 0 {
		return out, nil
	}
	var rows []model.DiscountProduct
	if err := r.db.WithContext(ctx).Where("discount_id IN ?", discountIDs).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list discount product scope: %w", err)
	}
	for _, row := range rows {
		out[row.DiscountID] = append(out[row.DiscountID], row.ProductID)
	}
	return out, nil
}

// CustomerIDs returns, for each discount id in `discountIDs`, the list of
// customer ids currently scoped to it (discount_customers rows, §30.3) —
// bulk variant used both for single lookups (Get, ResolveForOrder) and for
// listing pages (List, Applicable) to avoid N+1 queries — mirror ProductIDs.
func (r *DiscountRepository) CustomerIDs(ctx context.Context, discountIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error) {
	out := make(map[uuid.UUID][]uuid.UUID, len(discountIDs))
	if len(discountIDs) == 0 {
		return out, nil
	}
	var rows []model.DiscountCustomer
	if err := r.db.WithContext(ctx).Where("discount_id IN ?", discountIDs).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list discount member scope: %w", err)
	}
	for _, row := range rows {
		out[row.DiscountID] = append(out[row.DiscountID], row.CustomerID)
	}
	return out, nil
}

// insertDiscountProducts bulk-inserts discount_products rows for
// discountID — shared by Create and UpdateWithScopes. No-op when
// productIDs is empty. Validates every id exists in `products` FIRST
// (temuan review #3) — kalau tidak, memberikan ErrProductNotFound yang
// jelas alih-alih membiarkan raw FK violation (23503) meledak jadi 500 di
// lapisan atas. Repository ini boleh query tabel `products` langsung
// (bukan import package catalog) — pola sama seperti CountUsage terhadap
// `orders`.
func insertDiscountProducts(tx *gorm.DB, discountID uuid.UUID, productIDs []uuid.UUID) error {
	if len(productIDs) == 0 {
		return nil
	}
	if err := ensureProductsExist(tx, productIDs); err != nil {
		return err
	}
	values := make([]any, 0, len(productIDs)*2)
	placeholders := ""
	for i, pid := range productIDs {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "(?, ?)"
		values = append(values, discountID, pid)
	}
	sql := "INSERT INTO discount_products (discount_id, product_id) VALUES " + placeholders
	if err := tx.Exec(sql, values...).Error; err != nil {
		return fmt.Errorf("insert discount product scope: %w", err)
	}
	return nil
}

// ensureProductsExist bulk-checks (single query) that every id in
// productIDs exists in `products`, BEFORE any insert is attempted (temuan
// review #3). Caller is expected to have already de-duplicated productIDs
// (service.dedupeUUIDs) — count is compared against len(productIDs)
// exactly, so a caller passing duplicates would false-positive here.
func ensureProductsExist(tx *gorm.DB, productIDs []uuid.UUID) error {
	var count int64
	if err := tx.Table("products").Where("id IN ?", productIDs).Count(&count).Error; err != nil {
		return fmt.Errorf("verify product ids exist: %w", err)
	}
	if count != int64(len(productIDs)) {
		return ErrProductNotFound
	}
	return nil
}

// insertDiscountCustomers bulk-inserts discount_customers rows for
// discountID (§30.3) — mirror insertDiscountProducts, shared by Create and
// UpdateWithScopes. No-op when customerIDs is empty. Validates every id
// exists in `users` FIRST — kalau tidak, memberikan ErrCustomerNotFound yang
// jelas alih-alih membiarkan raw FK violation (23503) meledak jadi 500.
func insertDiscountCustomers(tx *gorm.DB, discountID uuid.UUID, customerIDs []uuid.UUID) error {
	if len(customerIDs) == 0 {
		return nil
	}
	if err := ensureCustomersExist(tx, customerIDs); err != nil {
		return err
	}
	values := make([]any, 0, len(customerIDs)*2)
	placeholders := ""
	for i, cid := range customerIDs {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "(?, ?)"
		values = append(values, discountID, cid)
	}
	sql := "INSERT INTO discount_customers (discount_id, customer_id) VALUES " + placeholders
	if err := tx.Exec(sql, values...).Error; err != nil {
		return fmt.Errorf("insert discount member scope: %w", err)
	}
	return nil
}

// ensureCustomersExist bulk-checks (single query) that every id in
// customerIDs exists in `users` AND is actually a customer account (mirror
// ensureProductsExist). `users` holds BOTH customers and staff (migration
// 000001, column `user_type` CHECK IN ('customer','staff')) — without this
// filter a staff/admin UUID would silently pass as a valid discount member
// scope target, get inserted into discount_customers, and then NEVER match
// any real order.customer_id (orders are always created for customer
// accounts), leaving the discount quietly unusable while admin believes it's
// configured (code review finding, §30.3). Caller is expected to have
// already de-duplicated customerIDs (service.dedupeUUIDs) — count is
// compared against len(customerIDs) exactly, so a caller passing duplicates
// would false-positive here.
func ensureCustomersExist(tx *gorm.DB, customerIDs []uuid.UUID) error {
	var count int64
	if err := tx.Table("users").
		Where("id IN ? AND user_type = ?", customerIDs, "customer").
		Count(&count).Error; err != nil {
		return fmt.Errorf("verify customer ids exist: %w", err)
	}
	if count != int64(len(customerIDs)) {
		return ErrCustomerNotFound
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == pgUniqueViolationCode
	}
	return false
}

// classifyScopeInsertErr maps an error coming out of Create/UpdateWithScopes'
// transaction closure to the right §28.9/§30.3 sentinel — checking the
// EXPLICIT bulk-existence-check sentinels first (ensureProductsExist/
// ensureCustomersExist, the normal path), then falling back to the FK
// violation's TableName (23503 — jaring pengaman kalau baris dihapus di
// antara pengecekan dan INSERT, race sempit). Returns ok=false when err
// isn't one of these — caller then wraps it as a generic internal error.
func classifyScopeInsertErr(err error) (error, bool) {
	if errors.Is(err, ErrProductNotFound) {
		return ErrProductNotFound, true
	}
	if errors.Is(err, ErrCustomerNotFound) {
		return ErrCustomerNotFound, true
	}
	if table, ok := foreignKeyViolationTable(err); ok {
		if table == "discount_customers" {
			return ErrCustomerNotFound, true
		}
		// TableName kosong/tidak dikenal default ke ErrProductNotFound —
		// cocok dengan perilaku repository ini SEBELUM discount_customers
		// ada (satu-satunya tabel cakupan waktu itu adalah discount_products).
		return ErrProductNotFound, true
	}
	return nil, false
}

// foreignKeyViolationTable reports the referencing table name of a Postgres
// FK-violation error (23503), if err is one — driver populates pgErr.
// TableName with the table the failed INSERT targeted (discount_products or
// discount_customers here), which is how classifyScopeInsertErr tells the
// two scope kinds apart without needing separate error types per call site.
func foreignKeyViolationTable(err error) (string, bool) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgForeignKeyViolationCode {
		return pgErr.TableName, true
	}
	return "", false
}
