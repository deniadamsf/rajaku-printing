package handler

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/auth/service"
	"github.com/rajaku-printing/backend/internal/httpx"
	"github.com/rajaku-printing/backend/internal/pkg/phone"
)

// oauthStateCookie carries "<nonce>|<redirectPath>" between /start and
// /callback — HttpOnly so it can't be read/tampered with by page JS, scoped
// to the /auth/google path so it isn't sent on unrelated requests.
const oauthStateCookie = "rajaku_oauth_state"

// GoogleOAuthHandler exposes the Google login/registration HTTP endpoints.
// Kept separate from Handler (email+password login/register) — different
// concern, avoids a god-handler (§22).
type GoogleOAuthHandler struct {
	svc *service.GoogleOAuthService

	// enabled mirrors config.GoogleOAuthConfig.Enabled() — when false, every
	// endpoint replies 503 OAUTH_NOT_CONFIGURED instead of 404, so the
	// frontend gets an explicit, actionable error rather than "route doesn't
	// exist".
	enabled bool
	// frontendURL — every redirect this handler issues (success or error)
	// goes here (§2 "satu sumber APP_BASE_URL", read once at config load).
	frontendURL string
	// cookieSecure — Secure flag on the state cookie; false only in
	// development (plain http://localhost), true everywhere else.
	cookieSecure bool
}

func NewGoogleOAuthHandler(svc *service.GoogleOAuthService, enabled bool, frontendURL string, cookieSecure bool) *GoogleOAuthHandler {
	return &GoogleOAuthHandler{
		svc:          svc,
		enabled:      enabled,
		frontendURL:  strings.TrimRight(frontendURL, "/"),
		cookieSecure: cookieSecure,
	}
}

// Start — GET /auth/google/start?redirect=<path>. Redirects the browser to
// Google's consent screen.
func (h *GoogleOAuthHandler) Start(c *gin.Context) {
	if !h.enabled {
		httpx.Error(c, http.StatusServiceUnavailable, httpx.CodeOAuthNotConfigured, "Login Google belum dikonfigurasi")
		return
	}

	redirectPath := sanitizeRedirectPath(c.Query("redirect"))

	nonce, err := randomNonce()
	if err != nil {
		log.Ctx(c.Request.Context()).Error().Err(err).Msg("google oauth: generate state nonce")
		httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
		return
	}

	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(oauthStateCookie, nonce+"|"+redirectPath, 600, "/api/v1/auth/google", "", h.cookieSecure, true)

	c.Redirect(http.StatusFound, h.svc.StartURL(nonce))
}

// Callback — GET /auth/google/callback?code=&state=&error=. This is a
// top-level browser navigation coming back from Google, NOT an API call —
// it must always end in a 302 redirect to the frontend, never a JSON body
// (the browser is mid-navigation, there's no JS listening for a response).
func (h *GoogleOAuthHandler) Callback(c *gin.Context) {
	// Always consume+clear the state cookie exactly once, regardless of
	// outcome, so a retried/duplicated callback can't replay it.
	rawCookie, cookieErr := c.Cookie(oauthStateCookie)
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(oauthStateCookie, "", -1, "/api/v1/auth/google", "", h.cookieSecure, true)

	if !h.enabled {
		h.redirectLoginError(c, "server_error")
		return
	}
	if errParam := c.Query("error"); errParam != "" {
		// mis. access_denied — user cancelled consent on Google's screen.
		h.redirectLoginError(c, "dibatalkan")
		return
	}

	nonce, redirectPath, ok := splitStateCookie(rawCookie)
	// Re-validate even though /start already sanitized it before setting the
	// cookie (review finding #9) — defense in depth against any way the
	// cookie value could end up unexpected by the time it round-trips back.
	redirectPath = sanitizeRedirectPath(redirectPath)
	state := c.Query("state")
	if cookieErr != nil || !ok || state == "" ||
		subtle.ConstantTimeCompare([]byte(nonce), []byte(state)) != 1 {
		h.redirectLoginError(c, "state_mismatch")
		return
	}

	code := c.Query("code")
	if code == "" {
		h.redirectLoginError(c, "server_error")
		return
	}

	handoff, err := h.svc.HandleCallback(c.Request.Context(), code, redirectPath)
	if err != nil {
		h.redirectLoginError(c, h.oauthErrorSlug(c, err))
		return
	}

	c.Redirect(http.StatusFound, h.frontendURL+"/auth/google?code="+url.QueryEscape(handoff))
}

