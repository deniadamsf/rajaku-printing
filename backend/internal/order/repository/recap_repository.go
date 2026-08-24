package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// RecapFilter — filter untuk GET /admin/order-recap (§28.5). From/To adalah
// batas UTC [From, To) SUDAH dikonversi dari tanggal WIB oleh service (pola
// sama seperti ListPOSByDateRange).
type RecapFilter struct {
	From, To       time.Time
	Channel        string
	Status         string
	CreatedBy      *uuid.UUID
	DiscountID     *uuid.UUID
	OnlyDiscounted bool
	// DiscountManual — filter khusus diskon MANUAL (discount_id IS NULL AND
	// discount_amount > 0), mutually exclusive dengan DiscountID (dicek di
	// service, lihat toRecapRepoFilter). false = tanpa filter ini.
	DiscountManual bool

	// Pagination — diabaikan kalau NoLimit true (dipakai ekspor CSV, yang
	// butuh SELURUH baris yang cocok filter, bukan satu halaman).
	Page     int
	PageSize int
	NoLimit  bool
}

// RecapSummary — kartu ringkasan §28.5, dihitung atas SELURUH hasil filter
// (query agregat terpisah dari RecapList, bukan dijumlah dari satu halaman).
type RecapSummary struct {
	OrderCount    int64
	GrossSubtotal int64
	TotalDiscount int64
	TotalShipping int64
	NetTotal      int64
}

// RecapRow — satu baris tabel rekap. DiscountNameSnapshot/DiscountTypeSnapshot
// nullable persis seperti kolom snapshot di orders (§28.2) — nama/label
// akhirnya dihitung di service (order/service, fungsi discountLabel yang
// sama dipakai POS/invoice), bukan di sini.
type RecapRow struct {
	CreatedAt            time.Time
	Resi                 string
	CustomerName         *string
	ProductName          string
	Channel              string
	Status               string
	Subtotal             int64
	DiscountAmount       int64
	DiscountNameSnapshot *string
	DiscountTypeSnapshot *string
	ShippingCost         *int64
	Total                int64
	MetodeBayar          *string
	CreatedByName        *string
}

// RecapSummary computes the aggregate card (§28.5) over EVERY row matching
// f — NOT just the current page. All money figures come straight from
// orders' own snapshot columns (subtotal/discount_amount/shipping_cost/total)
// — never a JOIN to discounts for money (§28.2 lapis 3), which is exactly
// why a discount that's since been soft-deleted from the master still adds
// up correctly here.
func (r *OrderRepository) RecapSummary(ctx context.Context, f RecapFilter) (*RecapSummary, error) {
	var row struct {
		OrderCount    int64
		GrossSubtotal int64
		TotalDiscount int64
		TotalShipping int64
		NetTotal      int64
	}
	err := recapBaseQuery(r.db, ctx, f).
		Select(`COUNT(*) AS order_count,
			COALESCE(SUM(orders.subtotal), 0) AS gross_subtotal,
			COALESCE(SUM(orders.discount_amount), 0) AS total_discount,
			COALESCE(SUM(COALESCE(orders.shipping_cost, 0)), 0) AS total_shipping,
			COALESCE(SUM(orders.total), 0) AS net_total`).
		Scan(&row).Error
	if err != nil {
		return nil, fmt.Errorf("recap summary: %w", err)
	}
	return &RecapSummary{
		OrderCount:    row.OrderCount,
		GrossSubtotal: row.GrossSubtotal,
		TotalDiscount: row.TotalDiscount,
		TotalShipping: row.TotalShipping,
		NetTotal:      row.NetTotal,
	}, nil
}

// recapRowSelect is the shared SELECT list for RecapList/RecapListBatch —
// factored out so both callers stay in sync (temuan review #4b).
const recapRowSelect = `orders.created_at,
	orders.resi,
	cu.name AS customer_name,
	orders.product_name_snapshot AS product_name,
	orders.channel,
	orders.status,
	orders.subtotal,
	orders.discount_amount,
	orders.discount_name_snapshot,
	orders.discount_type_snapshot,
	orders.shipping_cost,
	orders.total,
	orders.metode_bayar,
	su.name AS created_by_name`

// RecapList returns the per-order rows for the table (§28.5), newest first.
// Paginated unless f.NoLimit (CSV export legacy path — kept for callers that
// still want a single in-memory slice; the HTTP export handler itself now
// streams via RecapListBatch, see temuan review #4b).
func (r *OrderRepository) RecapList(ctx context.Context, f RecapFilter) ([]RecapRow, error) {
	q := recapBaseQuery(r.db, ctx, f).
		Select(recapRowSelect).
		Order("orders.created_at DESC")

	if !f.NoLimit {
		page, pageSize := f.Page, f.PageSize
		if page < 1 {
			page = 1
		}
		if pageSize < 1 || pageSize > 200 {
			pageSize = 50
		}
		q = q.Offset((page - 1) * pageSize).Limit(pageSize)
	}

	var rows []RecapRow
	if err := q.Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("recap list: %w", err)
	}
	return rows, nil
}

