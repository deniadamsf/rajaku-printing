package service

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/discount/discountapi"
	"github.com/rajaku-printing/backend/internal/discount/model"
)

func floatPtr(f float64) *float64 { return &f }
func int64Ptr(v int64) *int64     { return &v }
func intPtr(v int) *int           { return &v }

// TestComputeAmount_Percent_HappyPath — §28.3: round HALF-UP, tanpa cap.
func TestComputeAmount_Percent_HappyPath(t *testing.T) {
	d := &model.Discount{Type: model.DiscountTypePercent, ValuePercent: floatPtr(25)}
	got := computeAmount(d, 250_000)
	if got != 62_500 {
		t.Fatalf("computeAmount() = %d, want 62500", got)
	}
}

// TestComputeAmount_Percent_RoundsHalfUp — 33.33% dari 100.000 = 33330 pas
// (tanpa desimal); pakai kasus yang benar2 .5 supaya round-half-up teruji:
// 12.5% dari 1000 = 125 pas (tidak menguji pembulatan). Gunakan subtotal yang
// menghasilkan pecahan .5: value_percent=15, subtotal=333 -> 49.95 -> round
// HALF-UP -> 50.
func TestComputeAmount_Percent_RoundsHalfUp(t *testing.T) {
	d := &model.Discount{Type: model.DiscountTypePercent, ValuePercent: floatPtr(15)}
	got := computeAmount(d, 333)
	if got != 50 {
		t.Fatalf("computeAmount() = %d, want 50 (round half-up of 49.95)", got)
	}
}

// TestComputeAmount_Percent_CappedByMaxDiscount — §28.3: hitungan dipotong
// max_discount_amount kalau ada.
func TestComputeAmount_Percent_CappedByMaxDiscount(t *testing.T) {
	d := &model.Discount{
		Type:              model.DiscountTypePercent,
		ValuePercent:      floatPtr(25),
		MaxDiscountAmount: int64Ptr(50_000),
	}
	got := computeAmount(d, 1_000_000) // 25% = 250.000, dipotong ke 50.000
	if got != 50_000 {
		t.Fatalf("computeAmount() = %d, want 50000 (capped)", got)
	}
}

// TestComputeAmount_ClampedToSubtotal — hitungan tidak boleh > subtotal
// (nominal fixed lebih besar dari subtotal order).
func TestComputeAmount_ClampedToSubtotal(t *testing.T) {
	d := &model.Discount{Type: model.DiscountTypeNominal, ValueAmount: int64Ptr(100_000)}
	got := computeAmount(d, 30_000)
	if got != 30_000 {
		t.Fatalf("computeAmount() = %d, want 30000 (clamped to subtotal)", got)
	}
}

func baseDiscount(now time.Time) *model.Discount {
	return &model.Discount{
		IsActive:     true,
		ChannelScope: model.ChannelScopeAll,
		MinSubtotal:  0,
	}
}

func TestValidateForUse_HappyPath(t *testing.T) {
	now := time.Now()
	d := baseDiscount(now)
	if err := validateForUse(d, 100_000, "pos", 0, now, uuid.Nil, nil, uuid.Nil, false, false, nil); err != nil {
		t.Fatalf("validateForUse() error = %v, want nil", err)
	}
}

func TestValidateForUse_Inactive(t *testing.T) {
	now := time.Now()
	d := baseDiscount(now)
	d.IsActive = false
	if err := validateForUse(d, 100_000, "pos", 0, now, uuid.Nil, nil, uuid.Nil, false, false, nil); err != discountapi.ErrDiscountInactive {
		t.Fatalf("validateForUse() error = %v, want ErrDiscountInactive", err)
	}
}

func TestValidateForUse_NotStarted(t *testing.T) {
	now := time.Now()
	d := baseDiscount(now)
	future := now.Add(24 * time.Hour)
	d.StartsAt = &future
	if err := validateForUse(d, 100_000, "pos", 0, now, uuid.Nil, nil, uuid.Nil, false, false, nil); err != discountapi.ErrDiscountNotStarted {
		t.Fatalf("validateForUse() error = %v, want ErrDiscountNotStarted", err)
	}
}

func TestValidateForUse_Expired(t *testing.T) {
	now := time.Now()
	d := baseDiscount(now)
	past := now.Add(-24 * time.Hour)
	d.EndsAt = &past
	if err := validateForUse(d, 100_000, "pos", 0, now, uuid.Nil, nil, uuid.Nil, false, false, nil); err != discountapi.ErrDiscountExpired {
		t.Fatalf("validateForUse() error = %v, want ErrDiscountExpired", err)
	}
}

func TestValidateForUse_ChannelMismatch(t *testing.T) {
	now := time.Now()
	d := baseDiscount(now)
	d.ChannelScope = model.ChannelScopeOnline
	if err := validateForUse(d, 100_000, "pos", 0, now, uuid.Nil, nil, uuid.Nil, false, false, nil); err != discountapi.ErrDiscountChannelMismatch {
		t.Fatalf("validateForUse() error = %v, want ErrDiscountChannelMismatch", err)
	}
}

