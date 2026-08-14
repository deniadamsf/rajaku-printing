package authapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
)

// fakeService is a minimal authapi.Service — VerifyToken always returns the
// preconfigured identity/error regardless of the raw token string (the
// middleware only cares about what VerifyToken returns, not JWT internals —
// those are covered by internal/auth/token tests).
type fakeService struct {
	identity *authapi.Identity
	err      error
}

func (f *fakeService) VerifyToken(_ context.Context, _ string) (*authapi.Identity, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.identity, nil
}

func newProtectedRouter(mw gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/protected", mw, func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return r
}

func doAuthedGet(r *gin.Engine) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer some-token")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestRequireAuth_RejectsScopedToken(t *testing.T) {
	svc := &fakeService{identity: &authapi.Identity{
		UserID: uuid.New(), UserType: authapi.UserTypeCustomer, Scope: authapi.ScopeGuestOrder,
	}}
	rec := doAuthedGet(newProtectedRouter(authapi.RequireAuth(svc)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("RequireAuth should reject scope=%q token, got status %d body=%s",
			authapi.ScopeGuestOrder, rec.Code, rec.Body.String())
	}
}

func TestRequireAuth_AllowsFullSessionToken(t *testing.T) {
	svc := &fakeService{identity: &authapi.Identity{
		UserID: uuid.New(), UserType: authapi.UserTypeCustomer, Scope: "",
	}}
	rec := doAuthedGet(newProtectedRouter(authapi.RequireAuth(svc)))
	if rec.Code != http.StatusOK {
		t.Fatalf("RequireAuth should allow full-session token, got status %d body=%s",
			rec.Code, rec.Body.String())
	}
}

func TestRequireAuthAllowScope_AcceptsAllowedScope(t *testing.T) {
	svc := &fakeService{identity: &authapi.Identity{
		UserID: uuid.New(), UserType: authapi.UserTypeCustomer, Scope: authapi.ScopeGuestOrder,
	}}
	mw := authapi.RequireAuthAllowScope(svc, authapi.ScopeGuestOrder)
	rec := doAuthedGet(newProtectedRouter(mw))
	if rec.Code != http.StatusOK {
		t.Fatalf("RequireAuthAllowScope(ScopeGuestOrder) should accept scope=%q, got status %d body=%s",
			authapi.ScopeGuestOrder, rec.Code, rec.Body.String())
	}
}

func TestRequireAuthAllowScope_AcceptsFullSessionToken(t *testing.T) {
	svc := &fakeService{identity: &authapi.Identity{
		UserID: uuid.New(), UserType: authapi.UserTypeStaff, Scope: "",
	}}
	mw := authapi.RequireAuthAllowScope(svc, authapi.ScopeGuestOrder)
	rec := doAuthedGet(newProtectedRouter(mw))
	if rec.Code != http.StatusOK {
		t.Fatalf("RequireAuthAllowScope should still accept a full session (scope=\"\"), got status %d body=%s",
			rec.Code, rec.Body.String())
	}
}

// newOptionalRouter mounts OptionalAuth in front of a handler that reports
// whether an Identity was attached — needed because OptionalAuth (unlike
// RequireAuth) never rejects the request, so status code alone can't
// distinguish "attached" from "not attached".
func newOptionalRouter(mw gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/protected", mw, func(c *gin.Context) {
		if _, err := authapi.IdentityFromContext(c.Request.Context()); err != nil {
			c.String(http.StatusOK, "anonymous")
			return
		}
		c.String(http.StatusOK, "identified")
	})
	return r
}

func TestOptionalAuth_ScopedToken_NotAttached(t *testing.T) {
	// §4 review finding: a scope-limited token (mis. guest_order, meant only
	// for the 5 design routes) must NOT silently work as a full session on
	// endpoints using OptionalAuth (mis. POST /orders).
	svc := &fakeService{identity: &authapi.Identity{
		UserID: uuid.New(), UserType: authapi.UserTypeCustomer, Scope: authapi.ScopeGuestOrder,
	}}
	rec := doAuthedGet(newOptionalRouter(authapi.OptionalAuth(svc)))
	if rec.Code != http.StatusOK {
		t.Fatalf("OptionalAuth must never reject the request, got status %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "anonymous" {
		t.Fatalf("expected caller to be treated as anonymous for a scoped token, got body=%s", rec.Body.String())
	}
}

func TestOptionalAuth_FullSessionToken_Attached(t *testing.T) {
	svc := &fakeService{identity: &authapi.Identity{
		UserID: uuid.New(), UserType: authapi.UserTypeCustomer, Scope: "",
	}}
	rec := doAuthedGet(newOptionalRouter(authapi.OptionalAuth(svc)))
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "identified" {
		t.Fatalf("expected a full-session token to attach Identity, got body=%s", rec.Body.String())
	}
}

func TestRequireAuthAllowScope_RejectsDisallowedScope(t *testing.T) {
	svc := &fakeService{identity: &authapi.Identity{
		UserID: uuid.New(), UserType: authapi.UserTypeCustomer, Scope: "some_other_scope",
	}}
	mw := authapi.RequireAuthAllowScope(svc, authapi.ScopeGuestOrder)
	rec := doAuthedGet(newProtectedRouter(mw))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("RequireAuthAllowScope(ScopeGuestOrder) should reject an unrelated scope, got status %d body=%s",
			rec.Code, rec.Body.String())
	}
}
