package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/order/orderapi"
	"github.com/rajaku-printing/backend/internal/order/repository"
)

func strPtrTest(s string) *string { return &s }
func int64PtrTest(v int64) *int64 { return &v }

// TestOrderRecap_Aggregation — kartu ringkasan (§28.5) dihitung dari
// repository.RecapSummary (query agregat terpisah), bukan dijumlah manual
// dari halaman `items`; average_order_value = pembagian bulat net_total/order_count.
func TestOrderRecap_Aggregation(t *testing.T) {
	store := &fakeStore{
		recapSummary: &repository.RecapSummary{
			OrderCount: 3, GrossSubtotal: 900000, TotalDiscount: 150000,
			TotalShipping: 45000, NetTotal: 795000,
		},
		recapRows: []repository.RecapRow{
			{Resi: "RJK-1", ProductName: "Banner", Channel: "pos", Status: "selesai", Subtotal: 300000, Total: 265000},
		},
	}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	loc, _ := time.LoadLocation("Asia/Jakarta")
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, loc)
	to := time.Date(2026, 8, 31, 0, 0, 0, 0, loc)

	res, err := svc.OrderRecap(context.Background(), RecapFilter{From: from, To: to})
	if err != nil {
		t.Fatalf("OrderRecap() error = %v, want nil", err)
	}
	if res.Summary.OrderCount != 3 || res.Summary.NetTotal != 795000 {
		t.Fatalf("Summary = %+v, want OrderCount 3 NetTotal 795000", res.Summary)
	}
	// 795000 / 3 = 265000 pas.
	if res.Summary.AverageOrderValue != 265000 {
		t.Fatalf("AverageOrderValue = %d, want 265000", res.Summary.AverageOrderValue)
	}
	if res.Total != 3 {
		t.Fatalf("Total (pagination) = %d, want 3 (== summary.order_count)", res.Total)
	}
	if len(res.Items) != 1 {
		t.Fatalf("Items = %d, want 1", len(res.Items))
	}
}

// TestOrderRecap_ZeroOrders_NoDivisionByZero — order_count 0 harus
// menghasilkan average_order_value 0, bukan panic/NaN.
func TestOrderRecap_ZeroOrders_NoDivisionByZero(t *testing.T) {
	store := &fakeStore{recapSummary: &repository.RecapSummary{}}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	loc, _ := time.LoadLocation("Asia/Jakarta")
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, loc)
	to := time.Date(2026, 8, 1, 0, 0, 0, 0, loc)

	res, err := svc.OrderRecap(context.Background(), RecapFilter{From: from, To: to})
	if err != nil {
		t.Fatalf("OrderRecap() error = %v, want nil", err)
	}
	if res.Summary.AverageOrderValue != 0 {
		t.Fatalf("AverageOrderValue = %d, want 0", res.Summary.AverageOrderValue)
	}
}

// TestOrderRecap_InvalidDateRange_Rejected.
func TestOrderRecap_InvalidDateRange_Rejected(t *testing.T) {
	store := &fakeStore{}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	loc, _ := time.LoadLocation("Asia/Jakarta")
	from := time.Date(2026, 8, 10, 0, 0, 0, 0, loc)
	to := time.Date(2026, 8, 1, 0, 0, 0, 0, loc) // before 'from'

	_, err := svc.OrderRecap(context.Background(), RecapFilter{From: from, To: to})
	if !errors.Is(err, orderapi.ErrRecapInvalidDateRange) {
		t.Fatalf("OrderRecap() error = %v, want ErrRecapInvalidDateRange", err)
	}
}

// ---- discount_manual (permintaan tambahan pemilik proyek — filter diskon
// manual berdiri sendiri, mutually exclusive dengan discount_id) ----

