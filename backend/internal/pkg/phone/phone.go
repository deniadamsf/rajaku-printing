// Package phone normalizes & validates Indonesian mobile numbers to the
// canonical `62xxxxxxxxxx` format required across the system (spec section 13):
// link `wa.me/62xxx` dan API Baileys butuh format internasional tanpa `+`/`0`.
//
// Aturan konversi (idempotent):
//   - Strip semua whitespace, dash, dot, parenthesis.
//   - Leading `+62`  → `62`
//   - Leading `0`    → `62`
//   - Leading `62`   → biarkan
//   - Selain itu     → invalid.
//
// Validasi panjang: total 10–15 digit (rekomendasi ITU E.164), sisa setelah `62`
// harus 9–13 digit — cukup longgar untuk menampung provider ID variabel.
package phone

import (
	"errors"
	"strings"
	"unicode"
)

var (
	ErrEmpty           = errors.New("phone: empty")
	ErrContainsLetters = errors.New("phone: contains non-digit characters after cleanup")
	ErrInvalidPrefix   = errors.New("phone: must start with 0, 62, or +62")
	ErrInvalidLength   = errors.New("phone: invalid length (expected 10-15 digits)")
)

// Normalize returns the canonical `62xxxxxxxxxx` form, or an error explaining
// why the input is not a valid Indonesian mobile number.
func Normalize(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", ErrEmpty
	}

	cleaned := stripSeparators(raw)
	if cleaned == "" {
		return "", ErrEmpty
	}

	switch {
	case strings.HasPrefix(cleaned, "+62"):
		cleaned = "62" + cleaned[3:]
	case strings.HasPrefix(cleaned, "62"):
		// already canonical prefix
	case strings.HasPrefix(cleaned, "0"):
		cleaned = "62" + cleaned[1:]
	case strings.HasPrefix(cleaned, "8"):
		cleaned = "62" + cleaned
	default:
		return "", ErrInvalidPrefix
	}

	for _, r := range cleaned {
		if !unicode.IsDigit(r) {
			return "", ErrContainsLetters
		}
	}

	if len(cleaned) < 10 || len(cleaned) > 15 {
		return "", ErrInvalidLength
	}

	return cleaned, nil
}

// MustNormalize panics on invalid input. Use only for test fixtures / seed data
// where the input is a compile-time constant guaranteed to be valid.
func MustNormalize(raw string) string {
	n, err := Normalize(raw)
	if err != nil {
		panic("phone.MustNormalize: " + err.Error() + " (input=" + raw + ")")
	}
	return n
}

// Mask censors a canonical `62xxxxxxxxxx` number for display in
// public-facing responses (spec section 5: "0812****678") — e.g. the
// phone_masked field returned by POST /auth/google/request-otp, or public
// tracking pages. Returns "" if p62 isn't in the expected canonical shape
// (don't leak partial data for unexpected input).
func Mask(p62 string) string {
	if !strings.HasPrefix(p62, "62") || len(p62) < 6 {
		return ""
	}
	display := "0" + p62[2:] // 62xxx → 0xxx, matches how numbers are shown to Indonesian users
	n := len(display)
	if n <= 7 {
		return display[:2] + strings.Repeat("*", n-2)
	}
	head := display[:4]
	tail := display[n-3:]
	return head + "****" + tail
}

// NormalizeForSearch applies the same 0/+62→62 prefix rewrite as Normalize,
// but WITHOUT digit/length validation — meant for partial-match search
// queries (mis. POS customer search, §11) where the kasir may still be
// mid-typing a number ("0812" while typing, not yet a full 10-15 digit
// number). Separators (whitespace/dash/dot/parens — see stripSeparators) are
// ALWAYS stripped, even when the input doesn't start with 0/62/+62 — e.g. a
// number pasted from WhatsApp as "812-3456-7890" (no leading 0/62/+, a common
// way numbers get shared) must still come out digits-only so it can match the
// canonically-stored phone column. This is safe for non-phone-shaped queries
// too: the caller (user_repository.go's SearchCustomers) ONLY ever uses this
// function's output to build the phone-matching LIKE pattern — a separate,
// untouched copy of the raw query is used for name matching — so whatever
// this returns for e.g. a customer's name never affects search correctness.
func NormalizeForSearch(raw string) string {
	trimmed := strings.TrimSpace(raw)
	cleaned := stripSeparators(trimmed)
	switch {
	case strings.HasPrefix(cleaned, "+62"):
		return "62" + cleaned[3:]
	case strings.HasPrefix(cleaned, "62"):
		return cleaned
	case strings.HasPrefix(cleaned, "0"):
		return "62" + cleaned[1:]
	default:
		return cleaned
	}
}

func stripSeparators(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case ' ', '\t', '\n', '-', '.', '(', ')':
			continue
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
