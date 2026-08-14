package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
)

// RegisterRoutes wires invoice endpoints.
//
// Public:
//   GET  /invoices/:id/pdf                stream PDF (UUID unguessable = de-facto signed)
//
// Staff-only:
//   POST /admin/orders/:resi/invoice      manual regenerate (idempotent, atau force baru)
func (h *Handler) RegisterRoutes(v1 *gin.RouterGroup, auth authapi.Service) {
	// Public download — no auth. UUID sebagai token; ditambahkan ke public
	// group biar rate-limit publik jaga dari brute-force scan UUID.
	v1.GET("/invoices/:id/pdf", h.DownloadPDF)

	admin := v1.Group("/admin/orders")
	admin.Use(
		authapi.RequireAuth(auth),
		authapi.RequireUserType(authapi.UserTypeStaff),
		authapi.RequirePermission("order.update_status"),
	)
	admin.POST("/:resi/invoice", h.AdminGenerate)
}
