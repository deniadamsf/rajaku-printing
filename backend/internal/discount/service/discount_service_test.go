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

// ---- Cakupan diskon per produk (§28.9) ----

func TestCreate_AppliesToSelected_HappyPath(t *testing.T) {
	store := &fakeDiscountStore{}
	svc := New(store)
	productID := uuid.New()
	view, err := svc.Create(context.Background(), CreateInput{
		Code: "PRODUKA", Name: "Diskon Produk A", Type: "percent", ValuePercent: floatPtr(10),
		ChannelScope: "all", IsActive: true,
		AppliesTo: "selected", ProductIDs: []uuid.UUID{productID},
	})
	if err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}
	if view.AppliesTo != "selected" {
		t.Fatalf("Create() applies_to = %q, want selected", view.AppliesTo)
	}
	if len(view.ProductIDs) != 1 || view.ProductIDs[0] != productID {
		t.Fatalf("Create() product_ids = %+v, want [%s]", view.ProductIDs, productID)
	}
	if len(store.createProductIDs) != 1 || store.createProductIDs[0] != productID {
		t.Fatalf("Create() productIDs passed to store = %+v, want [%s]", store.createProductIDs, productID)
	}
}

// TestCreate_AppliesToSelected_EmptyProductList_Rejected — §28.9 aturan
// keras: daftar produk kosong TIDAK boleh diperlakukan sebagai "berlaku
// untuk semua" — ditolak sejak create.
func TestCreate_AppliesToSelected_EmptyProductList_Rejected(t *testing.T) {
	svc := New(&fakeDiscountStore{})
	_, err := svc.Create(context.Background(), CreateInput{
		Code: "PRODUKA", Name: "Diskon Produk A", Type: "percent", ValuePercent: floatPtr(10),
		AppliesTo: "selected",
	})
	if !errors.Is(err, discountapi.ErrDiscountScopeEmpty) {
		t.Fatalf("Create() error = %v, want ErrDiscountScopeEmpty", err)
	}
}

func TestCreate_AppliesToAll_DefaultWhenOmitted(t *testing.T) {
	store := &fakeDiscountStore{}
	svc := New(store)
	view, err := svc.Create(context.Background(), CreateInput{
		Code: "GLOBAL", Name: "Diskon Semua", Type: "percent", ValuePercent: floatPtr(10),
	})
	if err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}
	if view.AppliesTo != "all" {
		t.Fatalf("Create() applies_to = %q, want all (default kalau tidak dikirim)", view.AppliesTo)
	}
	if len(view.ProductIDs) != 0 {
		t.Fatalf("Create() product_ids = %+v, want empty", view.ProductIDs)
	}
}

func TestCreate_InvalidAppliesTo_ReturnsSentinel(t *testing.T) {
	svc := New(&fakeDiscountStore{})
	_, err := svc.Create(context.Background(), CreateInput{
		Code: "X", Name: "Promo", Type: "percent", ValuePercent: floatPtr(10),
		AppliesTo: "sebagian",
	})
	if !errors.Is(err, discountapi.ErrDiscountAppliesToInvalid) {
		t.Fatalf("Create() error = %v, want ErrDiscountAppliesToInvalid", err)
	}
}

// TestUpdate_ReplacesProductScopeEntirely — §28.9: PATCH product_ids
// mengganti SELURUH daftar (delete+insert), bukan menambah/merge parsial.
func TestUpdate_ReplacesProductScopeEntirely(t *testing.T) {
	id := uuid.New()
	oldProductID := uuid.New()
	newProductID := uuid.New()
	store := &fakeDiscountStore{
		findResult: &model.Discount{
			ID: id, Code: "X", Name: "Promo",
			Type: model.DiscountTypePercent, ValuePercent: floatPtr(10),
			IsActive: true, ChannelScope: model.ChannelScopeAll,
			AppliesTo: model.AppliesToSelected,
		},
		productIDsResult: map[uuid.UUID][]uuid.UUID{id: {oldProductID}},
	}
	svc := New(store)
	newIDs := []uuid.UUID{newProductID}
	view, err := svc.Update(context.Background(), id, UpdateInput{ProductIDs: &newIDs})
	if err != nil {
		t.Fatalf("Update() error = %v, want nil", err)
	}
	if store.replaceProductsID != id {
		t.Fatalf("ReplaceProducts discountID = %s, want %s", store.replaceProductsID, id)
	}
	if len(store.replaceProductsIDs) != 1 || store.replaceProductsIDs[0] != newProductID {
		t.Fatalf("ReplaceProducts productIDs = %+v, want [%s] (entire replace, old id dropped)", store.replaceProductsIDs, newProductID)
	}
	if len(view.ProductIDs) != 1 || view.ProductIDs[0] != newProductID {
		t.Fatalf("Update() view.product_ids = %+v, want [%s]", view.ProductIDs, newProductID)
	}
}