// TestOrderRecap_DiscountManual_PassesFilterThrough proves DiscountManual=true
// reaches the repository filter (WHERE discount_id IS NULL AND
// discount_amount > 0 is enforced in SQL, recapBaseQuery — this test covers
// the service-layer contract: the correct filter value is actually passed
// down, and discount_id stays nil since the two are mutually exclusive).
func TestOrderRecap_DiscountManual_PassesFilterThrough(t *testing.T) {
	store := &fakeStore{recapSummary: &repository.RecapSummary{}}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	loc, _ := time.LoadLocation("Asia/Jakarta")
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, loc)
	to := time.Date(2026, 8, 31, 0, 0, 0, 0, loc)

	_, err := svc.OrderRecap(context.Background(), RecapFilter{From: from, To: to, DiscountManual: true})
	if err != nil {
		t.Fatalf("OrderRecap() error = %v, want nil", err)
	}
	if !store.lastRecapFilter.DiscountManual {
		t.Fatalf("repository.RecapFilter.DiscountManual = false, want true")
	}
	if store.lastRecapFilter.DiscountID != nil {
		t.Fatalf("repository.RecapFilter.DiscountID = %v, want nil (mutually exclusive with DiscountManual)", store.lastRecapFilter.DiscountID)
	}
}

// TestOrderRecap_DiscountManualWithDiscountID_Rejected is the regression
// guard for the mutual-exclusivity rule: discount_manual=true AND
// discount_id sent together must be rejected explicitly (400 via
// mapRecapErr), never silently pick one and ignore the other.
func TestOrderRecap_DiscountManualWithDiscountID_Rejected(t *testing.T) {
	store := &fakeStore{}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	loc, _ := time.LoadLocation("Asia/Jakarta")
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, loc)
	to := time.Date(2026, 8, 31, 0, 0, 0, 0, loc)
	discID := uuid.New()

	_, err := svc.OrderRecap(context.Background(), RecapFilter{
		From: from, To: to, DiscountManual: true, DiscountID: &discID,
	})
	if !errors.Is(err, orderapi.ErrRecapDiscountFilterAmbiguous) {
		t.Fatalf("OrderRecap() error = %v, want ErrRecapDiscountFilterAmbiguous", err)
	}
}

// TestOrderRecapExportStream_DiscountManual_PassesFilterThrough — sama
// seperti di atas tapi untuk jalur ekspor CSV streaming (§4b).
func TestOrderRecapExportStream_DiscountManual_PassesFilterThrough(t *testing.T) {
	store := &fakeStore{}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	loc, _ := time.LoadLocation("Asia/Jakarta")
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, loc)
	to := time.Date(2026, 8, 31, 0, 0, 0, 0, loc)

	err := svc.OrderRecapExportStream(context.Background(), RecapFilter{
		From: from, To: to, DiscountManual: true,
	}, func(RecapRow) error { return nil })
	if err != nil {
		t.Fatalf("OrderRecapExportStream() error = %v, want nil", err)
	}
	if !store.lastRecapFilter.DiscountManual {
		t.Fatalf("repository.RecapFilter.DiscountManual = false, want true")
	}
}

// TestOrderRecapExportStream_DiscountManualWithDiscountID_Rejected — sama
// aturan mutual exclusivity, tapi lewat jalur ekspor.
func TestOrderRecapExportStream_DiscountManualWithDiscountID_Rejected(t *testing.T) {
	store := &fakeStore{}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	loc, _ := time.LoadLocation("Asia/Jakarta")
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, loc)
	to := time.Date(2026, 8, 31, 0, 0, 0, 0, loc)
	discID := uuid.New()

	err := svc.OrderRecapExportStream(context.Background(), RecapFilter{
		From: from, To: to, DiscountManual: true, DiscountID: &discID,
	}, func(RecapRow) error { return nil })
	if !errors.Is(err, orderapi.ErrRecapDiscountFilterAmbiguous) {
		t.Fatalf("OrderRecapExportStream() error = %v, want ErrRecapDiscountFilterAmbiguous", err)
	}
}

