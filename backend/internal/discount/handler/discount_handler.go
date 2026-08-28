// Package handler contains the HTTP layer for the discount module (§28).
// Handlers only bind/validate input and call the service — never touch
// *gorm.DB directly (§22).
package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/discount/discountapi"
	"github.com/rajaku-printing/backend/internal/discount/service"
	"github.com/rajaku-printing/backend/internal/httpx"
)

type Handler struct {
	svc *service.Service
}

func New(svc *service.Service) *Handler { return &Handler{svc: svc} }

// GET /admin/discounts — list + filter (§28.8).
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
		Query:   c.Query("q"),
		Page:    page,
		PerPage: perPage,
	})
	if err != nil {
		h.mapErr(c, err)
		return
	}
	httpx.OK(c, res)
}

// POST /admin/discounts — buat master diskon baru.
func (h *Handler) Create(c *gin.Context) {
	var req createDiscountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
		return
	}
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	view, err := h.svc.Create(c.Request.Context(), service.CreateInput{
		Code:              req.Code,
		Name:              req.Name,
		Type:              req.Type,
		ValuePercent:      req.ValuePercent,
		ValueAmount:       req.ValueAmount,
		MaxDiscountAmount: req.MaxDiscountAmount,
		MinSubtotal:       req.MinSubtotal,
		StartsAt:          req.StartsAt,
		EndsAt:            req.EndsAt,
		Quota:             req.Quota,
		ChannelScope:      req.ChannelScope,
		IsActive:          isActive,
		AppliesTo:         req.AppliesTo,
		ProductIDs:        req.ProductIDs,
		AudienceScope:     req.AudienceScope,
		MemberScope:       req.MemberScope,
		CustomerIDs:       req.CustomerIDs,
	})
	if err != nil {
		h.mapErr(c, err)
		return
	}
	httpx.Created(c, view)
}

// GET /admin/discounts/applicable?channel=pos&subtotal=250000&product_id=...&customer_id=...
// — dipakai layar kasir (§28.5/§28.8/§30.3). Permission BERBEDA dari CRUD
// lain (discount.apply, bukan discount.manage) — dicek di routes.go.
//
// product_id (§28.9) opsional — kalau dikirim, WAJIB UUID valid DAN bukan
// UUID kosong (temuan review #4 — 00000000-...-0000 ditolak 400, supaya
// tidak diam-diam disamakan dengan "tidak dikirim"), lalu hasilnya disaring
// supaya diskon applies_to="selected" yang cakupannya tidak menyertakan
// produk itu tidak ikut muncul. Tidak dikirim sama sekali = tidak difilter
// berdasarkan produk (kompatibel dgn pemanggil lama).
//
// customer_id (§30.3) opsional — validasi sintaks SAMA persis dengan
// product_id (UUID valid, bukan UUID kosong kalau dikirim eksplisit). Tidak
// dikirim sama sekali = diskon audience_scope="member" TIDAK ikut muncul di
// hasil (§30.3 — beda dari product_id, di sini "tidak dikirim" TIDAK
// membuat diskon member lolos, karena tanpa customer_id tidak ada cara
// mengecek status membernya).
func (h *Handler) Applicable(c *gin.Context) {
	channel := c.Query("channel")
	if channel != "online" && channel != "pos" {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "channel wajib 'online' atau 'pos'")
		return
	}
	subtotal, err := strconv.ParseInt(c.Query("subtotal"), 10, 64)
	if err != nil || subtotal < 0 {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "subtotal wajib angka >= 0")
		return
	}
	productID, err := parseOptionalUUIDQuery(c, "product_id")
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
		return
	}
	customerID, err := parseOptionalUUIDQuery(c, "customer_id")
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
		return
	}
	views, err := h.svc.Applicable(c.Request.Context(), channel, subtotal, productID, customerID)
	if err != nil {
		h.mapErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": views})
}

