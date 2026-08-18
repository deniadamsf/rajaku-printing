package model

import (
	"time"

	"github.com/google/uuid"
)

type PricingType string

const (
	PricingTypePerM2 PricingType = "per_m2"
	PricingTypePaket PricingType = "paket"
)

type Product struct {
	ID            uuid.UUID   `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Slug          string      `gorm:"uniqueIndex;size:150;not null"                  json:"slug"`
	Name          string      `gorm:"size:200;not null"                              json:"name"`
	Description   *string     `                                                      json:"description,omitempty"`
	Category      string      `gorm:"size:50;not null;index"                         json:"category"`
	PricingType   PricingType `gorm:"size:20;not null"                               json:"pricing_type"`
	MinWidthCm    *int        `                                                      json:"min_width_cm,omitempty"`
	MinHeightCm   *int        `                                                      json:"min_height_cm,omitempty"`
	MaxWidthCm    *int        `                                                      json:"max_width_cm,omitempty"`
	MaxHeightCm   *int        `                                                      json:"max_height_cm,omitempty"`
	IsActive      bool        `gorm:"not null;default:true"                          json:"is_active"`
	DisplayOrder  int         `gorm:"not null;default:0"                             json:"display_order"`
	CreatedAt     time.Time   `gorm:"not null;default:now()"                         json:"created_at"`
	UpdatedAt     time.Time   `gorm:"not null;default:now()"                         json:"updated_at"`

	Pricings []ProductPricing `gorm:"foreignKey:ProductID" json:"pricings,omitempty"`
}

func (Product) TableName() string { return "products" }
