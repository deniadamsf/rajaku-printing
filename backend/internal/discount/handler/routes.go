package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
)

// RegisterRoutes wires discount endpoints under /admin/discounts (§28.6) —
// all staff-only.
//
// PENTING soal urutan (§28 brief): "/applicable" adalah path statis yang
// bertetangga dengan "/:id" — didaftarkan SEBELUM "/:id" di bawah supaya gin
// menempatkannya sebagai node statis (tidak pernah tertangkap oleh wildcard
// ":id"). gin/httprouter memang mengizinkan static+wildcard hidup
// berdampingan di segmen yang sama untuk method yang sama (pola umum lain di
// codebase ini: /me vs /:id di modul lain) — routes_test.go memverifikasi ini
// tidak panic saat registrasi.
func (h *Handler) RegisterRoutes(v1 *gin.RouterGroup, auth authapi.Service) {
	admin := v1.Group("/admin/discounts")
	admin.Use(authapi.RequireAuth(auth), authapi.RequireUserType(authapi.UserTypeStaff))

	admin.GET("", authapi.RequirePermission("discount.manage"), h.List)
	admin.POST("", authapi.RequirePermission("discount.manage"), h.Create)

	// GET /applicable — permission BERBEDA (discount.apply): kasir yang
	// boleh MEMAKAI diskon belum tentu boleh MENGELOLA master diskon.
	admin.GET("/applicable", authapi.RequirePermission("discount.apply"), h.Applicable)

	admin.GET("/:id", authapi.RequirePermission("discount.manage"), h.Get)
	admin.PATCH("/:id", authapi.RequirePermission("discount.manage"), h.Update)
	admin.DELETE("/:id", authapi.RequirePermission("discount.manage"), h.Delete)
}
