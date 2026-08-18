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

// fakePhoneClaimTxRunner is the test stand-in for phoneClaimTxRunner — same
// snapshot/rollback contract as fakeTxRunner in google_oauth_service_test.go,
// just without a Codes store (this flow has no OAuth handoff code).
type fakePhoneClaimTxRunner struct {
	users *fakeGoogleUserStore
	otps  *fakeOTPStore
}

func (f *fakePhoneClaimTxRunner) RunInTx(_ context.Context, fn func(tx phoneClaimTx) error) error {
	usersSnap := f.users.snapshot()
	otpsSnap := f.otps.snapshot()
	err := fn(phoneClaimTx{Users: f.users, OTPs: f.otps})
	if err != nil {
		f.users.restore(usersSnap)
		f.otps.restore(otpsSnap)
	}
	return err
}

// phoneClaimTestHarness bundles the fakes so tests can build the service AND
// drive the OTP round-trip without repeating the wiring in every test — same
// pattern as testHarness in google_oauth_service_test.go.
type phoneClaimTestHarness struct {
	users  *fakeGoogleUserStore
	otps   *fakeOTPStore
	sender *fakeOTPSender
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
	svc := &PhoneClaimService{
		users: users, otps: otps, otpCfg: otpCfg, otpSender: sender,
		phoneLockRunner: newFakePhoneLockTxRunner(otps),
		txRunner:        &fakePhoneClaimTxRunner{users: users, otps: otps},
	}
	return &phoneClaimTestHarness{users: users, otps: otps, sender: sender, svc: svc}
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

	u, err := h.svc.Claim(context.Background(), PhoneClaimInput{
		UserID: caller.ID, Phone: "081234700002",
	})
	if err != nil {
		t.Fatalf("Claim: unexpected err: %v", err)
	}
	if u.Phone == nil || *u.Phone != "6281234700002" {
		t.Fatalf("expected normalized phone attached, got %+v", u.Phone)
	}
	if u.PhoneVerifiedAt != nil {
		t.Fatalf("expected phone_verified_at nil (claimed but unproven), got %v", *u.PhoneVerifiedAt)
	}
	if h.sender.calls != 0 {
		t.Fatalf("expected NO otp sent for a free-phone claim, got %d calls", h.sender.calls)
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
	// Guest must be untouched.
	reloadedGuest, _ := h.users.FindByID(context.Background(), guest.ID)
	if reloadedGuest.Phone == nil || *reloadedGuest.Phone != "6281234700003" {
		t.Fatalf("expected guest's phone untouched after rejected claim, got %+v", reloadedGuest.Phone)
	}
}

// (4) Phone owned by a GUEST + valid OTP → the number is released from the
// guest row and attached, VERIFIED, to the caller (§11 going-forward
// ownership; the guest's past orders stay on the guest row).
func TestPhoneClaimService_Claim_PhoneOwnedByOtherGuest_ValidOTP_ReleasesAndAttaches(t *testing.T) {
	h := newPhoneClaimTestHarness(t, OTPConfig{})
	guestType := model.CustomerTypeGuest
	guest := &model.User{
		ID: uuid.New(), Phone: strp("6281234700004"), Name: "Guest Diambil Alih",
		UserType: model.UserTypeCustomer, CustomerType: &guestType, IsActive: true,
	}
	h.users.index(guest)
	caller := newSessionUser(h, "", false)

	code := h.requestAndReadOTP(t, caller.ID, "081234700004")

	u, err := h.svc.Claim(context.Background(), PhoneClaimInput{
		UserID: caller.ID, Phone: "081234700004", OTP: code,
	})
	if err != nil {
		t.Fatalf("Claim: unexpected err: %v", err)
	}
	if u.ID != caller.ID {
		t.Fatalf("expected phone attached to the CALLER's row, got user_id %s want %s", u.ID, caller.ID)
	}
	if u.Phone == nil || *u.Phone != "6281234700004" {
		t.Fatalf("expected normalized phone attached, got %+v", u.Phone)
	}
	if u.PhoneVerifiedAt == nil {
		t.Fatal("expected phone_verified_at set after a valid OTP round-trip")
	}
	reloadedGuest, err := h.users.FindByID(context.Background(), guest.ID)
	if err != nil {
		t.Fatalf("reload guest: %v", err)
	}
	if reloadedGuest.Phone != nil {
		t.Fatalf("expected guest's phone released (nil) after being absorbed, got %q", *reloadedGuest.Phone)
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
	reloadedOther, _ := h.users.FindByID(context.Background(), other.ID)
	if reloadedOther.Phone == nil || *reloadedOther.Phone != "6281234700005" {
		t.Fatalf("expected other registered customer's phone untouched, got %+v", reloadedOther.Phone)
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

	u, err := h.svc.Claim(context.Background(), PhoneClaimInput{
		UserID: caller.ID, Phone: "081234700008", OTP: code,
	})
	if err != nil {
		t.Fatalf("Claim: unexpected err: %v", err)
	}
	if u.ID != caller.ID {
		t.Fatalf("expected same user id, got %s want %s", u.ID, caller.ID)
	}
	if u.Phone == nil || *u.Phone != "6281234700008" {
		t.Fatalf("expected phone unchanged, got %+v", u.Phone)
	}
	if u.PhoneVerifiedAt == nil {
		t.Fatal("expected phone_verified_at set after a valid self-verify OTP round-trip")
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

	u, err := h.svc.Claim(context.Background(), PhoneClaimInput{
		UserID: caller.ID, Phone: "081234700010",
	})
	if err != nil {
		t.Fatalf("Claim: unexpected err: %v", err)
	}
	if u.PhoneVerifiedAt == nil || !u.PhoneVerifiedAt.Equal(originalVerifiedAt) {
		t.Fatalf("expected phone_verified_at unchanged (idempotent), got %+v want %v", u.PhoneVerifiedAt, originalVerifiedAt)
	}
	if h.sender.calls != 0 {
		t.Fatalf("expected NO otp sent for an idempotent self-claim, got %d calls", h.sender.calls)
	}
}
