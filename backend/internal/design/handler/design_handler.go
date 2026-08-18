// Package handler — HTTP endpoints untuk modul design.
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
	"github.com/rajaku-printing/backend/internal/design/designapi"
	"github.com/rajaku-printing/backend/internal/design/service"
	"github.com/rajaku-printing/backend/internal/httpx"
)

type Handler struct {
	svc *service.Service
}

func New(svc *service.Service) *Handler { return &Handler{svc: svc} }

// mapDomainErr — konversi sentinel designapi errors ke response HTTP.
// Non-domain error di-log & jadi 500.
func mapDomainErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, designapi.ErrOrderNotFound), errors.Is(err, designapi.ErrFileNotFound):
		httpx.Error(c, http.StatusNotFound, httpx.CodeNotFound, err.Error())
	case errors.Is(err, designapi.ErrNotOrderOwner):
		httpx.Error(c, http.StatusForbidden, httpx.CodeForbidden, err.Error())
	case errors.Is(err, designapi.ErrOrderNotDesignReady),
		errors.Is(err, designapi.ErrDesignSourceMismatch),
		errors.Is(err, designapi.ErrInvalidRole),
		errors.Is(err, designapi.ErrInvalidMimeType),
		errors.Is(err, designapi.ErrFileTooLarge),
		errors.Is(err, designapi.ErrFileEmpty),
		errors.Is(err, designapi.ErrRevisionNotesRequired),
		errors.Is(err, designapi.ErrWalkinOnlyForPOS):
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
	case errors.Is(err, designapi.ErrPendingDraftExists),
		errors.Is(err, designapi.ErrDraftAlreadyReviewed),
		errors.Is(err, designapi.ErrOrderStateChanged):
		httpx.Error(c, http.StatusConflict, httpx.CodeConflict, err.Error())
	case errors.Is(err, designapi.ErrFilePurged):
		httpx.Error(c, http.StatusGone, httpx.CodeNotFound, err.Error())
	default:
		log.Ctx(c.Request.Context()).Error().Err(err).Msg("design handler: unmapped error")
		httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal, "internal server error")
	}
}

// POST /orders/:resi/design-files
// Content-Type: multipart/form-data
// Fields: file (required), notes (optional)
// Auth: RequireAuth (customer OR staff). Service derive role dari order.design_source.
func (h *Handler) UploadCustomerFile(c *gin.Context) {
	id, err := authapi.IdentityFromContext(c.Request.Context())
	if err != nil || id == nil {
		httpx.Error(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}
	resi := c.Param("resi")
	if resi == "" {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeBadRequest, "resi required")
		return
	}
	fh, ok := openUploadFile(c)
	if !ok {
		return
	}
	f, err := fh.Open()
	if err != nil {
		log.Ctx(c.Request.Context()).Error().Err(err).Msg("open uploaded design file")
		httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal, "gagal buka file upload")
		return
	}
	// Close diabaikan sengaja: f adalah file upload yang hanya DIBACA, jadi
	// tidak ada buffer tulis yang bisa gagal ter-flush. Ditulis eksplisit
	// supaya errcheck lolos tanpa mematikan linter, dan supaya jelas ini
	// keputusan, bukan kelalaian (CLAUDE.md 22).
	defer func() { _ = f.Close() }()

	saved, err := h.svc.UploadCustomerFile(c.Request.Context(), service.UploadInput{
		Resi:          resi,
		CallerID:      id.UserID,
		IsStaff:       id.UserType == authapi.UserTypeStaff,
		ScopedOrderID: id.OrderID,
		FileReader:    f,
		FileSize:      fh.Size,
		MimeType:      fh.Header.Get("Content-Type"),
		OriginalName:  fh.Filename,
		Notes:         c.PostForm("notes"),
	})
	if err != nil {
		mapDomainErr(c, err)
		return
	}
	httpx.Created(c, saved)
}

// POST /admin/orders/:resi/design-drafts
// Same multipart form; caller is staff (permission design.work).
func (h *Handler) StaffUploadDraft(c *gin.Context) {
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
		log.Ctx(c.Request.Context()).Error().Err(err).Msg("open uploaded draft file")
		httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal, "gagal buka file upload")
		return
	}
	// Close diabaikan sengaja: f adalah file upload yang hanya DIBACA, jadi
	// tidak ada buffer tulis yang bisa gagal ter-flush. Ditulis eksplisit
	// supaya errcheck lolos tanpa mematikan linter, dan supaya jelas ini
	// keputusan, bukan kelalaian (CLAUDE.md 22).
	defer func() { _ = f.Close() }()

	saved, err := h.svc.StaffUploadDraft(c.Request.Context(), service.UploadInput{
		Resi:          c.Param("resi"),
		CallerID:      id.UserID,
		IsStaff:       true,
		ScopedOrderID: id.OrderID,
		FileReader:    f,
		FileSize:      fh.Size,
		MimeType:      fh.Header.Get("Content-Type"),
		OriginalName:  fh.Filename,
		Notes:         c.PostForm("notes"),
	})
	if err != nil {
		mapDomainErr(c, err)
		return
	}
	httpx.Created(c, saved)
}

