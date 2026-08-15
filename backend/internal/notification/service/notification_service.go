// Package service — orkestrasi enqueue notifikasi + koordinasi dengan worker
// (via internal HTTP endpoints di handler package).
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/notification/model"
	"github.com/rajaku-printing/backend/internal/notification/notificationapi"
	"github.com/rajaku-printing/backend/internal/notification/repository"
	"github.com/rajaku-printing/backend/internal/order/orderapi"
)

// JobStore — narrowed storage contract untuk testability. *repository.Repository
// satisfies this.
type JobStore interface {
	Create(ctx context.Context, j *model.NotificationJob) error
	ClaimBatch(ctx context.Context, limit int) ([]model.NotificationJob, error)
	MarkSent(ctx context.Context, id uuid.UUID) error
	MarkFailure(ctx context.Context, id uuid.UUID, errMsg string, backoff time.Duration) error
	FindByID(ctx context.Context, id uuid.UUID) (*model.NotificationJob, error)
	List(ctx context.Context, f repository.ListFilter) (*repository.ListResult, error)
}

// Service — implementasi notificationapi.Enqueuer. Cross-module deps
// (orderCmd, customers) di-inject via interface — modul ini TIDAK boleh import
// internal/order/service atau internal/auth/service langsung (§22).
type Service struct {
	jobs      JobStore
	orderCmd  orderapi.OrderCommandService
	customers authapi.CustomerService
	cfg       Config
	nowFn     func() time.Time
}

type Config struct {
	// BaseURL — dari config.App.BaseURL. Dipakai untuk render tracking URL
	// di semua template. SATU sumber (§2 konfigurasi URL terpusat).
	BaseURL string
	// MaxAttempts — kalau > 0, override default max_attempts di job. 0 = pakai
	// default DB (5).
	MaxAttempts int
	// InitialBackoff — retry pertama setelah gagal. Backoff selanjutnya
	// exponential (initial * 2^attempts_after_first).
	InitialBackoff time.Duration
	// InternalAlertPhone — nomor WA ops/staff (format 62xxx) tujuan alert
	// internal (§19 reminder retensi). Kosong = fitur alert internal off;
	// EnqueueInternalAlert return ErrInternalRecipientMissing.
	InternalAlertPhone string
}

var (
	_ notificationapi.Enqueuer        = (*Service)(nil)
	_ notificationapi.InternalAlerter = (*Service)(nil)
	_ notificationapi.OTPSender       = (*Service)(nil)
)

func New(jobs JobStore, orderCmd orderapi.OrderCommandService, customers authapi.CustomerService, cfg Config) *Service {
	if cfg.InitialBackoff <= 0 {
		cfg.InitialBackoff = 30 * time.Second
	}
	return &Service{
		jobs:      jobs,
		orderCmd:  orderCmd,
		customers: customers,
		cfg:       cfg,
		nowFn:     time.Now,
	}
}

