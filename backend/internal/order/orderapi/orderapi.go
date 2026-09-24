// Package orderapi is the PUBLIC contract of the order module — only this
// package boleh diimport oleh modul lain (payment, design, production nanti).
package orderapi

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrOrderNotFound          = errors.New("orderapi: order not found")
	ErrNotOwner               = errors.New("orderapi: caller is not the order owner")
	ErrInvalidTransition      = errors.New("orderapi: invalid state transition")
	ErrResiCollisionGaveUp    = errors.New("orderapi: could not generate unique resi after retries")
	ErrShippingFieldsRequired = errors.New("orderapi: shipping address/recipient/phone required when metode_ambil=kirim")
	ErrPickupNoShipping       = errors.New("orderapi: pickup order must not carry shipping_cost")
	// ErrShippingCostOnPickup — admin tried to set ongkir on a pickup order.
	ErrShippingCostOnPickup = errors.New("orderapi: shipping_cost cannot be set on pickup order")
	// ErrConfirmPickupOnKirim — admin tried to confirm-pickup a kirim order.
	ErrConfirmPickupOnKirim = errors.New("orderapi: confirm-pickup only valid for metode_ambil=pickup")
	// ErrOrderStateChanged — the order moved to a different status between
	// read and update (concurrent staff action). UI should refresh & retry.
	ErrOrderStateChanged = errors.New("orderapi: order state changed concurrently")
	// ErrInvalidShippingCost — negative or absurd ongkir value.
	ErrInvalidShippingCost = errors.New("orderapi: shipping_cost must be >= 0")
	// ErrPaymentAlreadySettled — attempt to submit payment proof for an order
	// already in dibayar / selesai / dibatalkan (nothing to do).
	ErrPaymentAlreadySettled = errors.New("orderapi: order already settled")

	// ErrDikirimOnPickup — attempt to mark shipped on a pickup order.
	ErrDikirimOnPickup = errors.New("orderapi: mark-shipped only valid for metode_ambil=kirim")
	// ErrShippingTrackingRequired — courier/tracking wajib saat mark dikirim.
	ErrShippingTrackingRequired = errors.New("orderapi: courier & tracking number required when marking shipped")

	// --- Super admin order tools (§ super admin order tools) ---

	// ErrReasonRequired — admin tried to edit financial fields post-payment,
	// override status, or soft-delete an order without a sufficiently
	// detailed reason (min 10 chars for those; see EditOrder/OverrideStatus/
	// SoftDeleteOrder doc for exactly when this applies).
	ErrReasonRequired = errors.New("orderapi: reason required for this admin action")
	// ErrFieldNotEditable — admin tried to edit an order that's in a terminal
	// status (selesai/dibatalkan) — those are closed records; use
	// OverrideStatus first if genuinely need to reopen one.
	ErrFieldNotEditable = errors.New("orderapi: order status does not allow editing")
	// ErrStatusUnknown — OverrideStatus target isn't a status state.IsKnown() recognizes.
	ErrStatusUnknown = errors.New("orderapi: unknown target status")
	// ErrDeleteNotAllowedPaid — SoftDeleteOrder was called on an order whose
	// status is at/after `dibayar` (and isn't `dibatalkan`) — see
	// state.IsDeletable. A paid order must be cancelled first (via
	// OverrideStatus → dibatalkan) before it can be soft-deleted, otherwise it
	// would vanish from cash reconciliation (ListPOSByDateRange etc. filter
	// deleted_at IS NULL) without a trace.
	ErrDeleteNotAllowedPaid = errors.New("orderapi: order already paid — batalkan dulu sebelum dihapus")

	// --- Discount integration (§28) ---

	// ErrDiscountUnavailable — an order carried a discount_id or manual
	// discount amount, but the order service was never wired with a
	// discountapi.Resolver (composition-root bug, not a user error) — §22
	// no-silent-stub: refuse explicitly instead of silently ignoring the
	// requested discount.
	ErrDiscountUnavailable = errors.New("orderapi: discount resolver not configured, tapi permintaan diskon disertakan")

	// ErrDiscountAllocationMismatch — §32.2's invariant Σ item.DiscountAmount
	// == orders.DiscountAmount tidak terpenuhi oleh discountapi.Snapshot yang
	// dikembalikan resolver (Allocations kosong padahal Amount > 0, ATAU
	// jumlah Allocations tidak persis sama dengan Amount). Order module WAJIB
	// menolak menyimpan order dengan angka yang meleset ini SENDIRI — jangan
	// hanya mempercayakan ke discount/service/calc.go di seberang batas
	// modul (§22, kelas kegagalan senyap yang sama seperti rekap/struk yang
	// diam-diam beda angka).
	ErrDiscountAllocationMismatch = errors.New("orderapi: alokasi diskon per item tidak sama dengan discount_amount order — order tidak disimpan")

	// --- Order recap (§28.5) ---

	// ErrRecapInvalidDateRange — 'to' date is before 'from' date.
	ErrRecapInvalidDateRange = errors.New("orderapi: rentang tanggal rekap tidak valid, 'to' harus >= 'from'")
	// ErrRecapDateRangeTooWide — rentang 'from'..'to' lebih dari 366 hari
	// (temuan review #4a) — membatasi memori/waktu query rekap & ekspor CSV
	// di VPS Hostinger tunggal (§19), yang tidak punya auto-scaling resource.
	ErrRecapDateRangeTooWide = errors.New("orderapi: rentang tanggal rekap maksimum 366 hari")
	// ErrRecapInvalidStatus — filter status tidak dikenal §4/state package
	// (temuan review #5) — mis. typo "dibayarkan" akan diam-diam mengembalikan
	// 0 baris kalau tidak ditolak eksplisit.
	ErrRecapInvalidStatus = errors.New("orderapi: status filter rekap tidak dikenal")
	// ErrRecapInvalidChannel — filter channel bukan 'online' atau 'pos'
	// (temuan review #5).
	ErrRecapInvalidChannel = errors.New("orderapi: channel filter rekap harus 'online' atau 'pos'")
	// ErrRecapDiscountFilterAmbiguous — caller mengirim discount_manual=true
	// DAN discount_id sekaligus; keduanya mutually exclusive (permintaan
	// frontend: discount_manual berdiri sendiri, tidak pernah bareng
	// discount_id) — ditolak eksplisit, bukan diam-diam pilih salah satu.
	ErrRecapDiscountFilterAmbiguous = errors.New("orderapi: discount_manual tidak boleh dikirim bersama discount_id")

	// --- Order multi-item (§32) ---

	// ErrNoItems — order wajib punya minimal 1 item (§32.4).
	ErrNoItems = errors.New("orderapi: order wajib punya minimal 1 item")
	// ErrTooManyItems — maksimal 20 item per order (§32.4) — tanpa batas ini
	// satu request bisa memaksa ratusan Quote ke katalog sekaligus.
	ErrTooManyItems = errors.New("orderapi: maksimal 20 item per order")
	// ErrOrderSubtotalNotEditable — super admin mencoba mengirim
	// EditOrderInput.Subtotal dengan nilai yang BERBEDA dari subtotal order
	// saat ini. Sejak §32, subtotal order adalah TURUNAN (Σ item.subtotal,
	// §32.2) — mengeditnya langsung akan membuatnya menyimpang dari jumlah
	// item yang sebenarnya tanpa ada yang error (persis kelas kegagalan
	// senyap yang dilarang §22). Koreksi subtotal harus lewat koreksi item
	// (EditOrderInput.Items, §32.9), bukan lewat field ini — field Subtotal
	// dipertahankan HANYA supaya frontend yang mengirim balik nilai lama
	// (echo dari GET sebelumnya, bukan permintaan perubahan) tidak ditolak;
	// lihat EditOrder doc untuk kenapa perbandingan "berubah" dan bukan
	// "terkirim" yang dipakai.
	ErrOrderSubtotalNotEditable = errors.New("orderapi: subtotal tidak bisa diedit langsung — diturunkan dari item pesanan (§32), koreksi lewat items")

	// --- Koreksi baris item oleh super admin (§32.9) ---

	// ErrOrderItemNotFound — EditOrderInput.Items menyebut sebuah item ID
	// yang tidak ada di order ini (mungkin sudah dihapus admin lain
	// bersamaan, atau salah ketik id dari frontend).
	ErrOrderItemNotFound = errors.New("orderapi: baris item pesanan tidak ditemukan")
	// ErrOrderItemProductNotEditable — caller mencoba mengubah product_id/
	// material_id/design_source/design_brief pada baris item yang SUDAH ADA
	// (ID != nil). §32.9: mengganti produk sebuah baris berarti pesanan yang
	// berbeda — hapus barisnya (jangan sertakan di Items) lalu tambah baris
	// baru (ID nil) untuk produk yang benar.
	ErrOrderItemProductNotEditable = errors.New("orderapi: product_id/material_id/design_source baris item yang sudah ada tidak bisa diubah — hapus baris ini lalu tambah baris baru")
	// ErrOrderItemInvalid — bentuk baris item tidak valid (width_cm/
	// height_cm/quantity <= 0, unit_price < 0 untuk baris existing, atau
	// product_id/material_id kosong untuk baris baru).
	ErrOrderItemInvalid = errors.New("orderapi: baris item pesanan tidak valid")
	// ErrOrderDiscountExceedsSubtotal — hasil edit item membuat
	// orders.subtotal yang baru lebih kecil dari orders.discount_amount yang
	// sudah tersimpan (§28.2 — discount_amount TIDAK berubah nilainya saat
	// edit item, hanya pembagiannya ke baris). Admin baru saja membuat
	// pesanan yang potongannya melebihi nilai barangnya — ditolak eksplisit,
	// bukan diam-diam dijepit (§32.9).
	ErrOrderDiscountExceedsSubtotal = errors.New("orderapi: subtotal hasil edit lebih kecil dari diskon yang sudah tercatat — tidak bisa disimpan")

	// ErrOrderItemDuplicate — EditOrderInput.Items menyebut ID baris item yang
	// SAMA lebih dari sekali. Tanpa penolakan ini, resolveItemsForEdit
	// menjumlahkan subtotal baris itu dua kali ke orders.subtotal sementara
	// repository hanya meng-UPDATE baris fisiknya sekali (ID sama) — Σ
	// item.subtotal jadi tidak sama dengan orders.subtotal tanpa satu pun
	// error (kelas kegagalan senyap §22, temuan review §32.9 #1).
	ErrOrderItemDuplicate = errors.New("orderapi: id baris item pesanan dikirim lebih dari sekali")
	// ErrOrderItemHasDesignFiles — super admin mencoba menghapus baris item
	// yang masih dirujuk oleh satu atau lebih design_files.order_item_id
	// (migration 000034, RESTRICT — sengaja tanpa ON DELETE CASCADE karena
	// §19 mempertahankan record design_files selamanya untuk rekap, hanya
	// blob fisiknya yang dihapus). Menghapus baris pesanan itu berarti
	// membuang rujukan yang masih dipakai riwayat desain — ditolak eksplisit
	// dengan pesan yang menyebut baris mana, bukan 500 generik dari
	// pelanggaran FK Postgres (temuan review §32.9 #2).
	ErrOrderItemHasDesignFiles = errors.New("orderapi: baris item pesanan masih punya file desain terkait, tidak bisa dihapus")

	// --- Order multi-item, validasi input per baris (§32) ---

	// ErrInvalidDesignSource — sebuah baris item membawa design_source
	// selain "upload"/"request". Endpoint HTTP publik sudah menolak ini lewat
	// binding tag (`oneof=upload request`) sebelum sampai ke service — ini
	// pertahanan lapis kedua untuk pemanggil non-HTTP kontrak orderapi
	// (mis. modul lain yang menyusun POSCreateOrderInput sendiri), supaya
	// input salah jatuh ke 400/422 lewat errors.Is, bukan ke 500 generik
	// lewat branch default mapErr (temuan review §22).
	ErrInvalidDesignSource = errors.New("orderapi: design_source baris item harus 'upload' atau 'request'")
)

