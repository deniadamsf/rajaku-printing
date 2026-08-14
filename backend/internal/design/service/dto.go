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

	// ScopedOrderID — non-nil kalau caller memakai token guest_order
	// (authapi.Identity.OrderID). Token itu terbit untuk SATU order spesifik;
	// tanpa dicek di sini, token dari resi A akan tetap lolos ownership check
	// (order.CustomerID == callerID) untuk resi LAIN milik nomor WA yang
	// sama. nil untuk sesi penuh (login/register) — tidak ada pembatasan
	// tambahan selain ownership check yang sudah ada.
	ScopedOrderID *uuid.UUID

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
	DraftID  uuid.UUID
	CallerID uuid.UUID
	IsStaff  bool
	// ScopedOrderID — lihat dokumentasi UploadInput.ScopedOrderID.
	ScopedOrderID *uuid.UUID
}

// RevisionInput — customer minta revisi.
type RevisionInput struct {
	DraftID  uuid.UUID
	CallerID uuid.UUID
	IsStaff  bool
	Notes    string // wajib
	// ScopedOrderID — lihat dokumentasi UploadInput.ScopedOrderID.
	ScopedOrderID *uuid.UUID
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
