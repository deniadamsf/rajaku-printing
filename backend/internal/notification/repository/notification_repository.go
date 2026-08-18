// Package repository — DB access untuk notification module.
package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/rajaku-printing/backend/internal/notification/model"
)

var (
	ErrNotFound      = errors.New("notification/repository: not found")
	ErrDedupConflict = errors.New("notification/repository: dedup_key already exists")
	ErrJobStale      = errors.New("notification/repository: job status changed under update")
)

const pgUniqueViolationCode = "23505"

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

// Create inserts a job. Returns ErrDedupConflict when dedup_key unique index
// is violated — caller (service.Enqueue) treats as idempotent no-op.
func (r *Repository) Create(ctx context.Context, j *model.NotificationJob) error {
	if err := r.db.WithContext(ctx).Create(j).Error; err != nil {
		if isUniqueViolation(err) {
			return ErrDedupConflict
		}
		return fmt.Errorf("insert notification_job: %w", err)
	}
	return nil
}

// ClaimBatch selects up to `limit` jobs eligible for dispatch and atomically
// marks them `sending`, incrementing attempts + updated_at. Uses SKIP LOCKED
// so multiple worker instances (future) don't fight over the same rows.
//
// Eligibility: status IN ('pending','failed') AND next_attempt_at <= NOW().
// Note: `failed` rows are the transient-retry queue — worker will retry until
// attempts reaches max_attempts, at which point CompleteFailure moves them to
// `dead` (no more auto-retry).
func (r *Repository) ClaimBatch(ctx context.Context, limit int) ([]model.NotificationJob, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var jobs []model.NotificationJob
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.
			Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			// message <> '' — second-layer guard (review finding #4): a job
			// whose message was redacted by RedactStaleSensitive should
			// already have been moved out of pending/failed into `dead` by
			// that same sweep, so this should never match in practice. Kept
			// as a belt-and-suspenders check anyway so an empty-message job
			// can NEVER be claimed and sent, even if some future code path
			// redacts a message without also moving the status.
			Where("status IN ? AND next_attempt_at <= ? AND message <> ?",
				[]model.JobStatus{model.JobPending, model.JobFailed}, time.Now().UTC(), "").
			Order("next_attempt_at ASC").
			Limit(limit).
			Find(&jobs).Error; err != nil {
			return fmt.Errorf("select claimable: %w", err)
		}
		if len(jobs) == 0 {
			return nil
		}
		ids := make([]uuid.UUID, len(jobs))
		for i, j := range jobs {
			ids[i] = j.ID
		}
		res := tx.Model(&model.NotificationJob{}).
			Where("id IN ?", ids).
			Updates(map[string]any{
				"status":     model.JobSending,
				"attempts":   gorm.Expr("attempts + 1"),
				"updated_at": time.Now().UTC(),
			})
		if res.Error != nil {
			return fmt.Errorf("mark sending: %w", res.Error)
		}
		// Reflect changes locally for caller.
		for i := range jobs {
			jobs[i].Status = model.JobSending
			jobs[i].Attempts++
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return jobs, nil
}

// MarkSent finalizes a job. Idempotent: if the row is already sent, returns
// ErrJobStale so caller can log & ignore.
//
// Also redacts `message` INLINE, in the same statement, for jobs flagged
// is_sensitive (review finding #3) — a terminal "sent" job never needs its
// plaintext message again (the worker already sent it), so there's no reason
// to keep a WhatsApp OTP code sitting in the database once it's served its
// purpose. The CASE WHEN keeps non-sensitive jobs untouched.
func (r *Repository) MarkSent(ctx context.Context, id uuid.UUID) error {
	now := time.Now().UTC()
	res := r.db.WithContext(ctx).
		Model(&model.NotificationJob{}).
		Where("id = ? AND status = ?", id, model.JobSending).
		Updates(map[string]any{
			"status":     model.JobSent,
			"sent_at":    now,
			"updated_at": now,
			"message":    gorm.Expr("CASE WHEN is_sensitive THEN '' ELSE message END"),
		})
	if res.Error != nil {
		return fmt.Errorf("mark sent: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrJobStale
	}
	return nil
}

// MarkFailure records an error + reschedules per backoff. If attempts ≥
// max_attempts the job is moved to `dead` and won't be auto-claimed again
// (admin can manually re-enqueue).
//
// When a job lands on `dead` it has exhausted retries — no future send will
// ever consume `message` again — so, same as MarkSent, a sensitive job's
// message is redacted inline in that same UPDATE (review finding #3). A job
// that only moved to `failed` (still retryable) keeps its message; it's not
// yet at rest in the sense finding #3 cares about.
func (r *Repository) MarkFailure(ctx context.Context, id uuid.UUID, errMsg string, backoff time.Duration) error {
	// We need current attempts + max_attempts to decide.
	var current model.NotificationJob
	if err := r.db.WithContext(ctx).
		Select("id", "attempts", "max_attempts").
		First(&current, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("load job for failure: %w", err)
	}

	now := time.Now().UTC()
	next := now.Add(backoff)
	newStatus := model.JobFailed
	if current.Attempts >= current.MaxAttempts {
		newStatus = model.JobDead
	}
	// Cap error message so a huge stack trace doesn't blow the row.
	if len(errMsg) > 2000 {
		errMsg = errMsg[:2000] + "…(truncated)"
	}
	updates := map[string]any{
		"status":          newStatus,
		"last_error":      errMsg,
		"next_attempt_at": next,
		"updated_at":      now,
	}
	if newStatus == model.JobDead {
		updates["message"] = gorm.Expr("CASE WHEN is_sensitive THEN '' ELSE message END")
	}
	res := r.db.WithContext(ctx).
		Model(&model.NotificationJob{}).
		Where("id = ? AND status = ?", id, model.JobSending).
		Updates(updates)
	if res.Error != nil {
		return fmt.Errorf("mark failure: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrJobStale
	}
	return nil
}

// RedactStaleSensitive clears `message` for every is_sensitive job created
// before `olderThan`, REGARDLESS of status — a safety net for jobs that
// never reach a terminal status through the normal MarkSent/MarkFailure path
// (mis. worker down for a long stretch, job stuck pending/sending) — review
// finding #3. Returns how many rows were redacted, for logging.
//
// Review finding #4: clearing `message` alone is NOT enough for a job that's
// still in a non-terminal status (pending/sending/failed) — ClaimBatch's
// WHERE (status IN ('pending','failed') AND next_attempt_at <= NOW()) still
// selects it. If the worker that stalled long enough to trigger this sweep
// comes back online afterward, it would claim the now-EMPTY message, "send"
// it via Baileys, and MarkSent would record that as a successful delivery —
// i.e. an empty WhatsApp message sent to a real customer, logged as sent.
// So in the SAME statement, every non-terminal row swept here is ALSO moved
// to `dead` (no further auto-retry; matches what MarkFailure already does
// once a job exhausts its retries) — this makes it fall out of ClaimBatch's
// eligibility outright, not just rely on the message<>” guard added there
// as a second layer. Terminal rows (sent/dead) are left in their existing
// status; only `message` is touched for them, same as before.
func (r *Repository) RedactStaleSensitive(ctx context.Context, olderThan time.Time) (int64, error) {
	now := time.Now().UTC()
	res := r.db.WithContext(ctx).
		Model(&model.NotificationJob{}).
		Where("is_sensitive = TRUE AND message <> '' AND created_at < ?", olderThan).
		Updates(map[string]any{
			"message":    "",
			"updated_at": now,
			"status": gorm.Expr(
				"CASE WHEN status IN (?, ?, ?) THEN ? ELSE status END",
				model.JobPending, model.JobSending, model.JobFailed, model.JobDead,
			),
			"last_error": gorm.Expr(
				"CASE WHEN status IN (?, ?, ?) THEN ? ELSE last_error END",
				model.JobPending, model.JobSending, model.JobFailed,
				"redacted by stale-sensitive sweep before delivery (TTL exceeded); moved to dead so it can never be claimed and sent with an empty message",
			),
		})
	if res.Error != nil {
		return 0, fmt.Errorf("redact stale sensitive notification_jobs: %w", res.Error)
	}
	return res.RowsAffected, nil
}

// FindByID — for admin/debug inspection.
func (r *Repository) FindByID(ctx context.Context, id uuid.UUID) (*model.NotificationJob, error) {
	var j model.NotificationJob
	if err := r.db.WithContext(ctx).First(&j, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find job by id %s: %w", id, err)
	}
	return &j, nil
}

// ListFilter — admin inspection filter.
type ListFilter struct {
	Status   model.JobStatus
	Kind     string
	OrderID  *uuid.UUID
	Page     int
	PageSize int
}

type ListResult struct {
	Items    []model.NotificationJob
	Total    int64
	Page     int
	PageSize int
}

func (r *Repository) List(ctx context.Context, f ListFilter) (*ListResult, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 || f.PageSize > 100 {
		f.PageSize = 20
	}
	q := r.db.WithContext(ctx).Model(&model.NotificationJob{})
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.Kind != "" {
		q = q.Where("kind = ?", f.Kind)
	}
	if f.OrderID != nil {
		q = q.Where("order_id = ?", *f.OrderID)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("count jobs: %w", err)
	}
	var items []model.NotificationJob
	if err := q.
		Order("created_at DESC").
		Offset((f.Page - 1) * f.PageSize).
		Limit(f.PageSize).
		Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	return &ListResult{Items: items, Total: total, Page: f.Page, PageSize: f.PageSize}, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == pgUniqueViolationCode
	}
	return false
}
