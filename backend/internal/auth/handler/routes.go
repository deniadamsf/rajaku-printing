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

// RegisterRoutes mounts the Google OAuth login/registration endpoints.
// `public` gets the general public rate limit (see internal/server/router.go)
// — /start and /callback are browser navigations reachable by anyone,
// /exchange accepts a handoff code with no attacker-controlled identifier to
// brute-force. `otpLimited` MUST carry a much stricter, dedicated limiter
// (RATE_LIMIT_OTP_*): /request-otp and /complete both accept an arbitrary
// WhatsApp number / numeric OTP guess in an unauthenticated POST body — the
// same brute-force shape as POST /lacak/:resi/verify's guestVerify group.
func (h *GoogleOAuthHandler) RegisterRoutes(public, otpLimited *gin.RouterGroup) {
	g := public.Group("/auth/google")
	{
		g.GET("/start", h.Start)
		g.GET("/callback", h.Callback)
		g.POST("/exchange", h.Exchange)
	}
	ol := otpLimited.Group("/auth/google")
	{
		ol.POST("/request-otp", h.RequestOTP)
		ol.POST("/complete", h.Complete)
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

// RegisterRoutes mounts /admin/customers dan /admin/customers-export (fitur
// "Manajemen Pelanggan"). /admin/customers-export dipasang sebagai grup
// TERPISAH (bukan sub-path /admin/customers/export) dengan alasan yang PERSIS
// sama dengan order/handler/recap_handler.go (§28.5): grup /admin/customers
// sudah punya path wildcard GET /:id, dan menaruh path statis "export"
// bersebelahan dengan wildcard di level yang sama mengundang bentrok routing
// (gin akan menganggap "export" sebagai nilai :id kalau di-nest di bawahnya).
func (h *CustomerAdminHandler) RegisterRoutes(v1 *gin.RouterGroup, svc authapi.Service) {
	admin := v1.Group("/admin")
	admin.Use(authapi.RequireAuth(svc), authapi.RequireUserType(authapi.UserTypeStaff))

	customers := admin.Group("/customers")
	{
		customers.GET("", authapi.RequirePermission("customer.view"), h.ListCustomers)
		customers.GET("/:id", authapi.RequirePermission("customer.view"), h.GetCustomer)
		customers.PATCH("/:id", authapi.RequirePermission("customer.manage"), h.UpdateCustomer)
		customers.POST("/:id/deactivate", authapi.RequirePermission("customer.manage"), h.DeactivateCustomer)
		customers.POST("/:id/activate", authapi.RequirePermission("customer.manage"), h.ActivateCustomer)
	}

	admin.GET("/customers-export", authapi.RequirePermission("customer.view"), h.ExportCustomers)
}
