package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/invoice/invoiceapi"
	"github.com/rajaku-printing/backend/internal/invoice/model"
	"github.com/rajaku-printing/backend/internal/invoice/pdfrender"
	invrepo "github.com/rajaku-printing/backend/internal/invoice/repository"
	"github.com/rajaku-printing/backend/internal/notification/notificationapi"
	"github.com/rajaku-printing/backend/internal/order/orderapi"
)

// ---------- fakes ----------

type fakeInvoiceStore struct {
	byOrder         *model.Invoice
	byOrderErr      error
	byID            *model.Invoice
	byIDErr         error
	created         *model.Invoice
	createErr       error
	createCalls     int
	nextNumberValue int
	nextNumberErr   error
}

func (f *fakeInvoiceStore) Create(_ context.Context, inv *model.Invoice) error {
	f.createCalls++
	if f.createErr != nil {
		return f.createErr
	}
	f.created = inv
	return nil
}
func (f *fakeInvoiceStore) FindByID(_ context.Context, _ uuid.UUID) (*model.Invoice, error) {
	return f.byID, f.byIDErr
}
func (f *fakeInvoiceStore) FindByOrderID(_ context.Context, _ uuid.UUID) (*model.Invoice, error) {
	return f.byOrder, f.byOrderErr
}
func (f *fakeInvoiceStore) NextInvoiceNumber(_ context.Context, _ int) (int, error) {
	if f.nextNumberErr != nil {
		return 0, f.nextNumberErr
	}
	if f.nextNumberValue == 0 {
		return 1, nil
	}
	return f.nextNumberValue, nil
}

type fakeBlobs struct {
	saveCalls   int
	saveErr     error
	lastSubpath string
	written     int64
	deleteCalls int
}

func (f *fakeBlobs) Save(_ context.Context, subpath string, r io.Reader) (int64, error) {
	f.saveCalls++
	f.lastSubpath = subpath
	if f.saveErr != nil {
		return 0, f.saveErr
	}
	n, _ := io.Copy(io.Discard, r)
	if f.written > 0 {
		return f.written, nil
	}
	return n, nil
}
func (f *fakeBlobs) Delete(_ context.Context, _ string) error {
	f.deleteCalls++
	return nil
}
func (f *fakeBlobs) AbsPath(subpath string) (string, error) {
	return "/mock/" + subpath, nil
}

type fakeOrderCmd struct {
	view    *orderapi.OrderInvoiceView
	viewErr error

	summary    *orderapi.OrderSummary
	summaryErr error
}

