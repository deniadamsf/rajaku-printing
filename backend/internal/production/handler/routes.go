package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
)

// RegisterRoutes wires production endpoints — semua di /admin (staff-only)
// dgn permission production.update.
//
//   POST /admin/orders/:resi/production/start-cetak    desain_diverifikasi → proses_cetak
//   POST /admin/orders/:resi/production/start-qc       proses_cetak       → qc
//   POST /admin/orders/:resi/production/mark-siap      qc                 → siap_kirim | siap_ambil
//   POST /admin/orders/:resi/production/mark-shipped   siap_kirim         → dikirim (butuh courier+tracking)
//   POST /admin/orders/:resi/production/mark-selesai   dikirim|siap_ambil → selesai
func (h *Handler) RegisterRoutes(v1 *gin.RouterGroup, auth authapi.Service) {
	admin := v1.Group("/admin/orders/:resi/production")
	admin.Use(
		authapi.RequireAuth(auth),
		authapi.RequireUserType(authapi.UserTypeStaff),
		authapi.RequirePermission("production.update"),
	)
	admin.POST("/start-cetak", h.StartCetak)
	admin.POST("/start-qc", h.StartQC)
	admin.POST("/mark-siap", h.MarkSiap)
	admin.POST("/mark-shipped", h.MarkShipped)
	admin.POST("/mark-selesai", h.MarkSelesai)
}
