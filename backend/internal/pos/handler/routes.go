package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
)

// RegisterRoutes wires POS endpoints (semua staff-only).
//
//	POST /admin/pos/orders             kasir input walk-in (perm pos.create_order)
//	GET  /admin/pos/reconciliation     laporan harian (perm pos.reconcile)
func (h *Handler) RegisterRoutes(v1 *gin.RouterGroup, auth authapi.Service) {
	pos := v1.Group("/admin/pos")
	pos.Use(
		authapi.RequireAuth(auth),
		authapi.RequireUserType(authapi.UserTypeStaff),
	)
	pos.POST("/orders",
		authapi.RequirePermission("pos.create_order"), h.CreateOrder)
	pos.GET("/reconciliation",
		authapi.RequirePermission("pos.reconcile"), h.Reconciliation)
}
