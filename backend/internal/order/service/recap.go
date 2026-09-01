// Order recap (§28.5) — read-only report over `orders`, lives in the order
// module per §28 brief (NOT a new module). Kept in its own file per §22 (satu
// fungsi satu tanggung jawab / jangan gemukkan order_service.go).
package service

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/order/orderapi"
	"github.com/rajaku-printing/backend/internal/order/repository"
	"github.com/rajaku-printing/backend/internal/order/state"
)

// maxRecapRangeDays — batas lebar rentang tanggal 'from'..'to' (temuan
// review #4a) untuk GET /admin/order-recap, /export, DAN /filters — tanpa
// batas, `?from=2000-01-01&to=2099-12-31` bisa menahan seluruh tabel order
// di memori sekaligus (VPS Hostinger tunggal, §19, tidak auto-scale).
const maxRecapRangeDays = 366

// recapExportBatchSize — ukuran satu batch saat streaming CSV export
// (temuan review #4b): cukup besar untuk throughput query yang wajar, cukup
// kecil supaya tidak ada satu pun titik yang menahan seluruh hasil filter
// sekaligus di memori.
const recapExportBatchSize = 1000

// RecapFilter — input service.OrderRecap/OrderRecapExport. From/To adalah
// TANGGAL (bukan datetime) yang ditafsirkan zona WIB — service yang
// mengubahnya jadi window UTC [00:00 WIB 'from', 00:00 WIB hari SETELAH
// 'to') supaya seluruh hari 'to' ikut kehitung (pola sama seperti
// ListPOSOrdersByDate).
type RecapFilter struct {
	From, To       time.Time
	Channel        string
	Status         string
	CreatedBy      *uuid.UUID
	DiscountID     *uuid.UUID
	OnlyDiscounted bool
	// DiscountManual — saring order dengan diskon MANUAL saja (discount_id
	// IS NULL AND discount_amount > 0). Mutually exclusive dengan
	// DiscountID — mengisi keduanya sekaligus ditolak dengan
	// orderapi.ErrRecapDiscountFilterAmbiguous (permintaan frontend rekap).
	DiscountManual bool
	Page           int
	PageSize       int
}

// RecapSummary — kartu ringkasan (§28.5), JSON tags dikembalikan langsung
// oleh handler (pola sama seperti pos.service.CreateOrderResult).
type RecapSummary struct {
	OrderCount        int64 `json:"order_count"`
	GrossSubtotal     int64 `json:"gross_subtotal"`
	TotalDiscount     int64 `json:"total_discount"`
	TotalShipping     int64 `json:"total_shipping"`
	NetTotal          int64 `json:"net_total"`
	AverageOrderValue int64 `json:"average_order_value"`
}

// RecapRow — satu baris tabel/CSV (§28.5). ProductName sudah final untuk
// tampilan (nama item line_no=1 + akhiran "+N lainnya" kalau ItemCount > 1,
// §32.8) — konsumer (handler CSV) tinggal pakai apa adanya.
type RecapRow struct {
	CreatedAt      string `json:"created_at"`
	Resi           string `json:"resi"`
	CustomerName   string `json:"customer_name,omitempty"`
	ProductName    string `json:"product_name"`
	ItemCount      int64  `json:"jumlah_item"`
	Channel        string `json:"channel"`
	Status         string `json:"status"`
	Subtotal       int64  `json:"subtotal"`
	DiscountAmount int64  `json:"discount_amount"`
	DiscountLabel  string `json:"discount_label,omitempty"`
	ShippingCost   int64  `json:"shipping_cost"`
	Total          int64  `json:"total"`
	MetodeBayar    string `json:"metode_bayar,omitempty"`
	CreatedByName  string `json:"created_by_name,omitempty"`
}

// RecapResult — GET /admin/order-recap response body.
type RecapResult struct {
	Summary RecapSummary `json:"summary"`
	Items   []RecapRow   `json:"items"`
	Total   int64        `json:"total"`
	Page    int          `json:"page"`
	PerPage int          `json:"per_page"`
}