// TestUpdate_AppliesToSelected_EmptyProductList_Rejected — checked again at
// update time using the discount's CURRENT scope when product_ids isn't
// explicitly sent in the PATCH body — switching applies_to to "selected"
// without ever having assigned any product must be rejected, not silently
// treated as "all".
func TestUpdate_AppliesToSelected_EmptyProductList_Rejected(t *testing.T) {
	id := uuid.New()
	store := &fakeDiscountStore{
		findResult: &model.Discount{
			ID: id, Code: "X", Name: "Promo",
			Type: model.DiscountTypePercent, ValuePercent: floatPtr(10),
			IsActive: true, ChannelScope: model.ChannelScopeAll,
			AppliesTo: model.AppliesToAll,
		},
		productIDsResult: map[uuid.UUID][]uuid.UUID{}, // belum ada produk manapun
	}
	svc := New(store)
	scope := "selected"
	_, err := svc.Update(context.Background(), id, UpdateInput{AppliesTo: &scope})
	if !errors.Is(err, discountapi.ErrDiscountScopeEmpty) {
		t.Fatalf("Update() error = %v, want ErrDiscountScopeEmpty", err)
	}
}

// TestUpdate_ProductIDsOmitted_KeepsExistingScope — kalau PATCH tidak
// menyertakan product_ids sama sekali, cakupan produk yang sudah ada TIDAK
// disentuh (bukan direset kosong).
func TestUpdate_ProductIDsOmitted_KeepsExistingScope(t *testing.T) {
	id := uuid.New()
	existingProductID := uuid.New()
	store := &fakeDiscountStore{
		findResult: &model.Discount{
			ID: id, Code: "X", Name: "Promo",
			Type: model.DiscountTypePercent, ValuePercent: floatPtr(10),
			IsActive: true, ChannelScope: model.ChannelScopeAll,
			AppliesTo: model.AppliesToSelected,
		},
		productIDsResult: map[uuid.UUID][]uuid.UUID{id: {existingProductID}},
	}
	svc := New(store)
	newName := "Promo Diperbarui"
	view, err := svc.Update(context.Background(), id, UpdateInput{Name: &newName})
	if err != nil {
		t.Fatalf("Update() error = %v, want nil", err)
	}
	if store.replaceProductsID != uuid.Nil {
		t.Fatalf("ReplaceProducts must NOT be called when product_ids omitted, got discountID=%s", store.replaceProductsID)
	}
	if len(view.ProductIDs) != 1 || view.ProductIDs[0] != existingProductID {
		t.Fatalf("Update() product_ids = %+v, want existing [%s] untouched", view.ProductIDs, existingProductID)
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

// ---- Review finding #1: Update field + product scope harus atomik ----

// TestUpdate_ProductScopeReplaceFails_ReturnsError memastikan kegagalan
// repository (dilempar sebagai satu kesatuan lewat UpdateWithScopes) tetap
// dipropagasi sebagai error ke caller — atomicity SEBENARNYA (rollback DB)
// dibuktikan di repository_test.go (butuh sqlmock, bukan fake); test ini
// hanya menjaga service TIDAK menelan errornya atau melaporkan sukses palsu.
func TestUpdate_ProductScopeReplaceFails_ReturnsError(t *testing.T) {
	id := uuid.New()
	store := &fakeDiscountStore{
		findResult: &model.Discount{
			ID: id, Code: "X", Name: "Promo",
			Type: model.DiscountTypePercent, ValuePercent: floatPtr(10),
			IsActive: true, ChannelScope: model.ChannelScopeAll,
			AppliesTo: model.AppliesToSelected,
		},
		productIDsResult:    map[uuid.UUID][]uuid.UUID{id: {uuid.New()}},
		updateWithScopesErr: repository.ErrProductNotFound,
	}
	svc := New(store)
	newIDs := []uuid.UUID{uuid.New()}
	_, err := svc.Update(context.Background(), id, UpdateInput{ProductIDs: &newIDs})
	if !errors.Is(err, discountapi.ErrDiscountProductNotFound) {
		t.Fatalf("Update() error = %v, want ErrDiscountProductNotFound", err)
	}
}

// ---- Review finding #2: applies_to/channel_scope "" di PATCH ditolak ----

// TestUpdate_AppliesToEmptyString_Rejected — PATCH {"applies_to": ""} WAJIB
// ditolak 400, BUKAN diperlakukan sebagai "tidak dikirim" (yang akan diam-
// diam melebarkan cakupan diskon jadi "all" — kerugian uang).
func TestUpdate_AppliesToEmptyString_Rejected(t *testing.T) {
	id := uuid.New()
	store := &fakeDiscountStore{findResult: &model.Discount{
		ID: id, Code: "X", Name: "Promo",
		Type: model.DiscountTypePercent, ValuePercent: floatPtr(10),
		IsActive: true, ChannelScope: model.ChannelScopeAll,
		AppliesTo: model.AppliesToSelected,
	}}
	svc := New(store)
	empty := ""
	_, err := svc.Update(context.Background(), id, UpdateInput{AppliesTo: &empty})
	if !errors.Is(err, discountapi.ErrDiscountAppliesToInvalid) {
		t.Fatalf("Update() error = %v, want ErrDiscountAppliesToInvalid", err)
	}
}

// TestUpdate_ChannelScopeEmptyString_Rejected — bug kembar dari
// applies_to: "" di atas. PATCH {"channel_scope": ""} WAJIB ditolak 400,
// bukan diam-diam melebarkan diskon pos-only jadi berlaku di semua channel.
func TestUpdate_ChannelScopeEmptyString_Rejected(t *testing.T) {
	id := uuid.New()
	store := &fakeDiscountStore{findResult: &model.Discount{
		ID: id, Code: "X", Name: "Promo",
		Type: model.DiscountTypePercent, ValuePercent: floatPtr(10),
		IsActive: true, ChannelScope: model.ChannelScopePOS,
	}}
	svc := New(store)
	empty := ""
	_, err := svc.Update(context.Background(), id, UpdateInput{ChannelScope: &empty})
	if !errors.Is(err, discountapi.ErrDiscountChannelScopeInvalid) {
		t.Fatalf("Update() error = %v, want ErrDiscountChannelScopeInvalid", err)
	}
}

// TestCreate_AppliesToEmptyString_StillDefaultsToAll — regression guard:
// perbaikan finding #2 HANYA berlaku di jalur PATCH; create tetap boleh
// menerima "" sebagai "tidak diisi" -> default "all" (perilaku lama, tidak
// boleh berubah).
func TestCreate_AppliesToEmptyString_StillDefaultsToAll(t *testing.T) {
	store := &fakeDiscountStore{}
	svc := New(store)
	view, err := svc.Create(context.Background(), CreateInput{
		Code: "X", Name: "Promo", Type: "percent", ValuePercent: floatPtr(10),
		AppliesTo: "",
	})
	if err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}
	if view.AppliesTo != "all" {
		t.Fatalf("Create() applies_to = %q, want all", view.AppliesTo)
	}
}

// ---- Review finding #3: product_ids tak dikenal / uuid.Nil ----

// TestCreate_UnknownProductID_MapsToSentinel — repository menolak product
// id yang tidak ada di `products` dengan ErrProductNotFound; service harus
// memetakannya ke discountapi.ErrDiscountProductNotFound (400), bukan
// meneruskan error mentah (yang di handler akan jatuh ke 500).
func TestCreate_UnknownProductID_MapsToSentinel(t *testing.T) {
	store := &fakeDiscountStore{createErr: repository.ErrProductNotFound}
	svc := New(store)
	_, err := svc.Create(context.Background(), CreateInput{
		Code: "X", Name: "Promo", Type: "percent", ValuePercent: floatPtr(10),
		AppliesTo: "selected", ProductIDs: []uuid.UUID{uuid.New()},
	})
	if !errors.Is(err, discountapi.ErrDiscountProductNotFound) {
		t.Fatalf("Create() error = %v, want ErrDiscountProductNotFound", err)
	}
}

// TestCreate_ProductIDsOnlyNilUUID_TreatedAsEmptyScope — dedupeUUIDs
// membuang uuid.Nil (temuan review #3); product_ids berisi HANYA uuid.Nil
// jadi setara "tidak ada produk", ditolak sebagai ErrDiscountScopeEmpty
// (400) alih-alih lolos validasi lalu meledak jadi FK violation (500) di
// repository.
func TestCreate_ProductIDsOnlyNilUUID_TreatedAsEmptyScope(t *testing.T) {
	svc := New(&fakeDiscountStore{})
	_, err := svc.Create(context.Background(), CreateInput{
		Code: "X", Name: "Promo", Type: "percent", ValuePercent: floatPtr(10),
		AppliesTo: "selected", ProductIDs: []uuid.UUID{uuid.Nil},
	})
	if !errors.Is(err, discountapi.ErrDiscountScopeEmpty) {
		t.Fatalf("Create() error = %v, want ErrDiscountScopeEmpty", err)
	}
}

// ---- Tes yang reviewer sebut belum ada ----

// TestUpdate_ProductIDsEmptyList_OnSelectedDiscount_Rejected — PATCH
// {"product_ids": []} pada diskon yang SUDAH applies_to="selected" harus
// ditolak (mengosongkan cakupan diskon yang masih dipakai), bukan diam-diam
// diterima.
func TestUpdate_ProductIDsEmptyList_OnSelectedDiscount_Rejected(t *testing.T) {
	id := uuid.New()
	store := &fakeDiscountStore{
		findResult: &model.Discount{
			ID: id, Code: "X", Name: "Promo",
			Type: model.DiscountTypePercent, ValuePercent: floatPtr(10),
			IsActive: true, ChannelScope: model.ChannelScopeAll,
			AppliesTo: model.AppliesToSelected,
		},
		productIDsResult: map[uuid.UUID][]uuid.UUID{id: {uuid.New()}},
	}
	svc := New(store)
	empty := []uuid.UUID{}
	_, err := svc.Update(context.Background(), id, UpdateInput{ProductIDs: &empty})
	if !errors.Is(err, discountapi.ErrDiscountScopeEmpty) {
		t.Fatalf("Update() error = %v, want ErrDiscountScopeEmpty", err)
	}
}

// ---- Review finding #4/#5: Applicable menyaring lewat validateForUse ----

// TestApplicable_EmptyScope_FilteredOut_EvenWithoutProductID — diskon
// applies_to="selected" dengan cakupan KOSONG tidak boleh muncul di
// Applicable, TERLEPAS dari product_id dikirim atau tidak (temuan review
// #4 — sebelumnya hanya disaring kalau product_id dikirim).
func TestApplicable_EmptyScope_FilteredOut_EvenWithoutProductID(t *testing.T) {
	id := uuid.New()
	store := &fakeDiscountStore{
		activeResult: []model.Discount{{
			ID: id, Code: "X", Name: "Promo",
			Type: model.DiscountTypePercent, ValuePercent: floatPtr(10),
			IsActive: true, ChannelScope: model.ChannelScopeAll,
			AppliesTo: model.AppliesToSelected,
		}},
		productIDsResult: map[uuid.UUID][]uuid.UUID{}, // cakupan kosong
	}
	svc := New(store)
	views, err := svc.Applicable(context.Background(), "pos", singleItem(100_000, uuid.New()), uuid.Nil)
	if err != nil {
		t.Fatalf("Applicable() error = %v, want nil", err)
	}
	if len(views) != 0 {
		t.Fatalf("Applicable() = %+v, want empty (scope kosong harus disaring tanpa syarat)", views)
	}
}

// TestApplicable_SelectedNonEmptyScope_ShownWhenItemMatches — diskon
// applies_to="selected" dengan cakupan TIDAK kosong tampil kalau keranjang
// kasir mengandung item yang product_id-nya ADA di cakupan itu. (Sejak
// kontrak Applicable berubah ke daftar item — §32.3/temuan review #4 — tidak
// ada lagi konsep "product_id tidak dikirim sama sekali"; setiap baris
// keranjang selalu punya product_id.)
func TestApplicable_SelectedNonEmptyScope_ShownWhenItemMatches(t *testing.T) {
	id := uuid.New()
	scopedProductID := uuid.New()
	store := &fakeDiscountStore{
		activeResult: []model.Discount{{
			ID: id, Code: "X", Name: "Promo",
			Type: model.DiscountTypePercent, ValuePercent: floatPtr(10),
			IsActive: true, ChannelScope: model.ChannelScopeAll,
			AppliesTo: model.AppliesToSelected,
		}},
		productIDsResult: map[uuid.UUID][]uuid.UUID{id: {scopedProductID}},
	}
	svc := New(store)
	views, err := svc.Applicable(context.Background(), "pos", singleItem(100_000, scopedProductID), uuid.Nil)
	if err != nil {
		t.Fatalf("Applicable() error = %v, want nil", err)
	}
	if len(views) != 1 {
		t.Fatalf("Applicable() = %+v, want 1 item (selected discount w/ non-empty scope, item matches)", views)
	}
}

// TestApplicable_ProductIDGiven_FiltersOutOfScopeDiscount — diskon
// applies_to="selected" yang cakupannya TIDAK menyertakan product_id yang
// diminta harus disaring (kasir tidak boleh melihat promo yang akan
// ditolak saat disimpan).
func TestApplicable_ProductIDGiven_FiltersOutOfScopeDiscount(t *testing.T) {
	id := uuid.New()
	scopedProductID := uuid.New()
	requestedProductID := uuid.New()
	store := &fakeDiscountStore{
		activeResult: []model.Discount{{
			ID: id, Code: "X", Name: "Promo",
			Type: model.DiscountTypePercent, ValuePercent: floatPtr(10),
			IsActive: true, ChannelScope: model.ChannelScopeAll,
			AppliesTo: model.AppliesToSelected,
		}},
		productIDsResult: map[uuid.UUID][]uuid.UUID{id: {scopedProductID}},
	}
	svc := New(store)
	views, err := svc.Applicable(context.Background(), "pos", singleItem(100_000, requestedProductID), uuid.Nil)
	if err != nil {
		t.Fatalf("Applicable() error = %v, want nil", err)
	}
	if len(views) != 0 {
		t.Fatalf("Applicable() = %+v, want empty (produk yang diminta tidak ada di cakupan)", views)
	}
}

// TestApplicable_MixedCart_SelectedScope_MinSubtotalBasedOnEligibleOnly is
// the §32.3 regression guard for the exact bug report: promo "khusus flexi,
// min Rp200rb" pada keranjang [flexi Rp10rb, vinyl Rp500rb] TIDAK BOLEH
// muncul di kasir — subtotal SELURUH keranjang (510rb) lolos syarat minimum,
// tapi eligible_subtotal (cuma flexi, 10rb) TIDAK.
func TestApplicable_MixedCart_SelectedScope_MinSubtotalBasedOnEligibleOnly(t *testing.T) {
	id := uuid.New()
	flexiID := uuid.New()
	vinylID := uuid.New()
	store := &fakeDiscountStore{
		activeResult: []model.Discount{{
			ID: id, Code: "FLEXI20", Name: "Diskon Flexi",
			Type: model.DiscountTypePercent, ValuePercent: floatPtr(20),
			IsActive: true, ChannelScope: model.ChannelScopeAll,
			AppliesTo:   model.AppliesToSelected,
			MinSubtotal: 200_000,
		}},
		productIDsResult: map[uuid.UUID][]uuid.UUID{id: {flexiID}},
	}
	svc := New(store)
	items := []discountapi.ResolveItem{
		{LineNo: 1, ProductID: flexiID, Subtotal: 10_000},
		{LineNo: 2, ProductID: vinylID, Subtotal: 500_000},
	}
	views, err := svc.Applicable(context.Background(), "pos", items, uuid.Nil)
	if err != nil {
		t.Fatalf("Applicable() error = %v, want nil", err)
	}
	if len(views) != 0 {
		t.Fatalf("Applicable() = %+v, want empty (eligible_subtotal 10000 < min 200000, even though cart total is 510000)", views)
	}
}

// TestApplicable_QuotaExhausted_FilteredOut — regresi cepat: Applicable
// masih menyaring kuota habis setelah refactor ke validateForUse (temuan
// review #5) — bukan cuma cakupan produk yang harus tetap benar.
func TestApplicable_QuotaExhausted_FilteredOut(t *testing.T) {
	id := uuid.New()
	store := &fakeDiscountStore{
		activeResult: []model.Discount{{
			ID: id, Code: "X", Name: "Promo",
			Type: model.DiscountTypePercent, ValuePercent: floatPtr(10),
			IsActive: true, ChannelScope: model.ChannelScopeAll,
			AppliesTo: model.AppliesToAll,
			Quota:     intPtr(5),
		}},
		usageMap: map[uuid.UUID]int64{id: 5},
	}
	svc := New(store)
	views, err := svc.Applicable(context.Background(), "pos", singleItem(100_000, uuid.New()), uuid.Nil)
	if err != nil {
		t.Fatalf("Applicable() error = %v, want nil", err)
	}
	if len(views) != 0 {
		t.Fatalf("Applicable() = %+v, want empty (kuota sudah habis)", views)
	}
}

// ---- Diskon khusus member (§30.3) ----

func TestCreate_Member_HappyPath_AllMembers(t *testing.T) {
	store := &fakeDiscountStore{}
	svc := New(store)
	view, err := svc.Create(context.Background(), CreateInput{
		Code: "MEMBER10", Name: "Diskon Member", Type: "percent", ValuePercent: floatPtr(10),
		ChannelScope: "all", IsActive: true,
		AudienceScope: "member", MemberScope: "all_members",
	})
	if err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}
	if view.AudienceScope != "member" || view.MemberScope != "all_members" {
		t.Fatalf("Create() audience_scope/member_scope = %q/%q, want member/all_members",
			view.AudienceScope, view.MemberScope)
	}
}

