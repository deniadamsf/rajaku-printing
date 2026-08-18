package model

import (
	"time"

	"github.com/google/uuid"
)

// ProductPricing menyimpan aturan harga per (product, material). Field yang
// dipakai bergantung pada Product.PricingType parent (validasi konsistensi di
// service layer; CHECK constraint di DB enforce set kolom eksklusif).
type ProductPricing struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ProductID   uuid.UUID `gorm:"type:uuid;not null;index"                       json:"product_id"`
	MaterialID  uuid.UUID `gorm:"type:uuid;not null;index"                       json:"material_id"`

	// per_m2 fields
	PricePerM2   *int64   `                                                       json:"price_per_m2,omitempty"`
	MinChargeM2  *float64 `gorm:"type:numeric(10,4)"                              json:"min_charge_m2,omitempty"`

	// paket fields
	WidthCm      *int    `                                                        json:"width_cm,omitempty"`
	HeightCm     *int    `                                                        json:"height_cm,omitempty"`
	PackageLabel *string `gorm:"size:100"                                         json:"package_label,omitempty"`
	PriceTotal   *int64  `                                                        json:"price_total,omitempty"`

	IsActive     bool      `gorm:"not null;default:true"                          json:"is_active"`
	CreatedAt    time.Time `gorm:"not null;default:now()"                         json:"created_at"`
	UpdatedAt    time.Time `gorm:"not null;default:now()"                         json:"updated_at"`

	Material *Material `gorm:"foreignKey:MaterialID" json:"material,omitempty"`
}

func (ProductPricing) TableName() string { return "product_pricings" }

// IsPerM2Row returns true if this pricing row belongs to a per_m2 product.
func (p ProductPricing) IsPerM2Row() bool { return p.PricePerM2 != nil }

// IsPaketRow returns true if this pricing row belongs to a paket product.
func (p ProductPricing) IsPaketRow() bool { return p.PriceTotal != nil && p.WidthCm != nil }
