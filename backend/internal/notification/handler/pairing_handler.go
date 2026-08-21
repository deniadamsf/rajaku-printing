package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/httpx"
	"github.com/rajaku-printing/backend/internal/notification/service"
	"github.com/rajaku-printing/backend/internal/notification/workerclient"
)

// pairingService is the narrow slice of *service.PairingService this handler
// actually calls. Declared as an interface (rather than depending on the
// concrete struct) so handler tests can inject a fake — matches the pattern
// used by internal/payment/handler (paymentService interface).
type pairingService interface {
	GetPairingStatus(ctx context.Context) (service.PairingStatusView, error)
	Unlink(ctx context.Context) error
}

// PairingHandler — endpoint admin panel untuk pairing WhatsApp (QR).
// Terpisah dari Handler (worker-facing /internal/notifications/*) karena
// audiens dan mekanisme authnya beda: ini staff-only + permission, bukan
// shared-secret worker.
type PairingHandler struct {
	svc pairingService
}

func NewPairingHandler(svc pairingService) *PairingHandler {
	return &PairingHandler{svc: svc}
}

// GET /api/v1/admin/whatsapp/pairing
//
// PENTING: SELALU membalas HTTP 200, baik worker reachable maupun tidak.
// Kalau worker tidak bisa dihubungi, worker_reachable=false dan field
// lainnya kosong/null — operator perlu bisa membedakan "worker mati" dari
// "halaman/endpoint rusak" (keduanya beda tindak lanjut). Error asli dicatat
// di log server, TIDAK dibocorkan lewat response (QR/status pairing setara
// kredensial — lihat catatan keamanan tugas).
func (h *PairingHandler) GetPairing(c *gin.Context) {
	view, err := h.svc.GetPairingStatus(c.Request.Context())
	if err != nil {
		log.Ctx(c.Request.Context()).Error().Err(err).
			Msg("notification pairing: gagal ambil status dari notification-worker")
	}
	httpx.OK(c, gin.H{
		"worker_reachable": view.Reachable,
		"connected":        view.Connected,
		"self_number":      view.SelfNumber,
		"qr_data_url":      view.QRDataURL,
		"quota":            rawOrNull(view.Quota),
		"circuit":          rawOrNull(view.Circuit),
		"pacing":           rawOrNull(view.Pacing),
	})
}

// POST /api/v1/admin/whatsapp/unlink
//
// Kebalikan dari GetPairing: kalau worker tidak bisa dihubungi, endpoint ini
// WAJIB gagal (bukan 200 palsu) — operator harus tahu pemutusan pairing
// tidak benar-benar terjadi.
func (h *PairingHandler) Unlink(c *gin.Context) {
	if err := h.svc.Unlink(c.Request.Context()); err != nil {
		log.Ctx(c.Request.Context()).Error().Err(err).
			Msg("notification pairing: gagal memutus pairing whatsapp")
		switch {
		case errors.Is(err, workerclient.ErrWorkerUnreachable):
			httpx.Error(c, http.StatusServiceUnavailable, httpx.CodeServiceUnavail,
				"notification worker tidak bisa dihubungi, pemutusan pairing dibatalkan")
		case errors.Is(err, workerclient.ErrUnauthorized):
			httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal,
				"konfigurasi rahasia notification worker tidak valid")
		default:
			httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal,
				"gagal memutus pairing whatsapp")
		}
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// RegisterRoutes wires /admin/whatsapp/* — staff-only, permission
// notification.manage (di-seed migration 000021). QR pairing setara
// kredensial (§ catatan keamanan tugas) — sengaja TIDAK dimount di grup
// public.
func (h *PairingHandler) RegisterRoutes(v1 *gin.RouterGroup, auth authapi.Service) {
	admin := v1.Group("/admin/whatsapp")
	admin.Use(
		authapi.RequireAuth(auth),
		authapi.RequireUserType(authapi.UserTypeStaff),
		authapi.RequirePermission("notification.manage"),
	)
	admin.GET("/pairing", h.GetPairing)
	admin.POST("/unlink", h.Unlink)
}

// rawOrNull — json.RawMessage kosong (nil/len 0) di-marshal sebagai `null`,
// bukan string kosong — supaya kontrak field JSON konsisten dgn dokumentasi
// tugas ("field lain kosong/null" saat worker tak reachable).
func rawOrNull(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}
	return json.RawMessage(raw)
}
