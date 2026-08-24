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
	"github.com/rajaku-printing/backend/internal/discount/discountapi"
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
		errors.Is(err, orderapi.ErrInvalidShippingCost),
		errors.Is(err, discountapi.ErrDiscountAmbiguousInput),
		errors.Is(err, discountapi.ErrManualDiscountNoteRequired),
		errors.Is(err, discountapi.ErrManualDiscountInvalidAmount),
		errors.Is(err, discountapi.ErrDiscountMinSubtotal):
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
	case errors.Is(err, discountapi.ErrDiscountNotFound):
		httpx.Error(c, http.StatusNotFound, httpx.CodeNotFound, err.Error())
	case errors.Is(err, discountapi.ErrDiscountInactive),
		errors.Is(err, discountapi.ErrDiscountNotStarted),
		errors.Is(err, discountapi.ErrDiscountExpired),
		errors.Is(err, discountapi.ErrDiscountChannelMismatch),
		errors.Is(err, discountapi.ErrDiscountQuotaExhausted):
		httpx.Error(c, http.StatusUnprocessableEntity, httpx.CodeUnprocessable, err.Error())
	case errors.Is(err, posapi.ErrCustomerResolve):
		httpx.Error(c, http.StatusUnprocessableEntity, httpx.CodeUnprocessable, err.Error())
	case errors.Is(err, orderapi.ErrDiscountUnavailable):
		httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal, err.Error())
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

	// Discount (§28) — mutually exclusive: DiscountID (master) ATAU
	// ManualDiscountAmount+DiscountNote (manual). Kosong semua = tanpa
	// diskon. Wajib permission discount.apply (dicek di handler, bukan
	// route — lihat CreateOrder).
	DiscountID string `json:"discount_id"`
	// Review finding #6 — binding gte=0 menolak nominal negatif di edge
	// (§22), sebelum sempat lolos ke pengecekan requestsDiscount di bawah.
	ManualDiscountAmount int64  `json:"manual_discount_amount" binding:"gte=0"`
	DiscountNote         string `json:"discount_note"`

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

	// Diskon (§28) — dicek DI HANDLER (bukan route middleware, karena
	// aturannya "hanya kalau body membawa diskon", bukan blanket per-route):
	// kasir tanpa permission discount.apply TIDAK BOLEH menyertakan
	// discount_id ATAU manual_discount_amount sama sekali.
	// Review finding #6 — `!= 0` (bukan `> 0`) supaya nominal negatif (kalau
	// entah bagaimana lolos binding gte=0 di atas) tetap dianggap "meminta
	// diskon" dan masuk jalur permission+validasi, bukan diam-diam dilewati.
	requestsDiscount := body.DiscountID != "" || body.ManualDiscountAmount != 0
	if requestsDiscount && !id.HasPermission("discount.apply") {
		httpx.Error(c, http.StatusForbidden, httpx.CodeForbidden,
			"missing permission: discount.apply")
		return
	}
	var discountID *uuid.UUID
	if body.DiscountID != "" {
		parsed, derr := uuid.Parse(body.DiscountID)
		if derr != nil {
			httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid discount_id")
			return
		}
		discountID = &parsed
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
		DiscountID:             discountID,
		ManualDiscountAmount:   body.ManualDiscountAmount,
		DiscountNote:           body.DiscountNote,
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

// GET /admin/pos/receipt-config — lebar kertas struk aktif + daftar yang
// didukung. Config display, bukan data kritis: kegagalan baca setting sudah
// ditangani service (jatuh ke default), jadi endpoint ini selalu 200.
func (h *Handler) ReceiptConfig(c *gin.Context) {
	cfg := h.svc.ReceiptConfig(c.Request.Context())
	httpx.OK(c, cfg)
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
