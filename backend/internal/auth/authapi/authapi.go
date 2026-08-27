// Package authapi is the PUBLIC contract of the auth module — the ONLY package
// that other modules (order, catalog, ...) are allowed to import.
//
// Per spec section 22: "Dilarang modul saling import langsung ke internal
// package modul lain — komunikasi lewat interface/event." This package holds:
//   - Service interfaces yang di-consume modul lain (via dependency injection).
//   - Middleware factories (RequireAuth, RequirePermission).
//   - Types share-able (Identity, UserType, permission codes).
//
// Implementasi konkret hidup di internal/auth/service. Wiring dilakukan di
// cmd/api/main.go — modul lain menerima interface, bukan struct.
package authapi

import (
	"context"

	"github.com/google/uuid"
)

// UserType eksternal (duplicate string constant dari model — sengaja, supaya
// modul lain tidak perlu import package model).
type UserType string

const (
	UserTypeCustomer UserType = "customer"
	UserTypeStaff    UserType = "staff"
)

// ScopeGuestOrder marks a token issued via POST /lacak/:resi/verify — a guest
// customer proving ownership of an order with resi + nomor WA (no password).
// Session tokens (login/register) have an empty Scope. Endpoints that accept
// this scope MUST opt in explicitly via authapi.RequireAuthAllowScope — never
// compare the raw string literal outside this package.
const ScopeGuestOrder = "guest_order"

// Identity adalah proyeksi user aktif untuk cross-module — hanya field yang
// dibutuhkan modul lain untuk otorisasi & auditing.
type Identity struct {
	UserID      uuid.UUID
	UserType    UserType
	Email       string // "" untuk guest
	Phone       string
	Name        string
	Roles       []string // role names
	Permissions []string // permission codes (flatten)
	// Scope — "" untuk sesi penuh (login/register). Non-kosong (mis.
	// ScopeGuestOrder) menandai token terbatas — lihat token.Claims.Scope.
	Scope string
	// OrderID — non-nil hanya untuk token ber-scope yang terikat ke SATU order
	// (mis. ScopeGuestOrder). nil untuk sesi penuh. Modul yang menerima scope
	// terbatas ini (mis. design) WAJIB menolak akses ke order lain kalau field
	// ini terisi — lihat token.Claims.OrderID.
	OrderID *uuid.UUID
}

// HasPermission returns true iff `code` ada di Permissions.
func (i Identity) HasPermission(code string) bool {
	for _, p := range i.Permissions {
		if p == code {
			return true
		}
	}
	return false
}

// Service adalah kontrak auth untuk konsumen internal (mis. middleware, atau
// module lain yang butuh lookup identity dari token). Implementasi di
// internal/auth/service.
type Service interface {
	// VerifyToken validates an access token and returns the Identity carried
	// in its claims. Returns wrapped ErrInvalidToken / ErrExpiredToken on
	// failure — caller MUST map to 401.
	VerifyToken(ctx context.Context, rawToken string) (*Identity, error)
}

// CustomerService adalah kontrak untuk modul lain (order, POS) yang perlu
// resolve/create customer by phone (spec section 11 — nomor WA sebagai
// matching key lintas channel).
//
// TODO(auth): Implementasi konkret menyusul saat modul order/POS dibangun.
// Interface didefinisikan sekarang biar modul konsumer bisa mock & test lebih
// dulu tanpa nunggu auth selesai.
type CustomerService interface {
	// ResolveOrCreateGuest returns the existing customer with `phone` (if any),
	// otherwise creates a new guest customer with the given name. `phone` MUST
	// already be normalized to 62xxx by caller (pkg/phone).
	ResolveOrCreateGuest(ctx context.Context, phone, name string) (*Identity, error)

	// FindByID returns the Identity projection of a customer/user by primary
	// key. Dipakai modul notification untuk resolve nomor WA penerima dari
	// order.customer_id. Return ErrCustomerNotFound kalau tidak ada.
	FindByID(ctx context.Context, id uuid.UUID) (*Identity, error)

	// SearchCustomers returns customers (user_type=customer, is_active=true,
	// phone IS NOT NULL) whose name or phone contains `query` (case-insensitive
	// substring), ordered by name. `query` shorter than 2 characters (after
	// trimming whitespace) returns an EMPTY slice — not an error, and NOT a
	// listing of customers; this is a search endpoint, not a list endpoint.
	// Phone matching goes through a lenient 0/+62→62 normalization so a kasir
	// typing the local format ("0812...") still matches rows stored canonically
	// ("62812...", §13); name matching uses the raw query untouched. `limit` is
	// clamped by the repository to the 1-20 range (a caller-supplied value <= 0
	// defaults to 10). Used by POS (§11) so a kasir can find a customer who
	// already has an identity — online or walk-in — instead of re-entering
	// their data and creating a duplicate.
	SearchCustomers(ctx context.Context, query string, limit int) ([]Identity, error)
}
