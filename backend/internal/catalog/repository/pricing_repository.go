package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/rajaku-printing/backend/internal/catalog/model"
)

type PricingRepository struct {
	db *gorm.DB
}

func NewPricingRepository(db *gorm.DB) *PricingRepository { return &PricingRepository{db: db} }

// FindPerM2 returns the single per_m2 pricing row for (product, material), or ErrNotFound.
func (r *PricingRepository) FindPerM2(ctx context.Context, productID, materialID uuid.UUID) (*model.ProductPricing, error) {
	var p model.ProductPricing
	err := r.db.WithContext(ctx).
		Where("product_id = ? AND material_id = ? AND price_per_m2 IS NOT NULL AND is_active = ?",
			productID, materialID, true).
		First(&p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find per_m2 pricing (product=%s material=%s): %w", productID, materialID, err)
	}
	return &p, nil
}

// ListByProduct returns every pricing row (active + inactive) for a product,
// with material preloaded. Admin editor uses this.
func (r *PricingRepository) ListByProduct(ctx context.Context, productID uuid.UUID) ([]model.ProductPricing, error) {
	var items []model.ProductPricing
	err := r.db.WithContext(ctx).
		Preload("Material").
		Where("product_id = ?", productID).
		Order("created_at ASC").
		Find(&items).Error
	if err != nil {
		return nil, fmt.Errorf("list pricings for product %s: %w", productID, err)
	}
	return items, nil
}

func (r *PricingRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.ProductPricing, error) {
	var p model.ProductPricing
	err := r.db.WithContext(ctx).First(&p, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find pricing by id %s: %w", id, err)
	}
	return &p, nil
}

func (r *PricingRepository) Create(ctx context.Context, p *model.ProductPricing) error {
	if err := r.db.WithContext(ctx).Create(p).Error; err != nil {
		return fmt.Errorf("create pricing: %w", err)
	}
	return nil
}

func (r *PricingRepository) Update(ctx context.Context, id uuid.UUID, patch map[string]any) error {
	res := r.db.WithContext(ctx).Model(&model.ProductPricing{}).Where("id = ?", id).Updates(patch)
	if res.Error != nil {
		return fmt.Errorf("update pricing %s: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PricingRepository) Delete(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Delete(&model.ProductPricing{}, "id = ?", id)
	if res.Error != nil {
		return fmt.Errorf("delete pricing %s: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// FindPaket returns the paket pricing row that exactly matches (product, material, width, height).
func (r *PricingRepository) FindPaket(ctx context.Context, productID, materialID uuid.UUID, widthCm, heightCm int) (*model.ProductPricing, error) {
	var p model.ProductPricing
	err := r.db.WithContext(ctx).
		Where("product_id = ? AND material_id = ? AND width_cm = ? AND height_cm = ? AND is_active = ?",
			productID, materialID, widthCm, heightCm, true).
		First(&p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find paket pricing (product=%s material=%s size=%dx%d): %w",
			productID, materialID, widthCm, heightCm, err)
	}
	return &p, nil
}
