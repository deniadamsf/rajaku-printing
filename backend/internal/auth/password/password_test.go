package password

import (
	"errors"
	"strings"
	"testing"
)

func TestHashAndVerify_HappyPath(t *testing.T) {
	h, err := Hash("correct-horse")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if err := Verify(h, "correct-horse"); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func TestVerify_Mismatch(t *testing.T) {
	h, _ := Hash("secret-one")
	err := Verify(h, "secret-two")
	if !errors.Is(err, ErrMismatch) {
		t.Fatalf("expected ErrMismatch, got %v", err)
	}
}

func TestValidateStrength(t *testing.T) {
	if err := ValidateStrength("2short"); !errors.Is(err, ErrTooShort) {
		t.Fatalf("expected ErrTooShort, got %v", err)
	}
	if err := ValidateStrength(strings.Repeat("a", MaxLength+1)); !errors.Is(err, ErrTooLong) {
		t.Fatalf("expected ErrTooLong, got %v", err)
	}
	if err := ValidateStrength("okpassword"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}
