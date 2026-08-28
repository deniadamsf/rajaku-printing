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
	"github.com/rajaku-printing/backend/internal/membership/membershipapi"
	"github.com/rajaku-printing/backend/internal/settings/settingsapi"
)

// fakeMembershipChecker — minimal membershipapi.Checker fake for
// service-level tests (§30.3).
type fakeMembershipChecker struct {
	isActive bool
	err      error
}

var _ membershipapi.Checker = (*fakeMembershipChecker)(nil)

func (f *fakeMembershipChecker) IsActiveMember(_ context.Context, _ uuid.UUID) (bool, error) {
	return f.isActive, f.err
}

// fakeSettingsReader — minimal settingsapi.Reader fake for service-level
// tests (§30.1/§30.3). Only GetBool is exercised by the discount module.
type fakeSettingsReader struct {
	boolValue bool
	boolErr   error
}

var _ settingsapi.Reader = (*fakeSettingsReader)(nil)

func (f *fakeSettingsReader) GetInt(_ context.Context, _ string) (int, error) {
	return 0, errors.New("fakeSettingsReader: GetInt not implemented")
}

func (f *fakeSettingsReader) GetBool(_ context.Context, _ string) (bool, error) {
	return f.boolValue, f.boolErr
}

// fakeDiscountStore — minimal DiscountStore fake for service-level tests
// (mirrors the fakeStore pattern used in order/service tests).
type fakeDiscountStore struct {
	findResult *model.Discount
	findErr    error

	usageOne    int64
	usageOneErr error

	usageMap map[uuid.UUID]int64
	usageErr error

	createErr         error
	created           *model.Discount
	createProductIDs  []uuid.UUID
	createCustomerIDs []uuid.UUID

	listResult []model.Discount
	listErr    error

	activeResult []model.Discount
	activeErr    error

	updateWithScopesErr error
	updateFields        map[string]any

	softDeleteErr    error
	softDeleteParams repository.SoftDeleteParams

	productIDsResult map[uuid.UUID][]uuid.UUID
	productIDsErr    error

	customerIDsResult map[uuid.UUID][]uuid.UUID
	customerIDsErr    error

	// replaceProductsID/replaceProductsIDs are only populated when
	// UpdateWithScopes is called with products.Replace=true — mirrors the
	// old standalone ReplaceProducts fake so existing test assertions keep
	// working unchanged.
	replaceProductsID  uuid.UUID
	replaceProductsIDs []uuid.UUID

	// replaceCustomersID/replaceCustomersIDs — mirror above, for the member
	// scope axis (§30.3), only populated when customers.Replace=true.
	replaceCustomersID  uuid.UUID
	replaceCustomersIDs []uuid.UUID
}

