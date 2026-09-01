// Super admin item-level order correction (§32.9) — kept in its own file
// (§22, one concern per file) since it's a distinct algorithm (diff old vs
// new item list, re-quote new rows, reallocate a FIXED discount amount)
// from the rest of admin_override.go's field-level EditOrder plumbing.
package service

import (
	"context"
	"fmt"
	"math"
	"sort"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/catalog/catalogapi"
	"github.com/rajaku-printing/backend/internal/order/model"
	"github.com/rajaku-printing/backend/internal/order/orderapi"
)

// EditOrderItemInput — one row within EditOrderInput.Items (§32.9).
//
//   - ID != nil → edit of an EXISTING row. Editable: WidthCm, HeightCm,
//     Quantity, UnitPrice, ItemNotes. ProductID/MaterialID/DesignSource/
//     DesignBrief MUST be left at their zero value — sending any of them is
//     rejected with orderapi.ErrOrderItemProductNotEditable, because
//     changing a row's product is, per spec, a DIFFERENT order line (delete
//   - add), not an edit.
//   - ID == nil → a NEW row. ProductID/MaterialID/DesignSource are REQUIRED;
//     UnitPrice is IGNORED (always re-quoted via catalog, §32.9 — "Baris
//     baru tetap di-Quote lewat catalog supaya snapshot nama/pricing_type-nya
//     benar", never trust an admin-typed price for a fresh line).
//
// An existing item whose ID is NOT present anywhere in EditOrderInput.Items
// is treated as a DELETE — there is no separate "remove" flag, the final
// list IS the full desired state (mirrors how CreateOnlineOrder/
// CreatePOSOrder take the item list wholesale, §32).
type EditOrderItemInput struct {
	ID *uuid.UUID

	// New-row-only fields:
	ProductID    *uuid.UUID
	MaterialID   *uuid.UUID
	DesignSource string // "upload" | "request"; required for new rows
	DesignBrief  string

	// Editable on both new & existing rows:
	WidthCm   int
	HeightCm  int
	Quantity  int
	UnitPrice int64 // ignored for new rows (see doc above)
	ItemNotes *string
}

// editItemsResult — output of resolveItemsForEdit: the FULL final item list
// (existing rows updated in place, new rows appended, LineNo renumbered
// 1..N in EditOrderInput.Items order), which existing rows got dropped, the
// new Σ subtotal, and a per-row audit summary of what changed.
type editItemsResult struct {
	final       []model.OrderItem
	deletedIDs  []uuid.UUID
	newSubtotal int64
	changes     model.ChangeSet
}

// resolveItemsForEdit implements §32.9's per-row editing rules against an
// order's CURRENT items (`current`), given the admin's desired final list
// (`wanted`). It does NOT touch discount allocation — that is recomputed
// separately by applyFinancialFields once the final subtotal is known (§32.9
// keeps "what items look like" and "how the discount splits across them" as
// two separate steps, since the second one needs orders.DiscountAmount which
// this function doesn't have).
func (s *Service) resolveItemsForEdit(ctx context.Context, current []model.OrderItem, wanted []EditOrderItemInput) (*editItemsResult, error) {
	if err := validateItemCount(len(wanted)); err != nil {
		return nil, err
	}

	byID := make(map[uuid.UUID]*model.OrderItem, len(current))
	for i := range current {
		byID[current[i].ID] = &current[i]
	}
	kept := make(map[uuid.UUID]bool, len(wanted))
	changes := model.ChangeSet{}
	final := make([]model.OrderItem, 0, len(wanted))
	var subtotal int64

	for i, w := range wanted {
		lineNo := i + 1
		if w.ID != nil {
			// §32.9 review #1 — an ID sent twice would make this loop add its
			// Subtotal to `subtotal` twice while the repository only UPDATEs
			// the one physical row once (same ID), silently doubling
			// orders.subtotal against Σ order_items.subtotal (§32.2). Reject
			// before it ever reaches applyExistingItemEdit.
			if kept[*w.ID] {
				return nil, fmt.Errorf("baris %d: item id %s: %w", lineNo, *w.ID, orderapi.ErrOrderItemDuplicate)
			}
			updated, err := applyExistingItemEdit(byID, *w.ID, w, lineNo, changes)
			if err != nil {
				return nil, err
			}
			kept[*w.ID] = true
			subtotal += updated.Subtotal
			final = append(final, *updated)
			continue
		}

		newItem, err := s.quoteNewEditItem(ctx, w, lineNo)
		if err != nil {
			return nil, err
		}
		changes[fmt.Sprintf("item_baru_%d", lineNo)] = model.FieldChange{From: nil, To: itemChangeSummary(newItem)}
		subtotal += newItem.Subtotal
		final = append(final, *newItem)
	}

	deletedIDs := make([]uuid.UUID, 0)
	for id, old := range byID {
		if kept[id] {
			continue
		}
		deletedIDs = append(deletedIDs, id)
		changes[fmt.Sprintf("item_%s", id)] = model.FieldChange{From: itemChangeSummary(old), To: nil}
	}

	return &editItemsResult{final: final, deletedIDs: deletedIDs, newSubtotal: subtotal, changes: changes}, nil
}