func TestCreate_Member_SelectedMembers_HappyPath(t *testing.T) {
	store := &fakeDiscountStore{}
	svc := New(store)
	customerID := uuid.New()
	view, err := svc.Create(context.Background(), CreateInput{
		Code: "VIP10", Name: "Diskon VIP", Type: "percent", ValuePercent: floatPtr(10),
		ChannelScope: "all", IsActive: true,
		AudienceScope: "member", MemberScope: "selected_members",
		CustomerIDs: []uuid.UUID{customerID},
	})
	if err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}
	if len(view.CustomerIDs) != 1 || view.CustomerIDs[0] != customerID {
		t.Fatalf("Create() customer_ids = %+v, want [%s]", view.CustomerIDs, customerID)
	}
	if len(store.createCustomerIDs) != 1 || store.createCustomerIDs[0] != customerID {
		t.Fatalf("Create() customerIDs passed to store = %+v, want [%s]", store.createCustomerIDs, customerID)
	}
}

// TestCreate_Member_SelectedMembers_EmptyList_Rejected — §30.3 aturan
// keras: member_scope="selected_members" dengan daftar customer kosong
// TIDAK boleh diperlakukan sebagai "berlaku untuk semua member".
func TestCreate_Member_SelectedMembers_EmptyList_Rejected(t *testing.T) {
	svc := New(&fakeDiscountStore{})
	_, err := svc.Create(context.Background(), CreateInput{
		Code: "VIP10", Name: "Diskon VIP", Type: "percent", ValuePercent: floatPtr(10),
		AudienceScope: "member", MemberScope: "selected_members",
	})
	if !errors.Is(err, discountapi.ErrDiscountMemberScopeEmpty) {
		t.Fatalf("Create() error = %v, want ErrDiscountMemberScopeEmpty", err)
	}
}