// OrderItemView — projeksi satu baris order_items (§32) untuk konsumer lain
// (payment, notification, POS, invoice) tanpa import order/model. DiscountAmount
// di sini HANYA penjelas per-baris (§32.3 metode sisa terbesar) — layar uang
// tetap membaca OrderSummary.DiscountAmount/OrderInvoiceView.DiscountAmount
// (§32.2), jangan menjumlahkan field ini untuk hitungan uang.
type OrderItemView struct {
	// ID — order_items.id (§32.5). Dipakai modul design sebagai
	// order_item_id: setiap file desain menempel ke SATU baris item, bukan
	// ke order (design_files.order_item_id, migration 000034) — supaya
	// validasi design_source dan kepemilikan file dicek terhadap item yang
	// benar pada order campuran (mixed upload/request).
	ID             uuid.UUID
	LineNo         int
	ProductID      *uuid.UUID
	ProductName    string
	MaterialName   string
	PricingType    string
	WidthCm        int
	HeightCm       int
	Quantity       int
	UnitPrice      int64
	Subtotal       int64
	DiscountAmount int64
	DesignSource   string // "upload" | "request" (§32.5, per item)
	DesignBrief    string
	ItemNotes      string
}

// OrderSummary — projection modul lain (payment, notification, POS) butuh baca
// order tanpa import order/model. Field yang di-expose sengaja dibatasi.
type OrderSummary struct {
	ID                 uuid.UUID
	Resi               string
	CustomerID         uuid.UUID
	Status             string
	Total              int64
	MetodeAmbil        string
	MetodeBayar        string // "" kalau belum settled
	Channel            string
	DesignSource       string     // "upload" | "request" | "mixed" (§32.1, turunan dari Items)
	DesignApprovalMode *string    // "instant_walkin" | "async_notify" (§11); nil kalau belum di-set
	CreatedBy          *uuid.UUID // kasir POS (nil untuk order online)
	CreatedAt          time.Time

	// Items — daftar produk dalam order ini (§32), terurut LineNo ASC.
	// Subtotal/ShippingCost/Total di bawah TETAP di level order (§32.2) —
	// SATU-SATUNYA angka yang dipakai perhitungan uang; Items hanya untuk
	// merender rincian pesanan (mis. baris struk per produk).
	Items        []OrderItemView
	Subtotal     int64
	ShippingCost *int64 // nil kalau pickup / belum di-set

	// Discount (§28) — DiscountAmount 0 = tidak ada diskon dipakai (DiscountLabel
	// akan "" dalam kondisi itu). DiscountLabel dihitung sekali di sini
	// (order module) dari discount_name_snapshot / "Diskon" untuk manual —
	// konsumer (POS struk, invoice) tinggal pakai, tidak perlu duplikasi
	// logic "label apa untuk diskon manual" (§28.7).
	DiscountAmount         int64
	DiscountLabel          string
	ShippingRecipientPhone *string
}

