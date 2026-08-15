package repository

import (
	"context"
	"database/sql"
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

// ErrAttemptsExceeded — returned by IncrementAttemptsIfAllowed when the gated
// UPDATE matched no row because `attempts` was already >= the caller's
// maxAttempts at the moment the statement ran (never a stale in-memory read —
// see that method's doc).
var ErrAttemptsExceeded = errors.New("repository: phone_verification attempts exceeded")

// IncrementAttemptsIfAllowed atomically bumps attempts by 1 AND enforces
// maxAttempts as part of the SAME statement (review finding #1 — the
// pre-fix code read `pv.Attempts` from a plain SELECT done moments earlier,
// then incremented separately; under enough concurrent requests every one of
// them could read a stale, still-low attempts count and pass the gate,
// letting the 5-attempt ceiling be bypassed entirely). The WHERE clause
// below IS the gate: `attempts < maxAttempts` is evaluated by Postgres at
// UPDATE time under the row's lock, so two concurrent callers can never both
// succeed past the ceiling.
//
// Returns the NEW attempts value on success, or ErrAttemptsExceeded if the
// row was already at/over the ceiling (RowsAffected == 0). Callers MUST call
// this before comparing the guessed code against the hash — every call that
// reaches this point counts as an attempt, whether the code turns out right
// or wrong (that's what makes the ceiling meaningful).
func (r *PhoneVerificationRepository) IncrementAttemptsIfAllowed(ctx context.Context, id uuid.UUID, maxAttempts int) (int, error) {
	res := r.db.WithContext(ctx).
		Model(&model.PhoneVerification{}).
		Where("id = ? AND attempts < ?", id, maxAttempts).
		UpdateColumn("attempts", gorm.Expr("attempts + 1"))
	if res.Error != nil {
		return 0, fmt.Errorf("increment phone_verification attempts (gated): %w", res.Error)
	}
	if res.RowsAffected == 0 {
		// Either the row doesn't exist (shouldn't happen — caller just loaded
		// it via FindActive) or attempts was already >= maxAttempts. Either
		// way the caller wants the same outcome: refuse this attempt.
		return 0, ErrAttemptsExceeded
	}
	// Read-after-write for reporting ONLY (mis. "attempts_left" in the error
	// response) — the gate itself already happened, atomically, in the
	// UPDATE above; this SELECT cannot reopen the race it closed.
	var pv model.PhoneVerification
	if err := r.db.WithContext(ctx).Select("attempts").First(&pv, "id = ?", id).Error; err != nil {
		return 0, fmt.Errorf("reload phone_verification attempts: %w", err)
	}
	return pv.Attempts, nil
}

// CountAndOldestSince returns how many phone_verification challenges were
// created for `phone` after `since`, and the created_at of the OLDEST one in
// that window (nil if count == 0) — spans ALL registration handoffs for the
// number, not just one (review finding #2: without this, an attacker could
// open unlimited handoffs, each getting its own per-handoff cooldown, to
// spam OTP challenges — i.e. WhatsApp messages — at one victim's number).
// The oldest timestamp lets the caller report a meaningful
// "resend_available_in" once the cap is hit (the cap clears exactly one hour
// after that row ages out of the window).
func (r *PhoneVerificationRepository) CountAndOldestSince(ctx context.Context, phone string, since time.Time) (int64, *time.Time, error) {
	var row struct {
		Count  int64
		Oldest sql.NullTime
	}
	err := r.db.WithContext(ctx).
		Model(&model.PhoneVerification{}).
		Select("COUNT(*) AS count, MIN(created_at) AS oldest").
		Where("phone = ? AND created_at > ?", phone, since).
		Scan(&row).Error
	if err != nil {
		return 0, nil, fmt.Errorf("count phone_verifications since %s for phone: %w", since, err)
	}
	if !row.Oldest.Valid {
		return row.Count, nil, nil
	}
	oldest := row.Oldest.Time
	return row.Count, &oldest, nil
}

// SumAttemptsSince returns the total `attempts` recorded across every
// phone_verification challenge for `phone` created after `since` — an
// anti-bruteforce cap spanning MULTIPLE registration handoffs for the same
// number (review finding #2: without this, an attacker could open several
// handoffs for the same victim phone, each with its own full MaxAttempts
// budget, to keep guessing far past what one handoff alone allows). Counts
// every verify call logged in the window (right or wrong — attempts is
// incremented unconditionally per IncrementAttemptsIfAllowed's doc), which
// is a slightly stricter proxy for "failed attempts" but never
// under-counts abuse.
func (r *PhoneVerificationRepository) SumAttemptsSince(ctx context.Context, phone string, since time.Time) (int64, error) {
	var total sql.NullInt64
	err := r.db.WithContext(ctx).
		Model(&model.PhoneVerification{}).
		Select("COALESCE(SUM(attempts), 0)").
		Where("phone = ? AND created_at > ?", phone, since).
		Scan(&total).Error
	if err != nil {
		return 0, fmt.Errorf("sum phone_verification attempts since %s for phone: %w", since, err)
	}
	return total.Int64, nil
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
