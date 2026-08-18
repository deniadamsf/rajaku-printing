package service

import (
	"io"
	"time"

	"github.com/google/uuid"
)

// UploadInput — parameter unggah/ganti gambar satu slot.
type UploadInput struct {
	Slot         string
	FileReader   io.Reader
	FileSize     int64
	MimeType     string
	OriginalName string
	UploaderID   uuid.UUID
}

// FileHandle — hasil resolve slot ke path fisik, dipakai handler untuk
// menyajikan gambar (c.File / header Content-Type).
type FileHandle struct {
	AbsPath      string
	MimeType     string
	OriginalName string
}

// AdminSlotView — satu baris tampilan admin: gabungan definisi registry +
// nilai terisi (kalau ada). Slot yang belum diunggah tetap muncul dengan
// field media bernilai nil, supaya admin tahu slot apa saja yang tersedia.
type AdminSlotView struct {
	Slot              string `json:"slot"`
	Label             string `json:"label"`
	Description       string `json:"description"`
	SuggestedWidthPx  int    `json:"suggested_width_px"`
	SuggestedHeightPx int    `json:"suggested_height_px"`

	URL          *string    `json:"url"`
	OriginalName *string    `json:"original_name"`
	MimeType     *string    `json:"mime_type"`
	SizeBytes    *int64     `json:"size_bytes"`
	WidthPx      *int       `json:"width_px"`
	HeightPx     *int       `json:"height_px"`
	UploadedBy   *uuid.UUID `json:"uploaded_by"`
	UploadedAt   *time.Time `json:"uploaded_at"`
	UpdatedAt    *time.Time `json:"updated_at"`
}