func TestValidateForUse_MinSubtotal(t *testing.T) {
	now := time.Now()
	d := baseDiscount(now)
	d.MinSubtotal = 200_000
	if err := validateForUse(d, 100_000, "pos", 0, now, uuid.Nil, nil, uuid.Nil, false, false, nil); err != discountapi.ErrDiscountMinSubtotal {
		t.Fatalf("validateForUse() error = %v, want ErrDiscountMinSubtotal", err)
	}
}

func TestValidateForUse_QuotaExhausted(t *testing.T) {
	now := time.Now()
	d := baseDiscount(now)
	d.Quota = intPtr(5)
	if err := validateForUse(d, 100_000, "pos", 5, now, uuid.Nil, nil, uuid.Nil, false, false, nil); err != discountapi.ErrDiscountQuotaExhausted {
		t.Fatalf("validateForUse() error = %v, want ErrDiscountQuotaExhausted", err)
	}
	// One below quota should still pass.
	if err := validateForUse(d, 100_000, "pos", 4, now, uuid.Nil, nil, uuid.Nil, false, false, nil); err != nil {
		t.Fatalf("validateForUse() error = %v, want nil (usage below quota)", err)
	}
}

// ---- Cakupan diskon per produk (§28.9) ----

func TestValidateForUse_SelectedScope_ProductMatches(t *testing.T) {
	now := time.Now()
	d := baseDiscount(now)
	d.AppliesTo = model.AppliesToSelected
	productID := uuid.New()
	if err := validateForUse(d, 100_000, "pos", 0, now, productID, []uuid.UUID{productID}, uuid.Nil, false, false, nil); err != nil {
		t.Fatalf("validateForUse() error = %v, want nil", err)
	}
}

func TestValidateForUse_SelectedScope_ProductMismatch(t *testing.T) {
	now := time.Now()
	d := baseDiscount(now)
	d.AppliesTo = model.AppliesToSelected
	scopedID := uuid.New()
	otherID := uuid.New()
	if err := validateForUse(d, 100_000, "pos", 0, now, otherID, []uuid.UUID{scopedID}, uuid.Nil, false, false, nil); err != discountapi.ErrDiscountProductMismatch {
		t.Fatalf("validateForUse() error = %v, want ErrDiscountProductMismatch", err)
	}
}

// TestValidateForUse_SelectedScope_EmptyListNeverMatchesAll — §28.9 aturan
// keras: daftar produk kosong TIDAK berarti "berlaku untuk semua", bahkan
// waktu diskon benar-benar dipakai (bukan hanya saat create/update) — produk
// satu-satunya anggota cakupan bisa saja sudah dilepas setelah diskon dibuat.
func TestValidateForUse_SelectedScope_EmptyListNeverMatchesAll(t *testing.T) {
	now := time.Now()
	d := baseDiscount(now)
	d.AppliesTo = model.AppliesToSelected
	if err := validateForUse(d, 100_000, "pos", 0, now, uuid.New(), nil, uuid.Nil, false, false, nil); err != discountapi.ErrDiscountScopeEmpty {
		t.Fatalf("validateForUse() error = %v, want ErrDiscountScopeEmpty", err)
	}
}

func TestValidateForUse_AppliesToAll_IgnoresProductID(t *testing.T) {
	now := time.Now()
	d := baseDiscount(now)
	d.AppliesTo = model.AppliesToAll
	if err := validateForUse(d, 100_000, "pos", 0, now, uuid.New(), nil, uuid.Nil, false, false, nil); err != nil {
		t.Fatalf("validateForUse() error = %v, want nil (applies_to=all ignores product_id)", err)
	}
}

// ---- Diskon khusus member (§30.3) ----

func memberScopePtr(ms model.MemberScope) *model.MemberScope { return &ms }

func TestValidateForUse_Member_HappyPath_AllMembers(t *testing.T) {
	now := time.Now()
	d := baseDiscount(now)
	d.AudienceScope = model.AudienceScopeMember
	d.MemberScope = memberScopePtr(model.MemberScopeAllMembers)
	if err := validateForUse(d, 100_000, "pos", 0, now, uuid.Nil, nil,
		uuid.New(), true, true, nil); err != nil {
		t.Fatalf("validateForUse() error = %v, want nil", err)
	}
}

// TestValidateForUse_Member_MembershipDisabled — §30.1: ditolak SAAT
// DIPAKAI kalau membership_enabled=false, terlepas dari status member
// customer-nya.
func TestValidateForUse_Member_MembershipDisabled(t *testing.T) {
	now := time.Now()
	d := baseDiscount(now)
	d.AudienceScope = model.AudienceScopeMember
	d.MemberScope = memberScopePtr(model.MemberScopeAllMembers)
	if err := validateForUse(d, 100_000, "pos", 0, now, uuid.Nil, nil,
		uuid.New(), true, false, nil); err != discountapi.ErrDiscountMembershipDisabled {
		t.Fatalf("validateForUse() error = %v, want ErrDiscountMembershipDisabled", err)
	}
}

