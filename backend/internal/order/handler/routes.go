package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
)

// RegisterRoutes wires order endpoints under the given groups.
//
//   - `public` (no auth, rate limited): POST /orders (guest allowed), GET /lacak/:resi
//   - `v1` (root api group): mount GET /orders/:resi behind RequireAuth
//   - Admin endpoints under /admin/orders (staff-only + per-endpoint permission)
//
// Two groups needed because /orders/:resi requires authentication whereas
// POST /orders + /lacak/:resi are public.
func (h *Handler) RegisterRoutes(v1, public *gin.RouterGroup, auth authapi.Service) {
	// CreateOnline: auth OPSIONAL — kalau customer login, order tied ke
	// customer_id; kalau guest, resolve/create by nomor WA di body.
	public.POST("/orders", authapi.OptionalAuth(auth), h.CreateOnline)
	public.GET("/lacak/:resi", h.PublicTracking)

	protected := v1.Group("/orders")
	protected.Use(authapi.RequireAuth(auth))
	protected.GET("", h.ListMine)
	protected.GET("/:resi", h.GetByResi)

	// Admin routes — staff only. Each endpoint additionally gated by the
	// specific permission code (per spec section 10 — permissions modular per role).
	admin := v1.Group("/admin/orders")
	admin.Use(authapi.RequireAuth(auth), authapi.RequireUserType(authapi.UserTypeStaff))
	admin.GET("", authapi.RequirePermission("order.view"), h.AdminList)
	admin.GET("/:resi", authapi.RequirePermission("order.view"), h.AdminGetDetail)
	admin.POST("/:resi/shipping-cost", authapi.RequirePermission("shipping.set_cost"), h.AdminSetShippingCost)
	admin.POST("/:resi/confirm-pickup", authapi.RequirePermission("order.update_status"), h.AdminConfirmPickup)
	admin.POST("/:resi/cancel", authapi.RequirePermission("order.cancel"), h.AdminCancel)

	// Super admin order tools (§ super admin order tools) — edit data
	// pesanan, override status ke status manapun, soft-delete pesanan.
	admin.PATCH("/:resi", authapi.RequirePermission("order.edit"), h.AdminEditOrder)
	admin.POST("/:resi/override-status", authapi.RequirePermission("order.override_status"), h.AdminOverrideStatus)
	admin.DELETE("/:resi", authapi.RequirePermission("order.delete"), h.AdminDeleteOrder)

	// GET /admin/audit-log — bukan sub-resource /admin/orders, jadi mount
	// grup terpisah langsung di v1 (pola sama seperti modul lain mounting
	// beberapa /admin/* subgroup — lihat router.go).
	auditAdmin := v1.Group("/admin/audit-log")
	auditAdmin.Use(authapi.RequireAuth(auth), authapi.RequireUserType(authapi.UserTypeStaff))
	auditAdmin.GET("", authapi.RequirePermission("audit.view"), h.AdminListAuditLog)
}
