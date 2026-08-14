package service

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/order/orderapi"
	paymodel "github.com/rajaku-printing/backend/internal/payment/model"
	"github.com/rajaku-printing/backend/internal/payment/paymentapi"
	payrepo "github.com/rajaku-printing/backend/internal/payment/repository"
)

// ---------- fakes ----------

type fakeProofStore struct {
	createErr    error
	created      *paymodel.PaymentProof
	byID         map[uuid.UUID]*paymodel.PaymentProof
	pendingByOrd map[uuid.UUID]*paymodel.PaymentProof
	reviewErr    error
	reviewed     *payrepo.ReviewParams
	deletedIDs   []uuid.UUID
	listResult   *payrepo.ListResult
}

func (f *fakeProofStore) Create(_ context.Context, p *paymodel.PaymentProof) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.created = p
	if f.byID == nil {
		f.byID = map[uuid.UUID]*paymodel.PaymentProof{}
	}
	f.byID[p.ID] = p
	return nil
}
func (f *fakeProofStore) FindByID(_ context.Context, id uuid.UUID) (*paymodel.PaymentProof, error) {
	if p, ok := f.byID[id]; ok {
		return p, nil
	}
	return nil, payrepo.ErrNotFound
}
func (f *fakeProofStore) FindPendingByOrder(_ context.Context, orderID uuid.UUID) (*paymodel.PaymentProof, error) {
	if p, ok := f.pendingByOrd[orderID]; ok {
		return p, nil
	}
	return nil, payrepo.ErrNotFound
}
func (f *fakeProofStore) List(_ context.Context, _ payrepo.ListFilter) (*payrepo.ListResult, error) {
	return f.listResult, nil
}
func (f *fakeProofStore) Review(_ context.Context, p payrepo.ReviewParams) error {
	if f.reviewErr != nil {
		return f.reviewErr
	}
	f.reviewed = &p
	if existing := f.byID[p.ID]; existing != nil {
		existing.Status = p.NewStatus
		existing.ReviewedAt = &p.ReviewedAt
		reviewer := p.ReviewedBy
		existing.ReviewedBy = &reviewer
		if p.NewStatus == paymodel.ProofRejected {
			r := p.RejectReason
			existing.RejectReason = &r
		}
	}
	return nil
}
func (f *fakeProofStore) DeleteRow(_ context.Context, id uuid.UUID) error {
	f.deletedIDs = append(f.deletedIDs, id)
	delete(f.byID, id)
	return nil
}

type fakeFiles struct {
	saveErr   error
	saved     map[string][]byte
	deleted   []string
	forceMismatch bool // force written != declared size
}

func (f *fakeFiles) Save(_ context.Context, subpath string, r io.Reader) (int64, error) {
	if f.saveErr != nil {
		return 0, f.saveErr
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return 0, err
	}
	if f.saved == nil {
		f.saved = map[string][]byte{}
	}
	f.saved[subpath] = b
	if f.forceMismatch {
		return int64(len(b)) + 1, nil
	}
	return int64(len(b)), nil
}
func (f *fakeFiles) Delete(_ context.Context, subpath string) error {
	f.deleted = append(f.deleted, subpath)
	if f.saved != nil {
		delete(f.saved, subpath)
	}
	return nil
}
func (f *fakeFiles) AbsPath(subpath string) (string, error) {
	// Test double: return a deterministic pseudo-absolute path.
	return "/tmp/rajaku-test/" + subpath, nil
}

type fakeOrderCmd struct {
	summary        *orderapi.OrderSummary
	summaryErr     error
	// summaryByID: separate hook for FindSummaryByID; falls back to summary/summaryErr if nil.
	summaryByID    map[uuid.UUID]*orderapi.OrderSummary
	summaryByIDErr map[uuid.UUID]error
	pendingErr     error
	pendingCalls   int
	pendingOrderID uuid.UUID
	dibayarErr     error
	dibayarCalls   int
	ditolakErr     error
	ditolakCalls   int
	lastMetodeBayar string
	lastReason      string
}

