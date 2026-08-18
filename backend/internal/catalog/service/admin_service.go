// Package service — admin CRUD (materials, products, pricings) untuk §9/§10.
//
// Kontrak: hanya modul catalog yang tahu shape DB. Handler admin panel harus
// panggil AdminService.* → AdminService validasi & delegate ke repository.
// Validasi konsistensi (pricing_type parent vs bentuk pricing row, slug format,
// dimensi paket unik) di-enforce di sini, bukan cuma di DB CHECK.
package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/catalog/catalogapi"
	"github.com/rajaku-printing/backend/internal/catalog/model"
	"github.com/rajaku-printing/backend/internal/catalog/repository"
)

// --- Extra domain errors (admin-only) ------------------------------------

var (
	ErrValidation             = errors.New("catalog admin: validation failed")
	ErrPricingTypeLocked      = errors.New("catalog admin: pricing_type tidak bisa diubah setelah produk punya pricing row")
	ErrPricingShapeMismatch   = errors.New("catalog admin: bentuk pricing tidak cocok dengan pricing_type produk")
	ErrDuplicateSlug          = errors.New("catalog admin: slug produk sudah dipakai")
	ErrDuplicateMaterialCode  = errors.New("catalog admin: code bahan sudah dipakai")
	ErrDuplicatePricing       = errors.New("catalog admin: pricing untuk kombinasi ini sudah ada")
	ErrPricingRowNotFound     = errors.New("catalog admin: pricing row tidak ditemukan")
)

var slugRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
var codeRe = regexp.MustCompile(`^[A-Z0-9_-]+$`)

// -------------------------------------------------------------------------
//  Material inputs
// -------------------------------------------------------------------------

type MaterialInput struct {
	Code        string
	Name        string
	Description string
}

func (in MaterialInput) validate() error {
	code := strings.TrimSpace(in.Code)
	name := strings.TrimSpace(in.Name)
	if code == "" || len(code) > 50 || !codeRe.MatchString(code) {
		return fmt.Errorf("%w: code wajib (A-Z, 0-9, -, _), max 50 karakter", ErrValidation)
	}
	if name == "" || len(name) > 150 {
		return fmt.Errorf("%w: name wajib, max 150 karakter", ErrValidation)
	}
	return nil
}

// -------------------------------------------------------------------------
//  Product inputs
// -------------------------------------------------------------------------

type ProductInput struct {
	Slug         string
	Name         string
	Description  string
	Category     string
	PricingType  string // per_m2 | paket
	MinWidthCm   *int
	MinHeightCm  *int
	MaxWidthCm   *int
	MaxHeightCm  *int
	DisplayOrder int
}

func (in ProductInput) validate(requirePricingType bool) error {
	slug := strings.TrimSpace(in.Slug)
	name := strings.TrimSpace(in.Name)
	cat := strings.TrimSpace(in.Category)
	if slug == "" || len(slug) > 150 || !slugRe.MatchString(slug) {
		return fmt.Errorf("%w: slug wajib (a-z, 0-9, -), max 150 karakter", ErrValidation)
	}
	if name == "" || len(name) > 200 {
		return fmt.Errorf("%w: name wajib, max 200 karakter", ErrValidation)
	}
	if cat == "" || len(cat) > 50 {
		return fmt.Errorf("%w: category wajib, max 50 karakter", ErrValidation)
	}
	if requirePricingType {
		if in.PricingType != string(model.PricingTypePerM2) && in.PricingType != string(model.PricingTypePaket) {
			return fmt.Errorf("%w: pricing_type harus per_m2 atau paket", ErrValidation)
		}
	}
	// Dimension bounds sanity: min <= max jika keduanya diisi.
	if in.MinWidthCm != nil && in.MaxWidthCm != nil && *in.MinWidthCm > *in.MaxWidthCm {
		return fmt.Errorf("%w: min_width_cm > max_width_cm", ErrValidation)
	}
	if in.MinHeightCm != nil && in.MaxHeightCm != nil && *in.MinHeightCm > *in.MaxHeightCm {
		return fmt.Errorf("%w: min_height_cm > max_height_cm", ErrValidation)
	}
	return nil
}

// -------------------------------------------------------------------------
//  Pricing inputs
// -------------------------------------------------------------------------

type PricingInput struct {
	MaterialID   uuid.UUID
	// per_m2:
	PricePerM2  *int64
	MinChargeM2 *float64
	// paket:
	WidthCm      *int
	HeightCm     *int
	PackageLabel string
	PriceTotal   *int64
}

// -------------------------------------------------------------------------
//  Service methods — Materials
// -------------------------------------------------------------------------

func (s *Service) AdminListMaterials(ctx context.Context) ([]model.Material, error) {
	return s.materials.ListAll(ctx)
}

func (s *Service) AdminCreateMaterial(ctx context.Context, in MaterialInput) (*model.Material, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	m := &model.Material{
		Code:     strings.TrimSpace(in.Code),
		Name:     strings.TrimSpace(in.Name),
		IsActive: true,
	}
	if d := strings.TrimSpace(in.Description); d != "" {
		m.Description = &d
	}
	if err := s.materials.Create(ctx, m); err != nil {
		if isUniqueViolation(err, "materials_code") || strings.Contains(err.Error(), "materials_code_key") {
			return nil, ErrDuplicateMaterialCode
		}
		return nil, err
	}
	return m, nil
}

