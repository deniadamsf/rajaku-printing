package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
)

// RegisterRoutes wires settings endpoints.
//
// Public (no auth, rate limited — caller wajib mount `public` group yang
// sudah dipasangi middleware.RateLimitPublic, lihat internal/server/router.go):
//
//	GET /payment-info          info rekening/QRIS (§7), HANYA 6 key payment.*
//
// Admin — staff-only + permission `settings.manage` (di-seed migration
// 000011, hanya super_admin yang dapat):
//
//	GET /admin/settings        daftar setting global
//	PUT /admin/settings/:key   ubah satu setting (mis. design_retention_days)
func (h *Handler) RegisterRoutes(v1, public *gin.RouterGroup, auth authapi.Service) {
	public.GET("/payment-info", h.PaymentInfo)

	admin := v1.Group("/admin/settings")
	admin.Use(
		authapi.RequireAuth(auth),
		authapi.RequireUserType(authapi.UserTypeStaff),
		authapi.RequirePermission("settings.manage"),
	)
	{
		admin.GET("", h.List)
		admin.PUT("/:key", h.Update)
	}
}
