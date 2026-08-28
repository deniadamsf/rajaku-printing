// Package service holds discount module business logic (§28).
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
	"github.com/rajaku-printing/backend/internal/membership/membershipapi"
	"github.com/rajaku-printing/backend/internal/settings/settingsapi"
)

// DiscountStore is the storage contract the service depends on — narrowed to
// just what the service needs, so tests can supply a fake without pulling
// GORM. *repository.DiscountRepository satisfies this.
type DiscountStore interface {
	Create(ctx context.Context, d *model.Discount, productIDs, customerIDs []uuid.UUID) error
	FindByID(ctx context.Context, id uuid.UUID) (*model.Discount, error)
	List(ctx context.Context, q string) ([]model.Discount, error)
	ListActiveForChannel(ctx context.Context, channel string, subtotal int64, now time.Time) ([]model.Discount, error)
	CountUsage(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]int64, error)
	CountUsageOne(ctx context.Context, id uuid.UUID) (int64, error)
	SoftDelete(ctx context.Context, p repository.SoftDeleteParams) error
	// ProductIDs returns discount_id -> []product_id for every id given
	// (§28.9 — bulk to avoid N+1 in List/Applicable).
	ProductIDs(ctx context.Context, discountIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error)
	// CustomerIDs returns discount_id -> []customer_id for every id given
	// (§30.3 — mirror ProductIDs, bulk to avoid N+1 in List/Applicable).
	CustomerIDs(ctx context.Context, discountIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error)
	// UpdateWithScopes atomically applies a partial field update AND (kalau
	// Replace=true di salah satu/kedua ScopeReplace) mengganti SELURUH
	// cakupan produk dan/atau member, DALAM SATU TRANSAKSI (temuan review
	// #1, extended §30.3 — dulu dua panggilan/transaksi terpisah, bisa
	// meninggalkan applies_to='selected'/audience_scope='member' dengan
	// cakupan kosong ter-commit sendirian kalau langkah berikutnya gagal).
	UpdateWithScopes(ctx context.Context, id uuid.UUID, fields map[string]any, products, customers repository.ScopeReplace) error
}

// Compile-time assertions.
var _ DiscountStore = (*repository.DiscountRepository)(nil)
var _ discountapi.Resolver = (*Service)(nil)

type Service struct {
	discounts DiscountStore
	// members/settings — dependency §30.3, di-wire lewat setter (bukan
	// constructor) karena urutan konstruksi service di server/router.go
	// membuat discountSvc dibuat SEBELUM settingsSvc dan membershipSvc ada
	// (pola sama dengan design/pos SetSettingsReader, membership
	// SetNotifier). nil sampai di-wire — dicek eksplisit di resolve.go
	// (ErrDiscountMembershipUnavailable), BUKAN diam-diam dianggap "bukan
	// member" (§22 no-silent-stub).
	members  membershipapi.Checker
	settings settingsapi.Reader
}

func New(discounts DiscountStore) *Service { return &Service{discounts: discounts} }

// SetMembershipChecker wires the membership module's Checker (§30.3) — used
// by validateForUse/resolveMasterDiscount to test whether an order's
// customer is an active member for audience_scope="member" discounts.
func (s *Service) SetMembershipChecker(m membershipapi.Checker) { s.members = m }

// SetSettingsReader wires the settings module's Reader (§30.1) — used to
// read the membership_enabled saklar for audience_scope="member" discounts.
func (s *Service) SetSettingsReader(r settingsapi.Reader) { s.settings = r }

// Create implements POST /admin/discounts (§28.1/§28.9/§30.3).
func (s *Service) Create(ctx context.Context, in CreateInput) (*DiscountView, error) {
	d, productIDs, customerIDs, err := buildDiscountForCreate(in)
	if err != nil {
		return nil, err
	}
	if err := s.discounts.Create(ctx, d, productIDs, customerIDs); err != nil {
		if errors.Is(err, repository.ErrCodeConflict) {
			return nil, discountapi.ErrDiscountCodeConflict
		}
		if errors.Is(err, repository.ErrProductNotFound) {
			return nil, discountapi.ErrDiscountProductNotFound
		}
		if errors.Is(err, repository.ErrCustomerNotFound) {
			return nil, discountapi.ErrDiscountCustomerNotFound
		}
		return nil, fmt.Errorf("create discount: %w", err)
	}
	view := toDiscountView(d, 0, productIDs, customerIDs, time.Now())
	return &view, nil
}

// Get implements GET /admin/discounts/:id.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*DiscountView, error) {
	d, err := s.discounts.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, discountapi.ErrDiscountNotFound
		}
		return nil, fmt.Errorf("get discount %s: %w", id, err)
	}
	usage, err := s.discounts.CountUsageOne(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get discount %s: count usage: %w", id, err)
	}
	productMap, err := s.discounts.ProductIDs(ctx, []uuid.UUID{id})
	if err != nil {
		return nil, fmt.Errorf("get discount %s: product scope: %w", id, err)
	}
	customerMap, err := s.discounts.CustomerIDs(ctx, []uuid.UUID{id})
	if err != nil {
		return nil, fmt.Errorf("get discount %s: member scope: %w", id, err)
	}
	view := toDiscountView(d, usage, productMap[id], customerMap[id], time.Now())
	return &view, nil
}

