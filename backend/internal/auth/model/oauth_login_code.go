package model

import (
	"time"

	"github.com/google/uuid"
)

// OAuthLoginCodeKind — see migration 000012 for the DB-level CHECK constraint
// this must stay in sync with.
type OAuthLoginCodeKind string

const (
	// OAuthLoginCodeKindSession — identity already resolved (existing linked
	// account, or matched by verified email). UserID is set.
	OAuthLoginCodeKindSession OAuthLoginCodeKind = "session"
	// OAuthLoginCodeKindRegistration — brand-new Google identity, no local
	// phone number on file yet. Subject/Email are set instead of UserID.
	OAuthLoginCodeKindRegistration OAuthLoginCodeKind = "registration"
)

// OAuthLoginCode is a one-time, short-lived handoff code bridging the
// server-redirected OAuth callback and the frontend SPA (see migration
// 000012 for the rationale).
type OAuthLoginCode struct {
	ID           uuid.UUID          `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CodeHash     string             `gorm:"column:code_hash;size:64;not null;uniqueIndex"  json:"-"`
	Kind         OAuthLoginCodeKind `gorm:"size:20;not null"                                json:"kind"`
	UserID       *uuid.UUID         `gorm:"type:uuid"                                       json:"user_id,omitempty"`
	Provider     string             `gorm:"size:20;not null;default:google"                 json:"provider"`
	Subject      *string            `gorm:"size:255"                                        json:"subject,omitempty"`
	Email        *string            `gorm:"size:255"                                        json:"email,omitempty"`
	Name         *string            `gorm:"size:255"                                        json:"name,omitempty"`
	RedirectPath *string            `                                                       json:"redirect_path,omitempty"`
	ExpiresAt    time.Time          `gorm:"not null"                                        json:"expires_at"`
	UsedAt       *time.Time         `                                                       json:"used_at,omitempty"`
	CreatedAt    time.Time          `gorm:"not null;default:now()"                          json:"created_at"`
}

func (OAuthLoginCode) TableName() string { return "oauth_login_codes" }