// TestOrderRecap_DeletedDiscountMaster_StillShowsInRecap — REGRESI PALING
// PENTING (brief §6, inti permintaan pemilik proyek): sebuah order yang
// diskonnya sudah di-soft-delete dari master `discounts` TETAP muncul
// lengkap di rekap dengan discount_amount & discount_label yang benar.
//
// Ini disimulasikan di level fake dengan sengaja TIDAK menyediakan cara
// apa pun bagi fakeStore untuk mengetahui keberadaan baris `discounts` — row
// yang dikembalikan RecapList HANYA berisi kolom SNAPSHOT milik `orders`
// (discount_name_snapshot/discount_amount), yang dijamin oleh
// recapBaseQuery() (order/repository/recap_repository.go) tidak pernah
// JOIN ke tabel discounts sama sekali (§28.2 lapis 3) — jadi baris ini
// identik persis dengan apa yang akan dikembalikan real repository untuk
// order dengan diskon yang masternya sudah dihapus.
func TestOrderRecap_DeletedDiscountMaster_StillShowsInRecap(t *testing.T) {
	store := &fakeStore{
		recapSummary: &repository.RecapSummary{OrderCount: 1, GrossSubtotal: 250000, TotalDiscount: 50000, NetTotal: 200000},
		recapRows: []repository.RecapRow{
			{
				Resi: "RJK-OLDPROMO", ProductName: "Banner", Channel: "pos", Status: "selesai",
				Subtotal: 250000, DiscountAmount: 50000,
				DiscountNameSnapshot: strPtrTest("Promo Lebaran (sudah dihapus)"),
				DiscountTypeSnapshot: strPtrTest("percent"),
				ShippingCost:         int64PtrTest(0),
				Total:                200000,
			},
		},
	}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	loc, _ := time.LoadLocation("Asia/Jakarta")
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	to := time.Date(2026, 12, 31, 0, 0, 0, 0, loc)

	res, err := svc.OrderRecap(context.Background(), RecapFilter{From: from, To: to})
	if err != nil {
		t.Fatalf("OrderRecap() error = %v, want nil", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("Items = %d, want 1", len(res.Items))
	}
	row := res.Items[0]
	if row.DiscountAmount != 50000 {
		t.Fatalf("row.DiscountAmount = %d, want 50000 (still present despite deleted master)", row.DiscountAmount)
	}
	if row.DiscountLabel != "Promo Lebaran (sudah dihapus)" {
		t.Fatalf("row.DiscountLabel = %q, want the snapshot name, unaffected by master deletion", row.DiscountLabel)
	}
	if row.Total != 200000 {
		t.Fatalf("row.Total = %d, want 200000", row.Total)
	}
}

// TestOrderRecapExportStream_StreamsEveryRow is the regression guard for
// review finding #4b: export must stream via RecapListBatch (bounded
// offset/limit), not load every matching row into one []RecapRow slice —
// asserts EVERY row reaches writeRow, across multiple batch calls.
func TestOrderRecapExportStream_StreamsEveryRow(t *testing.T) {
	rows := make([]repository.RecapRow, 0, 3)
	for i := 0; i < 3; i++ {
		rows = append(rows, repository.RecapRow{Resi: "RJK-" + string(rune('1'+i))})
	}
	store := &fakeStore{recapRows: rows}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	loc, _ := time.LoadLocation("Asia/Jakarta")
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, loc)
	to := time.Date(2026, 8, 31, 0, 0, 0, 0, loc)

	var got []RecapRow
	err := svc.OrderRecapExportStream(context.Background(), RecapFilter{From: from, To: to}, func(r RecapRow) error {
		got = append(got, r)
		return nil
	})
	if err != nil {
		t.Fatalf("OrderRecapExportStream() error = %v, want nil", err)
	}
	if len(got) != 3 {
		t.Fatalf("OrderRecapExportStream() streamed %d rows, want 3", len(got))
	}
	// The batch call must have been offset/limit bounded (real repository
	// query), never the unbounded f.NoLimit=true path the old implementation
	// used.
	if len(store.recapBatchCalls) == 0 {
		t.Fatalf("OrderRecapExportStream() never called RecapListBatch")
	}
	if store.recapBatchCalls[0].Limit != recapExportBatchSize {
		t.Fatalf("OrderRecapExportStream() batch limit = %d, want %d", store.recapBatchCalls[0].Limit, recapExportBatchSize)
	}
	if store.lastRecapFilter.NoLimit {
		t.Fatalf("OrderRecapExportStream() must NOT rely on repository.RecapFilter.NoLimit")
	}
}

