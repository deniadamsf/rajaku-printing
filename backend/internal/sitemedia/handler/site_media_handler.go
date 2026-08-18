// Package handler — HTTP endpoints modul sitemedia (§22: handler HTTP only,
// tidak menyentuh *gorm.DB).
package handler

import (
	"errors"
	"mime/multipart"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/httpx"
	"github.com/rajaku-printing/backend/internal/pkg/webp"
	"github.com/rajaku-printing/backend/internal/sitemedia/model"
	"github.com/rajaku-printing/backend/internal/sitemedia/service"
	"github.com/rajaku-printing/backend/internal/sitemedia/sitemediaapi"
)

type Handler struct {
	svc *service.Service
}

func New(svc *service.Service) *Handler { return &Handler{svc: svc} }

func mapDomainErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, sitemediaapi.ErrSlotEmpty):
		httpx.Error(c, http.StatusNotFound, httpx.CodeNotFound, err.Error())
	case errors.Is(err, sitemediaapi.ErrUnknownSlot),
		errors.Is(err, sitemediaapi.ErrImageEmpty),
		errors.Is(err, sitemediaapi.ErrImageTooLarge),
		errors.Is(err, sitemediaapi.ErrImageInvalidType),
		errors.Is(err, sitemediaapi.ErrImageDecodeFailed):
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
	default:
		log.Ctx(c.Request.Context()).Error().Err(err).Msg("sitemedia handler: unmapped error")
		httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal, "internal server error")
	}
}

// -------- Public --------

// GET /site-media — peta slot->url untuk slot yang sudah diisi admin.
// Dipanggil landing page saat SSR, no-auth, harus ringan.
func (h *Handler) ListPublic(c *gin.Context) {
	m, err := h.svc.ListPublicMap(c.Request.Context())
	if err != nil {
		mapDomainErr(c, err)
		return
	}
	httpx.OK(c, m)
}

// GET /site-media/file/:slot — sajikan blob WebP.
func (h *Handler) ServeFile(c *gin.Context) {
	slot := c.Param("slot")
	handle, err := h.svc.GetFile(c.Request.Context(), slot)
	if err != nil {
		mapDomainErr(c, err)
		return
	}
	// Cache pendek (bukan immutable — slot bisa ditimpa admin kapan saja,
	// beda dgn CMS article image yang id-based dan konten tidak pernah berubah).
	c.Header("Content-Type", handle.MimeType)
	c.Header("Cache-Control", "public, max-age=300")
	c.File(handle.AbsPath)
}

// -------- Admin --------

// GET /admin/site-media — SEMUA slot registry + nilai terisi (slot kosong
// tetap muncul dengan field media null).
func (h *Handler) ListAdmin(c *gin.Context) {
	items, err := h.svc.ListAdmin(c.Request.Context())
	if err != nil {
		mapDomainErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

// POST /admin/site-media/:slot (multipart, field "file") — ganti gambar slot.
func (h *Handler) Upload(c *gin.Context) {
	id, err := authapi.IdentityFromContext(c.Request.Context())
	if err != nil || id == nil {
		httpx.Error(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}
	slot := c.Param("slot")
	if !model.IsValidSlot(slot) {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, sitemediaapi.ErrUnknownSlot.Error())
		return
	}
	fh, ok := openUploadFile(c)
	if !ok {
		return
	}
	if !webp.IsAllowedMime(fh.Header.Get("Content-Type")) {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, sitemediaapi.ErrImageInvalidType.Error())
		return
	}
	f, err := fh.Open()
	if err != nil {
		log.Ctx(c.Request.Context()).Error().Err(err).Msg("open uploaded sitemedia file")
		httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal, "gagal buka file upload")
		return
	}
	defer f.Close()

	row, err := h.svc.Upload(c.Request.Context(), service.UploadInput{
		Slot:         slot,
		FileReader:   f,
		FileSize:     fh.Size,
		MimeType:     fh.Header.Get("Content-Type"),
		OriginalName: fh.Filename,
		UploaderID:   id.UserID,
	})
	if err != nil {
		mapDomainErr(c, err)
		return
	}
	httpx.OK(c, row)
}

// DELETE /admin/site-media/:slot — kosongkan slot (frontend kembali pakai
// file statis bawaan).
func (h *Handler) Delete(c *gin.Context) {
	slot := c.Param("slot")
	if err := h.svc.Delete(c.Request.Context(), slot); err != nil {
		mapDomainErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// -------- helpers --------

// maxMultipartMemory — batas ukuran raw upload yang dicek di edge (handler)
// SEBELUM masuk service (§23 poin 3). Sama dgn service.MaxUploadBytes —
// dijaga tetap sinkron karena keduanya merujuk konstanta service (single
// source of truth, bukan angka telanjang duplikat).
const maxMultipartMemory = service.MaxUploadBytes

func openUploadFile(c *gin.Context) (*multipart.FileHeader, bool) {
	fh, err := c.FormFile("file")
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "field 'file' wajib")
		return nil, false
	}
	if fh.Size <= 0 {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "file kosong")
		return nil, false
	}
	if fh.Size > maxMultipartMemory {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "file melebihi batas ukuran unggahan")
		return nil, false
	}
	return fh, true
}
