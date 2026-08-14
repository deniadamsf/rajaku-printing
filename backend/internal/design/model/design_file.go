// Package model contains GORM entities for the design module.
package model

import (
	"time"

	"github.com/google/uuid"
)

// Role — apa peran file dalam design flow (§6).
type Role string

const (
	// RoleCustomerUpload — file siap-cetak yg diupload customer (path
	// "upload desain sendiri"). Staff cukup verifikasi & lanjut.
	RoleCustomerUpload Role = "customer_upload"

	// RoleCustomerAsset — aset atau brief attachment untuk request desain
	// (logo, foto referensi, PDF brief). Bukan file untuk dicetak.
	RoleCustomerAsset Role = "customer_asset"

	// RoleStaffDraft — hasil kerja staff desain per iterasi. Setiap draft
	// punya lifecycle approval sendiri (pending → approved | revision_requested).
	RoleStaffDraft Role = "staff_draft"
)

// ApprovalStatus — HANYA berlaku untuk role=staff_draft.
type ApprovalStatus string

const (
	ApprovalPending  ApprovalStatus = "pending"
	ApprovalApproved ApprovalStatus = "approved"
	ApprovalRevision ApprovalStatus = "revision_requested"
)

type DesignFile struct {
	ID      uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	OrderID uuid.UUID `gorm:"type:uuid;not null;index"                       json:"order_id"`
	Role    Role      `gorm:"size:30;not null"                               json:"role"`

	FilePath         string `gorm:"not null"           json:"-"` // internal; served via /design-files/:id/file
	FileOriginalName string `gorm:"size:255;not null"  json:"file_original_name"`
	FileSizeBytes    int64  `gorm:"not null"           json:"file_size_bytes"`
	FileMimeType     string `gorm:"size:100;not null"  json:"file_mime_type"`
	IsPreviewable    bool   `gorm:"not null;default:false" json:"is_previewable"`

	Notes *string `json:"notes,omitempty"`

	UploadedBy *uuid.UUID `gorm:"type:uuid"              json:"uploaded_by,omitempty"`
	UploadedAt time.Time  `gorm:"not null;default:now()" json:"uploaded_at"`

	// Retention (§19)
	IsPurged bool       `gorm:"not null;default:false" json:"is_purged"`
	PurgedAt *time.Time `                              json:"purged_at,omitempty"`
	// RetentionReminderAt — kapan reminder H-3 dikirim ke staff. NULL = belum
	// pernah; dipakai job reminder supaya tidak kirim WA berulang tiap run.
	RetentionReminderAt *time.Time `json:"retention_reminder_at,omitempty"`

	// Approval (staff_draft only)
	ApprovalStatus *ApprovalStatus `gorm:"size:20" json:"approval_status,omitempty"`
	ReviewedBy     *uuid.UUID      `gorm:"type:uuid" json:"reviewed_by,omitempty"`
	ReviewedAt     *time.Time      `                 json:"reviewed_at,omitempty"`
	RevisionNotes  *string         `                 json:"revision_notes,omitempty"`
}

func (DesignFile) TableName() string { return "design_files" }