// POSOrderItemInput — satu baris produk untuk POSCreateOrderInput (§32).
// Service akan Quote via catalog untuk hitung harga otoritatif per baris
// (jangan trust harga dari client). DesignSource/DesignBrief/ItemNotes
// sekarang PER ITEM (§32.5) — satu order boleh campur upload & request.
type POSOrderItemInput struct {
	ProductID  uuid.UUID
	MaterialID uuid.UUID
	WidthCm    int
	HeightCm   int
	Quantity   int

	DesignSource string // "upload" | "request"
	DesignBrief  string
	ItemNotes    string
}

// POSCreateOrderInput — payload untuk create walk-in order (§11).
// Customer sudah harus resolved (nomor WA → CustomerID via authapi.CustomerService).
// Metode bayar wajib POS-specific: cash atau qris_pos.
type POSCreateOrderInput struct {
	CustomerID uuid.UUID
	KasirID    uuid.UUID // staff yg input order (untuk rekonsiliasi)

	// Items — 1..20 baris produk (§32.4, ErrNoItems/ErrTooManyItems).
	Items []POSOrderItemInput

	// Fulfillment
	MetodeAmbil            string // "pickup" | "kirim"
	ShippingAddress        string // wajib kalau kirim
	ShippingRecipientName  string
	ShippingRecipientPhone string // raw, service normalize
	ShippingCost           int64  // POS: kasir hitung langsung, tidak async

	// Payment (POS-only)
	MetodeBayar string // "cash" | "qris_pos"

	// Discount (§28) — mutually exclusive: DiscountID (master) ATAU
	// ManualDiscountAmount+DiscountNote (manual kasir). Kosong semua = order
	// ini tidak pakai diskon (kasus normal, bukan error). Diselesaikan lewat
	// discountapi.Resolver — order service TIDAK tahu aturan validasi
	// diskon, cuma menyalurkan input ke Resolver lalu menyalin hasilnya
	// (discountapi.Snapshot) ke kolom snapshot orders DAN mengalokasikan
	// Snapshot.Allocations ke discount_amount masing-masing item (§32.3).
	DiscountID           *uuid.UUID
	ManualDiscountAmount int64
	DiscountNote         string

	// DesignApprovalMode tetap PER ORDER (§11) — walau item-nya banyak,
	// mode approval walk-in ("instant_walkin"/"async_notify") satu untuk
	// seluruh order.
	DesignApprovalMode string // "instant_walkin" | "async_notify" (§11); "" = tidak di-set

	Notes string
}

