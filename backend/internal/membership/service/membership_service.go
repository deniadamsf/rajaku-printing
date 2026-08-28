// Package service holds membership module business logic (§30 CLAUDE.md).
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/membership/membershipapi"
	"github.com/rajaku-printing/backend/internal/membership/model"
	"github.com/rajaku-printing/backend/internal/notification/notificationapi"
	"github.com/rajaku-printing/backend/internal/settings/settingsapi"
)

// LogStore — narrowed storage contract for the audit-trail log
// (membership_status_logs). *repository.LogRepository satisfies this.
type LogStore interface {
	Create(ctx context.Context, l *model.MembershipStatusLog) error
}

// Service implements membershipapi.Checker. Cross-module deps (customers,
// notifier, settings) di-inject via interface — modul ini TIDAK boleh import
// internal/auth/service atau internal/notification/service langsung (§22).
type Service struct {
	customers authapi.CustomerService
	logs      LogStore
	settings  settingsapi.Reader
	notifier  notificationapi.CustomerEventEnqueuer
	nowFn     func() time.Time
}

var _ membershipapi.Checker = (*Service)(nil)

func New(customers authapi.CustomerService, logs LogStore) *Service {
	return &Service{customers: customers, logs: logs, nowFn: time.Now}
}

func (s *Service) SetSettingsReader(r settingsapi.Reader)              { s.settings = r }
func (s *Service) SetNotifier(n notificationapi.CustomerEventEnqueuer) { s.notifier = n }

// IsActiveMember implements membershipapi.Checker (§30.3 — dipakai modul
// discount untuk validasi diskon audience_scope='member').
func (s *Service) IsActiveMember(ctx context.Context, customerID uuid.UUID) (bool, error) {
	if customerID == uuid.Nil {
		return false, nil
	}
	info, err := s.customers.GetMembershipInfo(ctx, customerID)
	if err != nil {
		if errors.Is(err, authapi.ErrCustomerNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("membership: check active member %s: %w", customerID, err)
	}
	return info.Status == string(membershipapi.StatusActive), nil
}

// Apply implements POST /account/membership/apply (§30.2) — customer
// mengajukan diri jadi member. Hanya sah dari status none/rejected, dan
// hanya untuk akun customer_type='registered'.
func (s *Service) Apply(ctx context.Context, customerID uuid.UUID) (*MembershipView, error) {
	enabled, err := s.isEnabled(ctx)
	if err != nil {
		return nil, err
	}
	if !enabled {
		return nil, membershipapi.ErrMembershipDisabled
	}

	info, err := s.mustFind(ctx, customerID)
	if err != nil {
		return nil, err
	}
	if info.CustomerType != authapi.CustomerTypeRegistered {
		return nil, membershipapi.ErrMembershipRequiresRegisteredAccount
	}
	if !canApply(info.Status) {
		return nil, membershipapi.ErrMembershipInvalidTransition
	}

	now := s.now()
	if err := s.transition(ctx, transitionInput{
		CustomerID:  customerID,
		FromStatus:  info.Status,
		ToStatus:    string(membershipapi.StatusPending),
		RequestedAt: &now,
		// ClearDecision — wajib (§30.2): saat re-apply dari rejected, catatan
		// keputusan penolakan sebelumnya (decided_at/decided_by/note) TIDAK
		// boleh menempel di pengajuan baru yang statusnya pending lagi —
		// kalau tidak, reviewer kedua bisa salah baca sebagai "sudah pernah
		// diputuskan". Aman juga dipanggil saat apply pertama kali dari
		// none, karena kolomnya memang masih kosong.
		ClearDecision: true,
	}); err != nil {
		return nil, err
	}
	return s.viewOf(ctx, customerID)
}

// Get implements GET /account/membership (§30.2) — customer membaca status
// membership dirinya sendiri. Tanpa ini, status cuma terbaca sekali di body
// respons mutasi (Apply/dst) lalu hilang begitu customer reload halaman.
func (s *Service) Get(ctx context.Context, customerID uuid.UUID) (*MembershipView, error) {
	return s.viewOf(ctx, customerID)
}

// canApply — §30.2: none/rejected -> pending diperbolehkan; selain itu tidak.
func canApply(current string) bool {
	return current == string(membershipapi.StatusNone) || current == string(membershipapi.StatusRejected)
}

// Approve implements POST /admin/membership/:id/approve (§30.2) —
// pending -> active.
func (s *Service) Approve(ctx context.Context, customerID, actorID uuid.UUID) (*MembershipView, error) {
	return s.decide(ctx, decideParams{
		CustomerID:  customerID,
		ActorID:     actorID,
		RequireFrom: string(membershipapi.StatusPending),
		ToStatus:    string(membershipapi.StatusActive),
		NotifyKind:  notificationapi.KindMembershipApproved,
	})
}

// Reject implements POST /admin/membership/:id/reject (§30.2) —
// pending -> rejected, reason wajib.
func (s *Service) Reject(ctx context.Context, customerID, actorID uuid.UUID, reason string) (*MembershipView, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, membershipapi.ErrMembershipReasonRequired
	}
	return s.decide(ctx, decideParams{
		CustomerID:   customerID,
		ActorID:      actorID,
		RequireFrom:  string(membershipapi.StatusPending),
		ToStatus:     string(membershipapi.StatusRejected),
		Note:         reason,
		NotifyKind:   notificationapi.KindMembershipRejected,
		NotifyExtras: map[string]any{"reason": reason},
	})
}

