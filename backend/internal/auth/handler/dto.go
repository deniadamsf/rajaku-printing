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
