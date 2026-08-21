package handler

import (
	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/catalog/catalogapi"
	"github.com/rajaku-printing/backend/internal/catalog/model"
	"github.com/rajaku-printing/backend/internal/catalog/service"
)

// productImageURL — URL publik gambar produk, DIHITUNG dari APP_BASE_URL
// (§2) via service.ProductImageURL saat serialisasi response — DB (model.
// Product.ImagePath) cuma menyimpan path relatif, tidak pernah dipancarkan
// mentah ke klien (§22, hindari data lama menunjuk host lama kalau
// APP_BASE_URL berubah). Nil kalau produk belum punya gambar.
func productImageURL(svc *service.Service, p model.Product) *string {
	if p.ImagePath == nil {
		return nil
	}
	url := svc.ProductImageURL(p.ID)
	return &url
}

// ---- Request DTOs ----

type quoteRequest struct {
	ProductID  uuid.UUID `json:"product_id"  binding:"required"`
	MaterialID uuid.UUID `json:"material_id" binding:"required"`
	WidthCm    int       `json:"width_cm"    binding:"required,gt=0"`
	HeightCm   int       `json:"height_cm"   binding:"required,gt=0"`
}

// ---- Response DTOs ----

type materialResponse struct {
	ID          uuid.UUID `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
}

func toMaterialResponse(m model.Material) materialResponse {
	r := materialResponse{ID: m.ID, Code: m.Code, Name: m.Name}
	if m.Description != nil {
		r.Description = *m.Description
	}
	return r
}

type productListItem struct {
	ID           uuid.UUID `json:"id"`
	Slug         string    `json:"slug"`
	Name         string    `json:"name"`
	Description  string    `json:"description,omitempty"`
	Category     string    `json:"category"`
	PricingType  string    `json:"pricing_type"`
	DisplayOrder int       `json:"display_order"`
	ImageURL     *string   `json:"image_url,omitempty"`
}

func toProductListItem(svc *service.Service, p model.Product) productListItem {
	r := productListItem{
		ID:           p.ID,
		Slug:         p.Slug,
		Name:         p.Name,
		Category:     p.Category,
		PricingType:  string(p.PricingType),
		DisplayOrder: p.DisplayOrder,
		ImageURL:     productImageURL(svc, p),
	}
	if p.Description != nil {
		r.Description = *p.Description
	}
	return r
}

type pricingRowResponse struct {
	MaterialID   uuid.UUID `json:"material_id"`
	MaterialName string    `json:"material_name"`
	MaterialCode string    `json:"material_code"`

	// per_m2
	PricePerM2  *int64   `json:"price_per_m2,omitempty"`
	MinChargeM2 *float64 `json:"min_charge_m2,omitempty"`

	// paket
	WidthCm      *int    `json:"width_cm,omitempty"`
	HeightCm     *int    `json:"height_cm,omitempty"`
	PackageLabel *string `json:"package_label,omitempty"`
	PriceTotal   *int64  `json:"price_total,omitempty"`
}

func toPricingRowResponse(p model.ProductPricing) pricingRowResponse {
	r := pricingRowResponse{
		MaterialID:   p.MaterialID,
		PricePerM2:   p.PricePerM2,
		MinChargeM2:  p.MinChargeM2,
		WidthCm:      p.WidthCm,
		HeightCm:     p.HeightCm,
		PackageLabel: p.PackageLabel,
		PriceTotal:   p.PriceTotal,
	}
	if p.Material != nil {
		r.MaterialName = p.Material.Name
		r.MaterialCode = p.Material.Code
	}
	return r
}

type productDetailResponse struct {
	ID          uuid.UUID            `json:"id"`
	Slug        string               `json:"slug"`
	Name        string               `json:"name"`
	Description string               `json:"description,omitempty"`
	Category    string               `json:"category"`
	PricingType string               `json:"pricing_type"`
	MinWidthCm  *int                 `json:"min_width_cm,omitempty"`
	MinHeightCm *int                 `json:"min_height_cm,omitempty"`
	MaxWidthCm  *int                 `json:"max_width_cm,omitempty"`
	MaxHeightCm *int                 `json:"max_height_cm,omitempty"`
	ImageURL    *string              `json:"image_url,omitempty"`
	Pricings    []pricingRowResponse `json:"pricings"`
}

func toProductDetailResponse(svc *service.Service, p model.Product) productDetailResponse {
	rows := make([]pricingRowResponse, 0, len(p.Pricings))
	for _, pr := range p.Pricings {
		rows = append(rows, toPricingRowResponse(pr))
	}
	r := productDetailResponse{
		ID:          p.ID,
		Slug:        p.Slug,
		Name:        p.Name,
		Category:    p.Category,
		PricingType: string(p.PricingType),
		MinWidthCm:  p.MinWidthCm,
		MinHeightCm: p.MinHeightCm,
		MaxWidthCm:  p.MaxWidthCm,
		MaxHeightCm: p.MaxHeightCm,
		ImageURL:    productImageURL(svc, p),
		Pricings:    rows,
	}
	if p.Description != nil {
		r.Description = *p.Description
	}
	return r
}

type quoteResponse struct {
	ProductID    uuid.UUID `json:"product_id"`
	ProductName  string    `json:"product_name"`
	MaterialID   uuid.UUID `json:"material_id"`
	MaterialName string    `json:"material_name"`
	PricingType  string    `json:"pricing_type"`

	WidthCm    int   `json:"width_cm"`
	HeightCm   int   `json:"height_cm"`
	TotalPrice int64 `json:"total_price"`

	// per_m2 breakdown (omit for paket)
	AreaM2       *float64 `json:"area_m2,omitempty"`
	ChargeableM2 *float64 `json:"chargeable_m2,omitempty"`
	PricePerM2   *int64   `json:"price_per_m2,omitempty"`

	// paket breakdown (omit for per_m2)
	PackageLabel string `json:"package_label,omitempty"`
}

// ---- Admin response DTOs ----

type materialAdminResponse struct {
	ID          uuid.UUID `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   string    `json:"created_at"`
	UpdatedAt   string    `json:"updated_at"`
}

