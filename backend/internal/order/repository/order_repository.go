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
	"github.com/rajaku-printing/backend/internal/order/state"
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
	// ErrDeleteForbiddenStatus — SoftDelete's row-locked re-check found the
	// order's CURRENT status disallows deletion (state.IsDeletable false),
	// even though the caller may have validated against an earlier read.
	// Mapped by the service to orderapi.ErrDeleteNotAllowedPaid.
	ErrDeleteForbiddenStatus = errors.New("order/repository: order status does not allow deletion")
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

// FindByResi returns the order (without history) or ErrNotFound. Soft-deleted
// orders (§ super admin order tools) are treated as not found — once an order
// is deleted it must vanish from every reading path, this one included.
func (r *OrderRepository) FindByResi(ctx context.Context, resi string) (*model.Order, error) {
	var o model.Order
	err := r.db.WithContext(ctx).Where("resi = ? AND deleted_at IS NULL", resi).First(&o).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find order by resi %q: %w", resi, err)
	}
	return &o, nil
}

// FindByID returns the order or ErrNotFound. Same deleted_at exclusion as
// FindByResi — every consumer module (payment, design, production, invoice,
// notification) reaches orders exclusively through orderapi.OrderCommandService,
// which is backed by FindByResi/FindByID, so this one filter covers them all.
func (r *OrderRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.Order, error) {
	var o model.Order
	err := r.db.WithContext(ctx).Where("deleted_at IS NULL").First(&o, "id = ?", id).Error
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

	q := r.db.WithContext(ctx).Model(&model.Order{}).Where("deleted_at IS NULL")
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
	q := r.db.WithContext(ctx).Model(&model.Order{}).
		Where("customer_id = ? AND deleted_at IS NULL", f.CustomerID)
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
// [start, end). Ordered by created_at ASC. Dipakai untuk rekonsiliasi harian —
// deleted_at exclusion is critical here: a deleted order must NOT inflate the
// cash reconciliation total.
func (r *OrderRepository) ListPOSByDateRange(ctx context.Context, start, end time.Time) ([]model.Order, error) {
	var items []model.Order
	err := r.db.WithContext(ctx).
		Where("channel = ? AND created_at >= ? AND created_at < ? AND deleted_at IS NULL", model.ChannelPOS, start, end).
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
			Where("id = ? AND status = ? AND deleted_at IS NULL", p.OrderID, p.FromStatus).
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
			Where("id = ? AND deleted_at IS NULL", p.OrderID).
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

// ReassignCustomer moves ALL orders currently owned by fromCustomerID to
// toCustomerID and returns the IDs of every order that moved. Used
// exclusively by the auth module's phone-claim flow when a registered
// customer absorbs a GUEST identity that owned the phone number they just
// claimed (§11 satu pelanggan satu riwayat) — see orderapi.CustomerMerger's
// doc. `orders.customer_id` is deliberately the ONLY column touched here:
// every other FK to `users` across the schema (orders.created_by,
// design_files.uploaded_by, payment_proofs.uploaded_by,
// order_state_history.changed_by, invoices.generated_by) records who
// PERFORMED an action, not who OWNS the order, and must keep pointing at the
// original actor for audit purposes.
//
// Uses `UPDATE ... RETURNING id` (via GORM's Clauses(clause.Returning{...}))
// so the moved IDs come back from the SAME statement that moved them — a
// separate SELECT-then-UPDATE would be both an extra round trip and racy
// (rows matching the SELECT could stop matching the WHERE by the time the
// UPDATE runs).
func (r *OrderRepository) ReassignCustomer(ctx context.Context, fromCustomerID, toCustomerID uuid.UUID) ([]uuid.UUID, error) {
	var moved []model.Order
	err := r.db.WithContext(ctx).
		Clauses(clause.Returning{Columns: []clause.Column{{Name: "id"}}}).
		Model(&moved).
		Where("customer_id = ? AND deleted_at IS NULL", fromCustomerID).
		Updates(map[string]any{
			"customer_id": toCustomerID,
			"updated_at":  gorm.Expr("NOW()"),
		}).Error
	if err != nil {
		return nil, fmt.Errorf("reassign customer orders from %s to %s: %w", fromCustomerID, toCustomerID, err)
	}
	ids := make([]uuid.UUID, len(moved))
	for i, o := range moved {
		ids[i] = o.ID
	}
	return ids, nil
}

// ---------------------------------------------------------------------------
// Super admin order tools (§ super admin order tools, migration 000025):
// edit data pesanan, override status ke status manapun, soft-delete pesanan.
// Each of the three methods below writes its `orders`/`order_state_history`
// mutation AND its admin_audit_log row in the SAME transaction — an audit
// entry must never exist without the change it describes actually landing,
// or vice versa.
// ---------------------------------------------------------------------------

// UpdateFieldsParams — payload for a super-admin field-level correction
// (order.edit). Fields is a raw column→value map (caller/service decides
// which columns are allowed given the order's current status) — MAY be
// empty (audit-only write, e.g. a call where every requested change turned
// out to be a no-op against the CURRENT row).
//
// ExpectedStatus — the order status the SERVICE validated `Fields` against
// (e.g. which fields are editable, whether the >=10-char financial reason
// threshold applies) BEFORE opening this transaction. Non-empty means: lock
// the row, and if its status has since changed, abort with ErrStaleState
// instead of writing a validation the current row no longer matches (TOCTOU
// guard — see EditOrder doc). Empty = no status guard (used by callers that
// don't validate against status, if any).
type UpdateFieldsParams struct {
	OrderID        uuid.UUID
	ExpectedStatus string
	Fields         map[string]any
	Audit          *model.AdminAuditLog
}

// UpdateFields applies an admin-driven partial update to `orders` columns +
// writes the matching admin_audit_log row atomically. The order row is
// locked (SELECT ... FOR UPDATE) FIRST — same pattern as AdvanceStatus/
// OverrideStatus — so ExpectedStatus is checked against the true concurrent
// state, not a possibly-stale value the caller read earlier. Returns
// ErrNotFound if the order doesn't exist or is already soft-deleted, or
// ErrStaleState if ExpectedStatus no longer matches.
func (r *OrderRepository) UpdateFields(ctx context.Context, p UpdateFieldsParams) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current struct{ Status string }
		err := tx.Table("orders").
			Select("status").
			Where("id = ? AND deleted_at IS NULL", p.OrderID).
			Clauses(gormForUpdate()).
			Take(&current).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return fmt.Errorf("lock order row: %w", err)
		}
		if p.ExpectedStatus != "" && current.Status != p.ExpectedStatus {
			return ErrStaleState
		}

		if len(p.Fields) > 0 {
			updates := make(map[string]any, len(p.Fields)+1)
			for k, v := range p.Fields {
				updates[k] = v
			}
			updates["updated_at"] = gorm.Expr("NOW()")
			if err := tx.Model(&model.Order{}).
				Where("id = ?", p.OrderID).
				Updates(updates).Error; err != nil {
				return fmt.Errorf("update order fields: %w", err)
			}
		}
		return insertAuditLog(tx, p.Audit)
	})
}

