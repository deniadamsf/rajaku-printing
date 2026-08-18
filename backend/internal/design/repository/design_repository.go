// Package repository — DB access untuk design module.
package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/rajaku-printing/backend/internal/design/model"
)

var (
	ErrNotFound          = errors.New("design/repository: not found")
	ErrPendingDraftExists = errors.New("design/repository: another pending staff_draft exists for this order")
	ErrDraftAlreadyReviewed = errors.New("design/repository: staff_draft already reviewed (status not pending)")
)

const pgUniqueViolationCode = "23505"

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

// Create inserts a design_file row. Returns ErrPendingDraftExists when the
// partial-unique index (one pending staff_draft per order) is violated.
func (r *Repository) Create(ctx context.Context, f *model.DesignFile) error {
	if err := r.db.WithContext(ctx).Create(f).Error; err != nil {
		if isUniqueViolation(err) {
			return ErrPendingDraftExists
		}
		return fmt.Errorf("insert design_file: %w", err)
	}
	return nil
}

// FindByID returns the file or ErrNotFound.
func (r *Repository) FindByID(ctx context.Context, id uuid.UUID) (*model.DesignFile, error) {
	var f model.DesignFile
	err := r.db.WithContext(ctx).First(&f, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find design_file by id %s: %w", id, err)
	}
	return &f, nil
}

// FindPendingDraftByOrder returns the current pending staff_draft for an order
// (there can be at most one) or ErrNotFound.
func (r *Repository) FindPendingDraftByOrder(ctx context.Context, orderID uuid.UUID) (*model.DesignFile, error) {
	var f model.DesignFile
	err := r.db.WithContext(ctx).
		Where("order_id = ? AND role = ? AND approval_status = ?",
			orderID, model.RoleStaffDraft, model.ApprovalPending).
		First(&f).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find pending draft: %w", err)
	}
	return &f, nil
}

// ListByOrder returns all design files for an order, newest first. Dipakai
// customer (lihat riwayat) dan staff (lihat semua iterasi).
func (r *Repository) ListByOrder(ctx context.Context, orderID uuid.UUID) ([]model.DesignFile, error) {
	var items []model.DesignFile
	err := r.db.WithContext(ctx).
		Where("order_id = ?", orderID).
		Order("uploaded_at DESC").
		Find(&items).Error
	if err != nil {
		return nil, fmt.Errorf("list design files for order %s: %w", orderID, err)
	}
	return items, nil
}

// ReviewDraftParams — payload untuk customer approve/revision.
type ReviewDraftParams struct {
	ID             uuid.UUID
	NewStatus      model.ApprovalStatus // approved | revision_requested
	ReviewedBy     uuid.UUID
	ReviewedAt     time.Time
	RevisionNotes  string // required kalau revision_requested
}

