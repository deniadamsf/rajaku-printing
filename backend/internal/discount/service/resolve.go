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

func resolveManualDiscount(in discountapi.ResolveInput) (*discountapi.Snapshot, error) {
	note := strings.TrimSpace(in.Note)
	if note == "" {
		return nil, discountapi.ErrManualDiscountNoteRequired
	}
	if in.ManualAmount <= 0 {
		return nil, discountapi.ErrManualDiscountInvalidAmount
	}
	return &discountapi.Snapshot{
		Type:   string(model.DiscountTypeManual),
		Amount: clampAmount(in.ManualAmount, in.Subtotal),
		Note:   note,
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

	if err := validateForUse(d, in.Subtotal, in.Channel, usage, time.Now(),
		in.ProductID, scopedProductIDs,
		in.CustomerID, isActiveMember, membershipEnabled, scopedCustomerIDs); err != nil {
		return nil, err
	}

	snap := &discountapi.Snapshot{
		DiscountID: &d.ID,
		Code:       d.Code,
		Name:       d.Name,
		Type:       string(d.Type),
		Amount:     computeAmount(d, in.Subtotal),
		Note:       strings.TrimSpace(in.Note),
	}
	switch d.Type {
	case model.DiscountTypePercent:
		if d.ValuePercent != nil {
			snap.Value = *d.ValuePercent
		}
	case model.DiscountTypeNominal:
		if d.ValueAmount != nil {
			snap.Value = float64(*d.ValueAmount)
		}
	}
	return snap, nil
}