// EnqueueOrderEvent implements notificationapi.Enqueuer. Ordering:
//  1. Resolve order summary (via orderCmd) → dapat resi, customer_id, total, metode_ambil.
//  2. Resolve recipient phone (customer lookup — untuk saat ini customer.phone
//     dipakai baik pickup maupun kirim; alasan: nomor pemesan lebih relevan
//     dari nomor penerima paket untuk update status transaksi).
//  3. Render template.
//  4. Insert row. Kalau dedup_key sudah ada → treat sebagai idempotent no-op
//     (bukan error, return nil).
func (s *Service) EnqueueOrderEvent(ctx context.Context, kind notificationapi.Kind, orderID uuid.UUID, extras map[string]any) error {
	summary, err := s.orderCmd.FindSummaryByID(ctx, orderID)
	if err != nil {
		if errors.Is(err, orderapi.ErrOrderNotFound) {
			return notificationapi.ErrOrderNotFound
		}
		return fmt.Errorf("enqueue: lookup order: %w", err)
	}

	cust, err := s.customers.FindByID(ctx, summary.CustomerID)
	if err != nil {
		if errors.Is(err, authapi.ErrCustomerNotFound) {
			return notificationapi.ErrRecipientMissing
		}
		return fmt.Errorf("enqueue: lookup customer: %w", err)
	}
	phone := strings.TrimSpace(cust.Phone)
	if phone == "" {
		return notificationapi.ErrRecipientMissing
	}

	msg, err := render(kind, templateCtx{
		CustomerName: cust.Name,
		Resi:         summary.Resi,
		BaseURL:      s.cfg.BaseURL,
		Total:        summary.Total,
		Extras:       extras,
	})
	if err != nil {
		return err
	}

	payload := model.JSONPayload{
		"resi":         summary.Resi,
		"order_status": summary.Status,
		"total":        summary.Total,
		"metode_ambil": summary.MetodeAmbil,
		"channel":      summary.Channel,
	}
	for k, v := range extras {
		payload[k] = v
	}

	dedup := fmt.Sprintf("%s:%s", kind, orderID)
	job := &model.NotificationJob{
		Kind:           string(kind),
		RecipientPhone: phone,
		Message:        msg,
		Payload:        payload,
		DedupKey:       &dedup,
		Status:         model.JobPending,
		Attempts:       0,
		MaxAttempts:    s.maxAttempts(),
		NextAttemptAt:  s.nowFn().UTC(),
		OrderID:        &orderID,
	}
	if err := s.jobs.Create(ctx, job); err != nil {
		if errors.Is(err, repository.ErrDedupConflict) {
			// Idempotent: same kind+order already enqueued. Not a business error.
			return nil
		}
		return fmt.Errorf("enqueue: insert job: %w", err)
	}
	return nil
}

// EnqueueInternalAlert implements notificationapi.InternalAlerter — tujuan
// nomor ops (cfg.InternalAlertPhone), BUKAN customer. Tidak ada lookup
// customer sama sekali di sini; itu yang membedakannya dari EnqueueOrderEvent.
func (s *Service) EnqueueInternalAlert(
	ctx context.Context,
	kind notificationapi.Kind,
	orderID *uuid.UUID,
	extras map[string]any,
	dedupKey string,
) error {
	phone := strings.TrimSpace(s.cfg.InternalAlertPhone)
	if phone == "" {
		return notificationapi.ErrInternalRecipientMissing
	}
	if strings.TrimSpace(dedupKey) == "" {
		return fmt.Errorf("enqueue internal alert: dedup key required for kind %s", kind)
	}

	payload := model.JSONPayload{"internal": true}
	tctx := templateCtx{BaseURL: s.cfg.BaseURL, Extras: extras}

	// Order opsional — alert internal ke depan bisa saja tidak terkait order
	// (mis. alert disk penuh §19). Kalau ada, perkaya konteks template.
	if orderID != nil {
		summary, err := s.orderCmd.FindSummaryByID(ctx, *orderID)
		if err != nil {
			if errors.Is(err, orderapi.ErrOrderNotFound) {
				return notificationapi.ErrOrderNotFound
			}
			return fmt.Errorf("enqueue internal alert: lookup order: %w", err)
		}
		tctx.Resi = summary.Resi
		tctx.Total = summary.Total
		payload["resi"] = summary.Resi
		payload["order_status"] = summary.Status
	}
	for k, v := range extras {
		payload[k] = v
	}

	msg, err := render(kind, tctx)
	if err != nil {
		return err
	}

	job := &model.NotificationJob{
		Kind:           string(kind),
		RecipientPhone: phone,
		Message:        msg,
		Payload:        payload,
		DedupKey:       &dedupKey,
		Status:         model.JobPending,
		Attempts:       0,
		MaxAttempts:    s.maxAttempts(),
		NextAttemptAt:  s.nowFn().UTC(),
		OrderID:        orderID,
	}
	if err := s.jobs.Create(ctx, job); err != nil {
		if errors.Is(err, repository.ErrDedupConflict) {
			// Alert yang sama sudah antre — idempotent no-op.
			return nil
		}
		return fmt.Errorf("enqueue internal alert: insert job: %w", err)
	}
	return nil
}

