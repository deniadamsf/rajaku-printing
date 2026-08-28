// Package handler contains the HTTP layer for the membership module (§30
// CLAUDE.md). Handlers only bind/validate input and call the service — never
// touch *gorm.DB directly (§22).
package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/httpx"
	"github.com/rajaku-printing/backend/internal/membership/membershipapi"
	"github.com/rajaku-printing/backend/internal/membership/service"
)

type Handler struct {
	svc *service.Service
}

func New(svc *service.Service) *Handler { return &Handler{svc: svc} }

// Apply implements POST /account/membership/apply (§30.2) — customer
// mengajukan diri jadi member. Identity diambil dari token (bukan body),
// sehingga customer tidak bisa mengajukan atas nama customer lain.
func (h *Handler) Apply(c *gin.Context) {
	identity, err := requireIdentity(c)
	if err != nil {
		return
	}
	view, err := h.svc.Apply(c.Request.Context(), identity.UserID)
	if err != nil {
		h.mapErr(c, err)
		return
	}
	httpx.OK(c, view)
}

// GetOwn implements GET /account/membership (§30.2) — customer membaca
// status membership dirinya sendiri. Tanpa ini, status cuma terbaca sekali
// di body respons mutasi (Apply/dst) lalu hilang begitu customer reload
// halaman akun.
func (h *Handler) GetOwn(c *gin.Context) {
	identity, err := requireIdentity(c)
	if err != nil {
		return
	}
	view, err := h.svc.Get(c.Request.Context(), identity.UserID)
	if err != nil {
		h.mapErr(c, err)
		return
	}
	httpx.OK(c, view)
}

// List implements GET /admin/membership — daftar + filter status (§30.2).
func (h *Handler) List(c *gin.Context) {
	page, err := httpx.ParseIntQuery(c, "page", 1)
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
		return
	}
	perPage, err := httpx.ParseIntQuery(c, "per_page", 0)
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
		return
	}
	res, err := h.svc.List(c.Request.Context(), service.ListFilter{
		Status:  c.Query("status"),
		Page:    page,
		PerPage: perPage,
	})
	if err != nil {
		h.mapErr(c, err)
		return
	}
	httpx.OK(c, res)
}

// Approve implements POST /admin/membership/:id/approve (§30.2) —
// pending -> active.
func (h *Handler) Approve(c *gin.Context) {
	id, ok := h.parseID(c)
	if !ok {
		return
	}
	identity, err := requireIdentity(c)
	if err != nil {
		return
	}
	view, err := h.svc.Approve(c.Request.Context(), id, identity.UserID)
	if err != nil {
		h.mapErr(c, err)
		return
	}
	httpx.OK(c, view)
}

// Reject implements POST /admin/membership/:id/reject (§30.2) —
// pending -> rejected, body {"reason": "..."} wajib.
func (h *Handler) Reject(c *gin.Context) {
	id, ok := h.parseID(c)
	if !ok {
		return
	}
	var req rejectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
		return
	}
	identity, err := requireIdentity(c)
	if err != nil {
		return
	}
	view, err := h.svc.Reject(c.Request.Context(), id, identity.UserID, req.Reason)
	if err != nil {
		h.mapErr(c, err)
		return
	}
	httpx.OK(c, view)
}

// Revoke implements POST /admin/membership/:id/revoke (§30.2) —
// active -> revoked, body {"reason": "..."} wajib.
func (h *Handler) Revoke(c *gin.Context) {
	id, ok := h.parseID(c)
	if !ok {
		return
	}
	var req revokeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
		return
	}
	identity, err := requireIdentity(c)
	if err != nil {
		return
	}
	view, err := h.svc.Revoke(c.Request.Context(), id, identity.UserID, req.Reason)
	if err != nil {
		h.mapErr(c, err)
		return
	}
	httpx.OK(c, view)
}

// Reinstate implements POST /admin/membership/:id/reinstate (§30.2) —
// revoked -> active, body {"reason": "..."} wajib.
func (h *Handler) Reinstate(c *gin.Context) {
	id, ok := h.parseID(c)
	if !ok {
		return
	}
	var req reinstateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
		return
	}
	identity, err := requireIdentity(c)
	if err != nil {
		return
	}
	view, err := h.svc.Reinstate(c.Request.Context(), id, identity.UserID, req.Reason)
	if err != nil {
		h.mapErr(c, err)
		return
	}
	httpx.OK(c, view)
}

func (h *Handler) parseID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "id bukan UUID valid")
		return uuid.Nil, false
	}
	return id, true
}

// requireIdentity writes the 401 response itself on failure so callers can
// just check err != nil and return.
func requireIdentity(c *gin.Context) (*authapi.Identity, error) {
	identity, err := authapi.IdentityFromContext(c.Request.Context())
	if err != nil || identity == nil {
		httpx.Error(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		if err == nil {
			err = errors.New("identity missing from context")
		}
		return nil, err
	}
	return identity, nil
}

func (h *Handler) mapErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, membershipapi.ErrMembershipCustomerNotFound):
		httpx.Error(c, http.StatusNotFound, httpx.CodeNotFound, "customer tidak ditemukan")
	case errors.Is(err, membershipapi.ErrMembershipDisabled):
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "fitur membership sedang nonaktif")
	case errors.Is(err, membershipapi.ErrMembershipRequiresRegisteredAccount):
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "hanya akun terdaftar yang bisa mengajukan membership")
	case errors.Is(err, membershipapi.ErrMembershipInvalidTransition):
		httpx.Error(c, http.StatusConflict, httpx.CodeConflict, "transisi status membership tidak valid dari status saat ini")
	case errors.Is(err, membershipapi.ErrMembershipReasonRequired):
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "alasan wajib diisi")
	case errors.Is(err, membershipapi.ErrMembershipInvalidStatusFilter):
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "status filter tidak valid")
	default:
		log.Ctx(c.Request.Context()).Error().Err(err).Msg("membership handler: unmapped error")
		httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal, "internal server error")
	}
}
