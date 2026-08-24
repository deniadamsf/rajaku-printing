package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/discount/discountapi"
	"github.com/rajaku-printing/backend/internal/discount/model"
	"github.com/rajaku-printing/backend/internal/discount/repository"
)

func timePtr(t time.Time) *time.Time { return &t }

func TestCreate_HappyPath_Percent(t *testing.T) {
	store := &fakeDiscountStore{}
	svc := New(store)
	view, err := svc.Create(context.Background(), CreateInput{
		Code: "lebaran25", Name: "Promo Lebaran", Type: "percent",
		ValuePercent: floatPtr(25), MaxDiscountAmount: int64Ptr(50_000),
		ChannelScope: "all", IsActive: true,
	})
	if err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}
	if view.Code != "LEBARAN25" { // uppercased
		t.Fatalf("Create() code = %q, want LEBARAN25", view.Code)
	}
	if store.created.Type != model.DiscountTypePercent {
		t.Fatalf("stored discount type = %q, want percent", store.created.Type)
	}
}

// TestCreate_ManualNoteRequired is covered by resolve_test.go
// (TestResolveForOrder_ManualDiscount_NoteRequired) — manual discounts have
// no CRUD row (§28.1), so the "note required" rule lives in ResolveForOrder,
// not Create.

func TestCreate_MissingCode(t *testing.T) {
	svc := New(&fakeDiscountStore{})
	_, err := svc.Create(context.Background(), CreateInput{
		Name: "Promo", Type: "percent", ValuePercent: floatPtr(10),
	})
	if err != discountapi.ErrDiscountCodeRequired {
		t.Fatalf("Create() error = %v, want ErrDiscountCodeRequired", err)
	}
}

func TestCreate_InvalidPercentValue(t *testing.T) {
	svc := New(&fakeDiscountStore{})
	_, err := svc.Create(context.Background(), CreateInput{
		Code: "X", Name: "Promo", Type: "percent", ValuePercent: floatPtr(150),
	})
	if err != discountapi.ErrDiscountValuePercentInvalid {
		t.Fatalf("Create() error = %v, want ErrDiscountValuePercentInvalid", err)
	}
}

func TestCreate_UnknownType(t *testing.T) {
	svc := New(&fakeDiscountStore{})
	_, err := svc.Create(context.Background(), CreateInput{
		Code: "X", Name: "Promo", Type: "buy1get1",
	})
	if err != discountapi.ErrDiscountTypeInvalid {
		t.Fatalf("Create() error = %v, want ErrDiscountTypeInvalid", err)
	}
}

func TestCreate_CodeConflict(t *testing.T) {
	store := &fakeDiscountStore{createErr: repository.ErrCodeConflict}
	svc := New(store)
	_, err := svc.Create(context.Background(), CreateInput{
		Code: "X", Name: "Promo", Type: "nominal", ValueAmount: int64Ptr(1000),
	})
	if !errors.Is(err, discountapi.ErrDiscountCodeConflict) {
		t.Fatalf("Create() error = %v, want ErrDiscountCodeConflict", err)
	}
}

func TestDelete_ReasonRequired(t *testing.T) {
	svc := New(&fakeDiscountStore{})
	err := svc.Delete(context.Background(), uuid.New(), uuid.New(), "   ")
	if err != discountapi.ErrDiscountDeleteReasonRequired {
		t.Fatalf("Delete() error = %v, want ErrDiscountDeleteReasonRequired", err)
	}
}

func TestDelete_HappyPath(t *testing.T) {
	store := &fakeDiscountStore{}
	svc := New(store)
	actor := uuid.New()
	id := uuid.New()
	if err := svc.Delete(context.Background(), id, actor, "duplikat entri"); err != nil {
		t.Fatalf("Delete() error = %v, want nil", err)
	}
	if store.softDeleteParams.DiscountID != id || store.softDeleteParams.ActorID != actor {
		t.Fatalf("Delete() params = %+v, want id=%s actor=%s", store.softDeleteParams, id, actor)
	}
}

func TestUpdate_SwitchTypeRequiresMatchingValue(t *testing.T) {
	store := &fakeDiscountStore{findResult: &model.Discount{
		ID: uuid.New(), Code: "X", Name: "Promo",
		Type: model.DiscountTypePercent, ValuePercent: floatPtr(10),
		IsActive: true, ChannelScope: model.ChannelScopeAll,
	}}
	svc := New(store)
	newType := "nominal"
	_, err := svc.Update(context.Background(), store.findResult.ID, UpdateInput{Type: &newType})
	if err != discountapi.ErrDiscountValueAmountInvalid {
		t.Fatalf("Update() error = %v, want ErrDiscountValueAmountInvalid (missing value_amount)", err)
	}
}