// applyExistingItemEdit validates & applies an edit to an EXISTING row
// (w.ID != nil), returning the updated row (LineNo already set to its new
// position) and staging an audit entry IF anything actually changed.
func applyExistingItemEdit(byID map[uuid.UUID]*model.OrderItem, id uuid.UUID, w EditOrderItemInput, lineNo int, changes model.ChangeSet) (*model.OrderItem, error) {
	old, ok := byID[id]
	if !ok {
		return nil, orderapi.ErrOrderItemNotFound
	}
	if w.ProductID != nil || w.MaterialID != nil || w.DesignSource != "" || w.DesignBrief != "" {
		return nil, orderapi.ErrOrderItemProductNotEditable
	}
	if w.WidthCm <= 0 || w.HeightCm <= 0 || w.Quantity <= 0 || w.UnitPrice < 0 {
		return nil, orderapi.ErrOrderItemInvalid
	}

	updated := *old
	updated.LineNo = lineNo
	updated.WidthCm = w.WidthCm
	updated.HeightCm = w.HeightCm
	updated.Quantity = w.Quantity
	updated.UnitPrice = w.UnitPrice
	updated.Subtotal = w.UnitPrice * int64(w.Quantity)
	if w.ItemNotes != nil {
		updated.ItemNotes = w.ItemNotes
	}

	if oldSummary, newSummary := itemChangeSummary(old), itemChangeSummary(&updated); oldSummary != newSummary {
		changes[fmt.Sprintf("item_%s", old.ID)] = model.FieldChange{From: oldSummary, To: newSummary}
	}
	return &updated, nil
}

// quoteNewEditItem builds a brand-new order_items row for §32.9's "tambah
// baris" case — mirrors quoteOnlineItems/quotePOSItems (re-quote via
// catalog, never trust a client-supplied price).
func (s *Service) quoteNewEditItem(ctx context.Context, w EditOrderItemInput, lineNo int) (*model.OrderItem, error) {
	if w.ProductID == nil || w.MaterialID == nil {
		return nil, orderapi.ErrOrderItemInvalid
	}
	designSource := model.DesignSource(w.DesignSource)
	if designSource != model.DesignSourceUpload && designSource != model.DesignSourceRequest {
		return nil, fmt.Errorf("item baru %d: design_source %q: %w", lineNo, w.DesignSource, orderapi.ErrInvalidDesignSource)
	}
	qty := w.Quantity
	if qty <= 0 {
		qty = 1
	}
	quote, err := s.catalog.Quote(ctx, catalogapi.QuoteRequest{
		ProductID:  *w.ProductID,
		MaterialID: *w.MaterialID,
		WidthCm:    w.WidthCm,
		HeightCm:   w.HeightCm,
	})
	if err != nil {
		return nil, fmt.Errorf("item baru %d: quote: %w", lineNo, err)
	}
	item := &model.OrderItem{
		LineNo:               lineNo,
		ProductID:            &quote.ProductID,
		ProductNameSnapshot:  quote.ProductName,
		MaterialID:           &quote.MaterialID,
		MaterialNameSnapshot: quote.MaterialName,
		PricingTypeSnapshot:  string(quote.PricingType),
		WidthCm:              w.WidthCm,
		HeightCm:             w.HeightCm,
		Quantity:             qty,
		UnitPrice:            quote.TotalPrice,
		Subtotal:             quote.TotalPrice * int64(qty),
		DesignSource:         designSource,
	}
	if w.DesignBrief != "" {
		item.DesignBrief = strPtr(w.DesignBrief)
	}
	if w.ItemNotes != nil {
		item.ItemNotes = w.ItemNotes
	}
	return item, nil
}

