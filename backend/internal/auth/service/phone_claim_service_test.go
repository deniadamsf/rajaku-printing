package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/auth/model"
)

// ---------------------------------------------------------------------------
// fakeCustomerMergeStore — test stand-in for customerMergeAuditStore, the
// customer_merges audit write (§ migration 000019). Records every Create
// call so tests can assert exactly what got written (from/to/phone/
// orders_moved/order_ids), and is snapshotted/restored by
// fakePhoneClaimTxRunner like every other store in this transaction (§ same
// review-finding-#3 reasoning that already covers orders — an audit row
// written mid-transaction must roll back too if a LATER write in the same
// tx fails).
// ---------------------------------------------------------------------------

type fakeCustomerMergeStore struct {
	rows []model.CustomerMerge
	err  error
}

func newFakeCustomerMergeStore() *fakeCustomerMergeStore {
	return &fakeCustomerMergeStore{}
}

func (f *fakeCustomerMergeStore) Create(_ context.Context, cm *model.CustomerMerge) error {
	if f.err != nil {
		return f.err
	}
	cm.ID = uuid.New()
	f.rows = append(f.rows, *cm)
	return nil
}

type fakeCustomerMergeStoreSnapshot struct {
	rows []model.CustomerMerge
}

func (f *fakeCustomerMergeStore) snapshot() fakeCustomerMergeStoreSnapshot {
	rows := make([]model.CustomerMerge, len(f.rows))
	copy(rows, f.rows)
	return fakeCustomerMergeStoreSnapshot{rows: rows}
}

func (f *fakeCustomerMergeStore) restore(snap fakeCustomerMergeStoreSnapshot) {
	f.rows = snap.rows
}

// fakePhoneClaimTxRunner is the test stand-in for phoneClaimTxRunner — same
// snapshot/rollback contract as fakeTxRunner in google_oauth_service_test.go.
// Orders (fakeOrderCustomerMerger) is ALSO snapshotted/restored here (§
// phone-claim review finding #3) — before this fix, a failure injected AFTER
// ReassignCustomer had already mutated the fake's own ordersByCustomer map
// (mis. SetActive failing on the absorbed guest row) would leave that
// mutation in place even though users/otps correctly rolled back, silently
// hiding exactly the "most dangerous failure ordering" (order moved, then
// something after it failed) this whole feature exists to protect against.
type fakePhoneClaimTxRunner struct {
	users  *fakeGoogleUserStore
	otps   *fakeOTPStore
	orders *fakeOrderCustomerMerger
	merges *fakeCustomerMergeStore
}

func (f *fakePhoneClaimTxRunner) RunInTx(_ context.Context, fn func(tx phoneClaimTx) error) error {
	usersSnap := f.users.snapshot()
	otpsSnap := f.otps.snapshot()
	ordersSnap := f.orders.snapshot()
	mergesSnap := f.merges.snapshot()
	err := fn(phoneClaimTx{Users: f.users, OTPs: f.otps, Orders: f.orders, CustomerMerges: f.merges})
	if err != nil {
		f.users.restore(usersSnap)
		f.otps.restore(otpsSnap)
		f.orders.restore(ordersSnap)
		f.merges.restore(mergesSnap)
	}
	return err
}

// fakeOrderCustomerMerger is the test stand-in for orderapi.CustomerMerger.
// ordersByCustomer optionally seeds a tiny in-memory "orders table" keyed by
// owner so ReassignCustomer's returned count is realistic instead of a fixed
// stub — tests that care about MergedOrders seed this map before calling
// Claim. err, when set, makes every call fail (drives the rollback test).
// ordersByCustomer maps a customer owner to the SET of order IDs it owns —
// widened from a plain count so ReassignCustomer can return realistic order
// IDs (needed to assert what recordCustomerMerge writes into order_ids),
// not just a fixed stub.
type fakeOrderCustomerMerger struct {
	calls    int
	lastFrom uuid.UUID
	lastTo   uuid.UUID

	ordersByCustomer map[uuid.UUID][]uuid.UUID
	err              error
}

func newFakeOrderCustomerMerger() *fakeOrderCustomerMerger {
	return &fakeOrderCustomerMerger{ordersByCustomer: map[uuid.UUID][]uuid.UUID{}}
}

// seedOrders gives customerID n freshly-generated order IDs, returning them
// so tests can assert against exactly that set later.
func (f *fakeOrderCustomerMerger) seedOrders(customerID uuid.UUID, n int) []uuid.UUID {
	ids := make([]uuid.UUID, n)
	for i := range ids {
		ids[i] = uuid.New()
	}
	f.ordersByCustomer[customerID] = append(f.ordersByCustomer[customerID], ids...)
	return ids
}

func (f *fakeOrderCustomerMerger) ReassignCustomer(_ context.Context, fromCustomerID, toCustomerID uuid.UUID) ([]uuid.UUID, error) {
	f.calls++
	f.lastFrom, f.lastTo = fromCustomerID, toCustomerID
	if f.err != nil {
		return nil, f.err
	}
	ids := f.ordersByCustomer[fromCustomerID]
	f.ordersByCustomer[toCustomerID] = append(f.ordersByCustomer[toCustomerID], ids...)
	delete(f.ordersByCustomer, fromCustomerID)
	return ids, nil
}

// fakeOrderCustomerMergerSnapshot is fakeOrderCustomerMerger's rollback
// snapshot — see fakePhoneClaimTxRunner's doc above for why this needs to
// exist at all (§ phone-claim review finding #3). Deliberately covers ONLY
// ordersByCustomer (the fake's stand-in for the real `orders` TABLE) — calls/
// lastFrom/lastTo are call-spy diagnostics, not table state, and tests rely
// on them staying observable AFTER a rollback (mis. "the merger WAS called
// once, then everything it did got reverted" — see
// TestPhoneClaimService_Claim_PhoneOwnedByOtherGuest_MarkConsumedFailsAfterReassign_RollsBackEntirely).
// Rolling those back too would make it impossible to ever assert that
// distinction.
type fakeOrderCustomerMergerSnapshot struct {
	ordersByCustomer map[uuid.UUID][]uuid.UUID
}

func (f *fakeOrderCustomerMerger) snapshot() fakeOrderCustomerMergerSnapshot {
	byCustomer := make(map[uuid.UUID][]uuid.UUID, len(f.ordersByCustomer))
	for k, v := range f.ordersByCustomer {
		ids := make([]uuid.UUID, len(v))
		copy(ids, v)
		byCustomer[k] = ids
	}
	return fakeOrderCustomerMergerSnapshot{ordersByCustomer: byCustomer}
}