func TestUpdate_ClearMaxDiscountAmount(t *testing.T) {
	id := uuid.New()
	store := &fakeDiscountStore{findResult: &model.Discount{
		ID: id, Code: "X", Name: "Promo",
		Type: model.DiscountTypePercent, ValuePercent: floatPtr(10),
		MaxDiscountAmount: int64Ptr(50_000),
		IsActive:          true, ChannelScope: model.ChannelScopeAll,
	}}
	svc := New(store)
	_, err := svc.Update(context.Background(), id, UpdateInput{ClearMaxDiscountAmount: true})
	if err != nil {
		t.Fatalf("Update() error = %v, want nil", err)
	}
	if v, ok := store.updateFields["max_discount_amount"]; !ok || v != nil {
		t.Fatalf("Update() fields = %+v, want max_discount_amount explicitly nil", store.updateFields)
	}
}

// ---- Review finding #2: validasi CRUD pakai sentinel, bukan fmt.Errorf polos ----

func TestCreate_QuotaInvalid_ReturnsSentinel(t *testing.T) {
	svc := New(&fakeDiscountStore{})
	_, err := svc.Create(context.Background(), CreateInput{
		Code: "X", Name: "Promo", Type: "percent", ValuePercent: floatPtr(10),
		Quota: intPtr(-1),
	})
	if !errors.Is(err, discountapi.ErrDiscountQuotaInvalid) {
		t.Fatalf("Create() error = %v, want ErrDiscountQuotaInvalid", err)
	}
}

func TestCreate_MaxDiscountAmountInvalid_ReturnsSentinel(t *testing.T) {
	svc := New(&fakeDiscountStore{})
	_, err := svc.Create(context.Background(), CreateInput{
		Code: "X", Name: "Promo", Type: "percent", ValuePercent: floatPtr(10),
		MaxDiscountAmount: int64Ptr(0),
	})
	if !errors.Is(err, discountapi.ErrDiscountMaxAmountInvalid) {
		t.Fatalf("Create() error = %v, want ErrDiscountMaxAmountInvalid", err)
	}
}

func TestCreate_MinSubtotalInvalid_ReturnsSentinel(t *testing.T) {
	svc := New(&fakeDiscountStore{})
	_, err := svc.Create(context.Background(), CreateInput{
		Code: "X", Name: "Promo", Type: "percent", ValuePercent: floatPtr(10),
		MinSubtotal: -1,
	})
	if !errors.Is(err, discountapi.ErrDiscountMinSubtotalInvalid) {
		t.Fatalf("Create() error = %v, want ErrDiscountMinSubtotalInvalid", err)
	}
}

func TestUpdate_QuotaInvalid_ReturnsSentinel(t *testing.T) {
	store := &fakeDiscountStore{findResult: &model.Discount{
		ID: uuid.New(), Code: "X", Name: "Promo",
		Type: model.DiscountTypePercent, ValuePercent: floatPtr(10),
		IsActive: true, ChannelScope: model.ChannelScopeAll,
	}}
	svc := New(store)
	_, err := svc.Update(context.Background(), store.findResult.ID, UpdateInput{Quota: intPtr(0)})
	if !errors.Is(err, discountapi.ErrDiscountQuotaInvalid) {
		t.Fatalf("Update() error = %v, want ErrDiscountQuotaInvalid", err)
	}
}

func TestUpdate_MaxDiscountAmountInvalid_ReturnsSentinel(t *testing.T) {
	store := &fakeDiscountStore{findResult: &model.Discount{
		ID: uuid.New(), Code: "X", Name: "Promo",
		Type: model.DiscountTypePercent, ValuePercent: floatPtr(10),
		IsActive: true, ChannelScope: model.ChannelScopeAll,
	}}
	svc := New(store)
	_, err := svc.Update(context.Background(), store.findResult.ID, UpdateInput{MaxDiscountAmount: int64Ptr(-100)})
	if !errors.Is(err, discountapi.ErrDiscountMaxAmountInvalid) {
		t.Fatalf("Update() error = %v, want ErrDiscountMaxAmountInvalid", err)
	}
}