// List implements GET /admin/discounts. Status turunan dihitung per-row
// (butuh usage count, lihat calc.go computeStatus) — karena itu filter
// status & paginasi dilakukan di memori, bukan di query SQL. Dataset (jumlah
// program diskon sebuah toko) diasumsikan kecil (puluhan, bukan jutaan).
func (s *Service) List(ctx context.Context, f ListFilter) (*ListResult, error) {
	rows, err := s.discounts.List(ctx, strings.TrimSpace(f.Query))
	if err != nil {
		return nil, fmt.Errorf("list discounts: %w", err)
	}
	ids := make([]uuid.UUID, len(rows))
	for i := range rows {
		ids[i] = rows[i].ID
	}
	usage, err := s.discounts.CountUsage(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list discounts: count usage: %w", err)
	}
	productMap, err := s.discounts.ProductIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list discounts: product scope: %w", err)
	}
	customerMap, err := s.discounts.CustomerIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list discounts: member scope: %w", err)
	}

	now := time.Now()
	views := make([]DiscountView, 0, len(rows))
	for i := range rows {
		d := &rows[i]
		v := toDiscountView(d, usage[d.ID], productMap[d.ID], customerMap[d.ID], now)
		if f.Status != "" && v.Status != f.Status {
			continue
		}
		views = append(views, v)
	}

	page, perPage := normalizePaging(f.Page, f.PerPage)
	total := int64(len(views))
	start := (page - 1) * perPage
	if start > len(views) {
		start = len(views)
	}
	end := start + perPage
	if end > len(views) {
		end = len(views)
	}
	return &ListResult{Items: views[start:end], Total: total, Page: page, PerPage: perPage}, nil
}

// Applicable implements GET /admin/discounts/applicable (§28.5 UI — layar
// kasir). Mengembalikan hanya diskon yang lolos SEMUA validasi
// §28.4/§28.9/§30.3 untuk subtotal & channel yang diberikan, ditambah
// preview_amount.
//
// Temuan review #5 — penyaringan dilakukan dengan memanggil validateForUse
// (calc.go), SATU-SATUNYA tempat aturan §28.4/§28.9/§30.3 boleh hidup, bukan
// menuliskan ulang subset aturannya di sini. Ini juga yang membuat temuan
// #4 (cakupan produk kosong tetap lolos saat product_id tidak dikirim)
// otomatis ikut tertutup: validateForUse menolak cakupan kosong TANPA
// SYARAT, terlepas dari productID diisi atau tidak — begitu juga untuk
// cakupan member (§30.3).
//
// productID (§28.9) — uuid.Nil berarti "tidak difilter berdasarkan produk"
// (kompatibel dengan pemanggil lama yang belum kirim product_id — lihat
// juga guard di validateForUse). Kalau diisi (bukan uuid.Nil), diskon
// applies_to="selected" yang cakupannya tidak menyertakan productID
// DISARING dari hasil — supaya kasir tidak pernah melihat promo yang akan
// ditolak saat disimpan. Diskon applies_to="all" selalu lolos filter ini,
// apa pun productID-nya. Handler bertanggung jawab menolak uuid.Nil yang
// DIKIRIM EKSPLISIT sebagai product_id (400) sebelum sampai di sini.
//
// customerID (§30.3) — uuid.Nil berarti "tidak difilter berdasarkan
// customer" — diskon audience_scope="member" SAMA SEKALI TIDAK MUNCUL di
// hasil untuk kasus ini (aman by default, BEDA dari productID di mana
// applies_to="all" tetap lolos tanpa product_id — di sini "member" TIDAK
// PERNAH lolos tanpa customer_id, karena tanpa customer_id tidak ada cara
// mengecek status membernya sama sekali). Kalau diisi, diskon
// audience_scope="member" yang customer ini TIDAK MEMENUHI SYARAT (bukan
// member aktif, membership_enabled=false, atau selected_members tapi tidak
// match) DISARING dari hasil.
func (s *Service) Applicable(ctx context.Context, channel string, subtotal int64, productID, customerID uuid.UUID) ([]ApplicableView, error) {
	now := time.Now()
	candidates, err := s.discounts.ListActiveForChannel(ctx, channel, subtotal, now)
	if err != nil {
		return nil, fmt.Errorf("list applicable discounts: %w", err)
	}
	ids := make([]uuid.UUID, len(candidates))
	for i := range candidates {
		ids[i] = candidates[i].ID
	}
	// Bulk sekali di depan (bebas N+1), lalu dioper ke validateForUse per
	// kandidat — bukan query ulang per baris.
	usage, err := s.discounts.CountUsage(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list applicable discounts: count usage: %w", err)
	}
	productMap, err := s.discounts.ProductIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list applicable discounts: product scope: %w", err)
	}
	customerMap, err := s.discounts.CustomerIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list applicable discounts: member scope: %w", err)
	}

	isActiveMember, membershipEnabled, err := s.applicableMembershipContext(ctx, customerID, candidates)
	if err != nil {
		return nil, err
	}

	out := make([]ApplicableView, 0, len(candidates))
	for i := range candidates {
		d := &candidates[i]
		// §30.3 — tanpa customer_id, diskon member SAMA SEKALI TIDAK
		// MUNCUL (aman by default), disaring di sini SEBELUM validateForUse
		// (yang kalau dipanggil dengan customerID=uuid.Nil justru akan
		// LOLOS untuk member_scope="all_members" — makna itu benar untuk
		// resolveMasterDiscount [order yang genuinely tanpa customer_id],
		// tapi SALAH untuk layar kasir yang belum tentu sudah memilih
		// pelanggan).
		if d.AudienceScope == model.AudienceScopeMember && customerID == uuid.Nil {
			continue
		}
		u := usage[d.ID]
		scopedProducts := productMap[d.ID]
		scopedCustomers := customerMap[d.ID]
		if err := validateForUse(d, subtotal, channel, u, now,
			productID, scopedProducts,
			customerID, isActiveMember, membershipEnabled, scopedCustomers); err != nil {
			continue
		}
		out = append(out, ApplicableView{
			DiscountView:  toDiscountView(d, u, scopedProducts, scopedCustomers, now),
			PreviewAmount: computeAmount(d, subtotal),
		})
	}
	return out, nil
}

