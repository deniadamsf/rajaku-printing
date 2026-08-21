// Package service holds the catalog business logic. The price calculator here
// is deliberately kept as pure functions (no DB access) so it's easy to unit
// test in isolation — repository calls happen in the outer catalog_service.
package service

import (
	"math"

	"github.com/rajaku-printing/backend/internal/catalog/catalogapi"
	"github.com/rajaku-printing/backend/internal/catalog/model"
)

// calcPerM2 computes total price for a per_m2 pricing row.
//
// Formula: chargeable_m2 = max(width_m × height_m, min_charge_m2)
//
//	total         = round(chargeable_m2 × price_per_m2)
//
// Rounding: HALF-UP ke rupiah terdekat (praktik toko cetak umum — customer
// jarang dihadapkan angka desimal).
func calcPerM2(row model.ProductPricing, widthCm, heightCm int) catalogapi.QuoteResult {
	widthM := float64(widthCm) / 100.0
	heightM := float64(heightCm) / 100.0
	area := widthM * heightM

	minCharge := 0.0
	if row.MinChargeM2 != nil {
		minCharge = *row.MinChargeM2
	}
	chargeable := area
	if chargeable < minCharge {
		chargeable = minCharge
	}

	pricePerM2 := *row.PricePerM2
	total := int64(math.Round(chargeable * float64(pricePerM2)))

	return catalogapi.QuoteResult{
		WidthCm:      widthCm,
		HeightCm:     heightCm,
		TotalPrice:   total,
		AreaM2:       roundTo4(area),
		ChargeableM2: roundTo4(chargeable),
		PricePerM2:   pricePerM2,
		PricingType:  catalogapi.PricingTypePerM2,
	}
}

// calcPaket returns the fixed price from a paket pricing row.
func calcPaket(row model.ProductPricing) catalogapi.QuoteResult {
	label := ""
	if row.PackageLabel != nil {
		label = *row.PackageLabel
	}
	return catalogapi.QuoteResult{
		WidthCm:      *row.WidthCm,
		HeightCm:     *row.HeightCm,
		TotalPrice:   *row.PriceTotal,
		PackageLabel: label,
		PricingType:  catalogapi.PricingTypePaket,
	}
}

// validateDimensions returns ErrDimensionsInvalid on non-positive input and
// ErrDimensionsOutOfRange when outside the product's min/max window.
func validateDimensions(p model.Product, widthCm, heightCm int) error {
	if widthCm <= 0 || heightCm <= 0 {
		return catalogapi.ErrDimensionsInvalid
	}
	if p.MinWidthCm != nil && widthCm < *p.MinWidthCm {
		return catalogapi.ErrDimensionsOutOfRange
	}
	if p.MaxWidthCm != nil && widthCm > *p.MaxWidthCm {
		return catalogapi.ErrDimensionsOutOfRange
	}
	if p.MinHeightCm != nil && heightCm < *p.MinHeightCm {
		return catalogapi.ErrDimensionsOutOfRange
	}
	if p.MaxHeightCm != nil && heightCm > *p.MaxHeightCm {
		return catalogapi.ErrDimensionsOutOfRange
	}
	return nil
}

func roundTo4(f float64) float64 {
	return math.Round(f*10000) / 10000
}
