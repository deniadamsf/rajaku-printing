// Package handler — HTTP endpoints modul POS (§11).
package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/httpx"
	"github.com/rajaku-printing/backend/internal/order/orderapi"
	"github.com/rajaku-printing/backend/internal/pos/posapi"
	"github.com/rajaku-printing/backend/internal/pos/service"
)

type Handler struct {
	svc *service.Service
}

func New(svc *service.Service) *Handler { return &Handler{svc: svc} }

func mapDomainErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, posapi.ErrInvalidPhone),
		errors.Is(err, posapi.ErrInvalidMetodeBayar),
		errors.Is(err, posapi.ErrInvalidDate),
		errors.Is(err, orderapi.ErrShippingFieldsRequired),
		errors.Is(err, orderapi.ErrInvalidShippingCost):
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
	case errors.Is(err, posapi.ErrCustomerResolve):
		httpx.Error(c, http.StatusUnprocessableEntity, httpx.CodeUnprocessable, err.Error())
	case errors.Is(err, posapi.ErrOrderCreate),
		errors.Is(err, orderapi.ErrResiCollisionGaveUp):
		httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal, err.Error())
	default:
		log.Ctx(c.Request.Context()).Error().Err(err).Msg("pos handler: unmapped error")
		httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal, "internal server error")
	}
}

type createOrderBody struct {
	CustomerName  string `json:"customer_name"`
	CustomerPhone string `json:"customer_phone"`

	ProductID  string `json:"product_id"`
	MaterialID string `json:"material_id"`
	WidthCm    int    `json:"width_cm"`
	HeightCm   int    `json:"height_cm"`
	Quantity   int    `json:"quantity"`

	MetodeAmbil            string `json:"metode_ambil"`
	ShippingAddress        string `json:"shipping_address"`
	ShippingRecipientName  string `json:"shipping_recipient_name"`
	ShippingRecipientPhone string `json:"shipping_recipient_phone"`
	ShippingCost           int64  `json:"shipping_cost"`

	MetodeBayar string `json:"metode_bayar"`

	DesignSource       string `json:"design_source"`
	DesignApprovalMode string `json:"design_approval_mode"`
	DesignBrief        string `json:"design_brief"`

	Notes string `json:"notes"`
}

// POST /admin/pos/orders — kasir input order walk-in.
func (h *Handler) CreateOrder(c *gin.Context) {
	id, err := authapi.IdentityFromContext(c.Request.Context())
	if err != nil || id == nil {
		httpx.Error(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}
	var body createOrderBody
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid json body")
		return
	}
	productID, perr := uuid.Parse(body.ProductID)
	if perr != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid product_id")
		return
	}
	materialID, merr := uuid.Parse(body.MaterialID)
	if merr != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid material_id")
		return
	}

	result, err := h.svc.CreateOrder(c.Request.Context(), service.CreateOrderInput{
		KasirID:                id.UserID,
		CustomerName:           body.CustomerName,
		CustomerPhone:          body.CustomerPhone,
		ProductID:              productID,
		MaterialID:             materialID,
		WidthCm:                body.WidthCm,
		HeightCm:               body.HeightCm,
		Quantity:               body.Quantity,
		MetodeAmbil:            body.MetodeAmbil,
		ShippingAddress:        body.ShippingAddress,
		ShippingRecipientName:  body.ShippingRecipientName,
		ShippingRecipientPhone: body.ShippingRecipientPhone,
		ShippingCost:           body.ShippingCost,
		MetodeBayar:            body.MetodeBayar,
		DesignSource:           body.DesignSource,
		DesignApprovalMode:     body.DesignApprovalMode,
		DesignBrief:            body.DesignBrief,
		Notes:                  body.Notes,
	})
	if err != nil {
		mapDomainErr(c, err)
		return
	}
	httpx.Created(c, result)
}

// GET /admin/pos/reconciliation?date=YYYY-MM-DD — laporan harian.
func (h *Handler) Reconciliation(c *gin.Context) {
	dateStr := c.Query("date")
	if dateStr == "" {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "query param 'date' wajib (format YYYY-MM-DD)")
		return
	}
	// Parse sebagai WIB (spec §11 rekonsiliasi harian pakai zona lokal).
	loc, _ := time.LoadLocation("Asia/Jakarta")
	if loc == nil {
		loc = time.FixedZone("WIB", 7*3600)
	}
	d, err := time.ParseInLocation("2006-01-02", dateStr, loc)
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "date invalid; format YYYY-MM-DD")
		return
	}
	report, err := h.svc.DailyReconciliation(c.Request.Context(), d)
	if err != nil {
		mapDomainErr(c, err)
		return
	}
	httpx.OK(c, report)
}
