package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/notification/notificationapi"
	"github.com/rajaku-printing/backend/internal/order/model"
	"github.com/rajaku-printing/backend/internal/order/orderapi"
	"github.com/rajaku-printing/backend/internal/order/repository"
	"github.com/rajaku-printing/backend/internal/order/state"
)

// fakeNotifier — spy implementing notificationapi.Enqueuer, used to assert
// OverrideStatus NEVER triggers WA (§ super admin order tools).
type fakeNotifier struct {
	calls int
}

func (f *fakeNotifier) EnqueueOrderEvent(_ context.Context, _ notificationapi.Kind, _ uuid.UUID, _ map[string]any) error {
	f.calls++
	return nil
}

// ---------- EditOrder ----------

func TestEditOrder_HappyPath_AlwaysEditableFields_PreDibayar(t *testing.T) {
	orderID := uuid.New()
	actor := uuid.New()
	existing := &model.Order{
		ID: orderID, Resi: "RJK-EDIT1", Status: state.OrderMasuk,
		ShippingAddress: strPtr("Jl. Lama No. 1"),
	}
	updated := &model.Order{
		ID: orderID, Resi: "RJK-EDIT1", Status: state.OrderMasuk,
		ShippingAddress: strPtr("Jl. Baru No. 2"),
	}
	store := &fakeStore{findOrders: []*model.Order{existing, updated}}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	newAddr := "Jl. Baru No. 2"
	got, err := svc.EditOrder(context.Background(), "RJK-EDIT1", actor, EditOrderInput{
		ShippingAddress: &newAddr,
	}, "koreksi alamat salah ketik")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if store.updateFieldsCalls != 1 {
		t.Fatalf("want 1 update fields call, got %d", store.updateFieldsCalls)
	}
	p := store.updateFieldsParams
	if p.Fields["shipping_address"] != newAddr {
		t.Errorf("shipping_address not staged: %+v", p.Fields)
	}
	if p.Audit == nil || p.Audit.Action != "order.edit" || p.Audit.ActorUserID != actor {
		t.Errorf("audit row wrong: %+v", p.Audit)
	}
	if _, ok := p.Audit.Changes["shipping_address"]; !ok {
		t.Errorf("changes must include shipping_address, got %+v", p.Audit.Changes)
	}
	if got.Resi != "RJK-EDIT1" {
		t.Errorf("returned order mismatch")
	}
}

func TestEditOrder_SubtotalChange_PostDibayar_ShortReason_ReturnsErrReasonRequired(t *testing.T) {
	existing := &model.Order{
		ID: uuid.New(), Resi: "RJK-EDIT2", Status: state.Dibayar, Subtotal: 100000, Total: 100000,
	}
	store := &fakeStore{findOrder: existing}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	newSubtotal := int64(150000)
	_, err := svc.EditOrder(context.Background(), "RJK-EDIT2", uuid.New(), EditOrderInput{
		Subtotal: &newSubtotal,
	}, "singkat") // < 10 chars
	if !errors.Is(err, orderapi.ErrReasonRequired) {
		t.Fatalf("want ErrReasonRequired, got %v", err)
	}
	if store.updateFieldsCalls != 0 {
		t.Errorf("must not touch store when reason too short, calls=%d", store.updateFieldsCalls)
	}
}

