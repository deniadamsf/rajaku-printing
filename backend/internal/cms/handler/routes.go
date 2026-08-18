package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
)

// RegisterRoutes — mount CMS endpoints.
//
// Public (no auth):
//   GET  /articles                     — list published (page/limit/q)
//   GET  /articles/:slug               — detail (published only, else 404)
//   GET  /cms/images/:id               — serve WebP file
//
// Admin (staff + article permissions):
//   GET    /admin/articles              article.view
//   POST   /admin/articles              article.create
//   GET    /admin/articles/:id          article.view
//   PUT    /admin/articles/:id          article.create   (edit = create scope)
//   POST   /admin/articles/:id/publish  article.publish
//   POST   /admin/articles/:id/archive  article.publish
//   DELETE /admin/articles/:id          article.publish  (destructive → guarded)
//   POST   /admin/articles/images       article.create   (upload gambar → webp)
func (h *Handler) RegisterRoutes(v1, public *gin.RouterGroup, auth authapi.Service) {
	// Public
	public.GET("/articles", h.ListPublic)
	public.GET("/articles/:slug", h.GetPublicBySlug)
	public.GET("/cms/images/:id", h.ServeImage)

	// Admin
	admin := v1.Group("/admin/articles")
	admin.Use(authapi.RequireAuth(auth), authapi.RequireUserType(authapi.UserTypeStaff))
	{
		admin.GET("",
			authapi.RequirePermission("article.view"), h.ListAdmin)
		admin.POST("",
			authapi.RequirePermission("article.create"), h.CreateArticle)
		admin.POST("/images",
			authapi.RequirePermission("article.create"), h.UploadImage)
		admin.GET("/:id",
			authapi.RequirePermission("article.view"), h.GetAdmin)
		admin.PUT("/:id",
			authapi.RequirePermission("article.create"), h.UpdateArticle)
		admin.POST("/:id/publish",
			authapi.RequirePermission("article.publish"), h.PublishArticle)
		admin.POST("/:id/archive",
			authapi.RequirePermission("article.publish"), h.ArchiveArticle)
		admin.DELETE("/:id",
			authapi.RequirePermission("article.publish"), h.DeleteArticle)
	}
}