// applicableMembershipContext loads the membership_enabled saklar + the
// caller's active-member status ONCE for the whole Applicable() call —
// only when at least one candidate is audience_scope="member" AND a
// customerID was actually supplied, mirroring the "no extra query for the
// common all-audience case" pattern used throughout this module (§28.9/
// §30.3). Returns zero values (false, false) without error when no
// candidate needs it, or when customerID is uuid.Nil (those discounts get
// filtered out by their own uuid.Nil guard in Applicable's loop anyway).
func (s *Service) applicableMembershipContext(ctx context.Context, customerID uuid.UUID, candidates []model.Discount) (isActiveMember, membershipEnabled bool, err error) {
	if customerID == uuid.Nil {
		return false, false, nil
	}
	needsMembership := false
	for i := range candidates {
		if candidates[i].AudienceScope == model.AudienceScopeMember {
			needsMembership = true
			break
		}
	}
	if !needsMembership {
		return false, false, nil
	}
	if s.members == nil || s.settings == nil {
		return false, false, discountapi.ErrDiscountMembershipUnavailable
	}
	membershipEnabled, err = s.settings.GetBool(ctx, settingsapi.KeyMembershipEnabled)
	if err != nil {
		return false, false, fmt.Errorf("list applicable discounts: read membership_enabled: %w", err)
	}
	isActiveMember, err = s.members.IsActiveMember(ctx, customerID)
	if err != nil {
		return false, false, fmt.Errorf("list applicable discounts: check active member: %w", err)
	}
	return isActiveMember, membershipEnabled, nil
}

// Update implements PATCH /admin/discounts/:id.
func (s *Service) Update(ctx context.Context, id uuid.UUID, in UpdateInput) (*DiscountView, error) {
	d, err := s.discounts.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, discountapi.ErrDiscountNotFound
		}
		return nil, fmt.Errorf("update discount %s: lookup: %w", id, err)
	}

	fields := map[string]any{}
	if err := applySimpleUpdateFields(d, in, fields); err != nil {
		return nil, err
	}
	if err := applyTypeValueUpdate(d, in, fields); err != nil {
		return nil, err
	}
	// Temuan review #9(b) — dicek terhadap NILAI AKHIR d.Type/d.MaxDiscountAmount
	// (setelah kedua apply* di atas jalan), sama seperti buildDiscountForCreate.
	if d.Type == model.DiscountTypeNominal && d.MaxDiscountAmount != nil {
		return nil, discountapi.ErrDiscountMaxAmountNotAllowed
	}

	// §30.3 — reconcile audience_scope<->member_scope FIRST (member_scope
	// bisa auto-cleared di sini kalau audience_scope beralih keluar dari
	// "member" pada PATCH yang sama) SEBELUM resolveCustomerScopeForUpdate
	// membaca d.MemberScope, karena resolver itu butuh nilai FINAL.
	if err := reconcileMemberScope(d, in, fields); err != nil {
		return nil, err
	}

	// §28.9 — tentukan cakupan produk FINAL (daftar baru dari caller, atau
	// daftar existing kalau product_ids tidak dikirim di PATCH ini), lalu
	// validasi terhadap d.AppliesTo yang FINAL (bisa berubah di titik ini
	// lewat applySimpleUpdateFields di atas) — SEBELUM commit apa pun ke DB.
	finalProductIDs, replaceProducts, err := s.resolveProductScopeForUpdate(ctx, id, d.AppliesTo, in.ProductIDs)
	if err != nil {
		return nil, err
	}
	// §30.3 — mirror di atas, tapi untuk cakupan member (discount_customers).
	finalCustomerIDs, replaceCustomers, err := s.resolveCustomerScopeForUpdate(ctx, id, d.MemberScope, in.CustomerIDs)
	if err != nil {
		return nil, err
	}

	// Temuan review #1 — field diskon & cakupan produk/member dikirim
	// sebagai SATU panggilan repository (UpdateWithScopes), yang
	// membungkus semuanya dalam SATU transaksi DB. Sebelumnya field update
	// dan penggantian cakupan adalah panggilan/transaksi terpisah: kalau
	// salah satu gagal setelah yang lain sukses, applies_to='selected'/
	// audience_scope='member' bisa ter-commit sendirian TANPA cakupannya —
	// state yang §28.9/§30.3 larang keras.
	if len(fields) > 0 || replaceProducts || replaceCustomers {
		products := repository.ScopeReplace{Replace: replaceProducts, IDs: finalProductIDs}
		customers := repository.ScopeReplace{Replace: replaceCustomers, IDs: finalCustomerIDs}
		if err := s.discounts.UpdateWithScopes(ctx, id, fields, products, customers); err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return nil, discountapi.ErrDiscountNotFound
			}
			if errors.Is(err, repository.ErrCodeConflict) {
				return nil, discountapi.ErrDiscountCodeConflict
			}
			if errors.Is(err, repository.ErrProductNotFound) {
				return nil, discountapi.ErrDiscountProductNotFound
			}
			if errors.Is(err, repository.ErrCustomerNotFound) {
				return nil, discountapi.ErrDiscountCustomerNotFound
			}
			return nil, fmt.Errorf("update discount %s: %w", id, err)
		}
	}

	updated, err := s.discounts.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("update discount %s: reload: %w", id, err)
	}
	usage, err := s.discounts.CountUsageOne(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("update discount %s: count usage: %w", id, err)
	}
	view := toDiscountView(updated, usage, finalProductIDs, finalCustomerIDs, time.Now())
	return &view, nil
}

