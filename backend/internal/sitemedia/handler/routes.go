package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
)

// RegisterRoutes — mount sitemedia endpoints.
//
// Public (no auth) — dipanggil landing page saat SSR:
//
//	GET  /site-media            peta {slot: url} untuk slot yang sudah diisi
//	GET  /site-media/file/:slot sajikan blob WebP
//
// Admin (staff + permission sitemedia.manage, di-seed migration 000018):
//
//	GET    /admin/site-media         daftar SEMUA slot registry + nilai terisi
//	POST   /admin/site-media/:slot   ganti gambar slot (multipart, field "file")
//	DELETE /admin/site-media/:slot   kosongkan slot (kembali ke aset statis)
func (h *Handler) RegisterRoutes(v1, public *gin.RouterGroup, auth authapi.Service) {
	// Public
	public.GET("/site-media", h.ListPublic)
	public.GET("/site-media/file/:slot", h.ServeFile)

	// Admin
	admin := v1.Group("/admin/site-media")
	admin.Use(
		authapi.RequireAuth(auth),
		authapi.RequireUserType(authapi.UserTypeStaff),
		authapi.RequirePermission("sitemedia.manage"),
	)
	{
		admin.GET("", h.ListAdmin)
		admin.POST("/:slot", h.Upload)
		admin.DELETE("/:slot", h.Delete)
	}
}
