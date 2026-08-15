package service

import (
	"context"
	"errors"
	"io"
	"sort"
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
func (f *fakeProofStore) ListByOrder(_ context.Context, orderID uuid.UUID) ([]paymodel.PaymentProof, error) {
	var out []paymodel.PaymentProof
	for _, p := range f.byID {
		if p.OrderID == orderID {
			out = append(out, *p)
		}
	}
	// Newest first, mirroring the real repository's ORDER BY uploaded_at DESC.
	sort.Slice(out, func(i, j int) bool { return out[i].UploadedAt.After(out[j].UploadedAt) })
	return out, nil
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
	saveErr       error
	saved         map[string][]byte
	deleted       []string
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
	summary    *orderapi.OrderSummary
	summaryErr error
	// summaryByID: separate hook for FindSummaryByID; falls back to summary/summaryErr if nil.
	summaryByID     map[uuid.UUID]*orderapi.OrderSummary
	summaryByIDErr  map[uuid.UUID]error
	pendingErr      error
	pendingCalls    int
	pendingOrderID  uuid.UUID
	dibayarErr      error
	dibayarCalls    int
	ditolakErr      error
	ditolakCalls    int
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

// Locks §4 review finding: authorization (scope + ownership) must run BEFORE
// the status guard, not after. Uses a status that would normally trigger
// ErrPaymentAlreadySettled ("dibayar") combined with a non-owner caller — if
// the status guard still ran first, this would incorrectly return
// ErrPaymentAlreadySettled and let a stranger holding some unrelated valid
// token learn the order's coarse status via HTTP code alone. It must return
// ErrNotOrderOwner instead.
func TestUploadProof_NonOwnerRejected_BeforeStatusGuard(t *testing.T) {
	orderID := uuid.New()
	ownerID := uuid.New()
	strangerID := uuid.New()
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: orderID, CustomerID: ownerID, Status: "dibayar",
	}}
	svc := newSvc(&fakeProofStore{}, &fakeFiles{}, cmd)

	_, err := svc.UploadProof(context.Background(), baseUpload(strangerID))
	if !errors.Is(err, paymentapi.ErrNotOrderOwner) {
		t.Fatalf("want ErrNotOrderOwner (auth before status guard) got %v", err)
	}
}

// Same idea, but through the scope check instead of plain ownership: a
// guest_order token scoped to order A probing order B (status "dibayar",
// which would normally yield ErrPaymentAlreadySettled) must still get
// ErrNotOrderOwner from the scope check, not leak the settled status.
func TestUploadProof_ScopedToDifferentOrder_Rejected_BeforeStatusGuard(t *testing.T) {
	cust := uuid.New()
	orderA, orderB := uuid.New(), uuid.New()
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: orderB, CustomerID: cust, Status: "dibayar",
	}}
	svc := newSvc(&fakeProofStore{}, &fakeFiles{}, cmd)

	in := baseUpload(cust)
	in.ScopedOrderID = &orderA
	_, err := svc.UploadProof(context.Background(), in)
	if !errors.Is(err, paymentapi.ErrNotOrderOwner) {
		t.Fatalf("want ErrNotOrderOwner (scope check before status guard) got %v", err)
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

// Regresi batas scope: token guest_order terbit untuk order A (ScopedOrderID)
// tidak boleh lolos upload bukti ke order B walau memiliki uid customer yang
// sama (mis. nomor WA yang sama pernah order dua kali). Tanpa checkScopedOrder
// ini, hanya ownership check biasa yang jalan — dan itu otomatis lolos karena
// token guest membawa uid customer asli. Juga menegaskan tidak ada efek
// samping (blob/row) yang tertulis saat scope ditolak.
func TestUploadProof_ScopedToDifferentOrder_Rejected(t *testing.T) {
	cust := uuid.New()
	orderA, orderB := uuid.New(), uuid.New()
	store := &fakeProofStore{}
	files := &fakeFiles{}
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: orderB, Resi: "RJK-P0001", CustomerID: cust, Status: "menunggu_pembayaran",
	}}
	svc := newSvc(store, files, cmd)

	in := baseUpload(cust)
	in.ScopedOrderID = &orderA
	_, err := svc.UploadProof(context.Background(), in)
	if !errors.Is(err, paymentapi.ErrNotOrderOwner) {
		t.Fatalf("want ErrNotOrderOwner got %v", err)
	}
	if store.created != nil {
		t.Errorf("no proof row should be created when scope check rejects, got %+v", store.created)
	}
	if len(files.saved) != 0 {
		t.Errorf("no blob should be written when scope check rejects, saved=%v", files.saved)
	}
	if cmd.pendingCalls != 0 {
		t.Errorf("order should not be advanced when scope check rejects, calls=%d", cmd.pendingCalls)
	}
}