// OrderInvoiceView — projection lebih lengkap khusus modul invoice: mencakup
// line item snapshot, alamat, dan total. Tetap terbatas — jangan expose
// internal seperti design_source atau reject reason.
type OrderInvoiceView struct {
	ID          uuid.UUID
	Resi        string
	CustomerID  uuid.UUID
	CreatedAt   time.Time
	Channel     string
	Status      string
	MetodeAmbil string
	MetodeBayar string // "" kalau belum settled

	// Items — daftar produk dalam order ini (§32), terurut LineNo ASC.
	// Invoice PDF/struk mencetak SATU BARIS PER ITEM (§32.7); Subtotal di
	// bawah TETAP di level order, satu-satunya angka dipakai perhitungan
	// uang.
	Items    []OrderItemView
	Subtotal int64

	// Discount (§28.7) — DiscountAmount 0 = tidak ditampilkan sama sekali di
	// invoice/struk (jangan cetak "Diskon Rp 0"). DiscountLabel sudah final
	// (nama snapshot, atau "Diskon" untuk manual) — lihat OrderSummary doc.
	DiscountAmount int64
	DiscountLabel  string

	// Fulfillment
	ShippingCost       *int64
	ShippingAddress    *string
	ShippingRecipient  *string
	ShippingPhone      *string
	ShippingCourier    *string
	ShippingTrackingNo *string

	Total int64
	Notes *string
}

