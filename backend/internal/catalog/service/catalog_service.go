package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/catalog/catalogapi"
	"github.com/rajaku-printing/backend/internal/catalog/model"
	"github.com/rajaku-printing/backend/internal/catalog/repository"
)

// Service implements catalogapi.CatalogService by composing the three repos
// (product, material, pricing). Only this service knows the DB — handlers &
// external module consumers call the interface, never the repos directly.
type Service struct {
	products  *repository.ProductRepository
	materials *repository.MaterialRepository
	pricings  *repository.PricingRepository
}

var _ catalogapi.CatalogService = (*Service)(nil)

func New(products *repository.ProductRepository, materials *repository.MaterialRepository, pricings *repository.PricingRepository) *Service {
	return &Service{products: products, materials: materials, pricings: pricings}
}

// ListActiveProducts is a service helper (bukan bagian dari catalogapi
// interface karena hanya dipakai handler internal — konsumen eksternal cukup
// pakai Resolve*).
func (s *Service) ListActiveProducts(ctx context.Context) ([]model.Product, error) {
	return s.products.ListActive(ctx)
}

func (s *Service) ListActiveMaterials(ctx context.Context) ([]model.Material, error) {
	return s.materials.ListActive(ctx)
}

// GetProductDetail returns product + pricings preloaded (for public detail page).
func (s *Service) GetProductDetail(ctx context.Context, slug string) (*model.Product, error) {
	p, err := s.products.FindBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, catalogapi.ErrProductNotFound
		}
		return nil, fmt.Errorf("get product detail: %w", err)
	}
	return p, nil
}

func (s *Service) ResolveProductBySlug(ctx context.Context, slug string) (*catalogapi.ProductSummary, error) {
	p, err := s.products.FindBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, catalogapi.ErrProductNotFound
		}
		return nil, fmt.Errorf("resolve product by slug: %w", err)
	}
	return productToSummary(p), nil
}

func (s *Service) ResolveProductByID(ctx context.Context, id uuid.UUID) (*catalogapi.ProductSummary, error) {
	p, err := s.products.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, catalogapi.ErrProductNotFound
		}
		return nil, fmt.Errorf("resolve product by id: %w", err)
	}
	return productToSummary(p), nil
}

// Quote is the main external-facing pricing entrypoint. Delegates to pure
// calcPerM2/calcPaket after loading + validating the domain objects.
func (s *Service) Quote(ctx context.Context, req catalogapi.QuoteRequest) (*catalogapi.QuoteResult, error) {
	product, err := s.products.FindByID(ctx, req.ProductID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, catalogapi.ErrProductNotFound
		}
		return nil, fmt.Errorf("quote: load product: %w", err)
	}
	if !product.IsActive {
		return nil, catalogapi.ErrProductNotFound
	}

	material, err := s.materials.FindByID(ctx, req.MaterialID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, catalogapi.ErrMaterialNotFound
		}
		return nil, fmt.Errorf("quote: load material: %w", err)
	}
	if !material.IsActive {
		return nil, catalogapi.ErrMaterialNotFound
	}

	if err := validateDimensions(*product, req.WidthCm, req.HeightCm); err != nil {
		return nil, err
	}

	var result catalogapi.QuoteResult
	switch product.PricingType {
	case model.PricingTypePerM2:
		row, err := s.pricings.FindPerM2(ctx, product.ID, material.ID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return nil, catalogapi.ErrPricingNotAvailable
			}
			return nil, fmt.Errorf("quote: find per_m2 row: %w", err)
		}
		result = calcPerM2(*row, req.WidthCm, req.HeightCm)

	case model.PricingTypePaket:
		row, err := s.pricings.FindPaket(ctx, product.ID, material.ID, req.WidthCm, req.HeightCm)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return nil, catalogapi.ErrPricingNotAvailable
			}
			return nil, fmt.Errorf("quote: find paket row: %w", err)
		}
		result = calcPaket(*row)

	default:
		return nil, fmt.Errorf("quote: unknown pricing_type %q for product %s", product.PricingType, product.ID)
	}

	result.ProductID = product.ID
	result.ProductName = product.Name
	result.MaterialID = material.ID
	result.MaterialName = material.Name
	return &result, nil
}

func productToSummary(p *model.Product) *catalogapi.ProductSummary {
	return &catalogapi.ProductSummary{
		ID:            p.ID,
		Slug:          p.Slug,
		Name:          p.Name,
		Category:      p.Category,
		PricingType:   catalogapi.PricingType(p.PricingType),
		MinWidthCm:    p.MinWidthCm,
		MinHeightCm:   p.MinHeightCm,
		MaxWidthCm:    p.MaxWidthCm,
		MaxHeightCm:   p.MaxHeightCm,
		IsActive:      p.IsActive,
	}
}
