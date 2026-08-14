// Package repository — DB access untuk modul settings.
package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/rajaku-printing/backend/internal/settings/model"
)

// ErrNotFound — key tidak ada di tabel app_settings.
var ErrNotFound = errors.New("settings/repository: setting not found")

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

// Get returns satu setting by key, atau ErrNotFound.
func (r *Repository) Get(ctx context.Context, key string) (*model.AppSetting, error) {
	var s model.AppSetting
	err := r.db.WithContext(ctx).First(&s, "key = ?", key).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get setting %q: %w", key, err)
	}
	return &s, nil
}

// List returns semua setting, urut key (stabil untuk UI admin).
func (r *Repository) List(ctx context.Context) ([]model.AppSetting, error) {
	var items []model.AppSetting
	if err := r.db.WithContext(ctx).Order("key ASC").Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list settings: %w", err)
	}
	return items, nil
}

// Update mengubah value setting yang SUDAH ADA. Sengaja bukan upsert: key baru
// harus lewat migration (seed + aturan validasi di service), supaya API admin
// tidak bisa bikin key liar yang tidak dibaca siapa-siapa.
// Return ErrNotFound kalau key belum terdaftar.
func (r *Repository) Update(ctx context.Context, key, value string, actorID *uuid.UUID, at time.Time) error {
	res := r.db.WithContext(ctx).
		Model(&model.AppSetting{}).
		Where("key = ?", key).
		Updates(map[string]any{
			"value":      value,
			"updated_at": at,
			"updated_by": actorID,
		})
	if res.Error != nil {
		return fmt.Errorf("update setting %q: %w", key, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
