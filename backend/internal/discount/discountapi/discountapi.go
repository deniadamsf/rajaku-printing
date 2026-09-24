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
	ErrDiscountTypeInvalid          = errors.New("discountapi: type harus percent, nominal, atau nominal_per_m2")
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
	// ini (hanya relevan untuk type="percent" atau "nominal_per_m2") — §28 temuan #9(b).
	ErrDiscountMaxAmountNotAllowed = errors.New("discountapi: max_discount_amount hanya berlaku untuk diskon type percent atau nominal_per_m2")
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

	// --- Cakupan diskon per produk (§28.9) ---

	// ErrDiscountAppliesToInvalid — applies_to bukan "all" atau "selected".
	ErrDiscountAppliesToInvalid = errors.New("discountapi: applies_to harus all atau selected")
	// ErrDiscountScopeEmpty — applies_to="selected" tapi daftar produknya
	// kosong (belum diisi admin, ATAU seluruh produk cakupannya sudah tidak
	// lagi tercakup). Daftar kosong TIDAK PERNAH berarti "berlaku untuk
	// semua" — ditolak DUA KALI: saat create/update DAN saat validateForUse
	// dipakai (produk bisa berubah cakupannya setelah diskon dibuat).
	ErrDiscountScopeEmpty = errors.New("discountapi: cakupan produk diskon kosong")
	// ErrDiscountProductMismatch — applies_to="selected" dan product_id order
	// yang mau memakainya tidak ada dalam daftar cakupan.
	ErrDiscountProductMismatch = errors.New("discountapi: diskon tidak berlaku untuk produk ini")
	// ErrDiscountProductNotFound — salah satu product_id yang dikirim di
	// create/PATCH (product_ids) tidak ditemukan di katalog produk (temuan
	// review #3). Sebelumnya ini lolos validasi lalu meledak jadi FK
	// violation mentah (500) di repository.
	ErrDiscountProductNotFound = errors.New("discountapi: satu atau lebih product_id tidak ditemukan")

	// --- Diskon khusus member (§30.3) ---

	// ErrDiscountAudienceScopeInvalid — audience_scope bukan "all" atau
	// "member".
	ErrDiscountAudienceScopeInvalid = errors.New("discountapi: audience_scope harus all atau member")
	// ErrDiscountMemberScopeInvalid — member_scope bukan "all_members"/
	// "selected_members" yang valid, ATAU diisi padahal audience_scope bukan
	// "member", ATAU kosong padahal audience_scope="member" (kombinasi
	// keduanya wajib konsisten — CHECK constraint di DB, migration 000031,
	// jadi lapis terakhir; sentinel ini lapis pertama di service supaya
	// admin dapat pesan yang jelas sebelum meledak jadi 500 di DB).
	ErrDiscountMemberScopeInvalid = errors.New("discountapi: member_scope tidak valid untuk audience_scope yang dipilih")
	// ErrDiscountMemberScopeEmpty — member_scope="selected_members" tapi
	// discount_customers kosong (belum diisi admin, ATAU seluruh customer
	// cakupannya sudah tidak lagi member aktif — TIDAK relevan di sini,
	// keanggotaan tabelnya independen dari status membership). Daftar kosong
	// TIDAK PERNAH berarti "berlaku untuk semua member" — ditolak DUA KALI:
	// saat create/update DAN saat validateForUse dipakai (mirror
	// ErrDiscountScopeEmpty §28.9).
	ErrDiscountMemberScopeEmpty = errors.New("discountapi: cakupan member diskon kosong")
	// ErrDiscountCustomerNotFound — salah satu customer_id yang dikirim di
	// create/PATCH (customer_ids) tidak ditemukan (mirror
	// ErrDiscountProductNotFound).
	ErrDiscountCustomerNotFound = errors.New("discountapi: satu atau lebih customer_id tidak ditemukan")
	// ErrDiscountMembershipDisabled — audience_scope="member" dipakai untuk
	// membuat order, tapi setting membership_enabled=false secara global
	// (§30.1). Ditolak SAAT DIPAKAI, bukan saat create/update — admin boleh
	// menyiapkan diskon member sebelum fitur diaktifkan.
	ErrDiscountMembershipDisabled = errors.New("discountapi: fitur membership sedang nonaktif, diskon khusus member tidak bisa dipakai")
	// ErrDiscountMembershipRequired — audience_scope="member" tapi customer
	// order bukan member aktif (termasuk guest tanpa customer_id / order
	// tanpa customer_id sama sekali).
	ErrDiscountMembershipRequired = errors.New("discountapi: diskon ini khusus untuk member aktif")
	// ErrDiscountMemberMismatch — member_scope="selected_members" dan
	// customer_id order yang mau memakainya tidak ada dalam daftar cakupan.
	ErrDiscountMemberMismatch = errors.New("discountapi: diskon tidak berlaku untuk pelanggan ini")
	// ErrDiscountMembershipUnavailable — audience_scope="member" dipakai
	// tapi resolver membership/settings belum di-wire (nil) di discount
	// service (§22 no-silent-stub — jangan diam-diam anggap "bukan member"
	// kalau checker-nya memang belum ter-wire, mirror
	// orderapi.ErrDiscountUnavailable).
	ErrDiscountMembershipUnavailable = errors.New("discountapi: validasi membership belum siap, coba lagi nanti")
)