// TestValidateForUse_Member_NotActiveMember — kasus gagal wajib: customer
// bukan member aktif (termasuk guest tanpa customer_id, direpresentasikan
// isActiveMember=false).
func TestValidateForUse_Member_NotActiveMember(t *testing.T) {
	now := time.Now()
	d := baseDiscount(now)
	d.AudienceScope = model.AudienceScopeMember
	d.MemberScope = memberScopePtr(model.MemberScopeAllMembers)
	if err := validateForUse(d, 100_000, "pos", 0, now, uuid.Nil, nil,
		uuid.New(), false, true, nil); err != discountapi.ErrDiscountMembershipRequired {
		t.Fatalf("validateForUse() error = %v, want ErrDiscountMembershipRequired", err)
	}
}

func TestValidateForUse_Member_SelectedScope_Matches(t *testing.T) {
	now := time.Now()
	d := baseDiscount(now)
	d.AudienceScope = model.AudienceScopeMember
	d.MemberScope = memberScopePtr(model.MemberScopeSelected)
	customerID := uuid.New()
	if err := validateForUse(d, 100_000, "pos", 0, now, uuid.Nil, nil,
		customerID, true, true, []uuid.UUID{customerID}); err != nil {
		t.Fatalf("validateForUse() error = %v, want nil", err)
	}
}

func TestValidateForUse_Member_SelectedScope_Mismatch(t *testing.T) {
	now := time.Now()
	d := baseDiscount(now)
	d.AudienceScope = model.AudienceScopeMember
	d.MemberScope = memberScopePtr(model.MemberScopeSelected)
	scopedID := uuid.New()
	otherID := uuid.New()
	if err := validateForUse(d, 100_000, "pos", 0, now, uuid.Nil, nil,
		otherID, true, true, []uuid.UUID{scopedID}); err != discountapi.ErrDiscountMemberMismatch {
		t.Fatalf("validateForUse() error = %v, want ErrDiscountMemberMismatch", err)
	}
}

// TestValidateForUse_Member_SelectedScope_EmptyListNeverMatchesAll — mirror
// §28.9's empty-list rule: member_scope="selected_members" dengan
// discount_customers kosong TIDAK PERNAH berarti "berlaku untuk semua
// member", walau customer-nya sungguhan member aktif.
func TestValidateForUse_Member_SelectedScope_EmptyListNeverMatchesAll(t *testing.T) {
	now := time.Now()
	d := baseDiscount(now)
	d.AudienceScope = model.AudienceScopeMember
	d.MemberScope = memberScopePtr(model.MemberScopeSelected)
	if err := validateForUse(d, 100_000, "pos", 0, now, uuid.Nil, nil,
		uuid.New(), true, true, nil); err != discountapi.ErrDiscountMemberScopeEmpty {
		t.Fatalf("validateForUse() error = %v, want ErrDiscountMemberScopeEmpty", err)
	}
}

func TestValidateForUse_AudienceAll_IgnoresMembershipArgs(t *testing.T) {
	now := time.Now()
	d := baseDiscount(now)
	d.AudienceScope = model.AudienceScopeAll
	if err := validateForUse(d, 100_000, "pos", 0, now, uuid.Nil, nil,
		uuid.Nil, false, false, nil); err != nil {
		t.Fatalf("validateForUse() error = %v, want nil (audience_scope=all ignores membership args)", err)
	}
}

func TestComputeStatus_Precedence(t *testing.T) {
	now := time.Now()
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)

	// is_active=false wins over everything else.
	d := baseDiscount(now)
	d.IsActive = false
	d.StartsAt = &future
	if got := computeStatus(d, 0, now); got != "nonaktif" {
		t.Fatalf("computeStatus() = %q, want nonaktif", got)
	}

	d2 := baseDiscount(now)
	d2.StartsAt = &future
	if got := computeStatus(d2, 0, now); got != "terjadwal" {
		t.Fatalf("computeStatus() = %q, want terjadwal", got)
	}

	d3 := baseDiscount(now)
	d3.EndsAt = &past
	if got := computeStatus(d3, 0, now); got != "kadaluarsa" {
		t.Fatalf("computeStatus() = %q, want kadaluarsa", got)
	}

	d4 := baseDiscount(now)
	d4.Quota = intPtr(3)
	if got := computeStatus(d4, 3, now); got != "kuota_habis" {
		t.Fatalf("computeStatus() = %q, want kuota_habis", got)
	}

	d5 := baseDiscount(now)
	if got := computeStatus(d5, 0, now); got != "aktif" {
		t.Fatalf("computeStatus() = %q, want aktif", got)
	}
}