// RecapListBatch returns a single, offset/limit-bounded batch of rows,
// ALWAYS bounded regardless of f.Page/f.PageSize/f.NoLimit — dipakai
// streaming CSV export (temuan review #4b) supaya caller bisa iterasi
// seluruh hasil filter tanpa pernah menahan satu pun salinan penuh di
// memori (beda dari RecapList(f.NoLimit=true) yang memuat SEMUA baris
// sekaligus).
func (r *OrderRepository) RecapListBatch(ctx context.Context, f RecapFilter, offset, limit int) ([]RecapRow, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("recap list batch: limit harus > 0, got %d", limit)
	}
	q := recapBaseQuery(r.db, ctx, f).
		Select(recapRowSelect).
		Order("orders.created_at DESC").
		Offset(offset).
		Limit(limit)

	var rows []RecapRow
	if err := q.Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("recap list batch: %w", err)
	}
	return rows, nil
}

// RecapKasirOption — satu kandidat filter "kasir" untuk dropdown admin
// (fitur baru §28 — GET /admin/order-recap/filters). Hanya kasir yang
// BENAR-BENAR muncul di order pada rentang tanggal itu.
type RecapKasirOption struct {
	ID   uuid.UUID
	Name string
}

// RecapDistinctKasir returns every distinct order.created_by (+ nama staff)
// that appears on an order within [from, to) — dipakai mengisi dropdown
// filter kasir (bukan kotak teks UUID).
func (r *OrderRepository) RecapDistinctKasir(ctx context.Context, from, to time.Time) ([]RecapKasirOption, error) {
	var rows []RecapKasirOption
	err := r.db.WithContext(ctx).
		Table("orders").
		Joins("LEFT JOIN users su ON su.id = orders.created_by").
		Where("orders.deleted_at IS NULL").
		Where("orders.created_at >= ? AND orders.created_at < ?", from, to).
		Where("orders.created_by IS NOT NULL").
		Select("DISTINCT orders.created_by AS id, COALESCE(su.name, '') AS name").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("recap distinct kasir: %w", err)
	}
	return rows, nil
}

// RecapDiscountSnapshotRow — satu baris mentah dari orders yang memakai
// diskon (discount_amount > 0), dipakai membangun dropdown filter diskon
// dari kolom SNAPSHOT (bukan JOIN ke tabel discounts — §28.2 lapis 3, supaya
// diskon yang sudah di-soft-delete dari master TETAP bisa dipakai filter).
type RecapDiscountSnapshotRow struct {
	DiscountID           *uuid.UUID
	DiscountNameSnapshot *string
	CreatedAt            time.Time
}

// RecapDistinctDiscounts returns every order row within [from, to) that
// actually applied a discount (discount_amount > 0), newest first — service
// layer collapses this into the deduplicated, alphabetically-sorted filter
// list (grouping manual-discount rows, discount_id IS NULL, into a single
// "Diskon manual" entry — see service.OrderRecapFilters).
func (r *OrderRepository) RecapDistinctDiscounts(ctx context.Context, from, to time.Time) ([]RecapDiscountSnapshotRow, error) {
	var rows []RecapDiscountSnapshotRow
	err := r.db.WithContext(ctx).
		Table("orders").
		Where("orders.deleted_at IS NULL").
		Where("orders.created_at >= ? AND orders.created_at < ?", from, to).
		Where("orders.discount_amount > 0").
		Select("orders.discount_id, orders.discount_name_snapshot, orders.created_at").
		Order("orders.created_at DESC").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("recap distinct discounts: %w", err)
	}
	return rows, nil
}

// recapBaseQuery builds the shared WHERE clause for RecapSummary/RecapList —
// LEFT JOIN raw ke tabel `users` (nama customer & nama kasir/pembuat) dan
// `orders.deleted_at IS NULL` (soft-deleted orders never counted as revenue,
// same invariant as every other reading query in this file).
//
// Joining `users` directly by table name (not importing auth's Go package)
// is the same allowed pattern as discount/repository's raw query against
// `orders` (§28 brief) — a cross-TABLE SQL join, not a cross-MODULE Go
// import (§22 forbids the latter, not the former).
func recapBaseQuery(db *gorm.DB, ctx context.Context, f RecapFilter) *gorm.DB {
	q := db.WithContext(ctx).
		Table("orders").
		Joins("LEFT JOIN users cu ON cu.id = orders.customer_id").
		Joins("LEFT JOIN users su ON su.id = orders.created_by").
		Where("orders.deleted_at IS NULL").
		Where("orders.created_at >= ? AND orders.created_at < ?", f.From, f.To)
	if f.Channel != "" {
		q = q.Where("orders.channel = ?", f.Channel)
	}
	if f.Status != "" {
		q = q.Where("orders.status = ?", f.Status)
	}
	if f.CreatedBy != nil {
		q = q.Where("orders.created_by = ?", *f.CreatedBy)
	}
	if f.DiscountID != nil {
		q = q.Where("orders.discount_id = ?", *f.DiscountID)
	}
	if f.DiscountManual {
		q = q.Where("orders.discount_id IS NULL AND orders.discount_amount > 0")
	}
	if f.OnlyDiscounted {
		q = q.Where("orders.discount_amount > 0")
	}
	return q
}
