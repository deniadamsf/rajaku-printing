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
)

// DiscountStore is the storage contract the service depends on — narrowed to
// just what the service needs, so tests can supply a fake without pulling
// GORM. *repository.DiscountRepository satisfies this.
type DiscountStore interface {
	Create(ctx context.Context, d *model.Discount, productIDs []uuid.UUID) error
	FindByID(ctx context.Context, id uuid.UUID) (*model.Discount, error)
	List(ctx context.Context, q string) ([]model.Discount, error)
	ListActiveForChannel(ctx context.Context, channel string, subtotal int64, now time.Time) ([]model.Discount, error)
	CountUsage(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]int64, error)
	CountUsageOne(ctx context.Context, id uuid.UUID) (int64, error)
	SoftDelete(ctx context.Context, p repository.SoftDeleteParams) error
	// ProductIDs returns discount_id -> []product_id for every id given
	// (§28.9 — bulk to avoid N+1 in List/Applicable).
	ProductIDs(ctx context.Context, discountIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error)
	// UpdateWithProducts atomically applies a partial field update AND
	// (kalau replaceProducts true) mengganti SELURUH cakupan produk, DALAM
	// SATU TRANSAKSI (temuan review #1 — dulu dua panggilan/transaksi
	// terpisah, bisa meninggalkan applies_to='selected' dengan cakupan
	// kosong ter-commit sendirian kalau langkah kedua gagal).
	UpdateWithProducts(ctx context.Context, id uuid.UUID, fields map[string]any, replaceProducts bool, productIDs []uuid.UUID) error
}

// Compile-time assertions.
var _ DiscountStore = (*repository.DiscountRepository)(nil)
var _ discountapi.Resolver = (*Service)(nil)

type Service struct {
	discounts DiscountStore
}

func New(discounts DiscountStore) *Service { return &Service{discounts: discounts} }

