// Package repository handles DB access for the discount module (§28).
package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/rajaku-printing/backend/internal/discount/model"
)

var (
	ErrNotFound = errors.New("discount/repository: not found")
	// ErrCodeConflict — unique index idx_discounts_code_active violated
	// (kode sudah dipakai diskon lain yang masih aktif/belum dihapus).
	ErrCodeConflict = errors.New("discount/repository: code unique conflict")
)

const pgUniqueViolationCode = "23505"

type DiscountRepository struct {
	db *gorm.DB
}

func NewDiscountRepository(db *gorm.DB) *DiscountRepository { return &DiscountRepository{db: db} }

// Create inserts a new discount row. Returns ErrCodeConflict specifically
// when the partial unique index on `code` is violated.
func (r *DiscountRepository) Create(ctx context.Context, d *model.Discount) error {
	if err := r.db.WithContext(ctx).Create(d).Error; err != nil {
		if isUniqueViolation(err) {
			return ErrCodeConflict
		}
		return fmt.Errorf("create discount: %w", err)
	}
	return nil
}

// FindByID returns the discount or ErrNotFound. Soft-deleted rows are
// treated as not found — matches order.repository.FindByID pattern.
func (r *DiscountRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.Discount, error) {
	var d model.Discount
	err := r.db.WithContext(ctx).Where("deleted_at IS NULL").First(&d, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find discount by id %s: %w", id, err)
	}
	return &d, nil
}

// List returns every non-deleted discount matching a free-text search on
// code/name (case-insensitive), newest first. No pagination here — dataset
// (jumlah program diskon sebuah toko) kecil, service yang menghitung status
// turunan (butuh usage count per row) & memaginasi di memori.
func (r *DiscountRepository) List(ctx context.Context, q string) ([]model.Discount, error) {
	query := r.db.WithContext(ctx).Where("deleted_at IS NULL")
	if q != "" {
		like := "%" + q + "%"
		query = query.Where("code ILIKE ? OR name ILIKE ?", like, like)
	}
	var items []model.Discount
	if err := query.Order("created_at DESC").Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list discounts: %w", err)
	}
	return items, nil
}

// ListActiveForChannel returns non-deleted, is_active, in-date-range,
// channel-compatible, min-subtotal-satisfied discounts — the DB-level
// prefilter for GET /admin/discounts/applicable. Quota (needs a usage count
// per row) is filtered afterwards by the service.
func (r *DiscountRepository) ListActiveForChannel(ctx context.Context, channel string, subtotal int64, now time.Time) ([]model.Discount, error) {
	var items []model.Discount
	err := r.db.WithContext(ctx).
		Where("deleted_at IS NULL").
		Where("is_active = true").
		Where("channel_scope = ? OR channel_scope = ?", model.ChannelScopeAll, channel).
		Where("min_subtotal <= ?", subtotal).
		Where("starts_at IS NULL OR starts_at <= ?", now).
		Where("ends_at IS NULL OR ends_at >= ?", now).
		Order("created_at DESC").
		Find(&items).Error
	if err != nil {
		return nil, fmt.Errorf("list active discounts for channel %q: %w", channel, err)
	}
	return items, nil
}

// CountUsage returns, for each discount id in `ids`, how many non-deleted
// orders reference it (§28.4 — kuota DIHITUNG dari orders, bukan disimpan
// sebagai counter). Raw SQL against the `orders` table — allowed per §28:
// this is a direct query to another module's TABLE, not an import of its Go
// package, which stays forbidden (§22).
func (r *DiscountRepository) CountUsage(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]int64, error) {
	out := make(map[uuid.UUID]int64, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var rows []struct {
		DiscountID uuid.UUID
		Count      int64
	}
	err := r.db.WithContext(ctx).
		Table("orders").
		Select("discount_id, COUNT(*) as count").
		Where("discount_id IN ? AND deleted_at IS NULL", ids).
		Group("discount_id").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("count discount usage: %w", err)
	}
	for _, row := range rows {
		out[row.DiscountID] = row.Count
	}
	return out, nil
}

// CountUsageOne — single-discount variant of CountUsage, dipakai path
// ResolveForOrder (validasi kuota saat SATU order akan dibuat).
func (r *DiscountRepository) CountUsageOne(ctx context.Context, id uuid.UUID) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Table("orders").
		Where("discount_id = ? AND deleted_at IS NULL", id).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("count discount usage for %s: %w", id, err)
	}
	return count, nil
}

// Update applies a partial update to a discount row. Returns ErrNotFound if
// the row doesn't exist or is already soft-deleted.
func (r *DiscountRepository) Update(ctx context.Context, id uuid.UUID, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	updates := make(map[string]any, len(fields)+1)
	for k, v := range fields {
		updates[k] = v
	}
	updates["updated_at"] = gorm.Expr("NOW()")
	res := r.db.WithContext(ctx).Model(&model.Discount{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Updates(updates)
	if res.Error != nil {
		if isUniqueViolation(res.Error) {
			return ErrCodeConflict
		}
		return fmt.Errorf("update discount %s: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// SoftDeleteParams — payload for soft-deleting a discount.
type SoftDeleteParams struct {
	DiscountID uuid.UUID
	ActorID    uuid.UUID
	Reason     string
}

// SoftDelete stamps deleted_at/deleted_by/delete_reason. NEVER removes the
// row — order.discount_id keeps pointing at it forever (§28.2 lapis 2).
func (r *DiscountRepository) SoftDelete(ctx context.Context, p SoftDeleteParams) error {
	res := r.db.WithContext(ctx).Model(&model.Discount{}).
		Where("id = ? AND deleted_at IS NULL", p.DiscountID).
		Updates(map[string]any{
			"deleted_at":    gorm.Expr("NOW()"),
			"deleted_by":    p.ActorID,
			"delete_reason": p.Reason,
			"updated_at":    gorm.Expr("NOW()"),
		})
	if res.Error != nil {
		return fmt.Errorf("soft delete discount %s: %w", p.DiscountID, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
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
