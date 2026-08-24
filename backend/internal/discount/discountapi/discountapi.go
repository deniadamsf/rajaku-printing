// Package discountapi is the PUBLIC contract of the discount module — the
// ONLY package other modules (order, pos) are allowed to import (§22: modul
// lain dilarang import internal/service/repository/model modul ini
// langsung).
package discountapi

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var (
	// --- Validasi pemakaian diskon (§28.4) — errors.Is, JANGAN compare string. ---

	// ErrDiscountNotFound — id tidak ada, atau sudah di-soft-delete.
	ErrDiscountNotFound = errors.New("discountapi: diskon tidak ditemukan")
	// ErrDiscountInactive — is_active = false (saklar manual admin).
	ErrDiscountInactive = errors.New("discountapi: diskon nonaktif")
	// ErrDiscountNotStarted — starts_at masih di masa depan.
	ErrDiscountNotStarted = errors.New("discountapi: diskon belum mulai berlaku")
	// ErrDiscountExpired — ends_at sudah lewat.
	ErrDiscountExpired = errors.New("discountapi: diskon sudah kadaluarsa")
	// ErrDiscountChannelMismatch — channel_scope tidak cocok channel order (online/pos).
	ErrDiscountChannelMismatch = errors.New("discountapi: diskon tidak berlaku untuk channel ini")
	// ErrDiscountMinSubtotal — subtotal order di bawah min_subtotal diskon.
	ErrDiscountMinSubtotal = errors.New("discountapi: subtotal di bawah minimum berlaku diskon")
	// ErrDiscountQuotaExhausted — jumlah order yang sudah memakai diskon ini
	// (dihitung dari orders, bukan counter tersimpan — §28.4) sudah >= quota.
	ErrDiscountQuotaExhausted = errors.New("discountapi: kuota diskon sudah habis")

	// --- Diskon manual (§28.1) ---

	// ErrManualDiscountNoteRequired — diskon manual (discount_id nil,
	// manual_amount > 0) wajib disertai alasan non-kosong.
	ErrManualDiscountNoteRequired = errors.New("discountapi: alasan wajib diisi untuk diskon manual")
	// ErrManualDiscountInvalidAmount — nominal diskon manual harus > 0.
	ErrManualDiscountInvalidAmount = errors.New("discountapi: nominal diskon manual harus lebih dari 0")
	// ErrDiscountAmbiguousInput — caller mengisi discount_id DAN
	// manual_amount sekaligus; harus pilih salah satu.
	ErrDiscountAmbiguousInput = errors.New("discountapi: tidak boleh mengisi discount_id dan diskon manual bersamaan")

	// --- CRUD master diskon ---

	ErrDiscountCodeRequired         = errors.New("discountapi: code wajib diisi")
	ErrDiscountNameRequired         = errors.New("discountapi: name wajib diisi")
	ErrDiscountTypeInvalid          = errors.New("discountapi: type harus percent atau nominal")
	ErrDiscountChannelScopeInvalid  = errors.New("discountapi: channel_scope harus all, online, atau pos")
	ErrDiscountCodeConflict         = errors.New("discountapi: code sudah dipakai diskon lain yang masih aktif")
	ErrDiscountDeleteReasonRequired = errors.New("discountapi: alasan hapus wajib diisi")

	// NOTE (review finding #7): jalur "resolver belum di-wire" dipetakan ke
	// orderapi.ErrDiscountUnavailable (lihat order/service/order_service.go)
	// — sentinel itulah yang benar-benar dicek dengan errors.Is di seluruh
	// call site (pos handler, pos service, order service). Sengaja TIDAK ada
	// sentinel kembar di sini supaya tidak ada dua sumber kebenaran untuk
	// error yang sama.

	// --- Validasi periode & pricing diskon (§28) ---

	// ErrDiscountQuotaInvalid — quota diisi TAPI <= 0 (0 berarti "tanpa
	// batas", nil juga berarti tanpa batas — ini khusus nilai eksplisit yang
	// tidak masuk akal, mis. quota=-5).
	ErrDiscountQuotaInvalid = errors.New("discountapi: quota harus lebih dari 0 kalau diisi")
	// ErrDiscountMaxAmountInvalid — max_discount_amount diisi TAPI <= 0.
	ErrDiscountMaxAmountInvalid = errors.New("discountapi: max_discount_amount harus lebih dari 0 kalau diisi")
	// ErrDiscountMaxAmountNotAllowed — max_discount_amount diisi untuk diskon
	// type="nominal", padahal computeAmount mengabaikannya total untuk tipe
	// ini (hanya relevan untuk type="percent") — §28 temuan #9(b).
	ErrDiscountMaxAmountNotAllowed = errors.New("discountapi: max_discount_amount hanya berlaku untuk diskon type percent")
	// ErrDiscountMinSubtotalInvalid — min_subtotal diisi TAPI negatif.
	ErrDiscountMinSubtotalInvalid = errors.New("discountapi: min_subtotal tidak boleh negatif")
	// ErrDiscountValuePercentInvalid — value_percent di luar rentang (0,100].
	ErrDiscountValuePercentInvalid = errors.New("discountapi: value_percent harus di antara 0 (eksklusif) sampai 100")
	// ErrDiscountValueAmountInvalid — value_amount (type=nominal) <= 0.
	ErrDiscountValueAmountInvalid = errors.New("discountapi: value_amount harus lebih dari 0")
	// ErrDiscountInvalidPeriod — ends_at <= starts_at (kalau keduanya diisi).
	// Diskon begini tersimpan tapi validateForUse menolaknya SELAMANYA tanpa
	// petunjuk (§28 temuan #3) — ditolak sejak create/update.
	ErrDiscountInvalidPeriod = errors.New("discountapi: ends_at harus setelah starts_at")
)

// ResolveInput — payload untuk menghitung potongan diskon sebuah order.
// Salah satu dari DiscountID (diskon master) ATAU ManualAmount+Note (diskon
// manual kasir) — mutually exclusive. Kalau keduanya kosong (DiscountID nil
// dan ManualAmount <= 0), berarti order ini TIDAK pakai diskon sama sekali —
// Resolver mengembalikan Snapshot kosong (Amount 0), bukan error.
type ResolveInput struct {
	DiscountID   *uuid.UUID
	ManualAmount int64
	Note         string
	Subtotal     int64
	Channel      string // "online" | "pos" — dicocokkan dengan channel_scope
}

// Snapshot — hasil resolusi diskon, SIAP disalin ke kolom snapshot orders
// (§28.2 lapis 1). Type: "percent" | "nominal" | "manual" | "" (tidak ada
// diskon dipakai).
type Snapshot struct {
	DiscountID *uuid.UUID
	Code       string
	Name       string
	Type       string
	Value      float64 // 25.00 (persen) atau 50000 (nominal); 0 untuk manual/tidak ada
	Amount     int64   // rupiah yang BENAR-BENAR dipotong, sudah dijepit ke subtotal
	Note       string
}

// Resolver adalah kontrak yang di-consume order/pos module untuk menghitung
// & memvalidasi pemakaian diskon saat order dibuat (§28.3, §28.4). Order
// module HARUS memanggil ini setelah subtotal produk terhitung, SEBELUM
// menghitung total akhir.
type Resolver interface {
	ResolveForOrder(ctx context.Context, in ResolveInput) (*Snapshot, error)
}
