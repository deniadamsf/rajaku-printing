package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/discount/discountapi"
	"github.com/rajaku-printing/backend/internal/discount/model"
	"github.com/rajaku-printing/backend/internal/discount/repository"
	"github.com/rajaku-printing/backend/internal/settings/settingsapi"
)

// ResolveForOrder implements discountapi.Resolver — dipanggil order module
// (online & POS) setelah subtotal produk terhitung, SEBELUM total akhir
// dihitung (§28.3). Tiga jalur:
//
//  1. Tidak ada diskon (DiscountID nil & ManualAmount <= 0) → Snapshot kosong,
//     TIDAK error (order tanpa diskon adalah kasus normal, bukan invalid).
//  2. Diskon manual (DiscountID nil, ManualAmount > 0) → Note wajib diisi.
//  3. Diskon master (DiscountID != nil) → divalidasi penuh §28.4, lalu
//     dihitung via computeAmount.
//
// DiscountID DAN ManualAmount > 0 sekaligus adalah input ambigu, ditolak
// eksplisit.
func (s *Service) ResolveForOrder(ctx context.Context, in discountapi.ResolveInput) (*discountapi.Snapshot, error) {
	// Review finding #6 — pakai `!= 0`, BUKAN `> 0`, supaya ManualAmount
	// negatif tetap masuk jalur validasi (resolveManualDiscount) alih-alih
	// diam-diam ditelan sebagai "tidak ada diskon". Hanya ManualAmount == 0
	// (persis nol) yang berarti "order ini tidak pakai diskon sama sekali".
	if in.DiscountID != nil && in.ManualAmount != 0 {
		return nil, discountapi.ErrDiscountAmbiguousInput
	}
	if in.DiscountID == nil && in.ManualAmount == 0 {
		return &discountapi.Snapshot{}, nil
	}
	if in.DiscountID == nil {
		return resolveManualDiscount(in)
	}
	return s.resolveMasterDiscount(ctx, in)
}

// totalSubtotal sums every item's Subtotal — the basis for a MANUAL discount
// (§32.3: diskon manual tidak punya cakupan produk, jadi basisnya SELURUH
// order, dialokasikan ke semua item).
func totalSubtotal(items []discountapi.ResolveItem) int64 {
	var sum int64
	for i := range items {
		sum += items[i].Subtotal
	}
	return sum
}

func resolveManualDiscount(in discountapi.ResolveInput) (*discountapi.Snapshot, error) {
	note := strings.TrimSpace(in.Note)
	if note == "" {
		return nil, discountapi.ErrManualDiscountNoteRequired
	}
	if in.ManualAmount <= 0 {
		return nil, discountapi.ErrManualDiscountInvalidAmount
	}
	subtotal := totalSubtotal(in.Items)
	amount := clampAmount(in.ManualAmount, subtotal)
	return &discountapi.Snapshot{
		Type:        string(model.DiscountTypeManual),
		Amount:      amount,
		Note:        note,
		Allocations: allocate(amount, in.Items),
	}, nil
}

