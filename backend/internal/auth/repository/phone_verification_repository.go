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

// FindActiveByUser / FindLatestByUser / CancelPendingForUser — user_id-keyed
// counterparts of FindActive / FindLatest / CancelPendingForHandoff above
// (migration 000017), used by an ALREADY-authenticated user proving ownership
// of a phone number they're adding/changing on their own account rather than
// a Google OAuth registration handoff. Same semantics, same
// "never requested"/"consumed"/"expired" → ErrNotFound collapsing.

func (r *PhoneVerificationRepository) FindActiveByUser(ctx context.Context, userID uuid.UUID, phone string) (*model.PhoneVerification, error) {
	var pv model.PhoneVerification
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND phone = ? AND consumed_at IS NULL AND expires_at > ?",
			userID, phone, time.Now().UTC()).
		Order("created_at DESC").
		First(&pv).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find active phone_verification for user: %w", err)
	}
	return &pv, nil
}

func (r *PhoneVerificationRepository) FindLatestByUser(ctx context.Context, userID uuid.UUID, phone string) (*model.PhoneVerification, error) {
	var pv model.PhoneVerification
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND phone = ?", userID, phone).
		Order("created_at DESC").
		First(&pv).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find latest phone_verification for user: %w", err)
	}
	return &pv, nil
}

func (r *PhoneVerificationRepository) CancelPendingForUser(ctx context.Context, userID uuid.UUID) error {
	if err := r.db.WithContext(ctx).
		Model(&model.PhoneVerification{}).
		Where("user_id = ? AND consumed_at IS NULL", userID).
		UpdateColumn("consumed_at", gorm.Expr("NOW()")).Error; err != nil {
		return fmt.Errorf("cancel pending phone_verifications for user: %w", err)
	}
	return nil
}

// ErrAttemptsExceeded — returned by IncrementAttemptsIfAllowed when the gated
// UPDATE matched no row SPECIFICALLY because `attempts` was already >= the
// caller's maxAttempts on an otherwise-active row at the moment the statement
// ran (never a stale in-memory read — see that method's doc). Distinguished
// from ErrNotFound (review finding #5a) — see classifyIncrementMiss.
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
// The WHERE clause also requires `consumed_at IS NULL AND expires_at > now()`
// (review finding #5b) — mirroring exactly what FindActive uses to select the
// row in the first place. Without this, a challenge that a CONCURRENT
// CancelPendingForHandoff consumed (or that expired) in the window between
// the caller's FindActive and this call would still get its attempts counter
// incremented and compared against — this statement is documented as THE
// gate for that scenario, so it needs to be a complete gate, not a partial
// one relying on callers to have re-checked freshness themselves.
//
// Returns the NEW attempts value on success. On failure (RowsAffected == 0)
// returns one of two DISTINCT sentinels (review finding #5a — the pre-fix
// code collapsed every miss into ErrAttemptsExceeded, which made an
// already-consumed/expired/nonexistent row get misreported to the end user
// as "too many attempts" even though they hadn't made any):
//   - ErrNotFound — the row doesn't exist, or exists but is no longer active
//     (consumed or expired). Callers should treat this the same as
//     FindActive's ErrNotFound (mis. verifyOTP maps both to ErrOTPExpired).
//   - ErrAttemptsExceeded — the row IS active, but attempts was already at
//     the ceiling. This is the actual "too many attempts" case.
//
// Callers MUST call this before comparing the guessed code against the hash —
// every call that reaches this point counts as an attempt, whether the code
// turns out right or wrong (that's what makes the ceiling meaningful).
func (r *PhoneVerificationRepository) IncrementAttemptsIfAllowed(ctx context.Context, id uuid.UUID, maxAttempts int) (int, error) {
	now := time.Now().UTC()
	res := r.db.WithContext(ctx).
		Model(&model.PhoneVerification{}).
		Where("id = ? AND attempts < ? AND consumed_at IS NULL AND expires_at > ?", id, maxAttempts, now).
		UpdateColumn("attempts", gorm.Expr("attempts + 1"))
	if res.Error != nil {
		return 0, fmt.Errorf("increment phone_verification attempts (gated): %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return 0, classifyIncrementMiss(ctx, r.db, id, maxAttempts, now)
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

// classifyIncrementMiss runs a best-effort follow-up SELECT — the atomicity
// guarantee already happened in the gated UPDATE above, this exists purely to
// give the caller (and logs/incident review) an ACCURATE reason the UPDATE
// matched nothing (review finding #5a), instead of collapsing every miss into
// ErrAttemptsExceeded. The row can still change between the UPDATE and this
// SELECT under concurrent writers; that's fine — worst case this reports a
// slightly stale reason for what is, either way, already a rejected attempt.
func classifyIncrementMiss(ctx context.Context, db *gorm.DB, id uuid.UUID, maxAttempts int, now time.Time) error {
	var pv model.PhoneVerification
	err := db.WithContext(ctx).First(&pv, "id = ?", id).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return ErrNotFound
	case err != nil:
		return fmt.Errorf("classify increment miss: reload phone_verification: %w", err)
	case pv.ConsumedAt != nil || !pv.ExpiresAt.After(now):
		// Row exists but is no longer active — same "not usable" outcome as
		// FindActive returning ErrNotFound.
		return ErrNotFound
	case pv.Attempts >= maxAttempts:
		return ErrAttemptsExceeded
	default:
		// Row is active and under the ceiling — the UPDATE must have missed
		// for a reason not modeled above (mis. changed again between the
		// UPDATE and this SELECT). Report the ceiling case as the safest
		// default: it never claims fewer attempts remain than reality.
		return ErrAttemptsExceeded
	}
}

// LockPhone acquires a TRANSACTION-SCOPED Postgres advisory lock keyed on
// `phone` (pg_advisory_xact_lock(hashtext(phone))) — serializes concurrent
// per-phone rate-limit check-then-mutate sequences (review finding #1: a
// plain COUNT/SUM SELECT followed by a separate INSERT/UPDATE is racy even at
// READ COMMITTED, because concurrent transactions don't see each other's
// uncommitted writes — an `INSERT ... SELECT COUNT(*)` pattern does NOT close
// this the way a gated `UPDATE ... WHERE` can for a row that already exists).
//
// MUST be called from inside an active transaction (`*gorm.DB` bound to a
// tx, e.g. via db.Transaction(...)) — pg_advisory_xact_lock is released
// automatically at COMMIT/ROLLBACK of that transaction, never held past it,
// so there is no leak risk even on panic. Called from
// service.gormPhoneLockTxRunner — this is the ONLY place in the codebase
// that should invoke it (service code must not embed raw SQL, §22).
func (r *PhoneVerificationRepository) LockPhone(ctx context.Context, phone string) error {
	if err := r.db.WithContext(ctx).Exec("SELECT pg_advisory_xact_lock(hashtext(?))", phone).Error; err != nil {
		return fmt.Errorf("pg_advisory_xact_lock for phone: %w", err)
	}
	return nil
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
