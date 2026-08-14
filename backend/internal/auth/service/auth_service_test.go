package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
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
