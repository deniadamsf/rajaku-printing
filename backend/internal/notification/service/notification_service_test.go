package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/notification/model"
	"github.com/rajaku-printing/backend/internal/notification/notificationapi"
	"github.com/rajaku-printing/backend/internal/notification/repository"
	"github.com/rajaku-printing/backend/internal/order/orderapi"
)

// ---------- fakes ----------

type fakeJobStore struct {
	createErr    error
	created      *model.NotificationJob
	createCalls  int
	claimResult  []model.NotificationJob
	claimErr     error
	markSentErr  error
	markSentID   uuid.UUID
	markFailErr  error
	markFailArgs struct {
		id      uuid.UUID
		errMsg  string
		backoff time.Duration
	}
	findByIDResult *model.NotificationJob
	findByIDErr    error

	redactStaleCalls int
	redactStaleArg   time.Time
	redactStaleN     int64
	redactStaleErr   error
}

func (f *fakeJobStore) Create(_ context.Context, j *model.NotificationJob) error {
	f.createCalls++
	if f.createErr != nil {
		return f.createErr
	}
	j.ID = uuid.New()
	j.CreatedAt = time.Now().UTC()
	f.created = j
	return nil
}
func (f *fakeJobStore) ClaimBatch(_ context.Context, _ int) ([]model.NotificationJob, error) {
	return f.claimResult, f.claimErr
}
func (f *fakeJobStore) MarkSent(_ context.Context, id uuid.UUID) error {
	f.markSentID = id
	return f.markSentErr
}
func (f *fakeJobStore) MarkFailure(_ context.Context, id uuid.UUID, errMsg string, backoff time.Duration) error {
	f.markFailArgs.id = id
	f.markFailArgs.errMsg = errMsg
	f.markFailArgs.backoff = backoff
	return f.markFailErr
}
func (f *fakeJobStore) FindByID(_ context.Context, _ uuid.UUID) (*model.NotificationJob, error) {
	return f.findByIDResult, f.findByIDErr
}
func (f *fakeJobStore) List(_ context.Context, _ repository.ListFilter) (*repository.ListResult, error) {
	return &repository.ListResult{}, nil
}
func (f *fakeJobStore) RedactStaleSensitive(_ context.Context, olderThan time.Time) (int64, error) {
	f.redactStaleCalls++
	f.redactStaleArg = olderThan
	return f.redactStaleN, f.redactStaleErr
}

type fakeOrderCmd struct {
	summary *orderapi.OrderSummary
	err     error
}

func (f *fakeOrderCmd) FindSummaryByResi(context.Context, string) (*orderapi.OrderSummary, error) {
	return f.summary, f.err
}
func (f *fakeOrderCmd) FindSummaryByID(context.Context, uuid.UUID) (*orderapi.OrderSummary, error) {
	return f.summary, f.err
}
func (f *fakeOrderCmd) MarkPendingVerification(context.Context, uuid.UUID, *uuid.UUID, string) error {
	return nil
}
func (f *fakeOrderCmd) MarkDibayar(context.Context, uuid.UUID, string, *uuid.UUID, string) error {
	return nil
}
func (f *fakeOrderCmd) MarkDitolak(context.Context, uuid.UUID, *uuid.UUID, string) error {
	return nil
}
func (f *fakeOrderCmd) MarkDesainDikerjakan(context.Context, uuid.UUID, *uuid.UUID, string) error {
	return nil
}
func (f *fakeOrderCmd) MarkMenungguApprovalDesain(context.Context, uuid.UUID, *uuid.UUID, string) error {
	return nil
}
func (f *fakeOrderCmd) MarkDesainDiverifikasi(context.Context, uuid.UUID, *uuid.UUID, string) error {
	return nil
}
func (f *fakeOrderCmd) MarkProsesCetak(context.Context, uuid.UUID, *uuid.UUID, string) error {
	return nil
}
func (f *fakeOrderCmd) MarkQC(context.Context, uuid.UUID, *uuid.UUID, string) error {
	return nil
}
func (f *fakeOrderCmd) MarkSiapKirimAtauAmbil(context.Context, uuid.UUID, *uuid.UUID, string) error {
	return nil
}
func (f *fakeOrderCmd) MarkDikirim(context.Context, uuid.UUID, *uuid.UUID, string, string, string) error {
	return nil
}
func (f *fakeOrderCmd) MarkSelesai(context.Context, uuid.UUID, *uuid.UUID, string) error {
	return nil
}
func (f *fakeOrderCmd) FindInvoiceViewByID(context.Context, uuid.UUID) (*orderapi.OrderInvoiceView, error) {
	return nil, nil
}
func (f *fakeOrderCmd) CreatePOSOrder(context.Context, orderapi.POSCreateOrderInput) (*orderapi.OrderSummary, error) {
	return nil, nil
}
func (f *fakeOrderCmd) ListPOSOrdersByDate(context.Context, time.Time) ([]orderapi.OrderSummary, error) {
	return nil, nil
}