func (s *Service) resolveMasterDiscount(ctx context.Context, in discountapi.ResolveInput) (*discountapi.Snapshot, error) {
	d, err := s.discounts.FindByID(ctx, *in.DiscountID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, discountapi.ErrDiscountNotFound
		}
		return nil, fmt.Errorf("resolve discount for order: lookup %s: %w", *in.DiscountID, err)
	}
	usage, err := s.discounts.CountUsageOne(ctx, d.ID)
	if err != nil {
		return nil, fmt.Errorf("resolve discount for order: count usage: %w", err)
	}
	// Cakupan produk (§28.9) hanya perlu dimuat kalau applies_to="selected" —
	// diskon "all" (mayoritas kasus) tidak butuh query tambahan ini sama
	// sekali.
	var scopedProductIDs []uuid.UUID
	if d.AppliesTo == model.AppliesToSelected {
		scoped, err := s.discounts.ProductIDs(ctx, []uuid.UUID{d.ID})
		if err != nil {
			return nil, fmt.Errorf("resolve discount for order: product scope: %w", err)
		}
		scopedProductIDs = scoped[d.ID]
	}

	// Membership (§30.3) hanya perlu dimuat kalau audience_scope="member" —
	// diskon "all" (mayoritas kasus) tidak butuh query tambahan ini sama
	// sekali, mirror pola cakupan produk di atas.
	var isActiveMember, membershipEnabled bool
	var scopedCustomerIDs []uuid.UUID
	if d.AudienceScope == model.AudienceScopeMember {
		if s.members == nil || s.settings == nil {
			return nil, discountapi.ErrDiscountMembershipUnavailable
		}
		var err error
		membershipEnabled, err = s.settings.GetBool(ctx, settingsapi.KeyMembershipEnabled)
		if err != nil {
			return nil, fmt.Errorf("resolve discount for order: read membership_enabled: %w", err)
		}
		isActiveMember, err = s.members.IsActiveMember(ctx, in.CustomerID)
		if err != nil {
			return nil, fmt.Errorf("resolve discount for order: check active member: %w", err)
		}
		if d.MemberScope != nil && *d.MemberScope == model.MemberScopeSelected {
			scoped, err := s.discounts.CustomerIDs(ctx, []uuid.UUID{d.ID})
			if err != nil {
				return nil, fmt.Errorf("resolve discount for order: member scope: %w", err)
			}
			scopedCustomerIDs = scoped[d.ID]
		}
	}

	// §32.3 — basis hitung tergantung applies_to: "all" memakai SELURUH
	// item, "selected" HANYA item yang product_id-nya ada di
	// scopedProductIDs. Dihitung SEBELUM validateForUse supaya min_subtotal
	// dibandingkan ke basis yang benar (eligible_subtotal), bukan subtotal
	// seluruh order.
	eligibleItems, eligibleSubtotal, eligibleAreaM2, allItemProductIDs := splitEligibleItems(in.Items, d, scopedProductIDs)

	if err := validateForUse(d, eligibleSubtotal, in.Channel, usage, time.Now(),
		allItemProductIDs, scopedProductIDs,
		in.CustomerID, isActiveMember, membershipEnabled, scopedCustomerIDs, eligibleAreaM2); err != nil {
		return nil, err
	}

	amount := computeAmount(d, eligibleSubtotal, eligibleAreaM2)
	snap := &discountapi.Snapshot{
		DiscountID:  &d.ID,
		Code:        d.Code,
		Name:        d.Name,
		Type:        string(d.Type),
		Amount:      amount,
		Note:        strings.TrimSpace(in.Note),
		Allocations: allocate(amount, eligibleItems, d.Type),
	}
	// allocate() only produced entries for eligibleItems — merge in the
	// non-eligible ones at Amount 0 so callers see one entry per LineNo in
	// in.Items, exactly as documented on discountapi.Snapshot.
	snap.Allocations = fillZeroAllocations(snap.Allocations, in.Items)
	switch d.Type {
	case model.DiscountTypePercent:
		if d.ValuePercent != nil {
			snap.Value = *d.ValuePercent
		}
	case model.DiscountTypeNominal, model.DiscountTypeNominalPerM2:
		if d.ValueAmount != nil {
			snap.Value = float64(*d.ValueAmount)
		}
	}
	return snap, nil
}

// splitEligibleItems implements §32.3's "item eligible" rule: applies_to=
// 'all' → every item is eligible; applies_to='selected' → only items whose
// ProductID is in scopedProductIDs. Also returns the full (unfiltered) list
// of every item's ProductID, used by validateForUse's §28.9 scope-match
// check (which needs to know "did ANY of the order's items match", not just
// the eligible subset — an order with zero matching items must fail with
// ErrDiscountProductMismatch, not silently compute a 0 eligible_subtotal).
func splitEligibleItems(items []discountapi.ResolveItem, d *model.Discount, scopedProductIDs []uuid.UUID) (eligible []discountapi.ResolveItem, eligibleSubtotal int64, eligibleAreaM2 float64, allProductIDs []uuid.UUID) {
	allProductIDs = make([]uuid.UUID, 0, len(items))
	for i := range items {
		allProductIDs = append(allProductIDs, items[i].ProductID)
	}
	if d.AppliesTo != model.AppliesToSelected {
		eligible = items
		eligibleSubtotal = totalSubtotal(items)
		for i := range items {
			eligibleAreaM2 += items[i].EffectiveAreaM2()
		}
		return eligible, eligibleSubtotal, eligibleAreaM2, allProductIDs
	}
	eligible = make([]discountapi.ResolveItem, 0, len(items))
	for i := range items {
		if containsUUID(scopedProductIDs, items[i].ProductID) {
			eligible = append(eligible, items[i])
			eligibleSubtotal += items[i].Subtotal
			eligibleAreaM2 += items[i].EffectiveAreaM2()
		}
	}
	return eligible, eligibleSubtotal, eligibleAreaM2, allProductIDs
}

// fillZeroAllocations merges `computed` (allocations for eligible items
// only) with a zero entry for every item in `all` that isn't already present
// — guarantees the final Snapshot.Allocations has exactly one entry per
// LineNo in `all`, in `all`'s original order (§32.3 doc on discountapi.
// Snapshot: "item yang tidak eligible dapat alokasi 0, TETAP muncul").
func fillZeroAllocations(computed []discountapi.ItemAllocation, all []discountapi.ResolveItem) []discountapi.ItemAllocation {
	byLine := make(map[int]int64, len(computed))
	for _, a := range computed {
		byLine[a.LineNo] = a.Amount
	}
	out := make([]discountapi.ItemAllocation, len(all))
	for i := range all {
		out[i] = discountapi.ItemAllocation{LineNo: all[i].LineNo, Amount: byLine[all[i].LineNo]}
	}
	return out
}
