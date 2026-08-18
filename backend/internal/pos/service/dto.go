package service

import (
	"time"

	"github.com/google/uuid"
)

// CreateOrderInput — payload dari kasir. Nomor WA raw (di-normalize di service).
type CreateOrderInput struct {
	KasirID     uuid.UUID

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