// Revoke implements POST /admin/membership/:id/revoke (§30.2) —
// active -> revoked, reason wajib. TIDAK bisa self-service ajukan ulang
// (beda dari rejected) — customer harus lewat Apply lagi tapi status
// revoked BUKAN salah satu status yang canApply terima, jadi pemulihan dari
// revoked wajib lewat intervensi admin manual — lihat Reinstate di bawah.
func (s *Service) Revoke(ctx context.Context, customerID, actorID uuid.UUID, reason string) (*MembershipView, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, membershipapi.ErrMembershipReasonRequired
	}
	return s.decide(ctx, decideParams{
		CustomerID:   customerID,
		ActorID:      actorID,
		RequireFrom:  string(membershipapi.StatusActive),
		ToStatus:     string(membershipapi.StatusRevoked),
		Note:         reason,
		NotifyKind:   notificationapi.KindMembershipRevoked,
		NotifyExtras: map[string]any{"reason": reason},
	})
}

// Reinstate implements POST /admin/membership/:id/reinstate (§30.2) —
// revoked -> active, reason wajib. Satu-satunya jalur pemulihan dari
// revoked — TIDAK self-service (beda dari rejected -> pending lewat Apply),
// karena revoke berarti ada alasan spesifik yang perlu ditinjau ulang
// manusia sebelum status member aktif lagi. Tanpa endpoint ini, revoked
// jadi jalan buntu permanen yang cuma bisa diperbaiki lewat UPDATE manual
// di DB (melewati membership_status_logs, merusak audit trail).
func (s *Service) Reinstate(ctx context.Context, customerID, actorID uuid.UUID, reason string) (*MembershipView, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, membershipapi.ErrMembershipReasonRequired
	}
	return s.decide(ctx, decideParams{
		CustomerID:   customerID,
		ActorID:      actorID,
		RequireFrom:  string(membershipapi.StatusRevoked),
		ToStatus:     string(membershipapi.StatusActive),
		Note:         reason,
		NotifyKind:   notificationapi.KindMembershipReinstated,
		NotifyExtras: map[string]any{"reason": reason},
	})
}

// decideParams — parameter bersama Approve/Reject/Revoke (dipecah supaya
// masing-masing endpoint tetap pendek, §22 nesting/panjang fungsi).
type decideParams struct {
	CustomerID   uuid.UUID
	ActorID      uuid.UUID
	RequireFrom  string
	ToStatus     string
	Note         string
	NotifyKind   notificationapi.Kind
	NotifyExtras map[string]any
}

