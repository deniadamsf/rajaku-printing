// Package catalogapi is the PUBLIC contract of the catalog module — the ONLY
// package that other modules (order, POS, ...) are allowed to import.
//
// Per spec section 22: komunikasi antar modul via interface, bukan import
// package internal.
package catalogapi

import (
	"context"

	"github.com/google/uuid"
)

type PricingType string

const (
	PricingTypePerM2 PricingType = "per_m2"
	PricingTypePaket PricingType = "paket"
)

// ProductSummary — proyeksi minimal untuk konsumen eksternal (order/POS).
type ProductSummary struct {
	ID            uuid.UUID
	Slug          string
	Name          string
	Category      string
	PricingType   PricingType
	MinWidthCm    *int
	MinHeightCm   *int
	MaxWidthCm    *int
	MaxHeightCm   *int
	IsActive      bool
}

type MaterialSummary struct {
	ID       uuid.UUID
	Code     string
	Name     string
	IsActive bool
}

// QuoteRequest — input untuk kalkulasi harga.
type QuoteRequest struct {
	ProductID  uuid.UUID
	MaterialID uuid.UUID
	// Dimensi dalam cm. Untuk pricing_type='paket' dipakai untuk lookup paket
	// yang persis cocok. Untuk pricing_type='per_m2' dipakai untuk hitung area.
	WidthCm  int
	HeightCm int
}

// QuoteResult — hasil kalkulasi harga. Field yg diisi tergantung PricingType.
type QuoteResult struct {
	ProductID    uuid.UUID
	ProductName  string
	MaterialID   uuid.UUID
	MaterialName string
	PricingType  PricingType

	// Common:
	WidthCm    int
	HeightCm   int
	TotalPrice int64 // IDR

	// per_m2 breakdown:
	AreaM2       float64
	ChargeableM2 float64
	PricePerM2   int64

	// paket breakdown:
	PackageLabel string
}

// CatalogService — kontrak untuk konsumen eksternal.
type CatalogService interface {
	// ResolveProductBySlug returns product summary or ErrProductNotFound.
	ResolveProductBySlug(ctx context.Context, slug string) (*ProductSummary, error)

	// ResolveProductByID returns product summary or ErrProductNotFound.
	ResolveProductByID(ctx context.Context, id uuid.UUID) (*ProductSummary, error)

	// Quote calculates the price for a (product, material, size) combination.
	// Returns ErrProductNotFound, ErrMaterialNotFound, ErrPricingNotAvailable,
	// or ErrDimensionsOutOfRange on domain failure.
	Quote(ctx context.Context, req QuoteRequest) (*QuoteResult, error)
}