func (f *fakeOrderCmd) FindSummaryByResi(_ context.Context, _ string) (*orderapi.OrderSummary, error) {
	if f.summaryErr != nil {
		return nil, f.summaryErr
	}
	return f.summary, nil
}
func (f *fakeOrderCmd) FindSummaryByID(_ context.Context, id uuid.UUID) (*orderapi.OrderSummary, error) {
	if err, ok := f.summaryByIDErr[id]; ok {
		return nil, err
	}
	if s, ok := f.summaryByID[id]; ok {
		return s, nil
	}
	if f.summaryErr != nil {
		return nil, f.summaryErr
	}
	return f.summary, nil
}
func (f *fakeOrderCmd) MarkPendingVerification(_ context.Context, orderID uuid.UUID, _ *uuid.UUID, _ string) error {
	f.pendingCalls++
	f.pendingOrderID = orderID
	return f.pendingErr
}
func (f *fakeOrderCmd) MarkDibayar(_ context.Context, _ uuid.UUID, metodeBayar string, _ *uuid.UUID, _ string) error {
	f.dibayarCalls++
	f.lastMetodeBayar = metodeBayar
	return f.dibayarErr
}
func (f *fakeOrderCmd) MarkDitolak(_ context.Context, _ uuid.UUID, _ *uuid.UUID, reason string) error {
	f.ditolakCalls++
	f.lastReason = reason
	return f.ditolakErr
}
func (f *fakeOrderCmd) MarkDesainDikerjakan(_ context.Context, _ uuid.UUID, _ *uuid.UUID, _ string) error {
	return nil
}
func (f *fakeOrderCmd) MarkMenungguApprovalDesain(_ context.Context, _ uuid.UUID, _ *uuid.UUID, _ string) error {
	return nil
}
func (f *fakeOrderCmd) MarkDesainDiverifikasi(_ context.Context, _ uuid.UUID, _ *uuid.UUID, _ string) error {
	return nil
}
func (f *fakeOrderCmd) MarkProsesCetak(_ context.Context, _ uuid.UUID, _ *uuid.UUID, _ string) error {
	return nil
}
func (f *fakeOrderCmd) MarkQC(_ context.Context, _ uuid.UUID, _ *uuid.UUID, _ string) error {
	return nil
}
func (f *fakeOrderCmd) MarkSiapKirimAtauAmbil(_ context.Context, _ uuid.UUID, _ *uuid.UUID, _ string) error {
	return nil
}
func (f *fakeOrderCmd) MarkDikirim(_ context.Context, _ uuid.UUID, _ *uuid.UUID, _, _, _ string) error {
	return nil
}
func (f *fakeOrderCmd) MarkSelesai(_ context.Context, _ uuid.UUID, _ *uuid.UUID, _ string) error {
	return nil
}
func (f *fakeOrderCmd) FindInvoiceViewByID(_ context.Context, _ uuid.UUID) (*orderapi.OrderInvoiceView, error) {
	return nil, nil
}
func (f *fakeOrderCmd) CreatePOSOrder(_ context.Context, _ orderapi.POSCreateOrderInput) (*orderapi.OrderSummary, error) {
	return nil, nil
}
func (f *fakeOrderCmd) ListPOSOrdersByDate(_ context.Context, _ time.Time) ([]orderapi.OrderSummary, error) {
	return nil, nil
}

// ---------- helpers ----------

func newSvc(store *fakeProofStore, files *fakeFiles, cmd *fakeOrderCmd) *Service {
	s := New(store, files, cmd, Config{MaxUploadMB: 1})
	s.nowFn = func() time.Time { return time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC) }
	return s
}

func baseUpload(customerID uuid.UUID) UploadProofInput {
	return UploadProofInput{
		Resi:         "RJK-P0001",
		CallerID:     customerID,
		IsStaff:      false,
		MetodeBayar:  paymodel.MetodeBayarTransfer,
		FileReader:   strings.NewReader("fake-image-bytes"),
		FileSize:     int64(len("fake-image-bytes")),
		OriginalName: "bukti.jpg",
		MimeType:     "image/jpeg",
	}
}

// ---------- UploadProof ----------

func TestUploadProof_HappyPath(t *testing.T) {
	orderID := uuid.New()
	customerID := uuid.New()
	store := &fakeProofStore{}
	files := &fakeFiles{}
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: orderID, Resi: "RJK-P0001", CustomerID: customerID,
		Status: "menunggu_pembayaran", Total: 100000,
	}}
	svc := newSvc(store, files, cmd)

	got, err := svc.UploadProof(context.Background(), baseUpload(customerID))
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got.Status != paymodel.ProofPending {
		t.Errorf("proof status not pending: %s", got.Status)
	}
	if store.created == nil {
		t.Fatal("proof row not created")
	}
	if len(files.saved) != 1 {
		t.Errorf("want 1 file saved, got %d", len(files.saved))
	}
	if cmd.pendingCalls != 1 || cmd.pendingOrderID != orderID {
		t.Errorf("order not advanced: calls=%d orderID=%s", cmd.pendingCalls, cmd.pendingOrderID)
	}
	// File subpath under payment_proofs/2026/08/…
	if !strings.HasPrefix(got.FilePath, "payment_proofs/2026/08/") {
		t.Errorf("subpath layout wrong: %s", got.FilePath)
	}
}