// reconcileMemberScope enforces §30.3's audience_scope<->member_scope
// consistency AFTER applySimpleUpdateFields has staged both fields
// (d.AudienceScope/d.MemberScope reflect this PATCH's FINAL values):
//
//  1. Final audience_scope="member" but member_scope ended up nil (never
//     had one, or this PATCH didn't send one while switching audience_scope
//     TO "member") -> reject, wajib diisi.
//  2. Final audience_scope!="member" and the caller EXPLICITLY sent a
//     non-empty member_scope this PATCH -> reject, contradicts audience
//     scope (explicit disagreement is surfaced as an error, not silently
//     resolved).
//  3. Final audience_scope!="member" and member_scope still carries a value
//     ONLY because it wasn't touched by this PATCH (audience_scope itself
//     was switched away from "member" in this same PATCH) -> auto-clear so
//     the row stays consistent with the DB CHECK constraint
//     (chk_discounts_member_scope_consistency, migration 000031), instead
//     of forcing every "leave member audience" PATCH to also explicitly
//     send member_scope:null.
func reconcileMemberScope(d *model.Discount, in UpdateInput, fields map[string]any) error {
	if d.AudienceScope == model.AudienceScopeMember {
		if d.MemberScope == nil {
			return discountapi.ErrDiscountMemberScopeInvalid
		}
		return nil
	}
	if in.MemberScope != nil && *in.MemberScope != "" {
		return discountapi.ErrDiscountMemberScopeInvalid
	}
	if d.MemberScope != nil {
		d.MemberScope = nil
		fields["member_scope"] = nil
	}
	return nil
}

// resolveProductScopeForUpdate figures out the FINAL product scope for a
// PATCH (§28.9): the caller's replacement list when product_ids was sent
// explicitly (replace=true — ReplaceProducts must run), or the discount's
// CURRENT scope left untouched otherwise. Either way, the final scope is
// validated against appliesTo (the discount's FINAL applies_to, after
// applySimpleUpdateFields already applied any change) — applies_to=
// "selected" with an empty final scope is rejected HERE too, not just at
// create, because product_ids might genuinely be omitted from this PATCH
// while an unrelated field (e.g. applies_to itself) switches to "selected".
func (s *Service) resolveProductScopeForUpdate(ctx context.Context, id uuid.UUID, appliesTo model.AppliesToScope, in *[]uuid.UUID) ([]uuid.UUID, bool, error) {
	var final []uuid.UUID
	replace := in != nil
	if replace {
		final = dedupeUUIDs(*in)
	} else {
		productMap, err := s.discounts.ProductIDs(ctx, []uuid.UUID{id})
		if err != nil {
			return nil, false, fmt.Errorf("update discount %s: load existing product scope: %w", id, err)
		}
		final = productMap[id]
	}
	if err := validateProductScope(appliesTo, final); err != nil {
		return nil, false, err
	}
	return final, replace, nil
}

// resolveCustomerScopeForUpdate figures out the FINAL member scope
// (discount_customers) for a PATCH (§30.3) — mirror
// resolveProductScopeForUpdate persis: the caller's replacement list when
// customer_ids was sent explicitly (replace=true — ReplaceCustomers must
// run), or the discount's CURRENT scope left untouched otherwise. Either
// way, the final scope is validated against memberScope (the discount's
// FINAL member_scope, after reconcileMemberScope already ran) —
// member_scope="selected_members" with an empty final scope is rejected
// HERE too, not just at create, because customer_ids might genuinely be
// omitted from this PATCH while an unrelated field (e.g. member_scope
// itself) switches to "selected_members".
func (s *Service) resolveCustomerScopeForUpdate(ctx context.Context, id uuid.UUID, memberScope *model.MemberScope, in *[]uuid.UUID) ([]uuid.UUID, bool, error) {
	var final []uuid.UUID
	replace := in != nil
	if replace {
		final = dedupeUUIDs(*in)
	} else {
		customerMap, err := s.discounts.CustomerIDs(ctx, []uuid.UUID{id})
		if err != nil {
			return nil, false, fmt.Errorf("update discount %s: load existing member scope: %w", id, err)
		}
		final = customerMap[id]
	}
	if err := validateMemberScope(memberScope, final); err != nil {
		return nil, false, err
	}
	return final, replace, nil
}

