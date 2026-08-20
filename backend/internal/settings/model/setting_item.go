package model

import (
	"time"

	"github.com/google/uuid"
)

// SettingItem — proyeksi transport dari AppSetting untuk GET /admin/settings.
//
// Sengaja struct terpisah dari AppSetting (entity GORM): AllowedValues bukan
// kolom DB, cuma proyeksi dari aturan enum di settings/service (intRules).
// Menambah field non-kolom langsung ke AppSetting akan mengundang bug GORM
// (mis. auto-migrate mencoba bikin kolom, atau field ikut ke-scan dari query
// SELECT * yang tidak menyertakannya) — pola yang sama seperti PaymentInfo di
// payment_info.go. Field lain (Key/Value/DisplayName/dst) sengaja disalin
// 1:1 dari AppSetting, bukan di-embed, supaya kontrak JSON tetap eksplisit
// dan tidak diam-diam bocor field baru yang ditambah ke entity di masa depan.
type SettingItem struct {
	Key         string     `json:"key"`
	Value       string     `json:"value"`
	DisplayName string     `json:"display_name"`
	Description *string    `json:"description,omitempty"`
	UpdatedAt   time.Time  `json:"updated_at"`
	UpdatedBy   *uuid.UUID `json:"updated_by,omitempty"`

	// AllowedValues — daftar nilai yang sah untuk key ini, diformat sebagai
	// string supaya sebanding dengan Value. Nil (omitempty) untuk key
	// free-form (retensi hari, payment.*) yang tidak punya daftar tertutup —
	// frontend tahu untuk merender input teks/angka biasa, bukan dropdown.
	AllowedValues []string `json:"allowed_values,omitempty"`
}