func TestUploadProof_FromDitolakAlsoAllowed(t *testing.T) {
	orderID := uuid.New()
	customerID := uuid.New()
	store := &fakeProofStore{}
	files := &fakeFiles{}
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: orderID, CustomerID: customerID, Status: "ditolak",
	}}
	svc := newSvc(store, files, cmd)
	if _, err := svc.UploadProof(context.Background(), baseUpload(customerID)); err != nil {
		t.Fatalf("upload from ditolak should be allowed, got %v", err)
	}
}

func TestUploadProof_OrderAlreadySettled(t *testing.T) {
	orderID := uuid.New()
	customerID := uuid.New()
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: orderID, CustomerID: customerID, Status: "dibayar",
	}}
	svc := newSvc(&fakeProofStore{}, &fakeFiles{}, cmd)
	_, err := svc.UploadProof(context.Background(), baseUpload(customerID))
	if !errors.Is(err, orderapi.ErrPaymentAlreadySettled) {
		t.Fatalf("want ErrPaymentAlreadySettled got %v", err)
	}
}

func TestUploadProof_WrongOwnerRejected(t *testing.T) {
	orderID := uuid.New()
	ownerID := uuid.New()
	otherID := uuid.New()
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: orderID, CustomerID: ownerID, Status: "menunggu_pembayaran",
	}}
	svc := newSvc(&fakeProofStore{}, &fakeFiles{}, cmd)

	_, err := svc.UploadProof(context.Background(), baseUpload(otherID))
	if !errors.Is(err, paymentapi.ErrNotOrderOwner) {
		t.Fatalf("want ErrNotOrderOwner got %v", err)
	}
}

func TestUploadProof_StaffBypassesOwnershipCheck(t *testing.T) {
	orderID := uuid.New()
	ownerID := uuid.New()
	staffID := uuid.New()
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: orderID, CustomerID: ownerID, Status: "menunggu_pembayaran",
	}}
	svc := newSvc(&fakeProofStore{}, &fakeFiles{}, cmd)

	in := baseUpload(staffID)
	in.IsStaff = true
	if _, err := svc.UploadProof(context.Background(), in); err != nil {
		t.Fatalf("staff upload should succeed, got %v", err)
	}
}

func TestUploadProof_ExistingPending_Rejected(t *testing.T) {
	orderID := uuid.New()
	customerID := uuid.New()
	existing := &paymodel.PaymentProof{ID: uuid.New(), OrderID: orderID, Status: paymodel.ProofPending}
	store := &fakeProofStore{pendingByOrd: map[uuid.UUID]*paymodel.PaymentProof{orderID: existing}}
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: orderID, CustomerID: customerID, Status: "menunggu_pembayaran",
	}}
	svc := newSvc(store, &fakeFiles{}, cmd)

	_, err := svc.UploadProof(context.Background(), baseUpload(customerID))
	if !errors.Is(err, paymentapi.ErrPendingProofExists) {
		t.Fatalf("want ErrPendingProofExists got %v", err)
	}
}

func TestUploadProof_InvalidMimeType_Rejected(t *testing.T) {
	orderID := uuid.New()
	customerID := uuid.New()
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: orderID, CustomerID: customerID, Status: "menunggu_pembayaran",
	}}
	svc := newSvc(&fakeProofStore{}, &fakeFiles{}, cmd)
	in := baseUpload(customerID)
	in.MimeType = "application/x-msdownload"
	_, err := svc.UploadProof(context.Background(), in)
	if !errors.Is(err, paymentapi.ErrInvalidMimeType) {
		t.Fatalf("want ErrInvalidMimeType got %v", err)
	}
}

func TestUploadProof_FileTooLarge_Rejected(t *testing.T) {
	orderID := uuid.New()
	customerID := uuid.New()
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: orderID, CustomerID: customerID, Status: "menunggu_pembayaran",
	}}
	svc := newSvc(&fakeProofStore{}, &fakeFiles{}, cmd) // limit 1 MB
	in := baseUpload(customerID)
	in.FileSize = 5 * 1024 * 1024 // 5 MB
	_, err := svc.UploadProof(context.Background(), in)
	if !errors.Is(err, paymentapi.ErrFileTooLarge) {
		t.Fatalf("want ErrFileTooLarge got %v", err)
	}
}