func toMaterialAdminResponse(m model.Material) materialAdminResponse {
	r := materialAdminResponse{
		ID:        m.ID,
		Code:      m.Code,
		Name:      m.Name,
		IsActive:  m.IsActive,
		CreatedAt: m.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt: m.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if m.Description != nil {
		r.Description = *m.Description
	}
	return r
}

type productAdminListItem struct {
	ID           uuid.UUID `json:"id"`
	Slug         string    `json:"slug"`
	Name         string    `json:"name"`
	Description  string    `json:"description,omitempty"`
	Category     string    `json:"category"`
	PricingType  string    `json:"pricing_type"`
	MinWidthCm   *int      `json:"min_width_cm,omitempty"`
	MinHeightCm  *int      `json:"min_height_cm,omitempty"`
	MaxWidthCm   *int      `json:"max_width_cm,omitempty"`
	MaxHeightCm  *int      `json:"max_height_cm,omitempty"`
	ImageURL     *string   `json:"image_url,omitempty"`
	IsActive     bool      `json:"is_active"`
	DisplayOrder int       `json:"display_order"`
	CreatedAt    string    `json:"created_at"`
	UpdatedAt    string    `json:"updated_at"`
}

func toProductAdminListItem(svc *service.Service, p model.Product) productAdminListItem {
	r := productAdminListItem{
		ID:           p.ID,
		Slug:         p.Slug,
		Name:         p.Name,
		Category:     p.Category,
		PricingType:  string(p.PricingType),
		MinWidthCm:   p.MinWidthCm,
		MinHeightCm:  p.MinHeightCm,
		MaxWidthCm:   p.MaxWidthCm,
		MaxHeightCm:  p.MaxHeightCm,
		ImageURL:     productImageURL(svc, p),
		IsActive:     p.IsActive,
		DisplayOrder: p.DisplayOrder,
		CreatedAt:    p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:    p.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if p.Description != nil {
		r.Description = *p.Description
	}
	return r
}

type pricingAdminResponse struct {
	ID           uuid.UUID `json:"id"`
	ProductID    uuid.UUID `json:"product_id"`
	MaterialID   uuid.UUID `json:"material_id"`
	MaterialName string    `json:"material_name,omitempty"`
	MaterialCode string    `json:"material_code,omitempty"`
	PricePerM2   *int64    `json:"price_per_m2,omitempty"`
	MinChargeM2  *float64  `json:"min_charge_m2,omitempty"`
	WidthCm      *int      `json:"width_cm,omitempty"`
	HeightCm     *int      `json:"height_cm,omitempty"`
	PackageLabel *string   `json:"package_label,omitempty"`
	PriceTotal   *int64    `json:"price_total,omitempty"`
	IsActive     bool      `json:"is_active"`
}

func toPricingAdminResponse(p model.ProductPricing) pricingAdminResponse {
	r := pricingAdminResponse{
		ID:           p.ID,
		ProductID:    p.ProductID,
		MaterialID:   p.MaterialID,
		PricePerM2:   p.PricePerM2,
		MinChargeM2:  p.MinChargeM2,
		WidthCm:      p.WidthCm,
		HeightCm:     p.HeightCm,
		PackageLabel: p.PackageLabel,
		PriceTotal:   p.PriceTotal,
		IsActive:     p.IsActive,
	}
	if p.Material != nil {
		r.MaterialName = p.Material.Name
		r.MaterialCode = p.Material.Code
	}
	return r
}

type productAdminDetail struct {
	productAdminListItem
	Pricings []pricingAdminResponse `json:"pricings"`
}

func toProductAdminDetail(svc *service.Service, p model.Product) productAdminDetail {
	base := toProductAdminListItem(svc, p)
	rows := make([]pricingAdminResponse, 0, len(p.Pricings))
	for _, pr := range p.Pricings {
		rows = append(rows, toPricingAdminResponse(pr))
	}
	return productAdminDetail{productAdminListItem: base, Pricings: rows}
}

func toQuoteResponse(q *catalogapi.QuoteResult) quoteResponse {
	r := quoteResponse{
		ProductID:    q.ProductID,
		ProductName:  q.ProductName,
		MaterialID:   q.MaterialID,
		MaterialName: q.MaterialName,
		PricingType:  string(q.PricingType),
		WidthCm:      q.WidthCm,
		HeightCm:     q.HeightCm,
		TotalPrice:   q.TotalPrice,
	}
	switch q.PricingType {
	case catalogapi.PricingTypePerM2:
		area := q.AreaM2
		chargeable := q.ChargeableM2
		price := q.PricePerM2
		r.AreaM2 = &area
		r.ChargeableM2 = &chargeable
		r.PricePerM2 = &price
	case catalogapi.PricingTypePaket:
		r.PackageLabel = q.PackageLabel
	}
	return r
}
