package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
)

// RegisterRoutes mounts catalog HTTP routes. Semua endpoint di sini publik
// (tidak butuh auth) — cukup rate-limited di router luar.
func (h *Handler) RegisterRoutes(v1 *gin.RouterGroup) {
	g := v1.Group("/catalog")
	{
		g.GET("/products", h.ListProducts)
		g.GET("/products/:slug", h.GetProductBySlug)
		g.GET("/materials", h.ListMaterials)
		g.POST("/quote", h.Quote)
	}
}

// RegisterAdminRoutes mounts admin CRUD endpoints — semua di-gate permission
// catalog.manage (§10). Mount di v1 (bukan public) supaya rate-limit publik
// tidak mengganggu operasional admin panel.
//
//	GET    /admin/catalog/materials                list all (incl. inactive)
//	POST   /admin/catalog/materials                create
//	PATCH  /admin/catalog/materials/:id            update
//	POST   /admin/catalog/materials/:id/activate
//	POST   /admin/catalog/materials/:id/deactivate
//
//	GET    /admin/catalog/products                 list all (incl. inactive)
//	POST   /admin/catalog/products                 create
//	GET    /admin/catalog/products/:id             detail incl. pricings
//	PATCH  /admin/catalog/products/:id             update
//	POST   /admin/catalog/products/:id/activate
//	POST   /admin/catalog/products/:id/deactivate
//
//	POST   /admin/catalog/products/:id/pricings    add pricing row
//	PATCH  /admin/catalog/pricings/:pid            update pricing row
//	POST   /admin/catalog/pricings/:pid/activate
//	POST   /admin/catalog/pricings/:pid/deactivate
//	DELETE /admin/catalog/pricings/:pid            hard delete
func (h *Handler) RegisterAdminRoutes(v1 *gin.RouterGroup, auth authapi.Service) {
	g := v1.Group("/admin/catalog")
	g.Use(
		authapi.RequireAuth(auth),
		authapi.RequireUserType(authapi.UserTypeStaff),
		authapi.RequirePermission("catalog.manage"),
	)

	// Materials
	g.GET("/materials", h.AdminListMaterials)
	g.POST("/materials", h.AdminCreateMaterial)
	g.PATCH("/materials/:id", h.AdminUpdateMaterial)
	g.POST("/materials/:id/activate", h.AdminActivateMaterial)
	g.POST("/materials/:id/deactivate", h.AdminDeactivateMaterial)

	// Products
	g.GET("/products", h.AdminListProducts)
	g.POST("/products", h.AdminCreateProduct)
	g.GET("/products/:id", h.AdminGetProduct)
	g.PATCH("/products/:id", h.AdminUpdateProduct)
	g.POST("/products/:id/activate", h.AdminActivateProduct)
	g.POST("/products/:id/deactivate", h.AdminDeactivateProduct)

	// Pricings (nested create under product; individual ops use /pricings/:pid)
	g.POST("/products/:id/pricings", h.AdminCreatePricing)
	g.PATCH("/pricings/:pid", h.AdminUpdatePricing)
	g.POST("/pricings/:pid/activate", h.AdminActivatePricing)
	g.POST("/pricings/:pid/deactivate", h.AdminDeactivatePricing)
	g.DELETE("/pricings/:pid", h.AdminDeletePricing)
}
