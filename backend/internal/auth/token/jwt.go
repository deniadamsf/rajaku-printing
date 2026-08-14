// Package token issues and verifies JWT access tokens (HS256).
//
// Design keputusan:
//   - Stateless access token, TTL pendek (default 24h — configurable via env).
//   - Claims memuat user_id + user_type + roles + permissions supaya middleware
//     tidak perlu hit DB tiap request.
//   - Refresh token BELUM diimplementasikan (deferred, ada TODO).
package token

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidToken   = errors.New("token: invalid")
	ErrExpiredToken   = errors.New("token: expired")
	ErrMalformedToken = errors.New("token: malformed")
)

// Claims adalah payload JWT — semua field diserialisasi ke JSON.
type Claims struct {
	UserID      string   `json:"uid"`
	UserType    string   `json:"typ"` // 'customer' | 'staff'
	Email       string   `json:"em,omitempty"`
	Phone       string   `json:"ph,omitempty"`
	Roles       []string `json:"roles,omitempty"` // role names
	Permissions []string `json:"perms,omitempty"` // permission codes (flatten dari roles)
	// Scope — kosong berarti sesi penuh biasa. Nilai non-kosong (mis.
	// "guest_order") menandai token TERBATAS yang hanya boleh dipakai untuk
	// endpoint yang secara eksplisit mengizinkan scope tersebut
	// (authapi.RequireAuthAllowScope). authapi.RequireAuth WAJIB menolak token
	// ber-scope — deny-by-default.
	Scope string `json:"scp,omitempty"`
	// OrderID — hanya diisi untuk token ber-scope yang terikat ke SATU order
	// (mis. guest_order dari POST /lacak/:resi/verify). Tanpa ini, token guest
	// dari satu resi bisa dipakai untuk order LAIN milik nomor WA yang sama —
	// service yang mengonsumsi scope ini (mis. modul design) WAJIB menolak
	// akses ke order selain yang tertera di sini. Kosong untuk sesi penuh.
	OrderID string `json:"oid,omitempty"`
	jwt.RegisteredClaims
}

// Issuer sign & verify JWT dengan secret + TTL yang di-inject sekali.
// Zero value TIDAK valid — gunakan NewIssuer.
type Issuer struct {
	secret []byte
	ttl    time.Duration
	iss    string
}

func NewIssuer(secret string, ttl time.Duration, issuerName string) (*Issuer, error) {
	if len(secret) < 32 {
		return nil, fmt.Errorf("token: secret must be >= 32 chars (got %d)", len(secret))
	}
	if ttl <= 0 {
		return nil, errors.New("token: ttl must be > 0")
	}
	return &Issuer{secret: []byte(secret), ttl: ttl, iss: issuerName}, nil
}

func (i *Issuer) Sign(c Claims) (string, error) {
	now := time.Now()
	c.RegisteredClaims = jwt.RegisteredClaims{
		Issuer:    i.iss,
		Subject:   c.UserID,
		IssuedAt:  jwt.NewNumericDate(now),
		NotBefore: jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(i.ttl)),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	s, err := tok.SignedString(i.secret)
	if err != nil {
		return "", fmt.Errorf("sign jwt: %w", err)
	}
	return s, nil
}

func (i *Issuer) Verify(raw string) (*Claims, error) {
	claims := &Claims{}
	tok, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Method.Alg())
		}
		return i.secret, nil
	}, jwt.WithIssuer(i.iss))
	if err != nil {
		switch {
		case errors.Is(err, jwt.ErrTokenExpired):
			return nil, ErrExpiredToken
		case errors.Is(err, jwt.ErrTokenMalformed):
			return nil, ErrMalformedToken
		default:
			return nil, ErrInvalidToken
		}
	}
	if !tok.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// TTL returns the configured access-token lifetime (untuk dikirim ke client
// sebagai `expires_in`).
func (i *Issuer) TTL() time.Duration { return i.ttl }
