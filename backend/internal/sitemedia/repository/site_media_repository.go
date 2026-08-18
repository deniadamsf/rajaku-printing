// Package repository — akses DB modul sitemedia (GORM only, §22).
package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/rajaku-printing/backend/internal/sitemedia/model"
)

// ErrNotFound — slot tidak punya baris di tabel site_media (belum pernah
// diunggah, atau sudah dikosongkan).
var ErrNotFound = errors.New("sitemedia/repository: slot not found")

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

// Get returns baris site_media untuk satu slot, atau ErrNotFound.
func (r *Repository) Get(ctx context.Context, slot string) (*model.SiteMedia, error) {
	var row model.SiteMedia
	err := r.db.WithContext(ctx).First(&row, "slot = ?", slot).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get site media %q: %w", slot, err)
	}
	return &row, nil
}

// List returns semua slot yang SUDAH terisi, urut slot (stabil untuk UI admin).
func (r *Repository) List(ctx context.Context) ([]model.SiteMedia, error) {
	var items []model.SiteMedia
	if err := r.db.WithContext(ctx).Order("slot ASC").Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list site media: %w", err)
	}
	return items, nil
}

// Upsert menyimpan baris slot — insert kalau slot belum pernah diisi, replace
// kalau sudah ada (unggahan baru menimpa slot lama). Dipanggil SETELAH blob
// baru berhasil tersimpan di filestore (service yang menjaga urutan ini).
func (r *Repository) Upsert(ctx context.Context, row *model.SiteMedia) error {
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "slot"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"storage_path", "original_name", "mime_type", "size_bytes",
			"width_px", "height_px", "uploaded_by", "uploaded_at", "updated_at",
		}),
	}).Create(row).Error
	if err != nil {
		return fmt.Errorf("upsert site media %q: %w", row.Slot, err)
	}
	return nil
}

// Delete menghapus baris slot. Return ErrNotFound kalau slot memang belum
// pernah diisi.
func (r *Repository) Delete(ctx context.Context, slot string) error {
	res := r.db.WithContext(ctx).Delete(&model.SiteMedia{}, "slot = ?", slot)
	if res.Error != nil {
		return fmt.Errorf("delete site media %q: %w", slot, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