type fakeCustomers struct {
	identity *authapi.Identity
	err      error
}

func (f *fakeCustomers) ResolveOrCreateGuest(context.Context, string, string) (*authapi.Identity, error) {
	return f.identity, f.err
}
func (f *fakeCustomers) FindByID(context.Context, uuid.UUID) (*authapi.Identity, error) {
	return f.identity, f.err
}

// ---------- tests ----------

func newSvc(store *fakeJobStore, oc *fakeOrderCmd, cs *fakeCustomers) *Service {
	return New(store, oc, cs, Config{
		BaseURL:        "https://rajaku.test",
		MaxAttempts:    5,
		InitialBackoff: 10 * time.Second,
	})
}

func TestEnqueueOrderEvent_HappyPath_OngkirReady(t *testing.T) {
	orderID := uuid.New()
	custID := uuid.New()
	store := &fakeJobStore{}
	svc := newSvc(store,
		&fakeOrderCmd{summary: &orderapi.OrderSummary{
			ID: orderID, Resi: "RJK-ABC12345", CustomerID: custID,
			Status: "menunggu_pembayaran", Total: 175000, MetodeAmbil: "kirim", Channel: "online",
		}},
		&fakeCustomers{identity: &authapi.Identity{UserID: custID, Name: "Budi", Phone: "628123456789"}},
	)

	err := svc.EnqueueOrderEvent(context.Background(), notificationapi.KindOngkirReady, orderID,
		map[string]any{"shipping_cost": int64(15000)})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if store.createCalls != 1 {
		t.Fatalf("expected 1 create call, got %d", store.createCalls)
	}
	j := store.created
	if j.Kind != "ongkir_ready" {
		t.Errorf("kind: want ongkir_ready, got %s", j.Kind)
	}
	if j.RecipientPhone != "628123456789" {
		t.Errorf("phone: want 628123456789, got %s", j.RecipientPhone)
	}
	if !strings.Contains(j.Message, "RJK-ABC12345") {
		t.Errorf("message should contain resi, got: %s", j.Message)
	}
	if !strings.Contains(j.Message, "Rp 175.000") {
		t.Errorf("message should contain formatted total, got: %s", j.Message)
	}
	if !strings.Contains(j.Message, "https://rajaku.test/lacak/RJK-ABC12345") {
		t.Errorf("message should contain tracking URL from BaseURL, got: %s", j.Message)
	}
	if j.DedupKey == nil || *j.DedupKey != "ongkir_ready:"+orderID.String() {
		t.Errorf("dedup_key mismatch, got: %v", j.DedupKey)
	}
	if j.OrderID == nil || *j.OrderID != orderID {
		t.Errorf("order_id not linked")
	}
	if j.Status != model.JobPending {
		t.Errorf("status: want pending, got %s", j.Status)
	}
	if j.MaxAttempts != 5 {
		t.Errorf("max_attempts: want 5, got %d", j.MaxAttempts)
	}
	if v, ok := j.Payload["shipping_cost"]; !ok || v != int64(15000) {
		t.Errorf("payload should carry shipping_cost extra, got: %v", j.Payload)
	}
}

