package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/catalog/catalogapi"
	"github.com/rajaku-printing/backend/internal/order/model"
	"github.com/rajaku-printing/backend/internal/order/orderapi"
	"github.com/rajaku-printing/backend/internal/order/repository"
	"github.com/rajaku-printing/backend/internal/order/state"
)

// ---------- fakes ----------

type fakeStore struct {
	createErrs   []error // consumed FIFO per CreateWithHistory call
	createCalls  int
	saved        *model.Order
	savedHistory *model.OrderStateHistory

	findResi  string
	findOrder *model.Order
	// findOrders — queue consumed FIFO by FindByResi (for tests that need
	// different results across multiple calls, e.g. re-read after update).
	findOrders []*model.Order
	findErr    error

	history    []model.OrderStateHistory
	historyErr error

	// Admin ops:
	listResult *repository.AdminListResult
	listErr    error
	listCalls  int
	// Filter terakhir yang diterima ListByCustomer — dipakai memastikan
	// scoping customer benar-benar diteruskan ke query (kalau CustomerID
	// hilang, satu customer bisa lihat order customer lain).
	listByCustomerFilter repository.CustomerListFilter

	setShippingParams repository.SetShippingCostParams
	setShippingErr    error
	setShippingCalls  int

	advanceParams repository.AdvanceStatusParams
	advanceErr    error
	advanceCalls  int

	// Super admin order tools (§ super admin order tools):
	updateFieldsParams repository.UpdateFieldsParams
	updateFieldsErr    error
	updateFieldsCalls  int

	overrideStatusParams repository.OverrideStatusParams
	overrideStatusErr    error
	overrideStatusCalls  int

	softDeleteParams repository.SoftDeleteParams
	softDeleteErr    error
	softDeleteCalls  int

	// Order recap (§28.5):
	recapSummary    *repository.RecapSummary
	recapRows       []repository.RecapRow
	recapErr        error
	lastRecapFilter repository.RecapFilter

	// RecapListBatch call tracking (temuan review #4b — streaming export):
	// each entry records the offset/limit RecapListBatch was called with, so
	// tests can assert the service paginates instead of loading everything
	// in one shot.
	recapBatchCalls []recapBatchCall

	// Recap filters dropdown (fitur baru §28 — GET /admin/order-recap/filters):
	recapKasirOptions []repository.RecapKasirOption
	recapDiscountRows []repository.RecapDiscountSnapshotRow
	recapFiltersErr   error
}

type recapBatchCall struct {
	Offset, Limit int
}

func (f *fakeStore) CreateWithHistory(_ context.Context, o *model.Order, h *model.OrderStateHistory) error {
	f.createCalls++
	var err error
	if len(f.createErrs) > 0 {
		err = f.createErrs[0]
		f.createErrs = f.createErrs[1:]
	}
	if err == nil {
		o.ID = uuid.New()
		o.CreatedAt = time.Now().UTC()
		f.saved = o
		f.savedHistory = h
	}
	return err
}

func (f *fakeStore) FindByResi(_ context.Context, resi string) (*model.Order, error) {
	f.findResi = resi
	if f.findErr != nil {
		return nil, f.findErr
	}
	if len(f.findOrders) > 0 {
		o := f.findOrders[0]
		f.findOrders = f.findOrders[1:]
		return o, nil
	}
	return f.findOrder, nil
}

func (f *fakeStore) FindByID(_ context.Context, _ uuid.UUID) (*model.Order, error) {
	if f.findErr != nil {
		return nil, f.findErr
	}
	if len(f.findOrders) > 0 {
		o := f.findOrders[0]
		f.findOrders = f.findOrders[1:]
		return o, nil
	}
	return f.findOrder, nil
}

func (f *fakeStore) ListForAdmin(_ context.Context, _ repository.AdminListFilter) (*repository.AdminListResult, error) {
	f.listCalls++
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.listResult, nil
}

func (f *fakeStore) ListByCustomer(_ context.Context, filter repository.CustomerListFilter) (*repository.AdminListResult, error) {
	f.listCalls++
	f.listByCustomerFilter = filter
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.listResult, nil
}

func (f *fakeStore) SetShippingCostAndAdvance(_ context.Context, p repository.SetShippingCostParams) error {
	f.setShippingCalls++
	f.setShippingParams = p
	return f.setShippingErr
}

func (f *fakeStore) AdvanceStatus(_ context.Context, p repository.AdvanceStatusParams) error {
	f.advanceCalls++
	f.advanceParams = p
	return f.advanceErr
}

func (f *fakeStore) FindHistoryByOrderID(_ context.Context, _ uuid.UUID) ([]model.OrderStateHistory, error) {
	if f.historyErr != nil {
		return nil, f.historyErr
	}
	return f.history, nil
}

func (f *fakeStore) ListPOSByDateRange(_ context.Context, _, _ time.Time) ([]model.Order, error) {
	return nil, nil
}

