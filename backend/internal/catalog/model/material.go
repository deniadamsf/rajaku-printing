package model

import (
	"time"

	"github.com/google/uuid"
)

type Material struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Code        string    `gorm:"uniqueIndex;size:50;not null"                   json:"code"`
	Name        string    `gorm:"size:150;not null"                              json:"name"`
	Description *string   `                                                      json:"description,omitempty"`
	IsActive    bool      `gorm:"not null;default:true"                          json:"is_active"`
	CreatedAt   time.Time `gorm:"not null;default:now()"                         json:"created_at"`
	UpdatedAt   time.Time `gorm:"not null;default:now()"                         json:"updated_at"`
}

func (Material) TableName() string { return "materials" }
