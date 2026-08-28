// Package repository handles ONLY database access for the membership module
// (§30 CLAUDE.md, §22). Status keanggotaan itu sendiri hidup di tabel `users`
// (modul auth) — dibaca/ditulis lewat authapi.CustomerService di service
// layer, TIDAK lewat repository ini (§22: dilarang import model/repository
// modul lain langsung). Repository ini HANYA menyentuh
// membership_status_logs — audit trail milik modul ini sendiri.
package repository

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/rajaku-printing/backend/internal/membership/model"
)

type LogRepository struct {
	db *gorm.DB
}

func NewLogRepository(db *gorm.DB) *LogRepository { return &LogRepository{db: db} }

// Create inserts one membership status transition row (§30.2). Best-effort
// from the service's point of view — see membership/service transition().
func (r *LogRepository) Create(ctx context.Context, l *model.MembershipStatusLog) error {
	if err := r.db.WithContext(ctx).Create(l).Error; err != nil {
		return fmt.Errorf("create membership status log: %w", err)
	}
	return nil
}