// Delete implements DELETE /admin/discounts/:id — SOFT delete only, reason
// wajib (§28.1/§28.2). Master tidak pernah hard-deleted supaya rekap/telusur
// balik order lama tetap utuh.
func (s *Service) Delete(ctx context.Context, id uuid.UUID, actorID uuid.UUID, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return discountapi.ErrDiscountDeleteReasonRequired
	}
	if err := s.discounts.SoftDelete(ctx, repository.SoftDeleteParams{
		DiscountID: id,
		ActorID:    actorID,
		Reason:     reason,
	}); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return discountapi.ErrDiscountNotFound
		}
		return fmt.Errorf("delete discount %s: %w", id, err)
	}
	return nil
}

func normalizePaging(page, perPage int) (int, int) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}
	return page, perPage
}

// --- Create/Update validation helpers ---

func buildDiscountForCreate(in CreateInput) (*model.Discount, []uuid.UUID, []uuid.UUID, error) {
	code := strings.ToUpper(strings.TrimSpace(in.Code))
	if code == "" {
		return nil, nil, nil, discountapi.ErrDiscountCodeRequired
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, nil, nil, discountapi.ErrDiscountNameRequired
	}
	scope, err := normalizeChannelScope(in.ChannelScope)
	if err != nil {
		return nil, nil, nil, err
	}
	if in.MinSubtotal < 0 {
		return nil, nil, nil, discountapi.ErrDiscountMinSubtotalInvalid
	}
	if in.Quota != nil && *in.Quota <= 0 {
		return nil, nil, nil, discountapi.ErrDiscountQuotaInvalid
	}
	if in.MaxDiscountAmount != nil && *in.MaxDiscountAmount <= 0 {
		return nil, nil, nil, discountapi.ErrDiscountMaxAmountInvalid
	}
	if err := validatePeriod(in.StartsAt, in.EndsAt); err != nil {
		return nil, nil, nil, err
	}
	appliesTo, err := normalizeAppliesTo(in.AppliesTo)
	if err != nil {
		return nil, nil, nil, err
	}
	productIDs := dedupeUUIDs(in.ProductIDs)
	if err := validateProductScope(appliesTo, productIDs); err != nil {
		return nil, nil, nil, err
	}
	audienceScope, err := normalizeAudienceScope(in.AudienceScope)
	if err != nil {
		return nil, nil, nil, err
	}
	memberScope, err := normalizeMemberScope(in.MemberScope, audienceScope)
	if err != nil {
		return nil, nil, nil, err
	}
	customerIDs := dedupeUUIDs(in.CustomerIDs)
	if err := validateMemberScope(memberScope, customerIDs); err != nil {
		return nil, nil, nil, err
	}

	d := &model.Discount{
		Code:              code,
		Name:              name,
		MaxDiscountAmount: in.MaxDiscountAmount,
		MinSubtotal:       in.MinSubtotal,
		StartsAt:          in.StartsAt,
		EndsAt:            in.EndsAt,
		Quota:             in.Quota,
		ChannelScope:      scope,
		AppliesTo:         appliesTo,
		AudienceScope:     audienceScope,
		MemberScope:       memberScope,
		IsActive:          in.IsActive,
	}
	if err := setDiscountTypeValue(d, in.Type, in.ValuePercent, in.ValueAmount); err != nil {
		return nil, nil, nil, err
	}
	// Temuan review #9(b) — max_discount_amount cuma berlaku untuk diskon
	// type=percent; computeAmount (calc.go) mengabaikannya total untuk
	// nominal, jadi diisi di sana tanpa efek = jebakan diam-diam bagi admin.
	if d.Type == model.DiscountTypeNominal && d.MaxDiscountAmount != nil {
		return nil, nil, nil, discountapi.ErrDiscountMaxAmountNotAllowed
	}
	return d, productIDs, customerIDs, nil
}

// validatePeriod menolak kombinasi starts_at/ends_at di mana ends_at <=
// starts_at (temuan review #3) — diskon begini tersimpan tapi validateForUse
// menolaknya SELAMANYA tanpa petunjuk apapun ke admin/kasir. nil di salah
// satu sisi (atau keduanya) selalu valid — hanya kalau KEDUANYA diisi
// urutannya dicek.
func validatePeriod(startsAt, endsAt *time.Time) error {
	if startsAt == nil || endsAt == nil {
		return nil
	}
	if !endsAt.After(*startsAt) {
		return discountapi.ErrDiscountInvalidPeriod
	}
	return nil
}

