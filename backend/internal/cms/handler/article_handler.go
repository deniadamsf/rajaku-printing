// Package handler — HTTP endpoints modul CMS artikel.
package handler

import (
	"errors"
	"mime/multipart"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/cms/cmsapi"
	"github.com/rajaku-printing/backend/internal/cms/service"
	"github.com/rajaku-printing/backend/internal/httpx"
)

type Handler struct {
	svc *service.Service
}

func New(svc *service.Service) *Handler { return &Handler{svc: svc} }

func mapDomainErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, cmsapi.ErrArticleNotFound), errors.Is(err, cmsapi.ErrImageNotFound):
		httpx.Error(c, http.StatusNotFound, httpx.CodeNotFound, err.Error())
	case errors.Is(err, cmsapi.ErrSlugTaken):
		httpx.Error(c, http.StatusConflict, httpx.CodeConflict, err.Error())
	case errors.Is(err, cmsapi.ErrInvalidSlug),
		errors.Is(err, cmsapi.ErrTitleRequired),
		errors.Is(err, cmsapi.ErrContentRequired),
		errors.Is(err, cmsapi.ErrInvalidStatus),
		errors.Is(err, cmsapi.ErrNotPublishable),
		errors.Is(err, cmsapi.ErrImageEmpty),
		errors.Is(err, cmsapi.ErrImageTooLarge),
		errors.Is(err, cmsapi.ErrImageInvalidType),
		errors.Is(err, cmsapi.ErrImageDecodeFailed):
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
	default:
		log.Ctx(c.Request.Context()).Error().Err(err).Msg("cms handler: unmapped error")
		httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal, "internal server error")
	}
}

// -------- Public --------

// GET /articles?page=1&limit=10&q=banner
func (h *Handler) ListPublic(c *gin.Context) {
	page, limit := parsePagination(c)
	q := c.Query("q")
	out, err := h.svc.ListPublished(c.Request.Context(), page, limit, q)
	if err != nil {
		mapDomainErr(c, err)
		return
	}
	httpx.OK(c, out)
}

// GET /articles/:slug
func (h *Handler) GetPublicBySlug(c *gin.Context) {
	a, err := h.svc.GetPublishedBySlug(c.Request.Context(), c.Param("slug"))
	if err != nil {
		mapDomainErr(c, err)
		return
	}
	httpx.OK(c, a)
}

// GET /cms/images/:id — serve WebP publicly (dipakai artikel di halaman public).
func (h *Handler) ServeImage(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid image id")
		return
	}
	handle, err := h.svc.GetImage(c.Request.Context(), id)
	if err != nil {
		mapDomainErr(c, err)
		return
	}
	// Cache 30 hari (image content immutable — id-based URL).
	c.Header("Content-Type", handle.Image.MimeType)
	c.Header("Cache-Control", "public, max-age=2592000, immutable")
	c.Header("Content-Disposition",
		"inline; filename="+strconv.Quote(handle.Image.OriginalName))
	c.File(handle.AbsPath)
}

// -------- Admin --------

// GET /admin/articles?status=draft&page=1&limit=20&q=x
func (h *Handler) ListAdmin(c *gin.Context) {
	page, limit := parsePagination(c)
	out, err := h.svc.ListForAdmin(c.Request.Context(), page, limit, c.Query("status"), c.Query("q"))
	if err != nil {
		mapDomainErr(c, err)
		return
	}
	httpx.OK(c, out)
}

// GET /admin/articles/:id
func (h *Handler) GetAdmin(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	a, err := h.svc.GetArticleByID(c.Request.Context(), id)
	if err != nil {
		mapDomainErr(c, err)
		return
	}
	httpx.OK(c, a)
}

type createArticleBody struct {
	Title           string     `json:"title" binding:"required"`
	Slug            string     `json:"slug"`
	Excerpt         string     `json:"excerpt"`
	ContentMD       string     `json:"content_md" binding:"required"`
	MetaTitle       string     `json:"meta_title"`
	MetaDescription string     `json:"meta_description"`
	CoverImageID    *uuid.UUID `json:"cover_image_id"`
}

