// Package repository handles DB access for order module.
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

	"github.com/rajaku-printing/backend/internal/order/model"
)

func gormForUpdate() clause.Locking { return clause.Locking{Strength: "UPDATE"} }

var (
	ErrNotFound     = errors.New("order/repository: not found")
	ErrResiConflict = errors.New("order/repository: resi unique conflict")
	// ErrStaleState indicates the WHERE-status guard on an update matched no
	// rows — meaning either the order was deleted or its status changed under us
	// (concurrent staff action). Caller should re-read + retry or surface a
	// "state changed, refresh" message to the user.
	ErrStaleState = errors.New("order/repository: order status changed under concurrent update")
)

// pgUniqueViolationCode is the SQLSTATE returned by Postgres on unique index
// violation. Used to detect resi collisions specifically.
const pgUniqueViolationCode = "23505"

type OrderRepository struct {
	db *gorm.DB
}

func NewOrderRepository(db *gorm.DB) *OrderRepository { return &OrderRepository{db: db} }

// CreateWithHistory inserts an order + an initial state_history row in a single
// transaction. Returns ErrResiConflict specifically when the resi UNIQUE
// constraint is violated (so the service can retry with a new random resi).
func (r *OrderRepository) CreateWithHistory(ctx context.Context, order *model.Order, initialHistory *model.OrderStateHistory) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(order).Error; err != nil {
			return err
		}
		initialHistory.OrderID = order.ID
		if err := tx.Create(initialHistory).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		if isUniqueViolation(err) {
			return ErrResiConflict
		}
		return fmt.Errorf("create order with history: %w", err)
	}
	return nil
}

// FindByResi returns the order (without history) or ErrNotFound.
func (r *OrderRepository) FindByResi(ctx context.Context, resi string) (*model.Order, error) {
	var o model.Order
	err := r.db.WithContext(ctx).Where("resi = ?", resi).First(&o).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find order by resi %q: %w", resi, err)
	}
	return &o, nil
}

// FindByID returns the order or ErrNotFound.
func (r *OrderRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.Order, error) {
	var o model.Order
	err := r.db.WithContext(ctx).First(&o, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find order by id %s: %w", id, err)
	}
	return &o, nil
}

// FindHistoryByOrderID returns state history ordered by changed_at ASC (oldest
// first — matches tracking display).
func (r *OrderRepository) FindHistoryByOrderID(ctx context.Context, orderID uuid.UUID) ([]model.OrderStateHistory, error) {
	var items []model.OrderStateHistory
	err := r.db.WithContext(ctx).
		Where("order_id = ?", orderID).
		Order("changed_at ASC").
		Find(&items).Error
	if err != nil {
		return nil, fmt.Errorf("find history for order %s: %w", orderID, err)
	}
	return items, nil
}

// AdminListFilter narrows admin order queries. Zero values = no filter.
type AdminListFilter struct {
	Status   string
	Channel  string
	Page     int
	PageSize int
}

// AdminListResult is a page of orders + total count matching the filter (for
// pagination UI).
type AdminListResult struct {
	Items    []model.Order
	Total    int64
	Page     int
	PageSize int
}

// ListForAdmin returns a paginated slice of orders matching the filter.
// Ordered by created_at DESC (newest first — matches admin dashboard usage).
func (r *OrderRepository) ListForAdmin(ctx context.Context, f AdminListFilter) (*AdminListResult, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 || f.PageSize > 100 {
		f.PageSize = 20
	}

	q := r.db.WithContext(ctx).Model(&model.Order{})
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.Channel != "" {
		q = q.Where("channel = ?", f.Channel)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("count orders: %w", err)
	}

	var items []model.Order
	offset := (f.Page - 1) * f.PageSize
	if err := q.
		Order("created_at DESC").
		Offset(offset).
		Limit(f.PageSize).
		Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list orders: %w", err)
	}
	return &AdminListResult{Items: items, Total: total, Page: f.Page, PageSize: f.PageSize}, nil
}

// ListByCustomer returns paginated orders belonging to a single customer,
// newest first. Dipakai halaman /akun (customer melihat pesanannya sendiri).
// Filter optional: Status.
type CustomerListFilter struct {
	CustomerID uuid.UUID
	Status     string
	Page       int
	PageSize   int
}

func (r *OrderRepository) ListByCustomer(ctx context.Context, f CustomerListFilter) (*AdminListResult, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 || f.PageSize > 100 {
		f.PageSize = 20
	}
	q := r.db.WithContext(ctx).Model(&model.Order{}).Where("customer_id = ?", f.CustomerID)
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("count customer orders: %w", err)
	}
	var items []model.Order
	offset := (f.Page - 1) * f.PageSize
	if err := q.
		Order("created_at DESC").
		Offset(offset).
		Limit(f.PageSize).
		Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list customer orders: %w", err)
	}
	return &AdminListResult{Items: items, Total: total, Page: f.Page, PageSize: f.PageSize}, nil
}

// ListPOSByDateRange returns POS orders (channel=pos) with created_at in
// [start, end). Ordered by created_at ASC. Dipakai untuk rekonsiliasi harian.
func (r *OrderRepository) ListPOSByDateRange(ctx context.Context, start, end time.Time) ([]model.Order, error) {
	var items []model.Order
	err := r.db.WithContext(ctx).
		Where("channel = ? AND created_at >= ? AND created_at < ?", model.ChannelPOS, start, end).
		Order("created_at ASC").
		Find(&items).Error
	if err != nil {
		return nil, fmt.Errorf("list POS by date range: %w", err)
	}
	return items, nil
}

