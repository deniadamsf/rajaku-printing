// Package service holds order business logic.
package service

import (
	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/order/model"
)

// CreateOnlineOrderItemInput — satu baris produk untuk CreateOnlineOrderInput
// (§32). DesignSource/DesignBrief sekarang PER ITEM (§32.5).
type CreateOnlineOrderItemInput struct {
	ProductID  uuid.UUID
	MaterialID uuid.UUID
	WidthCm    int
	HeightCm   int
	Quantity   int

	DesignSource model.DesignSource
	DesignBrief  string // untuk request
}

// CreateOnlineOrderInput is what handler passes to service.
// Salah satu dari CustomerID (kalau caller sudah login) ATAU (GuestPhone + GuestName)
// wajib terisi — service akan resolve identity dari mana pun yg tersedia.
type CreateOnlineOrderInput struct {
	// Identity — mutually exclusive:
	CustomerID *uuid.UUID // set dari authapi.Identity kalau user login
	GuestPhone string     // raw, di-normalize di service
	GuestName  string

	// Items — 1..20 baris produk (§32.4, ErrNoItems/ErrTooManyItems).
	Items []CreateOnlineOrderItemInput

	// Fulfillment:
	MetodeAmbil            model.MetodeAmbil
	ShippingAddress        string // wajib kalau kirim
	ShippingRecipientName  string // wajib kalau kirim
	ShippingRecipientPhone string // wajib kalau kirim (di-normalize)

	// Meta:
	Notes string
}

// PublicTrackingResultItem — satu baris produk (§32) yang boleh dilihat
// pelanggan yang melacak resinya (endpoint TANPA login, §5). Sengaja
// TIDAK memuat unit_price/subtotal/discount_amount apa pun — endpoint ini
// tidak pernah mengekspos uang, dan itu tetap harus benar sekarang order
// boleh multi-item (sebelumnya hanya PublicTrackingResult.ProductName/
// MaterialName datar, yang membuat pesanan 2 banner terlihat seperti 1
// banner bagi pelanggan yang melacaknya).
type PublicTrackingResultItem struct {
	// ID + DesignSource ada di sini SEMATA supaya tamu terverifikasi (token
	// scope=guest_order dari POST /lacak/:resi/verify) bisa mengunggah
	// desain untuk baris yang BENAR — POST /orders/:resi/design-files
	// mewajibkan order_item_id sejak §32.5, sementara GET /orders/:resi
	// memakai RequireAuth penuh sehingga token tamu tidak bisa mengambil
	// detail order untuk mencari id itemnya. Tanpa kedua field ini, satu-
	// satunya jalan adalah mematikan unggah mandiri tamu — kemampuan yang
	// disengaja ada (lihat doc RegisterRoutes di modul design).
	//
	// Aman diekspos tanpa login: id hanya berguna lewat endpoint unggah yang
	// TETAP memeriksa kepemilikan (id.UserID == order.CustomerID), dan
	// pemanggil sudah harus tahu resinya untuk sampai ke sini. Tidak ada
	// nominal uang yang ikut — batas §5 tidak bergeser.
	ID           string `json:"id"`
	ProductName  string `json:"product_name"`
	MaterialName string `json:"material_name"`
	WidthCm      int    `json:"width_cm"`
	HeightCm     int    `json:"height_cm"`
	Quantity     int    `json:"quantity"`
	DesignSource string `json:"design_source"`
}

// PublicTrackingResult — data yg boleh dilihat siapa saja (sensor field
// sensitif per spec section 5).
type PublicTrackingResult struct {
	Resi        string `json:"resi"`
	Status      string `json:"status"`
	Channel     string `json:"channel"`
	MetodeAmbil string `json:"metode_ambil"`
	// DesignSource dibuka di payload publik karena halaman lacak perlu tahu
	// pelanggan membawa desain sendiri ('upload') atau minta dibuatkan
	// ('request') untuk menentukan aksi unggah mana yang boleh ditawarkan ke
	// guest terverifikasi. Bukan data sensitif: tidak memuat identitas,
	// alamat, maupun nominal.
	DesignSource string `json:"design_source"`
	// Items — daftar SEMUA baris produk order ini (§32), terurut line_no ASC
	// — GANTI ProductName/MaterialName datar yang lama (yang cuma menampilkan
	// baris pertama). Lihat PublicTrackingResultItem doc soal batas apa yang
	// boleh diekspos di sini.
	Items                 []PublicTrackingResultItem       `json:"items"`
	ShippingRecipient     string                           `json:"shipping_recipient,omitempty"` // "Ani T***" (masked)
	ShippingPhoneMasked   string                           `json:"shipping_phone,omitempty"`     // "0812****678"
	ShippingAddressMasked string                           `json:"shipping_address,omitempty"`   // "Jl. Merdek**"
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