// OverrideStatusParams — payload for a super-admin forced status change
// (order.override_status). Unlike AdvanceStatus, this does NOT validate
// state.IsValidTransition — the service layer is the one that enforces
// state.IsKnown(NewStatus) before calling this; the repository just moves it.
type OverrideStatusParams struct {
	OrderID   uuid.UUID
	NewStatus string
	ChangedBy *uuid.UUID
	Note      *string // prefixed "[OVERRIDE] <reason>" by the service
	Audit     *model.AdminAuditLog
}

// OverrideStatus force-sets an order's status, recording the ACTUAL previous
// status (read under row lock, same pattern as AdvanceStatus) in
// order_state_history + the admin_audit_log row, atomically. Returns the
// previous status and ErrNotFound if the order doesn't exist / is deleted.
func (r *OrderRepository) OverrideStatus(ctx context.Context, p OverrideStatusParams) (string, error) {
	var fromStatus string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current struct{ Status string }
		err := tx.Table("orders").
			Select("status").
			Where("id = ? AND deleted_at IS NULL", p.OrderID).
			Clauses(gormForUpdate()).
			Take(&current).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return fmt.Errorf("lock order row: %w", err)
		}
		fromStatus = current.Status

		if err := tx.Model(&model.Order{}).
			Where("id = ?", p.OrderID).
			Updates(map[string]any{
				"status":     p.NewStatus,
				"updated_at": gorm.Expr("NOW()"),
			}).Error; err != nil {
			return fmt.Errorf("override status: %w", err)
		}
		if err := insertHistory(tx, p.OrderID, &fromStatus, p.NewStatus, p.ChangedBy, p.Note); err != nil {
			return err
		}
		return insertAuditLog(tx, p.Audit)
	})
	if err != nil {
		return "", err
	}
	return fromStatus, nil
}

// SoftDeleteParams — payload for a super-admin soft-delete (order.delete).
type SoftDeleteParams struct {
	OrderID uuid.UUID
	ActorID uuid.UUID
	Reason  string
	Audit   *model.AdminAuditLog
}

// SoftDelete stamps deleted_at/deleted_by/delete_reason + writes the
// admin_audit_log row atomically. NEVER removes the row or anything
// referencing it — every reading query elsewhere in this file filters
// `deleted_at IS NULL` so the order simply stops appearing.
//
// The row is locked (SELECT ... FOR UPDATE) FIRST and its status re-checked
// against state.IsDeletable — closing the TOCTOU window where the order gets
// paid (or otherwise advanced) between the service's earlier read and this
// write. Returns ErrNotFound if the order doesn't exist or is already
// deleted (idempotent guard — a double-delete is rejected rather than
// silently no-op'd), or ErrDeleteForbiddenStatus if the CURRENT status
// disallows deletion (§ super admin order tools — order sudah dibayar harus
// dibatalkan dulu).
func (r *OrderRepository) SoftDelete(ctx context.Context, p SoftDeleteParams) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current struct{ Status string }
		err := tx.Table("orders").
			Select("status").
			Where("id = ? AND deleted_at IS NULL", p.OrderID).
			Clauses(gormForUpdate()).
			Take(&current).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return fmt.Errorf("lock order row: %w", err)
		}
		if !state.IsDeletable(state.Status(current.Status)) {
			return ErrDeleteForbiddenStatus
		}

		if err := tx.Model(&model.Order{}).
			Where("id = ?", p.OrderID).
			Updates(map[string]any{
				"deleted_at":    gorm.Expr("NOW()"),
				"deleted_by":    p.ActorID,
				"delete_reason": p.Reason,
				"updated_at":    gorm.Expr("NOW()"),
			}).Error; err != nil {
			return fmt.Errorf("soft delete order: %w", err)
		}
		return insertAuditLog(tx, p.Audit)
	})
}

// insertAuditLog writes one admin_audit_log row inside the caller's open
// transaction `tx`. Shared by UpdateFields/OverrideStatus/SoftDelete above.
func insertAuditLog(tx *gorm.DB, entry *model.AdminAuditLog) error {
	if err := tx.Create(entry).Error; err != nil {
		return fmt.Errorf("insert admin_audit_log: %w", err)
	}
	return nil
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
