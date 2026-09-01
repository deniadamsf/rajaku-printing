// Order multi-item (§32) — happy path + invariant tests. Kept in its own
// file per §22 (satu fungsi/concern satu tempat, jangan gemukkan
// order_service_test.go yang sudah besar).
package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/catalog/catalogapi"
	"github.com/rajaku-printing/backend/internal/discount/discountapi"
	"github.com/rajaku-printing/backend/internal/order/model"
	"github.com/rajaku-printing/backend/internal/order/orderapi"
)

// TestCreateOnlineOrder_MultiItem_HappyPath_SubtotalIsSumOfItems — §32.2:
// orders.Subtotal HARUS persis Σ item.Subtotal untuk order dengan lebih
// dari satu baris (dua ukuran/produk berbeda dalam satu order/resi).
func TestCreateOnlineOrder_MultiItem_HappyPath_SubtotalIsSumOfItems(t *testing.T) {
	productA, materialA := uuid.New(), uuid.New()
	productB, materialB := uuid.New(), uuid.New()

	store := &fakeStore{}
	catalog := &fakeCatalog{quoteResults: []*catalogapi.QuoteResult{
		{
			ProductID: productA, ProductName: "Banner Flexi 1x2m", MaterialID: materialA,
			MaterialName: "Flexi China 280gsm", PricingType: catalogapi.PricingTypePerM2,
			WidthCm: 100, HeightCm: 200, TotalPrice: 50_000,
		},
		{
			ProductID: productB, ProductName: "Banner Flexi 3x1m", MaterialID: materialB,
			MaterialName: "Flexi Korea 340gsm", PricingType: catalogapi.PricingTypePerM2,
			WidthCm: 300, HeightCm: 100, TotalPrice: 90_000,
		},
	}}
	customers := &fakeCustomers{identity: &authapi.Identity{UserID: uuid.New(), UserType: authapi.UserTypeCustomer}}
	svc := New(store, catalog, customers)

	in := CreateOnlineOrderInput{
		GuestPhone:  "081234567890",
		GuestName:   "Ani Testing",
		MetodeAmbil: model.MetodeAmbilPickup,
		Items: []CreateOnlineOrderItemInput{
			{ProductID: productA, MaterialID: materialA, WidthCm: 100, HeightCm: 200, Quantity: 1, DesignSource: model.DesignSourceUpload},
			{ProductID: productB, MaterialID: materialB, WidthCm: 300, HeightCm: 100, Quantity: 2, DesignSource: model.DesignSourceUpload},
		},
	}

	got, err := svc.CreateOnlineOrder(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if catalog.quoteCalls != 2 {
		t.Errorf("want catalog.Quote called once per item (2), got %d", catalog.quoteCalls)
	}
	if len(got.Items) != 2 {
		t.Fatalf("want 2 order_items saved, got %d", len(got.Items))
	}
	// item A: 50000 * qty1 = 50000; item B: 90000 * qty2 = 180000.
	wantSubtotal := int64(50_000 + 180_000)
	var sumItems int64
	for _, it := range got.Items {
		sumItems += it.Subtotal
	}
	if sumItems != wantSubtotal {
		t.Fatalf("Σ item.Subtotal = %d, want %d", sumItems, wantSubtotal)
	}
	if got.Subtotal != wantSubtotal {
		t.Fatalf("orders.Subtotal = %d, want %d (must equal Σ item.Subtotal, §32.2)", got.Subtotal, wantSubtotal)
	}
	if got.Total != wantSubtotal {
		t.Fatalf("orders.Total = %d, want %d (no shipping/discount yet)", got.Total, wantSubtotal)
	}
	// line_no assigned in request order, starting at 1.
	if got.Items[0].LineNo != 1 || got.Items[1].LineNo != 2 {
		t.Errorf("line_no wrong: %d, %d", got.Items[0].LineNo, got.Items[1].LineNo)
	}
}

// TestCreatePOSOrder_MultiItem_DiscountAllocation_SumMatchesOrderDiscount —
// §32.2's second invariant: Σ item.DiscountAmount HARUS persis sama dengan
// orders.DiscountAmount untuk order POS berdiskon multi-item. Alokasi
// (999/2001) sengaja TIDAK proporsional-bulat (10000 kalau proporsional dari
// 1000:2000 => 1000/3000*3000=1000 & 2000/3000*3000=2000) untuk membuktikan
// service TIDAK menghitung ulang alokasinya sendiri, cuma menyalin apa
// adanya dari discountapi.Snapshot — SELAMA jumlahnya tetap persis sama
// dengan Amount (999+2001=3000).
func TestCreatePOSOrder_MultiItem_DiscountAllocation_SumMatchesOrderDiscount(t *testing.T) {
	productA, materialA := uuid.New(), uuid.New()
	productB, materialB := uuid.New(), uuid.New()
	customerID, kasirID := uuid.New(), uuid.New()

	store := &fakeStore{}
	catalog := &fakeCatalog{quoteResults: []*catalogapi.QuoteResult{
		{ProductID: productA, ProductName: "A", MaterialID: materialA, MaterialName: "MA", TotalPrice: 1_000},
		{ProductID: productB, ProductName: "B", MaterialID: materialB, MaterialName: "MB", TotalPrice: 2_000},
	}}
	discID := uuid.New()
	resolver := &fakeDiscountResolver{snap: &discountapi.Snapshot{
		DiscountID: &discID, Code: "PROMO", Name: "Promo", Type: "nominal", Value: 3_000, Amount: 3_000,
		Allocations: []discountapi.ItemAllocation{
			{LineNo: 1, Amount: 999},
			{LineNo: 2, Amount: 2001},
		},
	}}
	svc := New(store, catalog, &fakeCustomers{})
	svc.SetDiscountResolver(resolver)

	in := orderapi.POSCreateOrderInput{
		CustomerID: customerID,
		KasirID:    kasirID,
		Items: []orderapi.POSOrderItemInput{
			{ProductID: productA, MaterialID: materialA, WidthCm: 100, HeightCm: 100, Quantity: 1, DesignSource: "upload"},
			{ProductID: productB, MaterialID: materialB, WidthCm: 100, HeightCm: 100, Quantity: 1, DesignSource: "upload"},
		},
		MetodeAmbil: "pickup",
		MetodeBayar: "cash",
		DiscountID:  &discID,
	}

	sum, err := svc.CreatePOSOrder(context.Background(), in)
	if err != nil {
		t.Fatalf("CreatePOSOrder() error = %v, want nil", err)
	}
	if sum.DiscountAmount != 3_000 {
		t.Fatalf("orders.DiscountAmount = %d, want 3000", sum.DiscountAmount)
	}
	if len(sum.Items) != 2 {
		t.Fatalf("want 2 items in summary, got %d", len(sum.Items))
	}
	var sumItemDiscount int64
	byLine := map[int]int64{}
	for _, it := range sum.Items {
		sumItemDiscount += it.DiscountAmount
		byLine[it.LineNo] = it.DiscountAmount
	}
	if sumItemDiscount != sum.DiscountAmount {
		t.Fatalf("Σ item.DiscountAmount = %d, want == orders.DiscountAmount (%d)", sumItemDiscount, sum.DiscountAmount)
	}
	if byLine[1] != 999 || byLine[2] != 2001 {
		t.Errorf("per-item allocation wrong: %+v, want {1:999, 2:2001}", byLine)
	}
	// total = subtotal(3000) - discount(3000) + shipping(0) = 0.
	if sum.Total != 0 {
		t.Errorf("total = %d, want 0", sum.Total)
	}
}

// TestCreatePOSOrder_MultiItem_DiscountAllocationMismatch_Rejected is the
// §22/§32.2 regression guard for temuan review #3: kalau
// discountapi.Snapshot.Allocations yang dikembalikan resolver TIDAK berjumlah
// persis Amount, order module WAJIB menolak menyimpan order itu SENDIRI
// (orderapi.ErrDiscountAllocationMismatch) — bukan menyalin buta angka yang
// meleset ke order_items lalu membiarkan rekap/struk beda angka tanpa ada
// yang error.
func TestCreatePOSOrder_MultiItem_DiscountAllocationMismatch_Rejected(t *testing.T) {
	productA, materialA := uuid.New(), uuid.New()
	productB, materialB := uuid.New(), uuid.New()
	customerID, kasirID := uuid.New(), uuid.New()

	store := &fakeStore{}
	catalog := &fakeCatalog{quoteResults: []*catalogapi.QuoteResult{
		{ProductID: productA, ProductName: "A", MaterialID: materialA, MaterialName: "MA", TotalPrice: 1_000},
		{ProductID: productB, ProductName: "B", MaterialID: materialB, MaterialName: "MB", TotalPrice: 2_000},
	}}
	discID := uuid.New()
	resolver := &fakeDiscountResolver{snap: &discountapi.Snapshot{
		DiscountID: &discID, Code: "PROMO", Name: "Promo", Type: "nominal", Value: 3_000, Amount: 3_000,
		// Sengaja MELESET: Σ alokasi (500+2001=2501) != Amount (3000).
		Allocations: []discountapi.ItemAllocation{
			{LineNo: 1, Amount: 500},
			{LineNo: 2, Amount: 2001},
		},
	}}
	svc := New(store, catalog, &fakeCustomers{})
	svc.SetDiscountResolver(resolver)

	in := orderapi.POSCreateOrderInput{
		CustomerID: customerID,
		KasirID:    kasirID,
		Items: []orderapi.POSOrderItemInput{
			{ProductID: productA, MaterialID: materialA, WidthCm: 100, HeightCm: 100, Quantity: 1, DesignSource: "upload"},
			{ProductID: productB, MaterialID: materialB, WidthCm: 100, HeightCm: 100, Quantity: 1, DesignSource: "upload"},
		},
		MetodeAmbil: "pickup",
		MetodeBayar: "cash",
		DiscountID:  &discID,
	}

	_, err := svc.CreatePOSOrder(context.Background(), in)
	if !errors.Is(err, orderapi.ErrDiscountAllocationMismatch) {
		t.Fatalf("CreatePOSOrder() error = %v, want ErrDiscountAllocationMismatch", err)
	}
	if store.createCalls != 0 {
		t.Errorf("must not write order to store when allocation is mismatched, createCalls=%d", store.createCalls)
	}
}

// TestCreatePOSOrder_DiscountAllocationEmpty_WithPositiveAmount_Rejected —
// mirror of the mismatch test above for the OTHER silent-failure shape:
// Amount > 0 but the resolver returned NO allocations at all.
func TestCreatePOSOrder_DiscountAllocationEmpty_WithPositiveAmount_Rejected(t *testing.T) {
	productID, materialID := uuid.New(), uuid.New()
	customerID, kasirID := uuid.New(), uuid.New()

	store := &fakeStore{}
	catalog := &fakeCatalog{quoteResult: newQuote(productID, materialID)}
	discID := uuid.New()
	resolver := &fakeDiscountResolver{snap: &discountapi.Snapshot{
		DiscountID: &discID, Code: "PROMO", Name: "Promo", Type: "nominal", Value: 10_000, Amount: 10_000,
		Allocations: nil, // kosong padahal Amount > 0
	}}
	svc := New(store, catalog, &fakeCustomers{})
	svc.SetDiscountResolver(resolver)

	in := orderapi.POSCreateOrderInput{
		CustomerID: customerID,
		KasirID:    kasirID,
		Items: []orderapi.POSOrderItemInput{
			{ProductID: productID, MaterialID: materialID, WidthCm: 100, HeightCm: 200, Quantity: 1, DesignSource: "upload"},
		},
		MetodeAmbil: "pickup",
		MetodeBayar: "cash",
		DiscountID:  &discID,
	}

	_, err := svc.CreatePOSOrder(context.Background(), in)
	if !errors.Is(err, orderapi.ErrDiscountAllocationMismatch) {
		t.Fatalf("CreatePOSOrder() error = %v, want ErrDiscountAllocationMismatch", err)
	}
}

// TestCreateOnlineOrder_ZeroItems_Rejected — §32.4 batas bawah.
func TestCreateOnlineOrder_ZeroItems_Rejected(t *testing.T) {
	svc := New(&fakeStore{}, &fakeCatalog{}, &fakeCustomers{identity: &authapi.Identity{UserID: uuid.New()}})
	in := CreateOnlineOrderInput{
		GuestPhone: "081234567890", GuestName: "Ani",
		MetodeAmbil: model.MetodeAmbilPickup,
		Items:       nil,
	}
	_, err := svc.CreateOnlineOrder(context.Background(), in)
	if !errors.Is(err, orderapi.ErrNoItems) {
		t.Fatalf("want ErrNoItems, got %v", err)
	}
}

// TestCreateOnlineOrder_TooManyItems_Rejected — §32.4 batas atas (21 > 20).
func TestCreateOnlineOrder_TooManyItems_Rejected(t *testing.T) {
	productID, materialID := uuid.New(), uuid.New()
	svc := New(&fakeStore{}, &fakeCatalog{quoteResult: newQuote(productID, materialID)},
		&fakeCustomers{identity: &authapi.Identity{UserID: uuid.New()}})

	items := make([]CreateOnlineOrderItemInput, 21)
	for i := range items {
		items[i] = CreateOnlineOrderItemInput{
			ProductID: productID, MaterialID: materialID, WidthCm: 100, HeightCm: 100,
			Quantity: 1, DesignSource: model.DesignSourceUpload,
		}
	}
	in := CreateOnlineOrderInput{
		GuestPhone: "081234567890", GuestName: "Ani",
		MetodeAmbil: model.MetodeAmbilPickup,
		Items:       items,
	}
	_, err := svc.CreateOnlineOrder(context.Background(), in)
	if !errors.Is(err, orderapi.ErrTooManyItems) {
		t.Fatalf("want ErrTooManyItems, got %v", err)
	}
}

// TestCreatePOSOrder_ZeroItems_Rejected mirrors the online-order guard for
// the POS creation path.
func TestCreatePOSOrder_ZeroItems_Rejected(t *testing.T) {
	svc := New(&fakeStore{}, &fakeCatalog{}, &fakeCustomers{})
	_, err := svc.CreatePOSOrder(context.Background(), orderapi.POSCreateOrderInput{
		CustomerID: uuid.New(), KasirID: uuid.New(),
		MetodeAmbil: "pickup", MetodeBayar: "cash", Items: nil,
	})
	if !errors.Is(err, orderapi.ErrNoItems) {
		t.Fatalf("want ErrNoItems, got %v", err)
	}
}

// ---- deriveDesignSource (§32.1) ----

func TestDeriveDesignSource_AllUpload(t *testing.T) {
	items := []model.OrderItem{
		{DesignSource: model.DesignSourceUpload},
		{DesignSource: model.DesignSourceUpload},
	}
	if got := deriveDesignSource(items); got != model.DesignSourceUpload {
		t.Fatalf("deriveDesignSource() = %q, want upload", got)
	}
}

func TestDeriveDesignSource_AllRequest(t *testing.T) {
	items := []model.OrderItem{
		{DesignSource: model.DesignSourceRequest},
		{DesignSource: model.DesignSourceRequest},
	}
	if got := deriveDesignSource(items); got != model.DesignSourceRequest {
		t.Fatalf("deriveDesignSource() = %q, want request", got)
	}
}

// TestDeriveDesignSource_Mixed_ReturnsMixed — §32.1: satu order boleh campur
// upload & request; orders.DesignSource harus "mixed" dalam kondisi itu.
func TestDeriveDesignSource_Mixed_ReturnsMixed(t *testing.T) {
	items := []model.OrderItem{
		{LineNo: 1, DesignSource: model.DesignSourceUpload},
		{LineNo: 2, DesignSource: model.DesignSourceRequest},
	}
	if got := deriveDesignSource(items); got != model.DesignSourceMixed {
		t.Fatalf("deriveDesignSource() = %q, want mixed", got)
	}
}