// Create implements POST /admin/discounts (§28.1/§28.9).
func (s *Service) Create(ctx context.Context, in CreateInput) (*DiscountView, error) {
	d, productIDs, err := buildDiscountForCreate(in)
	if err != nil {
		return nil, err
	}
	if err := s.discounts.Create(ctx, d, productIDs); err != nil {
		if errors.Is(err, repository.ErrCodeConflict) {
			return nil, discountapi.ErrDiscountCodeConflict
		}
		if errors.Is(err, repository.ErrProductNotFound) {
			return nil, discountapi.ErrDiscountProductNotFound
		}
		return nil, fmt.Errorf("create discount: %w", err)
	}
	view := toDiscountView(d, 0, productIDs, time.Now())
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
	view := toDiscountView(d, usage, productMap[id], time.Now())
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

	now := time.Now()
	views := make([]DiscountView, 0, len(rows))
	for i := range rows {
		d := &rows[i]
		v := toDiscountView(d, usage[d.ID], productMap[d.ID], now)
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
// kasir). Mengembalikan hanya diskon yang lolos SEMUA validasi §28.4/§28.9
// untuk subtotal & channel yang diberikan, ditambah preview_amount.
//
// Temuan review #5 — penyaringan dilakukan dengan memanggil validateForUse
// (calc.go), SATU-SATUNYA tempat aturan §28.4/§28.9 boleh hidup, bukan
// menuliskan ulang subset aturannya di sini. Ini juga yang membuat temuan
// #4 (cakupan produk kosong tetap lolos saat product_id tidak dikirim)
// otomatis ikut tertutup: validateForUse menolak cakupan kosong TANPA
// SYARAT, terlepas dari productID diisi atau tidak.
//
// productID (§28.9) — uuid.Nil berarti "tidak difilter berdasarkan produk"
// (kompatibel dengan pemanggil lama yang belum kirim product_id — lihat
// juga guard di validateForUse). Kalau diisi (bukan uuid.Nil), diskon
// applies_to="selected" yang cakupannya tidak menyertakan productID
// DISARING dari hasil — supaya kasir tidak pernah melihat promo yang akan
// ditolak saat disimpan. Diskon applies_to="all" selalu lolos filter ini,
// apa pun productID-nya. Handler bertanggung jawab menolak uuid.Nil yang
// DIKIRIM EKSPLISIT sebagai product_id (400) sebelum sampai di sini.
func (s *Service) Applicable(ctx context.Context, channel string, subtotal int64, productID uuid.UUID) ([]ApplicableView, error) {
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

	out := make([]ApplicableView, 0, len(candidates))
	for i := range candidates {
		d := &candidates[i]
		u := usage[d.ID]
		scoped := productMap[d.ID]
		if err := validateForUse(d, subtotal, channel, u, now, productID, scoped); err != nil {
			continue
		}
		out = append(out, ApplicableView{
			DiscountView:  toDiscountView(d, u, scoped, now),
			PreviewAmount: computeAmount(d, subtotal),
		})
	}
	return out, nil
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

	// §28.9 — tentukan cakupan produk FINAL (daftar baru dari caller, atau
	// daftar existing kalau product_ids tidak dikirim di PATCH ini), lalu
	// validasi terhadap d.AppliesTo yang FINAL (bisa berubah di titik ini
	// lewat applySimpleUpdateFields di atas) — SEBELUM commit apa pun ke DB.
	finalProductIDs, replaceProducts, err := s.resolveProductScopeForUpdate(ctx, id, d.AppliesTo, in.ProductIDs)
	if err != nil {
		return nil, err
	}

	// Temuan review #1 — field diskon & cakupan produk dikirim sebagai SATU
	// panggilan repository (UpdateWithProducts), yang membungkus keduanya
	// dalam SATU transaksi DB. Sebelumnya ini dua panggilan/transaksi
	// terpisah: kalau ReplaceProducts gagal setelah Update sukses,
	// applies_to='selected' bisa ter-commit sendirian TANPA cakupan
	// produknya — state yang §28.9 larang keras.
	if len(fields) > 0 || replaceProducts {
		if err := s.discounts.UpdateWithProducts(ctx, id, fields, replaceProducts, finalProductIDs); err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return nil, discountapi.ErrDiscountNotFound
			}
			if errors.Is(err, repository.ErrCodeConflict) {
				return nil, discountapi.ErrDiscountCodeConflict
			}
			if errors.Is(err, repository.ErrProductNotFound) {
				return nil, discountapi.ErrDiscountProductNotFound
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
	view := toDiscountView(updated, usage, finalProductIDs, time.Now())
	return &view, nil
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

func buildDiscountForCreate(in CreateInput) (*model.Discount, []uuid.UUID, error) {
	code := strings.ToUpper(strings.TrimSpace(in.Code))
	if code == "" {
		return nil, nil, discountapi.ErrDiscountCodeRequired
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, nil, discountapi.ErrDiscountNameRequired
	}
	scope, err := normalizeChannelScope(in.ChannelScope)
	if err != nil {
		return nil, nil, err
	}
	if in.MinSubtotal < 0 {
		return nil, nil, discountapi.ErrDiscountMinSubtotalInvalid
	}
	if in.Quota != nil && *in.Quota <= 0 {
		return nil, nil, discountapi.ErrDiscountQuotaInvalid
	}
	if in.MaxDiscountAmount != nil && *in.MaxDiscountAmount <= 0 {
		return nil, nil, discountapi.ErrDiscountMaxAmountInvalid
	}
	if err := validatePeriod(in.StartsAt, in.EndsAt); err != nil {
		return nil, nil, err
	}
	appliesTo, err := normalizeAppliesTo(in.AppliesTo)
	if err != nil {
		return nil, nil, err
	}
	productIDs := dedupeUUIDs(in.ProductIDs)
	if err := validateProductScope(appliesTo, productIDs); err != nil {
		return nil, nil, err
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
		IsActive:          in.IsActive,
	}
	if err := setDiscountTypeValue(d, in.Type, in.ValuePercent, in.ValueAmount); err != nil {
		return nil, nil, err
	}
	// Temuan review #9(b) — max_discount_amount cuma berlaku untuk diskon
	// type=percent; computeAmount (calc.go) mengabaikannya total untuk
	// nominal, jadi diisi di sana tanpa efek = jebakan diam-diam bagi admin.
	if d.Type == model.DiscountTypeNominal && d.MaxDiscountAmount != nil {
		return nil, nil, discountapi.ErrDiscountMaxAmountNotAllowed
	}
	return d, productIDs, nil
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
