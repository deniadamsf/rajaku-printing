package repository

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/rajaku-printing/backend/internal/auth/model"
)

// CustomerMergeRepository persists the durable audit trail for guest-
// identity absorption during phone-claim (migration 000019, §11 satu
// pelanggan satu riwayat). Write-only for now — nothing in this codebase
// needs to query customer_merges back yet; support staff answers "who did
// this order used to belong to" via SQL directly (see runbook), not an
// admin-panel screen.
type CustomerMergeRepository struct {
	db *gorm.DB
}

func NewCustomerMergeRepository(db *gorm.DB) *CustomerMergeRepository {
	return &CustomerMergeRepository{db: db}
}

// Create inserts one customer_merges row. cm.ID and cm.MergedAt are
// populated by DB defaults (gen_random_uuid(), now()) and reflected back
// onto cm by GORM after insert.
func (r *CustomerMergeRepository) Create(ctx context.Context, cm *model.CustomerMerge) error {
	if err := r.db.WithContext(ctx).Create(cm).Error; err != nil {
		return fmt.Errorf("insert customer_merge: %w", err)
	}
	return nil
}
