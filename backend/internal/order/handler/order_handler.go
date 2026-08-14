package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/catalog/catalogapi"
	"github.com/rajaku-printing/backend/internal/httpx"
	"github.com/rajaku-printing/backend/internal/order/model"
	"github.com/rajaku-printing/backend/internal/order/orderapi"
	"github.com/rajaku-printing/backend/internal/order/service"
	"github.com/rajaku-printing/backend/internal/pkg/phone"
)

type Handler struct {
	svc *service.Service
}

func New(svc *service.Service) *Handler { return &Handler{svc: svc} }

// POST /orders — create online order. Auth OPSIONAL:
//   - Kalau header Authorization Bearer valid → order tied ke identity.UserID.
//   - Kalau tidak, request body wajib punya guest_phone + guest_name.
func (h *Handler) CreateOnline(c *gin.Context) {
	var req createOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
		return
	}

	// Resolve caller — best-effort, no error if not authed (endpoint publik).
	var customerID *uuid.UUID
	if id, err := authapi.IdentityFromContext(c.Request.Context()); err == nil && id != nil {
		uid := id.UserID
		customerID = &uid
	} else {
		// Guest path — validate presence of guest fields.
		if req.GuestPhone == "" || req.GuestName == "" {
			httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation,
				"guest_phone dan guest_name wajib diisi kalau tidak login")
			return
		}
	}

	in := service.CreateOnlineOrderInput{
		GuestPhone:             req.GuestPhone,
		GuestName:              req.GuestName,
		ProductID:              req.ProductID,
		MaterialID:             req.MaterialID,
		WidthCm:                req.WidthCm,
		HeightCm:               req.HeightCm,
		Quantity:               req.Quantity,
		MetodeAmbil:            model.MetodeAmbil(req.MetodeAmbil),
		ShippingAddress:        req.ShippingAddress,
		ShippingRecipientName:  req.ShippingRecipientName,
		ShippingRecipientPhone: req.ShippingRecipientPhone,
		DesignSource:           model.DesignSource(req.DesignSource),
		DesignBrief:            req.DesignBrief,
		Notes:                  req.Notes,
	}
	if customerID != nil {
		in.CustomerID = customerID
	}

	order, err := h.svc.CreateOnlineOrder(c.Request.Context(), in)
	if err != nil {
		h.mapErr(c, err)
		return
	}
	httpx.Created(c, toOrderResponse(order))
}

// GET /orders — customer melihat daftar pesanannya sendiri. Auth required.
// Query params: status (opsional), page, page_size.
// Staff callers ditolak — mereka pakai /admin/orders.
func (h *Handler) ListMine(c *gin.Context) {
	id, err := authapi.IdentityFromContext(c.Request.Context())
	if err != nil || id == nil {
		httpx.Error(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}
	if id.UserType == authapi.UserTypeStaff {
		httpx.Error(c, http.StatusForbidden, httpx.CodeForbidden,
			"endpoint ini khusus customer — staff pakai /admin/orders")
		return
	}
	page, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("page_size"))
	res, err := h.svc.ListForCustomer(c.Request.Context(), service.CustomerListInput{
		CustomerID: id.UserID,
		Status:     c.Query("status"),
		Page:       page,
		PageSize:   pageSize,
	})
	if err != nil {
		h.mapErr(c, err)
		return
	}
	items := make([]orderResponse, 0, len(res.Items))
	for i := range res.Items {
		items = append(items, toOrderResponse(&res.Items[i]))
	}
	httpx.OK(c, gin.H{
		"items":     items,
		"total":     res.Total,
		"page":      res.Page,
		"page_size": res.PageSize,
	})
}

// GET /orders/:resi — auth required. Customer only sees own order; staff sees all.
func (h *Handler) GetByResi(c *gin.Context) {
	resi := c.Param("resi")
	if resi == "" {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeBadRequest, "resi required")
		return
	}
	id, err := authapi.IdentityFromContext(c.Request.Context())
	if err != nil {
		httpx.Error(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}
	o, err := h.svc.GetByResiForOwner(c.Request.Context(), resi, id)
	if err != nil {
		h.mapErr(c, err)
		return
	}
	httpx.OK(c, toOrderResponse(o))
}

// GET /lacak/:resi — public tracking (censored).
func (h *Handler) PublicTracking(c *gin.Context) {
	resi := c.Param("resi")
	if resi == "" {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeBadRequest, "resi required")
		return
	}
	res, err := h.svc.GetByResiPublic(c.Request.Context(), resi)
	if err != nil {
		h.mapErr(c, err)
		return
	}
	httpx.OK(c, res)
}

func (h *Handler) mapErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, orderapi.ErrOrderNotFound):
		httpx.Error(c, http.StatusNotFound, httpx.CodeNotFound, "order tidak ditemukan")
	case errors.Is(err, orderapi.ErrNotOwner):
		httpx.Error(c, http.StatusForbidden, httpx.CodeForbidden, "tidak boleh mengakses order milik orang lain")
	case errors.Is(err, orderapi.ErrShippingFieldsRequired):
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation,
			"alamat, nama penerima, dan nomor WA penerima wajib diisi untuk metode kirim")
	case errors.Is(err, orderapi.ErrResiCollisionGaveUp):
		httpx.Error(c, http.StatusServiceUnavailable, httpx.CodeServiceUnavail,
			"gagal generate resi unik, coba beberapa saat lagi")
	// Catalog domain errors bubble via order service — map ke HTTP:
	case errors.Is(err, catalogapi.ErrProductNotFound):
		httpx.Error(c, http.StatusNotFound, httpx.CodeNotFound, "produk tidak ditemukan")
	case errors.Is(err, catalogapi.ErrMaterialNotFound):
		httpx.Error(c, http.StatusNotFound, httpx.CodeNotFound, "bahan tidak ditemukan")
	case errors.Is(err, catalogapi.ErrPricingNotAvailable):
		httpx.Error(c, http.StatusUnprocessableEntity, httpx.CodeUnprocessable,
			"harga belum tersedia untuk kombinasi produk & bahan ini")
	case errors.Is(err, catalogapi.ErrDimensionsOutOfRange):
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation,
			"ukuran di luar rentang yg didukung produk")
	case errors.Is(err, catalogapi.ErrDimensionsInvalid):
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "ukuran harus bilangan positif")
	// Phone normalize errors di guest path:
	case errors.Is(err, phone.ErrEmpty),
		errors.Is(err, phone.ErrInvalidPrefix),
		errors.Is(err, phone.ErrInvalidLength),
		errors.Is(err, phone.ErrContainsLetters):
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
	default:
		log.Ctx(c.Request.Context()).Error().Err(err).Msg("order service error")
		httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
	}
}