// OrderCommandService — kontrak untuk modul lain yang perlu memerintahkan
// transisi status order. Order module mengontrol validasi state machine di
// implementasinya — modul consumer TIDAK boleh punya pengetahuan state machine.
//
// actorID nullable — nil kalau system-triggered (mis. cron/job). Kalau customer
// atau staff, isi UserID untuk audit trail (state_history.changed_by).
type OrderCommandService interface {
	// FindSummaryByResi returns a projection of the order — used by consumer
	// modules to guard against acting on nonexistent or already-settled orders.
	FindSummaryByResi(ctx context.Context, resi string) (*OrderSummary, error)

	// FindSummaryByID returns the projection by primary key. Used when a
	// consumer holds only an order_id foreign key (e.g. payment_proofs.order_id).
	FindSummaryByID(ctx context.Context, id uuid.UUID) (*OrderSummary, error)

	// FindInvoiceViewByID returns the richer OrderInvoiceView. Dipakai modul
	// invoice untuk render PDF (line items + alamat + total).
	FindInvoiceViewByID(ctx context.Context, id uuid.UUID) (*OrderInvoiceView, error)

	// MarkPendingVerification advances the order to menunggu_verifikasi.
	// Valid from menunggu_pembayaran (first upload) or ditolak (re-upload).
	MarkPendingVerification(ctx context.Context, orderID uuid.UUID, actorID *uuid.UUID, note string) error

	// MarkDibayar advances the order to dibayar and records metode_bayar.
	// Valid from menunggu_verifikasi (staff approves proof) or menunggu_pembayaran
	// (POS cash/qris — no proof needed).
	MarkDibayar(ctx context.Context, orderID uuid.UUID, metodeBayar string, actorID *uuid.UUID, note string) error

	// MarkDitolak advances the order to ditolak (staff rejected the proof).
	// Valid only from menunggu_verifikasi. Reason is required, stored in the
	// state_history note.
	MarkDitolak(ctx context.Context, orderID uuid.UUID, actorID *uuid.UUID, reason string) error

	// --- Design flow transitions (§6, §11) ---

	// MarkDesainDikerjakan — staff mulai/lanjut kerjakan desain.
	// Valid dari: dibayar (initial) atau menunggu_approval_desain (revisi).
	MarkDesainDikerjakan(ctx context.Context, orderID uuid.UUID, actorID *uuid.UUID, note string) error

	// MarkMenungguApprovalDesain — staff selesai draft, kirim ke customer.
	// Valid dari: desain_dikerjakan.
	MarkMenungguApprovalDesain(ctx context.Context, orderID uuid.UUID, actorID *uuid.UUID, note string) error

	// MarkDesainDiverifikasi — desain final, siap masuk cetak.
	// Valid dari:
	//   - dibayar               (upload path: customer upload file valid & staff verifikasi)
	//   - menunggu_approval_desain (request path: customer approve draft)
	//   - desain_dikerjakan     (walk-in POS: "Disetujui Langsung" §11)
	MarkDesainDiverifikasi(ctx context.Context, orderID uuid.UUID, actorID *uuid.UUID, note string) error

	// --- Production flow transitions (§3.7, §4) ---

	// MarkProsesCetak — staff mulai cetak. Valid dari desain_diverifikasi.
	MarkProsesCetak(ctx context.Context, orderID uuid.UUID, actorID *uuid.UUID, note string) error

	// MarkQC — hasil cetak masuk QC. Valid dari proses_cetak.
	MarkQC(ctx context.Context, orderID uuid.UUID, actorID *uuid.UUID, note string) error

	// MarkSiapKirimAtauAmbil — lulus QC, siap diserahkan.
	// Auto-branch berdasarkan order.metode_ambil:
	//   - kirim  → siap_kirim
	//   - pickup → siap_ambil
	// Valid dari qc.
	MarkSiapKirimAtauAmbil(ctx context.Context, orderID uuid.UUID, actorID *uuid.UUID, note string) error

	// MarkDikirim — paket diserahkan ke ekspedisi. Kurir & no. resi ekspedisi
	// direcord di order (ShippingCourier, ShippingTrackingNumber).
	// Valid dari siap_kirim. Wajib metode_ambil=kirim.
	MarkDikirim(ctx context.Context, orderID uuid.UUID, actorID *uuid.UUID, courier, trackingNumber, note string) error

	// MarkSelesai — order selesai. Valid dari:
	//   - dikirim    (kirim path)
	//   - siap_ambil (pickup path — customer sudah ambil)
	MarkSelesai(ctx context.Context, orderID uuid.UUID, actorID *uuid.UUID, note string) error

	// --- POS / walk-in (§11) ---

	// CreatePOSOrder atomically creates walk-in order langsung di status
	// dibayar (skip menunggu_pembayaran & menunggu_verifikasi — cash/qris
	// di tempat, tidak butuh verifikasi async). Metode_bayar direcord.
	// Initial history row: from=null, to=dibayar (POS instant payment).
	CreatePOSOrder(ctx context.Context, in POSCreateOrderInput) (*OrderSummary, error)

	// ListPOSOrdersByDate returns all POS orders created on the given date
	// (WIB timezone). Dipakai modul POS untuk rekonsiliasi harian.
	// Result ordered by created_at ASC.
	ListPOSOrdersByDate(ctx context.Context, dateWIB time.Time) ([]OrderSummary, error)
}