// TestCreate_Member_MissingMemberScope_Rejected — audience_scope="member"
// wajib disertai member_scope, tidak boleh dibiarkan kosong.
func TestCreate_Member_MissingMemberScope_Rejected(t *testing.T) {
	svc := New(&fakeDiscountStore{})
	_, err := svc.Create(context.Background(), CreateInput{
		Code: "MEMBER10", Name: "Diskon Member", Type: "percent", ValuePercent: floatPtr(10),
		AudienceScope: "member",
	})
	if !errors.Is(err, discountapi.ErrDiscountMemberScopeInvalid) {
		t.Fatalf("Create() error = %v, want ErrDiscountMemberScopeInvalid", err)
	}
}

// TestCreate_MemberScopeWithoutAudienceMember_Rejected — member_scope diisi
// padahal audience_scope bukan "member" (default "all") — kombinasi yang
// tidak boleh diam-diam diterima (CHECK constraint di DB, migration 000031,
// jadi lapis terakhir kalau ini lolos).
func TestCreate_MemberScopeWithoutAudienceMember_Rejected(t *testing.T) {
	svc := New(&fakeDiscountStore{})
	_, err := svc.Create(context.Background(), CreateInput{
		Code: "X", Name: "Promo", Type: "percent", ValuePercent: floatPtr(10),
		MemberScope: "all_members",
	})
	if !errors.Is(err, discountapi.ErrDiscountMemberScopeInvalid) {
		t.Fatalf("Create() error = %v, want ErrDiscountMemberScopeInvalid", err)
	}
}

