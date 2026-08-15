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
	Phone string `json:"phone" binding:"required,min=8,max=20"`
	Name  string `json:"name"  binding:"max=255"`
	OTP   string `json:"otp"   binding:"required,min=6,max=8"`
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
// Deliberately identical in shape no matter the phone's status (see
// service.RequestOTPOutput doc) — anti-enumeration.
type googleRequestOTPResponse struct {
	Status            string `json:"status"` // always "otp_sent"
	PhoneMasked       string `json:"phone_masked"`
	ExpiresIn         int    `json:"expires_in"`          // seconds
	ResendAvailableIn int    `json:"resend_available_in"` // seconds
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
