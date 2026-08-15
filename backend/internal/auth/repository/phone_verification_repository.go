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

// PhoneVerificationRepository persists WhatsApp OTP challenges for the
// Google OAuth registration flow (migration 000013).
type PhoneVerificationRepository struct {
	db *gorm.DB
}

func NewPhoneVerificationRepository(db *gorm.DB) *PhoneVerificationRepository {
	return &PhoneVerificationRepository{db: db}
}

// Create inserts a new OTP challenge.
func (r *PhoneVerificationRepository) Create(ctx context.Context, pv *model.PhoneVerification) error {
	if err := r.db.WithContext(ctx).Create(pv).Error; err != nil {
		return fmt.Errorf("insert phone_verification: %w", err)
	}
	return nil
}

// FindActive returns the most recent NOT-consumed, NOT-expired OTP challenge
// for (handoffID, phone). "never requested", "already consumed", and
// "expired" all map to the SAME ErrNotFound — caller doesn't need to
// distinguish (mirrors OAuthLoginCodeRepository.FindActiveByCode).
func (r *PhoneVerificationRepository) FindActive(ctx context.Context, handoffID uuid.UUID, phone string) (*model.PhoneVerification, error) {
	var pv model.PhoneVerification
	err := r.db.WithContext(ctx).
		Where("oauth_login_code_id = ? AND phone = ? AND consumed_at IS NULL AND expires_at > ?",
			handoffID, phone, time.Now().UTC()).
		Order("created_at DESC").
		First(&pv).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find active phone_verification: %w", err)
	}
	return &pv, nil
}

// FindLatest returns the most recently created challenge for (handoffID,
// phone), regardless of status — used only to compute the resend cooldown
// window (a consumed/expired challenge still "counts" for cooldown purposes).
func (r *PhoneVerificationRepository) FindLatest(ctx context.Context, handoffID uuid.UUID, phone string) (*model.PhoneVerification, error) {
	var pv model.PhoneVerification
	err := r.db.WithContext(ctx).
		Where("oauth_login_code_id = ? AND phone = ?", handoffID, phone).
		Order("created_at DESC").
		First(&pv).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find latest phone_verification: %w", err)
	}
	return &pv, nil
}

// CancelPendingForHandoff marks every not-yet-consumed OTP challenge under
// this handoff as consumed — called right before issuing a fresh code so at
// most one challenge is ever active per handoff, even if the caller retried
// with a different (typo'd) phone number.
func (r *PhoneVerificationRepository) CancelPendingForHandoff(ctx context.Context, handoffID uuid.UUID) error {
	if err := r.db.WithContext(ctx).
		Model(&model.PhoneVerification{}).
		Where("oauth_login_code_id = ? AND consumed_at IS NULL", handoffID).
		UpdateColumn("consumed_at", gorm.Expr("NOW()")).Error; err != nil {
		return fmt.Errorf("cancel pending phone_verifications: %w", err)
	}
	return nil
}

// IncrementAttempts atomically bumps attempts by 1 (DB-level `attempts =
// attempts + 1`, avoids a read-modify-write race) and returns the new value.
func (r *PhoneVerificationRepository) IncrementAttempts(ctx context.Context, id uuid.UUID) (int, error) {
	res := r.db.WithContext(ctx).
		Model(&model.PhoneVerification{}).
		Where("id = ?", id).
		UpdateColumn("attempts", gorm.Expr("attempts + 1"))
	if res.Error != nil {
		return 0, fmt.Errorf("increment phone_verification attempts: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return 0, ErrNotFound
	}
	var pv model.PhoneVerification
	if err := r.db.WithContext(ctx).Select("attempts").First(&pv, "id = ?", id).Error; err != nil {
		return 0, fmt.Errorf("reload phone_verification attempts: %w", err)
	}
	return pv.Attempts, nil
}

// MarkVerified sets verified_at = now(). Return ErrNotFound kalau row tidak ada.
func (r *PhoneVerificationRepository) MarkVerified(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).
		Model(&model.PhoneVerification{}).
		Where("id = ?", id).
		UpdateColumn("verified_at", gorm.Expr("NOW()"))
	if res.Error != nil {
		return fmt.Errorf("mark phone_verification verified: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkConsumed sets consumed_at = now(). Return ErrNotFound kalau row tidak ada.
func (r *PhoneVerificationRepository) MarkConsumed(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).
		Model(&model.PhoneVerification{}).
		Where("id = ?", id).
		UpdateColumn("consumed_at", gorm.Expr("NOW()"))
	if res.Error != nil {
		return fmt.Errorf("mark phone_verification consumed: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