// Exchange — POST /auth/google/exchange {code}. Called by the frontend SPA
// right after landing on /auth/google?code=... (fetch/XHR, never logged in
// browser history — see migration 000012 for why the handoff-code
// indirection exists at all).
func (h *GoogleOAuthHandler) Exchange(c *gin.Context) {
	if !h.enabled {
		httpx.Error(c, http.StatusServiceUnavailable, httpx.CodeOAuthNotConfigured, "Login Google belum dikonfigurasi")
		return
	}
	var req googleExchangeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
		return
	}
	out, err := h.svc.Exchange(c.Request.Context(), req.Code)
	if err != nil {
		h.mapOAuthError(c, err)
		return
	}
	httpx.OK(c, toGoogleExchangeResponse(out))
}

// RequestOTP — POST /auth/google/request-otp {code, phone}. Sends a
// WhatsApp OTP to `phone` proving the caller controls it — required before
// Complete() will create/upgrade any user (nomor WA bukan rahasia, security
// review finding A). Response shape is fixed regardless of whether the phone
// is unclaimed, a guest, a registered customer, or staff — see
// service.RequestOTPOutput doc for why.
func (h *GoogleOAuthHandler) RequestOTP(c *gin.Context) {
	if !h.enabled {
		httpx.Error(c, http.StatusServiceUnavailable, httpx.CodeOAuthNotConfigured, "Login Google belum dikonfigurasi")
		return
	}
	var req googleRequestOTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
		return
	}
	out, err := h.svc.RequestOTP(c.Request.Context(), service.RequestOTPInput{
		Code:  req.Code,
		Phone: req.Phone,
	})
	if err != nil {
		h.mapOAuthError(c, err)
		return
	}
	httpx.OK(c, googleRequestOTPResponse{
		OTPRequired:        out.OTPRequired,
		PhoneMasked:        out.PhoneMasked,
		ExpiresIn:          out.ExpiresIn,
		ResendAfterSeconds: out.ResendAfterSeconds,
	})
}

// Complete — POST /auth/google/complete {code, phone, name, otp}. Finishes
// the registration flow started by a "need_phone" Exchange response — `otp`
// is the WhatsApp code obtained from RequestOTP, mandatory as of the OTP
// security fix (previously this endpoint trusted the raw phone number with
// no proof of ownership).
func (h *GoogleOAuthHandler) Complete(c *gin.Context) {
	if !h.enabled {
		httpx.Error(c, http.StatusServiceUnavailable, httpx.CodeOAuthNotConfigured, "Login Google belum dikonfigurasi")
		return
	}
	var req googleCompleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
		return
	}
	out, err := h.svc.Complete(c.Request.Context(), service.CompleteGoogleInput{
		Code:  req.Code,
		Phone: req.Phone,
		Name:  req.Name,
		OTP:   req.OTP,
	})
	if err != nil {
		h.mapOAuthError(c, err)
		return
	}
	httpx.OK(c, toGoogleExchangeResponse(out))
}

func (h *GoogleOAuthHandler) redirectLoginError(c *gin.Context, slug string) {
	c.Redirect(http.StatusFound, h.frontendURL+"/login?oauth_error="+url.QueryEscape(slug))
}

// oauthErrorSlug maps a service error to a frontend-facing slug, logging the
// underlying cause first (the slug alone isn't enough to debug a report from
// a customer — this is the only place that error is ever recorded, since the
// caller only sees the redirect).
func (h *GoogleOAuthHandler) oauthErrorSlug(c *gin.Context, err error) string {
	switch {
	case errors.Is(err, authapi.ErrOAuthStaffNotAllowed):
		return "staff_not_allowed"
	case errors.Is(err, authapi.ErrOAuthAccountConflict):
		return "account_conflict"
	case errors.Is(err, authapi.ErrUserInactive):
		return "user_inactive"
	case errors.Is(err, authapi.ErrOAuthEmailUnverified):
		return "email_unverified"
	default:
		log.Ctx(c.Request.Context()).Error().Err(err).Msg("google oauth callback failed")
		return "server_error"
	}
}

