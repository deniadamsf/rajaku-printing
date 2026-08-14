// Package handler — HTTP endpoints untuk modul production. Semua staff-only.
package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/httpx"
	"github.com/rajaku-printing/backend/internal/production/productionapi"
	"github.com/rajaku-printing/backend/internal/production/service"
)

type Handler struct {
	svc *service.Service
}

func New(svc *service.Service) *Handler { return &Handler{svc: svc} }

func mapDomainErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, productionapi.ErrOrderNotFound):
		httpx.Error(c, http.StatusNotFound, httpx.CodeNotFound, err.Error())
	case errors.Is(err, productionapi.ErrShippingTrackingRequired),
		errors.Is(err, productionapi.ErrDikirimOnPickup):
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
	case errors.Is(err, productionapi.ErrInvalidTransition):
		httpx.Error(c, http.StatusConflict, httpx.CodeConflict, err.Error())
	case errors.Is(err, productionapi.ErrOrderStateChanged):
		httpx.Error(c, http.StatusConflict, httpx.CodeConflict, err.Error())
	default:
		log.Ctx(c.Request.Context()).Error().Err(err).Msg("production handler: unmapped error")
		httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal, "internal server error")
	}
}

// noteBody — shared body untuk semua advance endpoints.
type noteBody struct {
	Note string `json:"note"`
}

func (h *Handler) StartCetak(c *gin.Context) {
	staffID, ok := requireStaff(c)
	if !ok {
		return
	}
	var b noteBody
	_ = c.ShouldBindJSON(&b)
	if err := h.svc.StartCetak(c.Request.Context(), service.AdvanceInput{
		Resi: c.Param("resi"), StaffID: staffID, Note: b.Note,
	}); err != nil {
		mapDomainErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func (h *Handler) StartQC(c *gin.Context) {
	staffID, ok := requireStaff(c)
	if !ok {
		return
	}
	var b noteBody
	_ = c.ShouldBindJSON(&b)
	if err := h.svc.StartQC(c.Request.Context(), service.AdvanceInput{
		Resi: c.Param("resi"), StaffID: staffID, Note: b.Note,
	}); err != nil {
		mapDomainErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func (h *Handler) MarkSiap(c *gin.Context) {
	staffID, ok := requireStaff(c)
	if !ok {
		return
	}
	var b noteBody
	_ = c.ShouldBindJSON(&b)
	if err := h.svc.MarkSiap(c.Request.Context(), service.AdvanceInput{
		Resi: c.Param("resi"), StaffID: staffID, Note: b.Note,
	}); err != nil {
		mapDomainErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func (h *Handler) MarkShipped(c *gin.Context) {
	staffID, ok := requireStaff(c)
	if !ok {
		return
	}
	var body struct {
		Courier        string `json:"courier"`
		TrackingNumber string `json:"tracking_number"`
		Note           string `json:"note"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid json body")
		return
	}
	if err := h.svc.MarkShipped(c.Request.Context(), service.MarkShippedInput{
		Resi:           c.Param("resi"),
		StaffID:        staffID,
		Courier:        body.Courier,
		TrackingNumber: body.TrackingNumber,
		Note:           body.Note,
	}); err != nil {
		mapDomainErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func (h *Handler) MarkSelesai(c *gin.Context) {
	staffID, ok := requireStaff(c)
	if !ok {
		return
	}
	var b noteBody
	_ = c.ShouldBindJSON(&b)
	if err := h.svc.MarkSelesai(c.Request.Context(), service.AdvanceInput{
		Resi: c.Param("resi"), StaffID: staffID, Note: b.Note,
	}); err != nil {
		mapDomainErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func requireStaff(c *gin.Context) (uuid.UUID, bool) {
	id, err := authapi.IdentityFromContext(c.Request.Context())
	if err != nil || id == nil {
		httpx.Error(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return uuid.Nil, false
	}
	return id.UserID, true
}
