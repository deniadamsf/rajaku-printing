// Package handler — admin CRUD endpoints untuk modul catalog (§9/§10).
//
// Semua endpoint di-gate permission catalog.manage; view read-only bisa dipakai
// tanpa permission karena endpoint list produk & bahan sudah publik.
package handler

import (
	"errors"
	"mime/multipart"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/rajaku-printing/backend/internal/catalog/catalogapi"
	"github.com/rajaku-printing/backend/internal/catalog/service"
	"github.com/rajaku-printing/backend/internal/httpx"
	"github.com/rajaku-printing/backend/internal/pkg/webp"
)

// -------- Materials ------------------------------------------------------

type materialRequest struct {
	Code        string `json:"code"        binding:"required"`
	Name        string `json:"name"        binding:"required"`
	Description string `json:"description"`
}

func (h *Handler) AdminListMaterials(c *gin.Context) {
	items, err := h.svc.AdminListMaterials(c.Request.Context())
	if err != nil {
		h.mapAdminErr(c, err)
		return
	}
	out := make([]materialAdminResponse, 0, len(items))
	for _, m := range items {
		out = append(out, toMaterialAdminResponse(m))
	}
	httpx.OK(c, gin.H{"materials": out})
}

func (h *Handler) AdminCreateMaterial(c *gin.Context) {
	var req materialRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
		return
	}
	m, err := h.svc.AdminCreateMaterial(c.Request.Context(), service.MaterialInput{
		Code:        req.Code,
		Name:        req.Name,
		Description: req.Description,
	})
	if err != nil {
		h.mapAdminErr(c, err)
		return
	}
	httpx.Created(c, toMaterialAdminResponse(*m))
}

func (h *Handler) AdminUpdateMaterial(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid id")
		return
	}
	var req materialRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
		return
	}
	m, err := h.svc.AdminUpdateMaterial(c.Request.Context(), id, service.MaterialInput{
		Code:        req.Code,
		Name:        req.Name,
		Description: req.Description,
	})
	if err != nil {
		h.mapAdminErr(c, err)
		return
	}
	httpx.OK(c, toMaterialAdminResponse(*m))
}

func (h *Handler) AdminActivateMaterial(c *gin.Context)   { h.setMaterialActive(c, true) }
func (h *Handler) AdminDeactivateMaterial(c *gin.Context) { h.setMaterialActive(c, false) }

func (h *Handler) setMaterialActive(c *gin.Context, active bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid id")
		return
	}
	if err := h.svc.AdminSetMaterialActive(c.Request.Context(), id, active); err != nil {
		h.mapAdminErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"ok": true, "is_active": active})
}

// -------- Products -------------------------------------------------------

type productRequest struct {
	Slug         string `json:"slug"          binding:"required"`
	Name         string `json:"name"          binding:"required"`
	Description  string `json:"description"`
	Category     string `json:"category"      binding:"required"`
	PricingType  string `json:"pricing_type"`
	MinWidthCm   *int   `json:"min_width_cm"`
	MinHeightCm  *int   `json:"min_height_cm"`
	MaxWidthCm   *int   `json:"max_width_cm"`
	MaxHeightCm  *int   `json:"max_height_cm"`
	DisplayOrder int    `json:"display_order"`
}

func (h *Handler) AdminListProducts(c *gin.Context) {
	items, err := h.svc.AdminListProducts(c.Request.Context())
	if err != nil {
		h.mapAdminErr(c, err)
		return
	}
	out := make([]productAdminListItem, 0, len(items))
	for _, p := range items {
		out = append(out, toProductAdminListItem(h.svc, p))
	}
	httpx.OK(c, gin.H{"products": out})
}

func (h *Handler) AdminGetProduct(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid id")
		return
	}
	p, err := h.svc.AdminGetProduct(c.Request.Context(), id)
	if err != nil {
		h.mapAdminErr(c, err)
		return
	}
	httpx.OK(c, toProductAdminDetail(h.svc, *p))
}

