package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
)

// RegisterRoutes wires payment endpoints:
//
//   - POST /orders/:resi/payment-proof — auth required (customer OR staff); no
//     public rate limit (upload is per-order, natural throttle).
//   - Admin group /admin/payment-proofs — staff-only + per-endpoint permission.
func (h *Handler) RegisterRoutes(v1 *gin.RouterGroup, auth authapi.Service) {
	// Customer/staff upload — mounted on v1 (not public) so no rate-limit body cap
	// interferes with multipart upload.
	customer := v1.Group("")
	customer.Use(authapi.RequireAuth(auth))
	customer.POST("/orders/:resi/payment-proof", h.UploadProof)
	// Serve proof file — auth guarantees identity; service enforces ownership
	// (staff bypass). Same endpoint used by admin dashboard preview.
	customer.GET("/payment-proofs/:id/file", h.GetFile)

	admin := v1.Group("/admin/payment-proofs")
	admin.Use(authapi.RequireAuth(auth), authapi.RequireUserType(authapi.UserTypeStaff))
	admin.GET("", authapi.RequirePermission("payment.verify"), h.AdminList)
	admin.POST("/:id/verify", authapi.RequirePermission("payment.verify"), h.AdminApprove)
	admin.POST("/:id/reject", authapi.RequirePermission("payment.reject"), h.AdminReject)
}
