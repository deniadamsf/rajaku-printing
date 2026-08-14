package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/rajaku-printing/backend/internal/auth/model"
)

type StaffInviteRepository struct {
	db *gorm.DB
}

func NewStaffInviteRepository(db *gorm.DB) *StaffInviteRepository {
	return &StaffInviteRepository{db: db}
}

// Create inserts a new invite. Return ErrNotFound sudah tidak relevan;
// unique conflict pada token_hash (probability nyaris 0 dengan 256-bit random)
// akan bubble sebagai plain error dari GORM.
func (r *StaffInviteRepository) Create(ctx context.Context, inv *model.StaffInvite) error {
	if err := r.db.WithContext(ctx).Create(inv).Error; err != nil {
		return fmt.Errorf("insert staff_invite: %w", err)
	}
	return nil
}

// FindByTokenHash returns the invite matching hash, atau ErrNotFound.
// Service akan hash raw token dari client dulu sebelum call.
func (r *StaffInviteRepository) FindByTokenHash(ctx context.Context, tokenHash string) (*model.StaffInvite, error) {
	var inv model.StaffInvite
	err := r.db.WithContext(ctx).Where("token_hash = ?", tokenHash).First(&inv).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find invite by token: %w", err)
	}
	return &inv, nil
}

// MarkUsed sets used_at = now(). Return ErrNotFound kalau row tidak ada.
// Idempotent: no-op kalau sudah used (row still matches WHERE tapi update
// tidak breaking).
func (r *StaffInviteRepository) MarkUsed(ctx context.Context, id uuid.UUID, at time.Time) error {
	res := r.db.WithContext(ctx).
		Model(&model.StaffInvite{}).
		Where("id = ?", id).
		UpdateColumn("used_at", at)
	if res.Error != nil {
		return fmt.Errorf("mark invite used: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteExpired cleanup helper. Return count of rows deleted. Dipanggil dari
// scheduled job (belum diimplementasi — TODO).
func (r *StaffInviteRepository) DeleteExpired(ctx context.Context, before time.Time) (int64, error) {
	res := r.db.WithContext(ctx).
		Where("expires_at < ? AND used_at IS NULL", before).
		Delete(&model.StaffInvite{})
	if res.Error != nil {
		return 0, fmt.Errorf("delete expired invites: %w", res.Error)
	}
	return res.RowsAffected, nil
}

// FindActiveByUserID — untuk cegah spam invite (opsional, MVP tidak dipakai).
func (r *StaffInviteRepository) FindActiveByUserID(ctx context.Context, userID uuid.UUID) (*model.StaffInvite, error) {
	var inv model.StaffInvite
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND used_at IS NULL AND expires_at > ?", userID, time.Now().UTC()).
		Order("created_at DESC").
		First(&inv).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find active invite: %w", err)
	}
	return &inv, nil
}