// setDiscountTypeValue validates & assigns Type + the matching value column,
// clearing the other one — mirrors the DB CHECK constraint
// chk_discounts_value_consistency (migration 000027) so a bad combination is
// rejected here with a friendly error BEFORE hitting the DB.
func setDiscountTypeValue(d *model.Discount, typ string, valuePercent *float64, valueAmount *int64) error {
	switch model.DiscountType(typ) {
	case model.DiscountTypePercent:
		if valuePercent == nil || *valuePercent <= 0 || *valuePercent > 100 {
			return discountapi.ErrDiscountValuePercentInvalid
		}
		d.Type = model.DiscountTypePercent
		d.ValuePercent = valuePercent
		d.ValueAmount = nil
	case model.DiscountTypeNominal:
		if valueAmount == nil || *valueAmount <= 0 {
			return discountapi.ErrDiscountValueAmountInvalid
		}
		d.Type = model.DiscountTypeNominal
		d.ValueAmount = valueAmount
		d.ValuePercent = nil
	default:
		return discountapi.ErrDiscountTypeInvalid
	}
	return nil
}

func normalizeChannelScope(raw string) (model.ChannelScope, error) {
	scope := model.ChannelScope(raw)
	if scope == "" {
		return model.ChannelScopeAll, nil
	}
	switch scope {
	case model.ChannelScopeAll, model.ChannelScopeOnline, model.ChannelScopePOS:
		return scope, nil
	default:
		return "", discountapi.ErrDiscountChannelScopeInvalid
	}
}

// normalizeChannelScopeForUpdate — temuan review #2. Dipakai HANYA di jalur
// PATCH, BUKAN create: in.ChannelScope adalah *string, jadi non-nil berarti
// caller BENAR-BENAR mengirim field ini (parseUpdateInput sudah membedakan
// "tidak dikirim" [nil] dari "dikirim" lebih dulu). "" DI SINI bukan lagi
// "tidak diisi" seperti di create — itu berarti klien sengaja mengirim
// string kosong, dan HARUS ditolak, bukan diam-diam diperlakukan sebagai
// "all" (yang akan melebarkan cakupan channel diskon tanpa sepengetahuan
// admin — bug kembar dari applies_to, lihat normalizeAppliesToForUpdate).
func normalizeChannelScopeForUpdate(raw string) (model.ChannelScope, error) {
	if raw == "" {
		return "", discountapi.ErrDiscountChannelScopeInvalid
	}
	return normalizeChannelScope(raw)
}

// normalizeAppliesTo — §28.9. "" (tidak dikirim) default ke "all", sama pola
// dengan normalizeChannelScope, supaya diskon yang dibuat tanpa menyebut
// applies_to sama sekali tetap berlaku untuk semua produk (kompatibel dengan
// klien lama yang belum tahu field ini).
func normalizeAppliesTo(raw string) (model.AppliesToScope, error) {
	scope := model.AppliesToScope(raw)
	if scope == "" {
		return model.AppliesToAll, nil
	}
	switch scope {
	case model.AppliesToAll, model.AppliesToSelected:
		return scope, nil
	default:
		return "", discountapi.ErrDiscountAppliesToInvalid
	}
}

// normalizeAppliesToForUpdate — temuan review #2 (kerugian uang): PATCH
// {"applies_to": ""} sebelumnya lolos lewat normalizeAppliesTo biasa, yang
// memperlakukan "" sebagai "tidak diisi" → default "all". Itu benar untuk
// CREATE (field belum pernah ada), tapi SALAH untuk PATCH: in.AppliesTo
// adalah *string, jadi non-nil berarti klien mengirim field ini SECARA
// EKSPLISIT — "" berarti klien benar-benar mengirim string kosong, bukan
// "abaikan field ini". Diam-diam melebarkannya jadi "all" bisa membuat
// diskon yang sengaja dibatasi ke satu produk mendadak berlaku ke seluruh
// katalog. Di sini "" ditolak sebagai input tidak valid (400), titik.
func normalizeAppliesToForUpdate(raw string) (model.AppliesToScope, error) {
	if raw == "" {
		return "", discountapi.ErrDiscountAppliesToInvalid
	}
	return normalizeAppliesTo(raw)
}

// validateProductScope enforces the §28.9 aturan keras: applies_to=
// "selected" dengan daftar produk kosong TIDAK PERNAH boleh diperlakukan
// sebagai "berlaku untuk semua" — ditolak di sini (dipanggil dari jalur
// create/update) DAN lagi di calc.go validateForUse (dipanggil saat diskon
// benar-benar dipakai, karena cakupannya bisa berubah jadi kosong setelah
// diskon dibuat).
func validateProductScope(appliesTo model.AppliesToScope, productIDs []uuid.UUID) error {
	if appliesTo == model.AppliesToSelected && len(productIDs) == 0 {
		return discountapi.ErrDiscountScopeEmpty
	}
	return nil
}