// ResolveItem — satu baris order (§32) yang ikut dihitung diskonnya.
// LineNo dipakai memetakan hasil alokasi (Snapshot.Allocations) balik ke
// baris asalnya — ProductID uuid.Nil berarti item ini tidak punya produk
// katalog (seharusnya tidak terjadi di order nyata, tapi tidak menghentikan
// hitungan: item begitu cukup dianggap "tidak eligible" untuk diskon
// applies_to='selected').
type ResolveItem struct {
	LineNo       int
	ProductID    uuid.UUID
	Subtotal     int64
	PricingType  string // "per_m2" | "paket"
	WidthCm      int
	HeightCm     int
	Quantity     int
	ChargeableM2 float64 // luas tertagih m2 untuk baris ini (sudah dikali qty jika ada)
}

// EffectiveAreaM2 mengembalikan luas tertagih dalam meter persegi untuk baris ini.
func (it ResolveItem) EffectiveAreaM2() float64 {
	if it.ChargeableM2 > 0 {
		return it.ChargeableM2
	}
	if it.PricingType == "per_m2" || (it.PricingType == "" && it.WidthCm > 0 && it.HeightCm > 0) {
		qty := it.Quantity
		if qty <= 0 {
			qty = 1
		}
		return (float64(it.WidthCm) / 100.0) * (float64(it.HeightCm) / 100.0) * float64(qty)
	}
	return 0
}

// ResolveInput — payload untuk menghitung potongan diskon sebuah order.
// Salah satu dari DiscountID (diskon master) ATAU ManualAmount+Note (diskon
// manual kasir) — mutually exclusive. Kalau keduanya kosong (DiscountID nil
// dan ManualAmount <= 0), berarti order ini TIDAK pakai diskon sama sekali —
// Resolver mengembalikan Snapshot kosong (Amount 0), bukan error.
type ResolveInput struct {
	DiscountID   *uuid.UUID
	ManualAmount int64
	Note         string
	// Items — 1..20 baris order (§32.4). Basis hitung diskon (§32.3):
	//   - applies_to='all' / diskon manual → basis = Σ semua item.Subtotal
	//   - applies_to='selected'            → basis = Σ item ELIGIBLE saja
	//     (product_id ada di discount_products); kalau tidak ada satu pun
	//     yang eligible → ErrDiscountProductMismatch.
	Items   []ResolveItem
	Channel string // "online" | "pos" — dicocokkan dengan channel_scope
	// CustomerID — customer order ini, dicocokkan dengan status membership &
	// cakupan diskon kalau audience_scope="member" (§30.3). uuid.Nil berarti
	// "tidak ada customer" (order guest tanpa customer_id) — selalu gagal
	// ErrDiscountMembershipRequired untuk diskon audience_scope="member".
	// Diabaikan untuk diskon manual & diskon audience_scope="all".
	CustomerID uuid.UUID
}

// ItemAllocation — bagian dari Snapshot.Amount yang jatuh ke satu baris
// order (LineNo), hasil metode sisa terbesar (§32.3). Σ semua Amount di
// sini SELALU persis sama dengan Snapshot.Amount — dijamin oleh algoritma
// alokasi, wajib dijaga unit test (discount/service/calc_test.go).
type ItemAllocation struct {
	LineNo int
	Amount int64
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
	// Allocations — Amount di atas dipecah per baris order (§32.3), SATU
	// entri untuk SETIAP item di ResolveInput.Items (termasuk yang tidak
	// eligible, atau saat Amount == 0 total — semuanya Amount 0 dalam
	// kondisi itu) — pemanggil tidak perlu menebak baris mana yang tidak
	// kebagian. Kosong ("nil"/panjang 0) HANYA kalau Snapshot ini
	// menggambarkan "tidak ada diskon sama sekali" (Type == "").
	Allocations []ItemAllocation
}

// Resolver adalah kontrak yang di-consume order/pos module untuk menghitung
// & memvalidasi pemakaian diskon saat order dibuat (§28.3, §28.4). Order
// module HARUS memanggil ini setelah subtotal produk terhitung, SEBELUM
// menghitung total akhir.
type Resolver interface {
	ResolveForOrder(ctx context.Context, in ResolveInput) (*Snapshot, error)
}
