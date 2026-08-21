package service

import (
	"errors"
	"testing"

	"github.com/rajaku-printing/backend/internal/catalog/catalogapi"
	"github.com/rajaku-printing/backend/internal/catalog/model"
)

func intPtr(v int) *int         { return &v }
func int64Ptr(v int64) *int64   { return &v }
func f64Ptr(v float64) *float64 { return &v }
func strPtr(v string) *string   { return &v }

func TestCalcPerM2_ExactArea(t *testing.T) {
	row := model.ProductPricing{
		PricePerM2:  int64Ptr(25_000),
		MinChargeM2: f64Ptr(1.0),
	}
	// 100cm × 200cm = 2 m² → 2 × 25000 = 50000
	got := calcPerM2(row, 100, 200)
	if got.TotalPrice != 50_000 {
		t.Fatalf("total: got %d want 50000", got.TotalPrice)
	}
	if got.AreaM2 != 2.0 || got.ChargeableM2 != 2.0 {
		t.Fatalf("area/chargeable: got %v/%v want 2.0/2.0", got.AreaM2, got.ChargeableM2)
	}
}

func TestCalcPerM2_MinChargeApplied(t *testing.T) {
	row := model.ProductPricing{
		PricePerM2:  int64Ptr(30_000),
		MinChargeM2: f64Ptr(1.0),
	}
	// 40cm × 50cm = 0.2 m² < min 1.0 → charge 1.0 × 30000 = 30000
	got := calcPerM2(row, 40, 50)
	if got.ChargeableM2 != 1.0 {
		t.Fatalf("chargeable: got %v want 1.0", got.ChargeableM2)
	}
	if got.TotalPrice != 30_000 {
		t.Fatalf("total: got %d want 30000", got.TotalPrice)
	}
}

func TestCalcPerM2_RoundingHalfUp(t *testing.T) {
	row := model.ProductPricing{
		PricePerM2:  int64Ptr(25_000),
		MinChargeM2: f64Ptr(0.0),
	}
	// 33 × 33 = 0.1089 m² → 0.1089 × 25000 = 2722.5 → round to 2723
	got := calcPerM2(row, 33, 33)
	if got.TotalPrice != 2723 {
		t.Fatalf("total: got %d want 2723", got.TotalPrice)
	}
}

func TestCalcPerM2_NoMinCharge(t *testing.T) {
	row := model.ProductPricing{
		PricePerM2:  int64Ptr(25_000),
		MinChargeM2: nil,
	}
	// 50 × 50 = 0.25 m² → 6250 (no min charge)
	got := calcPerM2(row, 50, 50)
	if got.TotalPrice != 6250 {
		t.Fatalf("total: got %d want 6250", got.TotalPrice)
	}
}

func TestCalcPaket_FixedPrice(t *testing.T) {
	row := model.ProductPricing{
		WidthCm:      intPtr(60),
		HeightCm:     intPtr(160),
		PriceTotal:   int64Ptr(85_000),
		PackageLabel: strPtr("Standard 60x160"),
	}
	got := calcPaket(row)
	if got.TotalPrice != 85_000 {
		t.Fatalf("total: got %d want 85000", got.TotalPrice)
	}
	if got.PackageLabel != "Standard 60x160" {
		t.Fatalf("label: got %q want %q", got.PackageLabel, "Standard 60x160")
	}
}

func TestValidateDimensions(t *testing.T) {
	p := model.Product{
		MinWidthCm:  intPtr(30),
		MinHeightCm: intPtr(30),
		MaxWidthCm:  intPtr(300),
		MaxHeightCm: intPtr(1000),
	}
	cases := []struct {
		name string
		w, h int
		want error
	}{
		{"in range", 100, 200, nil},
		{"width too small", 20, 100, catalogapi.ErrDimensionsOutOfRange},
		{"width too big", 500, 100, catalogapi.ErrDimensionsOutOfRange},
		{"height too small", 100, 10, catalogapi.ErrDimensionsOutOfRange},
		{"height too big", 100, 2000, catalogapi.ErrDimensionsOutOfRange},
		{"negative width", -1, 100, catalogapi.ErrDimensionsInvalid},
		{"zero height", 100, 0, catalogapi.ErrDimensionsInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateDimensions(p, tc.w, tc.h)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
		})
	}
}

func TestValidateDimensions_UnboundedProduct(t *testing.T) {
	p := model.Product{} // semua min/max nil (no constraint)
	if err := validateDimensions(p, 5000, 5000); err != nil {
		t.Fatalf("expected no error for unbounded product, got %v", err)
	}
}