func (f *fakeOrderCustomerMerger) restore(snap fakeOrderCustomerMergerSnapshot) {
	f.ordersByCustomer = snap.ordersByCustomer
}

// phoneClaimTestHarness bundles the fakes so tests can build the service AND
// drive the OTP round-trip without repeating the wiring in every test — same
// pattern as testHarness in google_oauth_service_test.go.
type phoneClaimTestHarness struct {
	users  *fakeGoogleUserStore
	otps   *fakeOTPStore
	sender *fakeOTPSender
	orders *fakeOrderCustomerMerger
	merges *fakeCustomerMergeStore
	svc    *PhoneClaimService
}

func newPhoneClaimTestHarness(t *testing.T, otpCfg OTPConfig) *phoneClaimTestHarness {
	t.Helper()
	if otpCfg.TTL == 0 {
		otpCfg.TTL = 5 * time.Minute
	}
	if otpCfg.MaxAttempts == 0 {
		otpCfg.MaxAttempts = 5
	}
	if otpCfg.ResendCooldown == 0 {
		otpCfg.ResendCooldown = 60 * time.Second
	}
	if otpCfg.CodeLength == 0 {
		otpCfg.CodeLength = 6
	}
	if otpCfg.MaxPerPhoneHour == 0 {
		otpCfg.MaxPerPhoneHour = 1000
	}
	if otpCfg.MaxFailedPerPhoneHour == 0 {
		otpCfg.MaxFailedPerPhoneHour = 1000
	}
	users := newFakeGoogleUserStore()
	otps := newFakeOTPStore()
	sender := &fakeOTPSender{}
	orders := newFakeOrderCustomerMerger()
	merges := newFakeCustomerMergeStore()
	svc := &PhoneClaimService{
		users: users, otps: otps, otpCfg: otpCfg, otpSender: sender,
		phoneLockRunner: newFakePhoneLockTxRunner(otps),
		txRunner:        &fakePhoneClaimTxRunner{users: users, otps: otps, orders: orders, merges: merges},
	}
	return &phoneClaimTestHarness{users: users, otps: otps, sender: sender, orders: orders, merges: merges, svc: svc}
}

// requestAndReadOTP drives RequestOTP for `userID` and returns the raw code
// the fake sender captured.
func (h *phoneClaimTestHarness) requestAndReadOTP(t *testing.T, userID uuid.UUID, rawPhone string) string {
	t.Helper()
	out, err := h.svc.RequestOTP(context.Background(), PhoneClaimRequestOTPInput{UserID: userID, Phone: rawPhone})
	if err != nil {
		t.Fatalf("RequestOTP: unexpected err: %v", err)
	}
	if !out.OTPRequired {
		t.Fatalf("expected otp_required=true, got false (reason=%q)", out.Reason)
	}
	code := extractOTPCode(h.sender.lastMessage)
	if code == "" {
		t.Fatalf("could not extract OTP code from message: %q", h.sender.lastMessage)
	}
	return code
}

func newSessionUser(h *phoneClaimTestHarness, phoneStr string, verified bool) *model.User {
	registered := model.CustomerTypeRegistered
	email := "caller-" + uuid.NewString() + "@example.com"
	u := &model.User{
		ID: uuid.New(), Name: "Pemanggil", Email: &email,
		UserType: model.UserTypeCustomer, CustomerType: &registered, IsActive: true,
	}
	if phoneStr != "" {
		p := phoneStr
		u.Phone = &p
		if verified {
			now := time.Now().UTC()
			u.PhoneVerifiedAt = &now
		}
	}
	h.users.index(u)
	return u
}

// newStaffUser builds a staff caller — real staff rows carry no
// customer_type (§ model.User: customer_type only applies to
// user_type=customer). Used by the CallerIsStaff regression tests (§
// phone-claim review finding #2).
func newStaffUser(h *phoneClaimTestHarness, phoneStr string) *model.User {
	email := "staff-" + uuid.NewString() + "@example.com"
	u := &model.User{
		ID: uuid.New(), Name: "Staf", Email: &email,
		UserType: model.UserTypeStaff, IsActive: true,
	}
	if phoneStr != "" {
		p := phoneStr
		u.Phone = &p
	}
	h.users.index(u)
	return u
}

// (1) Free phone (nobody owns it) → RequestOTP reports otp_required=false,
// reason=free, and sends NOTHING.
func TestPhoneClaimService_RequestOTP_FreePhone_ReturnsFreeNoSend(t *testing.T) {
	h := newPhoneClaimTestHarness(t, OTPConfig{})
	caller := newSessionUser(h, "", false)

	out, err := h.svc.RequestOTP(context.Background(), PhoneClaimRequestOTPInput{
		UserID: caller.ID, Phone: "081234700001",
	})
	if err != nil {
		t.Fatalf("RequestOTP: unexpected err: %v", err)
	}
	if out.OTPRequired {
		t.Fatalf("expected otp_required=false for a free phone, got true")
	}
	if out.Reason != PhoneClaimReasonFree {
		t.Fatalf("expected reason=%q, got %q", PhoneClaimReasonFree, out.Reason)
	}
	if h.sender.calls != 0 {
		t.Fatalf("expected NO otp sent for a free phone, got %d calls", h.sender.calls)
	}
}

// (2) Free phone → Claim attaches it immediately, UNVERIFIED, no OTP needed.
func TestPhoneClaimService_Claim_FreePhone_AttachesUnverifiedWithoutOTP(t *testing.T) {
	h := newPhoneClaimTestHarness(t, OTPConfig{})
	caller := newSessionUser(h, "", false)

	out, err := h.svc.Claim(context.Background(), PhoneClaimInput{
		UserID: caller.ID, Phone: "081234700002",
	})
	if err != nil {
		t.Fatalf("Claim: unexpected err: %v", err)
	}
	if out.User.Phone == nil || *out.User.Phone != "6281234700002" {
		t.Fatalf("expected normalized phone attached, got %+v", out.User.Phone)
	}
	if out.User.PhoneVerifiedAt != nil {
		t.Fatalf("expected phone_verified_at nil (claimed but unproven), got %v", *out.User.PhoneVerifiedAt)
	}
	if out.MergedOrders != 0 {
		t.Fatalf("expected no merge for a free-phone claim, got %d", out.MergedOrders)
	}
	if h.orders.calls != 0 {
		t.Fatalf("expected the order merger to never be called for a free-phone claim, got %d calls", h.orders.calls)
	}
	if h.sender.calls != 0 {
		t.Fatalf("expected NO otp sent for a free-phone claim, got %d calls", h.sender.calls)
	}
	if len(h.merges.rows) != 0 {
		t.Fatalf("expected NO customer_merges audit row for a free-phone claim, got %d", len(h.merges.rows))
	}
}

