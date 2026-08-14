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
	ErrEmpty            = errors.New("phone: empty")
	ErrContainsLetters  = errors.New("phone: contains non-digit characters after cleanup")
	ErrInvalidPrefix    = errors.New("phone: must start with 0, 62, or +62")
	ErrInvalidLength    = errors.New("phone: invalid length (expected 10-15 digits)")
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
