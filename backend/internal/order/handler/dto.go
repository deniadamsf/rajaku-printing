// Package handler contains HTTP layer for order module.
package handler

import (
	"time"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/order/model"
)

// ---- Request ----

type createOrderRequest struct {
	// Guest fields — required kalau caller belum login.
	GuestPhone string `json:"guest_phone" binding:"omitempty,min=8,max=20"`
	GuestName  string `json:"guest_name"  binding:"omitempty,min=1,max=255"`

	// Product spec:
	ProductID  uuid.UUID `json:"product_id"  binding:"required"`
	MaterialID uuid.UUID `json:"material_id" binding:"required"`
	WidthCm    int       `json:"width_cm"    binding:"required,gt=0"`
	HeightCm   int       `json:"height_cm"   binding:"required,gt=0"`
	Quantity   int       `json:"quantity"    binding:"omitempty,gt=0"`

	// Fulfillment:
	MetodeAmbil            string `json:"metode_ambil"              binding:"required,oneof=pickup kirim"`
	ShippingAddress        string `json:"shipping_address"          binding:"omitempty,max=1000"`
	ShippingRecipientName  string `json:"shipping_recipient_name"   binding:"omitempty,max=255"`
	ShippingRecipientPhone string `json:"shipping_recipient_phone"  binding:"omitempty,min=8,max=20"`

	// Design flow:
	DesignSource string `json:"design_source" binding:"required,oneof=upload request"`
	DesignBrief  string `json:"design_brief"  binding:"omitempty,max=2000"`

	// Meta:
	Notes string `json:"notes" binding:"omitempty,max=1000"`
}

// ---- Admin request DTOs ----

type setShippingCostRequest struct {
	ShippingCost int64  `json:"shipping_cost" binding:"required,gte=0"`
	Note         string `json:"note"          binding:"omitempty,max=500"`
}

type confirmPickupRequest struct {
	Note string `json:"note" binding:"omitempty,max=500"`
}

type cancelOrderRequest struct {
	Reason string `json:"reason" binding:"required,min=3,max=500"`
}

// ---- Response ----

type orderResponse struct {
	ID                     uuid.UUID `json:"id"`
	Resi                   string    `json:"resi"`
	Status                 string    `json:"status"`
	Channel                string    `json:"channel"`
	ProductName            string    `json:"product_name"`
	MaterialName           string    `json:"material_name"`
	PricingType            string    `json:"pricing_type"`
	WidthCm                int       `json:"width_cm"`
	HeightCm               int       `json:"height_cm"`
	Quantity               int       `json:"quantity"`
	UnitPrice              int64     `json:"unit_price"`
	Subtotal               int64     `json:"subtotal"`
	MetodeAmbil            string    `json:"metode_ambil"`
	ShippingCost           *int64    `json:"shipping_cost,omitempty"`
	ShippingAddress        string    `json:"shipping_address,omitempty"`
	ShippingRecipientName  string    `json:"shipping_recipient_name,omitempty"`
	ShippingRecipientPhone string    `json:"shipping_recipient_phone,omitempty"`
	MetodeBayar            string    `json:"metode_bayar,omitempty"`
	DesignSource           string    `json:"design_source"`
	DesignBrief            string    `json:"design_brief,omitempty"`
	// Discount (§28) — DiscountAmount 0 (default) berarti order ini tidak
	// pakai diskon; field snapshot lain dibiarkan omitempty dalam kondisi itu.
	DiscountID            *uuid.UUID `json:"discount_id,omitempty"`
	DiscountCodeSnapshot  string     `json:"discount_code_snapshot,omitempty"`
	DiscountNameSnapshot  string     `json:"discount_name_snapshot,omitempty"`
	DiscountTypeSnapshot  string     `json:"discount_type_snapshot,omitempty"`
	DiscountValueSnapshot *float64   `json:"discount_value_snapshot,omitempty"`
	DiscountAmount        int64      `json:"discount_amount"`
	DiscountNote          string     `json:"discount_note,omitempty"`
	Total                 int64      `json:"total"`
	Notes                 string     `json:"notes,omitempty"`
	CreatedAt             string     `json:"created_at"`
}

func toOrderResponse(o *model.Order) orderResponse {
	r := orderResponse{
		ID:                    o.ID,
		Resi:                  o.Resi,
		Status:                string(o.Status),
		Channel:               string(o.Channel),
		ProductName:           o.ProductNameSnapshot,
		MaterialName:          o.MaterialNameSnapshot,
		PricingType:           o.PricingTypeSnapshot,
		WidthCm:               o.WidthCm,
		HeightCm:              o.HeightCm,
		Quantity:              o.Quantity,
		UnitPrice:             o.UnitPrice,
		Subtotal:              o.Subtotal,
		MetodeAmbil:           string(o.MetodeAmbil),
		ShippingCost:          o.ShippingCost,
		DiscountID:            o.DiscountID,
		DiscountValueSnapshot: o.DiscountValueSnapshot,
		DiscountAmount:        o.DiscountAmount,
		Total:                 o.Total,
		DesignSource:          string(o.DesignSource),
		CreatedAt:             o.CreatedAt.UTC().Format(time.RFC3339),
	}
	if o.DiscountCodeSnapshot != nil {
		r.DiscountCodeSnapshot = *o.DiscountCodeSnapshot
	}
	if o.DiscountNameSnapshot != nil {
		r.DiscountNameSnapshot = *o.DiscountNameSnapshot
	}
	if o.DiscountTypeSnapshot != nil {
		r.DiscountTypeSnapshot = *o.DiscountTypeSnapshot
	}
	if o.DiscountNote != nil {
		r.DiscountNote = *o.DiscountNote
	}
	if o.ShippingAddress != nil {
		r.ShippingAddress = *o.ShippingAddress
	}
	if o.ShippingRecipientName != nil {
		r.ShippingRecipientName = *o.ShippingRecipientName
	}
	if o.ShippingRecipientPhone != nil {
		r.ShippingRecipientPhone = *o.ShippingRecipientPhone
	}
	if o.MetodeBayar != nil {
		r.MetodeBayar = string(*o.MetodeBayar)
	}
	if o.DesignBrief != nil {
		r.DesignBrief = *o.DesignBrief
	}
	if o.Notes != nil {
		r.Notes = *o.Notes
	}
	return r
}
