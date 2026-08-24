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
// "all") atomically in one transaction. Returns ErrCodeConflict specifically
// when the partial unique index on `code` is violated.
func (r *DiscountRepository) Create(ctx context.Context, d *model.Discount, productIDs []uuid.UUID) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(d).Error; err != nil {
			return err
		}
		return insertDiscountProducts(tx, d.ID, productIDs)
	})
	if err != nil {
		if isUniqueViolation(err) {
			return ErrCodeConflict
		}
		if errors.Is(err, ErrProductNotFound) || isForeignKeyViolation(err) {
			return ErrProductNotFound
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

// UpdateWithProducts applies a partial field update AND (kalau
// replaceProducts true) mengganti SELURUH cakupan produk diskon, DALAM SATU
// TRANSAKSI (temuan review #1). Sebelum ini, Update dan ReplaceProducts
// adalah dua transaksi terpisah dipanggil berurutan dari service — kalau
// yang kedua gagal (koneksi putus, ctx timeout, produk tidak ditemukan),
// field diskon (mis. applies_to='selected') sudah ter-commit duluan TANPA
// cakupan produknya, persis state yang §28.9 larang keras. Sekarang
// keduanya hidup/mati bersama, sama seperti Create.
//
// Mengembalikan ErrNotFound kalau baris tidak ada/sudah di-soft-delete
// (hanya dicek kalau fields tidak kosong — kalau caller cuma mengganti
// cakupan produk, PK/FK constraint discount_products sudah menegakkan
// keberadaan discountID). ErrProductNotFound kalau salah satu productIDs
// tidak ada di tabel products.
func (r *DiscountRepository) UpdateWithProducts(ctx context.Context, id uuid.UUID, fields map[string]any, replaceProducts bool, productIDs []uuid.UUID) error {
	if len(fields) == 0 && !replaceProducts {
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
		if replaceProducts {
			if err := tx.Exec("DELETE FROM discount_products WHERE discount_id = ?", id).Error; err != nil {
				return fmt.Errorf("clear discount product scope: %w", err)
			}
			if err := insertDiscountProducts(tx, id, productIDs); err != nil {
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
		if errors.Is(err, ErrProductNotFound) || isForeignKeyViolation(err) {
			return ErrProductNotFound
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

// insertDiscountProducts bulk-inserts discount_products rows for
// discountID — shared by Create and UpdateWithProducts. No-op when
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

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == pgUniqueViolationCode
	}
	return false
}

// isForeignKeyViolation — jaring pengaman kalau produk terhapus di antara
// ensureProductsExist dan INSERT (race sempit); ensureProductsExist
// menangani jalur normal (produk memang tidak ada dari awal).
func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == pgForeignKeyViolationCode
	}
	return false
}