// POST /admin/articles
func (h *Handler) CreateArticle(c *gin.Context) {
	id, err := authapi.IdentityFromContext(c.Request.Context())
	if err != nil || id == nil {
		httpx.Error(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}
	var body createArticleBody
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid json body")
		return
	}
	a, err := h.svc.CreateArticle(c.Request.Context(), service.CreateArticleInput{
		AuthorID:        id.UserID,
		Title:           body.Title,
		Slug:            body.Slug,
		Excerpt:         body.Excerpt,
		ContentMD:       body.ContentMD,
		MetaTitle:       body.MetaTitle,
		MetaDescription: body.MetaDescription,
		CoverImageID:    body.CoverImageID,
	})
	if err != nil {
		mapDomainErr(c, err)
		return
	}
	httpx.Created(c, a)
}

type updateArticleBody struct {
	Title           *string    `json:"title"`
	Slug            *string    `json:"slug"`
	Excerpt         *string    `json:"excerpt"`
	ContentMD       *string    `json:"content_md"`
	MetaTitle       *string    `json:"meta_title"`
	MetaDescription *string    `json:"meta_description"`
	// CoverImageID: pass uuid.Nil string ("00000000-...") untuk clear;
	// nil = tidak diubah.
	CoverImageID *uuid.UUID `json:"cover_image_id"`
}

// PUT /admin/articles/:id
func (h *Handler) UpdateArticle(c *gin.Context) {
	articleID, ok := parseIDParam(c)
	if !ok {
		return
	}
	var body updateArticleBody
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid json body")
		return
	}
	a, err := h.svc.UpdateArticle(c.Request.Context(), service.UpdateArticleInput{
		ID:              articleID,
		Title:           body.Title,
		Slug:            body.Slug,
		Excerpt:         body.Excerpt,
		ContentMD:       body.ContentMD,
		MetaTitle:       body.MetaTitle,
		MetaDescription: body.MetaDescription,
		CoverImageID:    body.CoverImageID,
	})
	if err != nil {
		mapDomainErr(c, err)
		return
	}
	httpx.OK(c, a)
}

// POST /admin/articles/:id/publish
func (h *Handler) PublishArticle(c *gin.Context) {
	articleID, ok := parseIDParam(c)
	if !ok {
		return
	}
	a, err := h.svc.PublishArticle(c.Request.Context(), articleID)
	if err != nil {
		mapDomainErr(c, err)
		return
	}
	httpx.OK(c, a)
}

// POST /admin/articles/:id/archive
func (h *Handler) ArchiveArticle(c *gin.Context) {
	articleID, ok := parseIDParam(c)
	if !ok {
		return
	}
	a, err := h.svc.UnpublishArticle(c.Request.Context(), articleID)
	if err != nil {
		mapDomainErr(c, err)
		return
	}
	httpx.OK(c, a)
}

// DELETE /admin/articles/:id
func (h *Handler) DeleteArticle(c *gin.Context) {
	articleID, ok := parseIDParam(c)
	if !ok {
		return
	}
	if err := h.svc.DeleteArticle(c.Request.Context(), articleID); err != nil {
		mapDomainErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// POST /admin/articles/images (multipart)
// Fields: file (required), alt_text, article_id (opsional).
func (h *Handler) UploadImage(c *gin.Context) {
	id, err := authapi.IdentityFromContext(c.Request.Context())
	if err != nil || id == nil {
		httpx.Error(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}
	fh, ok := openUploadFile(c)
	if !ok {
		return
	}
	f, err := fh.Open()
	if err != nil {
		log.Ctx(c.Request.Context()).Error().Err(err).Msg("open uploaded cms image")
		httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal, "gagal buka file upload")
		return
	}
	defer f.Close()

	var articleIDPtr *uuid.UUID
	if raw := c.PostForm("article_id"); raw != "" {
		aid, err := uuid.Parse(raw)
		if err != nil {
			httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "article_id bukan UUID valid")
			return
		}
		articleIDPtr = &aid
	}

	img, err := h.svc.UploadImage(c.Request.Context(), service.UploadImageInput{
		UploaderID:   id.UserID,
		ArticleID:    articleIDPtr,
		FileReader:   f,
		FileSize:     fh.Size,
		MimeType:     fh.Header.Get("Content-Type"),
		OriginalName: fh.Filename,
		AltText:      c.PostForm("alt_text"),
	})
	if err != nil {
		mapDomainErr(c, err)
		return
	}
	httpx.Created(c, img)
}

// -------- helpers --------

func parseIDParam(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid id")
		return uuid.Nil, false
	}
	return id, true
}

func parsePagination(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))
	return page, limit
}

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
	return fh, true
}