// itemChangeSummary renders a human-readable one-liner for an order_items
// row, used to build admin_audit_log's "nilai lama → baru" entries (§32.9,
// pola §31.5). Kept string-based (not structured) because different rows
// changed for different reasons (new/edited/deleted) and the audit log's
// Changes column is already a free-form map (model.ChangeSet).
//
// LineNo is included (temuan review §32.9 #6) — a PATCH that only swaps two
// rows' display order and touches nothing else would otherwise produce an
// identical summary before/after, so applyExistingItemEdit never stages a
// change, EditOrder's `len(changes) == 0` short-circuit fires, and the whole
// request is silently discarded (HTTP 200, nothing persisted, no audit row)
// even though the admin's re-ordering was never saved.
func itemChangeSummary(it *model.OrderItem) string {
	return fmt.Sprintf("#%d %s / %s, %dx%dcm, qty %d, @Rp%d = Rp%d",
		it.LineNo, it.ProductNameSnapshot, it.MaterialNameSnapshot, it.WidthCm, it.HeightCm, it.Quantity, it.UnitPrice, it.Subtotal)
}

// allocateDiscountAcrossItems splits `discountAmount` across `items`
// proportional to each item's Subtotal using the largest-remainder method
// (§32.3's algorithm), mutating items[i].DiscountAmount in place.
// Guarantees Σ result == discountAmount exactly.
//
// This is RE-IMPLEMENTED here (rather than reusing discount/service's
// unexported `allocate`) because this recomputation happens at EDIT time
// against an order's own ALREADY-COMMITTED discount_amount snapshot — a
// different concern from discount/service's resolve-time allocation (order
// module can't reach that unexported helper across the package boundary
// anyway, and discountapi deliberately doesn't expose it as part of its
// public contract, §22).
func allocateDiscountAcrossItems(discountAmount int64, items []model.OrderItem) {
	for i := range items {
		items[i].DiscountAmount = 0
	}
	if discountAmount <= 0 || len(items) == 0 {
		return
	}
	var basis int64
	for i := range items {
		basis += items[i].Subtotal
	}
	if basis <= 0 {
		return
	}

	type remainder struct {
		idx  int
		frac float64
		line int
	}
	remainders := make([]remainder, len(items))
	var allocated int64
	for i := range items {
		exact := float64(discountAmount) * float64(items[i].Subtotal) / float64(basis)
		floor := int64(math.Floor(exact))
		items[i].DiscountAmount = floor
		allocated += floor
		remainders[i] = remainder{idx: i, frac: exact - float64(floor), line: items[i].LineNo}
	}
	leftover := discountAmount - allocated
	sort.SliceStable(remainders, func(a, b int) bool {
		if remainders[a].frac != remainders[b].frac {
			return remainders[a].frac > remainders[b].frac
		}
		return remainders[a].line < remainders[b].line
	})
	for i := int64(0); i < leftover && int(i) < len(remainders); i++ {
		items[remainders[i].idx].DiscountAmount++
	}
}
