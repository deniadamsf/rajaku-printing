package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/membership/membershipapi"
	"github.com/rajaku-printing/backend/internal/membership/model"
	"github.com/rajaku-printing/backend/internal/notification/notificationapi"
)

// ---------------------------------------------------------------------------
// fakes
// ---------------------------------------------------------------------------

type fakeCustomers struct {
	infos     map[uuid.UUID]*authapi.MembershipInfo
	updateErr error
	listRes   *authapi.ListMembershipResult
	listErr   error
}

func newFakeCustomers() *fakeCustomers {
	return &fakeCustomers{infos: map[uuid.UUID]*authapi.MembershipInfo{}}
}

func (f *fakeCustomers) ResolveOrCreateGuest(context.Context, string, string) (*authapi.Identity, error) {
	return nil, errors.New("fakeCustomers.ResolveOrCreateGuest: not used by these tests")
}

func (f *fakeCustomers) FindByID(context.Context, uuid.UUID) (*authapi.Identity, error) {
	return nil, errors.New("fakeCustomers.FindByID: not used by these tests")
}

func (f *fakeCustomers) SearchCustomers(context.Context, string, int) ([]authapi.Identity, error) {
	return nil, errors.New("fakeCustomers.SearchCustomers: not used by these tests")
}

func (f *fakeCustomers) GetMembershipInfo(_ context.Context, customerID uuid.UUID) (*authapi.MembershipInfo, error) {
	info, ok := f.infos[customerID]
	if !ok {
		return nil, authapi.ErrCustomerNotFound
	}
	cp := *info
	return &cp, nil
}

func (f *fakeCustomers) UpdateMembershipStatus(_ context.Context, in authapi.UpdateMembershipStatusInput) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	info, ok := f.infos[in.CustomerID]
	if !ok {
		return authapi.ErrCustomerNotFound
	}
	if info.Status != in.FromStatus {
		return authapi.ErrMembershipStatusConflict
	}
	info.Status = in.ToStatus
	if in.RequestedAt != nil {
		info.RequestedAt = in.RequestedAt
	}
	if in.ClearDecision {
		info.DecidedAt = nil
		info.DecidedBy = nil
		info.DecisionNote = ""
	} else {
		if in.DecidedAt != nil {
			info.DecidedAt = in.DecidedAt
		}
		if in.DecidedBy != nil {
			info.DecidedBy = in.DecidedBy
		}
		if in.Note != nil {
			info.DecisionNote = *in.Note
		}
	}
	return nil
}

func (f *fakeCustomers) ListMembers(context.Context, authapi.ListMembershipFilter) (*authapi.ListMembershipResult, error) {
	return f.listRes, f.listErr
}

type fakeLogs struct {
	records   []model.MembershipStatusLog
	createErr error
}

func (f *fakeLogs) Create(_ context.Context, l *model.MembershipStatusLog) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.records = append(f.records, *l)
	return nil
}

type fakeSettings struct {
	enabled bool
	err     error
}

func (f *fakeSettings) GetInt(context.Context, string) (int, error) {
	return 0, errors.New("fakeSettings.GetInt: not used by these tests")
}

func (f *fakeSettings) GetBool(_ context.Context, _ string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.enabled, nil
}

type fakeNotifier struct {
	calls    int
	lastKind notificationapi.Kind
	err      error
}

