// Statistik & riwayat order singkat per pelanggan — mendukung
// orderapi.CustomerOrderReader (dipakai modul auth, fitur "Manajemen
// Pelanggan"). File terpisah dari order_repository.go per §22 (satu
// tanggung jawab per file), pola sama seperti recap_repository.go.
package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/lib/pq"

	"github.com/rajaku-printing/backend/internal/order/state"
)

// CustomerOrderStatsRow — hasil agregat SATU query untuk statistik order
// milik satu pelanggan. `deleted_at IS NULL` selalu disaring (order yang
// sudah soft-deleted bukan bagian dari riwayat nyata pelanggan — sama
// invarian dengan setiap query pembaca lain di modul ini).
//
// TotalSpend menjumlahkan HANYA order yang uangnya benar-benar sudah masuk:
// mengecualikan Dibatalkan DAN semua status pre-dibayar (temuan review #8).
// Tanpa itu, pelanggan dengan lima order mangkrak di menunggu_pembayaran
// tampil sebagai "Total belanja Rp 2 juta" padahal belum membayar sepeser
// pun — dan angka itulah yang dipakai admin menimbang approval membership
// (§30.2).
type CustomerOrderStatsRow struct {
	TotalOrders     int64
	CompletedOrders int64
	CancelledOrders int64
	TotalSpend      int64
	LastOrderAt     *time.Time
}

// unpaidStatuses — status yang TIDAK boleh menyumbang TotalSpend: seluruh
// status pre-dibayar plus Dibatalkan. Daftarnya DITURUNKAN dari package
// `state` (bukan ditulis ulang sebagai literal di SQL) supaya penambahan
// status baru di §4 otomatis ikut terhitung benar di sini — daftar status
// yang diketik ulang di satu tempat persis cara angka rekap melenceng
// diam-diam tanpa ada yang sadar.
func unpaidStatuses() []string {
	out := make([]string, 0, len(state.All()))
	for _, s := range state.All() {
		if state.IsPreDibayar(s) || s == state.Dibatalkan {
			out = append(out, string(s))
		}
	}
	return out
}

// CustomerOrderStats computes the aggregate card for one customer in a
// single round-trip (COUNT/SUM ... FILTER, bukan beberapa query terpisah).
func (r *OrderRepository) CustomerOrderStats(ctx context.Context, customerID uuid.UUID) (*CustomerOrderStatsRow, error) {
	var row CustomerOrderStatsRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT
			COUNT(*) AS total_orders,
			COUNT(*) FILTER (WHERE status = ?) AS completed_orders,
			COUNT(*) FILTER (WHERE status = ?) AS cancelled_orders,
			COALESCE(SUM(total) FILTER (WHERE status <> ALL(?)), 0) AS total_spend,
			MAX(created_at) AS last_order_at
		FROM orders
		WHERE customer_id = ? AND deleted_at IS NULL
	`, state.Selesai, state.Dibatalkan, pq.Array(unpaidStatuses()), customerID).Scan(&row).Error
	if err != nil {
		return nil, fmt.Errorf("customer order stats for %s: %w", customerID, err)
	}
	return &row, nil
}

// CustomerOrderBriefRow — satu baris "order terakhir" untuk halaman detail
// pelanggan admin.
type CustomerOrderBriefRow struct {
	Resi      string
	Status    string
	Channel   string
	Total     int64
	CreatedAt time.Time
}

// RecentOrdersByCustomer returns up to `limit` most recent (non-deleted)
// orders for a customer, newest first.
func (r *OrderRepository) RecentOrdersByCustomer(ctx context.Context, customerID uuid.UUID, limit int) ([]CustomerOrderBriefRow, error) {
	if limit <= 0 {
		limit = 5
	}
	var rows []CustomerOrderBriefRow
	err := r.db.WithContext(ctx).
		Table("orders").
		Where("customer_id = ? AND deleted_at IS NULL", customerID).
		Select("resi, status, channel, total, created_at").
		Order("created_at DESC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("recent orders by customer %s: %w", customerID, err)
	}
	return rows, nil
}