// recapSummary/recapRows/recapErr — configurable stub result for the order
// recap tests (recap_test.go).
func (f *fakeStore) RecapSummary(_ context.Context, _ repository.RecapFilter) (*repository.RecapSummary, error) {
	if f.recapErr != nil {
		return nil, f.recapErr
	}
	if f.recapSummary != nil {
		return f.recapSummary, nil
	}
	return &repository.RecapSummary{}, nil
}

func (f *fakeStore) RecapList(_ context.Context, filter repository.RecapFilter) ([]repository.RecapRow, error) {
	f.lastRecapFilter = filter
	if f.recapErr != nil {
		return nil, f.recapErr
	}
	return f.recapRows, nil
}

// RecapListBatch slices f.recapRows by [offset, offset+limit) — mirrors the
// real repository's Offset().Limit() behaviour closely enough to exercise
// the service's batching loop (temuan review #4b).
func (f *fakeStore) RecapListBatch(_ context.Context, filter repository.RecapFilter, offset, limit int) ([]repository.RecapRow, error) {
	f.lastRecapFilter = filter
	f.recapBatchCalls = append(f.recapBatchCalls, recapBatchCall{Offset: offset, Limit: limit})
	if f.recapErr != nil {
		return nil, f.recapErr
	}
	if offset >= len(f.recapRows) {
		return nil, nil
	}
	end := offset + limit
	if end > len(f.recapRows) {
		end = len(f.recapRows)
	}
	return f.recapRows[offset:end], nil
}

func (f *fakeStore) RecapDistinctKasir(_ context.Context, _, _ time.Time) ([]repository.RecapKasirOption, error) {
	if f.recapFiltersErr != nil {
		return nil, f.recapFiltersErr
	}
	return f.recapKasirOptions, nil
}

func (f *fakeStore) RecapDistinctDiscounts(_ context.Context, _, _ time.Time) ([]repository.RecapDiscountSnapshotRow, error) {
	if f.recapFiltersErr != nil {
		return nil, f.recapFiltersErr
	}
	return f.recapDiscountRows, nil
}

func (f *fakeStore) UpdateFields(_ context.Context, p repository.UpdateFieldsParams) error {
	f.updateFieldsCalls++
	f.updateFieldsParams = p
	return f.updateFieldsErr
}

func (f *fakeStore) OverrideStatus(_ context.Context, p repository.OverrideStatusParams) (string, error) {
	f.overrideStatusCalls++
	f.overrideStatusParams = p
	if f.overrideStatusErr != nil {
		return "", f.overrideStatusErr
	}
	return "dibayar", nil
}

func (f *fakeStore) SoftDelete(_ context.Context, p repository.SoftDeleteParams) error {
	f.softDeleteCalls++
	f.softDeleteParams = p
	return f.softDeleteErr
}

// fakeAuditStore implements AuditStore for ListAuditLog tests.
type fakeAuditStore struct {
	rows   []model.AdminAuditLog
	err    error
	filter repository.AdminAuditListFilter
	calls  int
}

func (f *fakeAuditStore) ListByEntity(_ context.Context, filter repository.AdminAuditListFilter) ([]model.AdminAuditLog, error) {
	f.calls++
	f.filter = filter
	if f.err != nil {
		return nil, f.err
	}
	return f.rows, nil
}

type fakeCatalog struct {
	quoteResult *catalogapi.QuoteResult
	quoteErr    error
	quoteCalls  int
}

func (f *fakeCatalog) ResolveProductBySlug(_ context.Context, _ string) (*catalogapi.ProductSummary, error) {
	return nil, errors.New("not used")
}
func (f *fakeCatalog) ResolveProductByID(_ context.Context, _ uuid.UUID) (*catalogapi.ProductSummary, error) {
	return nil, errors.New("not used")
}
func (f *fakeCatalog) Quote(_ context.Context, _ catalogapi.QuoteRequest) (*catalogapi.QuoteResult, error) {
	f.quoteCalls++
	if f.quoteErr != nil {
		return nil, f.quoteErr
	}
	return f.quoteResult, nil
}

type fakeCustomers struct {
	identity *authapi.Identity
	err      error
	calls    int
}

func (f *fakeCustomers) ResolveOrCreateGuest(_ context.Context, _, _ string) (*authapi.Identity, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.identity, nil
}

// FindByID — dummy impl agar fake satisfy authapi.CustomerService.FindByID
// (dipakai notification module). Order-side tests tidak butuh perilaku ini,
// jadi cukup return identity yang sudah di-set.
func (f *fakeCustomers) FindByID(_ context.Context, _ uuid.UUID) (*authapi.Identity, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.identity, nil
}

// SearchCustomers — dummy impl agar fake satisfy authapi.CustomerService.
// Order-side tests tidak butuh perilaku ini.
func (f *fakeCustomers) SearchCustomers(_ context.Context, _ string, _ int) ([]authapi.Identity, error) {
	return nil, f.err
}

// GetMembershipInfo/UpdateMembershipStatus/ListMembers (§30) — dummy impls
// agar fake satisfy authapi.CustomerService. Order-side tests tidak butuh
// perilaku ini.
func (f *fakeCustomers) GetMembershipInfo(_ context.Context, _ uuid.UUID) (*authapi.MembershipInfo, error) {
	return nil, f.err
}