func TestUploadProof_OrderAdvanceFailure_RollsBack(t *testing.T) {
	orderID := uuid.New()
	customerID := uuid.New()
	store := &fakeProofStore{}
	files := &fakeFiles{}
	cmd := &fakeOrderCmd{
		summary: &orderapi.OrderSummary{
			ID: orderID, CustomerID: customerID, Status: "menunggu_pembayaran",
		},
		pendingErr: orderapi.ErrOrderStateChanged, // order advanced elsewhere while we were uploading
	}
	svc := newSvc(store, files, cmd)

	_, err := svc.UploadProof(context.Background(), baseUpload(customerID))
	if err == nil {
		t.Fatal("expected upload to fail when order advance fails")
	}
	if len(store.deletedIDs) != 1 {
		t.Errorf("want proof row rolled back, deleted=%v", store.deletedIDs)
	}
	if len(files.deleted) != 1 {
		t.Errorf("want file rolled back, deleted=%v", files.deleted)
	}
}

// ---------- ApproveProof ----------

func TestApproveProof_HappyPath(t *testing.T) {
	proofID := uuid.New()
	orderID := uuid.New()
	staffID := uuid.New()
	proof := &paymodel.PaymentProof{
		ID: proofID, OrderID: orderID,
		MetodeBayar: paymodel.MetodeBayarTransfer,
		Status:      paymodel.ProofPending,
	}
	store := &fakeProofStore{byID: map[uuid.UUID]*paymodel.PaymentProof{proofID: proof}}
	cmd := &fakeOrderCmd{}
	svc := newSvc(store, &fakeFiles{}, cmd)

	got, err := svc.ApproveProof(context.Background(), ReviewInput{ProofID: proofID, StaffID: staffID})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got.Status != paymodel.ProofApproved {
		t.Errorf("want approved, got %s", got.Status)
	}
	if cmd.dibayarCalls != 1 {
		t.Errorf("want order.MarkDibayar called once, got %d", cmd.dibayarCalls)
	}
	if cmd.lastMetodeBayar != "transfer" {
		t.Errorf("want metode_bayar=transfer, got %s", cmd.lastMetodeBayar)
	}
}

func TestApproveProof_AlreadyReviewed(t *testing.T) {
	proofID := uuid.New()
	proof := &paymodel.PaymentProof{ID: proofID, Status: paymodel.ProofApproved}
	store := &fakeProofStore{byID: map[uuid.UUID]*paymodel.PaymentProof{proofID: proof}}
	svc := newSvc(store, &fakeFiles{}, &fakeOrderCmd{})

	_, err := svc.ApproveProof(context.Background(), ReviewInput{ProofID: proofID, StaffID: uuid.New()})
	if !errors.Is(err, paymentapi.ErrProofAlreadyReviewed) {
		t.Fatalf("want ErrProofAlreadyReviewed got %v", err)
	}
}

func TestApproveProof_NotFound(t *testing.T) {
	store := &fakeProofStore{}
	svc := newSvc(store, &fakeFiles{}, &fakeOrderCmd{})
	_, err := svc.ApproveProof(context.Background(), ReviewInput{ProofID: uuid.New(), StaffID: uuid.New()})
	if !errors.Is(err, paymentapi.ErrProofNotFound) {
		t.Fatalf("want ErrProofNotFound got %v", err)
	}
}

// ---------- RejectProof ----------

