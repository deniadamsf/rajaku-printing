package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
)

// RegisterRoutes mounts the auth module's HTTP routes under the given group.
// `svc` is passed so RequireAuth can call VerifyToken via the interface.
func (h *Handler) RegisterRoutes(v1 *gin.RouterGroup, svc authapi.Service) {
	authGroup := v1.Group("/auth")
	{
		authGroup.POST("/register", h.RegisterCustomer)
		authGroup.POST("/login", h.Login)
		authGroup.POST("/logout", h.Logout)

		// Protected — requires valid access token.
		protected := authGroup.Group("")
		protected.Use(authapi.RequireAuth(svc))
		protected.GET("/me", h.Me)
	}
}

// RegisterAdminRoutes mounts /admin/staff, /admin/roles, /admin/permissions,
// dan /invites/*. Terpisah dari RegisterRoutes karena butuh AdminHandler
// (yg wiring extra service — staff/role/invite). Semua /admin/* butuh
// staff.manage atau role.manage (per endpoint). /invites/* PUBLIC — dipakai
// staff dari URL yg dikirim ke email/WA (belum ada auth di titik itu).
func (h *AdminHandler) RegisterRoutes(v1 *gin.RouterGroup, svc authapi.Service) {
	// Public — staff accept invite dari link email/WA (belum ada session).
	inv := v1.Group("/invites")
	{
		inv.GET("/verify", h.VerifyInvite)
		inv.POST("/accept", h.AcceptInvite)
	}

	// Admin — semua wajib login staff + permission tertentu.
	admin := v1.Group("/admin")
	admin.Use(authapi.RequireAuth(svc), authapi.RequireUserType(authapi.UserTypeStaff))

	staff := admin.Group("/staff")
	staff.Use(authapi.RequirePermission("staff.manage"))
	{
		staff.POST("", h.CreateStaff)
		staff.GET("", h.ListStaff)
		staff.GET("/:id", h.GetStaff)
		staff.PATCH("/:id", h.UpdateStaff)
		staff.POST("/:id/deactivate", h.DeactivateStaff)
		staff.POST("/:id/activate", h.ActivateStaff)
		staff.POST("/:id/roles", h.AssignRoles)
	}

	// Roles + permissions — role.manage.
	rolesG := admin.Group("/roles")
	rolesG.Use(authapi.RequirePermission("role.manage"))
	{
		rolesG.GET("", h.ListRoles)
		rolesG.POST("", h.CreateRole)
		rolesG.GET("/:id", h.GetRole)
		rolesG.PATCH("/:id", h.UpdateRole)
		rolesG.DELETE("/:id", h.DeleteRole)
		rolesG.PUT("/:id/permissions", h.SetRolePermissions)
	}
	admin.GET("/permissions",
		authapi.RequirePermission("role.manage"), h.ListPermissions)
}