// Token ber-scope order A dipakai untuk order A sendiri → berhasil normal.
func TestUploadProof_ScopedToSameOrder_Allowed(t *testing.T) {
	cust := uuid.New()
	orderA := uuid.New()
	store := &fakeProofStore{}
	files := &fakeFiles{}
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: orderA, Resi: "RJK-P0001", CustomerID: cust, Status: "menunggu_pembayaran",
	}}
	svc := newSvc(store, files, cmd)

	in := baseUpload(cust)
	in.ScopedOrderID = &orderA
	got, err := svc.UploadProof(context.Background(), in)
	if err != nil {
		t.Fatalf("scoped token for its own order must be allowed, got %v", err)
	}
	if got.Status != paymodel.ProofPending {
		t.Errorf("proof status not pending: %s", got.Status)
	}
	if store.created == nil {
		t.Fatal("proof row not created")
	}
}

// Sesi penuh (ScopedOrderID nil) — tidak ada regresi terhadap perilaku
// sebelumnya. Ini sama persis dengan TestUploadProof_HappyPath tapi eksplisit
// menegaskan ScopedOrderID nil tidak menambah batasan apapun.
func TestUploadProof_FullSession_NoScopeRestriction(t *testing.T) {
	orderID := uuid.New()
	customerID := uuid.New()
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: orderID, CustomerID: customerID, Status: "menunggu_pembayaran",
	}}
	svc := newSvc(&fakeProofStore{}, &fakeFiles{}, cmd)

	in := baseUpload(customerID)
	in.ScopedOrderID = nil
	if _, err := svc.UploadProof(context.Background(), in); err != nil {
		t.Fatalf("full session upload should succeed, got %v", err)
	}
}

// Guard status §4 tetap berlaku untuk caller guest ber-scope: order sudah
// dibayar → tetap ErrPaymentAlreadySettled, meskipun scope token cocok dengan
// order tersebut. Membuktikan pembukaan akses guest tidak melonggarkan state
// machine.
func TestUploadProof_ScopedGuest_OrderAlreadySettled(t *testing.T) {
	orderID := uuid.New()
	customerID := uuid.New()
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: orderID, CustomerID: customerID, Status: "dibayar",
	}}
	svc := newSvc(&fakeProofStore{}, &fakeFiles{}, cmd)

	in := baseUpload(customerID)
	in.ScopedOrderID = &orderID
	_, err := svc.UploadProof(context.Background(), in)
	if !errors.Is(err, orderapi.ErrPaymentAlreadySettled) {
		t.Fatalf("want ErrPaymentAlreadySettled got %v", err)
	}
}

// Guard status §4 tetap berlaku untuk caller guest ber-scope: status di luar
// menunggu_pembayaran/ditolak → tetap ErrOrderNotPayable.
func TestUploadProof_ScopedGuest_OrderNotPayable(t *testing.T) {
	orderID := uuid.New()
	customerID := uuid.New()
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: orderID, CustomerID: customerID, Status: "menunggu_verifikasi",
	}}
	svc := newSvc(&fakeProofStore{}, &fakeFiles{}, cmd)

	in := baseUpload(customerID)
	in.ScopedOrderID = &orderID
	_, err := svc.UploadProof(context.Background(), in)
	if !errors.Is(err, paymentapi.ErrOrderNotPayable) {
		t.Fatalf("want ErrOrderNotPayable got %v", err)
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

	got, err := svc.GetProofFile(context.Background(), proofID, staffID, true, nil)
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

	got, err := svc.GetProofFile(context.Background(), proofID, customerID, false, nil)
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

	_, err := svc.GetProofFile(context.Background(), proofID, stranger, false, nil)
	if !errors.Is(err, paymentapi.ErrNotOrderOwner) {
		t.Fatalf("want ErrNotOrderOwner got %v", err)
	}
}

func TestGetProofFile_ProofNotFound(t *testing.T) {
	svc := newSvc(&fakeProofStore{}, &fakeFiles{}, &fakeOrderCmd{})
	_, err := svc.GetProofFile(context.Background(), uuid.New(), uuid.New(), false, nil)
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

	_, err := svc.GetProofFile(context.Background(), proofID, uuid.New(), false, nil)
	if !errors.Is(err, paymentapi.ErrProofNotFound) {
		t.Fatalf("want ErrProofNotFound got %v", err)
	}
}