// decide menegakkan status prasyarat (RequireFrom), lalu menjalankan
// transisi + notifikasi. Dipakai Approve/Reject/Revoke — ketiganya punya
// bentuk yang sama persis: cek status pending/active, tulis keputusan staff,
// beri tahu customer.
func (s *Service) decide(ctx context.Context, p decideParams) (*MembershipView, error) {
	info, err := s.mustFind(ctx, p.CustomerID)
	if err != nil {
		return nil, err
	}
	if info.Status != p.RequireFrom {
		return nil, membershipapi.ErrMembershipInvalidTransition
	}

	now := s.now()
	note := p.Note
	if err := s.transition(ctx, transitionInput{
		CustomerID: p.CustomerID,
		FromStatus: info.Status,
		ToStatus:   p.ToStatus,
		DecidedAt:  &now,
		DecidedBy:  &p.ActorID,
		Note:       &note,
		ChangedBy:  &p.ActorID,
	}); err != nil {
		return nil, err
	}
	s.notify(ctx, p.NotifyKind, p.CustomerID, p.NotifyExtras, now)
	return s.viewOf(ctx, p.CustomerID)
}

// List implements GET /admin/membership (§30.2).
func (s *Service) List(ctx context.Context, f ListFilter) (*ListResult, error) {
	status := strings.TrimSpace(f.Status)
	if status != "" && !isKnownStatus(status) {
		return nil, membershipapi.ErrMembershipInvalidStatusFilter
	}
	res, err := s.customers.ListMembers(ctx, authapi.ListMembershipFilter{
		Status:  status,
		Page:    f.Page,
		PerPage: f.PerPage,
	})
	if err != nil {
		return nil, fmt.Errorf("membership: list: %w", err)
	}
	items := make([]MembershipView, len(res.Items))
	for i := range res.Items {
		items[i] = *toView(&res.Items[i])
	}
	return &ListResult{Items: items, Total: res.Total, Page: res.Page, PerPage: res.PerPage}, nil
}

func isKnownStatus(s string) bool {
	switch membershipapi.Status(s) {
	case membershipapi.StatusNone, membershipapi.StatusPending, membershipapi.StatusActive,
		membershipapi.StatusRejected, membershipapi.StatusRevoked:
		return true
	default:
		return false
	}
}

// --- internal helpers ---

func (s *Service) now() time.Time { return s.nowFn().UTC() }

// mustFind loads a customer's membership info, mapping authapi's
// not-found sentinel to this module's own (§22 — modul lain tidak boleh
// bocor sentinel modul lain ke caller-nya).
func (s *Service) mustFind(ctx context.Context, customerID uuid.UUID) (*authapi.MembershipInfo, error) {
	info, err := s.customers.GetMembershipInfo(ctx, customerID)
	if err != nil {
		if errors.Is(err, authapi.ErrCustomerNotFound) {
			return nil, membershipapi.ErrMembershipCustomerNotFound
		}
		return nil, fmt.Errorf("membership: lookup customer %s: %w", customerID, err)
	}
	return info, nil
}

func (s *Service) viewOf(ctx context.Context, customerID uuid.UUID) (*MembershipView, error) {
	info, err := s.mustFind(ctx, customerID)
	if err != nil {
		return nil, err
	}
	// Setiap view yang keluar dari service ini menyertakan status saklar
	// membership_enabled saat itu juga — frontend customer pakai ini untuk
	// menyembunyikan CTA "Ajukan jadi Member" saat fitur nonaktif (§30.1),
	// bukan cuma bereaksi setelah backend menolak Apply.
	enabled, err := s.isEnabled(ctx)
	if err != nil {
		return nil, err
	}
	view := toView(info)
	view.MembershipEnabled = enabled
	return view, nil
}

func toView(info *authapi.MembershipInfo) *MembershipView {
	return &MembershipView{
		CustomerID:   info.CustomerID,
		Name:         info.Name,
		Phone:        info.Phone,
		Status:       info.Status,
		RequestedAt:  info.RequestedAt,
		DecidedAt:    info.DecidedAt,
		DecidedBy:    info.DecidedBy,
		DecisionNote: info.DecisionNote,
	}
}

