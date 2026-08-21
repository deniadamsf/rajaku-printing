// Super admin order tools — edit data pesanan, override status ke status
// manapun, dan soft-delete pesanan (§ super admin order tools). Kept in a
// separate file from order_service.go per §22 (satu fungsi satu tanggung
// jawab / jangan gemukkan satu file) — these three admin-only mutations are
// a distinct concern from the normal customer/staff order lifecycle above.
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/rajaku-printing/backend/internal/order/model"
	"github.com/rajaku-printing/backend/internal/order/orderapi"
	"github.com/rajaku-printing/backend/internal/order/repository"
	"github.com/rajaku-printing/backend/internal/order/state"
	"github.com/rajaku-printing/backend/internal/pkg/phone"
)

// minReasonLen / minFinancialReasonLen — §"kalau kosong, kembalikan sentinel
// error ErrReasonRequired" — every admin action needs SOME justification
// (min 3 chars, just enough to reject empty/whitespace-only input); editing
// financial fields on an order that's already `dibayar` or later needs a
// real explanation (min 10 chars) because it affects cash reconciliation.
const (
	minReasonLen          = 3
	minFinancialReasonLen = 10
)

// EditOrderInput — fields a super admin MAY correct. Every field is a
// pointer: nil = "leave untouched", set = "change to this value" (including
// zero values like ShippingCost=0 or Note="").
//
// NOTE: this deliberately does NOT include a `Total` field — total is NEVER
// writable directly, it's always recomputed as subtotal + shipping_cost (see
// applyFinancialFields) so it can't drift out of sync with the numbers that
// make it up. It also does NOT include customer name/phone — those live on
// `users` (auth module), a GLOBAL identity shared across every order that
// customer has (§11 matching key), and editing them here would silently
// rewrite contact info for every other order too, bypassing the phone_claim
// OTP proof-of-ownership flow. What CAN be corrected here is the PER-ORDER
// shipping recipient (who/where this one order should be delivered to).
type EditOrderInput struct {
	ShippingRecipientName  *string
	ShippingRecipientPhone *string // raw; normalized inside EditOrder
	ShippingAddress        *string
	Note                   *string
	Subtotal               *int64
	ShippingCost           *int64
}

// AuditLogRow — projection of model.AdminAuditLog for the admin panel.
type AuditLogRow struct {
	ID          string
	ActorUserID string
	Action      string
	EntityType  string
	EntityID    string
	EntityLabel string
	Changes     map[string]model.FieldChange
	Reason      string
	CreatedAt   string
}

