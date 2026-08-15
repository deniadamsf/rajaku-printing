package phone

import (
	"errors"
	"testing"
)

func TestNormalize_HappyPath(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"leading zero", "081234567890", "6281234567890"},
		{"already canonical", "6281234567890", "6281234567890"},
		{"with plus prefix", "+6281234567890", "6281234567890"},
		{"with dashes", "0812-3456-7890", "6281234567890"},
		{"with spaces", "0812 3456 7890", "6281234567890"},
		{"with parens & dots", "(0812).3456.7890", "6281234567890"},
		{"idempotent", "6281234567890", "6281234567890"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Normalize(tc.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNormalize_Errors(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want error
	}{
		{"empty", "", ErrEmpty},
		{"whitespace only", "   \t", ErrEmpty},
		{"letters", "0812ABCD5678", ErrContainsLetters},
		{"bad prefix", "1234567890", ErrInvalidPrefix},
		{"too short", "0812345", ErrInvalidLength},
		{"too long", "081234567890123456", ErrInvalidLength},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Normalize(tc.in)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got err=%v, want %v", err, tc.want)
			}
		})
	}
}

func TestMask(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"typical 13-digit", "6281234567890", "0812****890"},
		{"short number", "628123", "08***"},
		{"unexpected prefix", "0812345678", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Mask(tc.in); got != tc.want {
				t.Fatalf("Mask(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestMustNormalize_PanicsOnInvalid(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic, got none")
		}
	}()
	MustNormalize("not-a-phone")
}
