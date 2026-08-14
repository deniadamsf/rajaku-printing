package authapi

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/rajaku-printing/backend/internal/httpx"
)

// RequireAuth returns a gin middleware that extracts the Bearer token from the
// Authorization header, verifies it via Service, and attaches the resulting
// Identity to both the gin context and the request context.
//
// Downstream handlers retrieve it via IdentityFromContext(c.Request.Context()).
func RequireAuth(svc Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := extractBearer(c.GetHeader("Authorization"))
		if raw == "" {
			httpx.Error(c, 401, httpx.CodeUnauthorized, "missing or invalid Authorization header")
			return
		}

		id, err := svc.VerifyToken(c.Request.Context(), raw)
		if err != nil {
			// Distinguish expired vs invalid so client tahu apakah perlu refresh
			// (refresh flow menyusul — untuk sekarang cukup message berbeda).
			msg := "invalid token"
			if errors.Is(err, ErrExpiredToken) {
				msg = "token expired"
			}
			httpx.Error(c, 401, httpx.CodeUnauthorized, msg)
			return
		}

		ctx := WithIdentity(c.Request.Context(), id)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// OptionalAuth extracts the Bearer token if present + valid and attaches
// Identity to context. Kalau tidak ada header / token invalid, request tetap
// lolos (tanpa identity). Dipakai endpoint publik yang bisa berbeda perilaku
// untuk logged-in vs guest, mis. POST /orders (customer login → tied to
// customer_id, guest → resolved via nomor WA di body).
func OptionalAuth(svc Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := extractBearer(c.GetHeader("Authorization"))
		if raw == "" {
			c.Next()
			return
		}
		id, err := svc.VerifyToken(c.Request.Context(), raw)
		if err != nil {
			// Token invalid → treat as anonymous (best-effort). Handler yg
			// benar-benar butuh auth pakai RequireAuth, bukan ini.
			c.Next()
			return
		}
		ctx := WithIdentity(c.Request.Context(), id)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// RequirePermission enforces that the caller's Identity has `code`. Must be
// mounted AFTER RequireAuth in the middleware chain — errors 401 if no identity,
// 403 if identity present but lacking the permission.
func RequirePermission(code string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := IdentityFromContext(c.Request.Context())
		if err != nil {
			httpx.Error(c, 401, httpx.CodeUnauthorized, "authentication required")
			return
		}
		if !id.HasPermission(code) {
			httpx.Error(c, 403, httpx.CodeForbidden, "missing permission: "+code)
			return
		}
		c.Next()
	}
}

// RequireUserType enforces the caller is of the given type (mis. hanya staff
// yang boleh akses admin panel endpoints, atau hanya customer yang boleh akses
// customer profile).
func RequireUserType(want UserType) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := IdentityFromContext(c.Request.Context())
		if err != nil {
			httpx.Error(c, 401, httpx.CodeUnauthorized, "authentication required")
			return
		}
		if id.UserType != want {
			httpx.Error(c, 403, httpx.CodeForbidden, "endpoint restricted to user_type="+string(want))
			return
		}
		c.Next()
	}
}

func extractBearer(header string) string {
	const prefix = "Bearer "
	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}
