package service

import (
	"time"

	"github.com/google/uuid"
)

// CreateOrderItemInput — satu baris produk untuk CreateOrderInput (§32).
// DesignSource/DesignBrief/ItemNotes sekarang PER ITEM (§32.5) — satu order
// walk-in boleh campur: banner A bawa desain sendiri, banner B minta
// dibuatkan. Harga dihitung ulang di order module lewat catalog.Quote —
// service ini TIDAK trust unit_price dari client, jadi field itu sengaja
// tidak ada di sini.
type CreateOrderItemInput struct {
	ProductID  uuid.UUID
	MaterialID uuid.UUID
	WidthCm    int
	HeightCm   int
	Quantity   int

	DesignSource string // "upload" | "request"
	DesignBrief  string
	ItemNotes    string
}

// CreateOrderInput — payload dari kasir. Nomor WA raw (di-normalize di service).
type CreateOrderInput struct {
	KasirID uuid.UUID

	// Customer identity
	CustomerName  string
	CustomerPhone string // raw, akan di-normalize

	// Items — 1..20 baris produk (§32.4). Handler memvalidasi shape
	// (min=1,max=20); ErrNoItems/ErrTooManyItems tetap ditegakkan lagi di
	// orderapi (§22 defense in depth — validasi bisnis bukan tanggung jawab
	// binding tag).
	Items []CreateOrderItemInput

	// Fulfillment
	MetodeAmbil            string // "pickup" | "kirim"
	ShippingAddress        string
	ShippingRecipientName  string
	ShippingRecipientPhone string
	ShippingCost           int64

	// Payment
	MetodeBayar string // "cash" | "qris_pos"

	// Discount (§28) — mutually exclusive: DiscountID (master, dipilih dari
	// GET /admin/discounts/applicable) ATAU ManualDiscountAmount+DiscountNote
	// (nego di tempat, kasir). Handler menolak field ini dari kasir tanpa
	// permission discount.apply — lihat pos/handler/pos_handler.go.
	DiscountID           *uuid.UUID
	ManualDiscountAmount int64
	DiscountNote         string

	// DesignApprovalMode tetap PER ORDER (§11, §32.5) — walau item-nya
	// banyak, mode approval walk-in satu untuk seluruh order.
	DesignApprovalMode string // "instant_walkin" | "async_notify"

	Notes string
}

// CreateOrderResult — data yg dikembalikan ke kasir setelah order selesai.
// Cukup buat cetak struk & tampilkan konfirmasi di layar.
type CreateOrderResult struct {
	OrderID       uuid.UUID `json:"order_id"`
	Resi          string    `json:"resi"`
	CustomerID    uuid.UUID `json:"customer_id"`
	Total         int64     `json:"total"`
	MetodeBayar   string    `json:"metode_bayar"`
	MetodeAmbil   string    `json:"metode_ambil"`
	CreatedAt     time.Time `json:"created_at"`
	TrackingURL   string    `json:"tracking_url"`
	InvoiceURL    string    `json:"invoice_url,omitempty"` // "" kalau auto-invoice gagal
	InvoiceNumber string    `json:"invoice_number,omitempty"`
	// ReceiptWidthMM — lebar kertas struk (58/80 mm) untuk CSS `@page` di
	// layar kasir. Dibaca dari setting pos.receipt_width_mm; fallback ke
	// default kalau setting tidak tersedia (lihat resolveReceiptWidthMM).
	ReceiptWidthMM int `json:"receipt_width_mm"`

	// Rincian pelanggan & item — supaya struk yang dicetak menyebut nama
	// pembeli & barang yang dibeli, bukan cuma resi/tanggal/total.
	CustomerName  string `json:"customer_name"`
	CustomerPhone string `json:"customer_phone"`
	// Items — satu baris struk per produk (§32.7). Terurut sama seperti
	// order_items (LineNo ASC) — lihat sum.Items di orderapi.OrderSummary.
	Items []ReceiptItem `json:"items"`
	// Subtotal — agregat order (Σ item.subtotal, §32.2), BUKAN item pertama.
	// Layar uang selalu baca angka ini, tidak pernah menjumlahkan Items.
	Subtotal int64 `json:"subtotal"`
	// DiscountAmount/DiscountLabel — §28.7/§32.7: struk kasir menampilkan
	// SATU baris diskon agregat (bukan per item) HANYA kalau DiscountAmount
	// > 0 (frontend TIDAK boleh cetak "Diskon Rp 0"). DiscountLabel sudah
	// final (nama snapshot, atau "Diskon" untuk manual — dihitung sekali di
	// order module).
	DiscountAmount int64  `json:"discount_amount"`
	DiscountLabel  string `json:"discount_label,omitempty"`
	// ShippingCost — nil kalau pickup / belum di-set (§8).
	ShippingCost *int64 `json:"shipping_cost,omitempty"`
}

// ReceiptItem — satu baris produk untuk struk/tampilan kasir (§32.7).
type ReceiptItem struct {
	ProductName  string `json:"product_name"`
	MaterialName string `json:"material_name"`
	WidthCm      int    `json:"width_cm"`
	HeightCm     int    `json:"height_cm"`
	Quantity     int    `json:"quantity"`
	UnitPrice    int64  `json:"unit_price"`
	Subtotal     int64  `json:"subtotal"`
}

// ReconciliationReport — laporan harian POS (§11 rekonsiliasi).
type ReconciliationReport struct {
	Date          time.Time              `json:"date"`
	TotalOrders   int                    `json:"total_orders"`
	TotalRevenue  int64                  `json:"total_revenue"`
	ByMetodeBayar map[string]MethodStats `json:"by_metode_bayar"` // key: cash | qris_pos
	ByKasir       []KasirStats           `json:"by_kasir"`
}

type MethodStats struct {
	Count   int   `json:"count"`
	Revenue int64 `json:"revenue"`
}

type KasirStats struct {
	KasirID uuid.UUID `json:"kasir_id"`
	Count   int       `json:"count"`
	Revenue int64     `json:"revenue"`
}

// CustomerSearchResult — satu baris hasil pencarian pelanggan existing (§11).
type CustomerSearchResult struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Phone string    `json:"phone"`
	// MembershipStatus — proyeksi authapi.Identity.MembershipStatus (§30.4),
	// dipakai layar kasir untuk menampilkan badge "Member" tanpa round-trip
	// terpisah ke modul membership. "none"|"pending"|"active"|"rejected"|"revoked".
	MembershipStatus string `json:"membership_status"`
}

// ReceiptConfig — konfigurasi lebar kertas struk yang boleh dibaca staff
// mana pun (bukan cuma kasir) untuk keperluan cetak ulang struk dari halaman
// detail order. WidthMM adalah nilai aktif (hasil resolveReceiptWidthMM);
// AllowedWidthsMM daftar lebar yang didukung sistem (settingsapi.POSReceiptWidthsMM).
type ReceiptConfig struct {
	WidthMM         int   `json:"width_mm"`
	AllowedWidthsMM []int `json:"allowed_widths_mm"`
}