// isEnabled reads the membership_enabled saklar (§30.1). Fail-fast (bukan
// diam-diam anggap disabled) kalau settings reader belum di-wire atau
// row-nya hilang — misconfigurasi harus kelihatan, bukan tertelan jadi
// "fitur nonaktif" yang salah alasan.
func (s *Service) isEnabled(ctx context.Context) (bool, error) {
	if s.settings == nil {
		return false, errors.New("membership: settings reader belum di-wire")
	}
	enabled, err := s.settings.GetBool(ctx, settingsapi.KeyMembershipEnabled)
	if err != nil {
		if errors.Is(err, settingsapi.ErrSettingNotFound) {
			return false, fmt.Errorf(
				"membership: setting %q hilang (migration 000030 belum jalan?): %w",
				settingsapi.KeyMembershipEnabled, err)
		}
		return false, fmt.Errorf("membership: read setting %q: %w", settingsapi.KeyMembershipEnabled, err)
	}
	return enabled, nil
}

// transitionInput — parameter transisi CAS ke authapi + baris log audit.
type transitionInput struct {
	CustomerID  uuid.UUID
	FromStatus  string
	ToStatus    string
	RequestedAt *time.Time
	DecidedAt   *time.Time
	DecidedBy   *uuid.UUID
	Note        *string
	ChangedBy   *uuid.UUID
	// ClearDecision — lihat authapi.UpdateMembershipStatusInput.ClearDecision.
	// Dipakai Apply saat re-apply dari rejected (§30.2).
	ClearDecision bool
}

// transition writes the CAS status update via authapi (source of truth),
// then best-effort appends the audit log row. Log insert failure is logged
// & swallowed (bukan `_ = err` — ditangani via logging, sama pola dengan
// notifier best-effort di seluruh codebase, mis. payment/design service
// enqueueNotif) — status di `users` sudah benar terlepas dari log-nya
// tercatat atau tidak, dan membership_status_logs sengaja BUKAN sumber
// kebenaran (§30.2).
func (s *Service) transition(ctx context.Context, in transitionInput) error {
	err := s.customers.UpdateMembershipStatus(ctx, authapi.UpdateMembershipStatusInput{
		CustomerID:    in.CustomerID,
		FromStatus:    in.FromStatus,
		ToStatus:      in.ToStatus,
		RequestedAt:   in.RequestedAt,
		DecidedAt:     in.DecidedAt,
		DecidedBy:     in.DecidedBy,
		Note:          in.Note,
		ClearDecision: in.ClearDecision,
	})
	if err != nil {
		if errors.Is(err, authapi.ErrMembershipStatusConflict) {
			return membershipapi.ErrMembershipInvalidTransition
		}
		return fmt.Errorf("membership: transition customer %s %s->%s: %w",
			in.CustomerID, in.FromStatus, in.ToStatus, err)
	}

	logRow := &model.MembershipStatusLog{
		CustomerID: in.CustomerID,
		FromStatus: in.FromStatus,
		ToStatus:   in.ToStatus,
		ChangedBy:  in.ChangedBy,
		Note:       in.Note,
	}
	if err := s.logs.Create(ctx, logRow); err != nil {
		log.Ctx(ctx).Error().Err(err).
			Str("customer_id", in.CustomerID.String()).
			Str("from_status", in.FromStatus).
			Str("to_status", in.ToStatus).
			Msg("membership: insert status log failed — status sudah berubah, log audit tidak tercatat")
	}
	return nil
}

// notify enqueues a best-effort WA notification for a status change (§30.2
// — hanya active/rejected/revoked yang dikirimi notif, Apply tidak).
// dedupKey menyertakan timestamp nanodetik karena status membership bisa
// SIKLUS (active->revoked->pending->active lagi) — dedup statis per
// kind+customerID akan diam-diam menelan notifikasi kedua & seterusnya.
func (s *Service) notify(ctx context.Context, kind notificationapi.Kind, customerID uuid.UUID, extras map[string]any, at time.Time) {
	if s.notifier == nil {
		return
	}
	dedup := fmt.Sprintf("%s:%s:%d", kind, customerID, at.UnixNano())
	if err := s.notifier.EnqueueCustomerEvent(ctx, kind, customerID, extras, dedup); err != nil {
		log.Ctx(ctx).Error().Err(err).
			Str("customer_id", customerID.String()).
			Str("kind", string(kind)).
			Msg("membership: enqueue notification failed — WA tidak terkirim, cek manual")
	}
}
