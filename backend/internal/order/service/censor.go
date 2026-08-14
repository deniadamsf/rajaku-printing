package service

import (
	"strings"
	"unicode/utf8"
)

// maskPhone censors a 62xxx phone into "0812****678" style (spec section 5).
// Input contract: canonical 62xxx from pkg/phone; we convert leading 62 → 0
// for display (customer sees the format they typed originally).
func maskPhone(p62 string) string {
	if !strings.HasPrefix(p62, "62") || len(p62) < 6 {
		return "" // don't leak partial data if unexpected format
	}
	// convert 62xxx → 0xxx
	display := "0" + p62[2:]
	n := len(display)
	if n <= 7 {
		// too short to censor meaningfully; hide most of it
		return display[:2] + strings.Repeat("*", n-2)
	}
	head := display[:4]
	tail := display[n-3:]
	return head + "****" + tail
}

// maskAddress keeps first N runes then "**".
func maskAddress(addr string) string {
	const keep = 10
	if utf8.RuneCountInString(addr) <= keep {
		return "**"
	}
	runes := []rune(addr)
	return string(runes[:keep]) + "**"
}

// maskName keeps first name initial visible: "Ani Testing" → "Ani T***".
func maskName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	parts := strings.Fields(name)
	if len(parts) == 1 {
		if utf8.RuneCountInString(parts[0]) <= 2 {
			return parts[0]
		}
		runes := []rune(parts[0])
		return string(runes[:2]) + strings.Repeat("*", len(runes)-2)
	}
	first := parts[0]
	rest := parts[len(parts)-1]
	initial := ""
	if r := []rune(rest); len(r) > 0 {
		initial = string(r[0])
	}
	return first + " " + initial + "***"
}
