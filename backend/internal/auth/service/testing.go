package service

import (
	"testing"
	"time"

	"github.com/rajaku-printing/backend/internal/auth/token"
)

// newTestService constructs a Service instance suitable for unit tests that
// don't touch the database. Repositories are passed as nil — tests must only
// call methods that avoid DB access (e.g., VerifyToken, input-validation
// paths that fail before repo calls).
//
// For DB-touching tests (login happy path, register-then-login), use the
// integration test harness (TBD when CI is set up).
func newTestService(t *testing.T) *Service {
	t.Helper()
	iss, err := token.NewIssuer("test-secret-32-chars-minimum-abcdef", time.Hour, "rajaku-test")
	if err != nil {
		t.Fatalf("NewIssuer: %v", err)
	}
	return &Service{issuer: iss}
}