// TestOrderRecapExportStream_StopsOnWriterError — kasus gagal: a write error
// mid-stream (e.g. client disconnected) must abort immediately, not keep
// pulling more batches.
func TestOrderRecapExportStream_StopsOnWriterError(t *testing.T) {
	store := &fakeStore{recapRows: []repository.RecapRow{{Resi: "RJK-1"}, {Resi: "RJK-2"}}}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	loc, _ := time.LoadLocation("Asia/Jakarta")
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, loc)
	to := time.Date(2026, 8, 31, 0, 0, 0, 0, loc)

	writeErr := errors.New("client disconnected")
	calls := 0
	err := svc.OrderRecapExportStream(context.Background(), RecapFilter{From: from, To: to}, func(RecapRow) error {
		calls++
		return writeErr
	})
	if err == nil || !errors.Is(err, writeErr) {
		t.Fatalf("OrderRecapExportStream() error = %v, want wrapped writeErr", err)
	}
	if calls != 1 {
		t.Fatalf("OrderRecapExportStream() writeRow called %d times, want 1 (must abort on first error)", calls)
	}
}

// TestOrderRecapExportStream_DateRangeTooWide_Rejected is the regression
// guard for review finding #4a.
func TestOrderRecapExportStream_DateRangeTooWide_Rejected(t *testing.T) {
	store := &fakeStore{}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	loc, _ := time.LoadLocation("Asia/Jakarta")
	from := time.Date(2020, 1, 1, 0, 0, 0, 0, loc)
	to := time.Date(2026, 1, 1, 0, 0, 0, 0, loc) // ~6 tahun, jauh > 366 hari

	err := svc.OrderRecapExportStream(context.Background(), RecapFilter{From: from, To: to}, func(RecapRow) error {
		return nil
	})
	if !errors.Is(err, orderapi.ErrRecapDateRangeTooWide) {
		t.Fatalf("OrderRecapExportStream() error = %v, want ErrRecapDateRangeTooWide", err)
	}
}

// TestOrderRecap_DateRangeTooWide_Rejected — batas yang sama berlaku untuk
// endpoint JSON (bukan cuma export), lihat review finding #4a.
func TestOrderRecap_DateRangeTooWide_Rejected(t *testing.T) {
	store := &fakeStore{}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	loc, _ := time.LoadLocation("Asia/Jakarta")
	from := time.Date(2020, 1, 1, 0, 0, 0, 0, loc)
	to := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)

	_, err := svc.OrderRecap(context.Background(), RecapFilter{From: from, To: to})
	if !errors.Is(err, orderapi.ErrRecapDateRangeTooWide) {
		t.Fatalf("OrderRecap() error = %v, want ErrRecapDateRangeTooWide", err)
	}
}

// TestOrderRecap_InvalidStatusFilter_Rejected is the regression guard for
// review finding #5: an unknown status must be 400, not silently 0 rows.
func TestOrderRecap_InvalidStatusFilter_Rejected(t *testing.T) {
	store := &fakeStore{}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	loc, _ := time.LoadLocation("Asia/Jakarta")
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, loc)
	to := time.Date(2026, 8, 31, 0, 0, 0, 0, loc)

	_, err := svc.OrderRecap(context.Background(), RecapFilter{From: from, To: to, Status: "dibayarkan"})
	if !errors.Is(err, orderapi.ErrRecapInvalidStatus) {
		t.Fatalf("OrderRecap() error = %v, want ErrRecapInvalidStatus", err)
	}
}

// TestOrderRecap_InvalidChannelFilter_Rejected — sama seperti di atas untuk channel.
func TestOrderRecap_InvalidChannelFilter_Rejected(t *testing.T) {
	store := &fakeStore{}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	loc, _ := time.LoadLocation("Asia/Jakarta")
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, loc)
	to := time.Date(2026, 8, 31, 0, 0, 0, 0, loc)

	_, err := svc.OrderRecap(context.Background(), RecapFilter{From: from, To: to, Channel: "POS"})
	if !errors.Is(err, orderapi.ErrRecapInvalidChannel) {
		t.Fatalf("OrderRecap() error = %v, want ErrRecapInvalidChannel", err)
	}
}

// ---- OrderRecapFilters (fitur baru §28) ----