// EnqueueOTP implements notificationapi.OTPSender. Unlike EnqueueOrderEvent,
// there's no order/customer to resolve or template to render — the caller
// (auth module) already built the exact message (it's the only place that
// knows the raw OTP code; this module never sees it in plaintext beyond the
// rendered message string, and never persists it separately).
func (s *Service) EnqueueOTP(ctx context.Context, phone, message, dedupKey string) error {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return notificationapi.ErrRecipientMissing
	}
	if strings.TrimSpace(dedupKey) == "" {
		return fmt.Errorf("enqueue otp: dedup key required")
	}
	job := &model.NotificationJob{
		Kind:           string(notificationapi.KindOTPVerification),
		RecipientPhone: phone,
		Message:        message,
		Payload:        model.JSONPayload{"otp": true},
		DedupKey:       &dedupKey,
		Status:         model.JobPending,
		Attempts:       0,
		MaxAttempts:    s.maxAttempts(),
		NextAttemptAt:  s.nowFn().UTC(),
	}
	if err := s.jobs.Create(ctx, job); err != nil {
		if errors.Is(err, repository.ErrDedupConflict) {
			// Idempotent: same OTP challenge already enqueued.
			return nil
		}
		return fmt.Errorf("enqueue otp: insert job: %w", err)
	}
	return nil
}

func (s *Service) maxAttempts() int {
	if s.cfg.MaxAttempts > 0 {
		return s.cfg.MaxAttempts
	}
	return 5
}

// --- Worker-facing coordination (dipakai handler internal endpoints) ---

type ClaimedJob struct {
	ID             uuid.UUID `json:"id"`
	Kind           string    `json:"kind"`
	RecipientPhone string    `json:"recipient_phone"`
	Message        string    `json:"message"`
	Attempts       int       `json:"attempts"`
	MaxAttempts    int       `json:"max_attempts"`
}

// ClaimBatch dispatches up to `limit` eligible jobs ke worker. Row-locked
// SKIP LOCKED sehingga multiple worker aman.
func (s *Service) ClaimBatch(ctx context.Context, limit int) ([]ClaimedJob, error) {
	rows, err := s.jobs.ClaimBatch(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("claim batch: %w", err)
	}
	out := make([]ClaimedJob, len(rows))
	for i, r := range rows {
		out[i] = ClaimedJob{
			ID:             r.ID,
			Kind:           r.Kind,
			RecipientPhone: r.RecipientPhone,
			Message:        r.Message,
			Attempts:       r.Attempts,
			MaxAttempts:    r.MaxAttempts,
		}
	}
	return out, nil
}

// MarkSent dipanggil worker setelah Baileys sukses kirim.
func (s *Service) MarkSent(ctx context.Context, id uuid.UUID) error {
	if err := s.jobs.MarkSent(ctx, id); err != nil {
		return fmt.Errorf("mark sent: %w", err)
	}
	return nil
}

// MarkFailed dipanggil worker saat Baileys error. Backoff = initial * 2^(attempts-1)
// dengan cap 30 menit — cegah terlalu lama menunda job.
func (s *Service) MarkFailed(ctx context.Context, id uuid.UUID, errMsg string) error {
	// Load current attempt count untuk hitung backoff.
	j, err := s.jobs.FindByID(ctx, id)
	if err != nil {
		return fmt.Errorf("mark failed: %w", err)
	}
	backoff := s.computeBackoff(j.Attempts)
	if err := s.jobs.MarkFailure(ctx, id, errMsg, backoff); err != nil {
		return fmt.Errorf("mark failed: %w", err)
	}
	return nil
}

func (s *Service) computeBackoff(attempts int) time.Duration {
	if attempts <= 0 {
		return s.cfg.InitialBackoff
	}
	// Exponential w/ cap. shift = attempts-1 karena attempts sudah di-increment
	// oleh ClaimBatch sebelum sampai ke Failure path. Cap shift 20 semata
	// untuk hindari int64 overflow — cap sesungguhnya lewat maxBackoff.
	shift := attempts - 1
	if shift > 20 {
		shift = 20
	}
	d := s.cfg.InitialBackoff << shift
	const maxBackoff = 30 * time.Minute
	if d > maxBackoff || d < 0 {
		d = maxBackoff
	}
	return d
}
