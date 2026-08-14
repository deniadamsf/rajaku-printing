package model

import (
	"time"

	"github.com/google/uuid"
)

type StaffInvite struct {
	ID         uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID     uuid.UUID  `gorm:"type:uuid;not null;index"                       json:"user_id"`
	TokenHash  string     `gorm:"size:128;not null;uniqueIndex"                  json:"-"`
	ExpiresAt  time.Time  `gorm:"not null"                                       json:"expires_at"`
	UsedAt     *time.Time `                                                      json:"used_at,omitempty"`
	CreatedBy  uuid.UUID  `gorm:"type:uuid;not null"                             json:"created_by"`
	CreatedAt  time.Time  `gorm:"not null;default:now()"                         json:"created_at"`
}

func (StaffInvite) TableName() string { return "staff_invites" }
