// Package sitemediaapi — public contract modul sitemedia (§22: modul lain
// hanya boleh depend pada package ini, bukan internal service/repository).
// Belum ada modul lain yang consume sitemedia langsung; package ini berisi
// sentinel error yang di-map handler ke HTTP status.
package sitemediaapi

import "errors"

var (
	// ErrUnknownSlot — caller mereferensikan slot yang tidak terdaftar di
	// model.Registry. Slot baru harus ditambah di registry (kode Go), bukan
	// dibuat bebas dari request.
	ErrUnknownSlot = errors.New("sitemediaapi: unknown slot")
	// ErrSlotEmpty — slot valid tapi belum pernah diisi (tidak ada baris DB),
	// dikembalikan saat GetFile / Delete dipanggil untuk slot kosong.
	ErrSlotEmpty = errors.New("sitemediaapi: slot has no media uploaded yet")

	ErrImageEmpty        = errors.New("sitemediaapi: image file is empty")
	ErrImageTooLarge     = errors.New("sitemediaapi: image exceeds size limit")
	ErrImageInvalidType  = errors.New("sitemediaapi: image mime type not allowed (jpeg/png/webp only)")
	ErrImageDecodeFailed = errors.New("sitemediaapi: image decode failed (corrupt file?)")
)