// OrderRecap implements GET /admin/order-recap (§28.5). Summary dihitung atas
// SELURUH hasil filter (query agregat terpisah); `total` (utk paginasi) sama
// dengan summary.OrderCount — SATU query count, bukan dihitung dua kali.
func (s *Service) OrderRecap(ctx context.Context, f RecapFilter) (*RecapResult, error) {
	repoFilter, err := toRecapRepoFilter(f)
	if err != nil {
		return nil, err
	}

	summary, err := s.orders.RecapSummary(ctx, repoFilter)
	if err != nil {
		return nil, fmt.Errorf("order recap: summary: %w", err)
	}
	rows, err := s.orders.RecapList(ctx, repoFilter)
	if err != nil {
		return nil, fmt.Errorf("order recap: list: %w", err)
	}

	page, pageSize := normalizeRecapPaging(f.Page, f.PageSize)
	return &RecapResult{
		Summary: toRecapSummaryView(summary),
		Items:   toRecapRowViews(rows),
		Total:   summary.OrderCount,
		Page:    page,
		PerPage: pageSize,
	}, nil
}

// OrderRecapExportStream implements GET /admin/order-recap/export (§28.5) —
// streams EVERY row matching f to writeRow, batch by batch
// (recapExportBatchSize at a time via RecapListBatch), so the caller (HTTP
// handler) never has to hold the whole result set in memory (temuan review
// #4b — the old OrderRecapExport loaded ALL matching rows into a
// []RecapRow, then the handler serialized that into a second full []byte
// CSV buffer: THREE full in-memory copies of an unbounded table scan for one
// request). Stops as soon as writeRow returns an error (e.g. the client
// disconnected mid-stream) or every row has been sent.
func (s *Service) OrderRecapExportStream(ctx context.Context, f RecapFilter, writeRow func(RecapRow) error) error {
	repoFilter, err := toRecapRepoFilter(f)
	if err != nil {
		return err
	}

	offset := 0
	for {
		batch, err := s.orders.RecapListBatch(ctx, repoFilter, offset, recapExportBatchSize)
		if err != nil {
			return fmt.Errorf("order recap export: batch offset=%d: %w", offset, err)
		}
		for i := range batch {
			if err := writeRow(toRecapRowView(&batch[i])); err != nil {
				return fmt.Errorf("order recap export: write row: %w", err)
			}
		}
		if len(batch) < recapExportBatchSize {
			return nil
		}
		offset += recapExportBatchSize
	}
}

// toRecapRepoFilter converts the WIB-dates filter into a UTC [from, to)
// window for the repository — reuses jakartaTZ() (order_service.go).
func toRecapRepoFilter(f RecapFilter) (repository.RecapFilter, error) {
	if err := validateRecapFilterValues(f.Channel, f.Status); err != nil {
		return repository.RecapFilter{}, err
	}
	// discount_manual & discount_id mutually exclusive (permintaan frontend
	// rekap) — ditolak eksplisit, bukan diam-diam mengabaikan salah satunya.
	if f.DiscountManual && f.DiscountID != nil {
		return repository.RecapFilter{}, orderapi.ErrRecapDiscountFilterAmbiguous
	}
	start, end, err := recapDateWindow(f.From, f.To)
	if err != nil {
		return repository.RecapFilter{}, err
	}
	return repository.RecapFilter{
		From:           start,
		To:             end,
		Channel:        f.Channel,
		Status:         f.Status,
		CreatedBy:      f.CreatedBy,
		DiscountID:     f.DiscountID,
		OnlyDiscounted: f.OnlyDiscounted,
		DiscountManual: f.DiscountManual,
		Page:           f.Page,
		PageSize:       f.PageSize,
	}, nil
}

