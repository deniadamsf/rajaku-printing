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

	createErr error
	created   *model.Discount

	listResult []model.Discount
	listErr    error

	activeResult []model.Discount
	activeErr    error

	updateErr    error
	updateFields map[string]any

	softDeleteErr    error
	softDeleteParams repository.SoftDeleteParams
}

func (f *fakeDiscountStore) Create(_ context.Context, d *model.Discount) error {
	f.created = d
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

func (f *fakeDiscountStore) Update(_ context.Context, _ uuid.UUID, fields map[string]any) error {
	f.updateFields = fields
	return f.updateErr
}

func (f *fakeDiscountStore) SoftDelete(_ context.Context, p repository.SoftDeleteParams) error {
	f.softDeleteParams = p
	return f.softDeleteErr
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