// TestEditOrder_SubtotalChange_PostDibayar_LongReason_RecomputesTotal is the
// regression guard for review finding #1: `total` is NEVER writable directly
// (EditOrderInput has no Total field at all) — it must always be recomputed
// as subtotal + shipping_cost.
func TestEditOrder_SubtotalChange_PostDibayar_LongReason_RecomputesTotal(t *testing.T) {
	orderID := uuid.New()
	shipping := int64(20000)
	existing := &model.Order{
		ID: orderID, Resi: "RJK-EDIT3", Status: state.Dibayar,
		Subtotal: 100000, ShippingCost: &shipping, Total: 120000,
	}
	updated := &model.Order{
		ID: orderID, Resi: "RJK-EDIT3", Status: state.Dibayar,
		Subtotal: 150000, ShippingCost: &shipping, Total: 170000,
	}
	store := &fakeStore{findOrders: []*model.Order{existing, updated}}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	newSubtotal := int64(150000)
	got, err := svc.EditOrder(context.Background(), "RJK-EDIT3", uuid.New(), EditOrderInput{
		Subtotal: &newSubtotal,
	}, "koreksi karena salah hitung harga manual")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if store.updateFieldsCalls != 1 {
		t.Fatalf("want 1 update fields call, got %d", store.updateFieldsCalls)
	}
	fields := store.updateFieldsParams.Fields
	if fields["subtotal"] != newSubtotal {
		t.Errorf("subtotal not staged: %+v", fields)
	}
	// total = subtotal + shipping_cost = 150000 + 20000, RECOMPUTED, never
	// taken straight from caller input (there's no Total field to take from).
	wantTotal := int64(170000)
	if fields["total"] != wantTotal {
		t.Errorf("total not recomputed correctly: %+v (want total=%d)", fields, wantTotal)
	}
	if got.Total != wantTotal {
		t.Errorf("returned total wrong: %d", got.Total)
	}
}

// TestEditOrder_ShippingCostChange_PreDibayar_RecomputesTotal_NoLongReasonNeeded
// covers the pre-payment branch of the same invariant: shipping_cost alone
// changing must also recompute total, and (being pre-dibayar) doesn't need
// the long financial reason.
func TestEditOrder_ShippingCostChange_PreDibayar_RecomputesTotal_NoLongReasonNeeded(t *testing.T) {
	orderID := uuid.New()
	oldShipping := int64(10000)
	existing := &model.Order{
		ID: orderID, Resi: "RJK-EDIT9", Status: state.MenungguOngkir,
		Subtotal: 80000, ShippingCost: &oldShipping, Total: 90000,
	}
	store := &fakeStore{findOrder: existing}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	newShipping := int64(25000)
	_, err := svc.EditOrder(context.Background(), "RJK-EDIT9", uuid.New(), EditOrderInput{
		ShippingCost: &newShipping,
	}, "ubah") // short reason OK — order belum dibayar
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	fields := store.updateFieldsParams.Fields
	if fields["shipping_cost"] != newShipping {
		t.Errorf("shipping_cost not staged: %+v", fields)
	}
	wantTotal := int64(105000) // 80000 + 25000
	if fields["total"] != wantTotal {
		t.Errorf("total not recomputed: %+v (want %d)", fields, wantTotal)
	}
}

func TestEditOrder_TerminalStatus_ReturnsErrFieldNotEditable(t *testing.T) {
	existing := &model.Order{
		ID: uuid.New(), Resi: "RJK-EDIT4", Status: state.Selesai,
	}
	store := &fakeStore{findOrder: existing}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	addr := "Jl. Baru"
	_, err := svc.EditOrder(context.Background(), "RJK-EDIT4", uuid.New(), EditOrderInput{
		ShippingAddress: &addr,
	}, "koreksi alamat")
	if !errors.Is(err, orderapi.ErrFieldNotEditable) {
		t.Fatalf("want ErrFieldNotEditable, got %v", err)
	}
	if store.updateFieldsCalls != 0 {
		t.Errorf("must not touch store for terminal status, calls=%d", store.updateFieldsCalls)
	}
}

func TestEditOrder_ReasonTooShort_ReturnsErrReasonRequired(t *testing.T) {
	existing := &model.Order{ID: uuid.New(), Resi: "RJK-EDIT5", Status: state.OrderMasuk}
	store := &fakeStore{findOrder: existing}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	addr := "Jl. Baru"
	_, err := svc.EditOrder(context.Background(), "RJK-EDIT5", uuid.New(), EditOrderInput{
		ShippingAddress: &addr,
	}, "  ") // empty after trim
	if !errors.Is(err, orderapi.ErrReasonRequired) {
		t.Fatalf("want ErrReasonRequired, got %v", err)
	}
}

