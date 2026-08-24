package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/discount/handler"
)

// stubAuthService — minimal authapi.Service so RegisterRoutes can wire
// middleware without a real auth module. VerifyToken always fails (every
// test request below is anonymous — we only care about ROUTE REGISTRATION
// not panicking, not the auth outcome).
type stubAuthService struct{}

func (stubAuthService) VerifyToken(ctx context.Context, raw string) (*authapi.Identity, error) {
	return nil, authapi.ErrInvalidToken
}

// TestRegisterRoutes_NoPanic guards against the exact routing hazard called
// out in §28 brief: "/applicable" (static) sits next to "/:id" (wildcard)
// under the same GET method — if gin's radix tree ever rejected that
// combination, RegisterRoutes would panic HERE, at registration time, not at
// request time.
func TestRegisterRoutes_NoPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	v1 := r.Group("/api/v1")

	h := handler.New(nil)

	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("RegisterRoutes panicked (likely a gin route conflict between /applicable and /:id): %v", rec)
		}
	}()
	h.RegisterRoutes(v1, stubAuthService{})

	// Sanity check: both routes actually respond (401, since no token — the
	// point is that gin ROUTED the request instead of 404'ing due to a tree
	// conflict).
	for _, path := range []string{
		"/api/v1/admin/discounts/applicable?channel=pos&subtotal=1000",
		"/api/v1/admin/discounts/11111111-1111-1111-1111-111111111111",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code == http.StatusNotFound {
			t.Fatalf("GET %s: got 404, expected route to exist (got routed to 401 unauthenticated instead)", path)
		}
	}
}
