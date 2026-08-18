// phone_claim_service.go — self-service "add/change my own WhatsApp number"
// for an ALREADY AUTHENTICATED user (customer or staff), the counterpart of
// GoogleOAuthService's phone handling for registration. Same business rule
// (§ owner decision): OTP is only required when the number is CONTESTED —
// either it belongs to a different account, or it's the caller's own number
// still unproven. A genuinely free number, or a number the caller already
// proved, never triggers a WhatsApp send.
//
// Deliberately reuses several package-level building blocks from
// google_oauth_service.go (googleOTPStore, googleOAuthUserStore, OTPConfig,
// phoneLockTxRunner, checkPerPhoneIssueCapWith/checkPerPhoneFailedCapWith,
// verifyPhoneOTP, sendPhoneOTP, cooldownErrorFromLatest, generateOTPCode,
// hashOTP) — same package, same OTP mechanics, no reason to fork a second
// copy of the trickiest (rate-limit/attempts-ceiling) logic (§22 least
// duplication). Kept as a SEPARATE service/type from GoogleOAuthService
// because the two are conceptually different flows (registration handoff vs.
// an existing session) with different inputs and different failure modes.
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/auth/model"
	"github.com/rajaku-printing/backend/internal/auth/repository"
	"github.com/rajaku-printing/backend/internal/notification/notificationapi"
	"github.com/rajaku-printing/backend/internal/pkg/phone"
)

// PhoneClaimReason disambiguates WHY an OTP is/isn't required for a given
// phone number, distinct from the boolean OTPRequired — critically, it lets
// the caller tell "this number is already yours, just unverified"
// (PhoneClaimReasonSelfVerify) apart from "someone ELSE has this number"
// (PhoneClaimReasonOwnedByOther), which the OTPRequired boolean alone cannot
// (both are `true`). See coordinator brief: a customer re-submitting their
// OWN unverified number must never be told "nomor sudah digunakan" — that's
// their own number, not a conflict with anyone else.
type PhoneClaimReason string

const (
	PhoneClaimReasonFree         PhoneClaimReason = "free"
	PhoneClaimReasonSelfVerified PhoneClaimReason = "self_verified"
	PhoneClaimReasonSelfVerify   PhoneClaimReason = "self_verify"
	PhoneClaimReasonOwnedByOther PhoneClaimReason = "owned_by_other"
)

// PhoneClaimRequestOTPInput — POST /auth/phone/request-otp (authenticated).
type PhoneClaimRequestOTPInput struct {
	UserID uuid.UUID
	Phone  string // raw, service normalizes
}

// PhoneClaimRequestOTPOutput — see PhoneClaimReason doc for the `Reason`
// field. PhoneMasked/ExpiresIn/ResendAfterSeconds only meaningful when
// OTPRequired is true.
type PhoneClaimRequestOTPOutput struct {
	OTPRequired        bool
	Reason             PhoneClaimReason
	PhoneMasked        string
	ExpiresIn          int // seconds
	ResendAfterSeconds int // seconds
}

// PhoneClaimInput — POST /auth/phone (authenticated).
type PhoneClaimInput struct {
	UserID uuid.UUID
	Phone  string // raw, service normalizes
	OTP    string // required only when the number is contested — see Claim's doc
}

// PhoneClaimService implements the authenticated "add/change my own phone
// number" flow.
type PhoneClaimService struct {
	users     googleOAuthUserStore
	otps      googleOTPStore
	otpCfg    OTPConfig
	otpSender notificationapi.OTPSender
	// phoneLockRunner — same phoneLockTxRunner GoogleOAuthService uses (§22
	// least duplication); the per-phone advisory lock it opens has no notion
	// of WHICH flow is using it, so sharing it is safe.
	phoneLockRunner phoneLockTxRunner
	txRunner        phoneClaimTxRunner
}

func NewPhoneClaimService(
	users *repository.UserRepository,
	otps *repository.PhoneVerificationRepository,
	otpCfg OTPConfig,
	db *gorm.DB,
) *PhoneClaimService {
	return &PhoneClaimService{
		users: users, otps: otps, otpCfg: otpCfg,
		phoneLockRunner: newGormPhoneLockTxRunner(db),
		txRunner:        newGormPhoneClaimTxRunner(db),
	}
}

// SetOTPSender wires the notification module's OTP-sending capability —
// setter-injected for the same composition-root ordering reason as
// GoogleOAuthService.SetOTPSender (notification.Service is built AFTER auth
// in router.go).
func (s *PhoneClaimService) SetOTPSender(sender notificationapi.OTPSender) {
	s.otpSender = sender
}

