package service

import (
	"math"
	"sort"
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

// validateForUse checks every rule in §28.4/§32.3 and returns the FIRST one
// that fails, or nil if the discount may be used for this subtotal/channel
// right now.
//
// Urutan pemeriksaan (temuan review §32.3/§28.4 — WAJIB dalam urutan ini,
// jangan diubah tanpa alasan): is_active → masa berlaku → channel → CAKUPAN
// PRODUK (§28.9) → eligible_subtotal > 0 → min_subtotal → kuota → cakupan
// member (§30.3). Cakupan produk dicek SEBELUM min_subtotal dengan sengaja:
// min_subtotal dibandingkan terhadap eligible_subtotal (basis yang sudah
// disaring per cakupan produk, bukan subtotal seluruh keranjang), jadi kalau
// urutannya dibalik, kasir/pembeli akan melihat pesan "subtotal minimum
// tidak terpenuhi" padahal akar masalahnya "tidak ada produk yang cocok" —
// pesan yang salah untuk masalah yang sebenar-benarnya berbeda.
//
// subtotal — basis hitung (§32.3): pemanggil (resolveMasterDiscount) sudah
// menghitung eligible_subtotal SEBELUM memanggil ini untuk applies_to=
// "selected" (Σ subtotal item yang product_id-nya ada di scopedProductIDs),
// jadi min_subtotal di bawah otomatis dibandingkan ke basis yang benar tanpa
// fungsi ini perlu tahu konsep "item" sama sekali.
//
// productIDs + scopedProductIDs implement §28.9/§32.3: only checked when
// d.AppliesTo == "selected" — an "all" discount ignores both arguments
// entirely. scopedProductIDs being empty is NEVER treated as "applies to
// everything" (ErrDiscountScopeEmpty), even though the discount's row was
// saved with applies_to="selected" — a product that used to be the sole
// member of the scope can be dropped from it AFTER the discount was created,
// so this must be re-checked here every time, not just at create/update.
// productIDs is the set of product_id across every candidate item (order
// items, or the cart being previewed by Applicable) — an empty slice means
// "not filtering by product" (Applicable's compatibility case, no product_id
// sent at all); a non-empty slice with NO overlap against scopedProductIDs
// means none of the candidate items are eligible → ErrDiscountProductMismatch.
//
// customerID + isActiveMember + membershipEnabled + scopedCustomerIDs
// implement §30.3: only checked when d.AudienceScope == "member" — an "all"
// audience discount ignores all four arguments entirely. Checked AFTER the
// §28.9 product scope, so callers see the same ordering documented in
// §28.4/§30.3 (channel/period/quota/product scope first, member scope last).
func validateForUse(d *model.Discount, subtotal int64, channel string, usageCount int64, now time.Time,
	productIDs []uuid.UUID, scopedProductIDs []uuid.UUID,
	customerID uuid.UUID, isActiveMember bool, membershipEnabled bool, scopedCustomerIDs []uuid.UUID,
	areaM2 ...float64) error {
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
	// §28.9/§32.3 — cakupan produk WAJIB ditentukan SEBELUM min_subtotal.
	// Lihat doc-comment di atas fungsi ini untuk alasannya.
	if d.AppliesTo == model.AppliesToSelected {
		// Cakupan kosong ditolak TANPA SYARAT (temuan review #4) — tidak
		// peduli productIDs diisi atau tidak, applies_to="selected" dengan
		// discount_products kosong TIDAK PERNAH bisa dipakai.
		if len(scopedProductIDs) == 0 {
			return discountapi.ErrDiscountScopeEmpty
		}
		// productIDs kosong berarti "tidak difilter berdasarkan produk
		// tertentu" (dipakai Applicable ketika caller tidak mengirim
		// product_id sama sekali) — kecocokan produk baru dicek kalau
		// caller benar-benar menyebutkan produknya.
		if len(productIDs) > 0 && !containsAnyUUID(scopedProductIDs, productIDs) {
			return discountapi.ErrDiscountProductMismatch
		}
	}
	// §32.3 — eligible_subtotal <= 0 berarti tidak ada satu pun item yang
	// benar-benar menyumbang nilai ke basis diskon ini (mis. cakupan produk
	// cocok tapi item itu bernilai Rp0, atau applies_to='all' pada order yang
	// entah bagaimana bernilai nol) — tolak eksplisit dengan sentinel yang
	// sama seperti "tidak ada produk cocok" (ErrDiscountProductMismatch),
	// supaya diskon begini tidak lolos dengan potongan Rp0 sambil tetap
	// memakan satu slot kuota (§28.4 menghitung kuota dari orders.discount_id).
	if subtotal <= 0 {
		return discountapi.ErrDiscountProductMismatch
	}
	var eligibleArea float64
	if len(areaM2) > 0 {
		eligibleArea = areaM2[0]
	}
	if d.Type == model.DiscountTypeNominalPerM2 && eligibleArea <= 0 {
		return discountapi.ErrDiscountProductMismatch
	}
	if subtotal < d.MinSubtotal {
		return discountapi.ErrDiscountMinSubtotal
	}
	if d.Quota != nil && usageCount >= int64(*d.Quota) {
		return discountapi.ErrDiscountQuotaExhausted
	}
	if d.AudienceScope == model.AudienceScopeMember {
		// §30.1 — ditolak SAAT DIPAKAI, bukan cuma disembunyikan di UI. Dicek
		// duluan sebelum status member: kalau fiturnya mati total, tidak ada
		// gunanya membedakan "bukan member" dari "member" — keduanya sama2
		// tidak bisa memakainya.
		if !membershipEnabled {
			return discountapi.ErrDiscountMembershipDisabled
		}
		if !isActiveMember {
			return discountapi.ErrDiscountMembershipRequired
		}
		if d.MemberScope != nil && *d.MemberScope == model.MemberScopeSelected {
			// Cakupan kosong ditolak TANPA SYARAT (mirror §28.9 di atas) —
			// member_scope="selected_members" dengan discount_customers
			// kosong TIDAK PERNAH bisa dipakai, terlepas dari customerID
			// diisi atau tidak.
			if len(scopedCustomerIDs) == 0 {
				return discountapi.ErrDiscountMemberScopeEmpty
			}
			// customerID == uuid.Nil berarti "tidak difilter berdasarkan
			// customer tertentu" (dipakai Applicable ketika caller tidak
			// mengirim customer_id sama sekali).
			if customerID != uuid.Nil && !containsUUID(scopedCustomerIDs, customerID) {
				return discountapi.ErrDiscountMemberMismatch
			}
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

// containsAnyUUID reports whether at least one of `candidates` is present in
// `ids` (§32.3 — "diskon applies_to='selected' muncul kalau minimal satu
// product_id cocok").
func containsAnyUUID(ids []uuid.UUID, candidates []uuid.UUID) bool {
	for _, c := range candidates {
		if containsUUID(ids, c) {
			return true
		}
	}
	return false
}

// computeAmount implements the §28.3 formula for a MASTER discount (percent,
// nominal, or nominal_per_m2) against a subtotal:
//
//	percent:        hitungan = round(subtotal × value_percent / 100), HALF-UP,
//	                lalu dipotong max_discount_amount kalau ada
//	nominal:        hitungan = value_amount
//	nominal_per_m2: hitungan = round(eligibleAreaM2 × value_amount), HALF-UP,
//	                lalu dipotong max_discount_amount kalau ada
//	amount        = min(hitungan, subtotal)  -- tidak boleh > subtotal
//
// Rounding HALF-UP via math.Round — konsisten dengan calcPerM2 di
// catalog/service/pricing.go.
func computeAmount(d *model.Discount, subtotal int64, areaM2 ...float64) int64 {
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
	case model.DiscountTypeNominalPerM2:
		if d.ValueAmount == nil {
			return 0
		}
		var eligibleArea float64
		if len(areaM2) > 0 {
			eligibleArea = areaM2[0]
		}
		if eligibleArea <= 0 {
			return 0
		}
		raw = int64(math.Round(eligibleArea * float64(*d.ValueAmount)))
		if d.MaxDiscountAmount != nil && raw > *d.MaxDiscountAmount {
			raw = *d.MaxDiscountAmount
		}
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

// allocate implements §32.3's "metode sisa terbesar" (largest remainder
// method): `amount` is split across `items` proportionally to each item's
// Subtotal (or AreaM2 if typ is nominal_per_m2), floored, then the leftover
// rupiah is handed out ONE AT A TIME to the items with the largest fractional
// remainder — ties broken by the smallest LineNo.
func allocate(amount int64, items []discountapi.ResolveItem, typ ...model.DiscountType) []discountapi.ItemAllocation {
	out := make([]discountapi.ItemAllocation, len(items))
	for i := range items {
		out[i] = discountapi.ItemAllocation{LineNo: items[i].LineNo, Amount: 0}
	}
	if amount <= 0 || len(items) == 0 {
		return out
	}

	if len(typ) > 0 && typ[0] == model.DiscountTypeNominalPerM2 {
		var totalArea float64
		for i := range items {
			totalArea += items[i].EffectiveAreaM2()
		}
		if totalArea > 0 {
			type remainder struct {
				idx  int
				frac float64
				line int
			}
			remainders := make([]remainder, len(items))
			var allocated int64
			for i := range items {
				itemArea := items[i].EffectiveAreaM2()
				exact := float64(amount) * itemArea / totalArea
				floor := int64(math.Floor(exact))
				out[i].Amount = floor
				allocated += floor
				remainders[i] = remainder{idx: i, frac: exact - float64(floor), line: items[i].LineNo}
			}

			leftover := amount - allocated
			sort.SliceStable(remainders, func(a, b int) bool {
				if remainders[a].frac != remainders[b].frac {
					return remainders[a].frac > remainders[b].frac
				}
				return remainders[a].line < remainders[b].line
			})
			for i := int64(0); i < leftover && int(i) < len(remainders); i++ {
				out[remainders[i].idx].Amount++
			}
			return out
		}
	}

	var basis int64
	for i := range items {
		basis += items[i].Subtotal
	}
	if basis <= 0 {
		// Tidak ada dasar proporsional yang masuk akal (semua item subtotal
		// 0) — jangan bagi dengan nol, biarkan semua 0 apa adanya.
		return out
	}

	type remainder struct {
		idx  int
		frac float64
		line int
	}
	remainders := make([]remainder, len(items))
	var allocated int64
	for i := range items {
		exact := float64(amount) * float64(items[i].Subtotal) / float64(basis)
		floor := int64(math.Floor(exact))
		out[i].Amount = floor
		allocated += floor
		remainders[i] = remainder{idx: i, frac: exact - float64(floor), line: items[i].LineNo}
	}

	leftover := amount - allocated
	sort.SliceStable(remainders, func(a, b int) bool {
		if remainders[a].frac != remainders[b].frac {
			return remainders[a].frac > remainders[b].frac
		}
		return remainders[a].line < remainders[b].line
	})
	for i := int64(0); i < leftover && int(i) < len(remainders); i++ {
		out[remainders[i].idx].Amount++
	}
	return out
}

// toDiscountView projects a model.Discount + usage count + product/member
// scope into the admin-facing DiscountView (§28.8/§28.9/§30.3), computing
// the derived `status` field. productIDs/customerIDs are always normalized
// to a non-nil slice so the JSON field serializes as `[]`, never `null`.
func toDiscountView(d *model.Discount, usageCount int64, productIDs []uuid.UUID, customerIDs []uuid.UUID, now time.Time) DiscountView {
	if productIDs == nil {
		productIDs = []uuid.UUID{}
	}
	if customerIDs == nil {
		customerIDs = []uuid.UUID{}
	}
	var memberScope string
	if d.MemberScope != nil {
		memberScope = string(*d.MemberScope)
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
		AudienceScope:     string(d.AudienceScope),
		MemberScope:       memberScope,
		CustomerIDs:       customerIDs,
		IsActive:          d.IsActive,
		Status:            computeStatus(d, usageCount, now),
		CreatedAt:         d.CreatedAt,
		UpdatedAt:         d.UpdatedAt,
	}
}
