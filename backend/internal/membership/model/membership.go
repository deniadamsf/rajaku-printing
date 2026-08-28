// Package model contains GORM entities for the membership module (§30
// CLAUDE.md). Status keanggotaan itu sendiri (membership_status dkk) hidup
// di tabel `users` milik modul auth — diakses lewat authapi.CustomerService,
// TIDAK diduplikasi di sini (§22). Satu-satunya entity milik modul ini adalah
// log audit trail murni di bawah.
package model

import (
	"time"

	"github.com/google/uuid"
)

// MembershipStatusLog — satu baris = satu transisi status membership (§30.2).
// Audit trail MURNI — BUKAN sumber kebenaran status (users.membership_status
// itu sumber kebenarannya). Tidak pernah dibaca untuk keputusan bisnis.
type MembershipStatusLog struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CustomerID uuid.UUID `gorm:"type:uuid;not null;column:customer_id"          json:"customer_id"`
	FromStatus string    `gorm:"size:20;not null;column:from_status"            json:"from_status"`
	ToStatus   string    `gorm:"size:20;not null;column:to_status"              json:"to_status"`
	// ChangedBy — nil berarti transisi dipicu customer sendiri (Apply),
	// non-nil untuk transisi yang dipicu staff (approve/reject/revoke).
	ChangedBy *uuid.UUID `gorm:"type:uuid;column:changed_by" json:"changed_by,omitempty"`
	Note      *string    `gorm:"column:note"                 json:"note,omitempty"`
	CreatedAt time.Time  `gorm:"not null;default:now();column:created_at" json:"created_at"`
}

func (MembershipStatusLog) TableName() string { return "membership_status_logs" }