// POST /admin/orders/:resi/design-verify
// Staff verifikasi customer_upload → advance ke desain_diverifikasi.
func (h *Handler) StaffVerifyUpload(c *gin.Context) {
	id, err := authapi.IdentityFromContext(c.Request.Context())
	if err != nil || id == nil {
		httpx.Error(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}
	var body struct {
		Note string `json:"note"`
	}
	_ = c.ShouldBindJSON(&body)
	if err := h.svc.StaffVerifyUpload(c.Request.Context(), service.StaffVerifyInput{
		Resi:    c.Param("resi"),
		StaffID: id.UserID,
		Note:    body.Note,
	}); err != nil {
		mapDomainErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// POST /admin/orders/:resi/design-walkin-approve  (§11 POS shortcut)
func (h *Handler) StaffApproveWalkin(c *gin.Context) {
	id, err := authapi.IdentityFromContext(c.Request.Context())
	if err != nil || id == nil {
		httpx.Error(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}
	var body struct {
		Note string `json:"note"`
	}
	_ = c.ShouldBindJSON(&body)
	if err := h.svc.StaffApproveWalkinInstant(c.Request.Context(), service.WalkinInstantApproveInput{
		Resi:    c.Param("resi"),
		StaffID: id.UserID,
		Note:    body.Note,
	}); err != nil {
		mapDomainErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// POST /design-drafts/:id/approve  (customer)
func (h *Handler) ApproveDraft(c *gin.Context) {
	id, err := authapi.IdentityFromContext(c.Request.Context())
	if err != nil || id == nil {
		httpx.Error(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}
	draftID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid draft id")
		return
	}
	out, err := h.svc.ApproveDraft(c.Request.Context(), service.ApproveInput{
		DraftID:       draftID,
		CallerID:      id.UserID,
		IsStaff:       id.UserType == authapi.UserTypeStaff,
		ScopedOrderID: id.OrderID,
	})
	if err != nil {
		mapDomainErr(c, err)
		return
	}
	httpx.OK(c, out)
}

// POST /design-drafts/:id/revision  (customer)
// Body: {"notes": "…"}
func (h *Handler) RequestRevision(c *gin.Context) {
	id, err := authapi.IdentityFromContext(c.Request.Context())
	if err != nil || id == nil {
		httpx.Error(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}
	draftID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid draft id")
		return
	}
	var body struct {
		Notes string `json:"notes"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid json body")
		return
	}
	out, err := h.svc.RequestRevision(c.Request.Context(), service.RevisionInput{
		DraftID:       draftID,
		CallerID:      id.UserID,
		IsStaff:       id.UserType == authapi.UserTypeStaff,
		Notes:         body.Notes,
		ScopedOrderID: id.OrderID,
	})
	if err != nil {
		mapDomainErr(c, err)
		return
	}
	httpx.OK(c, out)
}

// GET /orders/:resi/design-files
func (h *Handler) List(c *gin.Context) {
	id, err := authapi.IdentityFromContext(c.Request.Context())
	if err != nil || id == nil {
		httpx.Error(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}
	items, err := h.svc.ListForOrder(c.Request.Context(), c.Param("resi"),
		id.UserID, id.UserType == authapi.UserTypeStaff, id.OrderID)
	if err != nil {
		mapDomainErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

// GET /design-files/:id/file — stream file content (auth-guarded via service).
func (h *Handler) GetFile(c *gin.Context) {
	id, err := authapi.IdentityFromContext(c.Request.Context())
	if err != nil || id == nil {
		httpx.Error(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}
	fileID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid file id")
		return
	}
	handle, err := h.svc.GetFile(c.Request.Context(), fileID,
		id.UserID, id.UserType == authapi.UserTypeStaff, id.OrderID)
	if err != nil {
		mapDomainErr(c, err)
		return
	}
	c.Header("Content-Type", handle.File.FileMimeType)
	c.Header("Content-Disposition",
		"inline; filename="+strconv.Quote(handle.File.FileOriginalName))
	c.File(handle.AbsPath)
}

// openUploadFile — validasi shape file field. Return FileHeader supaya caller
// bisa Open() sendiri dengan defer Close(). Menulis error response langsung
// kalau gagal; caller cukup return.
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