func (s *Service) AdminUpdateMaterial(ctx context.Context, id uuid.UUID, in MaterialInput) (*model.Material, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	patch := map[string]any{
		"code": strings.TrimSpace(in.Code),
		"name": strings.TrimSpace(in.Name),
	}
	if d := strings.TrimSpace(in.Description); d != "" {
		patch["description"] = d
	} else {
		patch["description"] = nil
	}
	if err := s.materials.Update(ctx, id, patch); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, catalogapi.ErrMaterialNotFound
		}
		if isUniqueViolation(err, "materials_code") || strings.Contains(err.Error(), "materials_code_key") {
			return nil, ErrDuplicateMaterialCode
		}
		return nil, err
	}
	return s.materials.FindByID(ctx, id)
}

func (s *Service) AdminSetMaterialActive(ctx context.Context, id uuid.UUID, active bool) error {
	if err := s.materials.SetActive(ctx, id, active); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return catalogapi.ErrMaterialNotFound
		}
		return err
	}
	return nil
}

// -------------------------------------------------------------------------
//  Service methods — Products
// -------------------------------------------------------------------------

func (s *Service) AdminListProducts(ctx context.Context) ([]model.Product, error) {
	return s.products.ListAll(ctx)
}

func (s *Service) AdminGetProduct(ctx context.Context, id uuid.UUID) (*model.Product, error) {
	p, err := s.products.FindByIDWithPricings(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, catalogapi.ErrProductNotFound
		}
		return nil, err
	}
	return p, nil
}

func (s *Service) AdminCreateProduct(ctx context.Context, in ProductInput) (*model.Product, error) {
	if err := in.validate(true); err != nil {
		return nil, err
	}
	p := &model.Product{
		Slug:         strings.TrimSpace(in.Slug),
		Name:         strings.TrimSpace(in.Name),
		Category:     strings.TrimSpace(in.Category),
		PricingType:  model.PricingType(in.PricingType),
		MinWidthCm:   in.MinWidthCm,
		MinHeightCm:  in.MinHeightCm,
		MaxWidthCm:   in.MaxWidthCm,
		MaxHeightCm:  in.MaxHeightCm,
		DisplayOrder: in.DisplayOrder,
		IsActive:     true,
	}
	if d := strings.TrimSpace(in.Description); d != "" {
		p.Description = &d
	}
	if err := s.products.Create(ctx, p); err != nil {
		if isUniqueViolation(err, "products_slug") || strings.Contains(err.Error(), "products_slug_key") {
			return nil, ErrDuplicateSlug
		}
		return nil, err
	}
	return p, nil
}

func (s *Service) AdminUpdateProduct(ctx context.Context, id uuid.UUID, in ProductInput) (*model.Product, error) {
	if err := in.validate(false); err != nil {
		return nil, err
	}
	existing, err := s.products.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, catalogapi.ErrProductNotFound
		}
		return nil, err
	}
	patch := map[string]any{
		"slug":          strings.TrimSpace(in.Slug),
		"name":          strings.TrimSpace(in.Name),
		"category":      strings.TrimSpace(in.Category),
		"min_width_cm":  in.MinWidthCm,
		"min_height_cm": in.MinHeightCm,
		"max_width_cm":  in.MaxWidthCm,
		"max_height_cm": in.MaxHeightCm,
		"display_order": in.DisplayOrder,
	}
	if d := strings.TrimSpace(in.Description); d != "" {
		patch["description"] = d
	} else {
		patch["description"] = nil
	}
	// pricing_type hanya boleh diubah kalau belum ada pricing row.
	if in.PricingType != "" && in.PricingType != string(existing.PricingType) {
		has, err := s.products.HasAnyPricings(ctx, id)
		if err != nil {
			return nil, err
		}
		if has {
			return nil, ErrPricingTypeLocked
		}
		if in.PricingType != string(model.PricingTypePerM2) && in.PricingType != string(model.PricingTypePaket) {
			return nil, fmt.Errorf("%w: pricing_type harus per_m2 atau paket", ErrValidation)
		}
		patch["pricing_type"] = in.PricingType
	}

	if err := s.products.Update(ctx, id, patch); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, catalogapi.ErrProductNotFound
		}
		if isUniqueViolation(err, "products_slug") || strings.Contains(err.Error(), "products_slug_key") {
			return nil, ErrDuplicateSlug
		}
		return nil, err
	}
	return s.products.FindByIDWithPricings(ctx, id)
}

func (s *Service) AdminSetProductActive(ctx context.Context, id uuid.UUID, active bool) error {
	if err := s.products.SetActive(ctx, id, active); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return catalogapi.ErrProductNotFound
		}
		return err
	}
	return nil
}

// -------------------------------------------------------------------------
//  Service methods — Pricings
// -------------------------------------------------------------------------

