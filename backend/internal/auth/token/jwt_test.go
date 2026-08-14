package token

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const testSecret = "test-secret-32-chars-minimum-abcdef"

func newTestIssuer(t *testing.T, ttl time.Duration) *Issuer {
	t.Helper()
	i, err := NewIssuer(testSecret, ttl, "rajaku-test")
	if err != nil {
		t.Fatalf("NewIssuer: %v", err)
	}
	return i
}

func TestSignVerify_Roundtrip(t *testing.T) {
	i := newTestIssuer(t, time.Hour)
	orig := Claims{
		UserID:      "user-123",
		UserType:    "staff",
		Email:       "kasir@example.com",
		Roles:       []string{"cashier"},
		Permissions: []string{"pos.create_order", "order.view"},
	}
	tok, err := i.Sign(orig)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	got, err := i.Verify(tok)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got.UserID != orig.UserID || got.UserType != orig.UserType || got.Email != orig.Email {
		t.Fatalf("claims mismatch: got %+v want %+v", got, orig)
	}
	if len(got.Roles) != 1 || got.Roles[0] != "cashier" {
		t.Fatalf("roles mismatch: %+v", got.Roles)
	}
}

func TestVerify_Expired(t *testing.T) {
	i := newTestIssuer(t, time.Millisecond)
	tok, _ := i.Sign(Claims{UserID: "u", UserType: "customer"})
	time.Sleep(5 * time.Millisecond)
	_, err := i.Verify(tok)
	if !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("expected ErrExpiredToken, got %v", err)
	}
}

func TestVerify_Malformed(t *testing.T) {
	i := newTestIssuer(t, time.Hour)
	_, err := i.Verify("this.is.not-a-jwt")
	if !errors.Is(err, ErrMalformedToken) && !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected malformed/invalid, got %v", err)
	}
}

func TestVerify_WrongSecret(t *testing.T) {
	i1 := newTestIssuer(t, time.Hour)
	i2, _ := NewIssuer(strings.Repeat("x", 32), time.Hour, "rajaku-test")
	tok, _ := i1.Sign(Claims{UserID: "u", UserType: "customer"})
	_, err := i2.Verify(tok)
	if err == nil {
		t.Fatal("expected error verifying with wrong secret, got nil")
	}
}

func TestNewIssuer_SecretTooShort(t *testing.T) {
	_, err := NewIssuer("short", time.Hour, "iss")
	if err == nil {
		t.Fatal("expected error for short secret, got nil")
	}
}
