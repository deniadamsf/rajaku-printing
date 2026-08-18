// Package handler contains the HTTP layer for auth. No business logic here —
// bind → validate → call service → map result to envelope.
package handler

type registerCustomerRequest struct {
	Email    string `json:"email"    binding:"required,email,max=255"`
	Phone    string `json:"phone"    binding:"required,min=8,max=20"`
	Name     string `json:"name"     binding:"required,min=1,max=255"`
	Password string `json:"password" binding:"required,min=8,max=72"`
}

type loginRequest struct {
	Email    string `json:"email"    binding:"required,email,max=255"`
	Password string `json:"password" binding:"required,min=1,max=72"`
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"` // seconds
}

type registerResponse struct {
	Token tokenResponse `json:"token"`
	User  userSummary   `json:"user"`
}

type userSummary struct {
	ID       string `json:"id"`
	Email    string `json:"email,omitempty"`
	Phone    string `json:"phone"`
	Name     string `json:"name"`
	UserType string `json:"user_type"`
}

// guestVerifyRequest — POST /lacak/:resi/verify body. Phone accepts any
// locally-recognized format (08xx / 62xx / +62xx); normalized in service.
type guestVerifyRequest struct {
	Phone string `json:"phone" binding:"required,min=8,max=20"`
}

// guestVerifyResponse — contract is fixed: exactly {"token", "expires_at"}.
type guestVerifyResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"` // RFC3339 UTC
}

// ---- Google OAuth (§ auth module) ----

// googleExchangeRequest — POST /auth/google/exchange body.
type googleExchangeRequest struct {
	Code string `json:"code" binding:"required"`
}

// googleCompleteRequest — POST /auth/google/complete body. Name is optional —
// falls back to the name Google supplied at the /exchange step. OTP is the
// WhatsApp code obtained from POST /auth/google/request-otp — mandatory,
// proves the caller controls `phone` before any user is created/upgraded.
// OTP min length matches the configured minimum OTP_CODE_LENGTH floor (6,
// review finding #7) — binding rejects an obviously-too-short guess before
// it ever reaches the service/DB round-trip.
type googleCompleteRequest struct {
	Code  string `json:"code"  binding:"required"`
	Phone string `json:"phone" binding:"omitempty,min=8,max=20"`
	Name  string `json:"name"  binding:"max=255"`
	OTP   string `json:"otp"   binding:"omitempty,min=6,max=8"`
}

// googleRequestOTPRequest — POST /auth/google/request-otp body. No Name
// field (review finding #10) — RequestOTP never used it; the display name is
// collected/validated by googleCompleteRequest instead, where it's actually
// consumed.
type googleRequestOTPRequest struct {
	Code  string `json:"code"  binding:"required"`
	Phone string `json:"phone" binding:"required,min=8,max=20"`
}

// googleRequestOTPResponse — POST /auth/google/request-otp response.
// OTPRequired deliberately REVEALS whether the phone is already claimed —
// see service.RequestOTPOutput doc for why this is no longer anti-
// enumeration. PhoneMasked/ExpiresIn/ResendAfterSeconds are only meaningful
// when OTPRequired is true.
type googleRequestOTPResponse struct {
	OTPRequired        bool   `json:"otp_required"`
	PhoneMasked        string `json:"phone_masked,omitempty"`
	ExpiresIn          int    `json:"expires_in,omitempty"`
	ResendAfterSeconds int    `json:"resend_after_seconds,omitempty"`
}

// ---- Phone claim (§ authenticated users adding/changing their own number) ----

// phoneClaimRequestOTPRequest — POST /auth/phone/request-otp body
// (authenticated). No `otp` field — this endpoint only ever ISSUES a
// challenge, mirroring googleRequestOTPRequest.
type phoneClaimRequestOTPRequest struct {
	Phone string `json:"phone" binding:"required,min=8,max=20"`
}

// phoneClaimRequestOTPResponse — response contract for POST
// /auth/phone/request-otp. `Reason` disambiguates WHY an OTP is/isn't
// required so the frontend can pick accurate copy:
//   - "free"           — nobody has this number, no OTP needed.
//   - "self_verified"  — already the caller's own number AND already proven,
//     nothing to do (no OTP sent).
//   - "self_verify"    — already the caller's own number but NOT yet proven
//     — NOT a conflict with anyone else, just needs the OTP round-trip.
//   - "owned_by_other" — a DIFFERENT account (guest/registered/staff) holds
//     this number — the conflict path, OTP proves ownership before claiming.
type phoneClaimRequestOTPResponse struct {
	OTPRequired        bool   `json:"otp_required"`
	Reason             string `json:"reason"`
	PhoneMasked        string `json:"phone_masked,omitempty"`
	ExpiresIn          int    `json:"expires_in,omitempty"`
	ResendAfterSeconds int    `json:"resend_after_seconds,omitempty"`
}

// phoneClaimRequest — POST /auth/phone body (authenticated) — add/change the
// caller's own phone number. `otp` mandatory only when the number is already
// claimed by someone (self-unverified or a different account) — see
// service.PhoneClaimService.Claim.
type phoneClaimRequest struct {
	Phone string `json:"phone" binding:"required,min=8,max=20"`
	OTP   string `json:"otp"   binding:"omitempty,min=6,max=8"`
}

// phoneClaimResponse — POST /auth/phone response. PhoneVerified is a boolean
// projection of phone_verified_at — never leak the raw timestamp to
// customers (§ /auth/me contract).
type phoneClaimResponse struct {
	Phone         string `json:"phone"`
	PhoneVerified bool   `json:"phone_verified"`
}

// googleExchangeResponse is returned by both /exchange and /complete — its
// shape depends on Status:
//   - "session":    Token + User are populated (same shapes as registerResponse).
//   - "need_phone": Code/Email/Name/RedirectPath are populated; frontend must
//     collect a phone number and POST to /complete with the same Code.
type googleExchangeResponse struct {
	Status       string         `json:"status"`
	Token        *tokenResponse `json:"token,omitempty"`
	User         *userSummary   `json:"user,omitempty"`
	Code         string         `json:"code,omitempty"`
	Email        string         `json:"email,omitempty"`
	Name         string         `json:"name,omitempty"`
	RedirectPath string         `json:"redirect_path,omitempty"`
}