func TestEnqueueOrderEvent_IdempotentOnDedupConflict(t *testing.T) {
	orderID := uuid.New()
	store := &fakeJobStore{createErr: repository.ErrDedupConflict}
	svc := newSvc(store,
		&fakeOrderCmd{summary: &orderapi.OrderSummary{
			ID: orderID, Resi: "RJK-X", CustomerID: uuid.New(), Total: 1000,
		}},
		&fakeCustomers{identity: &authapi.Identity{Name: "A", Phone: "6281111"}},
	)
	err := svc.EnqueueOrderEvent(context.Background(), notificationapi.KindPaymentVerified, orderID, nil)
	if err != nil {
		t.Fatalf("dedup conflict must be treated as idempotent no-op, got err: %v", err)
	}
}

func TestEnqueueOrderEvent_OrderNotFound(t *testing.T) {
	store := &fakeJobStore{}
	svc := newSvc(store,
		&fakeOrderCmd{err: orderapi.ErrOrderNotFound},
		&fakeCustomers{},
	)
	err := svc.EnqueueOrderEvent(context.Background(), notificationapi.KindOngkirReady, uuid.New(), nil)
	if !errors.Is(err, notificationapi.ErrOrderNotFound) {
		t.Fatalf("want ErrOrderNotFound, got: %v", err)
	}
	if store.createCalls != 0 {
		t.Fatalf("must not create job when order missing")
	}
}

func TestEnqueueOrderEvent_RecipientMissing_EmptyPhone(t *testing.T) {
	store := &fakeJobStore{}
	svc := newSvc(store,
		&fakeOrderCmd{summary: &orderapi.OrderSummary{ID: uuid.New(), Resi: "RJK-Y", CustomerID: uuid.New()}},
		&fakeCustomers{identity: &authapi.Identity{Name: "NoPhone"}}, // Phone: ""
	)
	err := svc.EnqueueOrderEvent(context.Background(), notificationapi.KindOngkirReady, uuid.New(), nil)
	if !errors.Is(err, notificationapi.ErrRecipientMissing) {
		t.Fatalf("want ErrRecipientMissing, got: %v", err)
	}
	if store.createCalls != 0 {
		t.Fatalf("must not create job without recipient")
	}
}

func TestEnqueueOrderEvent_UnknownKind(t *testing.T) {
	store := &fakeJobStore{}
	svc := newSvc(store,
		&fakeOrderCmd{summary: &orderapi.OrderSummary{ID: uuid.New(), Resi: "RJK-Z", CustomerID: uuid.New()}},
		&fakeCustomers{identity: &authapi.Identity{Name: "A", Phone: "6281111"}},
	)
	err := svc.EnqueueOrderEvent(context.Background(), notificationapi.Kind("bogus_kind"), uuid.New(), nil)
	if !errors.Is(err, notificationapi.ErrUnknownKind) {
		t.Fatalf("want ErrUnknownKind, got: %v", err)
	}
}

// ---------- OTP sender (auth module — Google OAuth registration) ----------

func TestEnqueueOTP_HappyPath(t *testing.T) {
	store := &fakeJobStore{}
	svc := newSvc(store, &fakeOrderCmd{}, &fakeCustomers{})

	err := svc.EnqueueOTP(context.Background(), "6281234500001", "Kode verifikasi Rajaku Printing: 123456.", "otp:abc-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store.created == nil {
		t.Fatal("job tidak dibuat")
	}
	if store.created.RecipientPhone != "6281234500001" {
		t.Errorf("recipient mismatch, got %q", store.created.RecipientPhone)
	}
	if store.created.Kind != string(notificationapi.KindOTPVerification) {
		t.Errorf("kind mismatch, got %q", store.created.Kind)
	}
	if store.created.DedupKey == nil || *store.created.DedupKey != "otp:abc-1" {
		t.Errorf("dedup key mismatch, got %v", store.created.DedupKey)
	}
	// review finding #3: OTP jobs must be flagged is_sensitive so
	// MarkSent/MarkFailure (and the stale-message sweep) know to redact the
	// plaintext code from `message` once it's no longer needed.
	if !store.created.IsSensitive {
		t.Error("otp job harus IsSensitive=true (review finding #3)")
	}
}