// normalizeAudienceScope — §30.3. "" (tidak dikirim) default ke "all", sama
// pola dengan normalizeAppliesTo/normalizeChannelScope, supaya diskon yang
// dibuat tanpa menyebut audience_scope sama sekali tetap berlaku untuk
// semua orang (kompatibel dengan klien lama yang belum tahu field ini).
func normalizeAudienceScope(raw string) (model.AudienceScope, error) {
	scope := model.AudienceScope(raw)
	if scope == "" {
		return model.AudienceScopeAll, nil
	}
	switch scope {
	case model.AudienceScopeAll, model.AudienceScopeMember:
		return scope, nil
	default:
		return "", discountapi.ErrDiscountAudienceScopeInvalid
	}
}

// normalizeAudienceScopeForUpdate — mirror normalizeAppliesToForUpdate
// (temuan review #2, applies ke sumbu audience juga): PATCH
// {"audience_scope": ""} TIDAK boleh diam-diam diperlakukan sebagai "all" —
// in.AudienceScope adalah *string, jadi non-nil berarti klien mengirim
// field ini SECARA EKSPLISIT, dan "" berarti klien benar-benar mengirim
// string kosong, bukan "abaikan field ini". Ditolak sebagai input tidak
// valid (400), titik — sama seperti applies_to.
func normalizeAudienceScopeForUpdate(raw string) (model.AudienceScope, error) {
	if raw == "" {
		return "", discountapi.ErrDiscountAudienceScopeInvalid
	}
	return normalizeAudienceScope(raw)
}

// normalizeMemberScope validates the (member_scope, audience_scope) pair for
// a CREATE input (§30.3) — audienceScope is the value ALREADY resolved by
// normalizeAudienceScope, so this only needs to check consistency:
//   - audienceScope != "member": raw MUST be "" (member_scope stays nil);
//     any non-empty value here means the caller sent member_scope for a
//     non-member discount, which is rejected outright (not silently
//     ignored — a caller who typed "all_members" almost certainly also
//     meant to set audience_scope="member" and just forgot).
//   - audienceScope == "member": raw MUST be a valid enum value
//     ("all_members" | "selected_members") — required, not optional.
func normalizeMemberScope(raw string, audienceScope model.AudienceScope) (*model.MemberScope, error) {
	if audienceScope != model.AudienceScopeMember {
		if raw != "" {
			return nil, discountapi.ErrDiscountMemberScopeInvalid
		}
		return nil, nil
	}
	switch model.MemberScope(raw) {
	case model.MemberScopeAllMembers, model.MemberScopeSelected:
		ms := model.MemberScope(raw)
		return &ms, nil
	default:
		return nil, discountapi.ErrDiscountMemberScopeInvalid
	}
}

// validateMemberScope enforces the §30.3 aturan keras: member_scope=
// "selected_members" dengan daftar customer kosong TIDAK PERNAH boleh
// diperlakukan sebagai "berlaku untuk semua member" — ditolak di sini
// (dipanggil dari jalur create/update) DAN lagi di calc.go validateForUse
// (dipanggil saat diskon benar-benar dipakai, karena cakupannya bisa
// berubah jadi kosong setelah diskon dibuat). Mirror validateProductScope.
func validateMemberScope(memberScope *model.MemberScope, customerIDs []uuid.UUID) error {
	if memberScope != nil && *memberScope == model.MemberScopeSelected && len(customerIDs) == 0 {
		return discountapi.ErrDiscountMemberScopeEmpty
	}
	return nil
}