// Regresi batas scope: token guest_order terbit untuk order A (ScopedOrderID)
// tidak boleh lolos mengunduh blob bukti bayar order B walau memiliki uid
// customer yang sama. Sebelum fix ini, checkScopedOrder dipanggil di
// GetProofFile tapi TIDAK ada satu pun test yang mengoper scopedOrderID
// non-nil — seluruh suite tetap hijau meskipun baris cek itu dihapus.
func TestGetProofFile_ScopedToDifferentOrder_Rejected(t *testing.T) {
	orderA, orderB := uuid.New(), uuid.New()
	proofID := uuid.New()
	proof := &paymodel.PaymentProof{ID: proofID, OrderID: orderB, FilePath: "payment_proofs/x.png"}
	store := &fakeProofStore{byID: map[uuid.UUID]*paymodel.PaymentProof{proofID: proof}}
	svc := newSvc(store, &fakeFiles{}, &fakeOrderCmd{})

	_, err := svc.GetProofFile(context.Background(), proofID, uuid.New(), false, &orderA)
	if !errors.Is(err, paymentapi.ErrNotOrderOwner) {
		t.Fatalf("want ErrNotOrderOwner got %v", err)
	}
}

// Token ber-scope order B dipakai untuk order B sendiri → berhasil normal.
func TestGetProofFile_ScopedToSameOrder_Allowed(t *testing.T) {
	cust := uuid.New()
	orderB := uuid.New()
	proofID := uuid.New()
	proof := &paymodel.PaymentProof{ID: proofID, OrderID: orderB, FilePath: "payment_proofs/x.png"}
	store := &fakeProofStore{byID: map[uuid.UUID]*paymodel.PaymentProof{proofID: proof}}
	cmd := &fakeOrderCmd{summaryByID: map[uuid.UUID]*orderapi.OrderSummary{
		orderB: {ID: orderB, CustomerID: cust},
	}}
	svc := newSvc(store, &fakeFiles{}, cmd)

	if _, err := svc.GetProofFile(context.Background(), proofID, cust, false, &orderB); err != nil {
		t.Fatalf("scoped token for its own order must be allowed, got %v", err)
	}
}

// Mirrors internal/design/service/design_service_test.go's
// TestGetFile_ScopedEnforcedEvenForStaffCaller: the scope-to-one-order
// restriction must hold even for isStaff=true — the comment in
// checkScopedOrder's call site already claimed this invariant; this test is
// what actually locks it. Without it, deleting the checkScopedOrder call
// entirely would leave every OTHER test in this file green (they all pass
// scopedOrderID=nil).
func TestGetProofFile_ScopedEnforcedEvenForStaffCaller(t *testing.T) {
	orderA, orderB := uuid.New(), uuid.New()
	proofID := uuid.New()
	proof := &paymodel.PaymentProof{ID: proofID, OrderID: orderB, FilePath: "payment_proofs/x.png"}
	store := &fakeProofStore{byID: map[uuid.UUID]*paymodel.PaymentProof{proofID: proof}}
	svc := newSvc(store, &fakeFiles{}, &fakeOrderCmd{})

	if _, err := svc.GetProofFile(context.Background(), proofID, uuid.New(), true, &orderA); !errors.Is(err, paymentapi.ErrNotOrderOwner) {
		t.Fatalf("scoped token must be order-bound even for staff caller, got %v", err)
	}
}

// ---------- ListForOrder (customer-facing: GET /orders/:resi/payment-proofs) ----------

func TestListForOrder_OwnerSeesOwnProofs_NewestFirst(t *testing.T) {
	orderID := uuid.New()
	customerID := uuid.New()
	older := &paymodel.PaymentProof{
		ID: uuid.New(), OrderID: orderID, Status: paymodel.ProofRejected,
		UploadedAt: time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC),
	}
	newer := &paymodel.PaymentProof{
		ID: uuid.New(), OrderID: orderID, Status: paymodel.ProofPending,
		UploadedAt: time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC),
	}
	store := &fakeProofStore{byID: map[uuid.UUID]*paymodel.PaymentProof{
		older.ID: older, newer.ID: newer,
	}}
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: orderID, Resi: "RJK-P0001", CustomerID: customerID, Status: "menunggu_verifikasi",
	}}
	svc := newSvc(store, &fakeFiles{}, cmd)

	items, err := svc.ListForOrder(context.Background(), "RJK-P0001", customerID, false, nil)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("want 2 items, got %d", len(items))
	}
	if items[0].ID != newer.ID || items[1].ID != older.ID {
		t.Errorf("want newest first: got order %v, %v", items[0].ID, items[1].ID)
	}
}