// EditOrder implements the super-admin "koreksi data pesanan" tool. Field
// editability depends on the order's CURRENT status:
//   - Selalu boleh: ShippingAddress, ShippingRecipientName,
//     ShippingRecipientPhone, Note.
//   - Kalau status BELUM `dibayar` (order_masuk..ditolak, see
//     state.IsPreDibayar): Subtotal & ShippingCost juga boleh diubah bebas.
//   - Kalau status SUDAH `dibayar` atau lanjut: Subtotal/ShippingCost tetap
//     boleh diubah, TAPI reason wajib >= 10 karakter (data finansial
//     pasca-bayar sensitif untuk rekonsiliasi kas) — kalau tidak,
//     orderapi.ErrReasonRequired.
//   - Order dalam status terminal (selesai/dibatalkan) tidak boleh diedit
//     sama sekali (orderapi.ErrFieldNotEditable) — itu riwayat final; kalau
//     memang perlu dikoreksi, buka dulu lewat OverrideStatus.
//
// `total` is NEVER a directly editable field — it's always RECOMPUTED as
// subtotal + shipping_cost (same formula as the normal ongkir-fill path,
// order_service.go SetShippingCost) whenever either operand changes, so it
// can never drift out of sync with the numbers that make it up (invoice/
// rekap balance depends on this invariant holding).
//
// The order row is locked under a DB transaction (via
// repository.UpdateFieldsParams.ExpectedStatus) so a status change that
// happens concurrently, AFTER this function validated `in` against `o.Status`
// but BEFORE the write lands, aborts the write with orderapi.ErrOrderStateChanged
// instead of silently applying a validation decision made against a status
// that's no longer current (TOCTOU guard).
func (s *Service) EditOrder(ctx context.Context, resiStr string, actorID uuid.UUID, in EditOrderInput, reason string) (*model.Order, error) {
	reason = strings.TrimSpace(reason)
	if len(reason) < minReasonLen {
		return nil, orderapi.ErrReasonRequired
	}

	o, err := s.orders.FindByResi(ctx, resiStr)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, orderapi.ErrOrderNotFound
		}
		return nil, fmt.Errorf("edit order: lookup %s: %w", resiStr, err)
	}
	if state.IsTerminal(o.Status) {
		return nil, orderapi.ErrFieldNotEditable
	}

	fields := map[string]any{}
	changes := model.ChangeSet{}

	applyStringField(fields, changes, "shipping_address", o.ShippingAddress, in.ShippingAddress)
	applyStringField(fields, changes, "notes", o.Notes, in.Note)

	if err := applyShippingRecipientFields(o, in, fields, changes); err != nil {
		return nil, err
	}
	if err := applyFinancialFields(o, in, reason, fields, changes); err != nil {
		return nil, err
	}

	if len(changes) == 0 {
		// Nothing actually changed — idempotent no-op, don't write an empty
		// audit row (nor bother re-reading the order).
		return o, nil
	}

	label := o.Resi
	if err := s.orders.UpdateFields(ctx, repository.UpdateFieldsParams{
		OrderID:        o.ID,
		ExpectedStatus: string(o.Status),
		Fields:         fields,
		Audit: &model.AdminAuditLog{
			ActorUserID: actorID,
			Action:      "order.edit",
			EntityType:  "order",
			EntityID:    o.ID,
			EntityLabel: &label,
			Changes:     changes,
			Reason:      reason,
		},
	}); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, orderapi.ErrOrderNotFound
		}
		if errors.Is(err, repository.ErrStaleState) {
			return nil, orderapi.ErrOrderStateChanged
		}
		return nil, fmt.Errorf("edit order: update fields: %w", err)
	}

	updated, err := s.orders.FindByResi(ctx, resiStr)
	if err != nil {
		return nil, fmt.Errorf("edit order: reload after update: %w", err)
	}
	return updated, nil
}

// applyFinancialFields validates & stages Subtotal/ShippingCost changes into
// fields/changes, RECOMPUTING `total` from the result (never writing `total`
// straight from caller input — see EditOrder doc). Returns
// orderapi.ErrReasonRequired if the order is at/past `dibayar` and reason is
// too short.
func applyFinancialFields(o *model.Order, in EditOrderInput, reason string, fields map[string]any, changes model.ChangeSet) error {
	touchesSubtotal := in.Subtotal != nil && *in.Subtotal != o.Subtotal
	touchesCost := in.ShippingCost != nil && (o.ShippingCost == nil || *o.ShippingCost != *in.ShippingCost)
	if (touchesSubtotal || touchesCost) && !state.IsPreDibayar(o.Status) && len(reason) < minFinancialReasonLen {
		return orderapi.ErrReasonRequired
	}
	if !touchesSubtotal && !touchesCost {
		return nil
	}

	newSubtotal := o.Subtotal
	if touchesSubtotal {
		fields["subtotal"] = *in.Subtotal
		changes["subtotal"] = model.FieldChange{From: o.Subtotal, To: *in.Subtotal}
		newSubtotal = *in.Subtotal
	}

	var newShippingCost int64
	if o.ShippingCost != nil {
		newShippingCost = *o.ShippingCost
	}
	if touchesCost {
		fields["shipping_cost"] = *in.ShippingCost
		changes["shipping_cost"] = model.FieldChange{From: derefInt64(o.ShippingCost), To: *in.ShippingCost}
		newShippingCost = *in.ShippingCost
	}

	newTotal := newSubtotal + newShippingCost
	if newTotal != o.Total {
		fields["total"] = newTotal
		changes["total"] = model.FieldChange{From: o.Total, To: newTotal}
	}
	return nil
}

// applyShippingRecipientFields stages ShippingRecipientName/
// ShippingRecipientPhone changes — PER-ORDER delivery contact fields on
// `orders` itself (NOT the customer's global identity on `users` — see
// EditOrderInput doc for why those were deliberately removed from this
// endpoint).
func applyShippingRecipientFields(o *model.Order, in EditOrderInput, fields map[string]any, changes model.ChangeSet) error {
	applyStringField(fields, changes, "shipping_recipient_name", o.ShippingRecipientName, in.ShippingRecipientName)
	if in.ShippingRecipientPhone != nil {
		normalized, err := phone.Normalize(*in.ShippingRecipientPhone)
		if err != nil {
			return fmt.Errorf("edit order: shipping recipient phone: %w", err)
		}
		applyStringField(fields, changes, "shipping_recipient_phone", o.ShippingRecipientPhone, &normalized)
	}
	return nil
}

