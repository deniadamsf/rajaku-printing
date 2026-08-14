package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/auth/token"
)

// Unit-level test: register-then-login happy path uses in-memory fake repo &
// real issuer + real bcrypt (fast enough). DB-dependent tests live under
// integration/ (menyusul saat CI diseting).

func TestRegisterCustomer_ValidationErrors(t *testing.T) {
	svc := newTestService(t)

	_, _, err := svc.RegisterCustomer(context.Background(), RegisterCustomerInput{
		Email: "not-an-email", Phone: "081234567890", Name: "X", Password: "abcd1234",
	})
	if err == nil {
		t.Fatal("expected error for invalid email, got nil")
	}
	if !IsBadInput(err) {
		t.Fatalf("expected IsBadInput true, got err=%v", err)
	}

	_, _, err = svc.RegisterCustomer(context.Background(), RegisterCustomerInput{
		Email: "a@b.com", Phone: "081234567890", Name: "X", Password: "short",
	})
	if err == nil || !strings.Contains(err.Error(), "password") {
		t.Fatalf("expected password strength error, got %v", err)
	}

	_, _, err = svc.RegisterCustomer(context.Background(), RegisterCustomerInput{
		Email: "a@b.com", Phone: "not-a-phone", Name: "X", Password: "abcd12345",
	})
	if err == nil {
		t.Fatal("expected phone error, got nil")
	}
}

func TestLogin_WrongPassword_ReturnsGenericError(t *testing.T) {
	// Setup: register a user, then attempt login with wrong password.
	// Verifies bahwa service tidak bocorkan info apakah email/password yg salah.
	// NOTE: butuh real DB — skip untuk unit test murni. Test ini adalah placeholder
	// yang di-run di integration suite.
	t.Skip("requires DB; integration test")
}

func TestVerifyToken_InvalidReturnsAuthapiError(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.VerifyToken(context.Background(), "garbage.token.here")
	if !errors.Is(err, authapi.ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

// TestVerifyToken_RealSignedScopedToken_PropagatesScopeAndOrderID is a
// regression test for a review finding: nothing exercised VerifyToken against
// a genuinely-signed token carrying a Scope, even though the ENTIRE
// deny-by-default guarantee (RequireAuth rejecting scoped tokens,
// RequireAuthAllowScope filtering them, OptionalAuth treating them as
// anonymous) depends on Service.VerifyToken actually copying claims.Scope
// (and now claims.OrderID) into the returned Identity. If that one line were
// ever deleted, every other test in this repo would stay green while a guest
// checkout token silently became valid everywhere. Must go through the real
// Sign → Verify round trip, not a fake.
func TestVerifyToken_RealSignedScopedToken_PropagatesScopeAndOrderID(t *testing.T) {
	svc := newTestService(t)
	uid := uuid.New()
	orderID := uuid.New()

	signed, err := svc.issuer.Sign(token.Claims{
		UserID:   uid.String(),
		UserType: string(authapi.UserTypeCustomer),
		Scope:    authapi.ScopeGuestOrder,
		OrderID:  orderID.String(),
	})
	if err != nil {
		t.Fatalf("sign test token: %v", err)
	}

	id, err := svc.VerifyToken(context.Background(), signed)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if id.Scope != authapi.ScopeGuestOrder {
		t.Fatalf("expected Identity.Scope=%q, got %q — deny-by-default in RequireAuth/OptionalAuth depends on this", authapi.ScopeGuestOrder, id.Scope)
	}
	if id.OrderID == nil || *id.OrderID != orderID {
		t.Fatalf("expected Identity.OrderID=%s, got %v", orderID, id.OrderID)
	}
}

// TestVerifyToken_FullSessionToken_HasEmptyScopeAndNilOrderID guards the
// other half of the same guarantee: a token WITHOUT scp/oid claims (the
// normal login/register path) must decode to an empty Scope and nil
// OrderID — otherwise every ordinary session would spuriously fail the
// scope checks above.
func TestVerifyToken_FullSessionToken_HasEmptyScopeAndNilOrderID(t *testing.T) {
	svc := newTestService(t)
	uid := uuid.New()

	signed, err := svc.issuer.Sign(token.Claims{
		UserID:   uid.String(),
		UserType: string(authapi.UserTypeCustomer),
	})
	if err != nil {
		t.Fatalf("sign test token: %v", err)
	}

	id, err := svc.VerifyToken(context.Background(), signed)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if id.Scope != "" {
		t.Fatalf("expected empty Scope for a full session token, got %q", id.Scope)
	}
	if id.OrderID != nil {
		t.Fatalf("expected nil OrderID for a full session token, got %v", id.OrderID)
	}
}
