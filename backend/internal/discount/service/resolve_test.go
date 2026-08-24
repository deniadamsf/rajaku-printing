package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/discount/discountapi"
	"github.com/rajaku-printing/backend/internal/discount/model"
	"github.com/rajaku-printing/backend/internal/discount/repository"
)

// fakeDiscountStore — minimal DiscountStore fake for service-level tests
// (mirrors the fakeStore pattern used in order/service tests).
type fakeDiscountStore struct {
	findResult *model.Discount
	findErr    error

	usageOne    int64
	usageOneErr error

	usageMap map[uuid.UUID]int64
	usageErr error

	createErr        error
	created          *model.Discount
	createProductIDs []uuid.UUID

	listResult []model.Discount
	listErr    error

	activeResult []model.Discount
	activeErr    error

	updateWithProductsErr error
	updateFields          map[string]any

	softDeleteErr    error
	softDeleteParams repository.SoftDeleteParams

	productIDsResult map[uuid.UUID][]uuid.UUID
	productIDsErr    error

	// replaceProductsID/replaceProductsIDs are only populated when
	// UpdateWithProducts is called with replaceProducts=true — mirrors the
	// old standalone ReplaceProducts fake so existing test assertions keep
	// working unchanged.
	replaceProductsID  uuid.UUID
	replaceProductsIDs []uuid.UUID
}

func (f *fakeDiscountStore) Create(_ context.Context, d *model.Discount, productIDs []uuid.UUID) error {
	f.created = d
	f.createProductIDs = productIDs
	return f.createErr
}

func (f *fakeDiscountStore) FindByID(_ context.Context, _ uuid.UUID) (*model.Discount, error) {
	if f.findErr != nil {
		return nil, f.findErr
	}
	return f.findResult, nil
}

func (f *fakeDiscountStore) List(_ context.Context, _ string) ([]model.Discount, error) {
	return f.listResult, f.listErr
}

func (f *fakeDiscountStore) ListActiveForChannel(_ context.Context, _ string, _ int64, _ time.Time) ([]model.Discount, error) {
	return f.activeResult, f.activeErr
}

func (f *fakeDiscountStore) CountUsage(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]int64, error) {
	if f.usageErr != nil {
		return nil, f.usageErr
	}
	if f.usageMap != nil {
		return f.usageMap, nil
	}
	out := make(map[uuid.UUID]int64, len(ids))
	return out, nil
}

func (f *fakeDiscountStore) CountUsageOne(_ context.Context, _ uuid.UUID) (int64, error) {
	return f.usageOne, f.usageOneErr
}

func (f *fakeDiscountStore) UpdateWithProducts(_ context.Context, id uuid.UUID, fields map[string]any, replaceProducts bool, productIDs []uuid.UUID) error {
	f.updateFields = fields
	if replaceProducts {
		f.replaceProductsID = id
		f.replaceProductsIDs = productIDs
	}
	return f.updateWithProductsErr
}

func (f *fakeDiscountStore) SoftDelete(_ context.Context, p repository.SoftDeleteParams) error {
	f.softDeleteParams = p
	return f.softDeleteErr
}

func (f *fakeDiscountStore) ProductIDs(_ context.Context, ids []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error) {
	if f.productIDsErr != nil {
		return nil, f.productIDsErr
	}
	if f.productIDsResult != nil {
		return f.productIDsResult, nil
	}
	out := make(map[uuid.UUID][]uuid.UUID, len(ids))
	return out, nil
}

// ---- ResolveForOrder ----

func TestResolveForOrder_NoDiscountRequested(t *testing.T) {
	svc := New(&fakeDiscountStore{})
	snap, err := svc.ResolveForOrder(context.Background(), discountapi.ResolveInput{Subtotal: 100_000, Channel: "pos"})
	if err != nil {
		t.Fatalf("ResolveForOrder() error = %v, want nil", err)
	}
	if snap.Amount != 0 || snap.Type != "" {
		t.Fatalf("ResolveForOrder() = %+v, want zero snapshot", snap)
	}
}

func TestResolveForOrder_MasterDiscount_HappyPath(t *testing.T) {
	id := uuid.New()
	store := &fakeDiscountStore{
		findResult: &model.Discount{
			ID: id, Code: "LEBARAN25", Name: "Promo Lebaran",
			Type: model.DiscountTypePercent, ValuePercent: floatPtr(25),
			IsActive: true, ChannelScope: model.ChannelScopeAll,
		},
		usageOne: 0,
	}
	svc := New(store)
	snap, err := svc.ResolveForOrder(context.Background(), discountapi.ResolveInput{
		DiscountID: &id, Subtotal: 200_000, Channel: "pos",
	})
	if err != nil {
		t.Fatalf("ResolveForOrder() error = %v, want nil", err)
	}
	if snap.Amount != 50_000 || snap.Type != "percent" || snap.Name != "Promo Lebaran" {
		t.Fatalf("ResolveForOrder() = %+v, want amount 50000 percent Promo Lebaran", snap)
	}
}

