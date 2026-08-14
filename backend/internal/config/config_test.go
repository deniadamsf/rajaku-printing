package config

import "testing"

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