func TestUpdate_MinSubtotalInvalid_ReturnsSentinel(t *testing.T) {
	store := &fakeDiscountStore{findResult: &model.Discount{
		ID: uuid.New(), Code: "X", Name: "Promo",
		Type: model.DiscountTypePercent, ValuePercent: floatPtr(10),
		IsActive: true, ChannelScope: model.ChannelScopeAll,
	}}
	svc := New(store)
	_, err := svc.Update(context.Background(), store.findResult.ID, UpdateInput{MinSubtotal: int64Ptr(-1)})
	if !errors.Is(err, discountapi.ErrDiscountMinSubtotalInvalid) {
		t.Fatalf("Update() error = %v, want ErrDiscountMinSubtotalInvalid", err)
	}
}

// ---- Review finding #3: ends_at harus setelah starts_at ----

func TestCreate_EndsAtBeforeStartsAt_ReturnsSentinel(t *testing.T) {
	svc := New(&fakeDiscountStore{})
	now := time.Now()
	_, err := svc.Create(context.Background(), CreateInput{
		Code: "X", Name: "Promo", Type: "percent", ValuePercent: floatPtr(10),
		StartsAt: timePtr(now), EndsAt: timePtr(now.Add(-time.Hour)),
	})
	if !errors.Is(err, discountapi.ErrDiscountInvalidPeriod) {
		t.Fatalf("Create() error = %v, want ErrDiscountInvalidPeriod", err)
	}
}

// TestUpdate_EndsAtBeforeExistingStartsAt_ReturnsSentinel is the regression
// guard for the "hanya mengubah salah satu kolom" case in finding #3: an
// update that patches ONLY ends_at (leaving the discount's existing
// starts_at untouched) must still be validated against the FINAL combined
// value, not just the field the caller happened to send.
func TestUpdate_EndsAtBeforeExistingStartsAt_ReturnsSentinel(t *testing.T) {
	now := time.Now()
	store := &fakeDiscountStore{findResult: &model.Discount{
		ID: uuid.New(), Code: "X", Name: "Promo",
		Type: model.DiscountTypePercent, ValuePercent: floatPtr(10),
		IsActive: true, ChannelScope: model.ChannelScopeAll,
		StartsAt: timePtr(now),
	}}
	svc := New(store)
	_, err := svc.Update(context.Background(), store.findResult.ID, UpdateInput{
		EndsAt: timePtr(now.Add(-time.Hour)),
	})
	if !errors.Is(err, discountapi.ErrDiscountInvalidPeriod) {
		t.Fatalf("Update() error = %v, want ErrDiscountInvalidPeriod", err)
	}
}

// ---- Review finding #9(b): max_discount_amount hanya berlaku type=percent ----

func TestCreate_MaxDiscountAmountOnNominal_ReturnsSentinel(t *testing.T) {
	svc := New(&fakeDiscountStore{})
	_, err := svc.Create(context.Background(), CreateInput{
		Code: "X", Name: "Promo", Type: "nominal", ValueAmount: int64Ptr(100_000),
		MaxDiscountAmount: int64Ptr(50_000),
	})
	if !errors.Is(err, discountapi.ErrDiscountMaxAmountNotAllowed) {
		t.Fatalf("Create() error = %v, want ErrDiscountMaxAmountNotAllowed", err)
	}
}

// TestUpdate_SwitchToNominalWithExistingMaxDiscountAmount_ReturnsSentinel —
// a percent discount already carrying max_discount_amount gets switched to
// nominal WITHOUT explicitly clearing max_discount_amount; must be rejected
// against the FINAL type, not silently left inconsistent.
func TestUpdate_SwitchToNominalWithExistingMaxDiscountAmount_ReturnsSentinel(t *testing.T) {
	store := &fakeDiscountStore{findResult: &model.Discount{
		ID: uuid.New(), Code: "X", Name: "Promo",
		Type: model.DiscountTypePercent, ValuePercent: floatPtr(10),
		MaxDiscountAmount: int64Ptr(50_000),
		IsActive:          true, ChannelScope: model.ChannelScopeAll,
	}}
	svc := New(store)
	newType := "nominal"
	_, err := svc.Update(context.Background(), store.findResult.ID, UpdateInput{
		Type: &newType, ValueAmount: int64Ptr(100_000),
	})
	if !errors.Is(err, discountapi.ErrDiscountMaxAmountNotAllowed) {
		t.Fatalf("Update() error = %v, want ErrDiscountMaxAmountNotAllowed", err)
	}
}
