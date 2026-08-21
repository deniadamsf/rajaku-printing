package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/catalog/catalogapi"
	"github.com/rajaku-printing/backend/internal/catalog/model"
	"github.com/rajaku-printing/backend/internal/catalog/repository"
)

// FileStore — narrowed filestore contract dipakai untuk gambar produk (§19 —
// disk lokal VPS). Shape sama dgn modul lain yang share instance filestore
// (mis. sitemedia, cms) — lihat internal/pkg/filestore.
type FileStore interface {
	Save(ctx context.Context, subpath string, r io.Reader) (int64, error)
	Delete(ctx context.Context, subpath string) error
	AbsPath(subpath string) (string, error)
}

// productStore — kontrak yang dipakai Service dari ProductRepository.
// Dipersempit jadi interface (bukan *repository.ProductRepository konkret)
// supaya bisa di-fake di unit test tanpa DB — pola sama seperti FileStore di
// atas. *repository.ProductRepository mengimplementasikan interface ini apa
// adanya (lihat var _ assertion di New).
type productStore interface {
	ListActive(ctx context.Context) ([]model.Product, error)
	ListAll(ctx context.Context) ([]model.Product, error)
	FindBySlug(ctx context.Context, slug string) (*model.Product, error)
	FindByID(ctx context.Context, id uuid.UUID) (*model.Product, error)
	FindByIDWithPricings(ctx context.Context, id uuid.UUID) (*model.Product, error)
	Create(ctx context.Context, p *model.Product) error
	Update(ctx context.Context, id uuid.UUID, patch map[string]any) error
	SetActive(ctx context.Context, id uuid.UUID, active bool) error
	HasAnyPricings(ctx context.Context, id uuid.UUID) (bool, error)
}

// Config — parameter service.
type Config struct {
	// BaseURL — dari config.App.BaseURL (§2 "satu sumber base URL"). Dipakai
	// membangun URL publik gambar produk (GET /api/v1/catalog/product-images/:id).
	// WAJIB diisi caller — service tidak fallback ke localhost/hardcode apa pun.
	BaseURL string
}

// Service implements catalogapi.CatalogService by composing the three repos
// (product, material, pricing). Only this service knows the DB — handlers &
// external module consumers call the interface, never the repos directly.
type Service struct {
	products  productStore
	materials *repository.MaterialRepository
	pricings  *repository.PricingRepository

	blobs   FileStore
	baseURL string
}

var _ catalogapi.CatalogService = (*Service)(nil)
var _ productStore = (*repository.ProductRepository)(nil)

func New(
	products *repository.ProductRepository,
	materials *repository.MaterialRepository,
	pricings *repository.PricingRepository,
	blobs FileStore,
	cfg Config,
) *Service {
	return &Service{
		products:  products,
		materials: materials,
		pricings:  pricings,
		blobs:     blobs,
		baseURL:   strings.TrimRight(cfg.BaseURL, "/"),
	}
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
		ID:          p.ID,
		Slug:        p.Slug,
		Name:        p.Name,
		Category:    p.Category,
		PricingType: catalogapi.PricingType(p.PricingType),
		MinWidthCm:  p.MinWidthCm,
		MinHeightCm: p.MinHeightCm,
		MaxWidthCm:  p.MaxWidthCm,
		MaxHeightCm: p.MaxHeightCm,
		IsActive:    p.IsActive,
	}
}