func (h *Handler) AdminCreateProduct(c *gin.Context) {
	var req productRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
		return
	}
	p, err := h.svc.AdminCreateProduct(c.Request.Context(), productInputFromRequest(req))
	if err != nil {
		h.mapAdminErr(c, err)
		return
	}
	httpx.Created(c, toProductAdminListItem(h.svc, *p))
}

func (h *Handler) AdminUpdateProduct(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid id")
		return
	}
	var req productRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
		return
	}
	p, err := h.svc.AdminUpdateProduct(c.Request.Context(), id, productInputFromRequest(req))
	if err != nil {
		h.mapAdminErr(c, err)
		return
	}
	httpx.OK(c, toProductAdminDetail(h.svc, *p))
}

func (h *Handler) AdminActivateProduct(c *gin.Context)   { h.setProductActive(c, true) }
func (h *Handler) AdminDeactivateProduct(c *gin.Context) { h.setProductActive(c, false) }

func (h *Handler) setProductActive(c *gin.Context, active bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid id")
		return
	}
	if err := h.svc.AdminSetProductActive(c.Request.Context(), id, active); err != nil {
		h.mapAdminErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"ok": true, "is_active": active})
}

// -------- Pricings -------------------------------------------------------

type pricingRequest struct {
	MaterialID   uuid.UUID `json:"material_id"   binding:"required"`
	PricePerM2   *int64    `json:"price_per_m2"`
	MinChargeM2  *float64  `json:"min_charge_m2"`
	WidthCm      *int      `json:"width_cm"`
	HeightCm     *int      `json:"height_cm"`
	PackageLabel string    `json:"package_label"`
	PriceTotal   *int64    `json:"price_total"`
}

func (h *Handler) AdminCreatePricing(c *gin.Context) {
	productID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid product id")
		return
	}
	var req pricingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
		return
	}
	row, err := h.svc.AdminCreatePricing(c.Request.Context(), productID, service.PricingInput{
		MaterialID:   req.MaterialID,
		PricePerM2:   req.PricePerM2,
		MinChargeM2:  req.MinChargeM2,
		WidthCm:      req.WidthCm,
		HeightCm:     req.HeightCm,
		PackageLabel: req.PackageLabel,
		PriceTotal:   req.PriceTotal,
	})
	if err != nil {
		h.mapAdminErr(c, err)
		return
	}
	httpx.Created(c, toPricingAdminResponse(*row))
}

func (h *Handler) AdminUpdatePricing(c *gin.Context) {
	id, err := uuid.Parse(c.Param("pid"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid pricing id")
		return
	}
	var req pricingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
		return
	}
	row, err := h.svc.AdminUpdatePricing(c.Request.Context(), id, service.PricingInput{
		MaterialID:   req.MaterialID,
		PricePerM2:   req.PricePerM2,
		MinChargeM2:  req.MinChargeM2,
		WidthCm:      req.WidthCm,
		HeightCm:     req.HeightCm,
		PackageLabel: req.PackageLabel,
		PriceTotal:   req.PriceTotal,
	})
	if err != nil {
		h.mapAdminErr(c, err)
		return
	}
	httpx.OK(c, toPricingAdminResponse(*row))
}

func (h *Handler) AdminActivatePricing(c *gin.Context)   { h.setPricingActive(c, true) }
func (h *Handler) AdminDeactivatePricing(c *gin.Context) { h.setPricingActive(c, false) }

func (h *Handler) setPricingActive(c *gin.Context, active bool) {
	id, err := uuid.Parse(c.Param("pid"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid pricing id")
		return
	}
	if err := h.svc.AdminSetPricingActive(c.Request.Context(), id, active); err != nil {
		h.mapAdminErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"ok": true, "is_active": active})
}