// TestEditOrder_ShippingRecipientChange_NormalizesPhoneAndStagesFields is the
// replacement coverage for review finding #4/#5: EditOrder no longer touches
// customer identity (users.phone, a GLOBAL matching key — §11) at all; it
// only edits the PER-ORDER shipping_recipient_name/phone columns, in the
// SAME orders-table transaction as everything else (no cross-module call).
func TestEditOrder_ShippingRecipientChange_NormalizesPhoneAndStagesFields(t *testing.T) {
	orderID := uuid.New()
	existing := &model.Order{
		ID: orderID, Resi: "RJK-EDIT6", Status: state.OrderMasuk,
		ShippingRecipientName:  strPtr("Ani Lama"),
		ShippingRecipientPhone: strPtr("6281111111111"),
	}
	store := &fakeStore{findOrder: existing}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	newName := "Ani Baru"
	newPhone := "081234567890" // raw → should normalize to 6281234567890
	got, err := svc.EditOrder(context.Background(), "RJK-EDIT6", uuid.New(), EditOrderInput{
		ShippingRecipientName:  &newName,
		ShippingRecipientPhone: &newPhone,
	}, "koreksi kontak salah input kasir")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if store.updateFieldsCalls != 1 {
		t.Fatalf("want 1 update fields call, got %d", store.updateFieldsCalls)
	}
	fields := store.updateFieldsParams.Fields
	if fields["shipping_recipient_name"] != newName {
		t.Errorf("name not staged: %+v", fields)
	}
	if fields["shipping_recipient_phone"] != "6281234567890" {
		t.Errorf("phone not normalized: %+v", fields)
	}
	if got.Resi != "RJK-EDIT6" {
		t.Errorf("returned order mismatch")
	}
}

// TestEditOrder_ShippingRecipientPhone_InvalidFormat_ReturnsError — kasus
// gagal: nomor yang tidak bisa dinormalisasi ditolak sebelum menyentuh store.
func TestEditOrder_ShippingRecipientPhone_InvalidFormat_ReturnsError(t *testing.T) {
	existing := &model.Order{ID: uuid.New(), Resi: "RJK-EDIT7", Status: state.OrderMasuk}
	store := &fakeStore{findOrder: existing}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	badPhone := "abc-not-a-phone"
	_, err := svc.EditOrder(context.Background(), "RJK-EDIT7", uuid.New(), EditOrderInput{
		ShippingRecipientPhone: &badPhone,
	}, "koreksi nomor penerima")
	if err == nil {
		t.Fatal("want error for invalid phone format")
	}
	if store.updateFieldsCalls != 0 {
		t.Errorf("must not touch store on invalid phone, calls=%d", store.updateFieldsCalls)
	}
}

// TestEditOrder_StatusChangedConcurrently_ReturnsErrOrderStateChanged is the
// regression guard for review finding #3 (TOCTOU): if the order's status
// changes between the initial read and the write, the repository's row-lock
// re-check must abort with ErrStaleState, which the service maps to
// orderapi.ErrOrderStateChanged.
func TestEditOrder_StatusChangedConcurrently_ReturnsErrOrderStateChanged(t *testing.T) {
	existing := &model.Order{ID: uuid.New(), Resi: "RJK-EDIT10", Status: state.OrderMasuk}
	store := &fakeStore{findOrder: existing, updateFieldsErr: repository.ErrStaleState}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	addr := "Jl. Baru"
	_, err := svc.EditOrder(context.Background(), "RJK-EDIT10", uuid.New(), EditOrderInput{
		ShippingAddress: &addr,
	}, "koreksi alamat")
	if !errors.Is(err, orderapi.ErrOrderStateChanged) {
		t.Fatalf("want ErrOrderStateChanged, got %v", err)
	}
}

