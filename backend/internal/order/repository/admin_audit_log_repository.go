package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/rajaku-printing/backend/internal/order/model"
)

// AdminAuditLogRepository handles read access to admin_audit_log (§ super
// admin order tools). WRITES to this table happen inside OrderRepository's
// own transactions (UpdateFields/OverrideStatus/SoftDelete → insertAuditLog)
// so an audit row is always atomic with the change it describes — this
// repository intentionally has no write path of its own.
type AdminAuditLogRepository struct {
	db *gorm.DB
}

func NewAdminAuditLogRepository(db *gorm.DB) *AdminAuditLogRepository {
	return &AdminAuditLogRepository{db: db}
}

// AdminAuditListFilter — filter for GET /admin/audit-log.
type AdminAuditListFilter struct {
	EntityType string // "" = any
	EntityID   *uuid.UUID
	Limit      int
}

// ListByEntity returns audit rows newest-first, optionally scoped to one
// entity_type/entity_id. Limit defaults to 50 when unset/invalid (<1), and
// is capped at 200 (NOT reset to the 50 default — a caller asking for 500
// should get as many as allowed, 200, not silently downgraded to 50).
func (r *AdminAuditLogRepository) ListByEntity(ctx context.Context, f AdminAuditListFilter) ([]model.AdminAuditLog, error) {
	if f.Limit < 1 {
		f.Limit = 50
	}
	if f.Limit > 200 {
		f.Limit = 200
	}
	q := r.db.WithContext(ctx).Model(&model.AdminAuditLog{})
	if f.EntityType != "" {
		q = q.Where("entity_type = ?", f.EntityType)
	}
	if f.EntityID != nil {
		q = q.Where("entity_id = ?", *f.EntityID)
	}
	var items []model.AdminAuditLog
	if err := q.Order("created_at DESC").Limit(f.Limit).Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list admin audit log: %w", err)
	}
	return items, nil
}
