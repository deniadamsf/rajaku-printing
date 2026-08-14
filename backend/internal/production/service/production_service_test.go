package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/notification/notificationapi"
	"github.com/rajaku-printing/backend/internal/order/orderapi"
	"github.com/rajaku-printing/backend/internal/production/productionapi"
)

// ---------- fakes ----------

type fakeOrderCmd struct {
	summary                 *orderapi.OrderSummary
	summaryErr              error
	cetakCalls              int
	cetakErr                error
	qcCalls                 int
	siapCalls               int
	dikirimCalls            int
	dikirimLastCourier      string
	dikirimLastTracking     string
	dikirimErr              error
	selesaiCalls            int
}

func (f *fakeOrderCmd) FindSummaryByResi(context.Context, string) (*orderapi.OrderSummary, error) {
	return f.summary, f.summaryErr
}
func (f *fakeOrderCmd) FindSummaryByID(context.Context, uuid.UUID) (*orderapi.OrderSummary, error) {
	return f.summary, f.summaryErr
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
func (f *fakeOrderCmd) MarkProsesCetak(_ context.Context, _ uuid.UUID, _ *uuid.UUID, _ string) error {
	f.cetakCalls++
	return f.cetakErr
}
func (f *fakeOrderCmd) MarkQC(_ context.Context, _ uuid.UUID, _ *uuid.UUID, _ string) error {
	f.qcCalls++
	return nil
}
func (f *fakeOrderCmd) MarkSiapKirimAtauAmbil(_ context.Context, _ uuid.UUID, _ *uuid.UUID, _ string) error {
	f.siapCalls++
	return nil
}
func (f *fakeOrderCmd) MarkDikirim(_ context.Context, _ uuid.UUID, _ *uuid.UUID, courier, tracking, _ string) error {
	f.dikirimCalls++
	f.dikirimLastCourier = courier
	f.dikirimLastTracking = tracking
	return f.dikirimErr
}
func (f *fakeOrderCmd) MarkSelesai(_ context.Context, _ uuid.UUID, _ *uuid.UUID, _ string) error {
	f.selesaiCalls++
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

type fakeNotifier struct {
	calls     int
	lastKind  notificationapi.Kind
	lastOrder uuid.UUID
	lastExtras map[string]any
}

func (f *fakeNotifier) EnqueueOrderEvent(_ context.Context, kind notificationapi.Kind, orderID uuid.UUID, extras map[string]any) error {
	f.calls++
	f.lastKind = kind
	f.lastOrder = orderID
	f.lastExtras = extras
	return nil
}

// ---------- tests ----------

func newSvc(cmd *fakeOrderCmd, notif *fakeNotifier) *Service {
	s := New(cmd)
	if notif != nil {
		s.SetNotifier(notif)
	}
	return s
}

func TestStartCetak_HappyPath(t *testing.T) {
	orderID := uuid.New()
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{ID: orderID, Resi: "RJK-A", Status: "desain_diverifikasi"}}
	notif := &fakeNotifier{}
	svc := newSvc(cmd, notif)

	if err := svc.StartCetak(context.Background(), AdvanceInput{
		Resi: "RJK-A", StaffID: uuid.New(),
	}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if cmd.cetakCalls != 1 {
		t.Errorf("MarkProsesCetak calls: want 1 got %d", cmd.cetakCalls)
	}
	if notif.calls != 0 {
		t.Errorf("StartCetak should not send WA (internal step), got %d notifs", notif.calls)
	}
}

func TestStartCetak_OrderNotFound(t *testing.T) {
	cmd := &fakeOrderCmd{summaryErr: orderapi.ErrOrderNotFound}
	svc := newSvc(cmd, nil)
	err := svc.StartCetak(context.Background(), AdvanceInput{Resi: "RJK-X", StaffID: uuid.New()})
	if !errors.Is(err, productionapi.ErrOrderNotFound) {
		t.Fatalf("want ErrOrderNotFound, got %v", err)
	}
}

func TestStartCetak_InvalidTransition(t *testing.T) {
	// Simulate order.commandTransition returning generic error (state-machine reject).
	cmd := &fakeOrderCmd{
		summary:  &orderapi.OrderSummary{ID: uuid.New(), Resi: "RJK-Q"},
		cetakErr: errors.New("boom: invalid transition from menunggu_pembayaran to proses_cetak"),
	}
	svc := newSvc(cmd, nil)
	err := svc.StartCetak(context.Background(), AdvanceInput{Resi: "RJK-Q", StaffID: uuid.New()})
	if !errors.Is(err, productionapi.ErrInvalidTransition) {
		t.Fatalf("want ErrInvalidTransition, got %v", err)
	}
}

func TestMarkSiap_KirimPath_TriggersReadyShip(t *testing.T) {
	orderID := uuid.New()
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: orderID, MetodeAmbil: "kirim",
	}}
	notif := &fakeNotifier{}
	svc := newSvc(cmd, notif)

	if err := svc.MarkSiap(context.Background(), AdvanceInput{Resi: "RJK-K", StaffID: uuid.New()}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if cmd.siapCalls != 1 {
		t.Errorf("expected siap-kirim/ambil advance called")
	}
	if notif.lastKind != notificationapi.KindReadyShip {
		t.Errorf("want KindReadyShip for kirim path, got %s", notif.lastKind)
	}
	if notif.lastOrder != orderID {
		t.Errorf("notif for wrong order id")
	}
}

func TestMarkSiap_PickupPath_TriggersReadyPickup(t *testing.T) {
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: uuid.New(), MetodeAmbil: "pickup",
	}}
	notif := &fakeNotifier{}
	svc := newSvc(cmd, notif)

	if err := svc.MarkSiap(context.Background(), AdvanceInput{Resi: "RJK-P", StaffID: uuid.New()}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if notif.lastKind != notificationapi.KindReadyPickup {
		t.Errorf("want KindReadyPickup for pickup path, got %s", notif.lastKind)
	}
}

