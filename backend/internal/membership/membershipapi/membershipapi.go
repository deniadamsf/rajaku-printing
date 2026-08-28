// Package membershipapi is the PUBLIC contract of the membership module
// (§30 CLAUDE.md) — the ONLY package other modules (khususnya discount,
// §30.3) are allowed to import (§22: modul lain dilarang import
// internal/service/repository/model modul ini langsung).
package membershipapi

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// Status — daftar resmi status keanggotaan member (§30.2). Jangan mengarang
// nilai baru — lihat state machine di membership/service.
type Status string

const (
	StatusNone     Status = "none"
	StatusPending  Status = "pending"
	StatusActive   Status = "active"
	StatusRejected Status = "rejected"
	StatusRevoked  Status = "revoked"
)

var (
	// ErrMembershipCustomerNotFound — customer_id tidak ada.
	ErrMembershipCustomerNotFound = errors.New("membershipapi: customer tidak ditemukan")

	// ErrMembershipRequiresRegisteredAccount — Apply dipanggil untuk customer
	// yang customer_type-nya bukan 'registered' (§30.2 — guest, termasuk hasil
	// walk-in POS, tidak bisa mengajukan langsung).
	ErrMembershipRequiresRegisteredAccount = errors.New("membershipapi: hanya akun terdaftar yang bisa mengajukan membership")

	// ErrMembershipDisabled — setting membership_enabled=false (§30.1).
	ErrMembershipDisabled = errors.New("membershipapi: fitur membership sedang nonaktif")

	// ErrMembershipInvalidTransition — transisi status yang diminta tidak ada
	// di daftar resmi §30.2 (termasuk kasus "sudah pernah mengajukan" —
	// SATU sentinel untuk semua transisi tidak valid, jangan tambah sentinel
	// kembar untuk kasus yang sama).
	ErrMembershipInvalidTransition = errors.New("membershipapi: transisi status membership tidak valid dari status saat ini")

	// ErrMembershipReasonRequired — Reject/Revoke dipanggil tanpa alasan
	// (§30.2 — wajib diisi untuk keduanya).
	ErrMembershipReasonRequired = errors.New("membershipapi: alasan wajib diisi")

	// ErrMembershipInvalidStatusFilter — GET /admin/membership dipanggil
	// dengan query status yang bukan salah satu dari daftar resmi.
	ErrMembershipInvalidStatusFilter = errors.New("membershipapi: status filter tidak valid")
)

// Checker adalah kontrak yang di-consume modul lain (khususnya discount,
// §30.3 — diskon audience_scope='member') untuk mengecek apakah seorang
// customer adalah member AKTIF saat ini.
type Checker interface {
	// IsActiveMember returns true kalau customerID berstatus membership
	// 'active'. customerID == uuid.Nil (mis. order guest tanpa customer_id)
	// selalu mengembalikan (false, nil) — bukan error.
	IsActiveMember(ctx context.Context, customerID uuid.UUID) (bool, error)
}
