package model

import (
	"time"

	"github.com/google/uuid"
)

type Role struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name        string    `gorm:"uniqueIndex;size:50;not null"                   json:"name"`
	DisplayName string    `gorm:"size:100;not null"                              json:"display_name"`
	Description *string   `                                                      json:"description,omitempty"`
	IsSystem    bool      `gorm:"not null;default:false"                         json:"is_system"`
	CreatedAt   time.Time `gorm:"not null;default:now()"                         json:"created_at"`
	UpdatedAt   time.Time `gorm:"not null;default:now()"                         json:"updated_at"`

	Permissions []MenuPermission `gorm:"many2many:role_permissions;joinForeignKey:role_id;joinReferences:permission_id" json:"permissions,omitempty"`
}

func (Role) TableName() string { return "roles" }

type MenuPermission struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Code        string    `gorm:"uniqueIndex;size:100;not null"                  json:"code"`
	DisplayName string    `gorm:"size:200;not null"                              json:"display_name"`
	Category    string    `gorm:"size:50;not null;index"                         json:"category"`
	Description *string   `                                                      json:"description,omitempty"`
	CreatedAt   time.Time `gorm:"not null;default:now()"                         json:"created_at"`
}

func (MenuPermission) TableName() string { return "menu_permissions" }

// UserRole is the join table with metadata (assigned_by, assigned_at) beyond
// what GORM's implicit many2many table provides. Access explicitly when you
// need audit info; otherwise use User.Roles / Role.Permissions.
type UserRole struct {
	UserID     uuid.UUID  `gorm:"type:uuid;primaryKey"    json:"user_id"`
	RoleID     uuid.UUID  `gorm:"type:uuid;primaryKey"    json:"role_id"`
	AssignedBy *uuid.UUID `gorm:"type:uuid"               json:"assigned_by,omitempty"`
	AssignedAt time.Time  `gorm:"not null;default:now()"  json:"assigned_at"`
}

func (UserRole) TableName() string { return "user_roles" }