func (f *fakeOrderCmd) FindSummaryByResi(context.Context, string) (*orderapi.OrderSummary, error) {
	return nil, nil
}
func (f *fakeOrderCmd) FindSummaryByID(context.Context, uuid.UUID) (*orderapi.OrderSummary, error) {
	return f.summary, f.summaryErr
}
func (f *fakeOrderCmd) FindInvoiceViewByID(context.Context, uuid.UUID) (*orderapi.OrderInvoiceView, error) {
	return f.view, f.viewErr
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

type fakeNotifier struct {
	calls      int
	lastKind   notificationapi.Kind
	lastExtras map[string]any
}

func (f *fakeNotifier) EnqueueOrderEvent(_ context.Context, kind notificationapi.Kind, _ uuid.UUID, extras map[string]any) error {
	f.calls++
	f.lastKind = kind
	f.lastExtras = extras
	return nil
}

// ---------- helpers ----------

func validOrderView() *orderapi.OrderInvoiceView {
	shipping := int64(15000)
	addr := "Jl. Contoh 1"
	name := "Ali"
	return &orderapi.OrderInvoiceView{
		ID:                uuid.New(),
		Resi:              "RJK-INV1",
		CustomerID:        uuid.New(),
		CreatedAt:         time.Now().UTC(),
		Channel:           "online",
		Status:            "dibayar",
		MetodeAmbil:       "kirim",
		MetodeBayar:       "transfer",
		ProductName:       "Banner Vinyl",
		MaterialName:      "Frontlite 340g",
		WidthCm:           100,
		HeightCm:          200,
		Quantity:          1,
		UnitPrice:         50000,
		Subtotal:          50000,
		ShippingCost:      &shipping,
		ShippingRecipient: &name,
		ShippingAddress:   &addr,
		Total:             65000,
	}
}

func newSvc(store *fakeInvoiceStore, blobs *fakeBlobs, cmd *fakeOrderCmd, cs *fakeCustomers) *Service {
	s := New(store, blobs, cmd, cs, Config{
		BaseURL: "https://rajaku.test",
		Company: pdfrender.CompanyInfo{Name: "Rajaku Test"},
	})
	s.nowFn = func() time.Time { return time.Date(2026, 8, 12, 3, 0, 0, 0, time.UTC) } // 10 WIB
	return s
}

// ---------- tests ----------

func TestGenerateForOrder_HappyPath_CreatesInvoiceAndSendsWA(t *testing.T) {
	view := validOrderView()
	store := &fakeInvoiceStore{
		byOrderErr:      invrepo.ErrNotFound, // no existing
		nextNumberValue: 42,
	}
	blobs := &fakeBlobs{}
	cmd := &fakeOrderCmd{view: view}
	custs := &fakeCustomers{identity: &authapi.Identity{Name: "Ali", Phone: "6281234"}}
	notif := &fakeNotifier{}
	svc := newSvc(store, blobs, cmd, custs)
	svc.SetNotifier(notif)

	info, err := svc.GenerateForOrder(context.Background(), view.ID, nil)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	// Nomor tahun WIB (12 Aug 2026 UTC 03:00 = 10:00 WIB) → 2026.
	wantNumber := "INV/2026/00042"
	if info.InvoiceNumber != wantNumber {
		t.Errorf("invoice_number: want %s got %s", wantNumber, info.InvoiceNumber)
	}
	if store.createCalls != 1 {
		t.Errorf("create called %d times, want 1", store.createCalls)
	}
	if store.created == nil {
		t.Fatal("create didn't capture row")
	}
	if !strings.HasPrefix(store.created.PDFPath, "invoices/2026/") {
		t.Errorf("pdf_path missing year dir, got: %s", store.created.PDFPath)
	}
	if !strings.HasSuffix(store.created.PDFPath, ".pdf") {
		t.Errorf("pdf_path missing .pdf, got: %s", store.created.PDFPath)
	}
	if blobs.saveCalls != 1 {
		t.Errorf("blob save calls: want 1 got %d", blobs.saveCalls)
	}

	// WA notif ter-enqueue dgn kind + extras.
	if notif.calls != 1 {
		t.Errorf("WA notif: want 1 call got %d", notif.calls)
	}
	if notif.lastKind != notificationapi.KindInvoiceReady {
		t.Errorf("WA kind: want invoice_ready got %s", notif.lastKind)
	}
	if url, _ := notif.lastExtras["invoice_url"].(string); !strings.Contains(url, info.ID.String()) {
		t.Errorf("WA extras invoice_url should embed invoice id, got: %v", url)
	}
}

func TestGenerateForOrder_Idempotent_ReturnsExisting(t *testing.T) {
	existing := &model.Invoice{
		ID:            uuid.New(),
		OrderID:       uuid.New(),
		InvoiceNumber: "INV/2026/00007",
		Version:       1,
	}
	store := &fakeInvoiceStore{byOrder: existing}
	blobs := &fakeBlobs{}
	svc := newSvc(store, blobs, &fakeOrderCmd{}, &fakeCustomers{})

	info, err := svc.GenerateForOrder(context.Background(), existing.OrderID, nil)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if info.InvoiceNumber != existing.InvoiceNumber {
		t.Errorf("want existing invoice returned, got %s", info.InvoiceNumber)
	}
	if store.createCalls != 0 {
		t.Errorf("should not create — invoice sudah ada")
	}
	if blobs.saveCalls != 0 {
		t.Errorf("should not render PDF — invoice sudah ada")
	}
}

func TestGenerateForOrder_OrderNotFound(t *testing.T) {
	store := &fakeInvoiceStore{byOrderErr: invrepo.ErrNotFound}
	cmd := &fakeOrderCmd{viewErr: orderapi.ErrOrderNotFound}
	svc := newSvc(store, &fakeBlobs{}, cmd, &fakeCustomers{})

	_, err := svc.GenerateForOrder(context.Background(), uuid.New(), nil)
	if !errors.Is(err, invoiceapi.ErrOrderNotFound) {
		t.Fatalf("want ErrOrderNotFound, got %v", err)
	}
}

func TestGenerateForOrder_OrderNotInvoiceable_UnpaidOrder(t *testing.T) {
	view := validOrderView()
	view.Status = "menunggu_pembayaran" // not yet paid
	store := &fakeInvoiceStore{byOrderErr: invrepo.ErrNotFound}
	svc := newSvc(store, &fakeBlobs{}, &fakeOrderCmd{view: view},
		&fakeCustomers{identity: &authapi.Identity{Name: "X"}})

	_, err := svc.GenerateForOrder(context.Background(), view.ID, nil)
	if !errors.Is(err, invoiceapi.ErrOrderNotInvoiceable) {
		t.Fatalf("want ErrOrderNotInvoiceable, got %v", err)
	}
}

func TestGenerateForOrder_DuplicateInsert_RefetchesExisting(t *testing.T) {
	view := validOrderView()
	existing := &model.Invoice{
		ID: uuid.New(), OrderID: view.ID, InvoiceNumber: "INV/2026/00003",
	}
	// Track calls: first FindByOrderID → NotFound (race window), Create →
	// DuplicateOrder (another request beat us), second FindByOrderID →
	// return existing. Since fake shares single byOrder field, mutate it.
	store := &fakeInvoiceStore{byOrderErr: invrepo.ErrNotFound, createErr: invrepo.ErrDuplicateOrder}
	blobs := &fakeBlobs{}
	svc := newSvc(store, blobs, &fakeOrderCmd{view: view},
		&fakeCustomers{identity: &authapi.Identity{Name: "X"}})

	// Kita monkey-patch: setelah createErr terpanggil, kita ingin FindByOrderID
	// return existing. Trick: hook Delete to swap store state — tapi lebih
	// mudah: pakai custom store dgn call counter.
	// Ganti pendekatan: set byOrder + byOrderErr = nil BEFORE call, tapi
	// itu memfailkan idempotent check awal. Jadi kita gunakan bendera terpisah.
	store2 := &fakeInvoiceStoreWithRace{
		byOrderErr:         invrepo.ErrNotFound,
		createErr:          invrepo.ErrDuplicateOrder,
		byOrderAfterCreate: existing,
		nextNumberValue:    3,
	}
	svc2 := newSvc(nil, blobs, &fakeOrderCmd{view: view}, &fakeCustomers{identity: &authapi.Identity{Name: "X"}})
	svc2.invoices = store2

	info, err := svc2.GenerateForOrder(context.Background(), view.ID, nil)
	if err != nil {
		t.Fatalf("expected race-recovery to succeed, got: %v", err)
	}
	if info.InvoiceNumber != existing.InvoiceNumber {
		t.Errorf("want re-fetched existing invoice, got %s", info.InvoiceNumber)
	}
	_ = svc // silence unused
}

// fakeInvoiceStoreWithRace — simulate: create returns DuplicateOrder,
// second FindByOrderID returns the winner's row.
type fakeInvoiceStoreWithRace struct {
	byOrderErr         error
	createErr          error
	byOrderAfterCreate *model.Invoice
	nextNumberValue    int
	createCalls        int
	findByOrderCalls   int
}

func (f *fakeInvoiceStoreWithRace) Create(_ context.Context, inv *model.Invoice) error {
	f.createCalls++
	return f.createErr
}
func (f *fakeInvoiceStoreWithRace) FindByID(context.Context, uuid.UUID) (*model.Invoice, error) {
	return nil, invrepo.ErrNotFound
}
func (f *fakeInvoiceStoreWithRace) FindByOrderID(context.Context, uuid.UUID) (*model.Invoice, error) {
	f.findByOrderCalls++
	if f.findByOrderCalls == 1 {
		return nil, f.byOrderErr
	}
	return f.byOrderAfterCreate, nil
}
func (f *fakeInvoiceStoreWithRace) NextInvoiceNumber(context.Context, int) (int, error) {
	return f.nextNumberValue, nil
}

func TestGetFile_ReturnsAbsPath(t *testing.T) {
	invID := uuid.New()
	inv := &model.Invoice{ID: invID, InvoiceNumber: "INV/2026/00001", PDFPath: "invoices/2026/foo.pdf"}
	store := &fakeInvoiceStore{byID: inv}
	svc := newSvc(store, &fakeBlobs{}, &fakeOrderCmd{}, &fakeCustomers{})

	h, err := svc.GetFile(context.Background(), invID)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if !strings.HasSuffix(h.AbsPath, "invoices/2026/foo.pdf") {
		t.Errorf("AbsPath: %s", h.AbsPath)
	}
}

func TestGetFile_NotFound(t *testing.T) {
	store := &fakeInvoiceStore{byIDErr: invrepo.ErrNotFound}
	svc := newSvc(store, &fakeBlobs{}, &fakeOrderCmd{}, &fakeCustomers{})

	_, err := svc.GetFile(context.Background(), uuid.New())
	if !errors.Is(err, invoiceapi.ErrInvoiceNotFound) {
		t.Fatalf("want ErrInvoiceNotFound, got %v", err)
	}
}

// TestGetFile_OrderSoftDeleted_ReturnsNotFound — § super admin order tools
// review finding #6: an invoice's underlying order being soft-deleted must
// make the PDF stop being downloadable via this public, no-auth endpoint.
func TestGetFile_OrderSoftDeleted_ReturnsNotFound(t *testing.T) {
	invID := uuid.New()
	inv := &model.Invoice{ID: invID, OrderID: uuid.New(), InvoiceNumber: "INV/2026/00002", PDFPath: "invoices/2026/bar.pdf"}
	store := &fakeInvoiceStore{byID: inv}
	cmd := &fakeOrderCmd{summaryErr: orderapi.ErrOrderNotFound}
	svc := newSvc(store, &fakeBlobs{}, cmd, &fakeCustomers{})

	_, err := svc.GetFile(context.Background(), invID)
	if !errors.Is(err, invoiceapi.ErrInvoiceNotFound) {
		t.Fatalf("want ErrInvoiceNotFound for deleted order, got %v", err)
	}
}

func TestPDFRender_Smoke_ValidPDFHeader(t *testing.T) {
	// Ensure the renderer produces something that starts with %PDF header.
	view := validOrderView()
	bytesOut, err := pdfrender.Render(
		pdfrender.CompanyInfo{Name: "Rajaku"},
		pdfrender.Meta{InvoiceNumber: "INV/2026/00001", GeneratedAt: time.Now()},
		view, "Ali", "6281234",
	)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if !bytes.HasPrefix(bytesOut, []byte("%PDF-")) {
		t.Errorf("output doesn't look like PDF (first 8 bytes: %q)", bytesOut[:8])
	}
	if len(bytesOut) < 500 {
		t.Errorf("PDF suspiciously small: %d bytes", len(bytesOut))
	}
}
