package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/invoice/invoiceapi"
	"github.com/rajaku-printing/backend/internal/notification/notificationapi"
	"github.com/rajaku-printing/backend/internal/order/orderapi"
	"github.com/rajaku-printing/backend/internal/pos/posapi"
)

// ---------- fakes ----------

type fakeOrderCmd struct {
	createInput  orderapi.POSCreateOrderInput
	createResult *orderapi.OrderSummary
	createErr    error
	createCalls  int
	listResult   []orderapi.OrderSummary
	listErr      error
	listLastDate time.Time
}

func (f *fakeOrderCmd) FindSummaryByResi(context.Context, string) (*orderapi.OrderSummary, error) {
	return nil, nil
}
func (f *fakeOrderCmd) FindSummaryByID(context.Context, uuid.UUID) (*orderapi.OrderSummary, error) {
	return nil, nil
}
func (f *fakeOrderCmd) FindInvoiceViewByID(context.Context, uuid.UUID) (*orderapi.OrderInvoiceView, error) {
	return nil, nil
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
func (f *fakeOrderCmd) CreatePOSOrder(_ context.Context, in orderapi.POSCreateOrderInput) (*orderapi.OrderSummary, error) {
	f.createCalls++
	f.createInput = in
	return f.createResult, f.createErr
}
func (f *fakeOrderCmd) ListPOSOrdersByDate(_ context.Context, d time.Time) ([]orderapi.OrderSummary, error) {
	f.listLastDate = d
	return f.listResult, f.listErr
}

type fakeCustomers struct {
	identity  *authapi.Identity
	err       error
	lastPhone string
	lastName  string
}

func (f *fakeCustomers) ResolveOrCreateGuest(_ context.Context, ph, name string) (*authapi.Identity, error) {
	f.lastPhone = ph
	f.lastName = name
	if f.err != nil {
		return nil, f.err
	}
	return f.identity, nil
}
func (f *fakeCustomers) FindByID(context.Context, uuid.UUID) (*authapi.Identity, error) {
	return f.identity, f.err
}

type fakeNotifier struct {
	calls    int
	lastKind notificationapi.Kind
}

func (f *fakeNotifier) EnqueueOrderEvent(_ context.Context, kind notificationapi.Kind, _ uuid.UUID, _ map[string]any) error {
	f.calls++
	f.lastKind = kind
	return nil
}

type fakeInvoiceGen struct {
	calls int
	info  *invoiceapi.Info
	err   error
}

func (f *fakeInvoiceGen) GenerateForOrder(_ context.Context, _ uuid.UUID, _ *uuid.UUID) (*invoiceapi.Info, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.info, nil
}

// ---------- helpers ----------

func newSvc(cmd *fakeOrderCmd, cs *fakeCustomers, notif *fakeNotifier, inv *fakeInvoiceGen) *Service {
	s := New(cmd, cs, Config{BaseURL: "https://rajaku.test"})
	if notif != nil {
		s.SetNotifier(notif)
	}
	if inv != nil {
		s.SetInvoiceGenerator(inv)
	}
	return s
}

func validInput(kasir uuid.UUID) CreateOrderInput {
	return CreateOrderInput{
		KasirID:       kasir,
		CustomerName:  "Budi",
		CustomerPhone: "081234567890", // will be normalized to 6281234567890
		ProductID:     uuid.New(),
		MaterialID:    uuid.New(),
		WidthCm:       100,
		HeightCm:      200,
		Quantity:      1,
		MetodeAmbil:   "pickup",
		MetodeBayar:   "cash",
		DesignSource:  "upload",
	}
}

// ---------- tests ----------

func TestCreateOrder_HappyPath_ResolvesCustomer_CreatesOrder_TriggersNotifAndInvoice(t *testing.T) {
	kasir := uuid.New()
	custID := uuid.New()
	orderID := uuid.New()
	invID := uuid.New()

	cmd := &fakeOrderCmd{createResult: &orderapi.OrderSummary{
		ID: orderID, Resi: "RJK-POS1", CustomerID: custID,
		Total: 100000, MetodeBayar: "cash", MetodeAmbil: "pickup",
		CreatedAt: time.Now(),
	}}
	custs := &fakeCustomers{identity: &authapi.Identity{UserID: custID, Name: "Budi", Phone: "6281234567890"}}
	notif := &fakeNotifier{}
	inv := &fakeInvoiceGen{info: &invoiceapi.Info{
		ID: invID, InvoiceNumber: "INV/2026/00001",
		DownloadURL: "https://rajaku.test/api/v1/invoices/" + invID.String() + "/pdf",
	}}
	svc := newSvc(cmd, custs, notif, inv)

	result, err := svc.CreateOrder(context.Background(), validInput(kasir))
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}

	// Customer resolved with normalized phone.
	if custs.lastPhone != "6281234567890" {
		t.Errorf("expected normalized phone in customer lookup, got %s", custs.lastPhone)
	}
	// Order created with kasir + resolved customer.
	if cmd.createCalls != 1 {
		t.Errorf("CreatePOSOrder calls: want 1 got %d", cmd.createCalls)
	}
	if cmd.createInput.KasirID != kasir {
		t.Errorf("kasir_id not passed through")
	}
	if cmd.createInput.CustomerID != custID {
		t.Errorf("resolved customer_id not passed through")
	}
	// Result populated.
	if result.Resi != "RJK-POS1" {
		t.Errorf("resi: want RJK-POS1 got %s", result.Resi)
	}
	if !strings.Contains(result.TrackingURL, "RJK-POS1") {
		t.Errorf("tracking URL missing resi: %s", result.TrackingURL)
	}
	if result.InvoiceURL == "" || result.InvoiceNumber != "INV/2026/00001" {
		t.Errorf("invoice fields not populated in result: %+v", result)
	}
	// Notif enqueued.
	if notif.calls != 1 || notif.lastKind != notificationapi.KindPOSOrderCreated {
		t.Errorf("expected 1 pos_order_created notif, got %d calls kind=%s", notif.calls, notif.lastKind)
	}
	// Invoice triggered.
	if inv.calls != 1 {
		t.Errorf("expected invoice generator called once, got %d", inv.calls)
	}
}