// ReviewDraft atomically transitions a pending draft. Returns
// ErrDraftAlreadyReviewed kalau tidak lagi pending.
func (r *Repository) ReviewDraft(ctx context.Context, p ReviewDraftParams) error {
	updates := map[string]any{
		"approval_status": p.NewStatus,
		"reviewed_at":     p.ReviewedAt,
		"reviewed_by":     p.ReviewedBy,
	}
	if p.NewStatus == model.ApprovalRevision {
		updates["revision_notes"] = p.RevisionNotes
	}
	res := r.db.WithContext(ctx).
		Model(&model.DesignFile{}).
		Where("id = ? AND role = ? AND approval_status = ?",
			p.ID, model.RoleStaffDraft, model.ApprovalPending).
		Updates(updates)
	if res.Error != nil {
		return fmt.Errorf("review draft: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		// Disambiguate: does the row exist at all?
		var count int64
		_ = r.db.WithContext(ctx).Model(&model.DesignFile{}).
			Where("id = ?", p.ID).Count(&count).Error
		if count == 0 {
			return ErrNotFound
		}
		return ErrDraftAlreadyReviewed
	}
	return nil
}

// MarkPurged menandai row purged & kosongkan file_path (§19). Idempotent —
// row yg sudah purged tetap return nil.
func (r *Repository) MarkPurged(ctx context.Context, id uuid.UUID, at time.Time) error {
	res := r.db.WithContext(ctx).
		Model(&model.DesignFile{}).
		Where("id = ? AND is_purged = false", id).
		Updates(map[string]any{
			"is_purged": true,
			"purged_at": at,
			"file_path": "",
		})
	if res.Error != nil {
		return fmt.Errorf("mark purged: %w", res.Error)
	}
	return nil
}

// ListPurgeCandidates mengembalikan file yang blob-nya sudah lewat masa
// retensi (uploaded_at < cutoff) dan belum di-purge. Dibatasi `limit` supaya
// satu run job tidak menahan koneksi DB lama / menghapus ribuan file sekaligus
// — sisa antrean diproses run berikutnya.
//
// Urut uploaded_at ASC: yang paling tua dibersihkan duluan (kalau ada backlog,
// disk yang paling lama nganggur yang dibebaskan dulu).
func (r *Repository) ListPurgeCandidates(ctx context.Context, cutoff time.Time, limit int) ([]model.DesignFile, error) {
	var items []model.DesignFile
	err := r.db.WithContext(ctx).
		Where("is_purged = false AND uploaded_at < ?", cutoff).
		Order("uploaded_at ASC").
		Limit(limit).
		Find(&items).Error
	if err != nil {
		return nil, fmt.Errorf("list purge candidates before %s: %w", cutoff.Format(time.RFC3339), err)
	}
	return items, nil
}

// ListReminderCandidates mengembalikan file yang akan kena purge dalam waktu
// dekat (uploaded_at antara purgeCutoff dan reminderCutoff) dan belum pernah
// diingatkan.
//
// Rentangnya setengah terbuka: uploaded_at >= purgeCutoff (belum lewat masa
// retensi — yang sudah lewat urusannya sweep, bukan reminder) DAN
// uploaded_at < reminderCutoff (sudah masuk jendela H-N).
func (r *Repository) ListReminderCandidates(ctx context.Context, purgeCutoff, reminderCutoff time.Time, limit int) ([]model.DesignFile, error) {
	var items []model.DesignFile
	err := r.db.WithContext(ctx).
		Where(`is_purged = false
		       AND retention_reminder_at IS NULL
		       AND uploaded_at >= ?
		       AND uploaded_at < ?`, purgeCutoff, reminderCutoff).
		Order("uploaded_at ASC").
		Limit(limit).
		Find(&items).Error
	if err != nil {
		return nil, fmt.Errorf("list reminder candidates in [%s, %s): %w",
			purgeCutoff.Format(time.RFC3339), reminderCutoff.Format(time.RFC3339), err)
	}
	return items, nil
}

// MarkReminderSent menandai reminder retensi sudah dikirim untuk file ini.
// Guard `retention_reminder_at IS NULL` bikin ini aman kalau dua run job
// kebetulan overlap — yang kedua tidak akan menimpa timestamp pertama.
func (r *Repository) MarkReminderSent(ctx context.Context, id uuid.UUID, at time.Time) error {
	res := r.db.WithContext(ctx).
		Model(&model.DesignFile{}).
		Where("id = ? AND retention_reminder_at IS NULL", id).
		Update("retention_reminder_at", at)
	if res.Error != nil {
		return fmt.Errorf("mark retention reminder sent for %s: %w", id, res.Error)
	}
	return nil
}

// DeleteRow hard-delete. Dipakai upload-flow rollback saat transisi order gagal.
func (r *Repository) DeleteRow(ctx context.Context, id uuid.UUID) error {
	if err := r.db.WithContext(ctx).Delete(&model.DesignFile{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("delete design_file %s: %w", id, err)
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == pgUniqueViolationCode
	}
	return false
}
