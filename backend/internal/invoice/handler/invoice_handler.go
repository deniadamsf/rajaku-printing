// Package handler — HTTP endpoints modul invoice.
package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/httpx"
	"github.com/rajaku-printing/backend/internal/invoice/invoiceapi"
	"github.com/rajaku-printing/backend/internal/invoice/service"
)

type Handler struct {
	svc *service.Service
}

func New(svc *service.Service) *Handler { return &Handler{svc: svc} }

func mapDomainErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, invoiceapi.ErrInvoiceNotFound),
		errors.Is(err, invoiceapi.ErrOrderNotFound):
		httpx.Error(c, http.StatusNotFound, httpx.CodeNotFound, err.Error())
	case errors.Is(err, invoiceapi.ErrOrderNotInvoiceable):
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
	case errors.Is(err, invoiceapi.ErrInvoiceAlreadyExists):
		httpx.Error(c, http.StatusConflict, httpx.CodeConflict, err.Error())
	default:
		log.Ctx(c.Request.Context()).Error().Err(err).Msg("invoice handler: unmapped error")
		httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal, "internal server error")
	}
}

// GET /invoices/:id/pdf — public download. UUID unguessable = de-facto signed link
// (spec §12: link download dikirim ke customer via WA; siapa punya link bisa download).
func (h *Handler) DownloadPDF(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid invoice id")
		return
	}
	handle, err := h.svc.GetFile(c.Request.Context(), id)
	if err != nil {
		mapDomainErr(c, err)
		return
	}
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", `inline; filename="`+handle.Invoice.InvoiceNumber+`.pdf"`)
	c.File(handle.AbsPath)
}

// POST /admin/orders/:resi/invoice — staff manual trigger generate (idempotent).
// Butuh permission order.update_status (staff yg atur order boleh).
func (h *Handler) AdminGenerate(c *gin.Context) {
	id, err := authapi.IdentityFromContext(c.Request.Context())
	if err != nil || id == nil {
		httpx.Error(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}
	// Resolve resi via orderCmd — kita minta payload berupa orderID langsung
	// untuk hindari extra lookup. Kalau caller punya resi saja, dia bisa
	// hit GET order dulu.
	var body struct {
		OrderID string `json:"order_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid json body")
		return
	}
	orderID, err := uuid.Parse(body.OrderID)
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid order_id")
		return
	}
	staff := id.UserID
	info, err := h.svc.GenerateForOrder(c.Request.Context(), orderID, &staff)
	if err != nil {
		mapDomainErr(c, err)
		return
	}
	httpx.OK(c, info)
}
