// Package service holds order business logic.
package service

import (
	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/order/model"
)

// CreateOnlineOrderInput is what handler passes to service.
// Salah satu dari CustomerID (kalau caller sudah login) ATAU (GuestPhone + GuestName)
// wajib terisi — service akan resolve identity dari mana pun yg tersedia.
type CreateOnlineOrderInput struct {
	// Identity — mutually exclusive:
	CustomerID *uuid.UUID // set dari authapi.Identity kalau user login
	GuestPhone string     // raw, di-normalize di service
	GuestName  string

	// Product / spec:
	ProductID  uuid.UUID
	MaterialID uuid.UUID
	WidthCm    int
	HeightCm   int
	Quantity   int

	// Fulfillment:
	MetodeAmbil            model.MetodeAmbil
	ShippingAddress        string // wajib kalau kirim
	ShippingRecipientName  string // wajib kalau kirim
	ShippingRecipientPhone string // wajib kalau kirim (di-normalize)

	// Design flow (section 6):
	DesignSource model.DesignSource
	DesignBrief  string // untuk request

	// Meta:
	Notes string
}

// PublicTrackingResult — data yg boleh dilihat siapa saja (sensor field
// sensitif per spec section 5).
type PublicTrackingResult struct {
	Resi                  string                           `json:"resi"`
	Status                string                           `json:"status"`
	Channel               string                           `json:"channel"`
	MetodeAmbil           string                           `json:"metode_ambil"`
	ProductName           string                           `json:"product_name"`
	MaterialName          string                           `json:"material_name"`
	ShippingRecipient     string                           `json:"shipping_recipient,omitempty"`     // "Ani T***" (masked)
	ShippingPhoneMasked   string                           `json:"shipping_phone,omitempty"`         // "0812****678"
	ShippingAddressMasked string                           `json:"shipping_address,omitempty"`       // "Jl. Merdek**"
	CreatedAt             string                           `json:"created_at"`
	History               []PublicTrackingResultHistoryRow `json:"history"`
}

type PublicTrackingResultHistoryRow struct {
	Status    string `json:"status"`
	ChangedAt string `json:"changed_at"`
}

// AdminHistoryRow — full history row untuk admin panel (from + to + who + note).
type AdminHistoryRow struct {
	ID         string  `json:"id"`
	FromStatus *string `json:"from_status,omitempty"`
	ToStatus   string  `json:"to_status"`
	ChangedBy  *string `json:"changed_by,omitempty"` // UUID string; frontend lookup ke StaffMap
	ChangedAt  string  `json:"changed_at"`
	Note       string  `json:"note,omitempty"`
}

// AdminOrderCustomer — customer info yang boleh diliat admin (full, tanpa
// sensor). Dipakai di /admin/orders/:resi/detail.
type AdminOrderCustomer struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Phone string `json:"phone"`
	Email string `json:"email,omitempty"`
	Type  string `json:"type,omitempty"` // "guest" | "registered"
}

// AdminOrderDetail — full detail untuk halaman /admin/order/:resi.
type AdminOrderDetail struct {
	// Order fields via handler.toOrderResponse (di-embed di handler DTO).
	// Di service kita cukup return order + customer + history; handler compose.
	CustomerInfo *AdminOrderCustomer `json:"customer,omitempty"`
	History      []AdminHistoryRow   `json:"history"`
}

// AdminListInput — filter/pagination for admin order list.
type AdminListInput struct {
	Status   string
	Channel  string
	Page     int
	PageSize int
}

// AdminListPage — paginated result plus meta for admin UI.
type AdminListPage struct {
	Items    []model.Order
	Total    int64
	Page     int
	PageSize int
}

// SetShippingCostInput — admin fills ongkir for a kirim order.
type SetShippingCostInput struct {
	Resi         string
	ShippingCost int64
	StaffID      uuid.UUID
	Note         string
}
