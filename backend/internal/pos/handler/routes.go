package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
)

// RegisterRoutes wires POS endpoints (semua staff-only).
//
//	POST /admin/pos/orders             kasir input walk-in (perm pos.create_order)
//	GET  /admin/pos/customers/search   cari pelanggan existing by nama/WA (perm pos.create_order)
//	GET  /admin/pos/reconciliation     laporan harian (perm pos.reconcile)
//	GET  /admin/pos/receipt-config     lebar kertas struk aktif (semua staff)
func (h *Handler) RegisterRoutes(v1 *gin.RouterGroup, auth authapi.Service) {
	pos := v1.Group("/admin/pos")
	pos.Use(
		authapi.RequireAuth(auth),
		authapi.RequireUserType(authapi.UserTypeStaff),
	)
	pos.POST("/orders",
		authapi.RequirePermission("pos.create_order"), h.CreateOrder)
	pos.GET("/customers/search",
		authapi.RequirePermission("pos.create_order"), h.SearchCustomers)
	pos.GET("/reconciliation",
		authapi.RequirePermission("pos.reconcile"), h.Reconciliation)
	// receipt-config sengaja TANPA RequirePermission tambahan (beda dari dua
	// route di atas): dipakai halaman detail order untuk fitur cetak ulang
	// struk, diakses lintas role (produksi, desain, kasir) — bukan cuma
	// kasir. Isinya cuma angka konfigurasi non-sensitif (lebar kertas), jadi
	// gate RequireAuth + RequireUserType(staff) saja sudah cukup; menaruhnya
	// di belakang permission settings.manage (super-admin-only) akan
	// memblokir staff lain yang sah butuh baca nilai ini.
	pos.GET("/receipt-config", h.ReceiptConfig)
}
