// Package handler contains the HTTP layer for catalog module.
package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/rajaku-printing/backend/internal/catalog/catalogapi"
	"github.com/rajaku-printing/backend/internal/catalog/service"
	"github.com/rajaku-printing/backend/internal/httpx"
)

type Handler struct {
	svc *service.Service
}

func New(svc *service.Service) *Handler { return &Handler{svc: svc} }

// GET /catalog/products — list produk aktif.
func (h *Handler) ListProducts(c *gin.Context) {
	items, err := h.svc.ListActiveProducts(c.Request.Context())
	if err != nil {
		log.Ctx(c.Request.Context()).Error().Err(err).Msg("list products failed")
		httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
		return
	}
	out := make([]productListItem, 0, len(items))
	for _, p := range items {
		out = append(out, toProductListItem(h.svc, p))
	}
	httpx.OK(c, gin.H{"products": out})
}

// GET /catalog/products/:slug — detail produk termasuk pricings.
func (h *Handler) GetProductBySlug(c *gin.Context) {
	slug := c.Param("slug")
	if slug == "" {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeBadRequest, "slug required")
		return
	}
	p, err := h.svc.GetProductDetail(c.Request.Context(), slug)
	if err != nil {
		h.mapErr(c, err)
		return
	}
	httpx.OK(c, toProductDetailResponse(h.svc, *p))
}

// GET /catalog/product-images/:id — sajikan gambar produk (WebP) publik.
// Dipakai kartu produk landing page sebagai src <img>. 404 kalau produk
// tidak ada atau belum diunggah gambarnya (frontend fallback ke ikon
// generik, lihat catalogapi.ErrProductNotFound / service.ErrProductImageEmpty).
func (h *Handler) ServeProductImage(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid product id")
		return
	}
	handle, err := h.svc.GetProductImage(c.Request.Context(), id)
	if err != nil {
		h.mapErr(c, err)
		return
	}
	// Cache pendek (bukan immutable — admin bisa timpa gambar kapan saja).
	c.Header("Content-Type", handle.MimeType)
	c.Header("Cache-Control", "public, max-age=300")
	c.File(handle.AbsPath)
}

// GET /catalog/materials — list bahan aktif.
func (h *Handler) ListMaterials(c *gin.Context) {
	items, err := h.svc.ListActiveMaterials(c.Request.Context())
	if err != nil {
		log.Ctx(c.Request.Context()).Error().Err(err).Msg("list materials failed")
		httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
		return
	}
	out := make([]materialResponse, 0, len(items))
	for _, m := range items {
		out = append(out, toMaterialResponse(m))
	}
	httpx.OK(c, gin.H{"materials": out})
}

// POST /catalog/quote — hitung harga untuk (product, material, dimensi).
func (h *Handler) Quote(c *gin.Context) {
	var req quoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
		return
	}
	q, err := h.svc.Quote(c.Request.Context(), catalogapi.QuoteRequest{
		ProductID:  req.ProductID,
		MaterialID: req.MaterialID,
		WidthCm:    req.WidthCm,
		HeightCm:   req.HeightCm,
	})
	if err != nil {
		h.mapErr(c, err)
		return
	}
	httpx.OK(c, toQuoteResponse(q))
}

func (h *Handler) mapErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, catalogapi.ErrProductNotFound), errors.Is(err, service.ErrProductImageEmpty):
		httpx.Error(c, http.StatusNotFound, httpx.CodeNotFound, "produk tidak ditemukan")
	case errors.Is(err, catalogapi.ErrMaterialNotFound):
		httpx.Error(c, http.StatusNotFound, httpx.CodeNotFound, "bahan tidak ditemukan")
	case errors.Is(err, catalogapi.ErrPricingNotAvailable):
		httpx.Error(c, http.StatusUnprocessableEntity, httpx.CodeUnprocessable,
			"harga belum tersedia untuk kombinasi produk & bahan ini")
	case errors.Is(err, catalogapi.ErrDimensionsOutOfRange):
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation,
			"ukuran di luar rentang yang didukung produk")
	case errors.Is(err, catalogapi.ErrDimensionsInvalid):
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation,
			"ukuran harus bilangan positif")
	default:
		log.Ctx(c.Request.Context()).Error().Err(err).Msg("catalog service error")
		httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
	}
}