func TestEditOrder_NoActualChanges_NoOpDoesNotWriteAudit(t *testing.T) {
	existing := &model.Order{
		ID: uuid.New(), Resi: "RJK-EDIT8", Status: state.OrderMasuk,
		ShippingAddress: strPtr("Jl. Sama"),
	}
	store := &fakeStore{findOrder: existing}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	same := "Jl. Sama"
	got, err := svc.EditOrder(context.Background(), "RJK-EDIT8", uuid.New(), EditOrderInput{
		ShippingAddress: &same,
	}, "tidak ada perubahan sebenarnya")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if store.updateFieldsCalls != 0 {
		t.Errorf("no-op edit must not write audit row, calls=%d", store.updateFieldsCalls)
	}
	if got.Resi != existing.Resi {
		t.Errorf("should return the order unchanged")
	}
}

func TestEditOrder_NotFound(t *testing.T) {
	store := &fakeStore{findErr: repository.ErrNotFound}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	addr := "Jl. Baru"
	_, err := svc.EditOrder(context.Background(), "RJK-NOPE", uuid.New(), EditOrderInput{
		ShippingAddress: &addr,
	}, "koreksi alamat")
	if !errors.Is(err, orderapi.ErrOrderNotFound) {
		t.Fatalf("want ErrOrderNotFound, got %v", err)
	}
}

// ---------- OverrideStatus ----------

func TestOverrideStatus_HappyPath(t *testing.T) {
	orderID := uuid.New()
	actor := uuid.New()
	existing := &model.Order{ID: orderID, Resi: "RJK-OVR1", Status: state.MenungguVerifikasi}
	updated := &model.Order{ID: orderID, Resi: "RJK-OVR1", Status: state.ProsesCetak}
	store := &fakeStore{findOrders: []*model.Order{existing, updated}}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	got, err := svc.OverrideStatus(context.Background(), "RJK-OVR1", actor, state.ProsesCetak,
		"stuck karena bug validasi, langsung dorong ke cetak")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if store.overrideStatusCalls != 1 {
		t.Fatalf("want 1 override call, got %d", store.overrideStatusCalls)
	}
	p := store.overrideStatusParams
	if p.NewStatus != string(state.ProsesCetak) {
		t.Errorf("new status wrong: %s", p.NewStatus)
	}
	if p.Note == nil || !strings.HasPrefix(*p.Note, "[OVERRIDE] ") {
		t.Errorf("note must be prefixed [OVERRIDE], got %v", p.Note)
	}
	if p.Audit == nil || p.Audit.Action != "order.override_status" {
		t.Errorf("audit action wrong: %+v", p.Audit)
	}
	if got.Status != state.ProsesCetak {
		t.Errorf("returned status wrong: %s", got.Status)
	}
}

func TestOverrideStatus_NoReason_ReturnsErrReasonRequired(t *testing.T) {
	store := &fakeStore{}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	_, err := svc.OverrideStatus(context.Background(), "RJK-OVR2", uuid.New(), state.Selesai, "")
	if !errors.Is(err, orderapi.ErrReasonRequired) {
		t.Fatalf("want ErrReasonRequired, got %v", err)
	}
	if store.overrideStatusCalls != 0 {
		t.Errorf("must not touch store without reason, calls=%d", store.overrideStatusCalls)
	}
}

func TestOverrideStatus_UnknownStatus_Rejected(t *testing.T) {
	store := &fakeStore{}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	_, err := svc.OverrideStatus(context.Background(), "RJK-OVR3", uuid.New(), state.Status("status_ngarang"),
		"alasan yang cukup panjang untuk lolos validasi")
	if !errors.Is(err, orderapi.ErrStatusUnknown) {
		t.Fatalf("want ErrStatusUnknown, got %v", err)
	}
	if store.overrideStatusCalls != 0 {
		t.Errorf("must not touch store for unknown status, calls=%d", store.overrideStatusCalls)
	}
}