func TestCreate_AudienceScopeAll_DefaultWhenOmitted(t *testing.T) {
	store := &fakeDiscountStore{}
	svc := New(store)
	view, err := svc.Create(context.Background(), CreateInput{
		Code: "GLOBAL", Name: "Diskon Semua", Type: "percent", ValuePercent: floatPtr(10),
	})
	if err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}
	if view.AudienceScope != "all" {
		t.Fatalf("Create() audience_scope = %q, want all (default kalau tidak dikirim)", view.AudienceScope)
	}
	if view.MemberScope != "" {
		t.Fatalf("Create() member_scope = %q, want empty", view.MemberScope)
	}
}

// TestUpdate_SwitchToMember_AutoClearsOnLeaving — reconcileMemberScope harus
// AUTO-CLEAR member_scope kalau audience_scope dipatch balik ke "all" pada
// PATCH yang sama, tanpa memaksa caller juga mengirim member_scope:null.
func TestUpdate_SwitchToMember_AutoClearsOnLeaving(t *testing.T) {
	id := uuid.New()
	selected := model.MemberScopeAllMembers
	store := &fakeDiscountStore{
		findResult: &model.Discount{
			ID: id, Code: "X", Name: "Promo",
			Type: model.DiscountTypePercent, ValuePercent: floatPtr(10),
			IsActive: true, ChannelScope: model.ChannelScopeAll,
			AudienceScope: model.AudienceScopeMember,
			MemberScope:   &selected,
		},
	}
	svc := New(store)
	newScope := "all"
	view, err := svc.Update(context.Background(), id, UpdateInput{AudienceScope: &newScope})
	if err != nil {
		t.Fatalf("Update() error = %v, want nil", err)
	}
	if view.AudienceScope != "all" || view.MemberScope != "" {
		t.Fatalf("Update() audience_scope/member_scope = %q/%q, want all/empty (auto-cleared)",
			view.AudienceScope, view.MemberScope)
	}
	if v, ok := store.updateFields["member_scope"]; !ok || v != nil {
		t.Fatalf("Update() fields[member_scope] = %v (ok=%v), want explicit nil (auto-clear must reach DB)", v, ok)
	}
}

