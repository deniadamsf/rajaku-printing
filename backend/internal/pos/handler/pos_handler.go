// Package handler — HTTP endpoints modul POS (§11).
package handler

import (
	"errors"
	"net/http"
	"strings"
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
		errors.Is(err, orderapi.ErrInvalidDesignSource),
		errors.Is(err, orderapi.ErrNoItems),
		errors.Is(err, orderapi.ErrTooManyItems),
		errors.Is(err, discountapi.ErrDiscountAmbiguousInput),
		errors.Is(err, discountapi.ErrManualDiscountNoteRequired),
		errors.Is(err, discountapi.ErrManualDiscountInvalidAmount),
		errors.Is(err, discountapi.ErrDiscountMinSubtotal),
		errors.Is(err, discountapi.ErrDiscountScopeEmpty),
		errors.Is(err, discountapi.ErrDiscountProductMismatch),
		errors.Is(err, discountapi.ErrDiscountMemberScopeEmpty),
		errors.Is(err, discountapi.ErrDiscountMemberMismatch):
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
	case errors.Is(err, discountapi.ErrDiscountNotFound):
		httpx.Error(c, http.StatusNotFound, httpx.CodeNotFound, err.Error())
	case errors.Is(err, discountapi.ErrDiscountInactive),
		errors.Is(err, discountapi.ErrDiscountNotStarted),
		errors.Is(err, discountapi.ErrDiscountExpired),
		errors.Is(err, discountapi.ErrDiscountChannelMismatch),
		errors.Is(err, discountapi.ErrDiscountQuotaExhausted),
		errors.Is(err, discountapi.ErrDiscountMembershipDisabled),
		errors.Is(err, discountapi.ErrDiscountMembershipRequired):
		httpx.Error(c, http.StatusUnprocessableEntity, httpx.CodeUnprocessable, err.Error())
	case errors.Is(err, authapi.ErrCustomerBlocked):
		httpx.Error(c, http.StatusForbidden, httpx.CodeForbidden,
			"pelanggan ini diblokir, tidak bisa membuat order baru — aktifkan dulu di Manajemen Pelanggan")
	case errors.Is(err, posapi.ErrCustomerResolve):
		httpx.Error(c, http.StatusUnprocessableEntity, httpx.CodeUnprocessable, err.Error())
	case errors.Is(err, orderapi.ErrDiscountUnavailable),
		errors.Is(err, discountapi.ErrDiscountMembershipUnavailable):
		httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal, err.Error())
	case errors.Is(err, posapi.ErrOrderCreate),
		errors.Is(err, orderapi.ErrResiCollisionGaveUp):
		httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal, err.Error())
	default:
		log.Ctx(c.Request.Context()).Error().Err(err).Msg("pos handler: unmapped error")
		httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal, "internal server error")
	}
}

// posOrderItemRequest — satu baris produk untuk createOrderBody (§32).
// DesignSource/DesignBrief/ItemNotes sekarang PER ITEM (§32.5) — pola sama
// dengan orderItemRequest (order/handler/dto.go), plus item_notes yang
// khusus kasir POS.
type posOrderItemRequest struct {
	ProductID  uuid.UUID `json:"product_id"  binding:"required"`
	MaterialID uuid.UUID `json:"material_id" binding:"required"`
	WidthCm    int       `json:"width_cm"    binding:"required,gt=0"`
	HeightCm   int       `json:"height_cm"   binding:"required,gt=0"`
	Quantity   int       `json:"quantity"    binding:"omitempty,gt=0"`

	DesignSource string `json:"design_source" binding:"required,oneof=upload request"`
	DesignBrief  string `json:"design_brief"  binding:"omitempty,max=2000"`
	ItemNotes    string `json:"item_notes"    binding:"omitempty,max=1000"`
}

type createOrderBody struct {
	CustomerName  string `json:"customer_name"`
	CustomerPhone string `json:"customer_phone"`

	// Items — 1..20 baris produk (§32.4). Shape divalidasi di sini
	// (binding), aturan bisnis (quote katalog, dll) tetap di service (§22,
	// §23.3).
	Items []posOrderItemRequest `json:"items" binding:"required,min=1,max=20,dive"`

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

	// DesignApprovalMode tetap PER ORDER (§11, §32.5).
	DesignApprovalMode string `json:"design_approval_mode"`

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
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
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

	items := make([]service.CreateOrderItemInput, 0, len(body.Items))
	for _, it := range body.Items {
		items = append(items, service.CreateOrderItemInput{
			ProductID:    it.ProductID,
			MaterialID:   it.MaterialID,
			WidthCm:      it.WidthCm,
			HeightCm:     it.HeightCm,
			Quantity:     it.Quantity,
			DesignSource: it.DesignSource,
			DesignBrief:  it.DesignBrief,
			ItemNotes:    it.ItemNotes,
		})
	}

	result, err := h.svc.CreateOrder(c.Request.Context(), service.CreateOrderInput{
		KasirID:                id.UserID,
		CustomerName:           body.CustomerName,
		CustomerPhone:          body.CustomerPhone,
		Items:                  items,
		MetodeAmbil:            body.MetodeAmbil,
		ShippingAddress:        body.ShippingAddress,
		ShippingRecipientName:  body.ShippingRecipientName,
		ShippingRecipientPhone: body.ShippingRecipientPhone,
		ShippingCost:           body.ShippingCost,
		DiscountID:             discountID,
		ManualDiscountAmount:   body.ManualDiscountAmount,
		DiscountNote:           body.DiscountNote,
		MetodeBayar:            body.MetodeBayar,
		DesignApprovalMode:     body.DesignApprovalMode,
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

// GET /admin/pos/customers/search?q=... — cari pelanggan existing by
// nama/WA, dipakai kasir supaya tidak input ulang data pelanggan yang sudah
// pernah order (online maupun walk-in) — nomor WA tetap matching key
// tunggal (§11).
func (h *Handler) SearchCustomers(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	results, err := h.svc.SearchCustomers(c.Request.Context(), q)
	if err != nil {
		log.Ctx(c.Request.Context()).Error().Err(err).Msg("pos handler: search customers gagal")
		httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal, "internal server error")
		return
	}
	httpx.OK(c, results)
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
