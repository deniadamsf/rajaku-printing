package model

import (
	"time"

	"github.com/google/uuid"
)

// PhoneVerification is a WhatsApp OTP challenge tied to exactly one Google
// OAuth registration handoff code (see migration 000013 for the "why" —
// nomor WA bukan rahasia, jadi kepemilikan wajib dibuktikan sebelum akun
// dibuat/di-upgrade dari guest).
type PhoneVerification struct {
	ID               uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	OAuthLoginCodeID uuid.UUID `gorm:"column:oauth_login_code_id;type:uuid;not null"   json:"oauth_login_code_id"`
	Phone            string    `gorm:"size:20;not null"                                json:"phone"`
	CodeHash         string    `gorm:"column:code_hash;size:64;not null"               json:"-"`
	Attempts         int       `gorm:"not null;default:0"                              json:"attempts"`
	ExpiresAt        time.Time `gorm:"not null"                                        json:"expires_at"`
	// VerifiedAt — AUDIT-ONLY (review finding #5): records when the correct
	// code was entered, but nothing in this codebase gates on it — the
	// authoritative "this challenge may no longer be used" signal is
	// ConsumedAt (checked by FindActive's WHERE clause). A row can be
	// verified but never consumed (e.g. Complete()'s downstream transaction
	// rolled back after a correct OTP — see google_oauth_service.go
	// Complete()), in which case the SAME code remains usable again until it
	// expires or IS consumed. Do not repurpose this column as a security
	// control without also updating FindActive.
	VerifiedAt *time.Time `                                                       json:"verified_at,omitempty"`
	ConsumedAt *time.Time `                                                       json:"consumed_at,omitempty"`
	CreatedAt  time.Time  `gorm:"not null;default:now()"                          json:"created_at"`
}

func (PhoneVerification) TableName() string { return "phone_verifications" }