// (3) SECURITY-CRITICAL: phone already belongs to a DIFFERENT customer — even
// a GUEST — must still require OTP. Claim without an OTP is rejected with
// ErrPhoneVerificationRequired (PHONE_ALREADY_IN_USE), never silently merged.
func TestPhoneClaimService_Claim_PhoneOwnedByOtherGuest_NoOTP_ReturnsErrPhoneVerificationRequired(t *testing.T) {
	h := newPhoneClaimTestHarness(t, OTPConfig{})
	guestType := model.CustomerTypeGuest
	guest := &model.User{
		ID: uuid.New(), Phone: strp("6281234700003"), Name: "Guest Punya Riwayat",
		UserType: model.UserTypeCustomer, CustomerType: &guestType, IsActive: true,
	}
	h.users.index(guest)
	caller := newSessionUser(h, "", false)

	_, err := h.svc.Claim(context.Background(), PhoneClaimInput{
		UserID: caller.ID, Phone: "081234700003",
	})
	if !errors.Is(err, authapi.ErrPhoneVerificationRequired) {
		t.Fatalf("expected ErrPhoneVerificationRequired, got %v", err)
	}
	if h.sender.calls != 0 {
		t.Fatalf("expected Claim() itself to never send an otp, got %d calls", h.sender.calls)
	}
	if h.orders.calls != 0 {
		t.Fatalf("expected the order merger to never be called without a valid OTP, got %d calls", h.orders.calls)
	}
	// Guest must be untouched.
	reloadedGuest, _ := h.users.FindByID(context.Background(), guest.ID)
	if reloadedGuest.Phone == nil || *reloadedGuest.Phone != "6281234700003" {
		t.Fatalf("expected guest's phone untouched after rejected claim, got %+v", reloadedGuest.Phone)
	}
}

// (4) HAPPY PATH — phone owned by a GUEST with N past orders + valid OTP →
// the number is released from the guest row, ALL of the guest's orders move
// to the caller (§11 satu pelanggan satu riwayat — the actual bug fix), the
// guest row is tombstoned (is_active=false), and the phone ends up attached,
// VERIFIED, on the caller.
func TestPhoneClaimService_Claim_PhoneOwnedByOtherGuest_ValidOTP_ReleasesAttachesAndMergesOrders(t *testing.T) {
	h := newPhoneClaimTestHarness(t, OTPConfig{})
	guestType := model.CustomerTypeGuest
	guest := &model.User{
		ID: uuid.New(), Phone: strp("6281234700004"), Name: "Guest Diambil Alih",
		UserType: model.UserTypeCustomer, CustomerType: &guestType, IsActive: true,
	}
	h.users.index(guest)
	const wantMerged = 3
	wantOrderIDs := h.orders.seedOrders(guest.ID, wantMerged)
	caller := newSessionUser(h, "", false)

	code := h.requestAndReadOTP(t, caller.ID, "081234700004")

	out, err := h.svc.Claim(context.Background(), PhoneClaimInput{
		UserID: caller.ID, Phone: "081234700004", OTP: code,
	})
	if err != nil {
		t.Fatalf("Claim: unexpected err: %v", err)
	}
	if out.User.ID != caller.ID {
		t.Fatalf("expected phone attached to the CALLER's row, got user_id %s want %s", out.User.ID, caller.ID)
	}
	if out.User.Phone == nil || *out.User.Phone != "6281234700004" {
		t.Fatalf("expected normalized phone attached, got %+v", out.User.Phone)
	}
	if out.User.PhoneVerifiedAt == nil {
		t.Fatal("expected phone_verified_at set after a valid OTP round-trip")
	}
	if out.MergedOrders != wantMerged {
		t.Fatalf("expected MergedOrders=%d, got %d", wantMerged, out.MergedOrders)
	}
	if h.orders.calls != 1 {
		t.Fatalf("expected the order merger called exactly once, got %d calls", h.orders.calls)
	}
	if h.orders.lastFrom != guest.ID || h.orders.lastTo != caller.ID {
		t.Fatalf("expected merger called with (from=%s, to=%s), got (from=%s, to=%s)",
			guest.ID, caller.ID, h.orders.lastFrom, h.orders.lastTo)
	}
	reloadedGuest, err := h.users.FindByID(context.Background(), guest.ID)
	if err != nil {
		t.Fatalf("reload guest: %v", err)
	}
	if reloadedGuest.Phone != nil {
		t.Fatalf("expected guest's phone released (nil) after being absorbed, got %q", *reloadedGuest.Phone)
	}
	if reloadedGuest.IsActive {
		t.Fatal("expected the absorbed guest row to be tombstoned (is_active=false)")
	}

	// The durable customer_merges audit row (§ migration 000019) — this is
	// the whole point of this task: even after logs rotate, this row is the
	// only place that answers "this order used to belong to whom".
	if len(h.merges.rows) != 1 {
		t.Fatalf("expected exactly 1 customer_merges audit row written, got %d", len(h.merges.rows))
	}
	row := h.merges.rows[0]
	if row.FromUserID != guest.ID {
		t.Fatalf("expected audit row from_user_id=%s, got %s", guest.ID, row.FromUserID)
	}
	if row.ToUserID != caller.ID {
		t.Fatalf("expected audit row to_user_id=%s, got %s", caller.ID, row.ToUserID)
	}
	if row.Phone != "6281234700004" {
		t.Fatalf("expected audit row phone=%q, got %q", "6281234700004", row.Phone)
	}
	if row.OrdersMoved != wantMerged {
		t.Fatalf("expected audit row orders_moved=%d, got %d", wantMerged, row.OrdersMoved)
	}
	if row.Source != model.CustomerMergeSourcePhoneClaim {
		t.Fatalf("expected audit row source=%q, got %q", model.CustomerMergeSourcePhoneClaim, row.Source)
	}
	if len(row.OrderIDs) != wantMerged {
		t.Fatalf("expected audit row order_ids to have %d entries, got %d (%v)", wantMerged, len(row.OrderIDs), row.OrderIDs)
	}
	gotOrderIDs := map[string]bool{}
	for _, id := range row.OrderIDs {
		gotOrderIDs[id] = true
	}
	for _, wantID := range wantOrderIDs {
		if !gotOrderIDs[wantID.String()] {
			t.Fatalf("expected audit row order_ids to contain %s, got %v", wantID, row.OrderIDs)
		}
	}
}

