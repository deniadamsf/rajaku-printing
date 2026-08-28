package authapi

import "errors"

// Sentinel errors exposed by the auth Service. Map to HTTP 401 at handler.
var (
	ErrInvalidToken = errors.New("authapi: invalid token")
	ErrExpiredToken = errors.New("authapi: expired token")

	// Login-specific
	ErrUserNotFound    = errors.New("authapi: user not found")
	ErrInvalidPassword = errors.New("authapi: invalid password")
	ErrUserInactive    = errors.New("authapi: user is inactive")

	// Registration
	ErrEmailAlreadyUsed = errors.New("authapi: email already registered")
	ErrPhoneAlreadyUsed = errors.New("authapi: phone already registered")

	// Lookup
	ErrCustomerNotFound = errors.New("authapi: customer not found")

	// Membership (§30 CLAUDE.md) — CAS transition conflict: membership_status
	// berubah di antara caller membaca status lama dan menulis status baru
	// (mis. dua approve konkuren). Caller (membership/service) memetakan ini
	// ke membershipapi.ErrMembershipInvalidTransition.
	ErrMembershipStatusConflict = errors.New("authapi: membership status changed concurrently, retry")

	// Guest order ownership verification (POST /lacak/:resi/verify). Deliberately
	// generic — resi-not-found and phone-mismatch map to THIS SAME sentinel so
	// the HTTP response is identical for both cases (no resi enumeration).
	ErrGuestVerificationFailed = errors.New("authapi: order/phone verification failed")

	// Staff / role admin (§10)
	ErrStaffNotFound         = errors.New("authapi: staff not found")
	ErrRoleNotFound          = errors.New("authapi: role not found")
	ErrRoleDuplicateName     = errors.New("authapi: role name already exists")
	ErrRoleSystemLocked      = errors.New("authapi: system role cannot be modified/deleted")
	ErrInviteInvalid         = errors.New("authapi: invite token invalid")
	ErrInviteExpired         = errors.New("authapi: invite token expired")
	ErrInviteAlreadyUsed     = errors.New("authapi: invite token already used")
	ErrInvalidRoleAssignment = errors.New("authapi: role id list contains invalid entries")
	ErrInvalidPermissionSet  = errors.New("authapi: permission codes contain invalid entries")

	// Google OAuth (§3, §10)
	ErrOAuthDisabled        = errors.New("authapi: google oauth not configured")
	ErrOAuthCodeInvalid     = errors.New("authapi: oauth handoff code invalid, used, or expired")
	ErrOAuthStaffNotAllowed = errors.New("authapi: staff accounts must log in with email + password")
	ErrOAuthAccountConflict = errors.New("authapi: google account is already linked to a different user")
	// ErrOAuthEmailUnverified — Google reported an unverified email for a
	// registration attempt. Customer accounts require a (unique) email column
	// per CHECK users_email_required — persisting an unverified address would
	// let anyone squat someone else's email just by typing it into Google's
	// consent screen for an account they don't control. Reject the whole login
	// instead of silently dropping the email.
	ErrOAuthEmailUnverified = errors.New("authapi: google email not verified")

	// OTP WhatsApp verification (Google OAuth registration — nomor WA bukan
	// rahasia, jadi kepemilikan wajib dibuktikan lewat kode OTP sebelum akun
	// dibuat/di-upgrade dari guest). Lihat google_oauth_service.go.
	ErrOTPInvalid         = errors.New("authapi: otp code invalid")
	ErrOTPExpired         = errors.New("authapi: otp code not found or expired")
	ErrOTPTooManyAttempts = errors.New("authapi: otp max attempts exceeded")
	ErrOTPCooldown        = errors.New("authapi: otp resend cooldown active")

	// Phone claim / ownership proof — §business rule: OTP hanya diterbitkan
	// saat terjadi tabrakan identitas, bukan setiap registrasi/klaim nomor.
	//
	// ErrPhoneVerificationRequired — the phone the caller is trying to
	// claim/register with is ALREADY OWNED BY SOMEONE ELSE (a different
	// user_id — guest, registered, or staff) and no (or no valid) OTP was
	// supplied to prove the caller actually controls that WhatsApp number.
	// Distinct from ErrPhoneAlreadyUsed — this one is ACTIONABLE (the caller
	// can request an OTP and retry); ErrPhoneAlreadyUsed is a dead end (the
	// number's current owner can never be merged into, even with a valid
	// OTP — see resolveUserForCompletion / PhoneClaimService.Claim).
	ErrPhoneVerificationRequired = errors.New("authapi: phone already claimed by another account, verification required")
	// ErrPhoneSelfVerificationRequired — the phone the caller is trying to
	// verify is ALREADY ATTACHED TO THEIR OWN ACCOUNT, just not yet proven
	// (phone_verified_at IS NULL). This is NOT a conflict with anyone else —
	// deliberately a separate sentinel/code from ErrPhoneVerificationRequired
	// so the frontend never tells a user "someone else has your own number".
	ErrPhoneSelfVerificationRequired = errors.New("authapi: caller's own phone not yet verified, otp required")
)

// OTPCooldownError wraps ErrOTPCooldown with the exact remaining wait time
// (seconds) so the handler can surface `resend_available_in` in the response
// details without the handler needing to recompute it. errors.Is(err,
// ErrOTPCooldown) still works via Unwrap — no custom Is() method needed.
type OTPCooldownError struct {
	ResendAvailableIn int
}

func (e *OTPCooldownError) Error() string { return ErrOTPCooldown.Error() }
func (e *OTPCooldownError) Unwrap() error { return ErrOTPCooldown }

// OTPInvalidError wraps ErrOTPInvalid with how many attempts remain before
// ErrOTPTooManyAttempts kicks in.
type OTPInvalidError struct {
	AttemptsLeft int
}

func (e *OTPInvalidError) Error() string { return ErrOTPInvalid.Error() }
func (e *OTPInvalidError) Unwrap() error { return ErrOTPInvalid }
