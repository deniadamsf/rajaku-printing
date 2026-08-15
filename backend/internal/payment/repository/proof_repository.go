// Package repository — DB access for payment module.
package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/rajaku-printing/backend/internal/payment/model"
)

var (
	ErrNotFound        = errors.New("payment/repository: not found")
	ErrPendingConflict = errors.New("payment/repository: another pending proof exists for this order")
	ErrAlreadyReviewed = errors.New("payment/repository: proof already reviewed (status not pending)")
)

const pgUniqueViolationCode = "23505"

type ProofRepository struct {
	db *gorm.DB
}

func NewProofRepository(db *gorm.DB) *ProofRepository { return &ProofRepository{db: db} }

// Create inserts a new proof row. Returns ErrPendingConflict when the DB's
// partial-unique index (one pending per order) is violated — telling caller
// they should not have gotten this far (race with another upload).
func (r *ProofRepository) Create(ctx context.Context, p *model.PaymentProof) error {
	if err := r.db.WithContext(ctx).Create(p).Error; err != nil {
		if isUniqueViolation(err) {
			return ErrPendingConflict
		}
		return fmt.Errorf("insert payment_proof: %w", err)
	}
	return nil
}

// FindByID returns the proof or ErrNotFound.
func (r *ProofRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.PaymentProof, error) {
	var p model.PaymentProof
	err := r.db.WithContext(ctx).First(&p, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find proof by id %s: %w", id, err)
	}
	return &p, nil
}

// FindPendingByOrder returns the current pending proof for an order (there can
// be at most one due to the partial unique index) or ErrNotFound.
func (r *ProofRepository) FindPendingByOrder(ctx context.Context, orderID uuid.UUID) (*model.PaymentProof, error) {
	var p model.PaymentProof
	err := r.db.WithContext(ctx).
		Where("order_id = ? AND status = ?", orderID, model.ProofPending).
		First(&p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find pending proof: %w", err)
	}
	return &p, nil
}

// ListFilter narrows list queries. Zero-values = no filter.
type ListFilter struct {
	Status   model.ProofStatus
	OrderID  *uuid.UUID
	Page     int
	PageSize int
}

type ListResult struct {
	Items    []model.PaymentProof
	Total    int64
	Page     int
	PageSize int
}

// List returns paginated proofs (default 20 per page, max 100). Ordered by
// uploaded_at DESC (newest first — matches staff dashboard usage).
func (r *ProofRepository) List(ctx context.Context, f ListFilter) (*ListResult, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 || f.PageSize > 100 {
		f.PageSize = 20
	}
	q := r.db.WithContext(ctx).Model(&model.PaymentProof{})
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.OrderID != nil {
		q = q.Where("order_id = ?", *f.OrderID)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("count proofs: %w", err)
	}
	var items []model.PaymentProof
	if err := q.
		Order("uploaded_at DESC").
		Offset((f.Page - 1) * f.PageSize).
		Limit(f.PageSize).
		Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list proofs: %w", err)
	}
	return &ListResult{Items: items, Total: total, Page: f.Page, PageSize: f.PageSize}, nil
}

// ListByOrder returns all proofs for one order, newest first. No pagination —
// unlike List (staff dashboard, all orders), a single order only ever
// accumulates a handful of proofs, so a bounded Count()+Offset()+Limit()
// query would be pure overhead. Dipakai oleh customer-facing
// GET /orders/:resi/payment-proofs.
func (r *ProofRepository) ListByOrder(ctx context.Context, orderID uuid.UUID) ([]model.PaymentProof, error) {
	var items []model.PaymentProof
	if err := r.db.WithContext(ctx).
		Where("order_id = ?", orderID).
		Order("uploaded_at DESC").
		Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list proofs by order %s: %w", orderID, err)
	}
	return items, nil
}

// ReviewParams — payload for approve/reject.
type ReviewParams struct {
	ID           uuid.UUID
	NewStatus    model.ProofStatus // approved | rejected
	ReviewedBy   uuid.UUID
	ReviewedAt   time.Time
	RejectReason string // required when NewStatus = rejected
}

// Review atomically transitions a pending proof to approved/rejected. Returns
// ErrAlreadyReviewed if the row is no longer pending (concurrent staff action).
func (r *ProofRepository) Review(ctx context.Context, p ReviewParams) error {
	updates := map[string]any{
		"status":      p.NewStatus,
		"reviewed_at": p.ReviewedAt,
		"reviewed_by": p.ReviewedBy,
	}
	if p.NewStatus == model.ProofRejected {
		updates["reject_reason"] = p.RejectReason
	}
	res := r.db.WithContext(ctx).
		Model(&model.PaymentProof{}).
		Where("id = ? AND status = ?", p.ID, model.ProofPending).
		Updates(updates)
	if res.Error != nil {
		return fmt.Errorf("review proof: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		// Either the row vanished or was already reviewed.
		// Cheap disambiguation: probe existence.
		var count int64
		_ = r.db.WithContext(ctx).Model(&model.PaymentProof{}).
			Where("id = ?", p.ID).Count(&count).Error
		if count == 0 {
			return ErrNotFound
		}
		return ErrAlreadyReviewed
	}
	return nil
}

// DeleteRow removes a proof row entirely — only used by upload-flow rollback
// when the downstream order transition fails. Not exposed via any handler.
func (r *ProofRepository) DeleteRow(ctx context.Context, id uuid.UUID) error {
	if err := r.db.WithContext(ctx).Delete(&model.PaymentProof{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("delete proof %s: %w", id, err)
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