func TestEnqueueOTP_EmptyPhone_ReturnsErrRecipientMissing(t *testing.T) {
	store := &fakeJobStore{}
	svc := newSvc(store, &fakeOrderCmd{}, &fakeCustomers{})

	err := svc.EnqueueOTP(context.Background(), "  ", "pesan", "otp:abc-2")
	if !errors.Is(err, notificationapi.ErrRecipientMissing) {
		t.Fatalf("want ErrRecipientMissing, got %v", err)
	}
	if store.createCalls != 0 {
		t.Error("tidak boleh insert job tanpa nomor tujuan")
	}
}

// ---------- sensitive-message sweep (review finding #3) ----------

func TestRedactStaleSensitiveMessages_HappyPath(t *testing.T) {
	store := &fakeJobStore{redactStaleN: 3}
	svc := New(store, &fakeOrderCmd{}, &fakeCustomers{}, Config{
		BaseURL:             "https://rajaku.test",
		InitialBackoff:      10 * time.Second,
		SensitiveMessageTTL: time.Hour,
	})

	n, err := svc.RedactStaleSensitiveMessages(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 3 {
		t.Fatalf("want 3 redacted, got %d", n)
	}
	if store.redactStaleCalls != 1 {
		t.Fatalf("expected exactly 1 call to RedactStaleSensitive, got %d", store.redactStaleCalls)
	}
}

func TestRedactStaleSensitiveMessages_TTLNotConfigured_IsNoOp(t *testing.T) {
	store := &fakeJobStore{redactStaleN: 99}
	svc := New(store, &fakeOrderCmd{}, &fakeCustomers{}, Config{
		BaseURL:        "https://rajaku.test",
		InitialBackoff: 10 * time.Second,
		// SensitiveMessageTTL left zero on purpose.
	})

	n, err := svc.RedactStaleSensitiveMessages(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Fatalf("want 0 (no-op) when TTL unset, got %d", n)
	}
	if store.redactStaleCalls != 0 {
		t.Fatalf("must not touch the store when TTL is unset, got %d calls", store.redactStaleCalls)
	}
}

func TestRedactStaleSensitiveMessages_RepositoryError_IsWrapped(t *testing.T) {
	sentinel := errors.New("boom")
	store := &fakeJobStore{redactStaleErr: sentinel}
	svc := New(store, &fakeOrderCmd{}, &fakeCustomers{}, Config{
		BaseURL:             "https://rajaku.test",
		InitialBackoff:      10 * time.Second,
		SensitiveMessageTTL: time.Hour,
	})

	_, err := svc.RedactStaleSensitiveMessages(context.Background())
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected wrapped sentinel error, got %v", err)
	}
}

