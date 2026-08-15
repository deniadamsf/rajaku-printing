package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/rajaku-printing/backend/internal/auth/model"
)

// OAuthLoginCodeRepository persists the one-time handoff codes used by the
// Google OAuth flow (see migration 000012 for the "why").
type OAuthLoginCodeRepository struct {
	db *gorm.DB
}

func NewOAuthLoginCodeRepository(db *gorm.DB) *OAuthLoginCodeRepository {
	return &OAuthLoginCodeRepository{db: db}
}

// Create inserts a new handoff code. As a side effect it opportunistically
// deletes rows that expired more than 24h ago — this table is small and
// short-lived enough that a dedicated cron job would be overkill; piggy-
// backing housekeeping onto every insert keeps it bounded without extra
// scheduling infrastructure.
//
// The housekeeping delete is best-effort: it runs AFTER the INSERT has
// already succeeded, so a failure here (mis. a transient DB hiccup) must
// NEVER fail the whole Create call — the caller's login/registration just
// succeeded and has a valid code in hand. Log and move on (review finding
// #8 — previously this returned an error here, which turned a successful
// login into a 500 "server_error" redirect for the user).
func (r *OAuthLoginCodeRepository) Create(ctx context.Context, c *model.OAuthLoginCode) error {
	if err := r.db.WithContext(ctx).Create(c).Error; err != nil {
		return fmt.Errorf("insert oauth_login_code: %w", err)
	}
	if err := r.db.WithContext(ctx).
		Where("expires_at < ?", time.Now().UTC().Add(-24*time.Hour)).
		Delete(&model.OAuthLoginCode{}).Error; err != nil {
		log.Ctx(ctx).Warn().Err(err).Msg("oauth_login_codes housekeeping delete failed (non-fatal)")
	}
	return nil
}

// FindActiveByCode returns the row matching codeHash IF it has not been used
// and has not expired. "not found", "already used", and "expired" all map to
// the SAME ErrNotFound sentinel — the service layer doesn't need to
// distinguish them, it just reports the handoff code as invalid either way.
func (r *OAuthLoginCodeRepository) FindActiveByCode(ctx context.Context, codeHash string) (*model.OAuthLoginCode, error) {
	var c model.OAuthLoginCode
	err := r.db.WithContext(ctx).
		Where("code_hash = ? AND used_at IS NULL AND expires_at > ?", codeHash, time.Now().UTC()).
		First(&c).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find oauth_login_code: %w", err)
	}
	return &c, nil
}

// MarkUsed atomically claims the code: only succeeds if used_at is still
// NULL (single WHERE ... AND used_at IS NULL, review finding #4). Without
// the AND clause, two concurrent requests replaying the same code could both
// pass FindActiveByCode, both call MarkUsed, and both succeed — turning one
// code into two access tokens (or, in Complete's case, two attempts at
// creating a user from the same registration handoff). RowsAffected == 0 now
// covers "not found" AND "already used by a concurrent request" — caller
// treats both as ErrOAuthCodeInvalid.
func (r *OAuthLoginCodeRepository) MarkUsed(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).
		Model(&model.OAuthLoginCode{}).
		Where("id = ? AND used_at IS NULL", id).
		UpdateColumn("used_at", gorm.Expr("NOW()"))
	if res.Error != nil {
		return fmt.Errorf("mark oauth_login_code used: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteExpired removes rows whose expires_at is before `olderThan`. Exposed
// for a future scheduled cleanup job; Create() above already keeps the table
// bounded in the meantime.
func (r *OAuthLoginCodeRepository) DeleteExpired(ctx context.Context, olderThan time.Time) (int64, error) {
	res := r.db.WithContext(ctx).
		Where("expires_at < ?", olderThan).
		Delete(&model.OAuthLoginCode{})
	if res.Error != nil {
		return 0, fmt.Errorf("delete expired oauth_login_codes: %w", res.Error)
	}
	return res.RowsAffected, nil
}