func (f *fakeNotifier) EnqueueCustomerEvent(_ context.Context, kind notificationapi.Kind, _ uuid.UUID, _ map[string]any, _ string) error {
	f.calls++
	f.lastKind = kind
	return f.err
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func newTestService(t *testing.T, enabled bool) (*Service, *fakeCustomers, *fakeLogs, *fakeNotifier) {
	t.Helper()
	customers := newFakeCustomers()
	logs := &fakeLogs{}
	notifier := &fakeNotifier{}
	svc := New(customers, logs)
	svc.SetSettingsReader(&fakeSettings{enabled: enabled})
	svc.SetNotifier(notifier)
	return svc, customers, logs, notifier
}

func seedCustomer(f *fakeCustomers, id uuid.UUID, customerType authapi.CustomerType, status membershipapi.Status) {
	f.infos[id] = &authapi.MembershipInfo{
		CustomerID:   id,
		Name:         "Budi",
		Phone:        "6281234567890",
		CustomerType: customerType,
		Status:       string(status),
	}
}

// ---------------------------------------------------------------------------
// Apply
// ---------------------------------------------------------------------------

func TestApply_HappyPath(t *testing.T) {
	svc, customers, logs, _ := newTestService(t, true)
	id := uuid.New()
	seedCustomer(customers, id, authapi.CustomerTypeRegistered, membershipapi.StatusNone)

	view, err := svc.Apply(context.Background(), id)
	if err != nil {
		t.Fatalf("Apply() error = %v, want nil", err)
	}
	if view.Status != string(membershipapi.StatusPending) {
		t.Fatalf("Apply() status = %q, want pending", view.Status)
	}
	if view.RequestedAt == nil {
		t.Fatal("Apply() RequestedAt = nil, want set")
	}
	if len(logs.records) != 1 {
		t.Fatalf("expected 1 audit log row, got %d", len(logs.records))
	}
	if logs.records[0].FromStatus != string(membershipapi.StatusNone) ||
		logs.records[0].ToStatus != string(membershipapi.StatusPending) {
		t.Fatalf("unexpected log transition: %+v", logs.records[0])
	}
}

func TestApply_RejectedCanReapply(t *testing.T) {
	svc, customers, _, _ := newTestService(t, true)
	id := uuid.New()
	seedCustomer(customers, id, authapi.CustomerTypeRegistered, membershipapi.StatusRejected)

	view, err := svc.Apply(context.Background(), id)
	if err != nil {
		t.Fatalf("Apply() error = %v, want nil", err)
	}
	if view.Status != string(membershipapi.StatusPending) {
		t.Fatalf("Apply() status = %q, want pending", view.Status)
	}
}

func TestApply_Disabled(t *testing.T) {
	svc, customers, _, _ := newTestService(t, false)
	id := uuid.New()
	seedCustomer(customers, id, authapi.CustomerTypeRegistered, membershipapi.StatusNone)

	_, err := svc.Apply(context.Background(), id)
	if !errors.Is(err, membershipapi.ErrMembershipDisabled) {
		t.Fatalf("Apply() error = %v, want ErrMembershipDisabled", err)
	}
}

func TestApply_RequiresRegisteredAccount(t *testing.T) {
	svc, customers, _, _ := newTestService(t, true)
	id := uuid.New()
	seedCustomer(customers, id, authapi.CustomerTypeGuest, membershipapi.StatusNone)

	_, err := svc.Apply(context.Background(), id)
	if !errors.Is(err, membershipapi.ErrMembershipRequiresRegisteredAccount) {
		t.Fatalf("Apply() error = %v, want ErrMembershipRequiresRegisteredAccount", err)
	}
}

func TestApply_InvalidTransitionFromActive(t *testing.T) {
	svc, customers, _, _ := newTestService(t, true)
	id := uuid.New()
	seedCustomer(customers, id, authapi.CustomerTypeRegistered, membershipapi.StatusActive)

	_, err := svc.Apply(context.Background(), id)
	if !errors.Is(err, membershipapi.ErrMembershipInvalidTransition) {
		t.Fatalf("Apply() error = %v, want ErrMembershipInvalidTransition", err)
	}
}

// TestApply_ReapplyClearsPriorDecision — reviewer bug: apply -> reject
// (dengan note) -> apply lagi harus mengosongkan decided_at/decided_by/note
// dari penolakan sebelumnya, bukan meninggalkannya menempel di pengajuan
// baru yang statusnya pending lagi (§30.2).
func TestApply_ReapplyClearsPriorDecision(t *testing.T) {
	svc, customers, _, _ := newTestService(t, true)
	id := uuid.New()
	seedCustomer(customers, id, authapi.CustomerTypeRegistered, membershipapi.StatusNone)
	ctx := context.Background()

	if _, err := svc.Apply(ctx, id); err != nil {
		t.Fatalf("Apply() (1st) error = %v, want nil", err)
	}
	rejectView, err := svc.Reject(ctx, id, uuid.New(), "data tidak valid")
	if err != nil {
		t.Fatalf("Reject() error = %v, want nil", err)
	}
	if rejectView.DecidedAt == nil || rejectView.DecidedBy == nil || rejectView.DecisionNote == "" {
		t.Fatalf("Reject() view = %+v, want decision fields set (sanity check before re-apply)", rejectView)
	}

	view, err := svc.Apply(ctx, id)
	if err != nil {
		t.Fatalf("Apply() (re-apply) error = %v, want nil", err)
	}
	if view.Status != string(membershipapi.StatusPending) {
		t.Fatalf("Apply() (re-apply) status = %q, want pending", view.Status)
	}
	if view.DecidedAt != nil {
		t.Fatalf("Apply() (re-apply) DecidedAt = %v, want nil (cleared)", view.DecidedAt)
	}
	if view.DecidedBy != nil {
		t.Fatalf("Apply() (re-apply) DecidedBy = %v, want nil (cleared)", view.DecidedBy)
	}
	if view.DecisionNote != "" {
		t.Fatalf("Apply() (re-apply) DecisionNote = %q, want empty (cleared)", view.DecisionNote)
	}
}

func TestApply_CustomerNotFound(t *testing.T) {
	svc, _, _, _ := newTestService(t, true)
	_, err := svc.Apply(context.Background(), uuid.New())
	if !errors.Is(err, membershipapi.ErrMembershipCustomerNotFound) {
		t.Fatalf("Apply() error = %v, want ErrMembershipCustomerNotFound", err)
	}
}

// ---------------------------------------------------------------------------
// Approve
// ---------------------------------------------------------------------------

func TestApprove_HappyPath(t *testing.T) {
	svc, customers, _, notifier := newTestService(t, true)
	id := uuid.New()
	seedCustomer(customers, id, authapi.CustomerTypeRegistered, membershipapi.StatusPending)
	actor := uuid.New()

	view, err := svc.Approve(context.Background(), id, actor)
	if err != nil {
		t.Fatalf("Approve() error = %v, want nil", err)
	}
	if view.Status != string(membershipapi.StatusActive) {
		t.Fatalf("Approve() status = %q, want active", view.Status)
	}
	if notifier.calls != 1 || notifier.lastKind != notificationapi.KindMembershipApproved {
		t.Fatalf("notifier calls = %d kind = %q, want 1/membership_approved", notifier.calls, notifier.lastKind)
	}
}

func TestApprove_InvalidTransitionFromNone(t *testing.T) {
	svc, customers, _, _ := newTestService(t, true)
	id := uuid.New()
	seedCustomer(customers, id, authapi.CustomerTypeRegistered, membershipapi.StatusNone)

	_, err := svc.Approve(context.Background(), id, uuid.New())
	if !errors.Is(err, membershipapi.ErrMembershipInvalidTransition) {
		t.Fatalf("Approve() error = %v, want ErrMembershipInvalidTransition", err)
	}
}

// ---------------------------------------------------------------------------
// Reject
// ---------------------------------------------------------------------------

func TestReject_HappyPath(t *testing.T) {
	svc, customers, _, notifier := newTestService(t, true)
	id := uuid.New()
	seedCustomer(customers, id, authapi.CustomerTypeRegistered, membershipapi.StatusPending)

	view, err := svc.Reject(context.Background(), id, uuid.New(), "data tidak valid")
	if err != nil {
		t.Fatalf("Reject() error = %v, want nil", err)
	}
	if view.Status != string(membershipapi.StatusRejected) {
		t.Fatalf("Reject() status = %q, want rejected", view.Status)
	}
	if view.DecisionNote != "data tidak valid" {
		t.Fatalf("Reject() note = %q, want %q", view.DecisionNote, "data tidak valid")
	}
	if notifier.calls != 1 || notifier.lastKind != notificationapi.KindMembershipRejected {
		t.Fatalf("notifier calls = %d kind = %q, want 1/membership_rejected", notifier.calls, notifier.lastKind)
	}
}

func TestReject_ReasonRequired(t *testing.T) {
	svc, customers, _, _ := newTestService(t, true)
	id := uuid.New()
	seedCustomer(customers, id, authapi.CustomerTypeRegistered, membershipapi.StatusPending)

	_, err := svc.Reject(context.Background(), id, uuid.New(), "   ")
	if !errors.Is(err, membershipapi.ErrMembershipReasonRequired) {
		t.Fatalf("Reject() error = %v, want ErrMembershipReasonRequired", err)
	}
}

// ---------------------------------------------------------------------------
// Revoke
// ---------------------------------------------------------------------------

func TestRevoke_HappyPath(t *testing.T) {
	svc, customers, _, notifier := newTestService(t, true)
	id := uuid.New()
	seedCustomer(customers, id, authapi.CustomerTypeRegistered, membershipapi.StatusActive)

	view, err := svc.Revoke(context.Background(), id, uuid.New(), "penyalahgunaan")
	if err != nil {
		t.Fatalf("Revoke() error = %v, want nil", err)
	}
	if view.Status != string(membershipapi.StatusRevoked) {
		t.Fatalf("Revoke() status = %q, want revoked", view.Status)
	}
	if notifier.calls != 1 || notifier.lastKind != notificationapi.KindMembershipRevoked {
		t.Fatalf("notifier calls = %d kind = %q, want 1/membership_revoked", notifier.calls, notifier.lastKind)
	}
}

func TestRevoke_InvalidTransitionFromPending(t *testing.T) {
	svc, customers, _, _ := newTestService(t, true)
	id := uuid.New()
	seedCustomer(customers, id, authapi.CustomerTypeRegistered, membershipapi.StatusPending)

	_, err := svc.Revoke(context.Background(), id, uuid.New(), "alasan")
	if !errors.Is(err, membershipapi.ErrMembershipInvalidTransition) {
		t.Fatalf("Revoke() error = %v, want ErrMembershipInvalidTransition", err)
	}
}

// ---------------------------------------------------------------------------
// Reinstate (§30.2 — revoked -> active, satu-satunya jalur pemulihan)
// ---------------------------------------------------------------------------

func TestReinstate_HappyPath(t *testing.T) {
	svc, customers, _, notifier := newTestService(t, true)
	id := uuid.New()
	seedCustomer(customers, id, authapi.CustomerTypeRegistered, membershipapi.StatusRevoked)

	view, err := svc.Reinstate(context.Background(), id, uuid.New(), "tinjauan ulang selesai, disetujui kembali")
	if err != nil {
		t.Fatalf("Reinstate() error = %v, want nil", err)
	}
	if view.Status != string(membershipapi.StatusActive) {
		t.Fatalf("Reinstate() status = %q, want active", view.Status)
	}
	if view.DecisionNote != "tinjauan ulang selesai, disetujui kembali" {
		t.Fatalf("Reinstate() note = %q, want set", view.DecisionNote)
	}
	if notifier.calls != 1 || notifier.lastKind != notificationapi.KindMembershipReinstated {
		t.Fatalf("notifier calls = %d kind = %q, want 1/membership_reinstated", notifier.calls, notifier.lastKind)
	}
}

func TestReinstate_ReasonRequired(t *testing.T) {
	svc, customers, _, _ := newTestService(t, true)
	id := uuid.New()
	seedCustomer(customers, id, authapi.CustomerTypeRegistered, membershipapi.StatusRevoked)

	_, err := svc.Reinstate(context.Background(), id, uuid.New(), "   ")
	if !errors.Is(err, membershipapi.ErrMembershipReasonRequired) {
		t.Fatalf("Reinstate() error = %v, want ErrMembershipReasonRequired", err)
	}
}

func TestReinstate_InvalidTransitionFromActive(t *testing.T) {
	svc, customers, _, _ := newTestService(t, true)
	id := uuid.New()
	seedCustomer(customers, id, authapi.CustomerTypeRegistered, membershipapi.StatusActive)

	_, err := svc.Reinstate(context.Background(), id, uuid.New(), "alasan")
	if !errors.Is(err, membershipapi.ErrMembershipInvalidTransition) {
		t.Fatalf("Reinstate() error = %v, want ErrMembershipInvalidTransition", err)
	}
}

// ---------------------------------------------------------------------------
// IsActiveMember (membershipapi.Checker)
// ---------------------------------------------------------------------------

func TestIsActiveMember_True(t *testing.T) {
	svc, customers, _, _ := newTestService(t, true)
	id := uuid.New()
	seedCustomer(customers, id, authapi.CustomerTypeRegistered, membershipapi.StatusActive)

	ok, err := svc.IsActiveMember(context.Background(), id)
	if err != nil {
		t.Fatalf("IsActiveMember() error = %v, want nil", err)
	}
	if !ok {
		t.Fatal("IsActiveMember() = false, want true")
	}
}

func TestIsActiveMember_NotFoundIsFalseNotError(t *testing.T) {
	svc, _, _, _ := newTestService(t, true)
	ok, err := svc.IsActiveMember(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("IsActiveMember() error = %v, want nil", err)
	}
	if ok {
		t.Fatal("IsActiveMember() = true, want false")
	}
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

func TestList_InvalidStatusFilter(t *testing.T) {
	svc, _, _, _ := newTestService(t, true)
	_, err := svc.List(context.Background(), ListFilter{Status: "bukan-status"})
	if !errors.Is(err, membershipapi.ErrMembershipInvalidStatusFilter) {
		t.Fatalf("List() error = %v, want ErrMembershipInvalidStatusFilter", err)
	}
}

func TestList_HappyPath(t *testing.T) {
	svc, customers, _, _ := newTestService(t, true)
	customers.listRes = &authapi.ListMembershipResult{
		Items: []authapi.MembershipInfo{
			{CustomerID: uuid.New(), Name: "Budi", Status: string(membershipapi.StatusPending)},
		},
		Total:   1,
		Page:    1,
		PerPage: 20,
	}
	res, err := svc.List(context.Background(), ListFilter{Status: "pending"})
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if res.Total != 1 || len(res.Items) != 1 {
		t.Fatalf("List() = %+v, want 1 item", res)
	}
}

// ---------------------------------------------------------------------------
// Get (GET /account/membership, §30.2)
// ---------------------------------------------------------------------------

func TestGet_HappyPath(t *testing.T) {
	svc, customers, _, _ := newTestService(t, true)
	id := uuid.New()
	seedCustomer(customers, id, authapi.CustomerTypeRegistered, membershipapi.StatusActive)

	view, err := svc.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if view.Status != string(membershipapi.StatusActive) {
		t.Fatalf("Get() status = %q, want active", view.Status)
	}
}

func TestGet_CustomerNotFound(t *testing.T) {
	svc, _, _, _ := newTestService(t, true)
	_, err := svc.Get(context.Background(), uuid.New())
	if !errors.Is(err, membershipapi.ErrMembershipCustomerNotFound) {
		t.Fatalf("Get() error = %v, want ErrMembershipCustomerNotFound", err)
	}
}