func TestResolveForOrder_MasterDiscount_NotFound(t *testing.T) {
	id := uuid.New()
	store := &fakeDiscountStore{findErr: repository.ErrNotFound}
	svc := New(store)
	_, err := svc.ResolveForOrder(context.Background(), discountapi.ResolveInput{
		DiscountID: &id, Subtotal: 200_000, Channel: "pos",
	})
	if err != discountapi.ErrDiscountNotFound {
		t.Fatalf("ResolveForOrder() error = %v, want ErrDiscountNotFound", err)
	}
}

func TestResolveForOrder_ManualDiscount_HappyPath(t *testing.T) {
	svc := New(&fakeDiscountStore{})
	snap, err := svc.ResolveForOrder(context.Background(), discountapi.ResolveInput{
		ManualAmount: 20_000, Note: "nego pelanggan lama", Subtotal: 100_000, Channel: "pos",
	})
	if err != nil {
		t.Fatalf("ResolveForOrder() error = %v, want nil", err)
	}
	if snap.Amount != 20_000 || snap.Type != "manual" || snap.Note != "nego pelanggan lama" {
		t.Fatalf("ResolveForOrder() = %+v, want manual/20000/note", snap)
	}
}

// TestResolveForOrder_ManualDiscount_NoteRequired — kasus gagal wajib
// (brief §6c): diskon manual tanpa note ditolak.
func TestResolveForOrder_ManualDiscount_NoteRequired(t *testing.T) {
	svc := New(&fakeDiscountStore{})
	_, err := svc.ResolveForOrder(context.Background(), discountapi.ResolveInput{
		ManualAmount: 20_000, Note: "   ", Subtotal: 100_000, Channel: "pos",
	})
	if err != discountapi.ErrManualDiscountNoteRequired {
		t.Fatalf("ResolveForOrder() error = %v, want ErrManualDiscountNoteRequired", err)
	}
}

// TestResolveForOrder_ManualDiscount_ZeroAmountIsNoDiscount — ManualAmount<=0
// with DiscountID nil is defined as "this order doesn't use a discount at
// all" (§28 ResolveInput doc), NOT a validation failure — even with a Note
// supplied, a zero/negative manual amount is a no-op, not an error.
func TestResolveForOrder_ManualDiscount_ZeroAmountIsNoDiscount(t *testing.T) {
	svc := New(&fakeDiscountStore{})
	snap, err := svc.ResolveForOrder(context.Background(), discountapi.ResolveInput{
		ManualAmount: 0, Note: "alasan", Subtotal: 100_000, Channel: "pos",
	})
	if err != nil {
		t.Fatalf("ResolveForOrder() error = %v, want nil", err)
	}
	if snap.Amount != 0 || snap.Type != "" {
		t.Fatalf("ResolveForOrder() = %+v, want zero snapshot", snap)
	}
}

// TestResolveForOrder_ManualDiscount_NegativeAmountRejected is the
// regression guard for review finding #6: a negative manual_discount_amount
// must NOT be silently treated as "no discount" — it has to reach
// resolveManualDiscount's own <= 0 check and come back as
// ErrManualDiscountInvalidAmount, which used to be dead code.
func TestResolveForOrder_ManualDiscount_NegativeAmountRejected(t *testing.T) {
	svc := New(&fakeDiscountStore{})
	_, err := svc.ResolveForOrder(context.Background(), discountapi.ResolveInput{
		ManualAmount: -5_000, Note: "alasan", Subtotal: 100_000, Channel: "pos",
	})
	if err != discountapi.ErrManualDiscountInvalidAmount {
		t.Fatalf("ResolveForOrder() error = %v, want ErrManualDiscountInvalidAmount", err)
	}
}

// ---- Cakupan diskon per produk (§28.9) ----

func TestResolveForOrder_MasterDiscount_ProductScope_Match(t *testing.T) {
	id := uuid.New()
	productID := uuid.New()
	store := &fakeDiscountStore{
		findResult: &model.Discount{
			ID: id, Code: "PRODUKA", Name: "Diskon Produk A",
			Type: model.DiscountTypePercent, ValuePercent: floatPtr(10),
			IsActive: true, ChannelScope: model.ChannelScopeAll,
			AppliesTo: model.AppliesToSelected,
		},
		productIDsResult: map[uuid.UUID][]uuid.UUID{id: {productID}},
	}
	svc := New(store)
	snap, err := svc.ResolveForOrder(context.Background(), discountapi.ResolveInput{
		DiscountID: &id, Subtotal: 100_000, Channel: "pos", ProductID: productID,
	})
	if err != nil {
		t.Fatalf("ResolveForOrder() error = %v, want nil", err)
	}
	if snap.Amount != 10_000 {
		t.Fatalf("ResolveForOrder() amount = %d, want 10000", snap.Amount)
	}
}

