package config

import (
	"testing"
	"time"
)

// TestIsValidIPOrCIDR covers the TRUSTED_PROXIES fail-fast validation (§22 —
// config invalid at startup must error, not silently degrade at runtime).
// Exercised directly rather than through Load() because Load() requires a
// full set of unrelated required env vars (DB_*, JWT_SECRET, ...) to reach
// this check — a unit test on the pure parsing function is more focused and
// doesn't couple to the rest of Load()'s required inputs.
func TestIsValidIPOrCIDR(t *testing.T) {
	valid := []string{
		"127.0.0.1",
		"10.0.0.0/8",
		"::1",
		"2001:db8::/32",
	}
	for _, v := range valid {
		if !isValidIPOrCIDR(v) {
			t.Errorf("isValidIPOrCIDR(%q) = false, want true", v)
		}
	}

	invalid := []string{
		"",
		"not-an-ip",
		"127.0.0.1/999",
		"0.0.0.0/0/extra",
		"999.999.999.999",
	}
	for _, v := range invalid {
		if isValidIPOrCIDR(v) {
			t.Errorf("isValidIPOrCIDR(%q) = true, want false", v)
		}
	}
}

// TestValidateGoogleOAuth covers the three cases required by the Google
// OAuth fail-fast rule: fully empty (OAuth off, valid), fully filled with a
// valid absolute redirect URL (valid), and every "invalid" shape (partial
// fill, or a filled-but-non-absolute redirect URL).
func TestValidateGoogleOAuth(t *testing.T) {
	t.Run("all empty is valid (oauth disabled)", func(t *testing.T) {
		if errs := validateGoogleOAuth("", "", ""); len(errs) != 0 {
			t.Errorf("expected no errors, got %v", errs)
		}
	})

	t.Run("fully filled with absolute redirect URL is valid", func(t *testing.T) {
		errs := validateGoogleOAuth("client-id", "client-secret", "http://localhost:8080/api/v1/auth/google/callback")
		if len(errs) != 0 {
			t.Errorf("expected no errors, got %v", errs)
		}
	})

	t.Run("https redirect URL is valid", func(t *testing.T) {
		errs := validateGoogleOAuth("client-id", "client-secret", "https://rajakuprinting.id/api/v1/auth/google/callback")
		if len(errs) != 0 {
			t.Errorf("expected no errors, got %v", errs)
		}
	})

	partial := [][3]string{
		{"client-id", "", ""},
		{"", "client-secret", ""},
		{"", "", "http://localhost:8080/callback"},
		{"client-id", "client-secret", ""},
	}
	for _, p := range partial {
		t.Run("partial fill is invalid", func(t *testing.T) {
			if errs := validateGoogleOAuth(p[0], p[1], p[2]); len(errs) == 0 {
				t.Errorf("validateGoogleOAuth(%q, %q, %q): expected errors, got none", p[0], p[1], p[2])
			}
		})
	}

	invalidRedirect := []string{"not-a-url", "/relative/path", "localhost:8080/callback"}
	for _, r := range invalidRedirect {
		t.Run("fully filled with non-absolute redirect URL is invalid", func(t *testing.T) {
			if errs := validateGoogleOAuth("client-id", "client-secret", r); len(errs) == 0 {
				t.Errorf("validateGoogleOAuth with redirect %q: expected errors, got none", r)
			}
		})
	}
}

// TestParseOTPConfig covers the OTP WhatsApp-verification fail-fast
// validation (§22) — Google OAuth registration requires a valid OTP policy
// before the server can accept traffic.
func TestParseOTPConfig(t *testing.T) {
	t.Run("valid defaults", func(t *testing.T) {
		cfg, errs := parseOTPConfig("5m", "60s", 5, 6, 5, 10)
		if len(errs) != 0 {
			t.Fatalf("expected no errors, got %v", errs)
		}
		if cfg.TTL != 5*time.Minute || cfg.ResendCooldown != 60*time.Second ||
			cfg.MaxAttempts != 5 || cfg.CodeLength != 6 ||
			cfg.MaxPerPhoneHour != 5 || cfg.MaxFailedPerPhoneHour != 10 {
			t.Fatalf("unexpected cfg: %+v", cfg)
		}
	})

	t.Run("invalid TTL duration string", func(t *testing.T) {
		if _, errs := parseOTPConfig("not-a-duration", "60s", 5, 6, 5, 10); len(errs) == 0 {
			t.Fatal("expected error for invalid OTP_TTL")
		}
	})

	t.Run("zero TTL rejected", func(t *testing.T) {
		if _, errs := parseOTPConfig("0s", "60s", 5, 6, 5, 10); len(errs) == 0 {
			t.Fatal("expected error for OTP_TTL <= 0")
		}
	})

	t.Run("invalid resend cooldown duration string", func(t *testing.T) {
		if _, errs := parseOTPConfig("5m", "not-a-duration", 5, 6, 5, 10); len(errs) == 0 {
			t.Fatal("expected error for invalid OTP_RESEND_COOLDOWN")
		}
	})

	t.Run("max attempts out of range", func(t *testing.T) {
		for _, n := range []int{0, -1, 11} {
			if _, errs := parseOTPConfig("5m", "60s", n, 6, 5, 10); len(errs) == 0 {
				t.Fatalf("expected error for OTP_MAX_ATTEMPTS=%d", n)
			}
		}
	})

	// Minimum raised 4 -> 6 (review finding #7) — 4 and 5 must now be
	// rejected, not just the previous out-of-range boundary of 3.
	t.Run("code length out of range", func(t *testing.T) {
		for _, n := range []int{3, 4, 5, 9} {
			if _, errs := parseOTPConfig("5m", "60s", 5, n, 5, 10); len(errs) == 0 {
				t.Fatalf("expected error for OTP_CODE_LENGTH=%d", n)
			}
		}
	})

	t.Run("max per phone hour out of range", func(t *testing.T) {
		for _, n := range []int{0, -1, 1001} {
			if _, errs := parseOTPConfig("5m", "60s", 5, 6, n, 10); len(errs) == 0 {
				t.Fatalf("expected error for OTP_MAX_PER_PHONE_HOUR=%d", n)
			}
		}
	})

	t.Run("max failed per phone hour out of range", func(t *testing.T) {
		for _, n := range []int{0, -1, 1001} {
			if _, errs := parseOTPConfig("5m", "60s", 5, 6, 5, n); len(errs) == 0 {
				t.Fatalf("expected error for OTP_MAX_FAILED_PER_PHONE_HOUR=%d", n)
			}
		}
	})
}

func TestGoogleOAuthConfig_Enabled(t *testing.T) {
	cases := []struct {
		name string
		cfg  GoogleOAuthConfig
		want bool
	}{
		{"all empty", GoogleOAuthConfig{}, false},
		{"all filled", GoogleOAuthConfig{ClientID: "a", ClientSecret: "b", RedirectURL: "c"}, true},
		{"partial", GoogleOAuthConfig{ClientID: "a"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.Enabled(); got != tc.want {
				t.Errorf("Enabled() = %v, want %v", got, tc.want)
			}
		})
	}
}