// RequestOTP issues a WhatsApp OTP proving ownership of `in.Phone` — but ONLY
// when that number is actually contested (see PhoneClaimReason doc). A free
// number, or a number the caller already proved, sends nothing.
func (s *PhoneClaimService) RequestOTP(ctx context.Context, in PhoneClaimRequestOTPInput) (*PhoneClaimRequestOTPOutput, error) {
	normalizedPhone, err := phone.Normalize(in.Phone)
	if err != nil {
		return nil, fmt.Errorf("phone invalid: %w", err)
	}

	existing, err := s.users.FindByPhone(ctx, normalizedPhone)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return &PhoneClaimRequestOTPOutput{OTPRequired: false, Reason: PhoneClaimReasonFree}, nil
		}
		return nil, fmt.Errorf("phone claim request otp: lookup phone: %w", err)
	}

	reason := PhoneClaimReasonOwnedByOther
	if existing.ID == in.UserID {
		if existing.PhoneVerifiedAt != nil {
			// Already the caller's own, already proven — nothing to do, and
			// definitely no WhatsApp send for zero-value work.
			return &PhoneClaimRequestOTPOutput{OTPRequired: false, Reason: PhoneClaimReasonSelfVerified}, nil
		}
		reason = PhoneClaimReasonSelfVerify
	}

	pv, code, err := s.issueChallenge(ctx, in.UserID, normalizedPhone)
	if err != nil {
		return nil, err
	}
	if err := sendPhoneOTP(ctx, s.otpSender, s.otpCfg, normalizedPhone, code, pv.ID); err != nil {
		return nil, err
	}

	return &PhoneClaimRequestOTPOutput{
		OTPRequired:        true,
		Reason:             reason,
		PhoneMasked:        phone.Mask(normalizedPhone),
		ExpiresIn:          int(s.otpCfg.TTL.Seconds()),
		ResendAfterSeconds: int(s.otpCfg.ResendCooldown.Seconds()),
	}, nil
}

// issueChallenge runs the per-phone rate-limit dance (issue cap → resend
// cooldown → cancel whatever was pending → create the fresh challenge) inside
// ONE transaction opened under a Postgres advisory lock scoped to
// normalizedPhone — same TOCTOU protection as GoogleOAuthService.RequestOTP
// (review finding #1), just keyed by userID instead of a handoff code.
func (s *PhoneClaimService) issueChallenge(ctx context.Context, userID uuid.UUID, normalizedPhone string) (*model.PhoneVerification, string, error) {
	code, err := generateOTPCode(s.otpCfg.CodeLength)
	if err != nil {
		return nil, "", fmt.Errorf("phone claim request otp: %w", err)
	}

	var pv *model.PhoneVerification
	err = s.phoneLockRunner.RunInTx(ctx, normalizedPhone, func(tx googleOTPStore) error {
		if err := checkPerPhoneIssueCapWith(ctx, tx, normalizedPhone, s.otpCfg.MaxPerPhoneHour); err != nil {
			return err
		}
		if err := checkResendCooldownForUserWith(ctx, tx, userID, normalizedPhone, s.otpCfg.ResendCooldown); err != nil {
			return err
		}
		// At most one active challenge per user at a time — cancel whatever
		// was pending (even under a different, possibly typo'd, phone
		// number) before issuing the new one. Mirrors
		// CancelPendingForHandoff's role in GoogleOAuthService.RequestOTP.
		if err := tx.CancelPendingForUser(ctx, userID); err != nil {
			return fmt.Errorf("phone claim request otp: cancel previous challenge: %w", err)
		}
		newPV := &model.PhoneVerification{
			UserID:    &userID,
			Phone:     normalizedPhone,
			CodeHash:  hashOTP(normalizedPhone, code),
			ExpiresAt: time.Now().UTC().Add(s.otpCfg.TTL),
		}
		if err := tx.Create(ctx, newPV); err != nil {
			return fmt.Errorf("phone claim request otp: create challenge: %w", err)
		}
		pv = newPV
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	return pv, code, nil
}

// Claim adds/changes the caller's own phone number.
//
//   - Number free (nobody owns it) → attached immediately, UNVERIFIED — no
//     OTP needed.
//   - Number already the caller's own, already verified → idempotent no-op
//     success (no OTP consumed, nothing re-sent).
//   - Number already the caller's own, NOT yet verified (the "click verifikasi
//     sekarang" scenario) → `otp` required; wrong/missing OTP returns
//     authapi.ErrPhoneSelfVerificationRequired (NOT ErrPhoneVerificationRequired
//     — this is never framed as "someone else has your number"). Valid OTP
//     stamps phone_verified_at on the SAME row.
//   - Number owned by a DIFFERENT account → `otp` required
//     (authapi.ErrPhoneVerificationRequired otherwise). Valid OTP: a STAFF or
//     already-REGISTERED owner is never merged into (authapi.ErrPhoneAlreadyUsed
//     — a dead end, pick a different number); a GUEST owner is absorbed —
//     the phone is released from the guest row and attached, verified, to the
//     caller (the guest's past order history stays on the guest row, just no
//     longer reachable via phone lookup going forward).
func (s *PhoneClaimService) Claim(ctx context.Context, in PhoneClaimInput) (*model.User, error) {
	normalizedPhone, err := phone.Normalize(in.Phone)
	if err != nil {
		return nil, fmt.Errorf("phone invalid: %w", err)
	}

	existing, err := s.users.FindByPhone(ctx, normalizedPhone)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			if err := s.users.SetPhone(ctx, in.UserID, &normalizedPhone, nil); err != nil {
				return nil, fmt.Errorf("phone claim: set phone: %w", err)
			}
			return s.reload(ctx, in.UserID)
		}
		return nil, fmt.Errorf("phone claim: lookup phone: %w", err)
	}

	if existing.ID == in.UserID {
		return s.claimSelf(ctx, in, existing)
	}
	return s.claimOther(ctx, in, normalizedPhone, existing)
}

