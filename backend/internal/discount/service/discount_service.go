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
	Create(ctx context.Context, d *model.Discount) error
	FindByID(ctx context.Context, id uuid.UUID) (*model.Discount, error)
	List(ctx context.Context, q string) ([]model.Discount, error)
	ListActiveForChannel(ctx context.Context, channel string, subtotal int64, now time.Time) ([]model.Discount, error)
	CountUsage(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]int64, error)
	CountUsageOne(ctx context.Context, id uuid.UUID) (int64, error)
	Update(ctx context.Context, id uuid.UUID, fields map[string]any) error
	SoftDelete(ctx context.Context, p repository.SoftDeleteParams) error
}

// Compile-time assertions.
var _ DiscountStore = (*repository.DiscountRepository)(nil)
var _ discountapi.Resolver = (*Service)(nil)

type Service struct {
	discounts DiscountStore
}

func New(discounts DiscountStore) *Service { return &Service{discounts: discounts} }

// Create implements POST /admin/discounts (§28.1).
func (s *Service) Create(ctx context.Context, in CreateInput) (*DiscountView, error) {
	d, err := buildDiscountForCreate(in)
	if err != nil {
		return nil, err
	}
	if err := s.discounts.Create(ctx, d); err != nil {
		if errors.Is(err, repository.ErrCodeConflict) {
			return nil, discountapi.ErrDiscountCodeConflict
		}
		return nil, fmt.Errorf("create discount: %w", err)
	}
	view := toDiscountView(d, 0, time.Now())
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
	view := toDiscountView(d, usage, time.Now())
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

	now := time.Now()
	views := make([]DiscountView, 0, len(rows))
	for i := range rows {
		d := &rows[i]
		v := toDiscountView(d, usage[d.ID], now)
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
// kasir). Mengembalikan hanya diskon yang lolos SEMUA validasi §28.4 untuk
// subtotal & channel yang diberikan, ditambah preview_amount.
func (s *Service) Applicable(ctx context.Context, channel string, subtotal int64) ([]ApplicableView, error) {
	now := time.Now()
	candidates, err := s.discounts.ListActiveForChannel(ctx, channel, subtotal, now)
	if err != nil {
		return nil, fmt.Errorf("list applicable discounts: %w", err)
	}
	ids := make([]uuid.UUID, len(candidates))
	for i := range candidates {
		ids[i] = candidates[i].ID
	}
	usage, err := s.discounts.CountUsage(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list applicable discounts: count usage: %w", err)
	}

	out := make([]ApplicableView, 0, len(candidates))
	for i := range candidates {
		d := &candidates[i]
		u := usage[d.ID]
		// ListActiveForChannel sudah filter is_active/date-range/channel/
		// min_subtotal di DB — sisanya (kuota) butuh usage count, dicek di sini.
		if d.Quota != nil && u >= int64(*d.Quota) {
			continue
		}
		out = append(out, ApplicableView{
			DiscountView:  toDiscountView(d, u, now),
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

	if len(fields) > 0 {
		if err := s.discounts.Update(ctx, id, fields); err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return nil, discountapi.ErrDiscountNotFound
			}
			if errors.Is(err, repository.ErrCodeConflict) {
				return nil, discountapi.ErrDiscountCodeConflict
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
	view := toDiscountView(updated, usage, time.Now())
	return &view, nil
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

func buildDiscountForCreate(in CreateInput) (*model.Discount, error) {
	code := strings.ToUpper(strings.TrimSpace(in.Code))
	if code == "" {
		return nil, discountapi.ErrDiscountCodeRequired
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, discountapi.ErrDiscountNameRequired
	}
	scope, err := normalizeChannelScope(in.ChannelScope)
	if err != nil {
		return nil, err
	}
	if in.MinSubtotal < 0 {
		return nil, discountapi.ErrDiscountMinSubtotalInvalid
	}
	if in.Quota != nil && *in.Quota <= 0 {
		return nil, discountapi.ErrDiscountQuotaInvalid
	}
	if in.MaxDiscountAmount != nil && *in.MaxDiscountAmount <= 0 {
		return nil, discountapi.ErrDiscountMaxAmountInvalid
	}
	if err := validatePeriod(in.StartsAt, in.EndsAt); err != nil {
		return nil, err
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
		IsActive:          in.IsActive,
	}
	if err := setDiscountTypeValue(d, in.Type, in.ValuePercent, in.ValueAmount); err != nil {
		return nil, err
	}
	// Temuan review #9(b) — max_discount_amount cuma berlaku untuk diskon
	// type=percent; computeAmount (calc.go) mengabaikannya total untuk
	// nominal, jadi diisi di sana tanpa efek = jebakan diam-diam bagi admin.
	if d.Type == model.DiscountTypeNominal && d.MaxDiscountAmount != nil {
		return nil, discountapi.ErrDiscountMaxAmountNotAllowed
	}
	return d, nil
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
		scope, err := normalizeChannelScope(*in.ChannelScope)
		if err != nil {
			return err
		}
		d.ChannelScope = scope
		fields["channel_scope"] = string(scope)
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
