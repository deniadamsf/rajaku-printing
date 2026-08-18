package model

import (
	"time"

	"github.com/google/uuid"
)

// SiteMedia — satu baris = satu slot yang SUDAH diisi admin. Slot yang belum
// pernah diunggah tidak punya baris di sini (lihat Registry untuk daftar
// slot yang tersedia termasuk yang masih kosong).
type SiteMedia struct {
	Slot         string `gorm:"primaryKey;size:80"  json:"slot"`
	StoragePath  string `gorm:"not null"            json:"-"`
	OriginalName string `gorm:"size:255;not null"   json:"original_name"`
	MimeType     string `gorm:"size:50;not null"    json:"mime_type"`
	SizeBytes    int64  `gorm:"not null"            json:"size_bytes"`
	WidthPx      int    `gorm:"not null"            json:"width_px"`
	HeightPx     int    `gorm:"not null"            json:"height_px"`

	UploadedBy *uuid.UUID `gorm:"type:uuid"              json:"uploaded_by,omitempty"`
	UploadedAt time.Time  `gorm:"not null;default:now()" json:"uploaded_at"`
	UpdatedAt  time.Time  `gorm:"not null;default:now()" json:"updated_at"`
}

func (SiteMedia) TableName() string { return "site_media" }