// TestUpdate_SwitchToMember_WithoutMemberScope_Rejected — audience_scope
// dipatch ke "member" tanpa member_scope disertakan -> ditolak, tidak boleh
// diam-diam dibiarkan NULL (akan melanggar CHECK constraint di DB kalau
// lolos sampai situ).
func TestUpdate_SwitchToMember_WithoutMemberScope_Rejected(t *testing.T) {
	id := uuid.New()
	store := &fakeDiscountStore{
		findResult: &model.Discount{
			ID: id, Code: "X", Name: "Promo",
			Type: model.DiscountTypePercent, ValuePercent: floatPtr(10),
			IsActive: true, ChannelScope: model.ChannelScopeAll,
			AudienceScope: model.AudienceScopeAll,
		},
	}
	svc := New(store)
	newScope := "member"
	_, err := svc.Update(context.Background(), id, UpdateInput{AudienceScope: &newScope})
	if !errors.Is(err, discountapi.ErrDiscountMemberScopeInvalid) {
		t.Fatalf("Update() error = %v, want ErrDiscountMemberScopeInvalid", err)
	}
}

// TestUpdate_MemberScopeExplicit_WithoutAudienceMember_Rejected — caller
// explicitly sends member_scope while the final audience_scope isn't
// "member" (never was, and this PATCH doesn't change it) — explicit
// disagreement, must be rejected outright, not silently ignored.
func TestUpdate_MemberScopeExplicit_WithoutAudienceMember_Rejected(t *testing.T) {
	id := uuid.New()
	store := &fakeDiscountStore{
		findResult: &model.Discount{
			ID: id, Code: "X", Name: "Promo",
			Type: model.DiscountTypePercent, ValuePercent: floatPtr(10),
			IsActive: true, ChannelScope: model.ChannelScopeAll,
			AudienceScope: model.AudienceScopeAll,
		},
	}
	svc := New(store)
	newMemberScope := "all_members"
	_, err := svc.Update(context.Background(), id, UpdateInput{MemberScope: &newMemberScope})
	if !errors.Is(err, discountapi.ErrDiscountMemberScopeInvalid) {
		t.Fatalf("Update() error = %v, want ErrDiscountMemberScopeInvalid", err)
	}
}

