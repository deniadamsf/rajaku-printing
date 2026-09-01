package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/discount/discountapi"
	"github.com/rajaku-printing/backend/internal/order/model"
	"github.com/rajaku-printing/backend/internal/order/orderapi"
	"github.com/rajaku-printing/backend/internal/order/state"
)

// fakeDiscountResolver — minimal discountapi.Resolver fake for order-service
// tests exercising the CreatePOSOrder discount integration (§28.3).
type fakeDiscountResolver struct {
	snap      *discountapi.Snapshot
	err       error
	lastInput discountapi.ResolveInput
	calls     int
}

func (f *fakeDiscountResolver) ResolveForOrder(_ context.Context, in discountapi.ResolveInput) (*discountapi.Snapshot, error) {
	f.calls++
	f.lastInput = in
	if f.err != nil {
		return nil, f.err
	}
	if f.snap == nil {
		return &discountapi.Snapshot{}, nil
	}
	return f.snap, nil
}

func posInput(productID, materialID, customerID, kasirID uuid.UUID) orderapi.POSCreateOrderInput {
	return orderapi.POSCreateOrderInput{
		CustomerID: customerID,
		KasirID:    kasirID,
		Items: []orderapi.POSOrderItemInput{{
			ProductID:    productID,
			MaterialID:   materialID,
			WidthCm:      100,
			HeightCm:     200,
			Quantity:     1,
			DesignSource: "upload",
		}},
		MetodeAmbil: "pickup",
		MetodeBayar: "cash",
	}
}

// TestCreatePOSOrder_TotalFormula_WithMasterDiscount — §28.3:
// total = subtotal - discount_amount + shipping_cost.
func TestCreatePOSOrder_TotalFormula_WithMasterDiscount(t *testing.T) {
	productID, materialID := uuid.New(), uuid.New()
	customerID, kasirID := uuid.New(), uuid.New()
	store := &fakeStore{}
	catalog := &fakeCatalog{quoteResult: newQuote(productID, materialID)} // TotalPrice 50000, qty 1 -> subtotal 50000
	discID := uuid.New()
	resolver := &fakeDiscountResolver{snap: &discountapi.Snapshot{
		DiscountID: &discID, Code: "PROMO10", Name: "Promo 10rb",
		Type: "nominal", Value: 10000, Amount: 10000,
		// Allocations wajib diisi (§32.2/§32.3) — resolver asli SELALU
		// mengembalikan satu entri per item; fixture ini meniru kontrak itu
		// supaya order/service.applyItemDiscountAllocations (guard §22
		// terhadap Σ alokasi != discount_amount) tidak menolaknya.
		Allocations: []discountapi.ItemAllocation{{LineNo: 1, Amount: 10000}},
	}}
	svc := New(store, catalog, &fakeCustomers{})
	svc.SetDiscountResolver(resolver)

	in := posInput(productID, materialID, customerID, kasirID)
	in.DiscountID = &discID

	sum, err := svc.CreatePOSOrder(context.Background(), in)
	if err != nil {
		t.Fatalf("CreatePOSOrder() error = %v, want nil", err)
	}
	if sum.Total != 40000 { // 50000 - 10000 + 0 shipping (pickup)
		t.Fatalf("CreatePOSOrder() total = %d, want 40000", sum.Total)
	}
	if store.saved.DiscountAmount != 10000 {
		t.Fatalf("saved order discount_amount = %d, want 10000", store.saved.DiscountAmount)
	}
	if store.saved.DiscountID == nil || *store.saved.DiscountID != discID {
		t.Fatalf("saved order discount_id = %v, want %s", store.saved.DiscountID, discID)
	}
	if store.saved.DiscountNameSnapshot == nil || *store.saved.DiscountNameSnapshot != "Promo 10rb" {
		t.Fatalf("saved order discount_name_snapshot = %v, want Promo 10rb", store.saved.DiscountNameSnapshot)
	}
	if len(resolver.lastInput.Items) != 1 || resolver.lastInput.Items[0].Subtotal != 50000 || resolver.lastInput.Channel != "pos" {
		t.Fatalf("resolver input = %+v, want 1 item subtotal=50000 channel=pos", resolver.lastInput)
	}
}