func (f *fakeCustomers) UpdateMembershipStatus(_ context.Context, _ authapi.UpdateMembershipStatusInput) error {
	return f.err
}

func (f *fakeCustomers) ListMembers(_ context.Context, _ authapi.ListMembershipFilter) (*authapi.ListMembershipResult, error) {
	return nil, f.err
}

// ---------- helpers ----------

func newQuote(productID, materialID uuid.UUID) *catalogapi.QuoteResult {
	return &catalogapi.QuoteResult{
		ProductID:    productID,
		ProductName:  "Banner Flexi",
		MaterialID:   materialID,
		MaterialName: "Flexi China 280gsm",
		PricingType:  catalogapi.PricingTypePerM2,
		WidthCm:      100,
		HeightCm:     200,
		TotalPrice:   50000,
	}
}

func baseInput(productID, materialID uuid.UUID) CreateOnlineOrderInput {
	return CreateOnlineOrderInput{
		GuestPhone:   "081234567890",
		GuestName:    "Ani Testing",
		ProductID:    productID,
		MaterialID:   materialID,
		WidthCm:      100,
		HeightCm:     200,
		Quantity:     2,
		MetodeAmbil:  model.MetodeAmbilPickup,
		DesignSource: model.DesignSourceUpload,
	}
}

// ---------- tests ----------

func TestCreateOnlineOrder_HappyPath_PickupGuest(t *testing.T) {
	productID, materialID := uuid.New(), uuid.New()
	guestID := uuid.New()

	store := &fakeStore{}
	catalog := &fakeCatalog{quoteResult: newQuote(productID, materialID)}
	customers := &fakeCustomers{identity: &authapi.Identity{UserID: guestID, UserType: authapi.UserTypeCustomer}}

	svc := New(store, catalog, customers)

	got, err := svc.CreateOnlineOrder(context.Background(), baseInput(productID, materialID))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if customers.calls != 1 {
		t.Errorf("expected ResolveOrCreateGuest called once, got %d", customers.calls)
	}
	if catalog.quoteCalls != 1 {
		t.Errorf("expected Quote called once, got %d", catalog.quoteCalls)
	}
	if store.createCalls != 1 {
		t.Errorf("expected 1 create call, got %d", store.createCalls)
	}
	if got.CustomerID != guestID {
		t.Errorf("customer id mismatch: want %s got %s", guestID, got.CustomerID)
	}
	if got.Status != state.OrderMasuk {
		t.Errorf("initial status want %s got %s", state.OrderMasuk, got.Status)
	}
	if got.Channel != model.ChannelOnline {
		t.Errorf("channel want online got %s", got.Channel)
	}
	if got.Subtotal != 100000 { // 50000 * qty 2
		t.Errorf("subtotal want 100000 got %d", got.Subtotal)
	}
	if got.Total != 100000 {
		t.Errorf("total want 100000 (no shipping yet) got %d", got.Total)
	}
	if got.Resi == "" || !strings.HasPrefix(got.Resi, "RJK-") {
		t.Errorf("resi malformed: %q", got.Resi)
	}
	if store.savedHistory == nil || store.savedHistory.ToStatus != state.OrderMasuk {
		t.Errorf("initial state_history not written correctly: %+v", store.savedHistory)
	}
}

func TestCreateOnlineOrder_KirimMissingShippingFields_Rejected(t *testing.T) {
	productID, materialID := uuid.New(), uuid.New()
	svc := New(&fakeStore{}, &fakeCatalog{quoteResult: newQuote(productID, materialID)},
		&fakeCustomers{identity: &authapi.Identity{UserID: uuid.New()}})

	in := baseInput(productID, materialID)
	in.MetodeAmbil = model.MetodeAmbilKirim
	// omit shipping fields

	_, err := svc.CreateOnlineOrder(context.Background(), in)
	if !errors.Is(err, orderapi.ErrShippingFieldsRequired) {
		t.Fatalf("want ErrShippingFieldsRequired, got %v", err)
	}
}