// CustomerMerger — dipakai modul auth saat menyerap identitas guest ke akun
// terdaftar (§11 satu pelanggan satu riwayat). Mengembalikan ID setiap order
// yang berpindah — bukan cuma jumlahnya — supaya modul auth bisa menulis
// baris audit customer_merges (order_ids) tanpa perlu query balik ke modul
// order untuk tahu order mana saja yang tadi dipindah.
type CustomerMerger interface {
	ReassignCustomer(ctx context.Context, fromCustomerID, toCustomerID uuid.UUID) ([]uuid.UUID, error)
}

// --- Customer admin (Manajemen Pelanggan) ---
//
// Fitur "Manajemen Pelanggan" secara fisik tinggal di modul auth (pelanggan =
// baris `users` dengan user_type='customer'), tapi butuh menampilkan
// statistik/riwayat order singkat per pelanggan tanpa modul auth pernah
// mengimport package internal order (§22). CustomerOrderReader adalah
// satu-satunya jembatan itu.

// CustomerOrderStats — agregat ringkas riwayat order satu pelanggan. Setiap
// query di baliknya WAJIB mengecualikan order yang sudah soft-deleted
// (`deleted_at IS NOT NULL`, §super admin order tools) — order yang dihapus
// bukan bagian dari riwayat nyata pelanggan. TotalSpend mengecualikan status
// `dibatalkan` (state.Dibatalkan) — uang yang batal bukan belanja pelanggan.
type CustomerOrderStats struct {
	TotalOrders     int64
	CompletedOrders int64
	CancelledOrders int64
	TotalSpend      int64
	LastOrderAt     *time.Time
}

