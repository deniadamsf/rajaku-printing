package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/auth/service"
	"github.com/rajaku-printing/backend/internal/httpx"
	"github.com/rajaku-printing/backend/internal/pkg/phone"
)

// PhoneClaimHandler exposes the authenticated "add/change my own WhatsApp
// number" endpoints (§ counterpart of GoogleOAuthHandler's phone handling for
// registration). Kept separate from Handler/GoogleOAuthHandler — different
// concern, avoids a god-handler (§22).
type PhoneClaimHandler struct {
	svc *service.PhoneClaimService
}

func NewPhoneClaimHandler(svc *service.PhoneClaimService) *PhoneClaimHandler {
	return &PhoneClaimHandler{svc: svc}
}

// RegisterRoutes mounts POST /auth/phone/request-otp and POST /auth/phone
// under `g` — the caller (router.go) MUST have already applied BOTH
// authapi.RequireAuth (this is a self-service endpoint for an existing
// session, not a public one) AND the strict OTP rate limiter (RATE_LIMIT_OTP_*
// — this endpoint can reveal whether an arbitrary phone number is already
// claimed, the same brute-force shape as POST /auth/google/request-otp).
func (h *PhoneClaimHandler) RegisterRoutes(g *gin.RouterGroup) {
	g.POST("/request-otp", h.RequestOTP)
	g.POST("", h.Claim)
}

// RequestOTP — POST /auth/phone/request-otp {phone}.
func (h *PhoneClaimHandler) RequestOTP(c *gin.Context) {
	id, err := authapi.IdentityFromContext(c.Request.Context())
	if err != nil {
		httpx.Error(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}
	var req phoneClaimRequestOTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
		return
	}
	out, err := h.svc.RequestOTP(c.Request.Context(), service.PhoneClaimRequestOTPInput{
		UserID: id.UserID,
		Phone:  req.Phone,
	})
	if err != nil {
		h.mapError(c, err)
		return
	}
	httpx.OK(c, phoneClaimRequestOTPResponse{
		OTPRequired:        out.OTPRequired,
		Reason:             string(out.Reason),
		PhoneMasked:        out.PhoneMasked,
		ExpiresIn:          out.ExpiresIn,
		ResendAfterSeconds: out.ResendAfterSeconds,
	})
}

// Claim — POST /auth/phone {phone, otp?}.
func (h *PhoneClaimHandler) Claim(c *gin.Context) {
	id, err := authapi.IdentityFromContext(c.Request.Context())
	if err != nil {
		httpx.Error(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}
	var req phoneClaimRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
		return
	}
	out, err := h.svc.Claim(c.Request.Context(), service.PhoneClaimInput{
		UserID: id.UserID,
		Phone:  req.Phone,
		OTP:    req.OTP,
		// CallerIsStaff — gates ONLY the guest-absorption branch inside
		// claimOther (§ phone-claim review finding #2); a staff caller can
		// still freely add/change their own number via this same endpoint.
		CallerIsStaff: id.UserType == authapi.UserTypeStaff,
	})
	if err != nil {
		h.mapError(c, err)
		return
	}
	resp := phoneClaimResponse{PhoneVerified: out.User.PhoneVerifiedAt != nil, MergedOrders: out.MergedOrders}
	if out.User.Phone != nil {
		resp.Phone = *out.User.Phone
	}
	httpx.OK(c, resp)
}

func (h *PhoneClaimHandler) mapError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, authapi.ErrPhoneVerificationRequired):
		httpx.Error(c, http.StatusConflict, httpx.CodePhoneVerificationRequired,
			"Nomor sudah digunakan, pakai nomor lain — atau verifikasi kalau ini memang nomormu")
	case errors.Is(err, authapi.ErrPhoneSelfVerificationRequired):
		httpx.Error(c, http.StatusConflict, httpx.CodePhoneSelfVerificationRequired,
			"Nomor ini sudah terhubung ke akunmu, masukkan kode OTP untuk verifikasi")
	case errors.Is(err, authapi.ErrPhoneAlreadyUsed):
		httpx.Error(c, http.StatusConflict, httpx.CodePhoneAlreadyUsed, "Nomor WA sudah terdaftar")
	case errors.Is(err, authapi.ErrOTPExpired):
		httpx.Error(c, http.StatusBadRequest, httpx.CodeOTPExpired, "Kode OTP tidak ditemukan atau sudah kedaluwarsa, minta kode baru")
	case errors.Is(err, authapi.ErrOTPTooManyAttempts):
		httpx.Error(c, http.StatusTooManyRequests, httpx.CodeOTPTooManyAttempts, "Terlalu banyak percobaan salah, minta kode OTP baru")
	case errors.Is(err, authapi.ErrOTPInvalid):
		var invalidErr *authapi.OTPInvalidError
		var details any
		if errors.As(err, &invalidErr) {
			details = gin.H{"attempts_left": invalidErr.AttemptsLeft}
		}
		httpx.ErrorWithDetails(c, http.StatusBadRequest, httpx.CodeOTPInvalid, "Kode OTP salah", details)
	case errors.Is(err, authapi.ErrOTPCooldown):
		var cooldownErr *authapi.OTPCooldownError
		var details any
		if errors.As(err, &cooldownErr) {
			details = gin.H{"resend_available_in": cooldownErr.ResendAvailableIn}
		}
		httpx.ErrorWithDetails(c, http.StatusTooManyRequests, httpx.CodeOTPCooldown, "Tunggu sebentar sebelum minta kode baru", details)
	case errors.Is(err, phone.ErrEmpty),
		errors.Is(err, phone.ErrInvalidPrefix),
		errors.Is(err, phone.ErrInvalidLength),
		errors.Is(err, phone.ErrContainsLetters):
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
	default:
		log.Ctx(c.Request.Context()).Error().Err(err).Msg("phone claim error")
		httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
	}
}
