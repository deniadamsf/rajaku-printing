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

// preloadItems is a GORM scope shared by every order-reading query in this
// file (§32) — preloads order_items terurut `line_no ASC` in ONE extra
// batch query (GORM's Preload issues a single `WHERE order_id IN (...)`
// regardless of how many parent rows Find() returns), not one query per
// order. Every FindByResi/FindByID/ListForAdmin/ListByCustomer/
// ListPOSByDateRange call goes through this so callers never see an order
// with a nil/stale Items slice.
func preloadItems(db *gorm.DB) *gorm.DB {
	return db.Preload("Items", func(db *gorm.DB) *gorm.DB {
		return db.Order("line_no ASC")
	})
}

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
	// ErrItemHasDesignFiles — UpdateItems was asked to delete an order_items
	// row that design_files.order_item_id still points to (migration 000034,
	// FK is RESTRICT — no ON DELETE CASCADE, §19 keeps design_files rows
	// forever for rekap). Checked explicitly BEFORE the DELETE so the caller
	// gets a mappable sentinel instead of a raw Postgres FK-violation wrapped
	// in a generic 500 (temuan review §32.9 #2). Mapped by the service to
	// orderapi.ErrOrderItemHasDesignFiles.
	ErrItemHasDesignFiles = errors.New("order/repository: order item still referenced by design_files")
)

// pgUniqueViolationCode is the SQLSTATE returned by Postgres on unique index
// violation. Used to detect resi collisions specifically.
const pgUniqueViolationCode = "23505"

type OrderRepository struct {
	db *gorm.DB
}

func NewOrderRepository(db *gorm.DB) *OrderRepository { return &OrderRepository{db: db} }

// CreateWithHistory inserts an order + its line items (§32) + an initial
// state_history row in a SINGLE transaction. `items` must be non-empty
// (service validates 1..20 — ErrNoItems/ErrTooManyItems, §32.4); each
// element's OrderID is set here from the just-created order.ID before
// insert, so callers don't need to know order.ID up front. Returns
// ErrResiConflict specifically when the resi UNIQUE constraint is violated
// (so the service can retry with a new random resi) — items are rolled back
// along with the order in that case, no partial insert survives.
func (r *OrderRepository) CreateWithHistory(ctx context.Context, order *model.Order, items []model.OrderItem, initialHistory *model.OrderStateHistory) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(order).Error; err != nil {
			return err
		}
		for i := range items {
			items[i].OrderID = order.ID
		}
		if len(items) > 0 {
			if err := tx.Create(&items).Error; err != nil {
				return err
			}
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
	order.Items = items
	return nil
}

