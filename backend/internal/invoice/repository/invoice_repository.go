// Package repository — DB access untuk invoice module.
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/rajaku-printing/backend/internal/invoice/model"
)

var (
	ErrNotFound        = errors.New("invoice/repository: not found")
	ErrDuplicateOrder  = errors.New("invoice/repository: invoice already exists for this order")
)

const pgUniqueViolationCode = "23505"

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

// Create inserts an invoice row. Returns ErrDuplicateOrder kalau order_id
// sudah ada (partial unique — 1 invoice per order).
func (r *Repository) Create(ctx context.Context, inv *model.Invoice) error {
	if err := r.db.WithContext(ctx).Create(inv).Error; err != nil {
		if isUniqueViolation(err) {
			return ErrDuplicateOrder
		}
		return fmt.Errorf("insert invoice: %w", err)
	}
	return nil
}

// FindByID returns invoice or ErrNotFound.
func (r *Repository) FindByID(ctx context.Context, id uuid.UUID) (*model.Invoice, error) {
	var inv model.Invoice
	err := r.db.WithContext(ctx).First(&inv, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find invoice by id %s: %w", id, err)
	}
	return &inv, nil
}

// FindByOrderID returns invoice for order or ErrNotFound.
func (r *Repository) FindByOrderID(ctx context.Context, orderID uuid.UUID) (*model.Invoice, error) {
	var inv model.Invoice
	err := r.db.WithContext(ctx).Where("order_id = ?", orderID).First(&inv).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find invoice by order %s: %w", orderID, err)
	}
	return &inv, nil
}

// NextInvoiceNumber atomically increments the per-year counter and returns
// the next number. Uses Postgres UPSERT (ON CONFLICT ... DO UPDATE) so 2
// concurrent requests are serialized correctly (no gap, no duplicate).
func (r *Repository) NextInvoiceNumber(ctx context.Context, year int) (int, error) {
	var out struct {
		LastNumber int
	}
	sql := `
		INSERT INTO invoice_counters (year, last_number)
		VALUES ($1, 1)
		ON CONFLICT (year) DO UPDATE
			SET last_number = invoice_counters.last_number + 1
		RETURNING last_number
	`
	if err := r.db.WithContext(ctx).Raw(sql, year).Scan(&out).Error; err != nil {
		return 0, fmt.Errorf("next invoice number for year %d: %w", year, err)
	}
	return out.LastNumber, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == pgUniqueViolationCode
	}
	return false
}
