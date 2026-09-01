// Tests for applyItemDiscountAllocations (§32.3 guard, temuan review #4).
package service

import (
	"errors"
	"testing"

	"github.com/rajaku-printing/backend/internal/discount/discountapi"
	"github.com/rajaku-printing/backend/internal/order/model"
	"github.com/rajaku-printing/backend/internal/order/orderapi"
)

// TestApplyItemDiscountAllocations_HappyPath verifies the normal case: every
// Allocation.LineNo matches a real item, and the sum lands exactly on
// snap.Amount.
func TestApplyItemDiscountAllocations_HappyPath(t *testing.T) {
	items := []model.OrderItem{
		{LineNo: 1, Subtotal: 70_000},
		{LineNo: 2, Subtotal: 30_000},
	}
	snap := &discountapi.Snapshot{
		Amount: 10_000,
		Allocations: []discountapi.ItemAllocation{
			{LineNo: 1, Amount: 7_000},
			{LineNo: 2, Amount: 3_000},
		},
	}
	if err := applyItemDiscountAllocations(items, snap); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if items[0].DiscountAmount != 7_000 || items[1].DiscountAmount != 3_000 {
		t.Fatalf("allocations not applied: %+v", items)
	}
}

// TestApplyItemDiscountAllocations_LineNoDoesNotMatchAnyItem is the
// regression guard for temuan review §32.3 #4: the OLD guard only checked
// that Σ Allocations.Amount == snap.Amount (the resolver's OWN report), not
// that the allocations actually landed on a real item. A resolver returning
// LineNo values off-by-one from what order/service actually assigned (e.g.
// 0-based instead of 1-based) would pass that old check while leaving every
// items[i].DiscountAmount at 0 — orders.discount_amount would then disagree
// with Σ order_items.discount_amount with no error anywhere (§32.2).
func TestApplyItemDiscountAllocations_LineNoDoesNotMatchAnyItem(t *testing.T) {
	items := []model.OrderItem{
		{LineNo: 1, Subtotal: 70_000},
		{LineNo: 2, Subtotal: 30_000},
	}
	snap := &discountapi.Snapshot{
		Amount: 10_000,
		Allocations: []discountapi.ItemAllocation{
			// 0-based LineNo — matches NEITHER real item (which are 1 and 2).
			{LineNo: 0, Amount: 7_000},
			{LineNo: 1, Amount: 3_000},
		},
	}
	err := applyItemDiscountAllocations(items, snap)
	if !errors.Is(err, orderapi.ErrDiscountAllocationMismatch) {
		t.Fatalf("want ErrDiscountAllocationMismatch, got %v", err)
	}
	// Whatever partial application happened before the guard tripped must
	// not matter to the caller — but it must also not silently proceed with
	// items[1] (LineNo 2) left at zero while the order carries a non-zero
	// discount_amount.
	if items[1].DiscountAmount != 0 {
		t.Errorf("item with unmatched LineNo must stay untouched: %+v", items[1])
	}
}

// TestApplyItemDiscountAllocations_NoDiscount_NoOp confirms Amount==0 with
// no Allocations is a legitimate "no discount" case, not an error.
func TestApplyItemDiscountAllocations_NoDiscount_NoOp(t *testing.T) {
	items := []model.OrderItem{{LineNo: 1, Subtotal: 50_000}}
	snap := &discountapi.Snapshot{Amount: 0}
	if err := applyItemDiscountAllocations(items, snap); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if items[0].DiscountAmount != 0 {
		t.Errorf("no discount must leave DiscountAmount at 0, got %d", items[0].DiscountAmount)
	}
}
