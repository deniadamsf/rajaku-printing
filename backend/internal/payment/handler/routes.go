package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
)

// RegisterRoutes wires payment endpoints:
//
//   - POST /orders/:resi/payment-proof, GET /orders/:resi/payment-proofs, and
//     GET /payment-proofs/:id/file — all accept BOTH full customer/staff
//     sessions AND scope-limited guest-order tokens minted by
//     POST /lacak/:resi/verify (authapi.ScopeGuestOrder). A guest checkout
//     (§6) has no account, so it has no other way to reach
//     menunggu_verifikasi — without this it upload buktinya buntu total
//     (§7). Ownership + the token's scope-to-one-order restriction are
//     enforced in service (checkScopedOrder), not here. Mounted on v1 (not
//     public) so no rate-limit body cap interferes with multipart upload.
//   - Admin group /admin/payment-proofs — staff-only + per-endpoint permission.
func (h *Handler) RegisterRoutes(v1 *gin.RouterGroup, auth authapi.Service) {
	guestAllowed := v1.Group("")
	guestAllowed.Use(authapi.RequireAuthAllowScope(auth, authapi.ScopeGuestOrder))
	guestAllowed.POST("/orders/:resi/payment-proof", h.UploadProof)
	guestAllowed.GET("/orders/:resi/payment-proofs", h.ListForOrder)
	// Serve proof file — auth guarantees identity; service enforces ownership
	// (staff bypass) + scope-to-order restriction. Same endpoint used by admin
	// dashboard preview.
	guestAllowed.GET("/payment-proofs/:id/file", h.GetFile)

	admin := v1.Group("/admin/payment-proofs")
	admin.Use(authapi.RequireAuth(auth), authapi.RequireUserType(authapi.UserTypeStaff))
	admin.GET("", authapi.RequirePermission("payment.verify"), h.AdminList)
	admin.POST("/:id/verify", authapi.RequirePermission("payment.verify"), h.AdminApprove)
	admin.POST("/:id/reject", authapi.RequirePermission("payment.reject"), h.AdminReject)
}
