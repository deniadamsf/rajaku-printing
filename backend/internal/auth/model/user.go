// Package model contains GORM entities for the auth module. Keep DB-facing
// concerns (tags, column names) here; expose only DTOs / interface types to
// callers outside the module.
package model

import (
	"time"

	"github.com/google/uuid"
)

type UserType string

const (
	UserTypeCustomer UserType = "customer"
	UserTypeStaff    UserType = "staff"
)

type CustomerType string

const (
	CustomerTypeGuest      CustomerType = "guest"
	CustomerTypeRegistered CustomerType = "registered"
)

// MembershipStatus — status keanggotaan member customer (§30.2 CLAUDE.md).
// Daftar resmi, jangan mengarang nilai baru — lihat state machine di
// membership/service.
type MembershipStatus string

const (
	MembershipStatusNone     MembershipStatus = "none"
	MembershipStatusPending  MembershipStatus = "pending"
	MembershipStatusActive   MembershipStatus = "active"
	MembershipStatusRejected MembershipStatus = "rejected"
	MembershipStatusRevoked  MembershipStatus = "revoked"
)

type User struct {
	ID    uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Email *string   `gorm:"uniqueIndex;size:255"                            json:"email,omitempty"`
	// Phone — nullable (migration 000017): a Google-registered customer may
	// skip supplying a WhatsApp number entirely (owner decision — OTP hanya
	// diterbitkan saat benar-benar dibutuhkan untuk membuktikan kepemilikan,
	// bukan setiap registrasi, supaya volume kirim WA lewat Baileys tidak
	// memicu banned, §13). Uniqueness is enforced by a PARTIAL unique index
	// (`WHERE phone IS NOT NULL`) so any number of NULL-phone accounts may
	// coexist. Still the matching key (§11) for every row that DOES have one.
	Phone *string `gorm:"size:20"                                         json:"phone,omitempty"`
	// PhoneVerifiedAt — NULL means `Phone` is merely CLAIMED (typed in by the
	// user, or attached by a kasir/POS walk-in) but never proven; non-NULL
	// records when a WhatsApp OTP round-trip actually confirmed ownership
	// (§ phone-claim review). Every write path that sets `Phone` MUST decide
	// this column explicitly — never let it silently carry over a stale
	// verification from a different number. Never expose the raw timestamp to
	// customers; only derive a `phone_verified` boolean (see service.MeOutput).
	PhoneVerifiedAt *time.Time    `gorm:"column:phone_verified_at"                        json:"-"`
	Name            string        `gorm:"size:255;not null"                               json:"name"`
	PasswordHash    *string       `gorm:"size:255"                                        json:"-"`
	UserType        UserType      `gorm:"size:20;not null"                                json:"user_type"`
	CustomerType    *CustomerType `gorm:"size:20"                                         json:"customer_type,omitempty"`
	OAuthProvider   *string       `gorm:"column:oauth_provider;size:20"                   json:"oauth_provider,omitempty"`
	OAuthSubject    *string       `gorm:"column:oauth_subject;size:255"                   json:"-"`
	// NB: DB has DEFAULT TRUE — but GORM's `default:true` tag would silently
	// overwrite an explicit `false` Go value (used by staff invite flow), so
	// we drop it here. Every Go path currently sets IsActive explicitly.
	IsActive    bool       `gorm:"not null"                                        json:"is_active"`
	LastLoginAt *time.Time `                                                       json:"last_login_at,omitempty"`

	// Membership (§30 CLAUDE.md) — status keanggotaan member. Kolom milik
	// `users` sendiri (bukan tabel terpisah), lihat migration 000030 untuk
	// alasannya. MembershipStatus sumber kebenaran; membership_status_logs
	// (modul membership) cuma audit trail.
	MembershipStatus       MembershipStatus `gorm:"size:20;not null;default:none;column:membership_status" json:"membership_status"`
	MembershipRequestedAt  *time.Time       `gorm:"column:membership_requested_at"                          json:"membership_requested_at,omitempty"`
	MembershipDecidedAt    *time.Time       `gorm:"column:membership_decided_at"                            json:"membership_decided_at,omitempty"`
	MembershipDecidedBy    *uuid.UUID       `gorm:"type:uuid;column:membership_decided_by"                  json:"membership_decided_by,omitempty"`
	MembershipDecisionNote *string          `gorm:"column:membership_decision_note"                         json:"membership_decision_note,omitempty"`

	CreatedAt time.Time `gorm:"not null;default:now()"                          json:"created_at"`
	UpdatedAt time.Time `gorm:"not null;default:now()"                          json:"updated_at"`

	Roles []Role `gorm:"many2many:user_roles;joinForeignKey:user_id;joinReferences:role_id" json:"roles,omitempty"`
}

func (User) TableName() string { return "users" }