func TestFormatIDR(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1_000, "1.000"},
		{175_000, "175.000"},
		{1_234_567, "1.234.567"},
		{-2_500, "-2.500"},
	}
	for _, c := range cases {
		if got := formatIDR(c.in); got != c.want {
			t.Errorf("formatIDR(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestComputeBackoff_Exponential(t *testing.T) {
	svc := New(nil, nil, nil, Config{InitialBackoff: 10 * time.Second})
	cases := []struct {
		attempts int
		want     time.Duration
	}{
		{0, 10 * time.Second},
		{1, 10 * time.Second},  // shift=0
		{2, 20 * time.Second},  // shift=1
		{3, 40 * time.Second},  // shift=2
		{12, 30 * time.Minute}, // capped (2^11 * 10s = 5h+ → cap 30m)
	}
	for _, c := range cases {
		got := svc.computeBackoff(c.attempts)
		if got != c.want {
			t.Errorf("computeBackoff(%d) = %v, want %v", c.attempts, got, c.want)
		}
	}
}

// ---------- internal alert (§19 reminder retensi) ----------

func newInternalSvc(store *fakeJobStore, oc *fakeOrderCmd, phone string) *Service {
	return New(store, oc, &fakeCustomers{}, Config{
		BaseURL:            "https://rajaku.test",
		MaxAttempts:        5,
		InitialBackoff:     10 * time.Second,
		InternalAlertPhone: phone,
	})
}

func TestEnqueueInternalAlert_HappyPath_GoesToOpsNotCustomer(t *testing.T) {
	orderID := uuid.New()
	store := &fakeJobStore{}
	// Customer punya nomor lain — alert internal TIDAK boleh nyasar ke sini.
	svc := newInternalSvc(store, &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: orderID, Resi: "RJK-RET00001", CustomerID: uuid.New(),
		Status: "proses_cetak", Total: 90000,
	}}, "628999000111")

	err := svc.EnqueueInternalAlert(context.Background(),
		notificationapi.KindDesignRetentionWarning,
		&orderID,
		map[string]any{"days_left": 3, "retention_days": 30, "file_name": "spanduk.cdr"},
		"design_retention_warning:file-1",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store.created == nil {
		t.Fatal("job tidak dibuat")
	}
	if store.created.RecipientPhone != "628999000111" {
		t.Errorf("recipient want nomor ops 628999000111, got %q", store.created.RecipientPhone)
	}
	if !strings.Contains(store.created.Message, "[INTERNAL]") {
		t.Errorf("pesan internal harus ditandai [INTERNAL], got %q", store.created.Message)
	}
	if !strings.Contains(store.created.Message, "RJK-RET00001") {
		t.Errorf("pesan harus menyebut resi, got %q", store.created.Message)
	}
	if !strings.Contains(store.created.Message, "spanduk.cdr") {
		t.Errorf("pesan harus menyebut nama file, got %q", store.created.Message)
	}
	if store.created.DedupKey == nil || *store.created.DedupKey != "design_retention_warning:file-1" {
		t.Errorf("dedup key tidak tersimpan: %v", store.created.DedupKey)
	}
}

func TestEnqueueInternalAlert_NoPhoneConfigured(t *testing.T) {
	orderID := uuid.New()
	store := &fakeJobStore{}
	svc := newInternalSvc(store, &fakeOrderCmd{}, "")

	err := svc.EnqueueInternalAlert(context.Background(),
		notificationapi.KindDesignRetentionWarning, &orderID, nil, "dedup-1")
	if !errors.Is(err, notificationapi.ErrInternalRecipientMissing) {
		t.Fatalf("want ErrInternalRecipientMissing, got %v", err)
	}
	if store.createCalls != 0 {
		t.Error("tidak boleh insert job tanpa tujuan")
	}
}

func TestEnqueueInternalAlert_DuplicateIsNoOp(t *testing.T) {
	orderID := uuid.New()
	store := &fakeJobStore{createErr: repository.ErrDedupConflict}
	svc := newInternalSvc(store, &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: orderID, Resi: "RJK-RET00002", Status: "qc",
	}}, "628999000111")

	err := svc.EnqueueInternalAlert(context.Background(),
		notificationapi.KindDesignRetentionWarning, &orderID,
		map[string]any{"file_name": "x.pdf"}, "design_retention_warning:file-2")
	if err != nil {
		t.Fatalf("alert duplikat harus idempotent no-op, got %v", err)
	}
}

func TestEnqueueInternalAlert_RequiresDedupKey(t *testing.T) {
	orderID := uuid.New()
	store := &fakeJobStore{}
	svc := newInternalSvc(store, &fakeOrderCmd{}, "628999000111")

	if err := svc.EnqueueInternalAlert(context.Background(),
		notificationapi.KindDesignRetentionWarning, &orderID, nil, "  "); err == nil {
		t.Fatal("dedup key kosong harus ditolak — tanpa itu reminder bisa dikirim berulang")
	}
	if store.createCalls != 0 {
		t.Error("tidak boleh insert job tanpa dedup key")
	}
}
