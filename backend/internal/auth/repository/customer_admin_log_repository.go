// Repository untuk customer_admin_logs — audit trail MURNI untuk SEMUA aksi
// admin di fitur "Manajemen Pelanggan" (blokir/aktifkan DAN ubah data).
// Terpisah dari user_repository.go per §22 (satu tanggung jawab per file),
// pola sama seperti membership/repository/membership_repository.go
// (LogRepository).
package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/rajaku-printing/backend/internal/auth/model"
)

type CustomerAdminLogRepository struct {
	db *gorm.DB
}

func NewCustomerAdminLogRepository(db *gorm.DB) *CustomerAdminLogRepository {
	return &CustomerAdminLogRepository{db: db}
}

// Create inserts one admin-action row (block/unblock/profile_update).
func (r *CustomerAdminLogRepository) Create(ctx context.Context, l *model.CustomerAdminLog) error {
	if err := r.db.WithContext(ctx).Create(l).Error; err != nil {
		return fmt.Errorf("create customer admin log: %w", err)
	}
	return nil
}

// CustomerAdminLogView — satu baris log + nama staff pengubah (LEFT JOIN ke
// users), dipakai render tab riwayat di halaman detail pelanggan admin.
// ChangedByName nil kalau staff pelakunya sudah tidak ada / baris lama tanpa
// changed_by.
type CustomerAdminLogView struct {
	Action        string
	Reason        *string
	Changes       *string
	ChangedByName *string
	CreatedAt     time.Time
}

// ListByCustomer returns up to `limit` most recent admin-action rows for a
// customer, newest first.
func (r *CustomerAdminLogRepository) ListByCustomer(ctx context.Context, customerID uuid.UUID, limit int) ([]CustomerAdminLogView, error) {
	if limit <= 0 {
		limit = 5
	}
	var rows []CustomerAdminLogView
	err := r.db.WithContext(ctx).
		Table("customer_admin_logs cal").
		Joins("LEFT JOIN users u ON u.id = cal.changed_by").
		Where("cal.customer_id = ?", customerID).
		Select("cal.action AS action, cal.reason AS reason, cal.changes AS changes, u.name AS changed_by_name, cal.created_at AS created_at").
		Order("cal.created_at DESC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list customer admin logs for %s: %w", customerID, err)
	}
	return rows, nil
}
