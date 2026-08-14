// Package resi generates unique tracking codes `RJK-XXXXXXXX` (spec section 5).
//
// Random 8-char suffix pakai crypto/rand — bukan math/rand — supaya tidak
// mudah ditebak (mencegah brute-force /lacak/:resi, spec section 23).
//
// Charset sengaja **tidak** mencakup karakter yg mudah tertukar (I/O/0/1)
// karena resi kadang dibaca lewat telepon oleh customer/kasir.
//
// Retry-on-collision (kalau random string bentrok dgn resi existing) di-handle
// di service layer via unique constraint DB (kolom `resi` UNIQUE) — package
// ini hanya generate, service loop panggil ulang.
package resi

import (
	"crypto/rand"
	"errors"
	"strings"
)

const (
	Prefix     = "RJK-"
	SuffixLen  = 8
	Charset    = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // 32 chars, aman dari confusables
)

var (
	ErrRandFailed = errors.New("resi: crypto/rand read failed")

	// Rejection-sampling threshold: 256 % 32 = 0, so we can use full byte range.
	// (Kalau charset panjangnya bukan pembagi 256, harus reject bytes > threshold.)
)

// Generate returns a fresh resi string dgn 8 karakter acak setelah prefix.
// Length total = 12 (RJK- + 8 chars).
func Generate() (string, error) {
	b := make([]byte, SuffixLen)
	if _, err := rand.Read(b); err != nil {
		return "", ErrRandFailed
	}
	var sb strings.Builder
	sb.Grow(len(Prefix) + SuffixLen)
	sb.WriteString(Prefix)
	for i := 0; i < SuffixLen; i++ {
		sb.WriteByte(Charset[int(b[i])%len(Charset)])
	}
	return sb.String(), nil
}