// recapDateWindow converts WIB dates from/to into a UTC [start, end) window
// and enforces both invariants shared by every rekap endpoint (JSON, CSV
// export, filters dropdown — temuan review #4a): 'to' >= 'from', and the
// span capped at maxRecapRangeDays.
func recapDateWindow(fromDate, toDate time.Time) (start, end time.Time, err error) {
	loc := jakartaTZ()
	from := fromDate.In(loc)
	to := toDate.In(loc)
	start = time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, loc)
	end = time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, loc).Add(24 * time.Hour)
	if !end.After(start) {
		return time.Time{}, time.Time{}, orderapi.ErrRecapInvalidDateRange
	}
	if end.Sub(start) > maxRecapRangeDays*24*time.Hour {
		return time.Time{}, time.Time{}, orderapi.ErrRecapDateRangeTooWide
	}
	return start.UTC(), end.UTC(), nil
}

// validateRecapFilterValues rejects channel/status filters that aren't part
// of the official vocabulary (§4 status list, "online"/"pos" channel —
// temuan review #5) — a typo like status=dibayarkan or channel=POS must
// come back as 400, not silently 0 rows indistinguishable from "genuinely no
// orders match". Empty string means "no filter", always valid.
func validateRecapFilterValues(channel, status string) error {
	if channel != "" && channel != "online" && channel != "pos" {
		return orderapi.ErrRecapInvalidChannel
	}
	if status != "" && !state.IsKnown(state.Status(status)) {
		return orderapi.ErrRecapInvalidStatus
	}
	return nil
}

func normalizeRecapPaging(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 50
	}
	return page, pageSize
}

func toRecapSummaryView(s *repository.RecapSummary) RecapSummary {
	var avg int64
	if s.OrderCount > 0 {
		// Pembagian bulat, bukan pembulatan — §28.5.
		avg = s.NetTotal / s.OrderCount
	}
	return RecapSummary{
		OrderCount:        s.OrderCount,
		GrossSubtotal:     s.GrossSubtotal,
		TotalDiscount:     s.TotalDiscount,
		TotalShipping:     s.TotalShipping,
		NetTotal:          s.NetTotal,
		AverageOrderValue: avg,
	}
}

func toRecapRowViews(rows []repository.RecapRow) []RecapRow {
	out := make([]RecapRow, 0, len(rows))
	for i := range rows {
		out = append(out, toRecapRowView(&rows[i]))
	}
	return out
}

func toRecapRowView(r *repository.RecapRow) RecapRow {
	productName := r.ProductName
	if r.ItemCount > 1 {
		productName = fmt.Sprintf("%s +%d lainnya", productName, r.ItemCount-1)
	}
	row := RecapRow{
		CreatedAt:      r.CreatedAt.UTC().Format(time.RFC3339),
		Resi:           r.Resi,
		ProductName:    productName,
		ItemCount:      r.ItemCount,
		Channel:        r.Channel,
		Status:         r.Status,
		Subtotal:       r.Subtotal,
		DiscountAmount: r.DiscountAmount,
		Total:          r.Total,
	}
	if r.CustomerName != nil {
		row.CustomerName = *r.CustomerName
	}
	if r.ShippingCost != nil {
		row.ShippingCost = *r.ShippingCost
	}
	if r.MetodeBayar != nil {
		row.MetodeBayar = *r.MetodeBayar
	}
	if r.CreatedByName != nil {
		row.CreatedByName = *r.CreatedByName
	}
	// Label sama seperti discountLabel(o *model.Order) di order_service.go —
	// duplikasi tak terhindarkan karena RecapRow bukan model.Order (proyeksi
	// SQL raw dengan JOIN), tapi ATURANNYA identik dan sengaja disinkronkan:
	// name snapshot kalau ada, "Diskon" untuk manual, "" kalau amount 0.
	if r.DiscountAmount > 0 {
		if r.DiscountNameSnapshot != nil && *r.DiscountNameSnapshot != "" {
			row.DiscountLabel = *r.DiscountNameSnapshot
		} else {
			row.DiscountLabel = "Diskon"
		}
	}
	return row
}

// --- GET /admin/order-recap/filters (fitur baru §28) ---

