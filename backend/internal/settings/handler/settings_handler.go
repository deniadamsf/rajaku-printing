// Package handler — HTTP endpoints untuk modul settings (admin-only).
package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/httpx"
	"github.com/rajaku-printing/backend/internal/settings/service"
	"github.com/rajaku-printing/backend/internal/settings/settingsapi"
)

type Handler struct {
	svc *service.Service
}

func New(svc *service.Service) *Handler { return &Handler{svc: svc} }

func mapDomainErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, settingsapi.ErrSettingNotFound), errors.Is(err, settingsapi.ErrUnknownKey):
		httpx.Error(c, http.StatusNotFound, httpx.CodeNotFound, err.Error())
	case errors.Is(err, settingsapi.ErrInvalidValue):
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
	default:
		log.Ctx(c.Request.Context()).Error().Err(err).Msg("settings handler: unmapped error")
		httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal, "internal server error")
	}
}

// GET /admin/settings — daftar semua setting global.
func (h *Handler) List(c *gin.Context) {
	items, err := h.svc.List(c.Request.Context())
	if err != nil {
		mapDomainErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

type updateRequest struct {
	// Value dikirim sebagai string apa adanya; service yang parse & validasi
	// sesuai tipe key-nya (§23.3 validasi di edge + validasi domain di service).
	Value string `json:"value" binding:"required"`
}

// PUT /admin/settings/:key — ubah nilai satu setting.
func (h *Handler) Update(c *gin.Context) {
	id, err := authapi.IdentityFromContext(c.Request.Context())
	if err != nil || id == nil {
		httpx.Error(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}
	key := c.Param("key")
	if key == "" {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeBadRequest, "key required")
		return
	}
	var req updateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "value required")
		return
	}

	row, err := h.svc.Update(c.Request.Context(), key, req.Value, id.UserID)
	if err != nil {
		mapDomainErr(c, err)
		return
	}
	httpx.OK(c, row)
}