func (h *GoogleOAuthHandler) mapOAuthError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, authapi.ErrOAuthCodeInvalid):
		httpx.Error(c, http.StatusBadRequest, httpx.CodeOAuthCodeInvalid, "Kode login Google tidak valid atau sudah kedaluwarsa")
	case errors.Is(err, authapi.ErrPhoneAlreadyUsed):
		httpx.Error(c, http.StatusConflict, httpx.CodePhoneAlreadyUsed, "Nomor WA sudah terdaftar")
	case errors.Is(err, authapi.ErrPhoneVerificationRequired):
		httpx.Error(c, http.StatusConflict, httpx.CodePhoneVerificationRequired,
			"Nomor sudah digunakan, pakai nomor lain — atau verifikasi kalau ini memang nomormu")
	case errors.Is(err, authapi.ErrEmailAlreadyUsed):
		httpx.Error(c, http.StatusConflict, httpx.CodeEmailAlreadyUsed, "Email sudah digunakan akun lain")
	case errors.Is(err, authapi.ErrOAuthStaffNotAllowed):
		httpx.Error(c, http.StatusForbidden, httpx.CodeForbidden, "Akun staff wajib login dengan email dan kata sandi")
	case errors.Is(err, authapi.ErrUserInactive):
		httpx.Error(c, http.StatusForbidden, httpx.CodeForbidden, "Akun dinonaktifkan")
	case errors.Is(err, authapi.ErrUserNotFound):
		httpx.Error(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "Sesi tidak ditemukan")
	case errors.Is(err, authapi.ErrOAuthEmailUnverified):
		httpx.Error(c, http.StatusForbidden, httpx.CodeForbidden, "Email Google belum terverifikasi, gunakan akun Google dengan email terverifikasi")
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
	case service.IsBadInput(err):
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
	default:
		log.Ctx(c.Request.Context()).Error().Err(err).Msg("google oauth error")
		httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
	}
}

func toGoogleExchangeResponse(out *service.GoogleExchangeOutput) googleExchangeResponse {
	// Re-validate — redirect_path traveled through a cookie then the DB
	// (oauth_login_codes.redirect_path) before landing here (review finding
	// #9); never trust it round-tripped unmodified.
	resp := googleExchangeResponse{Status: out.Status, RedirectPath: sanitizeRedirectPath(out.RedirectPath)}
	if out.Status == "session" && out.Token != nil && out.User != nil {
		email := ""
		if out.User.Email != nil {
			email = *out.User.Email
		}
		userPhone := ""
		if out.User.Phone != nil {
			userPhone = *out.User.Phone
		}
		resp.Token = &tokenResponse{
			AccessToken: out.Token.AccessToken,
			TokenType:   out.Token.TokenType,
			ExpiresIn:   int64(out.Token.ExpiresIn.Seconds()),
		}
		resp.User = &userSummary{
			ID:       out.User.ID.String(),
			Email:    email,
			Phone:    userPhone,
			Name:     out.User.Name,
			UserType: string(out.User.UserType),
		}
		return resp
	}
	resp.Code = out.Code
	resp.Email = out.Email
	resp.Name = out.Name
	return resp
}

// sanitizeRedirectPath — anti-open-redirect: only accept a same-origin
// absolute path. Anything else (empty, protocol-relative "//evil.com",
// backslash tricks some browsers normalize to "//") falls back to a safe
// default.
func sanitizeRedirectPath(raw string) string {
	const fallback = "/akun"
	if raw == "" {
		return fallback
	}
	if !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || strings.ContainsAny(raw, "\\") {
		return fallback
	}
	return raw
}

// splitStateCookie parses "<nonce>|<redirectPath>" — ok=false if malformed.
func splitStateCookie(raw string) (nonce, redirectPath string, ok bool) {
	parts := strings.SplitN(raw, "|", 2)
	if len(parts) != 2 || parts[0] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// randomNonce returns a 32-byte crypto-random value, base64url-encoded.
func randomNonce() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
