package service

import (
	"io"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/design/model"
)

// UploadInput — payload umum untuk semua jenis upload (customer_upload,
// customer_asset, staff_draft). Service derive role dari caller + order state
// (bukan input langsung dari client — cegah spoofing).
type UploadInput struct {
	// Order identifier — pakai resi (yg ditangani handler dari :resi param).
	Resi string

	// Caller identity — dipakai untuk ownership check + audit.
	CallerID uuid.UUID
	IsStaff  bool

	// File meta — handler validasi shape (non-empty, size, mime) sebelum call.
	FileReader   io.Reader
	FileSize     int64
	MimeType     string
	OriginalName string

	// Notes opsional — brief singkat (untuk customer_asset), catatan staff
	// (untuk staff_draft).
	Notes string
}

// ApproveInput — customer approve staff draft.
type ApproveInput struct {
	DraftID    uuid.UUID
	CallerID   uuid.UUID
	IsStaff    bool
}

// RevisionInput — customer minta revisi.
type RevisionInput struct {
	DraftID  uuid.UUID
	CallerID uuid.UUID
	IsStaff  bool
	Notes    string // wajib
}

// StaffVerifyInput — staff verifikasi file customer (upload path).
type StaffVerifyInput struct {
	Resi    string
	StaffID uuid.UUID
	Note    string
}

// WalkinInstantApproveInput — staff POS mark desain approved instantly (§11).
type WalkinInstantApproveInput struct {
	Resi    string
	StaffID uuid.UUID
	Note    string
}

// FileHandle — everything the handler needs to serve a design file.
type FileHandle struct {
	File    *model.DesignFile
	AbsPath string
}
