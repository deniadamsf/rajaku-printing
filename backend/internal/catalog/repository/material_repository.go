package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/rajaku-printing/backend/internal/catalog/model"
)

var ErrNotFound = errors.New("catalog/repository: not found")

type MaterialRepository struct {
	db *gorm.DB
}

func NewMaterialRepository(db *gorm.DB) *MaterialRepository { return &MaterialRepository{db: db} }

func (r *MaterialRepository) ListActive(ctx context.Context) ([]model.Material, error) {
	var items []model.Material
	err := r.db.WithContext(ctx).
		Where("is_active = ?", true).
		Order("name ASC").
		Find(&items).Error
	if err != nil {
		return nil, fmt.Errorf("list active materials: %w", err)
	}
	return items, nil
}

func (r *MaterialRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.Material, error) {
	var m model.Material
	err := r.db.WithContext(ctx).First(&m, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find material by id %s: %w", id, err)
	}
	return &m, nil
}

// FindByCode returns the material with the given unique code, or ErrNotFound.
// Dipakai untuk idempotency check (mis. cmd/seedcatalog get-or-create by code).
func (r *MaterialRepository) FindByCode(ctx context.Context, code string) (*model.Material, error) {
	var m model.Material
	err := r.db.WithContext(ctx).Where("code = ?", code).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find material by code %q: %w", code, err)
	}
	return &m, nil
}

// ListAll returns every material, active + inactive. Admin-only.
func (r *MaterialRepository) ListAll(ctx context.Context) ([]model.Material, error) {
	var items []model.Material
	err := r.db.WithContext(ctx).Order("name ASC").Find(&items).Error
	if err != nil {
		return nil, fmt.Errorf("list all materials: %w", err)
	}
	return items, nil
}

func (r *MaterialRepository) Create(ctx context.Context, m *model.Material) error {
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return fmt.Errorf("create material: %w", err)
	}
	return nil
}

// Update patches only non-zero fields via a map to avoid overwriting is_active
// implicitly. Caller passes the fields to change.
func (r *MaterialRepository) Update(ctx context.Context, id uuid.UUID, patch map[string]any) error {
	res := r.db.WithContext(ctx).Model(&model.Material{}).Where("id = ?", id).Updates(patch)
	if res.Error != nil {
		return fmt.Errorf("update material %s: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *MaterialRepository) SetActive(ctx context.Context, id uuid.UUID, active bool) error {
	return r.Update(ctx, id, map[string]any{"is_active": active})
}