// TestResolveForOrder_MasterDiscount_ProductScope_Mismatch — kasus gagal
// wajib (brief §4): produk order di luar cakupan applies_to="selected".
func TestResolveForOrder_MasterDiscount_ProductScope_Mismatch(t *testing.T) {
	id := uuid.New()
	scopedProductID := uuid.New()
	otherProductID := uuid.New()
	store := &fakeDiscountStore{
		findResult: &model.Discount{
			ID: id, Code: "PRODUKA", Name: "Diskon Produk A",
			Type: model.DiscountTypePercent, ValuePercent: floatPtr(10),
			IsActive: true, ChannelScope: model.ChannelScopeAll,
			AppliesTo: model.AppliesToSelected,
		},
		productIDsResult: map[uuid.UUID][]uuid.UUID{id: {scopedProductID}},
	}
	svc := New(store)
	_, err := svc.ResolveForOrder(context.Background(), discountapi.ResolveInput{
		DiscountID: &id, Subtotal: 100_000, Channel: "pos", ProductID: otherProductID,
	})
	if err != discountapi.ErrDiscountProductMismatch {
		t.Fatalf("ResolveForOrder() error = %v, want ErrDiscountProductMismatch", err)
	}
}

// TestResolveForOrder_MasterDiscount_SelectedScopeEmpty_Rejected — §28.9
// aturan keras: applies_to="selected" dengan daftar produk kosong (mis.
// produk satu-satunya sudah dilepas dari cakupan setelah diskon dibuat)
// TIDAK BOLEH dianggap berlaku untuk semua produk — dicek lagi di sini, di
// TITIK PEMAKAIAN, bukan hanya saat create/update (lihat
// discount_service_test.go untuk sisi create/update-nya).
func TestResolveForOrder_MasterDiscount_SelectedScopeEmpty_Rejected(t *testing.T) {
	id := uuid.New()
	store := &fakeDiscountStore{
		findResult: &model.Discount{
			ID: id, Code: "PRODUKA", Name: "Diskon Produk A",
			Type: model.DiscountTypePercent, ValuePercent: floatPtr(10),
			IsActive: true, ChannelScope: model.ChannelScopeAll,
			AppliesTo: model.AppliesToSelected,
		},
		productIDsResult: map[uuid.UUID][]uuid.UUID{}, // kosong, BUKAN berarti semua
	}
	svc := New(store)
	_, err := svc.ResolveForOrder(context.Background(), discountapi.ResolveInput{
		DiscountID: &id, Subtotal: 100_000, Channel: "pos", ProductID: uuid.New(),
	})
	if err != discountapi.ErrDiscountScopeEmpty {
		t.Fatalf("ResolveForOrder() error = %v, want ErrDiscountScopeEmpty", err)
	}
}

// TestResolveForOrder_MasterDiscount_AppliesToAll_IgnoresProductID —
// applies_to="all" tidak pernah terpengaruh product_id apa pun (brief §4).
func TestResolveForOrder_MasterDiscount_AppliesToAll_IgnoresProductID(t *testing.T) {
	id := uuid.New()
	store := &fakeDiscountStore{
		findResult: &model.Discount{
			ID: id, Code: "GLOBAL10", Name: "Diskon Semua Produk",
			Type: model.DiscountTypePercent, ValuePercent: floatPtr(10),
			IsActive: true, ChannelScope: model.ChannelScopeAll,
			AppliesTo: model.AppliesToAll,
		},
	}
	svc := New(store)
	snap, err := svc.ResolveForOrder(context.Background(), discountapi.ResolveInput{
		DiscountID: &id, Subtotal: 100_000, Channel: "pos", ProductID: uuid.New(),
	})
	if err != nil {
		t.Fatalf("ResolveForOrder() error = %v, want nil", err)
	}
	if snap.Amount != 10_000 {
		t.Fatalf("ResolveForOrder() amount = %d, want 10000", snap.Amount)
	}
}

func TestResolveForOrder_AmbiguousInput(t *testing.T) {
	id := uuid.New()
	svc := New(&fakeDiscountStore{})
	_, err := svc.ResolveForOrder(context.Background(), discountapi.ResolveInput{
		DiscountID: &id, ManualAmount: 10_000, Subtotal: 100_000, Channel: "pos",
	})
	if err != discountapi.ErrDiscountAmbiguousInput {
		t.Fatalf("ResolveForOrder() error = %v, want ErrDiscountAmbiguousInput", err)
	}
}
