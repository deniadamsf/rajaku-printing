// Package password wraps bcrypt with sensible defaults & explicit errors.
// Cost dipilih 12 (aman s/d ~2030, ~250ms/hash di CPU modern).
package password

import (
	"errors"
	"fmt"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const (
	// DefaultCost = 12 → ~250ms/hash. Naikkan seiring waktu (moving target).
	DefaultCost = 12

	// MinLength minimum length enforced at API boundary. Naikkan sesuai policy
	// keamanan (mis. 10-12 untuk staff dengan akses finansial — section 10).
	MinLength = 8
	// MaxLength bcrypt algo hanya menggunakan 72 byte pertama; explicit reject
	// input yang > 72 supaya user tidak tertipu password panjang yang efektif dipotong.
	MaxLength = 72
)

var (
	ErrTooShort      = fmt.Errorf("password: kurang dari %d karakter", MinLength)
	ErrTooLong       = fmt.Errorf("password: lebih dari %d karakter (batas bcrypt)", MaxLength)
	ErrMismatch      = errors.New("password: tidak cocok")
)

// Hash returns the bcrypt hash of the plaintext password. Caller MUST validate
// length via ValidateStrength first — Hash trusts its input.
func Hash(plain string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(plain), DefaultCost)
	if err != nil {
		return "", fmt.Errorf("bcrypt hash: %w", err)
	}
	return string(h), nil
}

// Verify returns nil iff `plain` matches `hash`. Returns ErrMismatch (mapped to
// 401 di handler) on mismatch, and wrapped error on unexpected bcrypt failure.
func Verify(hash, plain string) error {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain))
	if err == nil {
		return nil
	}
	if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		return ErrMismatch
	}
	return fmt.Errorf("bcrypt compare: %w", err)
}

// ValidateStrength enforces min/max length. Extend here if password policy
// tightens (mis. wajib campuran huruf/angka/simbol).
func ValidateStrength(plain string) error {
	n := utf8.RuneCountInString(plain)
	if n < MinLength {
		return ErrTooShort
	}
	if len(plain) > MaxLength { // byte length — bcrypt constraint is on bytes
		return ErrTooLong
	}
	return nil
}