// TestOverrideStatus_NeverEnqueuesNotification is the critical regression
// guard for § super admin order tools: an admin override must NEVER trigger
// customer-facing WA, unlike normal transitions.
func TestOverrideStatus_NeverEnqueuesNotification(t *testing.T) {
	orderID := uuid.New()
	existing := &model.Order{ID: orderID, Resi: "RJK-OVR4", Status: state.MenungguVerifikasi}
	updated := &model.Order{ID: orderID, Resi: "RJK-OVR4", Status: state.Selesai}
	store := &fakeStore{findOrders: []*model.Order{existing, updated}}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})
	spy := &fakeNotifier{}
	svc.SetNotifier(spy)

	_, err := svc.OverrideStatus(context.Background(), "RJK-OVR4", uuid.New(), state.Selesai,
		"tutup manual karena pembeli sudah lunas cash di luar sistem")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if spy.calls != 0 {
		t.Fatalf("OverrideStatus must NEVER enqueue notification, got %d calls", spy.calls)
	}
}

// ---------- SoftDeleteOrder ----------

func TestSoftDeleteOrder_HappyPath(t *testing.T) {
	orderID := uuid.New()
	actor := uuid.New()
	existing := &model.Order{ID: orderID, Resi: "RJK-DEL1", Status: state.OrderMasuk}
	store := &fakeStore{findOrder: existing}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	err := svc.SoftDeleteOrder(context.Background(), "RJK-DEL1", actor, "order duplikat, dibuat 2x oleh kasir")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if store.softDeleteCalls != 1 {
		t.Fatalf("want 1 soft delete call, got %d", store.softDeleteCalls)
	}
	p := store.softDeleteParams
	if p.OrderID != orderID || p.ActorID != actor {
		t.Errorf("params wrong: %+v", p)
	}
	if p.Audit == nil || p.Audit.Action != "order.delete" {
		t.Errorf("audit action wrong: %+v", p.Audit)
	}
	// This is exactly the guard that makes a soft-deleted order stop showing
	// up in admin list/rekap/tracking: repository.OrderRepository.SoftDelete
	// (see order_repository.go) stamps deleted_at, and EVERY reading query in
	// that file (ListForAdmin, FindByResi, FindByID, ListByCustomer,
	// ListPOSByDateRange) filters `deleted_at IS NULL`. The service layer
	// only orchestrates the call — actual exclusion is a repository-level
	// SQL guarantee, exercised here via the params passed to it.
}

func TestSoftDeleteOrder_ShortReason_ReturnsErrReasonRequired(t *testing.T) {
	store := &fakeStore{}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	err := svc.SoftDeleteOrder(context.Background(), "RJK-DEL2", uuid.New(), "duplikat")
	if !errors.Is(err, orderapi.ErrReasonRequired) {
		t.Fatalf("want ErrReasonRequired, got %v", err)
	}
	if store.softDeleteCalls != 0 {
		t.Errorf("must not touch store without valid reason, calls=%d", store.softDeleteCalls)
	}
}

func TestSoftDeleteOrder_NotFound(t *testing.T) {
	store := &fakeStore{findErr: repository.ErrNotFound}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	err := svc.SoftDeleteOrder(context.Background(), "RJK-DEL3", uuid.New(), "order duplikat dari kasir A")
	if !errors.Is(err, orderapi.ErrOrderNotFound) {
		t.Fatalf("want ErrOrderNotFound, got %v", err)
	}
}