// CustomerOrderBrief — satu baris ringkas untuk daftar "order terakhir" di
// halaman detail pelanggan (admin). Sengaja sedikit field — konsumen ini
// hanya perlu menampilkan daftar, bukan detail lengkap (pakai
// OrderInvoiceView/OrderSummary kalau butuh itu).
type CustomerOrderBrief struct {
	Resi      string
	Status    string
	Channel   string
	Total     int64
	CreatedAt time.Time
}

// CustomerOrderOverview — payload gabungan GET /admin/customers/:id (modul
// auth) — statistik + N order terakhir dalam satu round-trip ke modul order.
type CustomerOrderOverview struct {
	Stats  CustomerOrderStats
	Recent []CustomerOrderBrief
}

// CustomerOrderReader — kontrak baca-saja yang dipakai modul auth
// (customer_admin_service.go) untuk merender statistik order di halaman
// detail pelanggan admin. Implementasi WAJIB mengecualikan order yang
// deleted_at IS NOT NULL dari SEMUA angka (Stats maupun Recent) — order yang
// sudah dihapus bukan bagian dari riwayat yang ditampilkan ke admin.
type CustomerOrderReader interface {
	CustomerOrderOverview(ctx context.Context, customerID uuid.UUID, recentLimit int) (*CustomerOrderOverview, error)
}