func (f *fakeDiscountStore) Create(_ context.Context, d *model.Discount, productIDs, customerIDs []uuid.UUID) error {
	f.created = d
	f.createProductIDs = productIDs
	f.createCustomerIDs = customerIDs
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

func (f *fakeDiscountStore) UpdateWithScopes(_ context.Context, id uuid.UUID, fields map[string]any, products, customers repository.ScopeReplace) error {
	f.updateFields = fields
	if products.Replace {
		f.replaceProductsID = id
		f.replaceProductsIDs = products.IDs
	}
	if customers.Replace {
		f.replaceCustomersID = id
		f.replaceCustomersIDs = customers.IDs
	}
	return f.updateWithScopesErr
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

func (f *fakeDiscountStore) CustomerIDs(_ context.Context, ids []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error) {
	if f.customerIDsErr != nil {
		return nil, f.customerIDsErr
	}
	if f.customerIDsResult != nil {
		return f.customerIDsResult, nil
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

// ---- Diskon khusus member (§30.3) ----

func TestResolveForOrder_MasterDiscount_Member_HappyPath(t *testing.T) {
	id := uuid.New()
	customerID := uuid.New()
	store := &fakeDiscountStore{
		findResult: &model.Discount{
			ID: id, Code: "MEMBER10", Name: "Diskon Member",
			Type: model.DiscountTypePercent, ValuePercent: floatPtr(10),
			IsActive: true, ChannelScope: model.ChannelScopeAll,
			AudienceScope: model.AudienceScopeMember,
			MemberScope:   memberScopePtr(model.MemberScopeAllMembers),
		},
	}
	svc := New(store)
	svc.SetSettingsReader(&fakeSettingsReader{boolValue: true})
	svc.SetMembershipChecker(&fakeMembershipChecker{isActive: true})
	snap, err := svc.ResolveForOrder(context.Background(), discountapi.ResolveInput{
		DiscountID: &id, Subtotal: 100_000, Channel: "pos", CustomerID: customerID,
	})
	if err != nil {
		t.Fatalf("ResolveForOrder() error = %v, want nil", err)
	}
	if snap.Amount != 10_000 {
		t.Fatalf("ResolveForOrder() amount = %d, want 10000", snap.Amount)
	}
}

// TestResolveForOrder_MasterDiscount_Member_NotActiveMember_Rejected — kasus
// gagal wajib: customer bukan member aktif.
func TestResolveForOrder_MasterDiscount_Member_NotActiveMember_Rejected(t *testing.T) {
	id := uuid.New()
	store := &fakeDiscountStore{
		findResult: &model.Discount{
			ID: id, Code: "MEMBER10", Name: "Diskon Member",
			Type: model.DiscountTypePercent, ValuePercent: floatPtr(10),
			IsActive: true, ChannelScope: model.ChannelScopeAll,
			AudienceScope: model.AudienceScopeMember,
			MemberScope:   memberScopePtr(model.MemberScopeAllMembers),
		},
	}
	svc := New(store)
	svc.SetSettingsReader(&fakeSettingsReader{boolValue: true})
	svc.SetMembershipChecker(&fakeMembershipChecker{isActive: false})
	_, err := svc.ResolveForOrder(context.Background(), discountapi.ResolveInput{
		DiscountID: &id, Subtotal: 100_000, Channel: "pos", CustomerID: uuid.New(),
	})
	if err != discountapi.ErrDiscountMembershipRequired {
		t.Fatalf("ResolveForOrder() error = %v, want ErrDiscountMembershipRequired", err)
	}
}

func TestResolveForOrder_MasterDiscount_Member_Disabled_Rejected(t *testing.T) {
	id := uuid.New()
	store := &fakeDiscountStore{
		findResult: &model.Discount{
			ID: id, Code: "MEMBER10", Name: "Diskon Member",
			Type: model.DiscountTypePercent, ValuePercent: floatPtr(10),
			IsActive: true, ChannelScope: model.ChannelScopeAll,
			AudienceScope: model.AudienceScopeMember,
			MemberScope:   memberScopePtr(model.MemberScopeAllMembers),
		},
	}
	svc := New(store)
	svc.SetSettingsReader(&fakeSettingsReader{boolValue: false})
	svc.SetMembershipChecker(&fakeMembershipChecker{isActive: true})
	_, err := svc.ResolveForOrder(context.Background(), discountapi.ResolveInput{
		DiscountID: &id, Subtotal: 100_000, Channel: "pos", CustomerID: uuid.New(),
	})
	if err != discountapi.ErrDiscountMembershipDisabled {
		t.Fatalf("ResolveForOrder() error = %v, want ErrDiscountMembershipDisabled", err)
	}
}

// TestResolveForOrder_MasterDiscount_Member_SelectedScope_Match — §30.3
// member_scope="selected_members".
func TestResolveForOrder_MasterDiscount_Member_SelectedScope_Match(t *testing.T) {
	id := uuid.New()
	customerID := uuid.New()
	store := &fakeDiscountStore{
		findResult: &model.Discount{
			ID: id, Code: "VIP10", Name: "Diskon VIP",
			Type: model.DiscountTypePercent, ValuePercent: floatPtr(10),
			IsActive: true, ChannelScope: model.ChannelScopeAll,
			AudienceScope: model.AudienceScopeMember,
			MemberScope:   memberScopePtr(model.MemberScopeSelected),
		},
		customerIDsResult: map[uuid.UUID][]uuid.UUID{id: {customerID}},
	}
	svc := New(store)
	svc.SetSettingsReader(&fakeSettingsReader{boolValue: true})
	svc.SetMembershipChecker(&fakeMembershipChecker{isActive: true})
	snap, err := svc.ResolveForOrder(context.Background(), discountapi.ResolveInput{
		DiscountID: &id, Subtotal: 100_000, Channel: "pos", CustomerID: customerID,
	})
	if err != nil {
		t.Fatalf("ResolveForOrder() error = %v, want nil", err)
	}
	if snap.Amount != 10_000 {
		t.Fatalf("ResolveForOrder() amount = %d, want 10000", snap.Amount)
	}
}

func TestResolveForOrder_MasterDiscount_Member_SelectedScope_Mismatch_Rejected(t *testing.T) {
	id := uuid.New()
	scopedCustomerID := uuid.New()
	otherCustomerID := uuid.New()
	store := &fakeDiscountStore{
		findResult: &model.Discount{
			ID: id, Code: "VIP10", Name: "Diskon VIP",
			Type: model.DiscountTypePercent, ValuePercent: floatPtr(10),
			IsActive: true, ChannelScope: model.ChannelScopeAll,
			AudienceScope: model.AudienceScopeMember,
			MemberScope:   memberScopePtr(model.MemberScopeSelected),
		},
		customerIDsResult: map[uuid.UUID][]uuid.UUID{id: {scopedCustomerID}},
	}
	svc := New(store)
	svc.SetSettingsReader(&fakeSettingsReader{boolValue: true})
	svc.SetMembershipChecker(&fakeMembershipChecker{isActive: true})
	_, err := svc.ResolveForOrder(context.Background(), discountapi.ResolveInput{
		DiscountID: &id, Subtotal: 100_000, Channel: "pos", CustomerID: otherCustomerID,
	})
	if err != discountapi.ErrDiscountMemberMismatch {
		t.Fatalf("ResolveForOrder() error = %v, want ErrDiscountMemberMismatch", err)
	}
}

// TestResolveForOrder_MasterDiscount_Member_CheckerNotWired_Rejected — §22
// no-silent-stub: kalau discountSvc belum di-wire dengan
// SetMembershipChecker/SetSettingsReader, diskon audience_scope="member"
// HARUS ditolak eksplisit (ErrDiscountMembershipUnavailable), bukan
// diam-diam dianggap "bukan member".
func TestResolveForOrder_MasterDiscount_Member_CheckerNotWired_Rejected(t *testing.T) {
	id := uuid.New()
	store := &fakeDiscountStore{
		findResult: &model.Discount{
			ID: id, Code: "MEMBER10", Name: "Diskon Member",
			Type: model.DiscountTypePercent, ValuePercent: floatPtr(10),
			IsActive: true, ChannelScope: model.ChannelScopeAll,
			AudienceScope: model.AudienceScopeMember,
			MemberScope:   memberScopePtr(model.MemberScopeAllMembers),
		},
	}
	svc := New(store) // sengaja TIDAK memanggil SetMembershipChecker/SetSettingsReader
	_, err := svc.ResolveForOrder(context.Background(), discountapi.ResolveInput{
		DiscountID: &id, Subtotal: 100_000, Channel: "pos", CustomerID: uuid.New(),
	})
	if err != discountapi.ErrDiscountMembershipUnavailable {
		t.Fatalf("ResolveForOrder() error = %v, want ErrDiscountMembershipUnavailable", err)
	}
}

// TestResolveForOrder_MasterDiscount_AudienceAll_NoMembershipQuery — diskon
// audience_scope="all" (mayoritas kasus) tidak boleh butuh
// SetMembershipChecker/SetSettingsReader ter-wire sama sekali — regression
// guard supaya jalur "all" tidak diam-diam mulai memanggil dependency
// membership yang belum tentu ada.
func TestResolveForOrder_MasterDiscount_AudienceAll_NoMembershipQuery(t *testing.T) {
	id := uuid.New()
	store := &fakeDiscountStore{
		findResult: &model.Discount{
			ID: id, Code: "GLOBAL10", Name: "Diskon Semua",
			Type: model.DiscountTypePercent, ValuePercent: floatPtr(10),
			IsActive: true, ChannelScope: model.ChannelScopeAll,
			AudienceScope: model.AudienceScopeAll,
		},
	}
	svc := New(store) // TIDAK di-wire, dan seharusnya tidak masalah untuk diskon "all"
	snap, err := svc.ResolveForOrder(context.Background(), discountapi.ResolveInput{
		DiscountID: &id, Subtotal: 100_000, Channel: "pos",
	})
	if err != nil {
		t.Fatalf("ResolveForOrder() error = %v, want nil", err)
	}
	if snap.Amount != 10_000 {
		t.Fatalf("ResolveForOrder() amount = %d, want 10000", snap.Amount)
	}
}