func (h *Handler) AdminDeletePricing(c *gin.Context) {
	id, err := uuid.Parse(c.Param("pid"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid pricing id")
		return
	}
	if err := h.svc.AdminDeletePricing(c.Request.Context(), id); err != nil {
		h.mapAdminErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// -------- Product image ---------------------------------------------------

// maxProductImageMultipartMemory — batas ukuran raw upload dicek di edge
// (handler) SEBELUM masuk service (§23 poin 3). Sama dgn
// service.MaxProductImageUploadBytes — dijaga tetap sinkron karena keduanya
// merujuk konstanta service (single source of truth, bukan angka telanjang
// duplikat), pola sama seperti internal/sitemedia/handler.
const maxProductImageMultipartMemory = service.MaxProductImageUploadBytes

// POST /admin/catalog/products/:id/image (multipart, field "file") — ganti
// gambar kartu produk.
func (h *Handler) AdminUploadProductImage(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid id")
		return
	}
	fh, ok := openProductImageUploadFile(c)
	if !ok {
		return
	}
	if !webp.IsAllowedMime(fh.Header.Get("Content-Type")) {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, service.ErrImageInvalidType.Error())
		return
	}
	f, err := fh.Open()
	if err != nil {
		log.Ctx(c.Request.Context()).Error().Err(err).Msg("open uploaded product image file")
		httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal, "gagal buka file upload")
		return
	}
	// Close diabaikan sengaja: f hanya DIBACA, tidak ada buffer tulis yang
	// bisa gagal ter-flush. Ditulis eksplisit supaya errcheck lolos tanpa
	// mematikan linter (§22, sama pola dgn sitemedia handler).
	defer func() { _ = f.Close() }()

	p, err := h.svc.AdminUploadProductImage(c.Request.Context(), id, service.ProductImageInput{
		FileReader: f,
		FileSize:   fh.Size,
		MimeType:   fh.Header.Get("Content-Type"),
	})
	if err != nil {
		h.mapAdminErr(c, err)
		return
	}
	httpx.OK(c, toProductAdminDetail(h.svc, *p))
}

// DELETE /admin/catalog/products/:id/image — kosongkan gambar produk.
func (h *Handler) AdminDeleteProductImage(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid id")
		return
	}
	if err := h.svc.AdminDeleteProductImage(c.Request.Context(), id); err != nil {
		h.mapAdminErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func openProductImageUploadFile(c *gin.Context) (*multipart.FileHeader, bool) {
	fh, err := c.FormFile("file")
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "field 'file' wajib")
		return nil, false
	}
	if fh.Size <= 0 {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "file kosong")
		return nil, false
	}
	if fh.Size > maxProductImageMultipartMemory {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "file melebihi batas ukuran unggahan")
		return nil, false
	}
	return fh, true
}

// -------- Error mapping --------------------------------------------------

func (h *Handler) mapAdminErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrValidation),
		errors.Is(err, service.ErrPricingShapeMismatch),
		errors.Is(err, service.ErrImageEmpty),
		errors.Is(err, service.ErrImageTooLarge),
		errors.Is(err, service.ErrImageInvalidType),
		errors.Is(err, service.ErrImageDecodeFailed):
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
	case errors.Is(err, service.ErrPricingTypeLocked):
		httpx.Error(c, http.StatusConflict, "PRICING_TYPE_LOCKED", err.Error())
	case errors.Is(err, service.ErrDuplicateSlug),
		errors.Is(err, service.ErrDuplicateMaterialCode),
		errors.Is(err, service.ErrDuplicatePricing):
		httpx.Error(c, http.StatusConflict, "DUPLICATE", err.Error())
	case errors.Is(err, catalogapi.ErrProductNotFound),
		errors.Is(err, catalogapi.ErrMaterialNotFound),
		errors.Is(err, service.ErrPricingRowNotFound),
		errors.Is(err, service.ErrProductImageEmpty):
		httpx.Error(c, http.StatusNotFound, httpx.CodeNotFound, err.Error())
	default:
		log.Ctx(c.Request.Context()).Error().Err(err).Msg("catalog admin: unmapped error")
		httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
	}
}

// productInputFromRequest — DTO → domain input.
func productInputFromRequest(r productRequest) service.ProductInput {
	return service.ProductInput{
		Slug:         r.Slug,
		Name:         r.Name,
		Description:  r.Description,
		Category:     r.Category,
		PricingType:  r.PricingType,
		MinWidthCm:   r.MinWidthCm,
		MinHeightCm:  r.MinHeightCm,
		MaxWidthCm:   r.MaxWidthCm,
		MaxHeightCm:  r.MaxHeightCm,
		DisplayOrder: r.DisplayOrder,
	}
}