// claimSelf handles "the number is already attached to the caller's own
// account" — see Claim's doc.
func (s *PhoneClaimService) claimSelf(ctx context.Context, in PhoneClaimInput, existing *model.User) (*model.User, error) {
	if existing.PhoneVerifiedAt != nil {
		return existing, nil // idempotent — already theirs, already proven.
	}
	if strings.TrimSpace(in.OTP) == "" {
		return nil, authapi.ErrPhoneSelfVerificationRequired
	}

	normalizedPhone := derefStr(existing.Phone)
	pv, err := s.verifyOTP(ctx, in.UserID, normalizedPhone, in.OTP)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	err = s.txRunner.RunInTx(ctx, func(tx phoneClaimTx) error {
		if err := tx.Users.SetPhone(ctx, in.UserID, &normalizedPhone, &now); err != nil {
			return fmt.Errorf("phone claim: mark self-verified: %w", err)
		}
		if err := tx.OTPs.MarkConsumed(ctx, pv.ID); err != nil {
			return fmt.Errorf("phone claim: mark otp consumed: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.reload(ctx, in.UserID)
}

// claimOther handles "the number is owned by a DIFFERENT account" — see
// Claim's doc.
func (s *PhoneClaimService) claimOther(ctx context.Context, in PhoneClaimInput, normalizedPhone string, existing *model.User) (*model.User, error) {
	if strings.TrimSpace(in.OTP) == "" {
		return nil, authapi.ErrPhoneVerificationRequired
	}

	pv, err := s.verifyOTP(ctx, in.UserID, normalizedPhone, in.OTP)
	if err != nil {
		return nil, err
	}

	// Staff / already-registered owners are NEVER merged into — OTP proves
	// WA ownership, not the target account's password (same invariant as
	// GoogleOAuthService.resolveUserForCompletion).
	isRegisteredCustomer := existing.CustomerType != nil && *existing.CustomerType == model.CustomerTypeRegistered
	if existing.UserType == model.UserTypeStaff || isRegisteredCustomer {
		return nil, authapi.ErrPhoneAlreadyUsed
	}

	now := time.Now().UTC()
	previousOwnerID := existing.ID
	err = s.txRunner.RunInTx(ctx, func(tx phoneClaimTx) error {
		// Authoritative re-check, inside the transaction: the OTP just
		// verified proves ownership against the state read a moment ago —
		// if a DIFFERENT owner claimed the number in the race window since
		// then, that proof no longer applies to the current state, so
		// reject rather than silently act on stale data.
		fresh, ferr := tx.Users.FindByPhone(ctx, normalizedPhone)
		if ferr != nil && !errors.Is(ferr, repository.ErrNotFound) {
			return fmt.Errorf("phone claim: re-check owner: %w", ferr)
		}
		switch {
		case ferr == nil && fresh.ID != previousOwnerID:
			return authapi.ErrPhoneVerificationRequired
		case ferr == nil:
			// Still owned by the row the OTP was verified against — release it.
			if err := tx.Users.SetPhone(ctx, previousOwnerID, nil, nil); err != nil {
				return fmt.Errorf("phone claim: release previous owner's phone: %w", err)
			}
		}
		// ferr is ErrNotFound: owner released/vanished between the check and
		// now — proceed as if the number was already free, now provably owned.
		if err := tx.Users.SetPhone(ctx, in.UserID, &normalizedPhone, &now); err != nil {
			return fmt.Errorf("phone claim: set phone: %w", err)
		}
		if err := tx.OTPs.MarkConsumed(ctx, pv.ID); err != nil {
			return fmt.Errorf("phone claim: mark otp consumed: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.reload(ctx, in.UserID)
}

// verifyOTP is PhoneClaimService's thin wrapper over the shared
// verifyPhoneOTP engine (see google_oauth_service.go — §22 least
// duplication), keyed by userID instead of a registration handoff.
func (s *PhoneClaimService) verifyOTP(ctx context.Context, userID uuid.UUID, normalizedPhone, otp string) (*model.PhoneVerification, error) {
	return verifyPhoneOTP(ctx, s.otps, s.phoneLockRunner, s.otpCfg, normalizedPhone, otp,
		func(ctx context.Context, otps googleOTPStore) (*model.PhoneVerification, error) {
			return otps.FindActiveByUser(ctx, userID, normalizedPhone)
		})
}

func (s *PhoneClaimService) reload(ctx context.Context, id uuid.UUID) (*model.User, error) {
	u, err := s.users.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("phone claim: reload user %s: %w", id, err)
	}
	return u, nil
}
