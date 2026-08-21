package service

import (
	"time"

	"github.com/google/uuid"
)

// CreateOrderInput — payload dari kasir. Nomor WA raw (di-normalize di service).
type CreateOrderInput struct {
	KasirID uuid.UUID

	// Customer identity
	CustomerName  string
	CustomerPhone string // raw, akan di-normalize

	// Product spec
	ProductID  uuid.UUID
	MaterialID uuid.UUID
	WidthCm    int
	HeightCm   int
	Quantity   int

	// Fulfillment
	MetodeAmbil            string // "pickup" | "kirim"
	ShippingAddress        string
	ShippingRecipientName  string
	ShippingRecipientPhone string
	ShippingCost           int64

	// Payment
	MetodeBayar string // "cash" | "qris_pos"

	// Design
	DesignSource       string // "upload" | "request"
	DesignApprovalMode string // "instant_walkin" | "async_notify"
	DesignBrief        string

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
	ProductName   string `json:"product_name"`
	MaterialName  string `json:"material_name"`
	WidthCm       int    `json:"width_cm"`
	HeightCm      int    `json:"height_cm"`
	Quantity      int    `json:"quantity"`
	UnitPrice     int64  `json:"unit_price"`
	Subtotal      int64  `json:"subtotal"`
	// ShippingCost — nil kalau pickup / belum di-set (§8).
	ShippingCost *int64 `json:"shipping_cost,omitempty"`
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

// ReceiptConfig — konfigurasi lebar kertas struk yang boleh dibaca staff
// mana pun (bukan cuma kasir) untuk keperluan cetak ulang struk dari halaman
// detail order. WidthMM adalah nilai aktif (hasil resolveReceiptWidthMM);
// AllowedWidthsMM daftar lebar yang didukung sistem (settingsapi.POSReceiptWidthsMM).
type ReceiptConfig struct {
	WidthMM         int   `json:"width_mm"`
	AllowedWidthsMM []int `json:"allowed_widths_mm"`
}
