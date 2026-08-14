package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/auth/service"
	"github.com/rajaku-printing/backend/internal/httpx"
	"github.com/rajaku-printing/backend/internal/pkg/phone"
)

// GuestOrderHandler exposes the guest-checkout ownership-proof endpoint —
// separate from Handler (login/register/me) to keep each handler focused on
// one concern (§22).
type GuestOrderHandler struct {
	svc *service.GuestOrderService
}

func NewGuestOrderHandler(svc *service.GuestOrderService) *GuestOrderHandler {
	return &GuestOrderHandler{svc: svc}
}

// VerifyOwnership — POST /lacak/:resi/verify (public, rate-limited far more
// strictly than the regular public group — see internal/server/router.go).
//
// Body: {"phone": "08xxxxxxxxxx"}
// 200:  {"token": "<jwt>", "expires_at": "<RFC3339 UTC>"}
// 401:  identical message whether resi doesn't exist OR phone doesn't match
// (never disclose which resi numbers are valid).
func (h *GuestOrderHandler) VerifyOwnership(c *gin.Context) {
	resi := c.Param("resi")
	if resi == "" {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeBadRequest, "resi required")
		return
	}

	var req guestVerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
		return
	}

	out, err := h.svc.VerifyOwnership(c.Request.Context(), resi, req.Phone)
	if err != nil {
		h.mapVerifyError(c, err)
		return
	}

	httpx.OK(c, guestVerifyResponse{
		Token:     out.AccessToken,
		ExpiresAt: out.ExpiresAt.Format(time.RFC3339),
	})
}

func (h *GuestOrderHandler) mapVerifyError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, authapi.ErrGuestVerificationFailed):
		httpx.Error(c, http.StatusUnauthorized, httpx.CodeUnauthorized,
			"Nomor WhatsApp tidak cocok dengan pesanan ini.")
	case errors.Is(err, phone.ErrEmpty),
		errors.Is(err, phone.ErrInvalidPrefix),
		errors.Is(err, phone.ErrInvalidLength),
		errors.Is(err, phone.ErrContainsLetters):
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
	default:
		log.Ctx(c.Request.Context()).Error().Err(err).Msg("guest order verification failed")
		httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
	}
}