// TestUpdate_ReplacesCustomerScopeEntirely — §30.3 mirror
// TestUpdate_ReplacesProductScopeEntirely: PATCH customer_ids mengganti
// SELURUH daftar (delete+insert), bukan menambah/merge parsial.
func TestUpdate_ReplacesCustomerScopeEntirely(t *testing.T) {
	id := uuid.New()
	oldCustomerID := uuid.New()
	newCustomerID := uuid.New()
	selected := model.MemberScopeSelected
	store := &fakeDiscountStore{
		findResult: &model.Discount{
			ID: id, Code: "X", Name: "Promo",
			Type: model.DiscountTypePercent, ValuePercent: floatPtr(10),
			IsActive: true, ChannelScope: model.ChannelScopeAll,
			AudienceScope: model.AudienceScopeMember,
			MemberScope:   &selected,
		},
		customerIDsResult: map[uuid.UUID][]uuid.UUID{id: {oldCustomerID}},
	}
	svc := New(store)
	newIDs := []uuid.UUID{newCustomerID}
	view, err := svc.Update(context.Background(), id, UpdateInput{CustomerIDs: &newIDs})
	if err != nil {
		t.Fatalf("Update() error = %v, want nil", err)
	}
	if store.replaceCustomersID != id {
		t.Fatalf("ReplaceCustomers discountID = %s, want %s", store.replaceCustomersID, id)
	}
	if len(store.replaceCustomersIDs) != 1 || store.replaceCustomersIDs[0] != newCustomerID {
		t.Fatalf("ReplaceCustomers customerIDs = %+v, want [%s] (entire replace, old id dropped)",
			store.replaceCustomersIDs, newCustomerID)
	}
	if len(view.CustomerIDs) != 1 || view.CustomerIDs[0] != newCustomerID {
		t.Fatalf("Update() view.customer_ids = %+v, want [%s]", view.CustomerIDs, newCustomerID)
	}
}

// TestApplicable_Member_NoCustomerID_FilteredOutByDefault — §30.3: tanpa
// customer_id, diskon audience_scope="member" SAMA SEKALI TIDAK MUNCUL
// (aman by default) — beda dari product_id di mana applies_to="all" tetap
// lolos tanpa product_id.
func TestApplicable_Member_NoCustomerID_FilteredOutByDefault(t *testing.T) {
	id := uuid.New()
	store := &fakeDiscountStore{
		activeResult: []model.Discount{{
			ID: id, Code: "MEMBER10", Name: "Diskon Member",
			Type: model.DiscountTypePercent, ValuePercent: floatPtr(10),
			IsActive: true, ChannelScope: model.ChannelScopeAll,
			AudienceScope: model.AudienceScopeMember,
			MemberScope:   memberScopePtr(model.MemberScopeAllMembers),
		}},
	}
	svc := New(store)
	views, err := svc.Applicable(context.Background(), "pos", singleItem(100_000, uuid.New()), uuid.Nil)
	if err != nil {
		t.Fatalf("Applicable() error = %v, want nil", err)
	}
	if len(views) != 0 {
		t.Fatalf("Applicable() = %+v, want empty (member discount hidden without customer_id)", views)
	}
}

