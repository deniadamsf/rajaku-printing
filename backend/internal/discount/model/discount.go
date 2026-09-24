// Package model contains GORM entities for the discount module (§28).
package model

import (
	"time"

	"github.com/google/uuid"
)

// DiscountType — tipe kalkulasi diskon (§28.1).
type DiscountType string

const (
	DiscountTypePercent      DiscountType = "percent"
	DiscountTypeNominal      DiscountType = "nominal"
	DiscountTypeNominalPerM2 DiscountType = "nominal_per_m2"
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

// AppliesToScope — cakupan produk sebuah diskon (§28.9). Default 'all' wajib
// membuat diskon lama (dibuat sebelum kolom ini ada) tetap berperilaku
// persis seperti sebelumnya. 'selected' berarti diskon HANYA boleh dipakai
// untuk produk yang terdaftar di tabel discount_products — dan daftar
// kosong TIDAK PERNAH berarti "berlaku untuk semua" (lihat discountapi
// ErrDiscountScopeEmpty).
type AppliesToScope string

const (
	AppliesToAll      AppliesToScope = "all"
	AppliesToSelected AppliesToScope = "selected"
)

// AudienceScope — batas audiens sebuah diskon (§30.3): apakah diskon berlaku
// untuk semua orang, atau khusus member. Independen dari AppliesToScope
// (cakupan produk) — sebuah diskon boleh sekaligus "khusus member" DAN
// "khusus produk tertentu". Default 'all' wajib membuat diskon lama (dibuat
// sebelum kolom ini ada) tetap berperilaku persis seperti sebelumnya.
type AudienceScope string

const (
	AudienceScopeAll    AudienceScope = "all"
	AudienceScopeMember AudienceScope = "member"
)

// MemberScope — hanya relevan kalau AudienceScope == AudienceScopeMember.
// BEDA dengan AppliesToScope: ini enum eksplisit, bukan disimpulkan dari isi
// discount_customers kosong-atau-tidak (§30.3) — 'all_members' berarti
// berlaku untuk SEMUA member aktif (discount_customers tidak dipakai sama
// sekali, boleh kosong, itu normal); 'selected_members' berarti berlaku
// HANYA untuk customer yang terdaftar di discount_customers, dan daftar
// kosong TIDAK PERNAH berarti "berlaku untuk semua" (lihat discountapi
// ErrDiscountMemberScopeEmpty).
type MemberScope string

const (
	MemberScopeAllMembers MemberScope = "all_members"
	MemberScopeSelected   MemberScope = "selected_members"
)

// Discount — satu baris = satu program diskon (§28.1). TIDAK PERNAH
// di-hard-delete — soft delete via DeletedAt/DeletedBy/DeleteReason, pola
// sama seperti order.model.Order (migration 000025).
type Discount struct {
	ID                uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Code              string         `gorm:"size:30;not null;column:code"                    json:"code"`
	Name              string         `gorm:"size:150;not null;column:name"                   json:"name"`
	Type              DiscountType   `gorm:"size:20;not null;column:type"                    json:"type"`
	ValuePercent      *float64       `gorm:"column:value_percent"                            json:"value_percent"`
	ValueAmount       *int64         `gorm:"column:value_amount"                             json:"value_amount"`
	MaxDiscountAmount *int64         `gorm:"column:max_discount_amount"                      json:"max_discount_amount"`
	MinSubtotal       int64          `gorm:"not null;default:0;column:min_subtotal"          json:"min_subtotal"`
	StartsAt          *time.Time     `gorm:"column:starts_at"                                json:"starts_at"`
	EndsAt            *time.Time     `gorm:"column:ends_at"                                  json:"ends_at"`
	Quota             *int           `gorm:"column:quota"                                    json:"quota"`
	ChannelScope      ChannelScope   `gorm:"size:20;not null;default:all;column:channel_scope" json:"channel_scope"`
	AppliesTo         AppliesToScope `gorm:"size:20;not null;default:all;column:applies_to"  json:"applies_to"`
	AudienceScope     AudienceScope  `gorm:"size:20;not null;default:all;column:audience_scope" json:"audience_scope"`
	MemberScope       *MemberScope   `gorm:"size:20;column:member_scope"                     json:"member_scope"`
	IsActive          bool           `gorm:"not null;default:true;column:is_active"          json:"is_active"`

	DeletedAt    *time.Time `gorm:"column:deleted_at"    json:"deleted_at,omitempty"`
	DeletedBy    *uuid.UUID `gorm:"type:uuid;column:deleted_by" json:"deleted_by,omitempty"`
	DeleteReason *string    `gorm:"column:delete_reason" json:"delete_reason,omitempty"`

	CreatedAt time.Time `gorm:"not null;default:now();column:created_at" json:"created_at"`
	UpdatedAt time.Time `gorm:"not null;default:now();column:updated_at" json:"updated_at"`
}

func (Discount) TableName() string { return "discounts" }