// TestSoftDeleteOrder_AlreadyPaid_ReturnsErrDeleteNotAllowedPaid is the
// regression guard for review finding #2: a paid order must never
// disappear from cash reconciliation via delete — it has to be cancelled
// first.
func TestSoftDeleteOrder_AlreadyPaid_ReturnsErrDeleteNotAllowedPaid(t *testing.T) {
	existing := &model.Order{ID: uuid.New(), Resi: "RJK-DEL4", Status: state.Dibayar}
	store := &fakeStore{findOrder: existing}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	err := svc.SoftDeleteOrder(context.Background(), "RJK-DEL4", uuid.New(), "order duplikat tapi sudah dibayar")
	if !errors.Is(err, orderapi.ErrDeleteNotAllowedPaid) {
		t.Fatalf("want ErrDeleteNotAllowedPaid, got %v", err)
	}
	if store.softDeleteCalls != 0 {
		t.Errorf("must not touch store for an already-paid order, calls=%d", store.softDeleteCalls)
	}
}

// TestSoftDeleteOrder_Dibatalkan_StillDeletable — Dibatalkan is the one
// "post pre-dibayar" status that MUST remain deletable (a cancelled order
// never contributed to cash reconciliation in the first place).
func TestSoftDeleteOrder_Dibatalkan_StillDeletable(t *testing.T) {
	existing := &model.Order{ID: uuid.New(), Resi: "RJK-DEL5", Status: state.Dibatalkan}
	store := &fakeStore{findOrder: existing}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	err := svc.SoftDeleteOrder(context.Background(), "RJK-DEL5", uuid.New(), "order dibatalkan, hapus dari daftar")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if store.softDeleteCalls != 1 {
		t.Errorf("want 1 soft delete call, got %d", store.softDeleteCalls)
	}
}

// TestSoftDeleteOrder_RepositoryTOCTOUGuard_MapsToErrDeleteNotAllowedPaid
// covers the authoritative, row-locked re-check inside
// repository.OrderRepository.SoftDelete (review finding #3 applied to
// delete too) — even when the service's own early check passes (order was
// pre-dibayar at read time), the repository can still reject if it got paid
// concurrently; the service must map that to the same public sentinel.
func TestSoftDeleteOrder_RepositoryTOCTOUGuard_MapsToErrDeleteNotAllowedPaid(t *testing.T) {
	existing := &model.Order{ID: uuid.New(), Resi: "RJK-DEL6", Status: state.OrderMasuk}
	store := &fakeStore{findOrder: existing, softDeleteErr: repository.ErrDeleteForbiddenStatus}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	err := svc.SoftDeleteOrder(context.Background(), "RJK-DEL6", uuid.New(), "race — dibayar barusan")
	if !errors.Is(err, orderapi.ErrDeleteNotAllowedPaid) {
		t.Fatalf("want ErrDeleteNotAllowedPaid, got %v", err)
	}
}

// ---------- ListAuditLog ----------

func TestListAuditLog_HappyPath(t *testing.T) {
	orderID := uuid.New()
	label := "RJK-AUD1"
	audit := &fakeAuditStore{rows: []model.AdminAuditLog{
		{ID: uuid.New(), ActorUserID: uuid.New(), Action: "order.edit", EntityType: "order",
			EntityID: orderID, EntityLabel: &label, Reason: "koreksi data",
			Changes: model.ChangeSet{"shipping_address": {From: "A", To: "B"}}},
	}}
	store := &fakeStore{}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})
	svc.SetAuditStore(audit)

	rows, err := svc.ListAuditLog(context.Background(), &orderID, 10)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if audit.calls != 1 {
		t.Fatalf("want 1 list call, got %d", audit.calls)
	}
	if audit.filter.EntityType != "order" || audit.filter.EntityID == nil || *audit.filter.EntityID != orderID {
		t.Errorf("filter wrong: %+v", audit.filter)
	}
	if len(rows) != 1 || rows[0].EntityLabel != "RJK-AUD1" {
		t.Fatalf("rows wrong: %+v", rows)
	}
}

func TestListAuditLog_StoreNotWired_ReturnsError(t *testing.T) {
	svc := New(&fakeStore{}, &fakeCatalog{}, &fakeCustomers{})
	// SetAuditStore deliberately NOT called.
	_, err := svc.ListAuditLog(context.Background(), nil, 10)
	if err == nil {
		t.Fatal("want error when audit store not wired")
	}
}
