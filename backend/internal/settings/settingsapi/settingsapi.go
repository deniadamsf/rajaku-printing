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

	// KeyPaymentBankName — nama bank tujuan transfer (§7 — pembayaran manual).
	KeyPaymentBankName = "payment.bank_name"
	// KeyPaymentAccountName — nama pemilik rekening tujuan transfer.
	KeyPaymentAccountName = "payment.account_name"
	// KeyPaymentAccountNumber — nomor rekening tujuan transfer. Disimpan
	// ternormalisasi (digit saja, spasi/strip dibuang) oleh settings/service.
	KeyPaymentAccountNumber = "payment.account_number"
	// KeyPaymentQRISNote — instruksi teks di bawah gambar QRIS. Boleh kosong.
	KeyPaymentQRISNote = "payment.qris_note"
	// KeyPaymentQRISMerchantName — nama merchant yang tampil saat QRIS
	// dipindai (bisa beda dari nama rekening bank). Boleh kosong.
	KeyPaymentQRISMerchantName = "payment.qris_merchant_name"
	// KeyPaymentQRISNmid — National Merchant ID QRIS. Boleh kosong.
	KeyPaymentQRISNmid = "payment.qris_nmid"

	// KeyPOSReceiptWidthMM — lebar kertas struk thermal POS dalam mm, hanya
	// boleh 58 atau 80 (§11/§12 CLAUDE.md — thermal printer 58mm/80mm).
	// Dikembalikan ke kasir saat order POS dibuat supaya frontend bisa atur
	// CSS `@page` sesuai roll printer yang terpasang.
	KeyPOSReceiptWidthMM = "pos.receipt_width_mm"

	// KeyMembershipEnabled — saklar on/off fitur membership customer (§30
	// CLAUDE.md). Saat false: halaman "Ajukan jadi Member" disembunyikan DAN
	// service menolak pengajuan baru (ErrMembershipDisabled) — bukan cuma
	// UI-only hiding (§30.1, pola yang sama dilarang berulang di §22/§28.9).
	// Data member yang sudah ada TIDAK ikut ter-reset saat dimatikan.
	KeyMembershipEnabled = "membership_enabled"
)

// POSReceiptWidthsMM — SATU-SATUNYA daftar lebar roll thermal yang didukung
// sistem (§12). Sengaja tinggal di sini, bukan diduplikasi per modul: dipakai
// bareng oleh settings/service (validasi nilai yang BOLEH DISIMPAN) dan
// pos/service (validasi nilai yang DIBACA dari DB). Kalau daftarnya terpecah,
// menambah ukuran baru (mis. 76mm) di satu sisi saja bikin admin bisa memilih
// 76 lalu kasir diam-diam dapat struk 58mm — gagal senyap, bukan gagal keras.
//
// Menambah ukuran: cukup satu baris di sini (frontend & seed migration masih
// perlu disesuaikan terpisah).
var POSReceiptWidthsMM = []int{58, 80}

// DefaultPOSReceiptWidthMM — lebar yang dipakai kalau setting tidak terbaca
// atau nilainya rusak. Harus sama dengan nilai seed migration 000022.
const DefaultPOSReceiptWidthMM = 58

// IsValidPOSReceiptWidthMM melaporkan apakah mm termasuk lebar yang didukung.
func IsValidPOSReceiptWidthMM(mm int) bool {
	for _, w := range POSReceiptWidthsMM {
		if mm == w {
			return true
		}
	}
	return false
}

// Reader — kontrak baca untuk modul consumer (mis. design retention job).
// Sengaja minimal: consumer cuma butuh nilai, tidak butuh metadata/CRUD.
type Reader interface {
	// GetInt mengembalikan nilai setting sebagai integer.
	// Return ErrSettingNotFound kalau key tidak ada di DB, ErrInvalidValue
	// kalau nilainya bukan integer valid.
	GetInt(ctx context.Context, key string) (int, error)
	// GetBool mengembalikan nilai setting sebagai boolean (mis.
	// membership_enabled, §30). Return ErrSettingNotFound kalau key tidak ada
	// di DB, ErrInvalidValue kalau nilainya bukan "true"/"false" valid.
	GetBool(ctx context.Context, key string) (bool, error)
}
