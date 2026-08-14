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

type User struct {
	ID            uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Email         *string    `gorm:"uniqueIndex;size:255"                            json:"email,omitempty"`
	Phone         string     `gorm:"uniqueIndex;size:20;not null"                    json:"phone"`
	Name          string     `gorm:"size:255;not null"                               json:"name"`
	PasswordHash  *string    `gorm:"size:255"                                        json:"-"`
	UserType      UserType   `gorm:"size:20;not null"                                json:"user_type"`
	CustomerType  *CustomerType `gorm:"size:20"                                      json:"customer_type,omitempty"`
	OAuthProvider *string    `gorm:"column:oauth_provider;size:20"                   json:"oauth_provider,omitempty"`
	OAuthSubject  *string    `gorm:"column:oauth_subject;size:255"                   json:"-"`
	// NB: DB has DEFAULT TRUE — but GORM's `default:true` tag would silently
	// overwrite an explicit `false` Go value (used by staff invite flow), so
	// we drop it here. Every Go path currently sets IsActive explicitly.
	IsActive      bool       `gorm:"not null"                                        json:"is_active"`
	LastLoginAt   *time.Time `                                                       json:"last_login_at,omitempty"`
	CreatedAt     time.Time  `gorm:"not null;default:now()"                          json:"created_at"`
	UpdatedAt     time.Time  `gorm:"not null;default:now()"                          json:"updated_at"`

	Roles []Role `gorm:"many2many:user_roles;joinForeignKey:user_id;joinReferences:role_id" json:"roles,omitempty"`
}

func (User) TableName() string { return "users" }