func TestCreateOnlineOrder_KirimNormalizesShippingPhone(t *testing.T) {
	productID, materialID := uuid.New(), uuid.New()
	store := &fakeStore{}
	svc := New(store, &fakeCatalog{quoteResult: newQuote(productID, materialID)},
		&fakeCustomers{identity: &authapi.Identity{UserID: uuid.New()}})

	in := baseInput(productID, materialID)
	in.MetodeAmbil = model.MetodeAmbilKirim
	in.ShippingAddress = "Jl. Merdeka No. 12"
	in.ShippingRecipientName = "Ani"
	in.ShippingRecipientPhone = "081234567890" // → 6281234567890

	got, err := svc.CreateOnlineOrder(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got.ShippingRecipientPhone == nil || *got.ShippingRecipientPhone != "6281234567890" {
		t.Errorf("phone not normalized to 62-format, got %v", got.ShippingRecipientPhone)
	}
}

func TestCreateOnlineOrder_InvalidDesignSource_Rejected(t *testing.T) {
	productID, materialID := uuid.New(), uuid.New()
	svc := New(&fakeStore{}, &fakeCatalog{quoteResult: newQuote(productID, materialID)},
		&fakeCustomers{identity: &authapi.Identity{UserID: uuid.New()}})

	in := baseInput(productID, materialID)
	in.DesignSource = "" // invalid

	_, err := svc.CreateOnlineOrder(context.Background(), in)
	if err == nil {
		t.Fatal("expected error for empty design_source, got nil")
	}
}

func TestCreateOnlineOrder_ResiCollisionGivesUp(t *testing.T) {
	productID, materialID := uuid.New(), uuid.New()

	// Every attempt returns ErrResiConflict → service should retry then give up
	// with ErrResiCollisionGaveUp after resiRetryAttempts (8).
	always := make([]error, resiRetryAttempts)
	for i := range always {
		always[i] = repository.ErrResiConflict
	}
	store := &fakeStore{createErrs: always}
	svc := New(store, &fakeCatalog{quoteResult: newQuote(productID, materialID)},
		&fakeCustomers{identity: &authapi.Identity{UserID: uuid.New()}})

	_, err := svc.CreateOnlineOrder(context.Background(), baseInput(productID, materialID))
	if !errors.Is(err, orderapi.ErrResiCollisionGaveUp) {
		t.Fatalf("want ErrResiCollisionGaveUp got %v", err)
	}
	if store.createCalls != resiRetryAttempts {
		t.Errorf("want %d create attempts, got %d", resiRetryAttempts, store.createCalls)
	}
}

func TestCreateOnlineOrder_ResiRetrySucceedsAfterConflict(t *testing.T) {
	productID, materialID := uuid.New(), uuid.New()
	// Fail first attempt with conflict, succeed second.
	store := &fakeStore{createErrs: []error{repository.ErrResiConflict}}
	svc := New(store, &fakeCatalog{quoteResult: newQuote(productID, materialID)},
		&fakeCustomers{identity: &authapi.Identity{UserID: uuid.New()}})

	got, err := svc.CreateOnlineOrder(context.Background(), baseInput(productID, materialID))
	if err != nil {
		t.Fatalf("expected success after retry, got %v", err)
	}
	if store.createCalls != 2 {
		t.Errorf("want 2 create attempts (1 conflict + 1 success), got %d", store.createCalls)
	}
	if got.Resi == "" {
		t.Errorf("resi not set after retry")
	}
}

func TestCreateOnlineOrder_CatalogQuoteFailure_Bubbles(t *testing.T) {
	productID, materialID := uuid.New(), uuid.New()
	store := &fakeStore{}
	svc := New(store, &fakeCatalog{quoteErr: catalogapi.ErrProductNotFound},
		&fakeCustomers{identity: &authapi.Identity{UserID: uuid.New()}})

	_, err := svc.CreateOnlineOrder(context.Background(), baseInput(productID, materialID))
	if !errors.Is(err, catalogapi.ErrProductNotFound) {
		t.Fatalf("want catalog ErrProductNotFound to bubble, got %v", err)
	}
	if store.createCalls != 0 {
		t.Errorf("store should not be called when quote fails, calls=%d", store.createCalls)
	}
}

func TestCreateOnlineOrder_LoggedInSkipsGuestResolve(t *testing.T) {
	productID, materialID := uuid.New(), uuid.New()
	loggedIn := uuid.New()

	store := &fakeStore{}
	customers := &fakeCustomers{err: errors.New("should not be called")}
	svc := New(store, &fakeCatalog{quoteResult: newQuote(productID, materialID)}, customers)

	in := baseInput(productID, materialID)
	in.CustomerID = &loggedIn
	in.GuestPhone = ""
	in.GuestName = ""

	got, err := svc.CreateOnlineOrder(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if customers.calls != 0 {
		t.Errorf("ResolveOrCreateGuest must be skipped when CustomerID present, calls=%d", customers.calls)
	}
	if got.CustomerID != loggedIn {
		t.Errorf("customer id want %s got %s", loggedIn, got.CustomerID)
	}
}

func TestGetByResiForOwner_StaffSeesAny(t *testing.T) {
	orderCustomer := uuid.New()
	staff := uuid.New()
	stored := &model.Order{ID: uuid.New(), Resi: "RJK-STAFFSEE", CustomerID: orderCustomer}
	store := &fakeStore{findOrder: stored}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	got, err := svc.GetByResiForOwner(context.Background(), "RJK-STAFFSEE",
		&authapi.Identity{UserID: staff, UserType: authapi.UserTypeStaff})
	if err != nil {
		t.Fatalf("staff should see any order, got %v", err)
	}
	if got.ID != stored.ID {
		t.Errorf("order id mismatch")
	}
}

func TestGetByResiForOwner_CustomerNotOwner_Rejected(t *testing.T) {
	owner := uuid.New()
	other := uuid.New()
	stored := &model.Order{ID: uuid.New(), Resi: "RJK-OTHERONE", CustomerID: owner}
	store := &fakeStore{findOrder: stored}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	_, err := svc.GetByResiForOwner(context.Background(), "RJK-OTHERONE",
		&authapi.Identity{UserID: other, UserType: authapi.UserTypeCustomer})
	if !errors.Is(err, orderapi.ErrNotOwner) {
		t.Fatalf("want ErrNotOwner got %v", err)
	}
}

func TestGetByResiForOwner_NotFoundMapsToOrderNotFound(t *testing.T) {
	store := &fakeStore{findErr: repository.ErrNotFound}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	_, err := svc.GetByResiForOwner(context.Background(), "RJK-ABSENT",
		&authapi.Identity{UserID: uuid.New(), UserType: authapi.UserTypeCustomer})
	if !errors.Is(err, orderapi.ErrOrderNotFound) {
		t.Fatalf("want ErrOrderNotFound got %v", err)
	}
}

func TestGetByResiPublic_CensorsShippingFields(t *testing.T) {
	addr := "Jl. Merdeka Barat No. 42 RT 03 RW 05 Jakarta Pusat"
	name := "Ani Testing"
	phone62 := "6281234567890"
	stored := &model.Order{
		ID: uuid.New(), Resi: "RJK-TRACK123",
		Status: state.MenungguPembayaran, Channel: model.ChannelOnline,
		MetodeAmbil:            model.MetodeAmbilKirim,
		ProductNameSnapshot:    "Banner Flexi",
		MaterialNameSnapshot:   "Flexi 280",
		ShippingAddress:        &addr,
		ShippingRecipientName:  &name,
		ShippingRecipientPhone: &phone62,
		CreatedAt:              time.Now().UTC(),
	}
	hist := []model.OrderStateHistory{
		{ID: uuid.New(), OrderID: stored.ID, ToStatus: state.OrderMasuk, ChangedAt: time.Now().Add(-1 * time.Hour)},
		{ID: uuid.New(), OrderID: stored.ID, ToStatus: state.MenungguPembayaran, ChangedAt: time.Now()},
	}
	store := &fakeStore{findOrder: stored, history: hist}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	got, err := svc.GetByResiPublic(context.Background(), "RJK-TRACK123")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got.Resi != stored.Resi {
		t.Errorf("resi mismatch")
	}
	if len(got.History) != 2 {
		t.Fatalf("want 2 history rows, got %d", len(got.History))
	}
	// Shipping fields must be censored, not raw.
	if got.ShippingAddressMasked == addr {
		t.Errorf("shipping address not censored: %q", got.ShippingAddressMasked)
	}
	if !strings.HasSuffix(got.ShippingAddressMasked, "**") {
		t.Errorf("shipping address should end with **, got %q", got.ShippingAddressMasked)
	}
	if got.ShippingRecipient == name {
		t.Errorf("recipient name not censored: %q", got.ShippingRecipient)
	}
	if got.ShippingPhoneMasked == phone62 || got.ShippingPhoneMasked == "" {
		t.Errorf("phone not masked properly: %q", got.ShippingPhoneMasked)
	}
	if strings.Contains(got.ShippingPhoneMasked, "6281234567890") {
		t.Errorf("phone leaked raw digits: %q", got.ShippingPhoneMasked)
	}
}

func TestGetByResiPublic_NotFound(t *testing.T) {
	store := &fakeStore{findErr: repository.ErrNotFound}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	_, err := svc.GetByResiPublic(context.Background(), "RJK-NONE")
	if !errors.Is(err, orderapi.ErrOrderNotFound) {
		t.Fatalf("want ErrOrderNotFound got %v", err)
	}
}

// ---------- admin ops ----------

func TestListForAdmin_PassesFilterAndReturnsPage(t *testing.T) {
	store := &fakeStore{listResult: &repository.AdminListResult{
		Items:    []model.Order{{ID: uuid.New(), Resi: "RJK-ONE"}, {ID: uuid.New(), Resi: "RJK-TWO"}},
		Total:    2,
		Page:     1,
		PageSize: 20,
	}}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	page, err := svc.ListForAdmin(context.Background(), AdminListInput{Status: "order_masuk", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if store.listCalls != 1 {
		t.Errorf("want 1 list call, got %d", store.listCalls)
	}
	if page.Total != 2 || len(page.Items) != 2 {
		t.Errorf("page mismatch: %+v", page)
	}
}

// ---------- customer ops ----------

// Scoping customer adalah batas keamanan, bukan sekadar filter tampilan:
// kalau CustomerID tidak ikut turun ke query, satu customer bisa melihat
// order milik orang lain.
func TestListForCustomer_ScopesQueryToCallerCustomerID(t *testing.T) {
	custID := uuid.New()
	store := &fakeStore{listResult: &repository.AdminListResult{
		Items:    []model.Order{{ID: uuid.New(), Resi: "RJK-MINE"}},
		Total:    1,
		Page:     1,
		PageSize: 10,
	}}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	page, err := svc.ListForCustomer(context.Background(), CustomerListInput{
		CustomerID: custID,
		Status:     "dibayar",
		Page:       1,
		PageSize:   10,
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if store.listByCustomerFilter.CustomerID != custID {
		t.Errorf("customer scoping hilang: want %s, got %s", custID, store.listByCustomerFilter.CustomerID)
	}
	if store.listByCustomerFilter.Status != "dibayar" {
		t.Errorf("status filter want dibayar, got %q", store.listByCustomerFilter.Status)
	}
	if page.Total != 1 || len(page.Items) != 1 {
		t.Errorf("page mismatch: %+v", page)
	}
}

func TestListForCustomer_StoreErrorWrapped(t *testing.T) {
	store := &fakeStore{listErr: errors.New("db down")}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	_, err := svc.ListForCustomer(context.Background(), CustomerListInput{CustomerID: uuid.New()})
	if err == nil {
		t.Fatal("want error saat store gagal")
	}
	if !strings.Contains(err.Error(), "list customer orders") {
		t.Errorf("error harus dibungkus konteks (§22), got %v", err)
	}
}

func TestSetShippingCost_HappyPath_FromOrderMasuk(t *testing.T) {
	orderID := uuid.New()
	staffID := uuid.New()
	initial := &model.Order{
		ID: orderID, Resi: "RJK-K1", Status: state.OrderMasuk,
		MetodeAmbil: model.MetodeAmbilKirim, Subtotal: 100000, Total: 100000,
	}
	// After update, the re-read returns updated order.
	updated := &model.Order{
		ID: orderID, Resi: "RJK-K1", Status: state.MenungguPembayaran,
		MetodeAmbil: model.MetodeAmbilKirim, Subtotal: 100000, Total: 115000,
	}
	store := &fakeStore{findOrders: []*model.Order{initial, updated}}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	got, err := svc.SetShippingCost(context.Background(), SetShippingCostInput{
		Resi: "RJK-K1", ShippingCost: 15000, StaffID: staffID,
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if store.setShippingCalls != 1 {
		t.Fatalf("want 1 setshipping call, got %d", store.setShippingCalls)
	}
	p := store.setShippingParams
	if p.ShippingCost != 15000 || p.NewTotal != 115000 {
		t.Errorf("params wrong: %+v", p)
	}
	if p.FromStatus != string(state.OrderMasuk) {
		t.Errorf("from_status want order_masuk got %s", p.FromStatus)
	}
	if p.Intermediate != string(state.MenungguOngkir) {
		t.Errorf("intermediate want menunggu_ongkir got %s", p.Intermediate)
	}
	if p.NewStatus != string(state.MenungguPembayaran) {
		t.Errorf("new_status want menunggu_pembayaran got %s", p.NewStatus)
	}
	if p.ChangedBy == nil || *p.ChangedBy != staffID {
		t.Errorf("changed_by wrong: %v", p.ChangedBy)
	}
	if got.Total != 115000 {
		t.Errorf("returned order.total want 115000 got %d", got.Total)
	}
}

func TestSetShippingCost_HappyPath_FromMenungguOngkir_NoIntermediate(t *testing.T) {
	orderID := uuid.New()
	initial := &model.Order{
		ID: orderID, Resi: "RJK-K2", Status: state.MenungguOngkir,
		MetodeAmbil: model.MetodeAmbilKirim, Subtotal: 50000,
	}
	updated := &model.Order{
		ID: orderID, Resi: "RJK-K2", Status: state.MenungguPembayaran,
		MetodeAmbil: model.MetodeAmbilKirim, Subtotal: 50000, Total: 55000,
	}
	store := &fakeStore{findOrders: []*model.Order{initial, updated}}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	_, err := svc.SetShippingCost(context.Background(), SetShippingCostInput{
		Resi: "RJK-K2", ShippingCost: 5000, StaffID: uuid.New(),
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if store.setShippingParams.Intermediate != "" {
		t.Errorf("intermediate must be empty when starting from menunggu_ongkir, got %q",
			store.setShippingParams.Intermediate)
	}
}

func TestSetShippingCost_PickupRejected(t *testing.T) {
	initial := &model.Order{
		ID: uuid.New(), Resi: "RJK-P1", Status: state.OrderMasuk,
		MetodeAmbil: model.MetodeAmbilPickup,
	}
	store := &fakeStore{findOrder: initial}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	_, err := svc.SetShippingCost(context.Background(), SetShippingCostInput{
		Resi: "RJK-P1", ShippingCost: 15000, StaffID: uuid.New(),
	})
	if !errors.Is(err, orderapi.ErrShippingCostOnPickup) {
		t.Fatalf("want ErrShippingCostOnPickup got %v", err)
	}
	if store.setShippingCalls != 0 {
		t.Errorf("must not touch store, calls=%d", store.setShippingCalls)
	}
}

func TestSetShippingCost_InvalidStatus_Rejected(t *testing.T) {
	initial := &model.Order{
		ID: uuid.New(), Resi: "RJK-K3", Status: state.Dibayar, // already past ongkir stage
		MetodeAmbil: model.MetodeAmbilKirim,
	}
	store := &fakeStore{findOrder: initial}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	_, err := svc.SetShippingCost(context.Background(), SetShippingCostInput{
		Resi: "RJK-K3", ShippingCost: 15000, StaffID: uuid.New(),
	})
	if !errors.Is(err, orderapi.ErrInvalidTransition) {
		t.Fatalf("want ErrInvalidTransition got %v", err)
	}
}

func TestSetShippingCost_NegativeCost_Rejected(t *testing.T) {
	svc := New(&fakeStore{}, &fakeCatalog{}, &fakeCustomers{})
	_, err := svc.SetShippingCost(context.Background(), SetShippingCostInput{
		Resi: "RJK-K4", ShippingCost: -1, StaffID: uuid.New(),
	})
	if !errors.Is(err, orderapi.ErrInvalidShippingCost) {
		t.Fatalf("want ErrInvalidShippingCost got %v", err)
	}
}

func TestSetShippingCost_StaleStateMapsToStateChanged(t *testing.T) {
	initial := &model.Order{
		ID: uuid.New(), Resi: "RJK-K5", Status: state.OrderMasuk,
		MetodeAmbil: model.MetodeAmbilKirim, Subtotal: 100000,
	}
	store := &fakeStore{findOrder: initial, setShippingErr: repository.ErrStaleState}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	_, err := svc.SetShippingCost(context.Background(), SetShippingCostInput{
		Resi: "RJK-K5", ShippingCost: 10000, StaffID: uuid.New(),
	})
	if !errors.Is(err, orderapi.ErrOrderStateChanged) {
		t.Fatalf("want ErrOrderStateChanged got %v", err)
	}
}

func TestConfirmPickupTotal_HappyPath(t *testing.T) {
	orderID := uuid.New()
	staffID := uuid.New()
	initial := &model.Order{
		ID: orderID, Resi: "RJK-P2", Status: state.OrderMasuk,
		MetodeAmbil: model.MetodeAmbilPickup, Total: 50000,
	}
	updated := &model.Order{
		ID: orderID, Resi: "RJK-P2", Status: state.MenungguPembayaran,
		MetodeAmbil: model.MetodeAmbilPickup, Total: 50000,
	}
	store := &fakeStore{findOrders: []*model.Order{initial, updated}}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	got, err := svc.ConfirmPickupTotal(context.Background(), "RJK-P2", staffID, "")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if store.advanceCalls != 1 {
		t.Fatalf("want 1 advance call, got %d", store.advanceCalls)
	}
	if store.advanceParams.NewStatus != string(state.MenungguPembayaran) {
		t.Errorf("new_status wrong: %s", store.advanceParams.NewStatus)
	}
	if store.advanceParams.ChangedBy == nil || *store.advanceParams.ChangedBy != staffID {
		t.Errorf("changed_by wrong")
	}
	if got.Status != state.MenungguPembayaran {
		t.Errorf("returned status wrong: %s", got.Status)
	}
}

func TestConfirmPickupTotal_KirimRejected(t *testing.T) {
	initial := &model.Order{
		ID: uuid.New(), Resi: "RJK-K6", Status: state.OrderMasuk,
		MetodeAmbil: model.MetodeAmbilKirim,
	}
	store := &fakeStore{findOrder: initial}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	_, err := svc.ConfirmPickupTotal(context.Background(), "RJK-K6", uuid.New(), "")
	if !errors.Is(err, orderapi.ErrConfirmPickupOnKirim) {
		t.Fatalf("want ErrConfirmPickupOnKirim got %v", err)
	}
}

func TestConfirmPickupTotal_WrongStatus_Rejected(t *testing.T) {
	initial := &model.Order{
		ID: uuid.New(), Resi: "RJK-P3", Status: state.Dibayar,
		MetodeAmbil: model.MetodeAmbilPickup,
	}
	store := &fakeStore{findOrder: initial}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	_, err := svc.ConfirmPickupTotal(context.Background(), "RJK-P3", uuid.New(), "")
	if !errors.Is(err, orderapi.ErrInvalidTransition) {
		t.Fatalf("want ErrInvalidTransition got %v", err)
	}
}

// ---------- OrderCommandService (consumed by payment module) ----------

func TestFindSummaryByResi_HappyPath(t *testing.T) {
	orderID := uuid.New()
	customerID := uuid.New()
	stored := &model.Order{
		ID: orderID, Resi: "RJK-SUM1", CustomerID: customerID,
		Status: state.MenungguPembayaran, Total: 100000,
		MetodeAmbil: model.MetodeAmbilKirim, Channel: model.ChannelOnline,
	}
	svc := New(&fakeStore{findOrder: stored}, &fakeCatalog{}, &fakeCustomers{})

	got, err := svc.FindSummaryByResi(context.Background(), "RJK-SUM1")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got.ID != orderID || got.CustomerID != customerID {
		t.Errorf("id/customer mismatch: %+v", got)
	}
	if got.Status != string(state.MenungguPembayaran) {
		t.Errorf("status wrong: %s", got.Status)
	}
	if got.MetodeAmbil != string(model.MetodeAmbilKirim) {
		t.Errorf("metode_ambil wrong: %s", got.MetodeAmbil)
	}
}

func TestFindSummaryByResi_NotFound(t *testing.T) {
	svc := New(&fakeStore{findErr: repository.ErrNotFound}, &fakeCatalog{}, &fakeCustomers{})
	_, err := svc.FindSummaryByResi(context.Background(), "RJK-NOPE")
	if !errors.Is(err, orderapi.ErrOrderNotFound) {
		t.Fatalf("want ErrOrderNotFound got %v", err)
	}
}

func TestMarkPendingVerification_HappyPath(t *testing.T) {
	orderID := uuid.New()
	actor := uuid.New()
	store := &fakeStore{}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	if err := svc.MarkPendingVerification(context.Background(), orderID, &actor, "customer uploaded"); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	p := store.advanceParams
	if store.advanceCalls != 1 {
		t.Fatalf("want 1 advance call, got %d", store.advanceCalls)
	}
	if p.NewStatus != string(state.MenungguVerifikasi) {
		t.Errorf("new_status wrong: %s", p.NewStatus)
	}
	// Payment module MUST send AllowedFromStatuses so re-upload from ditolak
	// works — not FromStatus with a single value.
	if len(p.AllowedFromStatuses) != 2 {
		t.Errorf("want 2 allowed statuses, got %v", p.AllowedFromStatuses)
	}
	found := map[string]bool{}
	for _, s := range p.AllowedFromStatuses {
		found[s] = true
	}
	if !found[string(state.MenungguPembayaran)] || !found[string(state.Ditolak)] {
		t.Errorf("allowed statuses must include menunggu_pembayaran + ditolak, got %v", p.AllowedFromStatuses)
	}
	if p.MetodeBayar != "" {
		t.Errorf("metode_bayar must be empty for verifying transition, got %q", p.MetodeBayar)
	}
}

func TestMarkPendingVerification_StaleStateMaps(t *testing.T) {
	store := &fakeStore{advanceErr: repository.ErrStaleState}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})
	err := svc.MarkPendingVerification(context.Background(), uuid.New(), nil, "")
	if !errors.Is(err, orderapi.ErrOrderStateChanged) {
		t.Fatalf("want ErrOrderStateChanged got %v", err)
	}
}

func TestMarkDibayar_HappyPath_RecordsMetodeBayar(t *testing.T) {
	orderID := uuid.New()
	actor := uuid.New()
	store := &fakeStore{}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	if err := svc.MarkDibayar(context.Background(), orderID, "transfer", &actor, ""); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	p := store.advanceParams
	if p.NewStatus != string(state.Dibayar) {
		t.Errorf("new_status wrong: %s", p.NewStatus)
	}
	if p.MetodeBayar != "transfer" {
		t.Errorf("metode_bayar not recorded: %q", p.MetodeBayar)
	}
	// Both menunggu_verifikasi (proof approve) and menunggu_pembayaran (POS) valid.
	found := map[string]bool{}
	for _, s := range p.AllowedFromStatuses {
		found[s] = true
	}
	if !found[string(state.MenungguVerifikasi)] || !found[string(state.MenungguPembayaran)] {
		t.Errorf("allowed statuses wrong: %v", p.AllowedFromStatuses)
	}
}

func TestMarkDibayar_EmptyMetodeBayar_Rejected(t *testing.T) {
	svc := New(&fakeStore{}, &fakeCatalog{}, &fakeCustomers{})
	err := svc.MarkDibayar(context.Background(), uuid.New(), "", nil, "")
	if err == nil || !strings.Contains(err.Error(), "metode_bayar") {
		t.Fatalf("want metode_bayar validation error, got %v", err)
	}
}

func TestMarkDitolak_HappyPath(t *testing.T) {
	orderID := uuid.New()
	actor := uuid.New()
	store := &fakeStore{}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	if err := svc.MarkDitolak(context.Background(), orderID, &actor, "bukti buram"); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	p := store.advanceParams
	if p.NewStatus != string(state.Ditolak) {
		t.Errorf("new_status wrong: %s", p.NewStatus)
	}
	if len(p.AllowedFromStatuses) != 1 || p.AllowedFromStatuses[0] != string(state.MenungguVerifikasi) {
		t.Errorf("only menunggu_verifikasi allowed to transition to ditolak, got %v", p.AllowedFromStatuses)
	}
	if p.Note == nil || *p.Note != "bukti buram" {
		t.Errorf("reason not carried as note: %v", p.Note)
	}
}

func TestMarkDitolak_EmptyReason_Rejected(t *testing.T) {
	svc := New(&fakeStore{}, &fakeCatalog{}, &fakeCustomers{})
	err := svc.MarkDitolak(context.Background(), uuid.New(), nil, "   ")
	if err == nil {
		t.Fatal("want error for empty reason")
	}
}