func TestOrderRecapFilters_HappyPath_SortedAlphabetically(t *testing.T) {
	kasirA, kasirB := uuid.New(), uuid.New()
	discA, discB := uuid.New(), uuid.New()
	store := &fakeStore{
		recapKasirOptions: []repository.RecapKasirOption{
			{ID: kasirB, Name: "Zaenal"},
			{ID: kasirA, Name: "Budi"},
		},
		recapDiscountRows: []repository.RecapDiscountSnapshotRow{
			{DiscountID: &discB, DiscountNameSnapshot: strPtrTest("Promo Natal"), CreatedAt: time.Now()},
			{DiscountID: &discA, DiscountNameSnapshot: strPtrTest("Promo Lebaran"), CreatedAt: time.Now().Add(-time.Hour)},
			{DiscountID: nil, CreatedAt: time.Now().Add(-2 * time.Hour)}, // diskon manual
		},
	}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	loc, _ := time.LoadLocation("Asia/Jakarta")
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, loc)
	to := time.Date(2026, 8, 31, 0, 0, 0, 0, loc)

	res, err := svc.OrderRecapFilters(context.Background(), from, to)
	if err != nil {
		t.Fatalf("OrderRecapFilters() error = %v, want nil", err)
	}
	if len(res.Kasir) != 2 || res.Kasir[0].Name != "Budi" || res.Kasir[1].Name != "Zaenal" {
		t.Fatalf("Kasir = %+v, want [Budi, Zaenal] sorted", res.Kasir)
	}
	if len(res.Diskon) != 3 {
		t.Fatalf("Diskon = %+v, want 3 entries (2 master + 1 manual)", res.Diskon)
	}
	// alphabetis: "Diskon manual" < "Promo Lebaran" < "Promo Natal"
	if res.Diskon[0].Label != manualDiscountLabel || res.Diskon[0].ID != nil {
		t.Fatalf("Diskon[0] = %+v, want manual entry (id nil) first alphabetically", res.Diskon[0])
	}
	if res.Diskon[1].Label != "Promo Lebaran" || res.Diskon[2].Label != "Promo Natal" {
		t.Fatalf("Diskon[1:] = %+v, want [Promo Lebaran, Promo Natal]", res.Diskon[1:])
	}
}

// TestOrderRecapFilters_DeletedDiscountMaster_StillListed — sama seperti
// TestOrderRecap_DeletedDiscountMaster_StillShowsInRecap: dropdown filter
// diskon dibangun dari SNAPSHOT, jadi diskon yang masternya sudah dihapus
// tetap muncul sebagai pilihan filter.
func TestOrderRecapFilters_DeletedDiscountMaster_StillListed(t *testing.T) {
	discID := uuid.New()
	store := &fakeStore{
		recapDiscountRows: []repository.RecapDiscountSnapshotRow{
			{DiscountID: &discID, DiscountNameSnapshot: strPtrTest("Promo Lebaran (sudah dihapus)"), CreatedAt: time.Now()},
		},
	}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	loc, _ := time.LoadLocation("Asia/Jakarta")
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	to := time.Date(2026, 12, 31, 0, 0, 0, 0, loc)

	res, err := svc.OrderRecapFilters(context.Background(), from, to)
	if err != nil {
		t.Fatalf("OrderRecapFilters() error = %v, want nil", err)
	}
	if len(res.Diskon) != 1 || res.Diskon[0].Label != "Promo Lebaran (sudah dihapus)" {
		t.Fatalf("Diskon = %+v, want the deleted master's snapshot name still listed", res.Diskon)
	}
	if res.Diskon[0].ID == nil || *res.Diskon[0].ID != discID {
		t.Fatalf("Diskon[0].ID = %v, want %s", res.Diskon[0].ID, discID)
	}
}

// TestOrderRecapFilters_DateRangeTooWide_Rejected — batas 366 hari yang sama
// berlaku di endpoint filters (review finding #4a).
func TestOrderRecapFilters_DateRangeTooWide_Rejected(t *testing.T) {
	store := &fakeStore{}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	loc, _ := time.LoadLocation("Asia/Jakarta")
	from := time.Date(2020, 1, 1, 0, 0, 0, 0, loc)
	to := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)

	_, err := svc.OrderRecapFilters(context.Background(), from, to)
	if !errors.Is(err, orderapi.ErrRecapDateRangeTooWide) {
		t.Fatalf("OrderRecapFilters() error = %v, want ErrRecapDateRangeTooWide", err)
	}
}