// parseOptionalUUIDQuery reads an optional UUID query param — "" (not sent)
// returns uuid.Nil with no error; sent-but-invalid or sent-as-the-zero-UUID
// (00000000-...-0000, syntactically valid but semantically "no id" — temuan
// review #4) both return an error, so callers never silently conflate
// "not sent" with "sent as nil" (shared by product_id §28.9 & customer_id
// §30.3, which both use uuid.Nil as their own "not filtering" sentinel).
func parseOptionalUUIDQuery(c *gin.Context, param string) (uuid.UUID, error) {
	raw := c.Query(param)
	if raw == "" {
		return uuid.Nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, fmt.Errorf("%s bukan UUID valid", param)
	}
	if id == uuid.Nil {
		return uuid.Nil, fmt.Errorf("%s tidak boleh UUID kosong", param)
	}
	return id, nil
}

// GET /admin/discounts/:id — detail.
func (h *Handler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "id bukan UUID valid")
		return
	}
	view, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		h.mapErr(c, err)
		return
	}
	httpx.OK(c, view)
}

// PATCH /admin/discounts/:id — ubah master diskon.
func (h *Handler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "id bukan UUID valid")
		return
	}
	raw := map[string]json.RawMessage{}
	if err := c.ShouldBindJSON(&raw); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
		return
	}
	in, err := parseUpdateInput(raw)
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
		return
	}
	view, err := h.svc.Update(c.Request.Context(), id, *in)
	if err != nil {
		h.mapErr(c, err)
		return
	}
	httpx.OK(c, view)
}

// DELETE /admin/discounts/:id — soft delete, reason wajib (§28.1/§28.2).
func (h *Handler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "id bukan UUID valid")
		return
	}
	var req deleteDiscountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
		return
	}
	identity, err := authapi.IdentityFromContext(c.Request.Context())
	if err != nil || identity == nil {
		httpx.Error(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id, identity.UserID, req.Reason); err != nil {
		h.mapErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"id": id, "deleted": true})
}

func (h *Handler) mapErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, discountapi.ErrDiscountNotFound):
		httpx.Error(c, http.StatusNotFound, httpx.CodeNotFound, "diskon tidak ditemukan")
	case errors.Is(err, discountapi.ErrDiscountCodeRequired),
		errors.Is(err, discountapi.ErrDiscountNameRequired),
		errors.Is(err, discountapi.ErrDiscountTypeInvalid),
		errors.Is(err, discountapi.ErrDiscountValuePercentInvalid),
		errors.Is(err, discountapi.ErrDiscountValueAmountInvalid),
		errors.Is(err, discountapi.ErrDiscountChannelScopeInvalid),
		errors.Is(err, discountapi.ErrDiscountQuotaInvalid),
		errors.Is(err, discountapi.ErrDiscountMaxAmountInvalid),
		errors.Is(err, discountapi.ErrDiscountMaxAmountNotAllowed),
		errors.Is(err, discountapi.ErrDiscountMinSubtotalInvalid),
		errors.Is(err, discountapi.ErrDiscountInvalidPeriod),
		errors.Is(err, discountapi.ErrDiscountAppliesToInvalid),
		errors.Is(err, discountapi.ErrDiscountScopeEmpty),
		errors.Is(err, discountapi.ErrDiscountProductMismatch),
		errors.Is(err, discountapi.ErrDiscountProductNotFound),
		errors.Is(err, discountapi.ErrDiscountAudienceScopeInvalid),
		errors.Is(err, discountapi.ErrDiscountMemberScopeInvalid),
		errors.Is(err, discountapi.ErrDiscountMemberScopeEmpty),
		errors.Is(err, discountapi.ErrDiscountMemberMismatch),
		errors.Is(err, discountapi.ErrDiscountCustomerNotFound):
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
	case errors.Is(err, discountapi.ErrDiscountCodeConflict):
		httpx.Error(c, http.StatusConflict, httpx.CodeConflict,
			"code sudah dipakai diskon lain yang masih aktif")
	case errors.Is(err, discountapi.ErrDiscountDeleteReasonRequired):
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "alasan hapus wajib diisi")
	case errors.Is(err, discountapi.ErrDiscountMembershipDisabled),
		errors.Is(err, discountapi.ErrDiscountMembershipRequired):
		httpx.Error(c, http.StatusUnprocessableEntity, httpx.CodeUnprocessable, err.Error())
	case errors.Is(err, discountapi.ErrDiscountMembershipUnavailable):
		httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal, err.Error())
	default:
		log.Ctx(c.Request.Context()).Error().Err(err).Msg("discount handler: unmapped error")
		httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal, "internal server error")
	}
}