// OverrideStatus force-sets an order's status to ANY status state.IsKnown()
// recognizes, bypassing state.IsValidTransition — reserved for super_admin
// correction of stuck/mis-transitioned orders. reason is mandatory (>= 10
// chars).
//
// IMPORTANT — this method deliberately NEVER enqueues a WhatsApp
// notification. Every OTHER path that changes order status either goes
// through commandTransition (called by payment/design/production modules,
// which trigger WA at THEIR OWN call sites via notificationapi.Enqueuer —
// order.Service itself never auto-fires WA from a bare status change) or
// calls s.enqueueOngkirReady explicitly (SetShippingCost/ConfirmPickupTotal
// above). OverrideStatus calls neither: it talks straight to
// s.orders.OverrideStatus and never touches s.notifier. An admin override is
// an out-of-band correction (e.g. unsticking an order after a bug), not a
// genuine business event the customer should be auto-notified about — if a
// future requirement needs the customer told, that must be an explicit,
// separate opt-in action, never implicit here.
func (s *Service) OverrideStatus(ctx context.Context, resiStr string, actorID uuid.UUID, toStatus state.Status, reason string) (*model.Order, error) {
	reason = strings.TrimSpace(reason)
	if len(reason) < minFinancialReasonLen {
		return nil, orderapi.ErrReasonRequired
	}
	if !state.IsKnown(toStatus) {
		return nil, orderapi.ErrStatusUnknown
	}

	o, err := s.orders.FindByResi(ctx, resiStr)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, orderapi.ErrOrderNotFound
		}
		return nil, fmt.Errorf("override status: lookup %s: %w", resiStr, err)
	}

	note := "[OVERRIDE] " + reason
	label := o.Resi
	changes := model.ChangeSet{
		"status": {From: string(o.Status), To: string(toStatus)},
	}
	if _, err := s.orders.OverrideStatus(ctx, repository.OverrideStatusParams{
		OrderID:   o.ID,
		NewStatus: string(toStatus),
		ChangedBy: &actorID,
		Note:      &note,
		Audit: &model.AdminAuditLog{
			ActorUserID: actorID,
			Action:      "order.override_status",
			EntityType:  "order",
			EntityID:    o.ID,
			EntityLabel: &label,
			Changes:     changes,
			Reason:      reason,
		},
	}); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, orderapi.ErrOrderNotFound
		}
		return nil, fmt.Errorf("override status: %w", err)
	}

	updated, err := s.orders.FindByResi(ctx, resiStr)
	if err != nil {
		return nil, fmt.Errorf("override status: reload after update: %w", err)
	}
	return updated, nil
}