func TestRejectProof_HappyPath(t *testing.T) {
	proofID := uuid.New()
	orderID := uuid.New()
	staffID := uuid.New()
	proof := &paymodel.PaymentProof{
		ID: proofID, OrderID: orderID, Status: paymodel.ProofPending,
		MetodeBayar: paymodel.MetodeBayarTransfer,
	}
	store := &fakeProofStore{byID: map[uuid.UUID]*paymodel.PaymentProof{proofID: proof}}
	cmd := &fakeOrderCmd{}
	svc := newSvc(store, &fakeFiles{}, cmd)

	got, err := svc.RejectProof(context.Background(), ReviewInput{
		ProofID: proofID, StaffID: staffID, Reason: "bukti tidak terbaca",
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got.Status != paymodel.ProofRejected {
		t.Errorf("want rejected, got %s", got.Status)
	}
	if cmd.ditolakCalls != 1 {
		t.Errorf("want order.MarkDitolak called once, got %d", cmd.ditolakCalls)
	}
	if cmd.lastReason != "bukti tidak terbaca" {
		t.Errorf("want reason forwarded, got %q", cmd.lastReason)
	}
}

func TestRejectProof_EmptyReason_Rejected(t *testing.T) {
	svc := newSvc(&fakeProofStore{}, &fakeFiles{}, &fakeOrderCmd{})
	_, err := svc.RejectProof(context.Background(), ReviewInput{
		ProofID: uuid.New(), StaffID: uuid.New(), Reason: "   ",
	})
	if !errors.Is(err, paymentapi.ErrRejectReasonRequired) {
		t.Fatalf("want ErrRejectReasonRequired got %v", err)
	}
}

// ---------- GetProofFile ----------

func TestGetProofFile_StaffCanAccessAny(t *testing.T) {
	proofID := uuid.New()
	orderID := uuid.New()
	staffID := uuid.New()
	proof := &paymodel.PaymentProof{
		ID: proofID, OrderID: orderID,
		FilePath: "payment_proofs/2026/08/abc.jpg",
	}
	store := &fakeProofStore{byID: map[uuid.UUID]*paymodel.PaymentProof{proofID: proof}}
	// No summaryByID needed — staff bypass means orderCmd.FindSummaryByID is not called.
	cmd := &fakeOrderCmd{}
	svc := newSvc(store, &fakeFiles{}, cmd)

	got, err := svc.GetProofFile(context.Background(), proofID, staffID, true)
	if err != nil {
		t.Fatalf("staff access should succeed, got %v", err)
	}
	if got.Proof.ID != proofID {
		t.Errorf("proof id mismatch")
	}
	if got.AbsPath == "" {
		t.Errorf("abs path missing")
	}
}

func TestGetProofFile_OwnerCustomerAllowed(t *testing.T) {
	proofID := uuid.New()
	orderID := uuid.New()
	customerID := uuid.New()
	proof := &paymodel.PaymentProof{ID: proofID, OrderID: orderID, FilePath: "payment_proofs/x.png"}
	store := &fakeProofStore{byID: map[uuid.UUID]*paymodel.PaymentProof{proofID: proof}}
	cmd := &fakeOrderCmd{
		summaryByID: map[uuid.UUID]*orderapi.OrderSummary{
			orderID: {ID: orderID, CustomerID: customerID, Status: "menunggu_verifikasi"},
		},
	}
	svc := newSvc(store, &fakeFiles{}, cmd)

	got, err := svc.GetProofFile(context.Background(), proofID, customerID, false)
	if err != nil {
		t.Fatalf("owner access should succeed, got %v", err)
	}
	if got.Proof.ID != proofID {
		t.Errorf("proof id mismatch")
	}
}

func TestGetProofFile_NonOwnerCustomerRejected(t *testing.T) {
	proofID := uuid.New()
	orderID := uuid.New()
	owner := uuid.New()
	stranger := uuid.New()
	proof := &paymodel.PaymentProof{ID: proofID, OrderID: orderID, FilePath: "payment_proofs/x.png"}
	store := &fakeProofStore{byID: map[uuid.UUID]*paymodel.PaymentProof{proofID: proof}}
	cmd := &fakeOrderCmd{
		summaryByID: map[uuid.UUID]*orderapi.OrderSummary{
			orderID: {ID: orderID, CustomerID: owner},
		},
	}
	svc := newSvc(store, &fakeFiles{}, cmd)

	_, err := svc.GetProofFile(context.Background(), proofID, stranger, false)
	if !errors.Is(err, paymentapi.ErrNotOrderOwner) {
		t.Fatalf("want ErrNotOrderOwner got %v", err)
	}
}

func TestGetProofFile_ProofNotFound(t *testing.T) {
	svc := newSvc(&fakeProofStore{}, &fakeFiles{}, &fakeOrderCmd{})
	_, err := svc.GetProofFile(context.Background(), uuid.New(), uuid.New(), false)
	if !errors.Is(err, paymentapi.ErrProofNotFound) {
		t.Fatalf("want ErrProofNotFound got %v", err)
	}
}

func TestGetProofFile_OrphanOrderTreatedAsNotFound(t *testing.T) {
	// If the parent order lookup fails, don't leak the proof's existence to a
	// non-staff caller — return ErrProofNotFound instead of a server error.
	proofID := uuid.New()
	orderID := uuid.New()
	proof := &paymodel.PaymentProof{ID: proofID, OrderID: orderID, FilePath: "payment_proofs/x.png"}
	store := &fakeProofStore{byID: map[uuid.UUID]*paymodel.PaymentProof{proofID: proof}}
	cmd := &fakeOrderCmd{
		summaryByIDErr: map[uuid.UUID]error{orderID: orderapi.ErrOrderNotFound},
	}
	svc := newSvc(store, &fakeFiles{}, cmd)

	_, err := svc.GetProofFile(context.Background(), proofID, uuid.New(), false)
	if !errors.Is(err, paymentapi.ErrProofNotFound) {
		t.Fatalf("want ErrProofNotFound got %v", err)
	}
}
