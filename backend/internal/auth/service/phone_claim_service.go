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
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/auth/model"
	"github.com/rajaku-printing/backend/internal/auth/repository"
	"github.com/rajaku-printing/backend/internal/notification/notificationapi"
	"github.com/rajaku-printing/backend/internal/order/orderapi"
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
	// CallerIsStaff — true when the authenticated caller is a staff account
	// (authapi.Identity.UserType == authapi.UserTypeStaff), set by the
	// handler. Staff are fully allowed to use this endpoint to add/change
	// THEIR OWN number (free number, or self-verify of an unproven one) —
	// see Claim's doc. The ONLY thing this field gates is claimOther's
	// guest-absorption branch: a staff caller must never absorb a customer's
	// guest identity/order history into their own staff row (§ phone-claim
	// review finding #2). Zero value (false) is the correct default for
	// every customer caller, so callers that don't set it (e.g. tests) get
	// the permissive customer behavior, not an accidental staff lockout.
	CallerIsStaff bool
}

// PhoneClaimOutput — result of Claim. MergedOrders is the number of orders
// that moved from an absorbed GUEST row to User's account (§11 satu
// pelanggan satu riwayat) — always present, 0 when no guest was absorbed
// (free number, self-verify, or idempotent no-op).
type PhoneClaimOutput struct {
	User         *model.User
	MergedOrders int64
}

// customerMergeAuditStore narrows repository.CustomerMergeRepository to the
// single write claimOther's guest-absorption branch needs — the durable
// audit row for §11 satu pelanggan satu riwayat (see recordCustomerMerge).
// Kept unit-testable with an in-memory fake instead of a real *gorm.DB (§22
// test requirement), same pattern as every other narrowed store in this
// package.
type customerMergeAuditStore interface {
	Create(ctx context.Context, cm *model.CustomerMerge) error
}

// Compile-time assertion — concrete repo satisfies the narrowed contract.
var _ customerMergeAuditStore = (*repository.CustomerMergeRepository)(nil)

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

