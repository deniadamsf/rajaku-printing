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
	ID               uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	OAuthLoginCodeID uuid.UUID  `gorm:"column:oauth_login_code_id;type:uuid;not null"   json:"oauth_login_code_id"`
	Phone            string     `gorm:"size:20;not null"                                json:"phone"`
	CodeHash         string     `gorm:"column:code_hash;size:64;not null"               json:"-"`
	Attempts         int        `gorm:"not null;default:0"                              json:"attempts"`
	ExpiresAt        time.Time  `gorm:"not null"                                        json:"expires_at"`
	VerifiedAt       *time.Time `                                                       json:"verified_at,omitempty"`
	ConsumedAt       *time.Time `                                                       json:"consumed_at,omitempty"`
	CreatedAt        time.Time  `gorm:"not null;default:now()"                          json:"created_at"`
}

func (PhoneVerification) TableName() string { return "phone_verifications" }