// SetShippingCostParams — payload for atomic ongkir update + state advance.
type SetShippingCostParams struct {
	OrderID      uuid.UUID
	ShippingCost int64
	NewTotal     int64
	FromStatus   string // current status (for state_history row)
	NewStatus    string // usually "menunggu_pembayaran"
	Intermediate string // "menunggu_ongkir" if going from order_masuk (extra history row); empty if not needed
	ChangedBy    *uuid.UUID
	Note         *string
}

// SetShippingCostAndAdvance atomically updates shipping_cost + total + status
// and inserts state history row(s). If Intermediate is set, an extra history
// row (from → intermediate) is inserted before the final (intermediate → new)
// row — so the transition trail correctly reflects order_masuk → menunggu_ongkir
// → menunggu_pembayaran when admin fills ongkir in one click.
func (r *OrderRepository) SetShippingCostAndAdvance(ctx context.Context, p SetShippingCostParams) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.Order{}).
			Where("id = ? AND status = ?", p.OrderID, p.FromStatus).
			Updates(map[string]any{
				"shipping_cost": p.ShippingCost,
				"total":         p.NewTotal,
				"status":        p.NewStatus,
				"updated_at":    gorm.Expr("NOW()"),
			})
		if res.Error != nil {
			return fmt.Errorf("update order shipping+status: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			// Either the order vanished or somebody else moved it out of FromStatus.
			return ErrStaleState
		}

		from := p.FromStatus
		if p.Intermediate != "" {
			if err := insertHistory(tx, p.OrderID, &from, p.Intermediate, p.ChangedBy, p.Note); err != nil {
				return err
			}
			from = p.Intermediate
		}
		if err := insertHistory(tx, p.OrderID, &from, p.NewStatus, p.ChangedBy, p.Note); err != nil {
			return err
		}
		return nil
	})
}

// AdvanceStatusParams — payload for a status transition. Optionally sets
// metode_bayar / shipping-tracking in the same transaction.
type AdvanceStatusParams struct {
	OrderID     uuid.UUID
	FromStatus  string
	NewStatus   string
	MetodeBayar string // "" = don't touch; set to record payment method on approve
	ChangedBy   *uuid.UUID
	Note        *string
	// AllowedFromStatuses lets caller accept multiple valid current statuses
	// (e.g. MarkPendingVerification is valid from menunggu_pembayaran OR ditolak).
	// If set, FromStatus is ignored for the WHERE guard; NewStatus + history use it.
	AllowedFromStatuses []string
	// ShippingCourier / ShippingTrackingNumber — dipakai saat mark dikirim.
	// Kosong string = don't touch. Direcord di orders.shipping_courier + shipping_tracking_number.
	ShippingCourier        string
	ShippingTrackingNumber string
}

// AdvanceStatus atomically moves an order from FromStatus (or one of
// AllowedFromStatuses) → NewStatus, optionally sets metode_bayar, + inserts a
// state_history row with the ACTUAL previous status (not a guess). Returns
// ErrStaleState if the order is no longer in an acceptable status (someone
// else advanced it concurrently).
func (r *OrderRepository) AdvanceStatus(ctx context.Context, p AdvanceStatusParams) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// SELECT current status with row lock so we know the real "from" and
		// prevent races within this txn.
		var current struct {
			Status string
		}
		err := tx.Table("orders").
			Select("status").
			Where("id = ?", p.OrderID).
			Clauses(gormForUpdate()).
			Take(&current).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return fmt.Errorf("lock order row: %w", err)
		}

		if !isStatusAllowed(current.Status, p.FromStatus, p.AllowedFromStatuses) {
			return ErrStaleState
		}

		updates := map[string]any{
			"status":     p.NewStatus,
			"updated_at": gorm.Expr("NOW()"),
		}
		if p.MetodeBayar != "" {
			updates["metode_bayar"] = p.MetodeBayar
		}
		if p.ShippingCourier != "" {
			updates["shipping_courier"] = p.ShippingCourier
		}
		if p.ShippingTrackingNumber != "" {
			updates["shipping_tracking_number"] = p.ShippingTrackingNumber
		}
		if err := tx.Model(&model.Order{}).
			Where("id = ?", p.OrderID).
			Updates(updates).Error; err != nil {
			return fmt.Errorf("advance status: %w", err)
		}
		return insertHistory(tx, p.OrderID, &current.Status, p.NewStatus, p.ChangedBy, p.Note)
	})
}

func isStatusAllowed(current, from string, allowed []string) bool {
	if len(allowed) > 0 {
		for _, s := range allowed {
			if s == current {
				return true
			}
		}
		return false
	}
	return current == from
}

func insertHistory(tx *gorm.DB, orderID uuid.UUID, from *string, to string, changedBy *uuid.UUID, note *string) error {
	// Persist via raw map to avoid GORM zero-value pitfalls on nullable fields.
	row := map[string]any{
		"order_id":    orderID,
		"from_status": from,
		"to_status":   to,
		"changed_by":  changedBy,
		"note":        note,
	}
	if err := tx.Table("order_state_history").Create(&row).Error; err != nil {
		return fmt.Errorf("insert state_history: %w", err)
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
