package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/discount/discountapi"
	"github.com/rajaku-printing/backend/internal/invoice/invoiceapi"
	"github.com/rajaku-printing/backend/internal/notification/notificationapi"
	"github.com/rajaku-printing/backend/internal/order/orderapi"
	"github.com/rajaku-printing/backend/internal/pos/posapi"
	"github.com/rajaku-printing/backend/internal/settings/settingsapi"
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
func (f *fakeCustomers) SearchCustomers(context.Context, string, int) ([]authapi.Identity, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.identity == nil {
		return []authapi.Identity{}, nil
	}
	return []authapi.Identity{*f.identity}, nil
}

// GetMembershipInfo/UpdateMembershipStatus/ListMembers (§30) — dummy impls
// agar fake satisfy authapi.CustomerService. POS tests tidak butuh perilaku ini.
func (f *fakeCustomers) GetMembershipInfo(context.Context, uuid.UUID) (*authapi.MembershipInfo, error) {
	return nil, f.err
}
func (f *fakeCustomers) UpdateMembershipStatus(context.Context, authapi.UpdateMembershipStatusInput) error {
	return f.err
}
func (f *fakeCustomers) ListMembers(context.Context, authapi.ListMembershipFilter) (*authapi.ListMembershipResult, error) {
	return nil, f.err
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

type fakeSettings struct {
	width int
	err   error
	calls int
}

func (f *fakeSettings) GetInt(_ context.Context, _ string) (int, error) {
	f.calls++
	if f.err != nil {
		return 0, f.err
	}
	return f.width, nil
}

// GetBool — dummy impl agar fake satisfy settingsapi.Reader (§30). POS tests
// tidak butuh perilaku ini.
func (f *fakeSettings) GetBool(_ context.Context, _ string) (bool, error) {
	return false, f.err
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
		Items: []CreateOrderItemInput{{
			ProductID:    uuid.New(),
			MaterialID:   uuid.New(),
			WidthCm:      100,
			HeightCm:     200,
			Quantity:     1,
			DesignSource: "upload",
		}},
		MetodeAmbil: "pickup",
		MetodeBayar: "cash",
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

// TestCreateOrder_LineItemDetailsPopulated menutup gap struk POS yang cuma
// memuat resi/tanggal/metode/total (tugas A) — memastikan nama pelanggan &
// rincian item snapshot ikut sampai ke CreateOrderResult, diambil dari
// OrderSummary yang dikembalikan orderCmd, BUKAN dihitung ulang di sini.
func TestCreateOrder_LineItemDetailsPopulated(t *testing.T) {
	custID := uuid.New()
	shipping := int64(15000)
	cmd := &fakeOrderCmd{createResult: &orderapi.OrderSummary{
		ID: uuid.New(), Resi: "RJK-POS7", CustomerID: custID,
		Total: 215000, MetodeBayar: "cash", MetodeAmbil: "kirim",
		Items: []orderapi.OrderItemView{{
			LineNo:       1,
			ProductName:  "Banner Vinyl",
			MaterialName: "Flexi Korea",
			WidthCm:      100,
			HeightCm:     200,
			Quantity:     2,
			UnitPrice:    100000,
			Subtotal:     200000,
		}},
		Subtotal:     200000,
		ShippingCost: &shipping,
	}}
	custs := &fakeCustomers{identity: &authapi.Identity{UserID: custID, Name: "Budi Santoso", Phone: "6281234567890"}}
	svc := newSvc(cmd, custs, nil, nil)

	result, err := svc.CreateOrder(context.Background(), validInput(uuid.New()))
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}

	// validInput mengetik "Budi", sementara customer yang cocok di DB bernama
	// "Budi Santoso". Yang tercetak di struk WAJIB nama yang diketik kasir —
	// lihat TestCreateOrder_ReceiptUsesKasirTypedName di bawah.
	if result.CustomerName != "Budi" {
		t.Errorf("customer_name: want %q got %q", "Budi", result.CustomerName)
	}
	if result.CustomerPhone != "6281234567890" {
		t.Errorf("customer_phone: want %q got %q", "6281234567890", result.CustomerPhone)
	}
	if len(result.Items) != 1 {
		t.Fatalf("want 1 receipt item, got %d", len(result.Items))
	}
	item := result.Items[0]
	if item.ProductName != "Banner Vinyl" {
		t.Errorf("product_name: want Banner Vinyl got %q", item.ProductName)
	}
	if item.MaterialName != "Flexi Korea" {
		t.Errorf("material_name: want Flexi Korea got %q", item.MaterialName)
	}
	if item.WidthCm != 100 || item.HeightCm != 200 {
		t.Errorf("width/height: want 100x200 got %dx%d", item.WidthCm, item.HeightCm)
	}
	if item.Quantity != 2 {
		t.Errorf("quantity: want 2 got %d", item.Quantity)
	}
	if item.UnitPrice != 100000 {
		t.Errorf("unit_price: want 100000 got %d", item.UnitPrice)
	}
	if result.Subtotal != 200000 {
		t.Errorf("subtotal: want 200000 got %d", result.Subtotal)
	}
	if result.ShippingCost == nil || *result.ShippingCost != 15000 {
		t.Errorf("shipping_cost: want 15000 got %v", result.ShippingCost)
	}
}

// TestCreateOrder_MultiItem_TwoDifferentSizes_ProducesTwoReceiptLines
// (Tugas A) — order POS dengan 2 baris produk beda ukuran wajib meneruskan
// SELURUH item ke orderapi.POSCreateOrderInput (bukan cuma Items[0]), dan
// struk (result.Items) wajib memuat 2 baris dengan totalnya benar. Baris
// diskon TETAP satu agregat di level order (§32.7) — bukan per item.
func TestCreateOrder_MultiItem_TwoDifferentSizes_ProducesTwoReceiptLines(t *testing.T) {
	kasir := uuid.New()
	custID := uuid.New()
	discountAmt := int64(20000)
	cmd := &fakeOrderCmd{createResult: &orderapi.OrderSummary{
		ID: uuid.New(), Resi: "RJK-POS11", CustomerID: custID,
		Total: 280000, MetodeBayar: "cash", MetodeAmbil: "pickup",
		Items: []orderapi.OrderItemView{
			{
				LineNo: 1, ProductName: "Banner Vinyl", MaterialName: "Flexi Korea",
				WidthCm: 100, HeightCm: 200, Quantity: 1, UnitPrice: 200000, Subtotal: 200000,
			},
			{
				LineNo: 2, ProductName: "Banner Vinyl", MaterialName: "Flexi China",
				WidthCm: 300, HeightCm: 100, Quantity: 1, UnitPrice: 100000, Subtotal: 100000,
			},
		},
		Subtotal:       300000,
		DiscountAmount: discountAmt,
		DiscountLabel:  "Promo Lebaran",
	}}
	custs := &fakeCustomers{identity: &authapi.Identity{UserID: custID, Name: "Budi", Phone: "6281234567890"}}
	svc := newSvc(cmd, custs, nil, nil)

	in := validInput(kasir)
	in.Items = []CreateOrderItemInput{
		{ProductID: uuid.New(), MaterialID: uuid.New(), WidthCm: 100, HeightCm: 200, Quantity: 1, DesignSource: "upload"},
		{ProductID: uuid.New(), MaterialID: uuid.New(), WidthCm: 300, HeightCm: 100, Quantity: 1, DesignSource: "request", DesignBrief: "logo toko di tengah"},
	}

	result, err := svc.CreateOrder(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}

	// Kedua item dari input harus diteruskan utuh ke orderapi — bukan cuma
	// Items[0] (regresi yang persis ditinggalkan tahap 1 sebagai TODO).
	if len(cmd.createInput.Items) != 2 {
		t.Fatalf("orderapi.POSCreateOrderInput.Items: want 2 got %d", len(cmd.createInput.Items))
	}
	if cmd.createInput.Items[0].WidthCm != 100 || cmd.createInput.Items[1].WidthCm != 300 {
		t.Errorf("item order/width tidak sesuai input: %+v", cmd.createInput.Items)
	}
	if cmd.createInput.Items[1].DesignSource != "request" || cmd.createInput.Items[1].DesignBrief != "logo toko di tengah" {
		t.Errorf("item[1] design_source/brief tidak diteruskan: %+v", cmd.createInput.Items[1])
	}

	// Struk memuat 2 baris item.
	if len(result.Items) != 2 {
		t.Fatalf("result.Items: want 2 baris struk got %d", len(result.Items))
	}
	if result.Items[0].Subtotal != 200000 || result.Items[1].Subtotal != 100000 {
		t.Errorf("subtotal per baris salah: %+v", result.Items)
	}

	// Total tetap dibaca dari kolom agregat orders (§32.2), bukan dihitung
	// ulang dari Items di sini.
	if result.Subtotal != 300000 {
		t.Errorf("subtotal agregat: want 300000 got %d", result.Subtotal)
	}
	if result.DiscountAmount != 20000 || result.DiscountLabel != "Promo Lebaran" {
		t.Errorf("baris diskon agregat salah: amount=%d label=%q", result.DiscountAmount, result.DiscountLabel)
	}
	if result.Total != 280000 {
		t.Errorf("total: want 280000 got %d", result.Total)
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

// TestCreateOrder_DiscountSentinel_PropagatesViaErrorsIs mengunci temuan
// review "sentinel diskon ketelan jadi HTTP 500" — order/service.CreatePOSOrder
// mengembalikan sentinel diskon MENTAH (tidak dibungkus, lihat
// order/service/order_service.go resolveDiscount/CreatePOSOrder), dan
// pos/service.CreateOrder wajib meneruskannya sedemikian rupa sehingga
// errors.Is(hasil, sentinelAsli) TETAP true di level pemanggil manapun
// (handler). Sebelum diperbaiki, baris fallback di CreateOrder membungkus
// error asli dengan `%v` (bukan `%w`), memutus rantai ini untuk SETIAP
// sentinel yang tidak eksplisit ada di daftar bubble-up (§28.9 lama maupun
// §30.3 baru seperti ErrDiscountMembershipRequired) — errors.Is jadi selalu
// false dan handler jatuh ke case generic (500).
func TestCreateOrder_DiscountSentinel_PropagatesViaErrorsIs(t *testing.T) {
	sentinels := []error{
		discountapi.ErrDiscountMembershipRequired, // §30.3, baru — TIDAK ada di daftar bubble-up eksplisit pos/service
		discountapi.ErrDiscountMembershipDisabled, // §30.3, baru — idem
		discountapi.ErrDiscountMemberMismatch,     // §30.3, baru — idem
		discountapi.ErrDiscountMemberScopeEmpty,   // §30.3, baru — idem
		discountapi.ErrDiscountScopeEmpty,         // §28.9, lama — idem
		discountapi.ErrDiscountProductMismatch,    // §28.9, lama — idem
	}

	for _, sentinel := range sentinels {
		sentinel := sentinel
		t.Run(sentinel.Error(), func(t *testing.T) {
			cmd := &fakeOrderCmd{createErr: sentinel}
			custs := &fakeCustomers{identity: &authapi.Identity{
				UserID: uuid.New(), Name: "Budi", Phone: "6281234567890",
			}}
			svc := newSvc(cmd, custs, nil, nil)

			_, err := svc.CreateOrder(context.Background(), validInput(uuid.New()))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			// Rantai ke sentinel ASLI wajib tetap tertembus — ini yang patah
			// sebelum fix (%v).
			if !errors.Is(err, sentinel) {
				t.Errorf("errors.Is(err, %v) = false, want true (err: %v)", sentinel, err)
			}
			// Rantai ke posapi.ErrOrderCreate (fallback marker) juga wajib
			// tetap tertembus — ini bagian dari kontrak "%w dua kali", bukan
			// cuma salah satunya.
			if !errors.Is(err, posapi.ErrOrderCreate) {
				t.Errorf("errors.Is(err, posapi.ErrOrderCreate) = false, want true (err: %v)", err)
			}
		})
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

func TestCreateOrder_ReceiptWidthMM_ReadFromSettings(t *testing.T) {
	custID := uuid.New()
	cmd := &fakeOrderCmd{createResult: &orderapi.OrderSummary{
		ID: uuid.New(), Resi: "RJK-POS3", CustomerID: custID, Total: 10000,
		MetodeBayar: "cash", MetodeAmbil: "pickup",
	}}
	custs := &fakeCustomers{identity: &authapi.Identity{UserID: custID, Name: "Budi", Phone: "6281234567890"}}
	svc := newSvc(cmd, custs, nil, nil)
	svc.SetSettingsReader(&fakeSettings{width: 80})

	result, err := svc.CreateOrder(context.Background(), validInput(uuid.New()))
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if result.ReceiptWidthMM != 80 {
		t.Errorf("receipt_width_mm: want 80 got %d", result.ReceiptWidthMM)
	}
}

func TestCreateOrder_ReceiptWidthMM_FallsBackWhenSettingsUnavailable(t *testing.T) {
	custID := uuid.New()
	makeCmd := func() *fakeOrderCmd {
		return &fakeOrderCmd{createResult: &orderapi.OrderSummary{
			ID: uuid.New(), Resi: "RJK-POS4", CustomerID: custID, Total: 10000,
			MetodeBayar: "cash", MetodeAmbil: "pickup",
		}}
	}
	custs := func() *fakeCustomers {
		return &fakeCustomers{identity: &authapi.Identity{UserID: custID, Name: "Budi", Phone: "6281234567890"}}
	}

	// Case 1: reader nil (belum di-wire).
	svcNoReader := newSvc(makeCmd(), custs(), nil, nil)
	result, err := svcNoReader.CreateOrder(context.Background(), validInput(uuid.New()))
	if err != nil {
		t.Fatalf("order harus tetap sukses walau settings reader nil: %v", err)
	}
	if result.ReceiptWidthMM != defaultReceiptWidthMM {
		t.Errorf("receipt_width_mm: want fallback %d got %d", defaultReceiptWidthMM, result.ReceiptWidthMM)
	}

	// Case 2: reader error ErrSettingNotFound.
	svcErrReader := newSvc(makeCmd(), custs(), nil, nil)
	svcErrReader.SetSettingsReader(&fakeSettings{err: settingsapi.ErrSettingNotFound})
	result2, err := svcErrReader.CreateOrder(context.Background(), validInput(uuid.New()))
	if err != nil {
		t.Fatalf("order harus tetap sukses walau setting hilang: %v", err)
	}
	if result2.ReceiptWidthMM != defaultReceiptWidthMM {
		t.Errorf("receipt_width_mm: want fallback %d got %d", defaultReceiptWidthMM, result2.ReceiptWidthMM)
	}
}

// TestCreateOrder_ReceiptWidthMM_ReadsDefaultWidthFromDB mengunci bahwa nilai
// 58 yang dikembalikan berasal dari DB, BUKAN dari jalur fallback. Tanpa ini,
// bug "selalu fallback" tidak terdeteksi: keduanya menghasilkan angka sama.
func TestCreateOrder_ReceiptWidthMM_ReadsDefaultWidthFromDB(t *testing.T) {
	custID := uuid.New()
	cmd := &fakeOrderCmd{createResult: &orderapi.OrderSummary{
		ID: uuid.New(), Resi: "RJK-POS5", CustomerID: custID, Total: 10000,
		MetodeBayar: "cash", MetodeAmbil: "pickup",
	}}
	custs := &fakeCustomers{identity: &authapi.Identity{UserID: custID, Name: "Budi", Phone: "6281234567890"}}
	settings := &fakeSettings{width: 58}
	svc := newSvc(cmd, custs, nil, nil)
	svc.SetSettingsReader(settings)

	result, err := svc.CreateOrder(context.Background(), validInput(uuid.New()))
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if result.ReceiptWidthMM != 58 {
		t.Errorf("receipt_width_mm: want 58 got %d", result.ReceiptWidthMM)
	}
	if settings.calls != 1 {
		t.Errorf("settings reader harus dipanggil tepat sekali, dapat %d kali", settings.calls)
	}
}

// TestCreateOrder_ReceiptWidthMM_FallsBackWhenValueOutsideAllowedList menutup
// cabang kegagalan senyap: row DB diedit manual, ATAU daftar lebar di
// settingsapi diperluas tanpa menyesuaikan seluruh rantai. Nilai di luar
// daftar harus jatuh ke fallback, dan order tetap sukses.
func TestCreateOrder_ReceiptWidthMM_FallsBackWhenValueOutsideAllowedList(t *testing.T) {
	custID := uuid.New()
	cmd := &fakeOrderCmd{createResult: &orderapi.OrderSummary{
		ID: uuid.New(), Resi: "RJK-POS6", CustomerID: custID, Total: 10000,
		MetodeBayar: "cash", MetodeAmbil: "pickup",
	}}
	custs := &fakeCustomers{identity: &authapi.Identity{UserID: custID, Name: "Budi", Phone: "6281234567890"}}
	svc := newSvc(cmd, custs, nil, nil)
	svc.SetSettingsReader(&fakeSettings{width: 70})

	result, err := svc.CreateOrder(context.Background(), validInput(uuid.New()))
	if err != nil {
		t.Fatalf("order harus tetap sukses walau nilai setting di luar daftar: %v", err)
	}
	if result.ReceiptWidthMM != defaultReceiptWidthMM {
		t.Errorf("receipt_width_mm: want fallback %d got %d", defaultReceiptWidthMM, result.ReceiptWidthMM)
	}
}

// ---------- ReceiptConfig (tugas B) ----------

func TestReceiptConfig_ReadsActiveWidthAndListsAllowed(t *testing.T) {
	svc := newSvc(&fakeOrderCmd{}, &fakeCustomers{}, nil, nil)
	svc.SetSettingsReader(&fakeSettings{width: 80})

	cfg := svc.ReceiptConfig(context.Background())
	if cfg.WidthMM != 80 {
		t.Errorf("width_mm: want 80 got %d", cfg.WidthMM)
	}
	found58, found80 := false, false
	for _, w := range cfg.AllowedWidthsMM {
		if w == 58 {
			found58 = true
		}
		if w == 80 {
			found80 = true
		}
	}
	if !found58 || !found80 {
		t.Errorf("allowed_widths_mm harus memuat 58 & 80, got %v", cfg.AllowedWidthsMM)
	}
}

// TestReceiptConfig_FallsBackWhenReaderUnavailable — config display, bukan
// data kritis: reader nil/error harus tetap menghasilkan width_mm default,
// TIDAK boleh membuat endpoint gagal (§ tugas B).
func TestReceiptConfig_FallsBackWhenReaderUnavailable(t *testing.T) {
	svcNoReader := newSvc(&fakeOrderCmd{}, &fakeCustomers{}, nil, nil)
	cfg := svcNoReader.ReceiptConfig(context.Background())
	if cfg.WidthMM != defaultReceiptWidthMM {
		t.Errorf("width_mm: want fallback %d got %d", defaultReceiptWidthMM, cfg.WidthMM)
	}

	svcErrReader := newSvc(&fakeOrderCmd{}, &fakeCustomers{}, nil, nil)
	svcErrReader.SetSettingsReader(&fakeSettings{err: settingsapi.ErrSettingNotFound})
	cfg2 := svcErrReader.ReceiptConfig(context.Background())
	if cfg2.WidthMM != defaultReceiptWidthMM {
		t.Errorf("width_mm: want fallback %d got %d", defaultReceiptWidthMM, cfg2.WidthMM)
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

// TestCreateOrder_ReceiptUsesKasirTypedName mengunci keputusan yang tidak
// terlihat dari tanda tangan fungsi: ResolveOrCreateGuest mengembalikan nama
// LAMA di DB kalau nomor WA sudah terdaftar, dan nama itu TIDAK boleh dipakai
// di struk.
//
// Skenario nyata: satu nomor WA rumah/toko dipakai bergantian (§11 — nomor WA
// adalah matching key, bukan identitas orang). Kalau struk memakai nama DB,
// pelanggan yang berdiri di depan kasir menerima kertas bernama orang lain,
// dan itu sekaligus membocorkan siapa yang terdaftar di nomor tersebut.
func TestCreateOrder_ReceiptUsesKasirTypedName(t *testing.T) {
	custID := uuid.New()
	cmd := &fakeOrderCmd{createResult: &orderapi.OrderSummary{
		ID: uuid.New(), Resi: "RJK-POS8", CustomerID: custID, Total: 50000,
		MetodeBayar: "cash", MetodeAmbil: "pickup",
	}}
	// Nomor sudah terdaftar atas nama lain.
	custs := &fakeCustomers{identity: &authapi.Identity{
		UserID: custID, Name: "Nama Lama Di DB", Phone: "6281234567890",
	}}
	svc := newSvc(cmd, custs, nil, nil)

	in := validInput(uuid.New())
	in.CustomerName = "Ibu Sari"

	result, err := svc.CreateOrder(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if result.CustomerName != "Ibu Sari" {
		t.Errorf("struk harus pakai nama yang diketik kasir: want %q got %q",
			"Ibu Sari", result.CustomerName)
	}
	// Telepon tetap dari identity — versi ternormalisasi 62xxx (§13), bukan
	// "081234567890" mentah yang diketik kasir.
	if result.CustomerPhone != "6281234567890" {
		t.Errorf("customer_phone harus ternormalisasi: want %q got %q",
			"6281234567890", result.CustomerPhone)
	}
}

// TestCreateOrder_ShippingCostNilForPickup memagari kontrak tiga-nilai
// `shipping_cost` (*int64 + omitempty): absent = pickup, 0 = kirim tapi ongkir
// gratis, >0 = kirim berbayar. Kalau pointer ini suatu saat "disederhanakan"
// jadi int64, omitempty akan menelan nilai 0 dan kirim-gratis jadi tak bisa
// dibedakan dari pickup di struk — tanpa satu test pun memerah.
func TestCreateOrder_ShippingCostNilForPickup(t *testing.T) {
	custID := uuid.New()
	cmd := &fakeOrderCmd{createResult: &orderapi.OrderSummary{
		ID: uuid.New(), Resi: "RJK-POS9", CustomerID: custID, Total: 50000,
		MetodeBayar: "cash", MetodeAmbil: "pickup",
		ShippingCost: nil,
	}}
	custs := &fakeCustomers{identity: &authapi.Identity{UserID: custID, Name: "Budi", Phone: "6281234567890"}}
	svc := newSvc(cmd, custs, nil, nil)

	result, err := svc.CreateOrder(context.Background(), validInput(uuid.New()))
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if result.ShippingCost != nil {
		t.Errorf("pickup harus TIDAK punya shipping_cost, got %d", *result.ShippingCost)
	}

	// Kirim dengan ongkir gratis: 0 harus tetap terbawa sebagai nilai, bukan
	// hilang jadi absent seperti pickup.
	gratis := int64(0)
	cmd2 := &fakeOrderCmd{createResult: &orderapi.OrderSummary{
		ID: uuid.New(), Resi: "RJK-POS10", CustomerID: custID, Total: 50000,
		MetodeBayar: "cash", MetodeAmbil: "kirim",
		ShippingCost: &gratis,
	}}
	svc2 := newSvc(cmd2, custs, nil, nil)
	result2, err := svc2.CreateOrder(context.Background(), validInput(uuid.New()))
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if result2.ShippingCost == nil {
		t.Fatal("kirim-gratis harus punya shipping_cost 0, bukan absent")
	}
	if *result2.ShippingCost != 0 {
		t.Errorf("shipping_cost: want 0 got %d", *result2.ShippingCost)
	}
}

func TestSearchCustomers_HappyPath_MapsIdentitiesToResults(t *testing.T) {
	custID := uuid.New()
	custs := &fakeCustomers{identity: &authapi.Identity{UserID: custID, Name: "Budi", Phone: "6281234567890", MembershipStatus: "active"}}
	svc := newSvc(&fakeOrderCmd{}, custs, nil, nil)

	results, err := svc.SearchCustomers(context.Background(), "budi")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if results[0].ID != custID || results[0].Name != "Budi" || results[0].Phone != "6281234567890" || results[0].MembershipStatus != "active" {
		t.Errorf("result not mapped correctly: %+v", results[0])
	}
}

func TestSearchCustomers_NoMatch_ReturnsEmptyNotNil(t *testing.T) {
	custs := &fakeCustomers{identity: nil}
	svc := newSvc(&fakeOrderCmd{}, custs, nil, nil)

	results, err := svc.SearchCustomers(context.Background(), "tidak ada")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if results == nil {
		t.Error("expected non-nil empty slice, got nil")
	}
	if len(results) != 0 {
		t.Errorf("want 0 results, got %d", len(results))
	}
}

func TestSearchCustomers_RepositoryError_WrappedAndPropagated(t *testing.T) {
	wantErr := errors.New("db unavailable")
	custs := &fakeCustomers{err: wantErr}
	svc := newSvc(&fakeOrderCmd{}, custs, nil, nil)

	_, err := svc.SearchCustomers(context.Background(), "budi")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("expected wrapped %v, got %v", wantErr, err)
	}
}