func TestCreateOrder_InvalidPhone_Rejected(t *testing.T) {
	svc := newSvc(&fakeOrderCmd{}, &fakeCustomers{}, nil, nil)
	in := validInput(uuid.New())
	in.CustomerPhone = "not-a-phone"
	_, err := svc.CreateOrder(context.Background(), in)
	if !errors.Is(err, posapi.ErrInvalidPhone) {
		t.Fatalf("want ErrInvalidPhone, got %v", err)
	}
}

func TestCreateOrder_MissingName_Rejected(t *testing.T) {
	svc := newSvc(&fakeOrderCmd{}, &fakeCustomers{}, nil, nil)
	in := validInput(uuid.New())
	in.CustomerName = "   "
	_, err := svc.CreateOrder(context.Background(), in)
	if err == nil || !strings.Contains(err.Error(), "nama pelanggan") {
		t.Fatalf("want nama-pelanggan-wajib error, got %v", err)
	}
}

func TestCreateOrder_CustomerResolveFails_WrappedAsPOSErr(t *testing.T) {
	custs := &fakeCustomers{err: errors.New("db down")}
	svc := newSvc(&fakeOrderCmd{}, custs, nil, nil)
	_, err := svc.CreateOrder(context.Background(), validInput(uuid.New()))
	if !errors.Is(err, posapi.ErrCustomerResolve) {
		t.Fatalf("want ErrCustomerResolve, got %v", err)
	}
}

func TestCreateOrder_InvoiceGenFails_OrderStillSucceeds(t *testing.T) {
	kasir := uuid.New()
	custID := uuid.New()
	cmd := &fakeOrderCmd{createResult: &orderapi.OrderSummary{
		ID: uuid.New(), Resi: "RJK-POS2", CustomerID: custID, Total: 50000,
		MetodeBayar: "qris_pos", MetodeAmbil: "pickup",
	}}
	custs := &fakeCustomers{identity: &authapi.Identity{UserID: custID, Name: "X", Phone: "6281234"}}
	inv := &fakeInvoiceGen{err: errors.New("pdf disk full")}
	svc := newSvc(cmd, custs, &fakeNotifier{}, inv)

	result, err := svc.CreateOrder(context.Background(), validInput(kasir))
	if err != nil {
		t.Fatalf("order should succeed even if invoice fails: %v", err)
	}
	// Invoice URL should be empty (best-effort skip).
	if result.InvoiceURL != "" {
		t.Errorf("invoice URL should be empty on gen failure, got %q", result.InvoiceURL)
	}
}

func TestDailyReconciliation_AggregatesByMetodeAndKasir(t *testing.T) {
	kasirA := uuid.New()
	kasirB := uuid.New()
	cmd := &fakeOrderCmd{listResult: []orderapi.OrderSummary{
		{ID: uuid.New(), Total: 100000, MetodeBayar: "cash", CreatedBy: &kasirA},
		{ID: uuid.New(), Total: 50000, MetodeBayar: "cash", CreatedBy: &kasirA},
		{ID: uuid.New(), Total: 75000, MetodeBayar: "qris_pos", CreatedBy: &kasirB},
		{ID: uuid.New(), Total: 25000, MetodeBayar: "qris_pos", CreatedBy: &kasirA},
	}}
	svc := newSvc(cmd, &fakeCustomers{}, nil, nil)

	report, err := svc.DailyReconciliation(context.Background(), time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if report.TotalOrders != 4 {
		t.Errorf("total_orders: want 4 got %d", report.TotalOrders)
	}
	if report.TotalRevenue != 250000 {
		t.Errorf("total_revenue: want 250000 got %d", report.TotalRevenue)
	}
	if got := report.ByMetodeBayar["cash"]; got.Count != 2 || got.Revenue != 150000 {
		t.Errorf("cash stats wrong: %+v", got)
	}
	if got := report.ByMetodeBayar["qris_pos"]; got.Count != 2 || got.Revenue != 100000 {
		t.Errorf("qris_pos stats wrong: %+v", got)
	}
	if len(report.ByKasir) != 2 {
		t.Fatalf("expected 2 kasir rows, got %d", len(report.ByKasir))
	}
	// kasirA: 3 orders (100k+50k+25k=175k); kasirB: 1 (75k). Order-independent check.
	for _, k := range report.ByKasir {
		switch k.KasirID {
		case kasirA:
			if k.Count != 3 || k.Revenue != 175000 {
				t.Errorf("kasirA stats wrong: %+v", k)
			}
		case kasirB:
			if k.Count != 1 || k.Revenue != 75000 {
				t.Errorf("kasirB stats wrong: %+v", k)
			}
		default:
			t.Errorf("unexpected kasir id: %s", k.KasirID)
		}
	}
}