// dedupeUUIDs strips duplicate ids while preserving first-seen order — a
// caller sending the same product_id twice must not trip the
// UNIQUE(discount_id, product_id) constraint in discount_products. Also
// strips uuid.Nil (temuan review #3) — a product_ids list containing only
// the zero UUID would otherwise slide past validateProductScope's "len > 0"
// check and blow up as a raw FK violation when inserted.
func dedupeUUIDs(ids []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(ids))
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if id == uuid.Nil {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// applySimpleUpdateFields stages every UpdateInput field EXCEPT
// type/value_percent/value_amount (handled separately by
// applyTypeValueUpdate, since those three must stay consistent together).
func applySimpleUpdateFields(d *model.Discount, in UpdateInput, fields map[string]any) error {
	if in.Code != nil {
		code := strings.ToUpper(strings.TrimSpace(*in.Code))
		if code == "" {
			return discountapi.ErrDiscountCodeRequired
		}
		d.Code = code
		fields["code"] = code
	}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			return discountapi.ErrDiscountNameRequired
		}
		d.Name = name
		fields["name"] = name
	}
	if in.MinSubtotal != nil {
		if *in.MinSubtotal < 0 {
			return discountapi.ErrDiscountMinSubtotalInvalid
		}
		d.MinSubtotal = *in.MinSubtotal
		fields["min_subtotal"] = *in.MinSubtotal
	}
	if in.ChannelScope != nil {
		scope, err := normalizeChannelScopeForUpdate(*in.ChannelScope)
		if err != nil {
			return err
		}
		d.ChannelScope = scope
		fields["channel_scope"] = string(scope)
	}
	if in.AppliesTo != nil {
		scope, err := normalizeAppliesToForUpdate(*in.AppliesTo)
		if err != nil {
			return err
		}
		d.AppliesTo = scope
		fields["applies_to"] = string(scope)
	}
	if in.AudienceScope != nil {
		scope, err := normalizeAudienceScopeForUpdate(*in.AudienceScope)
		if err != nil {
			return err
		}
		d.AudienceScope = scope
		fields["audience_scope"] = string(scope)
	}
	// member_scope — BEDA dengan applies_to/audience_scope: "" (dikirim
	// eksplisit tapi kosong) di sini VALID, artinya "kosongkan member_scope"
	// (final consistency-nya divalidasi ulang di reconcileMemberScope,
	// dipanggil Update() SETELAH applySimpleUpdateFields — d.AudienceScope
	// final belum tentu diketahui di titik ini kalau audience_scope & ini
	// dikirim di urutan JSON key yang berbeda dari asumsi Go map).
	if in.MemberScope != nil {
		if *in.MemberScope == "" {
			d.MemberScope = nil
			fields["member_scope"] = nil
		} else {
			ms := model.MemberScope(*in.MemberScope)
			if ms != model.MemberScopeAllMembers && ms != model.MemberScopeSelected {
				return discountapi.ErrDiscountMemberScopeInvalid
			}
			d.MemberScope = &ms
			fields["member_scope"] = string(ms)
		}
	}
	if in.IsActive != nil {
		d.IsActive = *in.IsActive
		fields["is_active"] = *in.IsActive
	}

	applyClearableInt64(&d.MaxDiscountAmount, in.MaxDiscountAmount, in.ClearMaxDiscountAmount, "max_discount_amount", fields)
	applyClearableTime(&d.StartsAt, in.StartsAt, in.ClearStartsAt, "starts_at", fields)
	applyClearableTime(&d.EndsAt, in.EndsAt, in.ClearEndsAt, "ends_at", fields)
	applyClearableInt(&d.Quota, in.Quota, in.ClearQuota, "quota", fields)

	if fields["max_discount_amount"] != nil {
		if v, ok := fields["max_discount_amount"].(int64); ok && v <= 0 {
			return discountapi.ErrDiscountMaxAmountInvalid
		}
	}
	if fields["quota"] != nil {
		if v, ok := fields["quota"].(int); ok && v <= 0 {
			return discountapi.ErrDiscountQuotaInvalid
		}
	}
	// Temuan review #3 — validasi periode terhadap NILAI AKHIR setelah patch
	// (d.StartsAt/d.EndsAt sudah dimutasi oleh applyClearableTime di atas),
	// bukan cuma field yang dikirim caller — supaya update yang hanya
	// mengubah salah satu dari starts_at/ends_at tetap tervalidasi terhadap
	// nilai lainnya yang sudah tersimpan sebelumnya.
	if err := validatePeriod(d.StartsAt, d.EndsAt); err != nil {
		return err
	}
	// Temuan review #9(b) (max_discount_amount hanya utk type=percent) DICEK
	// DI Update() setelah applyTypeValueUpdate juga jalan — d.Type belum
	// tentu final di titik ini (Type bisa berubah setelahnya).
	return nil
}

// applyTypeValueUpdate handles the Type/ValuePercent/ValueAmount trio.
// Kalau Type diganti, kolom nilai yang SESUAI wajib disertakan (dan yang
// lama otomatis dikosongkan) — sama seperti buildDiscountForCreate. Kalau
// Type TIDAK diganti, ValuePercent/ValueAmount hanya boleh menyentuh kolom
// yang cocok dengan type saat ini.
func applyTypeValueUpdate(d *model.Discount, in UpdateInput, fields map[string]any) error {
	if in.Type != nil {
		if err := setDiscountTypeValue(d, *in.Type, in.ValuePercent, in.ValueAmount); err != nil {
			return err
		}
		fields["type"] = string(d.Type)
		fields["value_percent"] = d.ValuePercent
		fields["value_amount"] = d.ValueAmount
		return nil
	}
	if in.ValuePercent != nil {
		if d.Type != model.DiscountTypePercent {
			return discountapi.ErrDiscountValuePercentInvalid
		}
		if *in.ValuePercent <= 0 || *in.ValuePercent > 100 {
			return discountapi.ErrDiscountValuePercentInvalid
		}
		d.ValuePercent = in.ValuePercent
		fields["value_percent"] = *in.ValuePercent
	}
	if in.ValueAmount != nil {
		if d.Type != model.DiscountTypeNominal {
			return discountapi.ErrDiscountValueAmountInvalid
		}
		if *in.ValueAmount <= 0 {
			return discountapi.ErrDiscountValueAmountInvalid
		}
		d.ValueAmount = in.ValueAmount
		fields["value_amount"] = *in.ValueAmount
	}
	return nil
}

func applyClearableInt64(target **int64, val *int64, clear bool, column string, fields map[string]any) {
	switch {
	case clear:
		*target = nil
		fields[column] = nil
	case val != nil:
		*target = val
		fields[column] = *val
	}
}

func applyClearableInt(target **int, val *int, clear bool, column string, fields map[string]any) {
	switch {
	case clear:
		*target = nil
		fields[column] = nil
	case val != nil:
		*target = val
		fields[column] = *val
	}
}

func applyClearableTime(target **time.Time, val *time.Time, clear bool, column string, fields map[string]any) {
	switch {
	case clear:
		*target = nil
		fields[column] = nil
	case val != nil:
		*target = val
		fields[column] = *val
	}
}