// TestCreatePOSOrder_ManualDiscount_NoteRequired — kasus gagal: diskon
// manual tanpa note ditolak (bubbles straight from discountapi via the
// resolver — order service doesn't re-validate it, just propagates).
func TestCreatePOSOrder_ManualDiscount_NoteRequired(t *testing.T) {
	productID, materialID := uuid.New(), uuid.New()
	customerID, kasirID := uuid.New(), uuid.New()
	store := &fakeStore{}
	catalog := &fakeCatalog{quoteResult: newQuote(productID, materialID)}
	resolver := &fakeDiscountResolver{err: discountapi.ErrManualDiscountNoteRequired}
	svc := New(store, catalog, &fakeCustomers{})
	svc.SetDiscountResolver(resolver)

	in := posInput(productID, materialID, customerID, kasirID)
	in.ManualDiscountAmount = 20000

	_, err := svc.CreatePOSOrder(context.Background(), in)
	if !errors.Is(err, discountapi.ErrManualDiscountNoteRequired) {
		t.Fatalf("CreatePOSOrder() error = %v, want ErrManualDiscountNoteRequired", err)
	}
	if store.createCalls != 0 {
		t.Errorf("must not create order when discount resolution fails, calls=%d", store.createCalls)
	}
}

// TestCreatePOSOrder_DiscountResolverNotWired_Rejected — §22 no-silent-stub:
// a discount was requested but the resolver was never wired (composition
// root bug) — must fail loudly, not silently create the order without the
// discount.
func TestCreatePOSOrder_DiscountResolverNotWired_Rejected(t *testing.T) {
	productID, materialID := uuid.New(), uuid.New()
	customerID, kasirID := uuid.New(), uuid.New()
	store := &fakeStore{}
	catalog := &fakeCatalog{quoteResult: newQuote(productID, materialID)}
	svc := New(store, catalog, &fakeCustomers{}) // no SetDiscountResolver call

	in := posInput(productID, materialID, customerID, kasirID)
	discID := uuid.New()
	in.DiscountID = &discID

	_, err := svc.CreatePOSOrder(context.Background(), in)
	if !errors.Is(err, orderapi.ErrDiscountUnavailable) {
		t.Fatalf("CreatePOSOrder() error = %v, want ErrDiscountUnavailable", err)
	}
	if store.createCalls != 0 {
		t.Errorf("must not create order, calls=%d", store.createCalls)
	}
}

// TestCreatePOSOrder_NoDiscountRequested_ResolverNilIsFine — the reverse of
// the above: no resolver wired AND no discount requested must still work
// (most POS orders don't use a discount at all).
func TestCreatePOSOrder_NoDiscountRequested_ResolverNilIsFine(t *testing.T) {
	productID, materialID := uuid.New(), uuid.New()
	customerID, kasirID := uuid.New(), uuid.New()
	store := &fakeStore{}
	catalog := &fakeCatalog{quoteResult: newQuote(productID, materialID)}
	svc := New(store, catalog, &fakeCustomers{})

	sum, err := svc.CreatePOSOrder(context.Background(), posInput(productID, materialID, customerID, kasirID))
	if err != nil {
		t.Fatalf("CreatePOSOrder() error = %v, want nil", err)
	}
	if sum.Total != 50000 || sum.DiscountAmount != 0 {
		t.Fatalf("CreatePOSOrder() = total %d discount %d, want 50000/0", sum.Total, sum.DiscountAmount)
	}
}

// TestSetShippingCost_SubtractsExistingDiscountAmount — §28.3:
// newTotal = subtotal - discount_amount + shipping_cost (order already
// carries a discount from creation time; SetShippingCost must keep honoring
// it, not silently drop it when ongkir is filled in later).
func TestSetShippingCost_SubtractsExistingDiscountAmount(t *testing.T) {
	orderID := uuid.New()
	initial := &model.Order{
		ID: orderID, Resi: "RJK-D1", Status: state.OrderMasuk,
		MetodeAmbil: model.MetodeAmbilKirim, Subtotal: 100000, DiscountAmount: 20000, Total: 80000,
	}
	updated := &model.Order{
		ID: orderID, Resi: "RJK-D1", Status: state.MenungguPembayaran,
		MetodeAmbil: model.MetodeAmbilKirim, Subtotal: 100000, DiscountAmount: 20000, Total: 95000,
	}
	store := &fakeStore{findOrders: []*model.Order{initial, updated}}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	_, err := svc.SetShippingCost(context.Background(), SetShippingCostInput{
		Resi: "RJK-D1", ShippingCost: 15000, StaffID: uuid.New(),
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	// 100000 - 20000 + 15000 = 95000
	if store.setShippingParams.NewTotal != 95000 {
		t.Fatalf("NewTotal = %d, want 95000", store.setShippingParams.NewTotal)
	}
}