// (4b) FAILURE PATH — the order merger fails mid-absorption → the WHOLE
// transaction rolls back: the phone must NOT move (guest keeps it, caller
// stays phone-less) and the OTP must NOT be marked consumed.
func TestPhoneClaimService_Claim_PhoneOwnedByOtherGuest_MergeFails_RollsBackEntirely(t *testing.T) {
	h := newPhoneClaimTestHarness(t, OTPConfig{})
	guestType := model.CustomerTypeGuest
	guest := &model.User{
		ID: uuid.New(), Phone: strp("6281234700011"), Name: "Guest Gagal Diserap",
		UserType: model.UserTypeCustomer, CustomerType: &guestType, IsActive: true,
	}
	h.users.index(guest)
	caller := newSessionUser(h, "", false)

	code := h.requestAndReadOTP(t, caller.ID, "081234700011")

	wantErr := errors.New("boom: db down mid-merge")
	h.orders.err = wantErr

	_, err := h.svc.Claim(context.Background(), PhoneClaimInput{
		UserID: caller.ID, Phone: "081234700011", OTP: code,
	})
	if err == nil {
		t.Fatal("expected Claim to fail when the order merger fails")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected the merger's error to be wrapped (%%w) into the returned error, got %v", err)
	}

	reloadedGuest, ferr := h.users.FindByID(context.Background(), guest.ID)
	if ferr != nil {
		t.Fatalf("reload guest: %v", ferr)
	}
	if reloadedGuest.Phone == nil || *reloadedGuest.Phone != "6281234700011" {
		t.Fatalf("expected guest's phone UNTOUCHED after a rolled-back merge, got %+v", reloadedGuest.Phone)
	}
	if !reloadedGuest.IsActive {
		t.Fatal("expected guest row to remain active — tombstoning must roll back too")
	}
	reloadedCaller, ferr := h.users.FindByID(context.Background(), caller.ID)
	if ferr != nil {
		t.Fatalf("reload caller: %v", ferr)
	}
	if reloadedCaller.Phone != nil {
		t.Fatalf("expected caller to remain phone-less after a rolled-back merge, got %+v", reloadedCaller.Phone)
	}
	// OTP must not have been consumed — a retry with the same code should
	// still find it active. verifyPhoneOTP marks failed attempts on mismatch,
	// so instead assert directly against the OTP store: FindActiveByUser must
	// still return the same, still-pending challenge.
	pv, ferr := h.otps.FindActiveByUser(context.Background(), caller.ID, "6281234700011")
	if ferr != nil {
		t.Fatalf("expected the OTP challenge to still be active (not consumed) after rollback: %v", ferr)
	}
	if pv.ConsumedAt != nil {
		t.Fatal("expected the OTP to NOT be marked consumed after a rolled-back merge")
	}
	if len(h.merges.rows) != 0 {
		t.Fatalf("expected NO customer_merges audit row when the reassignment itself failed, got %d", len(h.merges.rows))
	}
}

// (5) Phone owned by an already-REGISTERED customer — even with a valid OTP,
// this is a dead end (OTP proves WA ownership, not that account's password).
func TestPhoneClaimService_Claim_PhoneOwnedByRegisteredCustomer_ValidOTP_ReturnsErrPhoneAlreadyUsed(t *testing.T) {
	h := newPhoneClaimTestHarness(t, OTPConfig{})
	other := newSessionUser(h, "6281234700005", true)
	caller := newSessionUser(h, "", false)

	code := h.requestAndReadOTP(t, caller.ID, "081234700005")

	_, err := h.svc.Claim(context.Background(), PhoneClaimInput{
		UserID: caller.ID, Phone: "081234700005", OTP: code,
	})
	if !errors.Is(err, authapi.ErrPhoneAlreadyUsed) {
		t.Fatalf("expected ErrPhoneAlreadyUsed, got %v", err)
	}
	if h.orders.calls != 0 {
		t.Fatalf("expected the order merger to never be called against a staff/registered owner, got %d calls", h.orders.calls)
	}
	reloadedOther, _ := h.users.FindByID(context.Background(), other.ID)
	if reloadedOther.Phone == nil || *reloadedOther.Phone != "6281234700005" {
		t.Fatalf("expected other registered customer's phone untouched, got %+v", reloadedOther.Phone)
	}
	if len(h.merges.rows) != 0 {
		t.Fatalf("expected NO customer_merges audit row against an already-registered owner, got %d", len(h.merges.rows))
	}
}

// (6) SELF-VERIFY, not a conflict: the caller's OWN number, already attached
// but unverified — RequestOTP must issue an OTP (reason=self_verify), NEVER
// framed as "someone else has it".
func TestPhoneClaimService_RequestOTP_SelfUnverified_ReturnsReasonSelfVerify(t *testing.T) {
	h := newPhoneClaimTestHarness(t, OTPConfig{})
	caller := newSessionUser(h, "6281234700006", false)

	out, err := h.svc.RequestOTP(context.Background(), PhoneClaimRequestOTPInput{
		UserID: caller.ID, Phone: "081234700006",
	})
	if err != nil {
		t.Fatalf("RequestOTP: unexpected err: %v", err)
	}
	if !out.OTPRequired {
		t.Fatal("expected otp_required=true for the caller's own unverified number")
	}
	if out.Reason != PhoneClaimReasonSelfVerify {
		t.Fatalf("expected reason=%q, got %q", PhoneClaimReasonSelfVerify, out.Reason)
	}
	if h.sender.calls != 1 {
		t.Fatalf("expected exactly 1 otp send, got %d", h.sender.calls)
	}
}

// (7) Self-verify, otp omitted at Claim() → ErrPhoneSelfVerificationRequired
// — a DIFFERENT sentinel/code from ErrPhoneVerificationRequired, so the
// frontend never tells the user "someone else has your own number".
func TestPhoneClaimService_Claim_SelfUnverified_NoOTP_ReturnsErrPhoneSelfVerificationRequired(t *testing.T) {
	h := newPhoneClaimTestHarness(t, OTPConfig{})
	caller := newSessionUser(h, "6281234700007", false)

	_, err := h.svc.Claim(context.Background(), PhoneClaimInput{
		UserID: caller.ID, Phone: "081234700007",
	})
	if !errors.Is(err, authapi.ErrPhoneSelfVerificationRequired) {
		t.Fatalf("expected ErrPhoneSelfVerificationRequired, got %v", err)
	}
	if errors.Is(err, authapi.ErrPhoneVerificationRequired) {
		t.Fatal("self-verify must NOT also satisfy errors.Is(ErrPhoneVerificationRequired) — distinct sentinels")
	}
}