func TestListForOrder_NonOwnerRejected(t *testing.T) {
	orderID := uuid.New()
	owner := uuid.New()
	stranger := uuid.New()
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: orderID, Resi: "RJK-P0001", CustomerID: owner, Status: "menunggu_verifikasi",
	}}
	svc := newSvc(&fakeProofStore{}, &fakeFiles{}, cmd)

	_, err := svc.ListForOrder(context.Background(), "RJK-P0001", stranger, false, nil)
	if !errors.Is(err, paymentapi.ErrNotOrderOwner) {
		t.Fatalf("want ErrNotOrderOwner got %v", err)
	}
}

func TestListForOrder_StaffCanSeeAnyOrder(t *testing.T) {
	orderID := uuid.New()
	owner := uuid.New()
	staffID := uuid.New()
	proof := &paymodel.PaymentProof{ID: uuid.New(), OrderID: orderID, Status: paymodel.ProofApproved}
	store := &fakeProofStore{byID: map[uuid.UUID]*paymodel.PaymentProof{proof.ID: proof}}
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: orderID, Resi: "RJK-P0001", CustomerID: owner, Status: "dibayar",
	}}
	svc := newSvc(store, &fakeFiles{}, cmd)

	items, err := svc.ListForOrder(context.Background(), "RJK-P0001", staffID, true, nil)
	if err != nil {
		t.Fatalf("staff should be able to list any order's proofs, got %v", err)
	}
	if len(items) != 1 {
		t.Errorf("want 1 item, got %d", len(items))
	}
}

func TestListForOrder_OrderNotFound(t *testing.T) {
	cmd := &fakeOrderCmd{summaryErr: orderapi.ErrOrderNotFound}
	svc := newSvc(&fakeProofStore{}, &fakeFiles{}, cmd)

	_, err := svc.ListForOrder(context.Background(), "RJK-GHOST", uuid.New(), false, nil)
	if !errors.Is(err, orderapi.ErrOrderNotFound) {
		t.Fatalf("want ErrOrderNotFound got %v", err)
	}
}

// Regresi batas scope: token guest_order terbit untuk resi A (ScopedOrderID)
// tidak boleh lolos membaca daftar bukti bayar order B walau memiliki uid
// customer yang sama (mis. nomor WA yang sama pernah order dua kali). Tanpa
// checkScopedOrder ini, hanya ownership check biasa yang jalan — dan itu
// otomatis lolos karena token guest membawa uid customer asli.
func TestListForOrder_ScopedToDifferentOrder_Rejected(t *testing.T) {
	cust := uuid.New()
	orderA, orderB := uuid.New(), uuid.New()
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: orderB, Resi: "RJK-B", CustomerID: cust, Status: "menunggu_verifikasi",
	}}
	svc := newSvc(&fakeProofStore{}, &fakeFiles{}, cmd)

	_, err := svc.ListForOrder(context.Background(), "RJK-B", cust, false, &orderA)
	if !errors.Is(err, paymentapi.ErrNotOrderOwner) {
		t.Fatalf("want ErrNotOrderOwner listing another order with scoped token, got %v", err)
	}
}

func TestListForOrder_ScopedToSameOrder_Allowed(t *testing.T) {
	cust := uuid.New()
	orderB := uuid.New()
	proof := &paymodel.PaymentProof{ID: uuid.New(), OrderID: orderB, Status: paymodel.ProofPending}
	store := &fakeProofStore{byID: map[uuid.UUID]*paymodel.PaymentProof{proof.ID: proof}}
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: orderB, Resi: "RJK-B", CustomerID: cust, Status: "menunggu_verifikasi",
	}}
	svc := newSvc(store, &fakeFiles{}, cmd)

	items, err := svc.ListForOrder(context.Background(), "RJK-B", cust, false, &orderB)
	if err != nil {
		t.Fatalf("scoped token for its own order must be allowed, got %v", err)
	}
	if len(items) != 1 {
		t.Errorf("want 1 item, got %d", len(items))
	}
}
