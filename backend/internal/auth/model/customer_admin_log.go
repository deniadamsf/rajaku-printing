package model

import (
	"time"

	"github.com/google/uuid"
)

// CustomerAdminLogAction — daftar resmi aksi admin yang tercatat di
// customer_admin_logs (fitur "Manajemen Pelanggan"). Jangan mengarang nilai
// baru tanpa CHECK constraint yang sejalan di migration 000032.
type CustomerAdminLogAction string

const (
	CustomerAdminActionBlock         CustomerAdminLogAction = "block"
	CustomerAdminActionUnblock       CustomerAdminLogAction = "unblock"
	CustomerAdminActionProfileUpdate CustomerAdminLogAction = "profile_update"
)

// CustomerAdminLog — satu baris = satu aksi admin terhadap pelanggan lewat
// admin panel: blokir/aktifkan (Action=Block/Unblock, Reason terisi) ATAU
// ubah data (Action=ProfileUpdate, Changes terisi). Audit trail MURNI —
// sumber kebenaran tetap kolom `users` sendiri (is_active/name/email/phone),
// pola yang sama dengan MembershipStatusLog (§30) — tabel ini tidak pernah
// dibaca untuk keputusan bisnis, hanya untuk telusur balik "kenapa/siapa
// yang mengubah pelanggan ini, dan apa yang berubah" (temuan code review #9
// — sebelumnya perubahan name/email/phone lewat UpdateCustomer tidak
// meninggalkan jejak sama sekali).
type CustomerAdminLog struct {
	ID         uuid.UUID              `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CustomerID uuid.UUID              `gorm:"type:uuid;not null;column:customer_id"          json:"customer_id"`
	Action     CustomerAdminLogAction `gorm:"size:30;not null;column:action"                 json:"action"`
	// Reason — wajib diisi SERVICE (bukan DB) untuk Action=Block/Unblock
	// (minimal 10 karakter). Nil untuk ProfileUpdate.
	Reason *string `gorm:"column:reason" json:"reason,omitempty"`
	// Changes — ringkasan LAMA -> BARU field yang benar-benar berubah, hanya
	// diisi untuk Action=ProfileUpdate. Nil untuk Block/Unblock, dan nil
	// kalau PATCH tidak benar-benar mengubah apa pun (no-op PATCH tidak
	// menulis baris log sama sekali — lihat customer_admin_service.go).
	Changes *string `gorm:"column:changes" json:"changes,omitempty"`
	// ChangedBy — staff yang melakukan aksi. Tidak nullable secara bisnis
	// (selalu staff admin lewat endpoint ini), tapi kolom dibuat nullable di
	// DB (pola sama membership_status_logs.changed_by) supaya baris tetap
	// aman kalau staff pelakunya suatu saat dihapus — FK tidak akan pernah
	// hard-delete users (§10/§22), jadi ini murni jaga-jaga skema.
	ChangedBy *uuid.UUID `gorm:"type:uuid;column:changed_by" json:"changed_by,omitempty"`
	CreatedAt time.Time  `gorm:"not null;default:now();column:created_at" json:"created_at"`
}

func (CustomerAdminLog) TableName() string { return "customer_admin_logs" }