// RecapKasirFilterOption — satu entri dropdown "kasir".
type RecapKasirFilterOption struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// RecapDiscountFilterOption — satu entri dropdown "diskon". ID nil + Label
// "Diskon manual" mewakili SEMUA order dengan discount_id NULL tapi
// discount_amount > 0 (kasir input nominal manual, bukan master diskon).
type RecapDiscountFilterOption struct {
	ID    *uuid.UUID `json:"id"`
	Label string     `json:"label"`
}

// RecapFiltersResult — GET /admin/order-recap/filters response body.
type RecapFiltersResult struct {
	Kasir  []RecapKasirFilterOption    `json:"kasir"`
	Diskon []RecapDiscountFilterOption `json:"diskon"`
}

const manualDiscountLabel = "Diskon manual"

// OrderRecapFilters implements GET /admin/order-recap/filters — mengisi
// dropdown kasir/diskon dari NILAI YANG BENAR-BENAR MUNCUL di order pada
// rentang tanggal itu, bukan daftar statis. Sesuai §28.2 lapis 3: daftar
// diskon diambil dari kolom SNAPSHOT di orders (discount_id +
// discount_name_snapshot), TIDAK PERNAH JOIN ke tabel discounts — supaya
// diskon yang sudah dihapus dari master tetap muncul sebagai pilihan filter
// (order lama yang memakainya masih perlu bisa difilter).
func (s *Service) OrderRecapFilters(ctx context.Context, fromDate, toDate time.Time) (*RecapFiltersResult, error) {
	start, end, err := recapDateWindow(fromDate, toDate)
	if err != nil {
		return nil, err
	}

	kasirRows, err := s.orders.RecapDistinctKasir(ctx, start, end)
	if err != nil {
		return nil, fmt.Errorf("order recap filters: kasir: %w", err)
	}
	discountRows, err := s.orders.RecapDistinctDiscounts(ctx, start, end)
	if err != nil {
		return nil, fmt.Errorf("order recap filters: diskon: %w", err)
	}

	return &RecapFiltersResult{
		Kasir:  toRecapKasirOptions(kasirRows),
		Diskon: toRecapDiscountOptions(discountRows),
	}, nil
}

func toRecapKasirOptions(rows []repository.RecapKasirOption) []RecapKasirFilterOption {
	out := make([]RecapKasirFilterOption, 0, len(rows))
	for _, r := range rows {
		out = append(out, RecapKasirFilterOption{ID: r.ID, Name: r.Name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// toRecapDiscountOptions collapses raw snapshot rows into one entry per
// discount_id, PLUS a single synthetic "Diskon manual" entry (id nil) if any
// row has discount_id IS NULL (manual kasir discount). rows arrive newest
// first (repository orders by created_at DESC), so the first row seen per
// discount_id wins its label — good enough for a filter dropdown even if a
// discount's name snapshot happened to vary across orders using the same
// discount_id (extremely unlikely: a snapshot is fixed at order-creation
// time, only the CURRENT-newest order's version needs to be shown here).
func toRecapDiscountOptions(rows []repository.RecapDiscountSnapshotRow) []RecapDiscountFilterOption {
	seen := make(map[uuid.UUID]string, len(rows))
	order := make([]uuid.UUID, 0, len(rows))
	hasManual := false
	for _, r := range rows {
		if r.DiscountID == nil {
			hasManual = true
			continue
		}
		if _, ok := seen[*r.DiscountID]; ok {
			continue
		}
		label := "Diskon"
		if r.DiscountNameSnapshot != nil && *r.DiscountNameSnapshot != "" {
			label = *r.DiscountNameSnapshot
		}
		seen[*r.DiscountID] = label
		order = append(order, *r.DiscountID)
	}

	out := make([]RecapDiscountFilterOption, 0, len(order)+1)
	for _, id := range order {
		id := id
		out = append(out, RecapDiscountFilterOption{ID: &id, Label: seen[id]})
	}
	if hasManual {
		out = append(out, RecapDiscountFilterOption{ID: nil, Label: manualDiscountLabel})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	return out
}