// TestApplicable_Member_CustomerIDGiven_ActiveMember_Shown — customer_id
// dikirim & customer-nya member aktif -> diskon member ikut muncul.
func TestApplicable_Member_CustomerIDGiven_ActiveMember_Shown(t *testing.T) {
	id := uuid.New()
	customerID := uuid.New()
	store := &fakeDiscountStore{
		activeResult: []model.Discount{{
			ID: id, Code: "MEMBER10", Name: "Diskon Member",
			Type: model.DiscountTypePercent, ValuePercent: floatPtr(10),
			IsActive: true, ChannelScope: model.ChannelScopeAll,
			AudienceScope: model.AudienceScopeMember,
			MemberScope:   memberScopePtr(model.MemberScopeAllMembers),
		}},
	}
	svc := New(store)
	svc.SetSettingsReader(&fakeSettingsReader{boolValue: true})
	svc.SetMembershipChecker(&fakeMembershipChecker{isActive: true})
	views, err := svc.Applicable(context.Background(), "pos", singleItem(100_000, uuid.New()), customerID)
	if err != nil {
		t.Fatalf("Applicable() error = %v, want nil", err)
	}
	if len(views) != 1 {
		t.Fatalf("Applicable() = %+v, want 1 item (active member)", views)
	}
}

// TestApplicable_Member_CustomerIDGiven_NotActiveMember_FilteredOut —
// customer_id dikirim tapi bukan member aktif -> diskon member disaring.
func TestApplicable_Member_CustomerIDGiven_NotActiveMember_FilteredOut(t *testing.T) {
	id := uuid.New()
	customerID := uuid.New()
	store := &fakeDiscountStore{
		activeResult: []model.Discount{{
			ID: id, Code: "MEMBER10", Name: "Diskon Member",
			Type: model.DiscountTypePercent, ValuePercent: floatPtr(10),
			IsActive: true, ChannelScope: model.ChannelScopeAll,
			AudienceScope: model.AudienceScopeMember,
			MemberScope:   memberScopePtr(model.MemberScopeAllMembers),
		}},
	}
	svc := New(store)
	svc.SetSettingsReader(&fakeSettingsReader{boolValue: true})
	svc.SetMembershipChecker(&fakeMembershipChecker{isActive: false})
	views, err := svc.Applicable(context.Background(), "pos", singleItem(100_000, uuid.New()), customerID)
	if err != nil {
		t.Fatalf("Applicable() error = %v, want nil", err)
	}
	if len(views) != 0 {
		t.Fatalf("Applicable() = %+v, want empty (not an active member)", views)
	}
}

func TestCreate_NominalPerM2_HappyPath(t *testing.T) {
	store := &fakeDiscountStore{}
	svc := New(store)
	view, err := svc.Create(context.Background(), CreateInput{
		Code:              "BANNER2K",
		Name:              "Diskon Banner 2rb/m2",
		Type:              "nominal_per_m2",
		ValueAmount:       int64Ptr(2000),
		MaxDiscountAmount: int64Ptr(50000),
	})
	if err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}
	if store.created.Type != model.DiscountTypeNominalPerM2 {
		t.Fatalf("stored discount type = %q, want nominal_per_m2", store.created.Type)
	}
	if view.Type != "nominal_per_m2" {
		t.Fatalf("view type = %q, want nominal_per_m2", view.Type)
	}
	if view.ValueAmount == nil || *view.ValueAmount != 2000 {
		t.Fatalf("view value_amount = %v, want 2000", view.ValueAmount)
	}
}

func TestApplicable_NominalPerM2_CalculatesByArea(t *testing.T) {
	id := uuid.New()
	bannerPID := uuid.New()
	store := &fakeDiscountStore{
		activeResult: []model.Discount{{
			ID:            id,
			Code:          "BANNER2K",
			Name:          "Diskon Banner 2k",
			Type:          model.DiscountTypeNominalPerM2,
			ValueAmount:   int64Ptr(2000),
			IsActive:      true,
			ChannelScope:  model.ChannelScopeAll,
			AudienceScope: model.AudienceScopeAll,
			AppliesTo:     model.AppliesToAll,
		}},
	}
	svc := New(store)

	items := []discountapi.ResolveItem{
		{
			LineNo:       1,
			ProductID:    bannerPID,
			Subtotal:     150_000,
			PricingType:  "per_m2",
			WidthCm:      200,
			HeightCm:     300,
			Quantity:     1,
			ChargeableM2: 6.0,
		},
	}
	views, err := svc.Applicable(context.Background(), "pos", items, uuid.Nil)
	if err != nil {
		t.Fatalf("Applicable() error = %v, want nil", err)
	}
	if len(views) != 1 {
		t.Fatalf("Applicable() len = %d, want 1", len(views))
	}
	// 6 m2 * 2000 = 12000
	if views[0].PreviewAmount != 12000 {
		t.Fatalf("PreviewAmount = %d, want 12000", views[0].PreviewAmount)
	}
}
