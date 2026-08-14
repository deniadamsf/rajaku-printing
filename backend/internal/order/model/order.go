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
)

type DesignApprovalMode string

const (
	DesignApprovalInstantWalkin DesignApprovalMode = "instant_walkin"
	DesignApprovalAsyncNotify   DesignApprovalMode = "async_notify"
)

type Order struct {
	ID          uuid.UUID    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Resi        string       `gorm:"uniqueIndex;size:30;not null"                   json:"resi"`
	CustomerID  uuid.UUID    `gorm:"type:uuid;not null;index"                       json:"customer_id"`
	Channel     Channel      `gorm:"size:20;not null;index"                         json:"channel"`
	Status      state.Status `gorm:"size:50;not null;index"                         json:"status"`

	// Product snapshot
	ProductID              *uuid.UUID  `gorm:"type:uuid"                             json:"product_id,omitempty"`
	ProductNameSnapshot    string      `gorm:"size:255;not null;column:product_name_snapshot"     json:"product_name_snapshot"`
	MaterialID             *uuid.UUID  `gorm:"type:uuid"                             json:"material_id,omitempty"`
	MaterialNameSnapshot   string      `gorm:"size:255;not null;column:material_name_snapshot"    json:"material_name_snapshot"`
	PricingTypeSnapshot    string      `gorm:"size:20;not null;column:pricing_type_snapshot"      json:"pricing_type_snapshot"`
	WidthCm                int         `gorm:"not null"                              json:"width_cm"`
	HeightCm               int         `gorm:"not null"                              json:"height_cm"`
	Quantity               int         `gorm:"not null;default:1"                    json:"quantity"`
	UnitPrice              int64       `gorm:"not null"                              json:"unit_price"`
	Subtotal               int64       `gorm:"not null"                              json:"subtotal"`

	// Fulfillment
	MetodeAmbil            MetodeAmbil `gorm:"size:20;not null"                      json:"metode_ambil"`
	ShippingCost           *int64      `                                             json:"shipping_cost,omitempty"`
	ShippingAddress        *string     `                                             json:"shipping_address,omitempty"`
	ShippingRecipientName  *string     `gorm:"size:255"                              json:"shipping_recipient_name,omitempty"`
	ShippingRecipientPhone *string     `gorm:"size:20"                               json:"shipping_recipient_phone,omitempty"`
	ShippingCourier        *string     `gorm:"size:100"                              json:"shipping_courier,omitempty"`
	ShippingTrackingNumber *string     `gorm:"size:100"                              json:"shipping_tracking_number,omitempty"`

	// Payment
	MetodeBayar            *MetodeBayar `gorm:"size:20"                              json:"metode_bayar,omitempty"`

	// Design flow
	DesignSource           DesignSource        `gorm:"size:20;not null"              json:"design_source"`
	DesignApprovalMode     *DesignApprovalMode `gorm:"size:20"                       json:"design_approval_mode,omitempty"`
	DesignBrief            *string             `                                     json:"design_brief,omitempty"`

	// Total
	Total                  int64      `gorm:"not null"                               json:"total"`

	// Meta
	Notes                  *string    `                                              json:"notes,omitempty"`
	CreatedBy              *uuid.UUID `gorm:"type:uuid"                              json:"created_by,omitempty"`
	CreatedAt              time.Time  `gorm:"not null;default:now()"                 json:"created_at"`
	UpdatedAt              time.Time  `gorm:"not null;default:now()"                 json:"updated_at"`
}

func (Order) TableName() string { return "orders" }
