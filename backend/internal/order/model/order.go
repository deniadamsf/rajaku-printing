// Package model contains GORM entities for order module.
package model

import (
	"time"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/order/state"
)

type Channel string

const (
	ChannelOnline Channel = "online"
	ChannelPOS    Channel = "pos"
)

type MetodeAmbil string

const (
	MetodeAmbilPickup MetodeAmbil = "pickup"
	MetodeAmbilKirim  MetodeAmbil = "kirim"
)

type MetodeBayar string

const (
	MetodeBayarTransfer MetodeBayar = "transfer"
	MetodeBayarQRIS     MetodeBayar = "qris"
	MetodeBayarCash     MetodeBayar = "cash"
	MetodeBayarQRISPOS  MetodeBayar = "qris_pos"
)

type DesignSource string

const (
	DesignSourceUpload  DesignSource = "upload"
	DesignSourceRequest DesignSource = "request"
	// DesignSourceMixed — nilai TURUNAN untuk orders.DesignSource saat item
	// di dalamnya campur upload & request (§32.1). TIDAK PERNAH valid untuk
	// order_items.DesignSource (per item hanya upload|request, §32.5) —
	// dipakai HANYA di level order, ditulis oleh deriveDesignSource.
	DesignSourceMixed DesignSource = "mixed"
)

type DesignApprovalMode string

const (
	DesignApprovalInstantWalkin DesignApprovalMode = "instant_walkin"
	DesignApprovalAsyncNotify   DesignApprovalMode = "async_notify"
)

type Order struct {
	ID         uuid.UUID    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Resi       string       `gorm:"uniqueIndex;size:30;not null"                   json:"resi"`
	CustomerID uuid.UUID    `gorm:"type:uuid;not null;index"                       json:"customer_id"`
	Channel    Channel      `gorm:"size:20;not null;index"                         json:"channel"`
	Status     state.Status `gorm:"size:50;not null;index"                         json:"status"`

	// Items — daftar produk dalam order ini (§32), preload terurut
	// `line_no ASC` oleh repository. Subtotal di bawah adalah Σ item.subtotal
	// (§32.2) — SATU-SATUNYA angka yang dibaca perhitungan uang (rekap,
	// invoice, struk); jangan pernah menjumlahkan Items ulang di layar uang.
	Items []OrderItem `gorm:"foreignKey:OrderID" json:"items,omitempty"`

	Subtotal int64 `gorm:"not null" json:"subtotal"`

	// Fulfillment
	MetodeAmbil            MetodeAmbil `gorm:"size:20;not null"                      json:"metode_ambil"`
	ShippingCost           *int64      `                                             json:"shipping_cost,omitempty"`
	ShippingAddress        *string     `                                             json:"shipping_address,omitempty"`
	ShippingRecipientName  *string     `gorm:"size:255"                              json:"shipping_recipient_name,omitempty"`
	ShippingRecipientPhone *string     `gorm:"size:20"                               json:"shipping_recipient_phone,omitempty"`
	ShippingCourier        *string     `gorm:"size:100"                              json:"shipping_courier,omitempty"`
	ShippingTrackingNumber *string     `gorm:"size:100"                              json:"shipping_tracking_number,omitempty"`

	// Payment
	MetodeBayar *MetodeBayar `gorm:"size:20"                              json:"metode_bayar,omitempty"`

	// Design flow — DesignSource di sini adalah TURUNAN (§32.1): upload |
	// request | mixed, dihitung oleh deriveDesignSource dari
	// order_items.DesignSource setiap kali daftar item berubah. Hanya untuk
	// ringkasan/pemilihan alur tampilan — JANGAN dipakai memvalidasi file
	// desain, itu dibaca dari OrderItem.DesignSource milik baris masing-
	// masing (§32.5). DesignBrief per item sekarang ada di OrderItem.
	DesignSource       DesignSource        `gorm:"size:20;not null"              json:"design_source"`
	DesignApprovalMode *DesignApprovalMode `gorm:"size:20"                       json:"design_approval_mode,omitempty"`

	// Discount snapshot (§28) — nilai diskon disalin ke order SAAT DIBUAT,
	// bukan cuma foreign key, supaya rekap/invoice/struk tetap benar walau
	// master diskon diubah/dihapus nanti. DiscountAmount adalah SATU-SATUNYA
	// kolom yang dipakai perhitungan uang di mana pun (rekap, invoice,
	// struk) — jangan pernah JOIN ke discounts untuk angka uang (§28.2 lapis
	// 3). Lihat migration 000027 & discountapi.Snapshot.
	DiscountID            *uuid.UUID `gorm:"type:uuid;column:discount_id"                      json:"discount_id,omitempty"`
	DiscountCodeSnapshot  *string    `gorm:"size:30;column:discount_code_snapshot"              json:"discount_code_snapshot,omitempty"`
	DiscountNameSnapshot  *string    `gorm:"size:150;column:discount_name_snapshot"             json:"discount_name_snapshot,omitempty"`
	DiscountTypeSnapshot  *string    `gorm:"size:20;column:discount_type_snapshot"              json:"discount_type_snapshot,omitempty"`
	DiscountValueSnapshot *float64   `gorm:"column:discount_value_snapshot"                     json:"discount_value_snapshot,omitempty"`
	DiscountAmount        int64      `gorm:"not null;default:0;column:discount_amount"          json:"discount_amount"`
	DiscountNote          *string    `gorm:"column:discount_note"                               json:"discount_note,omitempty"`

	// Total
	Total int64 `gorm:"not null"                               json:"total"`

	// Meta
	Notes     *string    `                                              json:"notes,omitempty"`
	CreatedBy *uuid.UUID `gorm:"type:uuid"                              json:"created_by,omitempty"`
	CreatedAt time.Time  `gorm:"not null;default:now()"                 json:"created_at"`
	UpdatedAt time.Time  `gorm:"not null;default:now()"                 json:"updated_at"`

	// Soft delete (§ super admin order tools, migration 000025). NEVER hard
	// delete an order — every reading query in repository.go MUST filter
	// `deleted_at IS NULL` so a "deleted" order stops showing up anywhere
	// (admin list, resi lookup, customer history, POS rekap, public tracking)
	// while the row + everything referencing it (payment proofs, design
	// files, invoices, state history) stays intact for audit/rekap.
	//
	// PRECISELY BECAUSE deletion makes an order invisible to POS
	// rekonsiliasi (ListPOSByDateRange also filters deleted_at IS NULL),
	// an order that's already `dibayar` or later CANNOT be soft-deleted
	// directly — state.IsDeletable(Status) gates this in both
	// service.SoftDeleteOrder (early check) and
	// repository.OrderRepository.SoftDelete (authoritative, row-locked
	// re-check). A paid order must be cancelled (→ Dibatalkan) FIRST; only
	// pre-payment statuses and Dibatalkan itself may be deleted, so a
	// deleted order was, by construction, never counted as revenue.
	DeletedAt    *time.Time `gorm:"column:deleted_at"                     json:"deleted_at,omitempty"`
	DeletedBy    *uuid.UUID `gorm:"type:uuid;column:deleted_by"           json:"deleted_by,omitempty"`
	DeleteReason *string    `gorm:"column:delete_reason"                  json:"delete_reason,omitempty"`
}

func (Order) TableName() string { return "orders" }
