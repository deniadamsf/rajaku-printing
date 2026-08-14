// Package model contains GORM entities for the settings module.
package model

import (
	"time"

	"github.com/google/uuid"
)

// AppSetting — satu baris konfigurasi global editable dari admin panel.
//
// Value sengaja disimpan TEXT: setting baru cukup INSERT row lewat migration,
// tidak perlu ALTER TABLE. Konsekuensinya, parsing & validasi range jadi
// tanggung jawab service (tiap key punya aturan sendiri).
type AppSetting struct {
	Key         string  `gorm:"primaryKey;size:80"      json:"key"`
	Value       string  `gorm:"type:text;not null"      json:"value"`
	DisplayName string  `gorm:"size:120;not null"       json:"display_name"`
	Description *string `gorm:"type:text"               json:"description,omitempty"`

	UpdatedAt time.Time  `gorm:"not null;default:now()" json:"updated_at"`
	UpdatedBy *uuid.UUID `gorm:"type:uuid"              json:"updated_by,omitempty"`
}

func (AppSetting) TableName() string { return "app_settings" }
