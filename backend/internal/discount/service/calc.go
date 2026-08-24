package service

import (
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/discount/discountapi"
	"github.com/rajaku-printing/backend/internal/discount/model"
)

// computeStatus derives the admin-facing status badge (§28.8) from a
// discount row + its (dihitung, bukan disimpan — §28.4) usage count.
// Precedence: is_active adalah saklar manual admin, jadi dicek PALING
// DULU — "nonaktif" menang atas jadwal/kuota apa pun kalau admin memang
// mematikannya secara manual.
func computeStatus(d *model.Discount, usageCount int64, now time.Time) string {
	switch {
	case !d.IsActive:
		return "nonaktif"
	case d.StartsAt != nil && now.Before(*d.StartsAt):
		return "terjadwal"
	case d.EndsAt != nil && now.After(*d.EndsAt):
		return "kadaluarsa"
	case d.Quota != nil && usageCount >= int64(*d.Quota):
		return "kuota_habis"
	default:
		return "aktif"
	}
}

// validateForUse checks every rule in §28.4 (in the order sentinel errors
// are documented there) and returns the FIRST one that fails, or nil if the
// discount may be used for this subtotal/channel right now.
//
// productID + scopedProductIDs implement §28.9: only checked when
// d.AppliesTo == "selected" — an "all" discount ignores both arguments
// entirely. scopedProductIDs being empty is NEVER treated as "applies to
// everything" (ErrDiscountScopeEmpty), even though the discount's row was
// saved with applies_to="selected" — a product that used to be the sole
// member of the scope can be dropped from it AFTER the discount was created,
// so this must be re-checked here every time, not just at create/update.
func validateForUse(d *model.Discount, subtotal int64, channel string, usageCount int64, now time.Time, productID uuid.UUID, scopedProductIDs []uuid.UUID) error {
	if !d.IsActive {
		return discountapi.ErrDiscountInactive
	}
	if d.StartsAt != nil && now.Before(*d.StartsAt) {
		return discountapi.ErrDiscountNotStarted
	}
	if d.EndsAt != nil && now.After(*d.EndsAt) {
		return discountapi.ErrDiscountExpired
	}
	if d.ChannelScope != model.ChannelScopeAll && string(d.ChannelScope) != channel {
		return discountapi.ErrDiscountChannelMismatch
	}
	if subtotal < d.MinSubtotal {
		return discountapi.ErrDiscountMinSubtotal
	}
	if d.Quota != nil && usageCount >= int64(*d.Quota) {
		return discountapi.ErrDiscountQuotaExhausted
	}
	if d.AppliesTo == model.AppliesToSelected {
		// Cakupan kosong ditolak TANPA SYARAT (temuan review #4) — tidak
		// peduli productID diisi atau tidak, applies_to="selected" dengan
		// discount_products kosong TIDAK PERNAH bisa dipakai.
		if len(scopedProductIDs) == 0 {
			return discountapi.ErrDiscountScopeEmpty
		}
		// productID == uuid.Nil berarti "tidak difilter berdasarkan produk
		// tertentu" (dipakai Applicable ketika caller tidak mengirim
		// product_id sama sekali) — kecocokan produk baru dicek kalau
		// caller benar-benar menyebutkan produknya.
		if productID != uuid.Nil && !containsUUID(scopedProductIDs, productID) {
			return discountapi.ErrDiscountProductMismatch
		}
	}
	return nil
}

// containsUUID reports whether id is present in ids.
func containsUUID(ids []uuid.UUID, id uuid.UUID) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

// computeAmount implements the §28.3 formula for a MASTER discount (percent
// or nominal) against a subtotal:
//
//	percent: hitungan = round(subtotal × value_percent / 100), HALF-UP,
//	         lalu dipotong max_discount_amount kalau ada
//	nominal: hitungan = value_amount
//	amount  = min(hitungan, subtotal)  -- tidak boleh > subtotal
//
// Rounding HALF-UP via math.Round — konsisten dengan calcPerM2 di
// catalog/service/pricing.go.
func computeAmount(d *model.Discount, subtotal int64) int64 {
	var raw int64
	switch d.Type {
	case model.DiscountTypePercent:
		if d.ValuePercent == nil {
			return 0
		}
		raw = int64(math.Round(float64(subtotal) * (*d.ValuePercent) / 100))
		if d.MaxDiscountAmount != nil && raw > *d.MaxDiscountAmount {
			raw = *d.MaxDiscountAmount
		}
	case model.DiscountTypeNominal:
		if d.ValueAmount == nil {
			return 0
		}
		raw = *d.ValueAmount
	default:
		return 0
	}
	return clampAmount(raw, subtotal)
}

// clampAmount jepit hitungan diskon ke [0, subtotal] — dipakai baik untuk
// diskon master (computeAmount) maupun diskon manual (resolve.go).
func clampAmount(raw, subtotal int64) int64 {
	if raw < 0 {
		return 0
	}
	if raw > subtotal {
		return subtotal
	}
	return raw
}

// toDiscountView projects a model.Discount + usage count + product scope
// into the admin-facing DiscountView (§28.8/§28.9), computing the derived
// `status` field. productIDs is always normalized to a non-nil slice so the
// JSON field serializes as `[]`, never `null`.
func toDiscountView(d *model.Discount, usageCount int64, productIDs []uuid.UUID, now time.Time) DiscountView {
	if productIDs == nil {
		productIDs = []uuid.UUID{}
	}
	return DiscountView{
		ID:                d.ID,
		Code:              d.Code,
		Name:              d.Name,
		Type:              string(d.Type),
		ValuePercent:      d.ValuePercent,
		ValueAmount:       d.ValueAmount,
		MaxDiscountAmount: d.MaxDiscountAmount,
		MinSubtotal:       d.MinSubtotal,
		StartsAt:          d.StartsAt,
		EndsAt:            d.EndsAt,
		Quota:             d.Quota,
		UsageCount:        usageCount,
		ChannelScope:      string(d.ChannelScope),
		AppliesTo:         string(d.AppliesTo),
		ProductIDs:        productIDs,
		IsActive:          d.IsActive,
		Status:            computeStatus(d, usageCount, now),
		CreatedAt:         d.CreatedAt,
		UpdatedAt:         d.UpdatedAt,
	}
}