// (8) Self-verify with a valid OTP → phone_verified_at gets stamped on the
// SAME row (no merge, no other row touched).
func TestPhoneClaimService_Claim_SelfUnverified_ValidOTP_MarksVerified(t *testing.T) {
	h := newPhoneClaimTestHarness(t, OTPConfig{})
	caller := newSessionUser(h, "6281234700008", false)

	code := h.requestAndReadOTP(t, caller.ID, "081234700008")

	out, err := h.svc.Claim(context.Background(), PhoneClaimInput{
		UserID: caller.ID, Phone: "081234700008", OTP: code,
	})
	if err != nil {
		t.Fatalf("Claim: unexpected err: %v", err)
	}
	if out.User.ID != caller.ID {
		t.Fatalf("expected same user id, got %s want %s", out.User.ID, caller.ID)
	}
	if out.User.Phone == nil || *out.User.Phone != "6281234700008" {
		t.Fatalf("expected phone unchanged, got %+v", out.User.Phone)
	}
	if out.User.PhoneVerifiedAt == nil {
		t.Fatal("expected phone_verified_at set after a valid self-verify OTP round-trip")
	}
	if out.MergedOrders != 0 {
		t.Fatalf("expected no merge on a self-verify claim, got %d", out.MergedOrders)
	}
	if h.orders.calls != 0 {
		t.Fatalf("expected the order merger to never be called on a self-verify claim, got %d calls", h.orders.calls)
	}
	if len(h.merges.rows) != 0 {
		t.Fatalf("expected NO customer_merges audit row on a self-verify claim, got %d", len(h.merges.rows))
	}
}

// (9) Self, ALREADY verified → RequestOTP is a no-op: reason=self_verified,
// otp_required=false, and — critically — NO notification job/OTP send at all
// (must not burn WhatsApp quota re-verifying something already proven).
func TestPhoneClaimService_RequestOTP_SelfAlreadyVerified_NoSend(t *testing.T) {
	h := newPhoneClaimTestHarness(t, OTPConfig{})
	caller := newSessionUser(h, "6281234700009", true)

	out, err := h.svc.RequestOTP(context.Background(), PhoneClaimRequestOTPInput{
		UserID: caller.ID, Phone: "081234700009",
	})
	if err != nil {
		t.Fatalf("RequestOTP: unexpected err: %v", err)
	}
	if out.OTPRequired {
		t.Fatal("expected otp_required=false for an already-verified own number")
	}
	if out.Reason != PhoneClaimReasonSelfVerified {
		t.Fatalf("expected reason=%q, got %q", PhoneClaimReasonSelfVerified, out.Reason)
	}
	if h.sender.calls != 0 {
		t.Fatalf("expected NO otp sent for an already-verified own number, got %d calls", h.sender.calls)
	}
}

// (10) Self, already verified → Claim() is idempotent success even with no
// OTP supplied — nothing left to prove.
func TestPhoneClaimService_Claim_SelfAlreadyVerified_IdempotentSuccess(t *testing.T) {
	h := newPhoneClaimTestHarness(t, OTPConfig{})
	caller := newSessionUser(h, "6281234700010", true)
	originalVerifiedAt := *caller.PhoneVerifiedAt

	out, err := h.svc.Claim(context.Background(), PhoneClaimInput{
		UserID: caller.ID, Phone: "081234700010",
	})
	if err != nil {
		t.Fatalf("Claim: unexpected err: %v", err)
	}
	if out.User.PhoneVerifiedAt == nil || !out.User.PhoneVerifiedAt.Equal(originalVerifiedAt) {
		t.Fatalf("expected phone_verified_at unchanged (idempotent), got %+v want %v", out.User.PhoneVerifiedAt, originalVerifiedAt)
	}
	if out.MergedOrders != 0 {
		t.Fatalf("expected no merge on an idempotent self-claim, got %d", out.MergedOrders)
	}
	if h.sender.calls != 0 {
		t.Fatalf("expected NO otp sent for an idempotent self-claim, got %d calls", h.sender.calls)
	}
}

// (11) Phone owned by STAFF — even with a valid OTP, this is a dead end,
// same reasoning as an already-registered customer (previously only that
// variant was covered — § phone-claim review finding #3).
func TestPhoneClaimService_Claim_PhoneOwnedByStaff_ValidOTP_ReturnsErrPhoneAlreadyUsed(t *testing.T) {
	h := newPhoneClaimTestHarness(t, OTPConfig{})
	staffOwner := newStaffUser(h, "6281234700022")
	caller := newSessionUser(h, "", false)

	code := h.requestAndReadOTP(t, caller.ID, "081234700022")

	_, err := h.svc.Claim(context.Background(), PhoneClaimInput{
		UserID: caller.ID, Phone: "081234700022", OTP: code,
	})
	if !errors.Is(err, authapi.ErrPhoneAlreadyUsed) {
		t.Fatalf("expected ErrPhoneAlreadyUsed for a staff-owned number, got %v", err)
	}
	if h.orders.calls != 0 {
		t.Fatalf("expected the order merger to never be called against a staff owner, got %d calls", h.orders.calls)
	}
	reloadedStaff, ferr := h.users.FindByID(context.Background(), staffOwner.ID)
	if ferr != nil {
		t.Fatalf("reload staff owner: %v", ferr)
	}
	if reloadedStaff.Phone == nil || *reloadedStaff.Phone != "6281234700022" {
		t.Fatalf("expected staff owner's phone untouched, got %+v", reloadedStaff.Phone)
	}
	if len(h.merges.rows) != 0 {
		t.Fatalf("expected NO customer_merges audit row against a staff owner, got %d", len(h.merges.rows))
	}
}

