package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
)

// RegisterRoutes wires membership endpoints (§30.2/§30.5):
//   - POST /account/membership/apply                       — customer, auth required
//   - GET  /account/membership                              — customer, auth required
//   - GET  /admin/membership                                — staff, permission membership.view
//   - POST /admin/membership/:id/approve|reject|revoke|reinstate — staff, permission membership.manage
func (h *Handler) RegisterRoutes(v1 *gin.RouterGroup, auth authapi.Service) {
	account := v1.Group("/account/membership")
	account.Use(authapi.RequireAuth(auth), authapi.RequireUserType(authapi.UserTypeCustomer))
	account.POST("/apply", h.Apply)
	account.GET("", h.GetOwn)

	admin := v1.Group("/admin/membership")
	admin.Use(authapi.RequireAuth(auth), authapi.RequireUserType(authapi.UserTypeStaff))
	admin.GET("", authapi.RequirePermission("membership.view"), h.List)
	admin.POST("/:id/approve", authapi.RequirePermission("membership.manage"), h.Approve)
	admin.POST("/:id/reject", authapi.RequirePermission("membership.manage"), h.Reject)
	admin.POST("/:id/revoke", authapi.RequirePermission("membership.manage"), h.Revoke)
	admin.POST("/:id/reinstate", authapi.RequirePermission("membership.manage"), h.Reinstate)
}