// FindByResi returns the order (without history) or ErrNotFound. Soft-deleted
// orders (§ super admin order tools) are treated as not found — once an order
// is deleted it must vanish from every reading path, this one included.
func (r *OrderRepository) FindByResi(ctx context.Context, resi string) (*model.Order, error) {
	var o model.Order
	err := r.db.WithContext(ctx).Scopes(preloadItems).Where("resi = ? AND deleted_at IS NULL", resi).First(&o).Error
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
	err := r.db.WithContext(ctx).Scopes(preloadItems).Where("deleted_at IS NULL").First(&o, "id = ?", id).Error
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
		Scopes(preloadItems).
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
		Scopes(preloadItems).
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
		Scopes(preloadItems).
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

// UpdateItemsParams — payload for a super-admin item-level correction
// (order.edit_items, §32.9). Unlike UpdateFields (which only ever touches
// `orders` columns), this ALSO replaces part of `order_items`:
//   - UpsertItems — every item that survives the edit (both rows the caller
//     changed and rows left untouched but re-allocated a different
//     discount_amount, §32.9) — written unconditionally on every call, since
//     the caller (service.resolveItemsForEdit) already computed the FINAL
//     state of each surviving row; the repository doesn't diff.
//   - DeleteItemIDs — existing order_items.id no longer present in the
//     caller's wanted list (§32.9: a row not mentioned is a delete).
//   - OrderFields — `orders` columns to update alongside (subtotal, total,
//     design_source) — same map shape as UpdateFieldsParams.Fields.
//
// ExpectedStatus / row-lock / audit semantics are identical to UpdateFields
// (see that doc) — this is a sibling method, not a replacement, kept
// separate because item mutation needs extra queries UpdateFields has no
// reason to carry (§22 one-function-one-responsibility).
type UpdateItemsParams struct {
	OrderID        uuid.UUID
	ExpectedStatus string
	OrderFields    map[string]any
	UpsertItems    []model.OrderItem
	DeleteItemIDs  []uuid.UUID
	Audit          *model.AdminAuditLog
}

// UpdateItems applies a super-admin item-level correction (§32.9): deletes
// removed rows, upserts surviving rows (insert if ID is zero, update
// otherwise), updates the `orders` aggregate columns, and writes the
// admin_audit_log row — ALL in the SAME transaction, row-locked first
// exactly like UpdateFields (TOCTOU guard against a concurrent status
// change — see EditOrder doc). Returns ErrNotFound / ErrStaleState with the
// same meaning as UpdateFields.
func (r *OrderRepository) UpdateItems(ctx context.Context, p UpdateItemsParams) error {
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

		if len(p.DeleteItemIDs) > 0 {
			// §22/§32.9 review #2 — design_files.order_item_id → order_items(id)
			// is RESTRICT (migration 000034, deliberately no ON DELETE CASCADE:
			// §19 keeps design_files rows forever, only the physical blob is
			// purged). Deleting a row still referenced there would otherwise
			// bubble up as a raw Postgres FK-violation wrapped into a generic
			// 500 by the caller's `default` branch. Check first, INSIDE this
			// same transaction (so it sees a consistent snapshot alongside the
			// row-lock above), and refuse with a sentinel that names the
			// offending line(s) instead.
			//
			// This queries the `design_files` table by NAME rather than
			// importing the design module's internal package: design/service
			// already imports orderapi (design depends on order, §22), so
			// order importing designapi back would make the two modules
			// mutually dependent on each other's public contracts for a single
			// existence check. A raw query against a physical table — no
			// design Go types involved — is the narrower boundary crossing of
			// the two options offered by review #2.
			lineNos, err := orderItemLineNosWithDesignFiles(tx, p.OrderID, p.DeleteItemIDs)
			if err != nil {
				return fmt.Errorf("check design_files references: %w", err)
			}
			if len(lineNos) > 0 {
				return fmt.Errorf("baris item %v masih punya file desain terkait (§19 — record dipertahankan untuk rekap): %w",
					lineNos, ErrItemHasDesignFiles)
			}
			if err := tx.Where("id IN ? AND order_id = ?", p.DeleteItemIDs, p.OrderID).
				Delete(&model.OrderItem{}).Error; err != nil {
				return fmt.Errorf("delete order items: %w", err)
			}
		}

		// Phase 1 — bump every SURVIVING existing row's line_no to a
		// collision-free offset FIRST, before writing anyone's final line_no.
		// UNIQUE(order_id, line_no) is checked immediately (not deferrable) —
		// a caller that reorders items (e.g. swaps line_no 1<->2) would hit a
		// unique-violation mid-transaction if rows were updated straight to
		// their final line_no one at a time. The offset (100000+i) can never
		// collide with a real line_no (§32.4 caps at 20 items) or with
		// another row's temp value (i is unique within this call).
		const lineNoOffset = 100_000
		for i := range p.UpsertItems {
			item := &p.UpsertItems[i]
			if item.ID == uuid.Nil {
				continue
			}
			res := tx.Model(&model.OrderItem{}).
				Where("id = ? AND order_id = ?", item.ID, p.OrderID).
				Update("line_no", lineNoOffset+i)
			if res.Error != nil {
				return fmt.Errorf("bump order item %s line_no: %w", item.ID, res.Error)
			}
			// §32.9 review #3 — the row-lock above only locks `orders`; a row
			// this call's caller computed against (resolveItemsForEdit, called
			// BEFORE this transaction opens) may have been deleted by a
			// CONCURRENT edit in the meantime. Matching 0 rows here means the
			// item list this call is about to write is already stale — abort
			// now rather than let Phase 2/3 build on top of a list that no
			// longer reflects reality.
			if res.RowsAffected == 0 {
				return ErrStaleState
			}
		}

		// Phase 2 — insert new rows (ID zero) directly at their FINAL
		// line_no: safe now, every surviving existing row is parked at the
		// offset above, so no collision is possible.
		for i := range p.UpsertItems {
			item := &p.UpsertItems[i]
			item.OrderID = p.OrderID
			if item.ID != uuid.Nil {
				continue
			}
			if err := tx.Create(item).Error; err != nil {
				return fmt.Errorf("insert order item (line_no %d): %w", item.LineNo, err)
			}
		}

		// Phase 3 — move every surviving existing row from its temp offset to
		// its FINAL line_no + write its other columns.
		for i := range p.UpsertItems {
			item := &p.UpsertItems[i]
			if item.ID == uuid.Nil {
				continue
			}
			res := tx.Model(&model.OrderItem{}).
				Where("id = ? AND order_id = ?", item.ID, p.OrderID).
				Updates(map[string]any{
					"line_no":         item.LineNo,
					"width_cm":        item.WidthCm,
					"height_cm":       item.HeightCm,
					"quantity":        item.Quantity,
					"unit_price":      item.UnitPrice,
					"subtotal":        item.Subtotal,
					"discount_amount": item.DiscountAmount,
					"item_notes":      item.ItemNotes,
					"updated_at":      gorm.Expr("NOW()"),
				})
			if res.Error != nil {
				return fmt.Errorf("update order item %s: %w", item.ID, res.Error)
			}
			// §32.9 review #3 — Updates() matching zero rows is NOT an error to
			// GORM (no unique/FK violation, just an empty WHERE match), so
			// without this check a row deleted by a CONCURRENT admin between
			// resolveItemsForEdit's read and this write would silently vanish
			// from the write while its Subtotal/DiscountAmount still counted
			// toward `orders.subtotal`/`orders.discount_amount` below —
			// exactly the Σ item != aggregate drift §32.2 forbids. Surface it
			// as the same "refresh & retry" signal as the row-lock above
			// instead of writing a mismatched aggregate.
			if res.RowsAffected == 0 {
				return ErrStaleState
			}
		}

		if len(p.OrderFields) > 0 {
			updates := make(map[string]any, len(p.OrderFields)+1)
			for k, v := range p.OrderFields {
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

// orderItemLineNosWithDesignFiles returns the line_no of every row in
// candidateItemIDs that design_files.order_item_id still references (§32.9
// review #2) — used by UpdateItems to refuse a delete with a sentinel that
// names the offending line(s), instead of letting Postgres reject it as a
// bare FK-violation. Queried by table name (not through the design module's
// Go package, see UpdateItems call site comment) inside the caller's open
// transaction `tx` so it sees the same snapshot as the row-lock above.
func orderItemLineNosWithDesignFiles(tx *gorm.DB, orderID uuid.UUID, candidateItemIDs []uuid.UUID) ([]int, error) {
	var lineNos []int
	err := tx.Table("order_items").
		Joins("JOIN design_files ON design_files.order_item_id = order_items.id").
		Where("order_items.order_id = ? AND order_items.id IN ?", orderID, candidateItemIDs).
		Distinct().
		Order("order_items.line_no ASC").
		Pluck("order_items.line_no", &lineNos).Error
	if err != nil {
		return nil, fmt.Errorf("query design_files references: %w", err)
	}
	return lineNos, nil
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
	//
	// changed_at diisi EKSPLISIT dengan clock_timestamp(), bukan dibiarkan
	// jatuh ke DEFAULT now(). Di Postgres, now() adalah waktu MULAI TRANSAKSI —
	// nilainya sama persis untuk semua baris di dalam satu transaksi. Ketika
	// SetShippingCostAndAdvance menulis dua baris sekaligus (order_masuk →
	// menunggu_ongkir → menunggu_pembayaran), keduanya jadi berstempel identik
	// dan `ORDER BY changed_at ASC` di FindHistoryByOrderID kehilangan urutan
	// — timeline di layar admin sempat menampilkan "menunggu pembayaran"
	// SEBELUM "menunggu ongkir". clock_timestamp() maju di tiap statement,
	// jadi urutan barisnya deterministik tanpa perlu kolom urutan baru.
	row := map[string]any{
		"order_id":    orderID,
		"from_status": from,
		"to_status":   to,
		"changed_by":  changedBy,
		"note":        note,
		"changed_at":  gorm.Expr("clock_timestamp()"),
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