// (12) SECURITY-CRITICAL, race window: the number changes hands to a
// DIFFERENT owner between the pre-transaction read (`existing`, captured
// before Claim opens the transaction) and the in-transaction, row-locked
// re-check. The stale OTP proof must not be honored against the new owner —
// ErrPhoneVerificationRequired, no merge. Exercises claimOther directly
// (same package) with a deliberately stale `existing` snapshot, since a
// single-threaded fake store can't reproduce the race by timing alone.
func TestPhoneClaimService_Claim_ClaimOther_OwnerChangedDuringOTPWindow_ReturnsErrPhoneVerificationRequired(t *testing.T) {
	h := newPhoneClaimTestHarness(t, OTPConfig{})
	guestType := model.CustomerTypeGuest
	originalGuest := &model.User{
		ID: uuid.New(), Phone: strp("6281234700020"), Name: "Guest Awal",
		UserType: model.UserTypeCustomer, CustomerType: &guestType, IsActive: true,
	}
	h.users.index(originalGuest)
	caller := newSessionUser(h, "", false)
	code := h.requestAndReadOTP(t, caller.ID, "081234700020")

	// Snapshot of what Claim()'s own pre-tx read would have seen a moment
	// earlier — the OTP was verified against ownership implied by THIS state.
	existingSnapshot := *originalGuest

	// Simulate a DIFFERENT owner claiming the number in the race window
	// since then (mis. another concurrent absorption/registration landing
	// first) — re-indexing the same phone number to a brand-new row.
	otherGuestType := model.CustomerTypeGuest
	newOwner := &model.User{
		ID: uuid.New(), Phone: strp("6281234700020"), Name: "Pemilik Baru",
		UserType: model.UserTypeCustomer, CustomerType: &otherGuestType, IsActive: true,
	}
	h.users.index(newOwner)

	in := PhoneClaimInput{UserID: caller.ID, Phone: "081234700020", OTP: code}
	_, err := h.svc.claimOther(context.Background(), in, "6281234700020", &existingSnapshot)
	if !errors.Is(err, authapi.ErrPhoneVerificationRequired) {
		t.Fatalf("expected ErrPhoneVerificationRequired when the owner changed mid-flight, got %v", err)
	}
	if h.orders.calls != 0 {
		t.Fatalf("expected the order merger to never be called, got %d calls", h.orders.calls)
	}
	if len(h.merges.rows) != 0 {
		t.Fatalf("expected NO customer_merges audit row when the owner changed mid-flight, got %d", len(h.merges.rows))
	}
}

// (13) The owner vanishes (phone released) between the pre-transaction read
// and the in-transaction re-check — `FindByPhoneForUpdate` returns
// ErrNotFound. Claim proceeds treating the number as now-free-and-proven: it
// succeeds, MergedOrders==0 (nothing concrete left to absorb), merger never
// called.
func TestPhoneClaimService_Claim_ClaimOther_OwnerVanishedDuringOTPWindow_SucceedsWithoutMerge(t *testing.T) {
	h := newPhoneClaimTestHarness(t, OTPConfig{})
	guestType := model.CustomerTypeGuest
	guest := &model.User{
		ID: uuid.New(), Phone: strp("6281234700021"), Name: "Guest Hilang",
		UserType: model.UserTypeCustomer, CustomerType: &guestType, IsActive: true,
	}
	h.users.index(guest)
	caller := newSessionUser(h, "", false)
	code := h.requestAndReadOTP(t, caller.ID, "081234700021")

	existingSnapshot := *guest

	// Simulate the owner's phone getting released/vanishing in the race
	// window since the pre-tx read.
	if err := h.users.SetPhone(context.Background(), guest.ID, nil, nil); err != nil {
		t.Fatalf("simulate vanished owner: %v", err)
	}

	in := PhoneClaimInput{UserID: caller.ID, Phone: "081234700021", OTP: code}
	out, err := h.svc.claimOther(context.Background(), in, "6281234700021", &existingSnapshot)
	if err != nil {
		t.Fatalf("claimOther: unexpected err: %v", err)
	}
	if out.MergedOrders != 0 {
		t.Fatalf("expected MergedOrders=0 when the previous owner vanished, got %d", out.MergedOrders)
	}
	if h.orders.calls != 0 {
		t.Fatalf("expected the order merger to never be called, got %d calls", h.orders.calls)
	}
	if out.User.Phone == nil || *out.User.Phone != "6281234700021" {
		t.Fatalf("expected the now-free number attached to the caller, got %+v", out.User.Phone)
	}
	if len(h.merges.rows) != 0 {
		t.Fatalf("expected NO customer_merges audit row when there was no confirmed guest left to absorb, got %d", len(h.merges.rows))
	}
}

// (14) REGRESSION TEST for review finding #1: the pre-transaction owner row
// changes TYPE in place (guest → registered, SAME id — mirrors
// UserRepository.UpgradeGuestToRegistered running on this exact row via a
// concurrent Google OAuth completion) between Claim()'s pre-tx read and the
// in-transaction re-check. Before the fix, the in-tx re-check only compared
// IDs (`fresh.ID != previousOwnerID`), so a same-ID-but-now-registered row
// sailed through and got absorbed+tombstoned. It must now be rejected with
// ErrPhoneAlreadyUsed, with NOTHING moved or tombstoned.
func TestPhoneClaimService_Claim_ClaimOther_OwnerUpgradedToRegisteredDuringOTPWindow_ReturnsErrPhoneAlreadyUsed(t *testing.T) {
	h := newPhoneClaimTestHarness(t, OTPConfig{})
	guestType := model.CustomerTypeGuest
	guest := &model.User{
		ID: uuid.New(), Phone: strp("6281234700023"), Name: "Guest Naik Level",
		UserType: model.UserTypeCustomer, CustomerType: &guestType, IsActive: true,
	}
	h.users.index(guest)
	const wantUntouchedOrders = 2
	h.orders.seedOrders(guest.ID, wantUntouchedOrders)
	caller := newSessionUser(h, "", false)
	code := h.requestAndReadOTP(t, caller.ID, "081234700023")

	// Snapshot of what Claim()'s own pre-tx read (and its fail-fast check)
	// would have seen a moment earlier — a GUEST, so the fail-fast check
	// would have let this through to the transaction.
	existingSnapshot := *guest

	// Simulate a concurrent Google OAuth completion running
	// UpgradeGuestToRegistered on the SAME row id, IN PLACE, in the race
	// window since the pre-tx read — exactly the race review finding #1
	// identified.
	registeredType := model.CustomerTypeRegistered
	guest.CustomerType = &registeredType

	in := PhoneClaimInput{UserID: caller.ID, Phone: "081234700023", OTP: code}
	_, err := h.svc.claimOther(context.Background(), in, "6281234700023", &existingSnapshot)
	if !errors.Is(err, authapi.ErrPhoneAlreadyUsed) {
		t.Fatalf("expected ErrPhoneAlreadyUsed for an owner upgraded to registered mid-flight, got %v", err)
	}
	if h.orders.calls != 0 {
		t.Fatalf("expected the order merger to never be called, got %d calls", h.orders.calls)
	}
	reloadedOwner, ferr := h.users.FindByID(context.Background(), guest.ID)
	if ferr != nil {
		t.Fatalf("reload owner: %v", ferr)
	}
	if !reloadedOwner.IsActive {
		t.Fatal("expected the now-registered owner to remain active (NOT tombstoned)")
	}
	if reloadedOwner.Phone == nil || *reloadedOwner.Phone != "6281234700023" {
		t.Fatalf("expected the owner's phone to remain attached (not released), got %+v", reloadedOwner.Phone)
	}
	if n := len(h.orders.ordersByCustomer[guest.ID]); n != wantUntouchedOrders {
		t.Fatalf("expected the owner's order count to stay untouched, got %d (want %d)", n, wantUntouchedOrders)
	}
	if len(h.merges.rows) != 0 {
		t.Fatalf("expected NO customer_merges audit row for a rejected (owner upgraded mid-flight) absorption, got %d", len(h.merges.rows))
	}
}