func TestMarkShipped_HappyPath_RecordsCourierTracking(t *testing.T) {
	orderID := uuid.New()
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{ID: orderID, MetodeAmbil: "kirim"}}
	notif := &fakeNotifier{}
	svc := newSvc(cmd, notif)

	err := svc.MarkShipped(context.Background(), MarkShippedInput{
		Resi: "RJK-S", StaffID: uuid.New(),
		Courier: "JNE", TrackingNumber: "1234567890",
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if cmd.dikirimCalls != 1 {
		t.Fatalf("want MarkDikirim called once, got %d", cmd.dikirimCalls)
	}
	if cmd.dikirimLastCourier != "JNE" || cmd.dikirimLastTracking != "1234567890" {
		t.Errorf("courier/tracking not passed through: got %q/%q", cmd.dikirimLastCourier, cmd.dikirimLastTracking)
	}
	if notif.lastKind != notificationapi.KindShipped {
		t.Errorf("want KindShipped, got %s", notif.lastKind)
	}
	if got := notif.lastExtras["courier"]; got != "JNE" {
		t.Errorf("notif extras courier want JNE got %v", got)
	}
	if got := notif.lastExtras["tracking"]; got != "1234567890" {
		t.Errorf("notif extras tracking want 1234567890 got %v", got)
	}
}

func TestMarkShipped_PickupRejected(t *testing.T) {
	// orderCmd.MarkDikirim will return ErrDikirimOnPickup (source of truth).
	cmd := &fakeOrderCmd{
		summary:    &orderapi.OrderSummary{ID: uuid.New(), MetodeAmbil: "pickup"},
		dikirimErr: orderapi.ErrDikirimOnPickup,
	}
	svc := newSvc(cmd, &fakeNotifier{})
	err := svc.MarkShipped(context.Background(), MarkShippedInput{
		Resi: "RJK-X", StaffID: uuid.New(),
		Courier: "JNE", TrackingNumber: "1",
	})
	if !errors.Is(err, productionapi.ErrDikirimOnPickup) {
		t.Fatalf("want ErrDikirimOnPickup, got %v", err)
	}
}

func TestMarkShipped_MissingCourier_Rejected(t *testing.T) {
	cmd := &fakeOrderCmd{
		summary:    &orderapi.OrderSummary{ID: uuid.New(), MetodeAmbil: "kirim"},
		dikirimErr: orderapi.ErrShippingTrackingRequired,
	}
	svc := newSvc(cmd, &fakeNotifier{})
	err := svc.MarkShipped(context.Background(), MarkShippedInput{
		Resi: "RJK-X", StaffID: uuid.New(),
		Courier: "", TrackingNumber: "",
	})
	if !errors.Is(err, productionapi.ErrShippingTrackingRequired) {
		t.Fatalf("want ErrShippingTrackingRequired, got %v", err)
	}
}

func TestMarkSelesai_HappyPath(t *testing.T) {
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{ID: uuid.New(), MetodeAmbil: "kirim"}}
	svc := newSvc(cmd, &fakeNotifier{})
	if err := svc.MarkSelesai(context.Background(), AdvanceInput{Resi: "RJK-S", StaffID: uuid.New()}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if cmd.selesaiCalls != 1 {
		t.Fatalf("want MarkSelesai called once, got %d", cmd.selesaiCalls)
	}
}
