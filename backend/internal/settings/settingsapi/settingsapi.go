// Package settingsapi is the PUBLIC contract of the settings module (§22 —
// modul lain hanya boleh import package api ini, bukan internal service/repo).
//
// Settings = key/value konfigurasi global yang boleh diubah super admin dari
// admin panel saat runtime, TANPA deploy ulang. Bedakan dengan `config`:
//
//	config (env var)  — infrastruktur: kredensial DB, URL, secret. Fail-fast
//	                    saat startup, tidak bisa diubah dari UI.
//	settings (DB)     — kebijakan bisnis: mis. retensi file desain 30 hari
//	                    (§19 "sebaiknya dibuat configurable... biar bisa diubah
//	                    tanpa deploy ulang kalau kebijakan berubah").
package settingsapi

import (
	"context"
	"errors"
)

var (
	// ErrSettingNotFound — key tidak dikenal / row-nya hilang. Consumer yang
	// punya fallback boleh treat ini sebagai "pakai default", tapi WAJIB
	// log warn — row hilang artinya seed migration bermasalah.
	ErrSettingNotFound = errors.New("settingsapi: setting not found")
	// ErrInvalidValue — nilai baru gagal validasi tipe/range untuk key tsb.
	ErrInvalidValue = errors.New("settingsapi: invalid value for setting")
	// ErrUnknownKey — caller mencoba update key yang tidak terdaftar. Setting
	// baru harus lewat migration (seed row + aturan validasinya), bukan
	// insert bebas dari API.
	ErrUnknownKey = errors.New("settingsapi: unknown setting key")
)

// Key — daftar setting yang dikenal sistem. String literal-nya harus sama
// dengan kolom `key` di tabel app_settings (di-seed lewat migration).
const (
	// KeyDesignRetentionDays — berapa hari blob file desain disimpan sebelum
	// dihapus otomatis dari disk (§19). Row DB tetap ada (audit/rekap).
	KeyDesignRetentionDays = "design_retention_days"
)

// Reader — kontrak baca untuk modul consumer (mis. design retention job).
// Sengaja minimal: consumer cuma butuh nilai, tidak butuh metadata/CRUD.
type Reader interface {
	// GetInt mengembalikan nilai setting sebagai integer.
	// Return ErrSettingNotFound kalau key tidak ada di DB, ErrInvalidValue
	// kalau nilainya bukan integer valid.
	GetInt(ctx context.Context, key string) (int, error)
}