// (15) SECURITY, review finding #2: a STAFF caller must never be able to
// absorb a customer's guest identity/order history into their own staff
// row, even with a valid OTP proving WA ownership of the number.
func TestPhoneClaimService_Claim_StaffCallerAttemptsToAbsorbGuest_ReturnsErrPhoneAlreadyUsed(t *testing.T) {
	h := newPhoneClaimTestHarness(t, OTPConfig{})
	guestType := model.CustomerTypeGuest
	guest := &model.User{
		ID: uuid.New(), Phone: strp("6281234700024"), Name: "Guest Disasar Staf",
		UserType: model.UserTypeCustomer, CustomerType: &guestType, IsActive: true,
	}
	h.users.index(guest)
	const wantUntouchedOrders = 4
	h.orders.seedOrders(guest.ID, wantUntouchedOrders)
	staff := newStaffUser(h, "")

	code := h.requestAndReadOTP(t, staff.ID, "081234700024")

	_, err := h.svc.Claim(context.Background(), PhoneClaimInput{
		UserID: staff.ID, Phone: "081234700024", OTP: code, CallerIsStaff: true,
	})
	if !errors.Is(err, authapi.ErrPhoneAlreadyUsed) {
		t.Fatalf("expected ErrPhoneAlreadyUsed when a staff caller tries to absorb a guest, got %v", err)
	}
	if h.orders.calls != 0 {
		t.Fatalf("expected the order merger to never be called for a staff absorption attempt, got %d calls", h.orders.calls)
	}
	reloadedGuest, ferr := h.users.FindByID(context.Background(), guest.ID)
	if ferr != nil {
		t.Fatalf("reload guest: %v", ferr)
	}
	if !reloadedGuest.IsActive {
		t.Fatal("expected the guest to remain untouched (not tombstoned) after a rejected staff absorption")
	}
	if reloadedGuest.Phone == nil || *reloadedGuest.Phone != "6281234700024" {
		t.Fatalf("expected guest's phone untouched, got %+v", reloadedGuest.Phone)
	}
	if n := len(h.orders.ordersByCustomer[guest.ID]); n != wantUntouchedOrders {
		t.Fatalf("expected the guest's order count to stay untouched, got %d (want %d)", n, wantUntouchedOrders)
	}
	if len(h.merges.rows) != 0 {
		t.Fatalf("expected NO customer_merges audit row for a rejected staff absorption attempt, got %d", len(h.merges.rows))
	}
}

// (16) A staff caller adding/changing THEIR OWN number — a genuinely free
// one — must still succeed. CallerIsStaff only gates the guest-absorption
// branch, never a staff caller's self-service use of this endpoint (§
// phone-claim review finding #2, "PENTING, jangan berlebihan").
func TestPhoneClaimService_Claim_StaffCallerClaimsFreeNumber_Succeeds(t *testing.T) {
	h := newPhoneClaimTestHarness(t, OTPConfig{})
	staff := newStaffUser(h, "")

	out, err := h.svc.Claim(context.Background(), PhoneClaimInput{
		UserID: staff.ID, Phone: "081234700025", CallerIsStaff: true,
	})
	if err != nil {
		t.Fatalf("Claim: unexpected err for a staff caller claiming a free number: %v", err)
	}
	if out.User.Phone == nil || *out.User.Phone != "6281234700025" {
		t.Fatalf("expected the free number attached to the staff caller, got %+v", out.User.Phone)
	}
	if out.MergedOrders != 0 {
		t.Fatalf("expected no merge for a free-number claim, got %d", out.MergedOrders)
	}
}

// (17) The MOST DANGEROUS failure ordering (§ phone-claim review finding #3):
// a write fails AFTER the order history has already moved inside the (still
// uncommitted) transaction — MarkConsumed is the LAST write in claimOther,
// running after SetPhone(release) → ReassignCustomer → SetActive(tombstone)
// → SetPhone(caller). The whole transaction, including the order
// reassignment, must roll back — nothing left half-migrated.
func TestPhoneClaimService_Claim_PhoneOwnedByOtherGuest_MarkConsumedFailsAfterReassign_RollsBackEntirely(t *testing.T) {
	h := newPhoneClaimTestHarness(t, OTPConfig{})
	guestType := model.CustomerTypeGuest
	guest := &model.User{
		ID: uuid.New(), Phone: strp("6281234700026"), Name: "Guest Gagal Setelah Pindah",
		UserType: model.UserTypeCustomer, CustomerType: &guestType, IsActive: true,
	}
	h.users.index(guest)
	const wantMerged = 5
	h.orders.seedOrders(guest.ID, wantMerged)
	caller := newSessionUser(h, "", false)

	code := h.requestAndReadOTP(t, caller.ID, "081234700026")
	h.otps.markConsumedErr = errors.New("boom: db down right after reassign")

	_, err := h.svc.Claim(context.Background(), PhoneClaimInput{
		UserID: caller.ID, Phone: "081234700026", OTP: code,
	})
	if err == nil {
		t.Fatal("expected Claim to fail when MarkConsumed fails after the order reassignment already ran")
	}
	// ReassignCustomer DID get called (it runs before MarkConsumed) — the
	// point of this test is that its effect must have been rolled back, not
	// left in place.
	if h.orders.calls != 1 {
		t.Fatalf("expected the order merger to have been called once before the failure, got %d calls", h.orders.calls)
	}
	reloadedGuest, ferr := h.users.FindByID(context.Background(), guest.ID)
	if ferr != nil {
		t.Fatalf("reload guest: %v", ferr)
	}
	if reloadedGuest.Phone == nil || *reloadedGuest.Phone != "6281234700026" {
		t.Fatalf("expected guest's phone to be restored (rolled back), got %+v", reloadedGuest.Phone)
	}
	if !reloadedGuest.IsActive {
		t.Fatal("expected the guest row's tombstone to be rolled back too")
	}
	if n := len(h.orders.ordersByCustomer[guest.ID]); n != wantMerged {
		t.Fatalf("expected the fake order table's ownership to be rolled back to the guest, got %d (want %d)", n, wantMerged)
	}
	// The customer_merges audit row DID get written earlier in this same
	// transaction (right after the reassignment succeeded, before the later
	// MarkConsumed failure) — it must have rolled back too, not been left
	// behind as an orphaned audit row describing a merge that never actually
	// committed.
	if len(h.merges.rows) != 0 {
		t.Fatalf("expected the customer_merges audit row to be rolled back too, got %d rows left", len(h.merges.rows))
	}
	reloadedCaller, ferr := h.users.FindByID(context.Background(), caller.ID)
	if ferr != nil {
		t.Fatalf("reload caller: %v", ferr)
	}
	if reloadedCaller.Phone != nil {
		t.Fatalf("expected caller to remain phone-less after a rolled-back claim, got %+v", reloadedCaller.Phone)
	}
}