// SoftDeleteOrder implements the super-admin "hapus pesanan" tool as a SOFT
// delete — orders.deleted_at/deleted_by/delete_reason are stamped; the row
// (and everything referencing it — payment proofs, design files, invoices,
// state history) stays intact for rekap/audit. From this point on every
// order-reading query across the system excludes it (repository.go filters
// `deleted_at IS NULL` everywhere) but it is NOT gone. reason is mandatory
// (>= 10 chars) — deleting an order is destructive-looking enough from the
// admin's perspective to warrant a real explanation.
//
// Guard: an order at/past `dibayar` (state.IsDeletable false — every status
// except dibatalkan and the pre-payment ones) CANNOT be deleted directly —
// orderapi.ErrDeleteNotAllowedPaid. Deleting a paid order would make it
// vanish from cash reconciliation (ListPOSByDateRange filters
// `deleted_at IS NULL`) without any trace, which is a money-laundering-shaped
// hole. The admin must cancel it first (OverrideStatus → dibatalkan), which
// itself is unaffected (Dibatalkan orders never counted as revenue). This is
// checked here (cheap early rejection against the read above) AND,
// authoritatively, inside repository.SoftDelete under a row lock — see that
// doc for the TOCTOU reasoning.
func (s *Service) SoftDeleteOrder(ctx context.Context, resiStr string, actorID uuid.UUID, reason string) error {
	reason = strings.TrimSpace(reason)
	if len(reason) < minFinancialReasonLen {
		return orderapi.ErrReasonRequired
	}

	o, err := s.orders.FindByResi(ctx, resiStr)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return orderapi.ErrOrderNotFound
		}
		return fmt.Errorf("soft delete order: lookup %s: %w", resiStr, err)
	}
	if !state.IsDeletable(o.Status) {
		return orderapi.ErrDeleteNotAllowedPaid
	}

	label := o.Resi
	if err := s.orders.SoftDelete(ctx, repository.SoftDeleteParams{
		OrderID: o.ID,
		ActorID: actorID,
		Reason:  reason,
		Audit: &model.AdminAuditLog{
			ActorUserID: actorID,
			Action:      "order.delete",
			EntityType:  "order",
			EntityID:    o.ID,
			EntityLabel: &label,
			Changes:     model.ChangeSet{"deleted_at": {From: nil, To: "now"}},
			Reason:      reason,
		},
	}); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return orderapi.ErrOrderNotFound
		}
		if errors.Is(err, repository.ErrDeleteForbiddenStatus) {
			return orderapi.ErrDeleteNotAllowedPaid
		}
		return fmt.Errorf("soft delete order: %w", err)
	}

	s.cancelPendingNotifications(ctx, o.ID, o.Resi)
	return nil
}

// cancelPendingNotifications best-effort cancels any WA job still queued
// (pending/failed) for an order that was just soft-deleted — a customer must
// never receive a WA carrying a `/lacak/<resi>` link that now 404s. Never
// blocks or fails the delete itself: s.notifCanceller is optional (nil in
// deployments/tests that don't wire it), and any error is logged, not
// returned — same best-effort contract as every other notificationapi call
// site in this module (see notifier field doc in order_service.go).
func (s *Service) cancelPendingNotifications(ctx context.Context, orderID uuid.UUID, resi string) {
	if s.notifCanceller == nil {
		return
	}
	n, err := s.notifCanceller.CancelOrderJobs(ctx, orderID)
	if err != nil {
		log.Ctx(ctx).Error().Err(err).
			Str("order_id", orderID.String()).
			Str("resi", resi).
			Msg("cancel pending notification jobs for deleted order failed — customer may still receive a stale WA")
		return
	}
	if n > 0 {
		log.Ctx(ctx).Info().
			Str("order_id", orderID.String()).
			Str("resi", resi).
			Int64("cancelled_jobs", n).
			Msg("cancelled pending notification jobs for deleted order")
	}
}

// ListAuditLog implements GET /admin/audit-log. entityID nil = all orders.
func (s *Service) ListAuditLog(ctx context.Context, entityID *uuid.UUID, limit int) ([]AuditLogRow, error) {
	if s.audit == nil {
		return nil, fmt.Errorf("list audit log: audit store not wired")
	}
	rows, err := s.audit.ListByEntity(ctx, repository.AdminAuditListFilter{
		EntityType: "order",
		EntityID:   entityID,
		Limit:      limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list audit log: %w", err)
	}
	out := make([]AuditLogRow, 0, len(rows))
	for _, r := range rows {
		row := AuditLogRow{
			ID:          r.ID.String(),
			ActorUserID: r.ActorUserID.String(),
			Action:      r.Action,
			EntityType:  r.EntityType,
			EntityID:    r.EntityID.String(),
			Changes:     r.Changes,
			Reason:      r.Reason,
			CreatedAt:   r.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		}
		if r.EntityLabel != nil {
			row.EntityLabel = *r.EntityLabel
		}
		out = append(out, row)
	}
	return out, nil
}

// --- small helpers ---

// applyStringField stages a *string field change into fields/changes iff
// `newVal` is non-nil AND differs from the current value.
func applyStringField(fields map[string]any, changes model.ChangeSet, column string, oldVal, newVal *string) {
	if newVal == nil {
		return
	}
	if oldVal != nil && *oldVal == *newVal {
		return
	}
	if oldVal == nil && *newVal == "" {
		return
	}
	fields[column] = *newVal
	var from any
	if oldVal != nil {
		from = *oldVal
	}
	changes[column] = model.FieldChange{From: from, To: *newVal}
}

func derefInt64(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}
