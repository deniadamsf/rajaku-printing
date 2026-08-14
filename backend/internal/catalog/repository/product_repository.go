package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/rajaku-printing/backend/internal/catalog/model"
)

type ProductRepository struct {
	db *gorm.DB
}

func NewProductRepository(db *gorm.DB) *ProductRepository { return &ProductRepository{db: db} }

// ListActive returns active products ordered by display_order. Pricings NOT
// preloaded (call FindBySlug for detail view).
func (r *ProductRepository) ListActive(ctx context.Context) ([]model.Product, error) {
	var items []model.Product
	err := r.db.WithContext(ctx).
		Where("is_active = ?", true).
		Order("display_order ASC, name ASC").
		Find(&items).Error
	if err != nil {
		return nil, fmt.Errorf("list active products: %w", err)
	}
	return items, nil
}

// FindBySlug returns the product with its pricings + material info preloaded.
func (r *ProductRepository) FindBySlug(ctx context.Context, slug string) (*model.Product, error) {
	var p model.Product
	err := r.db.WithContext(ctx).
		Preload("Pricings", "is_active = ?", true).
		Preload("Pricings.Material").
		Where("slug = ?", slug).
		First(&p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find product by slug %q: %w", slug, err)
	}
	return &p, nil
}

func (r *ProductRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.Product, error) {
	var p model.Product
	err := r.db.WithContext(ctx).First(&p, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find product by id %s: %w", id, err)
	}
	return &p, nil
}

// FindByIDWithPricings loads a product with its (all, incl. inactive) pricings
// and their materials preloaded. Admin detail view.
func (r *ProductRepository) FindByIDWithPricings(ctx context.Context, id uuid.UUID) (*model.Product, error) {
	var p model.Product
	err := r.db.WithContext(ctx).
		Preload("Pricings").
		Preload("Pricings.Material").
		First(&p, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find product by id %s with pricings: %w", id, err)
	}
	return &p, nil
}

// ListAll returns every product, active + inactive, ordered by display_order.
func (r *ProductRepository) ListAll(ctx context.Context) ([]model.Product, error) {
	var items []model.Product
	err := r.db.WithContext(ctx).
		Order("display_order ASC, name ASC").
		Find(&items).Error
	if err != nil {
		return nil, fmt.Errorf("list all products: %w", err)
	}
	return items, nil
}

func (r *ProductRepository) Create(ctx context.Context, p *model.Product) error {
	if err := r.db.WithContext(ctx).Create(p).Error; err != nil {
		return fmt.Errorf("create product: %w", err)
	}
	return nil
}

func (r *ProductRepository) Update(ctx context.Context, id uuid.UUID, patch map[string]any) error {
	res := r.db.WithContext(ctx).Model(&model.Product{}).Where("id = ?", id).Updates(patch)
	if res.Error != nil {
		return fmt.Errorf("update product %s: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *ProductRepository) SetActive(ctx context.Context, id uuid.UUID, active bool) error {
	return r.Update(ctx, id, map[string]any{"is_active": active})
}

// HasActivePricings returns true if at least one pricing row exists (any state).
// Dipakai untuk cegah ganti pricing_type produk yang sudah punya baris harga.
func (r *ProductRepository) HasAnyPricings(ctx context.Context, id uuid.UUID) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Table("product_pricings").
		Where("product_id = ?", id).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("count pricings for product %s: %w", id, err)
	}
	return count > 0, nil
}