// (18) A guest with ZERO past orders is still absorbed — and the
// customer_merges audit row is STILL written, with orders_moved=0 and an
// empty order_ids. The fact that a guest identity was absorbed is itself
// worth recording, whether or not that guest happened to have ordered
// anything yet (§ task spec: "Tulis barisnya walau orders_moved = 0").
func TestPhoneClaimService_Claim_PhoneOwnedByOtherGuest_NoOrders_StillWritesAuditRowWithZeroOrders(t *testing.T) {
	h := newPhoneClaimTestHarness(t, OTPConfig{})
	guestType := model.CustomerTypeGuest
	guest := &model.User{
		ID: uuid.New(), Phone: strp("6281234700027"), Name: "Guest Tanpa Order",
		UserType: model.UserTypeCustomer, CustomerType: &guestType, IsActive: true,
	}
	h.users.index(guest) // deliberately NOT seeded with any orders
	caller := newSessionUser(h, "", false)

	code := h.requestAndReadOTP(t, caller.ID, "081234700027")

	out, err := h.svc.Claim(context.Background(), PhoneClaimInput{
		UserID: caller.ID, Phone: "081234700027", OTP: code,
	})
	if err != nil {
		t.Fatalf("Claim: unexpected err: %v", err)
	}
	if out.MergedOrders != 0 {
		t.Fatalf("expected MergedOrders=0 for a guest with no past orders, got %d", out.MergedOrders)
	}
	// The absorption itself still happened — guest phone released + tombstoned.
	reloadedGuest, ferr := h.users.FindByID(context.Background(), guest.ID)
	if ferr != nil {
		t.Fatalf("reload guest: %v", ferr)
	}
	if reloadedGuest.IsActive {
		t.Fatal("expected the absorbed guest row to be tombstoned even with zero orders")
	}

	if len(h.merges.rows) != 1 {
		t.Fatalf("expected exactly 1 customer_merges audit row even for a zero-order absorption, got %d", len(h.merges.rows))
	}
	row := h.merges.rows[0]
	if row.FromUserID != guest.ID || row.ToUserID != caller.ID {
		t.Fatalf("expected audit row (from=%s, to=%s), got (from=%s, to=%s)", guest.ID, caller.ID, row.FromUserID, row.ToUserID)
	}
	if row.OrdersMoved != 0 {
		t.Fatalf("expected audit row orders_moved=0, got %d", row.OrdersMoved)
	}
	if len(row.OrderIDs) != 0 {
		t.Fatalf("expected audit row order_ids empty, got %v", row.OrderIDs)
	}
	if row.Phone != "6281234700027" {
		t.Fatalf("expected audit row phone=%q, got %q", "6281234700027", row.Phone)
	}
	if row.Source != model.CustomerMergeSourcePhoneClaim {
		t.Fatalf("expected audit row source=%q, got %q", model.CustomerMergeSourcePhoneClaim, row.Source)
	}
}

// (19) FAILURE PATH — the customer_merges audit write itself fails →
// the WHOLE transaction rolls back: the order reassignment, the guest
// tombstone/phone release, AND the caller's phone attach/OTP consumption
// must all be undone. An absorption that can't be durably audited must not
// be allowed to happen silently.
func TestPhoneClaimService_Claim_PhoneOwnedByOtherGuest_AuditWriteFails_RollsBackEntirely(t *testing.T) {
	h := newPhoneClaimTestHarness(t, OTPConfig{})
	guestType := model.CustomerTypeGuest
	guest := &model.User{
		ID: uuid.New(), Phone: strp("6281234700028"), Name: "Guest Audit Gagal",
		UserType: model.UserTypeCustomer, CustomerType: &guestType, IsActive: true,
	}
	h.users.index(guest)
	const wantOrders = 2
	h.orders.seedOrders(guest.ID, wantOrders)
	caller := newSessionUser(h, "", false)

	code := h.requestAndReadOTP(t, caller.ID, "081234700028")

	wantErr := errors.New("boom: db down writing the audit row")
	h.merges.err = wantErr

	_, err := h.svc.Claim(context.Background(), PhoneClaimInput{
		UserID: caller.ID, Phone: "081234700028", OTP: code,
	})
	if err == nil {
		t.Fatal("expected Claim to fail when the customer_merges audit write fails")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected the audit store's error to be wrapped (%%w), got %v", err)
	}

	// Order reassignment DID run (it happens before the audit write) — its
	// effect must be rolled back, not left in place.
	if n := len(h.orders.ordersByCustomer[guest.ID]); n != wantOrders {
		t.Fatalf("expected the fake order table's ownership to be rolled back to the guest, got %d (want %d)", n, wantOrders)
	}
	reloadedGuest, ferr := h.users.FindByID(context.Background(), guest.ID)
	if ferr != nil {
		t.Fatalf("reload guest: %v", ferr)
	}
	if reloadedGuest.Phone == nil || *reloadedGuest.Phone != "6281234700028" {
		t.Fatalf("expected guest's phone to be restored (rolled back), got %+v", reloadedGuest.Phone)
	}
	if !reloadedGuest.IsActive {
		t.Fatal("expected the guest row's tombstone to be rolled back too")
	}
	reloadedCaller, ferr := h.users.FindByID(context.Background(), caller.ID)
	if ferr != nil {
		t.Fatalf("reload caller: %v", ferr)
	}
	if reloadedCaller.Phone != nil {
		t.Fatalf("expected caller to remain phone-less after a rolled-back claim, got %+v", reloadedCaller.Phone)
	}
	pv, ferr := h.otps.FindActiveByUser(context.Background(), caller.ID, "6281234700028")
	if ferr != nil {
		t.Fatalf("expected the OTP challenge to still be active (not consumed) after rollback: %v", ferr)
	}
	if pv.ConsumedAt != nil {
		t.Fatal("expected the OTP to NOT be marked consumed after a rolled-back audit write")
	}
	if len(h.merges.rows) != 0 {
		t.Fatalf("expected no customer_merges rows left after a rolled-back audit write, got %d", len(h.merges.rows))
	}
}
