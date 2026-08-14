// Package handler — HTTP endpoints untuk worker + admin panel.
//
// /internal/notifications/* — dipakai notification-worker (Node.js/Baileys).
// Auth pakai shared secret header (bukan JWT) supaya worker tidak perlu
// tergantung modul auth. Secret disimpan di env NOTIFICATION_WORKER_SECRET.
package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/httpx"
	"github.com/rajaku-printing/backend/internal/notification/repository"
	"github.com/rajaku-printing/backend/internal/notification/service"
)

type Handler struct {
	svc            *service.Service
	internalSecret string
}

func New(svc *service.Service, internalSecret string) *Handler {
	return &Handler{svc: svc, internalSecret: internalSecret}
}

const internalSecretHeader = "X-Internal-Secret"

// requireInternalSecret — middleware sederhana constant-time compare.
func (h *Handler) requireInternalSecret() gin.HandlerFunc {
	return func(c *gin.Context) {
		got := strings.TrimSpace(c.GetHeader(internalSecretHeader))
		if got == "" || !secureEqual(got, h.internalSecret) {
			httpx.Error(c, http.StatusUnauthorized, httpx.CodeUnauthorized,
				"invalid or missing "+internalSecretHeader)
			return
		}
		c.Next()
	}
}

// POST /internal/notifications/claim
// Body: {"limit": 20}
// Returns: {"jobs": [{id, kind, recipient_phone, message, attempts, max_attempts}, ...]}
func (h *Handler) Claim(c *gin.Context) {
	var body struct {
		Limit int `json:"limit"`
	}
	// Body opsional (worker boleh POST kosong = default limit).
	_ = c.ShouldBindJSON(&body)
	if body.Limit <= 0 {
		body.Limit = 20
	}
	jobs, err := h.svc.ClaimBatch(c.Request.Context(), body.Limit)
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal, err.Error())
		return
	}
	httpx.OK(c, gin.H{"jobs": jobs})
}

// POST /internal/notifications/:id/sent
func (h *Handler) MarkSent(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid job id")
		return
	}
	if err := h.svc.MarkSent(c.Request.Context(), id); err != nil {
		if errors.Is(err, repository.ErrJobStale) || errors.Is(err, repository.ErrNotFound) {
			// Idempotent: worker retried a callback for a job we already finalized.
			httpx.OK(c, gin.H{"already_marked": true})
			return
		}
		httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal, err.Error())
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// POST /internal/notifications/:id/failed
// Body: {"error": "message"}
func (h *Handler) MarkFailed(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid job id")
		return
	}
	var body struct {
		Error string `json:"error"`
	}
	_ = c.ShouldBindJSON(&body)
	if strings.TrimSpace(body.Error) == "" {
		body.Error = "worker reported failure without message"
	}
	if err := h.svc.MarkFailed(c.Request.Context(), id, body.Error); err != nil {
		if errors.Is(err, repository.ErrJobStale) || errors.Is(err, repository.ErrNotFound) {
			httpx.OK(c, gin.H{"already_marked": true})
			return
		}
		httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal, err.Error())
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// RegisterRoutes wires:
//   - /internal/notifications/* — shared-secret auth (worker-facing).
//
// Admin dashboard endpoints (/admin/notifications) belum ada — akan
// ditambah saat admin panel dibangun. Cukup untuk MVP: worker bisa
// dispatch + report; admin bisa manual query DB kalau perlu inspeksi.
func (h *Handler) RegisterRoutes(v1 *gin.RouterGroup) {
	internal := v1.Group("/internal/notifications")
	internal.Use(h.requireInternalSecret())
	internal.POST("/claim", h.Claim)
	internal.POST("/:id/sent", h.MarkSent)
	internal.POST("/:id/failed", h.MarkFailed)
}

// secureEqual — constant-time equality untuk cegah timing attack pada
// shared secret. Pakai subtle.ConstantTimeCompare via manual byte loop
// (avoid crypto/subtle import — simple enough).
func secureEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := 0; i < len(a); i++ {
		v |= a[i] ^ b[i]
	}
	return v == 0
}
