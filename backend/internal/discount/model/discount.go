// Package model contains GORM entities for the discount module (§28).
package model

import (
	"time"

	"github.com/google/uuid"
)

// DiscountType — tipe kalkulasi diskon (§28.1).
type DiscountType string

const (
	DiscountTypePercent DiscountType = "percent"
	DiscountTypeNominal DiscountType = "nominal"
	// DiscountTypeManual — dipakai HANYA di kolom snapshot orders
	// (discount_type_snapshot), tidak pernah tersimpan sebagai baris
	// discounts.type (diskon manual tidak punya master row, discount_id nil).
	DiscountTypeManual DiscountType = "manual"
)

// ChannelScope — batas channel order tempat diskon boleh dipakai (§28.1).
type ChannelScope string

const (
	ChannelScopeAll    ChannelScope = "all"
	ChannelScopeOnline ChannelScope = "online"
	ChannelScopePOS    ChannelScope = "pos"
)

// Discount — satu baris = satu program diskon (§28.1). TIDAK PERNAH
// di-hard-delete — soft delete via DeletedAt/DeletedBy/DeleteReason, pola
// sama seperti order.model.Order (migration 000025).
type Discount struct {
	ID                uuid.UUID    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Code              string       `gorm:"size:30;not null;column:code"                    json:"code"`
	Name              string       `gorm:"size:150;not null;column:name"                   json:"name"`
	Type              DiscountType `gorm:"size:20;not null;column:type"                    json:"type"`
	ValuePercent      *float64     `gorm:"column:value_percent"                            json:"value_percent"`
	ValueAmount       *int64       `gorm:"column:value_amount"                             json:"value_amount"`
	MaxDiscountAmount *int64       `gorm:"column:max_discount_amount"                      json:"max_discount_amount"`
	MinSubtotal       int64        `gorm:"not null;default:0;column:min_subtotal"          json:"min_subtotal"`
	StartsAt          *time.Time   `gorm:"column:starts_at"                                json:"starts_at"`
	EndsAt            *time.Time   `gorm:"column:ends_at"                                  json:"ends_at"`
	Quota             *int         `gorm:"column:quota"                                    json:"quota"`
	ChannelScope      ChannelScope `gorm:"size:20;not null;default:all;column:channel_scope" json:"channel_scope"`
	IsActive          bool         `gorm:"not null;default:true;column:is_active"          json:"is_active"`

	DeletedAt    *time.Time `gorm:"column:deleted_at"    json:"deleted_at,omitempty"`
	DeletedBy    *uuid.UUID `gorm:"type:uuid;column:deleted_by" json:"deleted_by,omitempty"`
	DeleteReason *string    `gorm:"column:delete_reason" json:"delete_reason,omitempty"`

	CreatedAt time.Time `gorm:"not null;default:now();column:created_at" json:"created_at"`
	UpdatedAt time.Time `gorm:"not null;default:now();column:updated_at" json:"updated_at"`
}

func (Discount) TableName() string { return "discounts" }