func (s *Service) AdminCreatePricing(ctx context.Context, productID uuid.UUID, in PricingInput) (*model.ProductPricing, error) {
	product, err := s.products.FindByID(ctx, productID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, catalogapi.ErrProductNotFound
		}
		return nil, err
	}
	// Pastikan material ada & aktif (soft: bahan non-aktif tetap boleh dipakai kalau
	// admin sengaja migrasi; tapi validasi eksistensinya wajib).
	if _, err := s.materials.FindByID(ctx, in.MaterialID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, catalogapi.ErrMaterialNotFound
		}
		return nil, err
	}
	row, err := pricingFromInput(product, in)
	if err != nil {
		return nil, err
	}
	row.ProductID = product.ID
	row.MaterialID = in.MaterialID
	row.IsActive = true
	if err := s.pricings.Create(ctx, row); err != nil {
		if isUniqueViolation(err, "idx_pricing_") {
			return nil, ErrDuplicatePricing
		}
		return nil, err
	}
	return row, nil
}

func (s *Service) AdminUpdatePricing(ctx context.Context, id uuid.UUID, in PricingInput) (*model.ProductPricing, error) {
	existing, err := s.pricings.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrPricingRowNotFound
		}
		return nil, err
	}
	product, err := s.products.FindByID(ctx, existing.ProductID)
	if err != nil {
		return nil, err
	}
	rebuilt, err := pricingFromInput(product, in)
	if err != nil {
		return nil, err
	}
	patch := map[string]any{
		"material_id":   in.MaterialID,
		"price_per_m2":  rebuilt.PricePerM2,
		"min_charge_m2": rebuilt.MinChargeM2,
		"width_cm":      rebuilt.WidthCm,
		"height_cm":     rebuilt.HeightCm,
		"package_label": rebuilt.PackageLabel,
		"price_total":   rebuilt.PriceTotal,
	}
	if err := s.pricings.Update(ctx, id, patch); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrPricingRowNotFound
		}
		if isUniqueViolation(err, "idx_pricing_") {
			return nil, ErrDuplicatePricing
		}
		return nil, err
	}
	return s.pricings.FindByID(ctx, id)
}

func (s *Service) AdminSetPricingActive(ctx context.Context, id uuid.UUID, active bool) error {
	if err := s.pricings.Update(ctx, id, map[string]any{"is_active": active}); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrPricingRowNotFound
		}
		return err
	}
	return nil
}

func (s *Service) AdminDeletePricing(ctx context.Context, id uuid.UUID) error {
	if err := s.pricings.Delete(ctx, id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrPricingRowNotFound
		}
		return err
	}
	return nil
}

// -------------------------------------------------------------------------
//  Helpers
// -------------------------------------------------------------------------

// pricingFromInput builds a ProductPricing shell matching the parent product's
// pricing_type — it does NOT set ID/ProductID/MaterialID (caller fills those).
func pricingFromInput(product *model.Product, in PricingInput) (*model.ProductPricing, error) {
	row := &model.ProductPricing{}
	switch product.PricingType {
	case model.PricingTypePerM2:
		if in.PricePerM2 == nil || *in.PricePerM2 <= 0 {
			return nil, fmt.Errorf("%w: price_per_m2 wajib > 0", ErrValidation)
		}
		if in.WidthCm != nil || in.HeightCm != nil || in.PriceTotal != nil {
			return nil, fmt.Errorf("%w: %v", ErrPricingShapeMismatch, "pricing per_m2 tidak boleh isi width/height/price_total")
		}
		row.PricePerM2 = in.PricePerM2
		if in.MinChargeM2 != nil && *in.MinChargeM2 > 0 {
			row.MinChargeM2 = in.MinChargeM2
		}
	case model.PricingTypePaket:
		if in.WidthCm == nil || *in.WidthCm <= 0 {
			return nil, fmt.Errorf("%w: width_cm wajib > 0", ErrValidation)
		}
		if in.HeightCm == nil || *in.HeightCm <= 0 {
			return nil, fmt.Errorf("%w: height_cm wajib > 0", ErrValidation)
		}
		if in.PriceTotal == nil || *in.PriceTotal <= 0 {
			return nil, fmt.Errorf("%w: price_total wajib > 0", ErrValidation)
		}
		if in.PricePerM2 != nil {
			return nil, fmt.Errorf("%w: %v", ErrPricingShapeMismatch, "pricing paket tidak boleh isi price_per_m2")
		}
		row.WidthCm = in.WidthCm
		row.HeightCm = in.HeightCm
		row.PriceTotal = in.PriceTotal
		if lbl := strings.TrimSpace(in.PackageLabel); lbl != "" {
			row.PackageLabel = &lbl
		}
	default:
		return nil, fmt.Errorf("catalog admin: unknown pricing_type %q", product.PricingType)
	}
	return row, nil
}

// isUniqueViolation checks whether the DB error message hints at a unique
// constraint violation on the given hint (index or constraint name substring).
// GORM wraps pgconn errors; we do a substring check to avoid pulling pgconn
// directly into the domain package.
func isUniqueViolation(err error, hint string) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "duplicate key value") && strings.Contains(msg, hint)
}
