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
// Deny-by-default: tokens carrying a non-empty Scope (mis. ScopeGuestOrder)
// are REJECTED here (401) — a scoped/limited token must never work on a
// generic "requires session" endpoint. Endpoints that intentionally accept a
// scoped token must opt in explicitly via RequireAuthAllowScope.
//
// Downstream handlers retrieve it via IdentityFromContext(c.Request.Context()).
func RequireAuth(svc Service) gin.HandlerFunc {
	return requireAuth(svc, nil)
}

// RequireAuthAllowScope behaves like RequireAuth but additionally accepts
// tokens whose Scope matches one of `allowed` (on top of full sessions, i.e.
// Scope == ""). Use this ONLY on endpoints that were deliberately designed to
// accept a limited-purpose token (mis. desain routes accepting
// ScopeGuestOrder) — never as a blanket replacement for RequireAuth.
func RequireAuthAllowScope(svc Service, allowed ...string) gin.HandlerFunc {
	return requireAuth(svc, allowed)
}

func requireAuth(svc Service, allowedScopes []string) gin.HandlerFunc {
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

		if id.Scope != "" && !scopeAllowed(id.Scope, allowedScopes) {
			httpx.Error(c, 401, httpx.CodeUnauthorized, "token scope not allowed for this endpoint")
			return
		}

		ctx := WithIdentity(c.Request.Context(), id)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

func scopeAllowed(scope string, allowed []string) bool {
	for _, a := range allowed {
		if a == scope {
			return true
		}
	}
	return false
}

// OptionalAuth extracts the Bearer token if present + valid and attaches
// Identity to context. Kalau tidak ada header / token invalid, request tetap
// lolos (tanpa identity). Dipakai endpoint publik yang bisa berbeda perilaku
// untuk logged-in vs guest, mis. POST /orders (customer login → tied to
// customer_id, guest → resolved via nomor WA di body).
//
// Deny-by-default sama seperti RequireAuth: token BER-SCOPE (mis.
// ScopeGuestOrder) diperlakukan sebagai anonim di sini — TIDAK ditolak
// (endpoint ini memang boleh diakses tanpa auth), tapi Identity-nya juga
// TIDAK di-attach. Tanpa ini, token guest_order yang sempit (dibuat untuk
// upload/approve desain SATU order) akan diam-diam berfungsi sebagai sesi
// penuh di endpoint manapun yang memakai OptionalAuth — melanggar batas
// scope yang sama yang ditegakkan RequireAuth.
func OptionalAuth(svc Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := extractBearer(c.GetHeader("Authorization"))
		if raw == "" {
			c.Next()
			return
		}
		id, err := svc.VerifyToken(c.Request.Context(), raw)
		if err != nil || id.Scope != "" {
			// Token invalid, OR valid-but-scoped → treat as anonymous
			// (best-effort). Handler yg benar-benar butuh auth pakai
			// RequireAuth/RequireAuthAllowScope, bukan ini.
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
