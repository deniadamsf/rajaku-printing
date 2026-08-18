package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/auth/password"
	"github.com/rajaku-printing/backend/internal/auth/service"
	"github.com/rajaku-printing/backend/internal/httpx"
	"github.com/rajaku-printing/backend/internal/pkg/phone"
)

type Handler struct {
	svc *service.Service
}

func New(svc *service.Service) *Handler { return &Handler{svc: svc} }

// POST /api/v1/auth/register — customer registration.
func (h *Handler) RegisterCustomer(c *gin.Context) {
	var req registerCustomerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
		return
	}

	pair, user, err := h.svc.RegisterCustomer(c.Request.Context(), service.RegisterCustomerInput{
		Email:    req.Email,
		Phone:    req.Phone,
		Name:     req.Name,
		Password: req.Password,
	})
	if err != nil {
		h.mapServiceError(c, err)
		return
	}

	email := ""
	if user.Email != nil {
		email = *user.Email
	}
	userPhone := ""
	if user.Phone != nil {
		userPhone = *user.Phone
	}
	httpx.Created(c, registerResponse{
		Token: tokenResponse{
			AccessToken: pair.AccessToken,
			TokenType:   pair.TokenType,
			ExpiresIn:   int64(pair.ExpiresIn.Seconds()),
		},
		User: userSummary{
			ID:       user.ID.String(),
			Email:    email,
			Phone:    userPhone,
			Name:     user.Name,
			UserType: string(user.UserType),
		},
	})
}

// POST /api/v1/auth/login — unified login (customer + staff pakai form yang
// sama, spec section 3, 10). Frontend redirect berdasarkan user_type + roles
// dari response.
func (h *Handler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
		return
	}
	pair, user, err := h.svc.Login(c.Request.Context(), service.LoginInput{
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		h.mapServiceError(c, err)
		return
	}

	email := ""
	if user.Email != nil {
		email = *user.Email
	}
	userPhone := ""
	if user.Phone != nil {
		userPhone = *user.Phone
	}
	httpx.OK(c, registerResponse{
		Token: tokenResponse{
			AccessToken: pair.AccessToken,
			TokenType:   pair.TokenType,
			ExpiresIn:   int64(pair.ExpiresIn.Seconds()),
		},
		User: userSummary{
			ID:       user.ID.String(),
			Email:    email,
			Phone:    userPhone,
			Name:     user.Name,
			UserType: string(user.UserType),
		},
	})
}

// GET /api/v1/auth/me — return current user profile + roles + permissions.
// Requires RequireAuth middleware upstream.
func (h *Handler) Me(c *gin.Context) {
	id, err := authapi.IdentityFromContext(c.Request.Context())
	if err != nil {
		httpx.Error(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}
	me, err := h.svc.GetMe(c.Request.Context(), id.UserID)
	if err != nil {
		if errors.Is(err, authapi.ErrUserNotFound) {
			httpx.Error(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "user no longer exists")
			return
		}
		log.Ctx(c.Request.Context()).Error().Err(err).Msg("get me failed")
		httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
		return
	}
	httpx.OK(c, me)
}

// POST /api/v1/auth/logout — stateless JWT, jadi endpoint hanya konfirmasi
// (client drop token-nya). TODO(auth): saat refresh-token diimplement,
// invalidate refresh token di DB di sini.
func (h *Handler) Logout(c *gin.Context) {
	httpx.NoContent(c)
}

// mapServiceError translates known service/domain errors to HTTP responses.
// Any error not matched falls through to a generic 500 (with structured log).
func (h *Handler) mapServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, authapi.ErrEmailAlreadyUsed):
		httpx.Error(c, http.StatusConflict, httpx.CodeConflict, "email sudah terdaftar")
	case errors.Is(err, authapi.ErrPhoneAlreadyUsed):
		httpx.Error(c, http.StatusConflict, httpx.CodeConflict, "nomor WA sudah terdaftar")
	case errors.Is(err, authapi.ErrInvalidPassword):
		// Generic message — jangan bocor apakah email/password yg salah.
		httpx.Error(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "email atau password salah")
	case errors.Is(err, authapi.ErrUserInactive):
		httpx.Error(c, http.StatusForbidden, httpx.CodeForbidden, "akun dinonaktifkan")
	case errors.Is(err, password.ErrTooShort), errors.Is(err, password.ErrTooLong):
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
	case errors.Is(err, phone.ErrEmpty),
		errors.Is(err, phone.ErrInvalidPrefix),
		errors.Is(err, phone.ErrInvalidLength),
		errors.Is(err, phone.ErrContainsLetters):
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
	case service.IsBadInput(err):
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
	default:
		log.Ctx(c.Request.Context()).Error().Err(err).Msg("auth service error")
		httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
	}
}