// newOrderCustomerMerger builds the order module's tx-scoped
// orderapi.CustomerMerger from a live *gorm.DB transaction — supplied by the
// composition root (router.go), the only package allowed to import both the
// auth and order modules (§22 no cross-module internal imports). Required
// (non-nil) — see newGormPhoneClaimTxRunner for why a missing factory fails
// fast at startup instead of silently skipping the merge.
func NewPhoneClaimService(
	users *repository.UserRepository,
	otps *repository.PhoneVerificationRepository,
	otpCfg OTPConfig,
	db *gorm.DB,
	newOrderCustomerMerger func(tx *gorm.DB) orderapi.CustomerMerger,
) *PhoneClaimService {
	return &PhoneClaimService{
		users: users, otps: otps, otpCfg: otpCfg,
		phoneLockRunner: newGormPhoneLockTxRunner(db),
		txRunner:        newGormPhoneClaimTxRunner(db, newOrderCustomerMerger),
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
//
//   - Number already the caller's own, already verified → idempotent no-op
//     success (no OTP consumed, nothing re-sent).
//
//   - Number already the caller's own, NOT yet verified (the "click verifikasi
//     sekarang" scenario) → `otp` required; wrong/missing OTP returns
//     authapi.ErrPhoneSelfVerificationRequired (NOT ErrPhoneVerificationRequired
//     — this is never framed as "someone else has your number"). Valid OTP
//     stamps phone_verified_at on the SAME row.
//
//   - Number owned by a DIFFERENT account → `otp` required
//     (authapi.ErrPhoneVerificationRequired otherwise). Valid OTP: a STAFF or
//     already-REGISTERED owner is never merged into (authapi.ErrPhoneAlreadyUsed
//     — a dead end, pick a different number) — checked TWICE: once as a
//     fail-fast against the pre-transaction read, and AUTHORITATIVELY again
//     inside the transaction against a row-locked re-read, because the same
//     row id can change type between the two reads (mis. a concurrent Google
//     OAuth completion upgrading a guest to registered — see
//     absorbGuestOwnerAllowed's doc). A STAFF caller is also never allowed to
//     absorb, even a genuine guest — staff may only use this endpoint to
//     add/change their OWN number (see PhoneClaimInput.CallerIsStaff doc). A
//     GUEST owner absorbed by a non-staff caller is ABSORBED, in the SAME
//     transaction as everything else below:
//     1. the phone is released from the guest row;
//     2. every order the guest owned (orders.customer_id) is reassigned to
//     the caller via orderapi.CustomerMerger — this is the fix for the
//     bug this flow used to have: the guest's order history no longer
//     gets stranded on a phone-less row, it moves WITH the phone (§11
//     satu pelanggan satu riwayat);
//     3. the now-empty guest row is tombstoned (is_active=false) — kept, not
//     deleted, because audit columns elsewhere (orders.created_by,
//     design_files.uploaded_by, payment_proofs.uploaded_by,
//     order_state_history.changed_by, invoices.generated_by) still point
//     at it and must keep resolving to a real row;
//     4. the phone is attached, verified, to the caller.
//     PhoneClaimOutput.MergedOrders reports how many orders moved in step 2
//     (0 on every other path above).
//
//     Consequence worth noting so it's never mistaken for a regression: after
//     absorption those orders belong to a `customer_type=registered` owner,
//     so GuestOrderService.VerifyOwnership (which requires the order's owner
//     to be customer_type=guest) will no longer authenticate someone into
//     them via resi+phone. That's correct — the person now has a real
//     account and sees those orders in their own order history instead.
func (s *PhoneClaimService) Claim(ctx context.Context, in PhoneClaimInput) (*PhoneClaimOutput, error) {
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
			u, err := s.reload(ctx, in.UserID)
			if err != nil {
				return nil, err
			}
			return &PhoneClaimOutput{User: u}, nil
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
func (s *PhoneClaimService) claimSelf(ctx context.Context, in PhoneClaimInput, existing *model.User) (*PhoneClaimOutput, error) {
	if existing.PhoneVerifiedAt != nil {
		return &PhoneClaimOutput{User: existing}, nil // idempotent — already theirs, already proven.
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
	u, err := s.reload(ctx, in.UserID)
	if err != nil {
		return nil, err
	}
	return &PhoneClaimOutput{User: u}, nil
}

// claimOther handles "the number is owned by a DIFFERENT account" — see
// Claim's doc, including the guest-absorption steps (release phone → reassign
// order history → tombstone guest row → attach+verify to caller), all inside
// ONE transaction.
func (s *PhoneClaimService) claimOther(ctx context.Context, in PhoneClaimInput, normalizedPhone string, existing *model.User) (*PhoneClaimOutput, error) {
	if strings.TrimSpace(in.OTP) == "" {
		return nil, authapi.ErrPhoneVerificationRequired
	}

	pv, err := s.verifyOTP(ctx, in.UserID, normalizedPhone, in.OTP)
	if err != nil {
		return nil, err
	}

	// Fail-fast only, NOT authoritative — `existing` was read BEFORE this
	// transaction, so the row it describes can have changed type since (mis.
	// a concurrent Google OAuth completion running
	// UserRepository.UpgradeGuestToRegistered on the SAME row id, guest →
	// registered, in place — see FindByPhoneForUpdate's doc). Rejecting here
	// when we already know the answer just avoids paying for an OTP
	// round-trip on an obviously-doomed request; it is NOT what protects
	// against the guest-row-upgraded-mid-flight race. That protection is the
	// re-check inside the transaction below, against a ROW-LOCKED read.
	isRegisteredCustomer := existing.CustomerType != nil && *existing.CustomerType == model.CustomerTypeRegistered
	if existing.UserType == model.UserTypeStaff || isRegisteredCustomer {
		return nil, authapi.ErrPhoneAlreadyUsed
	}

	now := time.Now().UTC()
	previousOwnerID := existing.ID
	var mergedOrderIDs []uuid.UUID
	err = s.txRunner.RunInTx(ctx, func(tx phoneClaimTx) error {
		// AUTHORITATIVE re-check, inside the transaction, against a
		// row-locked read (`SELECT ... FOR UPDATE`): the OTP just verified
		// proves ownership against the state read a moment ago — if the
		// number changed hands OR the same row changed TYPE (guest →
		// staff/registered) in the race window since then, that proof no
		// longer applies to the current state, so reject rather than
		// silently act on stale data. The lock held from here until commit
		// also blocks a concurrent UpgradeGuestToRegistered on this same row
		// from racing past this check (it competes for the same row lock).
		fresh, ferr := tx.Users.FindByPhoneForUpdate(ctx, normalizedPhone)
		if ferr != nil && !errors.Is(ferr, repository.ErrNotFound) {
			return fmt.Errorf("phone claim: re-check owner: %w", ferr)
		}
		switch {
		case ferr == nil && fresh.ID != previousOwnerID:
			// A DIFFERENT owner claimed the number since the OTP was issued
			// — the proof no longer applies to the current owner.
			return authapi.ErrPhoneVerificationRequired
		case ferr == nil:
			if err := absorbGuestOwnerAllowed(in, fresh); err != nil {
				return err
			}
			ids, err := s.reassignGuestOrders(ctx, tx, previousOwnerID, in.UserID)
			if err != nil {
				return err
			}
			mergedOrderIDs = ids
			if err := s.recordCustomerMerge(ctx, tx, previousOwnerID, in.UserID, normalizedPhone, ids); err != nil {
				return err
			}
		}
		// ferr is ErrNotFound: owner released/vanished between the check and
		// now — proceed as if the number was already free, now provably
		// owned. No merge here: without a confirmed still-existing owner row
		// there is nothing concrete to absorb.
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
	return s.buildClaimOtherOutput(ctx, in.UserID, normalizedPhone, now, int64(len(mergedOrderIDs)))
}

// absorbGuestOwnerAllowed is claimOther's authoritative, in-transaction,
// row-locked gate on WHO may be absorbed (§ phone-claim review findings #1
// and #2):
//   - `fresh` (the row-locked re-read of the number's current owner) must
//     still be a GUEST — a staff or already-registered row reached this far
//     only if it changed type AFTER the pre-transaction fail-fast check
//     above ran (finding #1's race), and must be rejected here too, not just
//     there.
//   - the CALLER absorbing it must not be staff (finding #2) — staff are
//     free to add/change their OWN number (see PhoneClaimInput.CallerIsStaff
//     doc) but must never pull a customer's guest identity/order history
//     into their staff row.
//
// Either violation returns authapi.ErrPhoneAlreadyUsed — same sentinel the
// pre-transaction fail-fast check above uses for the identical business
// reason (OTP proves WA ownership, not permission to absorb).
func absorbGuestOwnerAllowed(in PhoneClaimInput, fresh *model.User) error {
	isRegisteredCustomer := fresh.CustomerType != nil && *fresh.CustomerType == model.CustomerTypeRegistered
	if fresh.UserType == model.UserTypeStaff || isRegisteredCustomer {
		return authapi.ErrPhoneAlreadyUsed
	}
	if in.CallerIsStaff {
		return authapi.ErrPhoneAlreadyUsed
	}
	return nil
}

// reassignGuestOrders releases the absorbed guest's phone, moves its order
// history to the caller (§11 satu pelanggan satu riwayat), then tombstones
// the now phone-less, order-less guest row — all against the transaction-
// scoped stores so it commits/rolls back atomically with everything else in
// claimOther. Returns the IDs of every order that moved (possibly empty —
// the guest may not have ordered anything yet).
func (s *PhoneClaimService) reassignGuestOrders(ctx context.Context, tx phoneClaimTx, previousOwnerID, toUserID uuid.UUID) ([]uuid.UUID, error) {
	if err := tx.Users.SetPhone(ctx, previousOwnerID, nil, nil); err != nil {
		return nil, fmt.Errorf("phone claim: release previous owner's phone: %w", err)
	}
	ids, err := tx.Orders.ReassignCustomer(ctx, previousOwnerID, toUserID)
	if err != nil {
		return nil, fmt.Errorf("phone claim: reassign guest %s order history to %s: %w", previousOwnerID, toUserID, err)
	}
	if err := tx.Users.SetActive(ctx, previousOwnerID, false); err != nil {
		return nil, fmt.Errorf("phone claim: tombstone absorbed guest %s: %w", previousOwnerID, err)
	}
	return ids, nil
}

// recordCustomerMerge persists the durable customer_merges audit row for a
// guest-identity absorption — inside the SAME transaction as the order
// reassignment and phone release/tombstone it describes (called right after
// reassignGuestOrders, per that ordering), so the two can never separate:
// either both commit or both roll back together. Written even when orderIDs
// is empty — a guest's identity being absorbed is itself the fact worth
// recording, whether or not that guest happened to have any orders yet.
func (s *PhoneClaimService) recordCustomerMerge(ctx context.Context, tx phoneClaimTx, fromUserID, toUserID uuid.UUID, phone string, orderIDs []uuid.UUID) error {
	cm := model.NewCustomerMerge(fromUserID, toUserID, phone, orderIDs, model.CustomerMergeSourcePhoneClaim)
	if err := tx.CustomerMerges.Create(ctx, cm); err != nil {
		return fmt.Errorf("phone claim: record customer merge audit row (from %s to %s): %w", fromUserID, toUserID, err)
	}
	return nil
}

// buildClaimOtherOutput reloads the caller's row after a COMMITTED claimOther
// transaction. Reload failure here is deliberately NON-FATAL (§ phone-claim
// review finding #4): the transaction already committed — the phone moved,
// N orders were reassigned, the guest row was tombstoned — so returning an
// error at this point would tell the caller the operation failed when it
// actually succeeded, with no way to undo it. A retry would then hit
// claimSelf's idempotent success path and report MergedOrders=0, silently
// losing the "N pesanan digabungkan" confirmation forever.
//
// On reload failure this logs the failure (with the exact data an operator
// would need to reconcile the discrepancy by hand) and falls back to a User
// built from `verifiedPhone`/`verifiedAt` — the values the just-committed
// transaction wrote for this user_id, so the fallback is still an accurate
// report of the number/verification state, not a placeholder. Only fields
// this flow doesn't touch (name, email, roles, ...) are missing from it,
// exactly what a caller retrying the reload themselves would also be missing
// until they re-fetch.
func (s *PhoneClaimService) buildClaimOtherOutput(ctx context.Context, userID uuid.UUID, verifiedPhone string, verifiedAt time.Time, mergedOrders int64) (*PhoneClaimOutput, error) {
	u, err := s.reload(ctx, userID)
	if err != nil {
		log.Ctx(ctx).Error().Err(err).
			Str("user_id", userID.String()).
			Int64("merged_orders", mergedOrders).
			Msg("phone claim: absorption committed but post-commit reload failed — reporting the committed result anyway")
		phoneCopy := verifiedPhone
		verifiedAtCopy := verifiedAt
		return &PhoneClaimOutput{
			User:         &model.User{ID: userID, Phone: &phoneCopy, PhoneVerifiedAt: &verifiedAtCopy},
			MergedOrders: mergedOrders,
		}, nil
	}
	return &PhoneClaimOutput{User: u, MergedOrders: mergedOrders}, nil
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
