package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
)

// RegisterRoutes wires design endpoints.
//
// Customer-facing (auth req, tanpa permission check — ownership di service):
//   POST   /orders/:resi/design-files             upload file (upload path atau asset)
//   GET    /orders/:resi/design-files             list file untuk order
//   POST   /design-drafts/:id/approve             approve staff draft (request path)
//   POST   /design-drafts/:id/revision            request revision (request path)
//   GET    /design-files/:id/file                 stream file (auth-guarded, ownership di service)
//
// Kelima rute di atas menerima BAIK sesi penuh (login/register) MAUPUN token
// guest-checkout scope=guest_order (POST /lacak/:resi/verify) — guest tanpa
// password bisa upload/approve desain sepanjang mereka bisa buktikan
// kepemilikan order (resi + no. WA). Ownership check tetap sepenuhnya di
// service (id.UserID == order.CustomerID) — token guest membawa uid customer
// asli, jadi cek itu otomatis lolos tanpa perubahan.
//
// Staff/admin (permission-based):
//   POST   /admin/orders/:resi/design-drafts          staff upload draft (design.work)
//   POST   /admin/orders/:resi/design-verify          staff verify customer upload (design.approve)
//   POST   /admin/orders/:resi/design-walkin-approve  POS instant approve (§11) (design.approve)
func (h *Handler) RegisterRoutes(v1 *gin.RouterGroup, auth authapi.Service) {
	authed := v1.Group("")
	authed.Use(authapi.RequireAuthAllowScope(auth, authapi.ScopeGuestOrder))
	{
		authed.POST("/orders/:resi/design-files", h.UploadCustomerFile)
		authed.GET("/orders/:resi/design-files", h.List)
		authed.POST("/design-drafts/:id/approve", h.ApproveDraft)
		authed.POST("/design-drafts/:id/revision", h.RequestRevision)
		authed.GET("/design-files/:id/file", h.GetFile)
	}

	admin := v1.Group("/admin/orders/:resi")
	admin.Use(authapi.RequireAuth(auth), authapi.RequireUserType(authapi.UserTypeStaff))
	{
		admin.POST("/design-drafts",
			authapi.RequirePermission("design.work"), h.StaffUploadDraft)
		admin.POST("/design-verify",
			authapi.RequirePermission("design.approve"), h.StaffVerifyUpload)
		admin.POST("/design-walkin-approve",
			authapi.RequirePermission("design.approve"), h.StaffApproveWalkin)
	}
}
