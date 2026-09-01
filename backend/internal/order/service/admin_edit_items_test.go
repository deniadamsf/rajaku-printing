// Super admin item-level order correction (§32.9) — tests. Kept separate
// from admin_override_test.go (§22, one concern per test file, mirrors the
// production file split).
package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/catalog/catalogapi"
	"github.com/rajaku-printing/backend/internal/order/model"
	"github.com/rajaku-printing/backend/internal/order/orderapi"
	"github.com/rajaku-printing/backend/internal/order/state"
)

// TestEditOrder_Items_ChangeQuantity_RecomputesSubtotalAndTotal is the §32.9
// happy path: super admin corrects an existing row's quantity, and the
// service recomputes item.subtotal, orders.subtotal, and orders.total —
// WITHOUT touching product_id/material_id snapshot columns.
func TestEditOrder_Items_ChangeQuantity_RecomputesSubtotalAndTotal(t *testing.T) {
	orderID := uuid.New()
	itemID := uuid.New()
	productID, materialID := uuid.New(), uuid.New()
	existing := &model.Order{
		ID: orderID, Resi: "RJK-ITEM1", Status: state.OrderMasuk,
		Subtotal: 50_000, Total: 50_000,
		Items: []model.OrderItem{{
			ID: itemID, LineNo: 1,
			ProductID: &productID, ProductNameSnapshot: "Banner Flexi",
			MaterialID: &materialID, MaterialNameSnapshot: "Flexi 280",
			PricingTypeSnapshot: "per_m2",
			WidthCm:             100, HeightCm: 200, Quantity: 1,
			UnitPrice: 50_000, Subtotal: 50_000,
			DesignSource: model.DesignSourceUpload,
		}},
	}
	updated := &model.Order{ID: orderID, Resi: "RJK-ITEM1", Status: state.OrderMasuk, Subtotal: 100_000, Total: 100_000}
	store := &fakeStore{findOrders: []*model.Order{existing, updated}}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	got, err := svc.EditOrder(context.Background(), "RJK-ITEM1", uuid.New(), EditOrderInput{
		Items: &[]EditOrderItemInput{
			{ID: &itemID, WidthCm: 100, HeightCm: 200, Quantity: 2, UnitPrice: 50_000},
		},
	}, "koreksi jumlah salah input kasir")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if store.updateItemsCalls != 1 {
		t.Fatalf("want 1 update items call, got %d", store.updateItemsCalls)
	}
	p := store.updateItemsParams
	if p.OrderFields["subtotal"] != int64(100_000) {
		t.Errorf("subtotal not recomputed: %+v", p.OrderFields)
	}
	if p.OrderFields["total"] != int64(100_000) {
		t.Errorf("total not recomputed: %+v", p.OrderFields)
	}
	if len(p.UpsertItems) != 1 || p.UpsertItems[0].Quantity != 2 || p.UpsertItems[0].Subtotal != 100_000 {
		t.Fatalf("upsert item wrong: %+v", p.UpsertItems)
	}
	if p.UpsertItems[0].ProductNameSnapshot != "Banner Flexi" {
		t.Errorf("product snapshot must stay untouched: %+v", p.UpsertItems[0])
	}
	if len(p.DeleteItemIDs) != 0 {
		t.Errorf("no rows deleted, got %+v", p.DeleteItemIDs)
	}
	if got.Resi != "RJK-ITEM1" {
		t.Errorf("returned order mismatch")
	}
}

// TestEditOrder_Items_ChangeProductID_Rejected is the §32.9 kasus gagal
// wajib: mengganti product_id/material_id pada baris yang SUDAH ADA ditolak
// eksplisit — mengganti produk berarti pesanan berbeda (hapus + tambah
// baru), bukan edit.
func TestEditOrder_Items_ChangeProductID_Rejected(t *testing.T) {
	orderID := uuid.New()
	itemID := uuid.New()
	oldProductID := uuid.New()
	existing := &model.Order{
		ID: orderID, Resi: "RJK-ITEM2", Status: state.OrderMasuk,
		Subtotal: 50_000, Total: 50_000,
		Items: []model.OrderItem{{
			ID: itemID, LineNo: 1, ProductID: &oldProductID, ProductNameSnapshot: "Banner Flexi",
			MaterialNameSnapshot: "Flexi 280", WidthCm: 100, HeightCm: 200, Quantity: 1,
			UnitPrice: 50_000, Subtotal: 50_000, DesignSource: model.DesignSourceUpload,
		}},
	}
	store := &fakeStore{findOrder: existing}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	newProductID := uuid.New()
	_, err := svc.EditOrder(context.Background(), "RJK-ITEM2", uuid.New(), EditOrderInput{
		Items: &[]EditOrderItemInput{
			{ID: &itemID, ProductID: &newProductID, WidthCm: 100, HeightCm: 200, Quantity: 1, UnitPrice: 50_000},
		},
	}, "coba ganti produk baris existing")
	if !errors.Is(err, orderapi.ErrOrderItemProductNotEditable) {
		t.Fatalf("want ErrOrderItemProductNotEditable, got %v", err)
	}
	if store.updateItemsCalls != 0 {
		t.Errorf("must not touch store, calls=%d", store.updateItemsCalls)
	}
}

// TestEditOrder_Items_AddNewRow_QuotedViaCatalog verifies §32.9's rule that
// a NEW row is always re-quoted via catalog (never trust an admin-typed
// price), and that adding a 'request' row to an all-'upload' order flips
// orders.design_source to 'mixed' (deriveDesignSource re-run, §32.1).
func TestEditOrder_Items_AddNewRow_QuotedViaCatalog(t *testing.T) {
	orderID := uuid.New()
	itemID := uuid.New()
	oldProductID, oldMaterialID := uuid.New(), uuid.New()
	existing := &model.Order{
		ID: orderID, Resi: "RJK-ITEM3", Status: state.OrderMasuk,
		Subtotal: 50_000, Total: 50_000, DesignSource: model.DesignSourceUpload,
		Items: []model.OrderItem{{
			ID: itemID, LineNo: 1, ProductID: &oldProductID, ProductNameSnapshot: "Banner Flexi",
			MaterialID: &oldMaterialID, MaterialNameSnapshot: "Flexi 280", WidthCm: 100, HeightCm: 200,
			Quantity: 1, UnitPrice: 50_000, Subtotal: 50_000, DesignSource: model.DesignSourceUpload,
		}},
	}
	updated := &model.Order{ID: orderID, Resi: "RJK-ITEM3", Status: state.OrderMasuk}
	store := &fakeStore{findOrders: []*model.Order{existing, updated}}
	newProductID, newMaterialID := uuid.New(), uuid.New()
	catalog := &fakeCatalog{quoteResult: &catalogapi.QuoteResult{
		ProductID: newProductID, ProductName: "Banner Vinyl", MaterialID: newMaterialID, MaterialName: "Vinyl",
		PricingType: catalogapi.PricingTypePerM2, WidthCm: 100, HeightCm: 100, TotalPrice: 30_000,
	}}
	svc := New(store, catalog, &fakeCustomers{})

	_, err := svc.EditOrder(context.Background(), "RJK-ITEM3", uuid.New(), EditOrderInput{
		Items: &[]EditOrderItemInput{
			{ID: &itemID, WidthCm: 100, HeightCm: 200, Quantity: 1, UnitPrice: 50_000},
			{ProductID: &newProductID, MaterialID: &newMaterialID, WidthCm: 100, HeightCm: 100, Quantity: 1,
				UnitPrice: 999_999_999, DesignSource: "request"}, // UnitPrice sengaja diabaikan
		},
	}, "tambah baris baru banner vinyl request desain")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if catalog.quoteCalls != 1 {
		t.Fatalf("want catalog.Quote called once for the new row, got %d", catalog.quoteCalls)
	}
	p := store.updateItemsParams
	if len(p.UpsertItems) != 2 {
		t.Fatalf("want 2 upsert items (1 existing + 1 new), got %d", len(p.UpsertItems))
	}
	newRow := p.UpsertItems[1]
	if newRow.ID != uuid.Nil {
		t.Errorf("new row must have zero ID (insert), got %s", newRow.ID)
	}
	if newRow.UnitPrice != 30_000 || newRow.Subtotal != 30_000 {
		t.Fatalf("new row must be re-quoted via catalog (30000), NOT trust client unit_price: %+v", newRow)
	}
	wantSubtotal := int64(50_000 + 30_000)
	if p.OrderFields["subtotal"] != wantSubtotal {
		t.Errorf("subtotal = %v, want %d", p.OrderFields["subtotal"], wantSubtotal)
	}
	if p.OrderFields["design_source"] != string(model.DesignSourceMixed) {
		t.Errorf("design_source = %v, want mixed (upload + request)", p.OrderFields["design_source"])
	}
}

// TestEditOrder_Items_DeleteRow_MixedBecomesUpload — §32.9: menghapus
// satu-satunya baris 'request' pada order campuran wajib mengubah
// orders.design_source dari 'mixed' jadi 'upload'.
func TestEditOrder_Items_DeleteRow_MixedBecomesUpload(t *testing.T) {
	orderID := uuid.New()
	keepID, dropID := uuid.New(), uuid.New()
	productID := uuid.New()
	existing := &model.Order{
		ID: orderID, Resi: "RJK-ITEM4", Status: state.OrderMasuk,
		Subtotal: 80_000, Total: 80_000, DesignSource: model.DesignSourceMixed,
		Items: []model.OrderItem{
			{ID: keepID, LineNo: 1, ProductID: &productID, ProductNameSnapshot: "A", MaterialNameSnapshot: "MA",
				WidthCm: 100, HeightCm: 100, Quantity: 1, UnitPrice: 50_000, Subtotal: 50_000, DesignSource: model.DesignSourceUpload},
			{ID: dropID, LineNo: 2, ProductID: &productID, ProductNameSnapshot: "B", MaterialNameSnapshot: "MB",
				WidthCm: 100, HeightCm: 100, Quantity: 1, UnitPrice: 30_000, Subtotal: 30_000, DesignSource: model.DesignSourceRequest},
		},
	}
	updated := &model.Order{ID: orderID, Resi: "RJK-ITEM4", Status: state.OrderMasuk}
	store := &fakeStore{findOrders: []*model.Order{existing, updated}}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	_, err := svc.EditOrder(context.Background(), "RJK-ITEM4", uuid.New(), EditOrderInput{
		Items: &[]EditOrderItemInput{
			{ID: &keepID, WidthCm: 100, HeightCm: 100, Quantity: 1, UnitPrice: 50_000},
		},
	}, "hapus baris request, pelanggan batal minta desain")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	p := store.updateItemsParams
	if len(p.DeleteItemIDs) != 1 || p.DeleteItemIDs[0] != dropID {
		t.Fatalf("DeleteItemIDs = %+v, want [%s]", p.DeleteItemIDs, dropID)
	}
	if p.OrderFields["design_source"] != string(model.DesignSourceUpload) {
		t.Errorf("design_source = %v, want upload (mixed -> upload after dropping the only 'request' row)", p.OrderFields["design_source"])
	}
	if p.OrderFields["subtotal"] != int64(50_000) {
		t.Errorf("subtotal = %v, want 50000", p.OrderFields["subtotal"])
	}
}

// TestEditOrder_Items_DiscountExceedsNewSubtotal_Rejected is the §32.9 kasus
// gagal wajib: mengecilkan subtotal lewat edit item sampai di bawah
// discount_amount yang sudah tercatat harus ditolak eksplisit, bukan
// dijepit diam-diam.
func TestEditOrder_Items_DiscountExceedsNewSubtotal_Rejected(t *testing.T) {
	orderID := uuid.New()
	itemID := uuid.New()
	existing := &model.Order{
		ID: orderID, Resi: "RJK-ITEM5", Status: state.Dibayar,
		Subtotal: 100_000, DiscountAmount: 80_000, Total: 20_000,
		Items: []model.OrderItem{{
			ID: itemID, LineNo: 1, ProductNameSnapshot: "A", MaterialNameSnapshot: "MA",
			WidthCm: 100, HeightCm: 100, Quantity: 1, UnitPrice: 100_000, Subtotal: 100_000,
			DesignSource: model.DesignSourceUpload,
		}},
	}
	store := &fakeStore{findOrder: existing}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	_, err := svc.EditOrder(context.Background(), "RJK-ITEM5", uuid.New(), EditOrderInput{
		Items: &[]EditOrderItemInput{
			{ID: &itemID, WidthCm: 100, HeightCm: 100, Quantity: 1, UnitPrice: 50_000}, // subtotal turun jadi 50000 < discount 80000
		},
	}, "koreksi harga jadi lebih murah dari diskon yang sudah tercatat")
	if !errors.Is(err, orderapi.ErrOrderDiscountExceedsSubtotal) {
		t.Fatalf("want ErrOrderDiscountExceedsSubtotal, got %v", err)
	}
	if store.updateItemsCalls != 0 {
		t.Errorf("must not touch store, calls=%d", store.updateItemsCalls)
	}
}

// TestEditOrder_Items_ReallocatesDiscountAcrossNewComposition — §32.9:
// discount_amount TIDAK berubah nilainya, hanya pembagiannya ke baris yang
// menyesuaikan komposisi item baru.
func TestEditOrder_Items_ReallocatesDiscountAcrossNewComposition(t *testing.T) {
	orderID := uuid.New()
	item1, item2 := uuid.New(), uuid.New()
	existing := &model.Order{
		ID: orderID, Resi: "RJK-ITEM6", Status: state.Dibayar,
		Subtotal: 100_000, DiscountAmount: 10_000, Total: 90_000,
		Items: []model.OrderItem{
			{ID: item1, LineNo: 1, ProductNameSnapshot: "A", MaterialNameSnapshot: "MA",
				WidthCm: 100, HeightCm: 100, Quantity: 1, UnitPrice: 50_000, Subtotal: 50_000,
				DiscountAmount: 5_000, DesignSource: model.DesignSourceUpload},
			{ID: item2, LineNo: 2, ProductNameSnapshot: "B", MaterialNameSnapshot: "MB",
				WidthCm: 100, HeightCm: 100, Quantity: 1, UnitPrice: 50_000, Subtotal: 50_000,
				DiscountAmount: 5_000, DesignSource: model.DesignSourceUpload},
		},
	}
	updated := &model.Order{ID: orderID, Resi: "RJK-ITEM6", Status: state.Dibayar}
	store := &fakeStore{findOrders: []*model.Order{existing, updated}}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	// Item 1 dinaikkan ke 90000, item 2 tetap 10000 -> subtotal baru 100000
	// (tidak berubah), TAPI komposisinya berubah drastis: diskon 10000 harus
	// dialokasikan ulang proporsional ke 90:10, bukan tetap 5000:5000.
	_, err := svc.EditOrder(context.Background(), "RJK-ITEM6", uuid.New(), EditOrderInput{
		Items: &[]EditOrderItemInput{
			{ID: &item1, WidthCm: 100, HeightCm: 100, Quantity: 1, UnitPrice: 90_000},
			{ID: &item2, WidthCm: 100, HeightCm: 100, Quantity: 1, UnitPrice: 10_000},
		},
	}, "koreksi harga jual dua baris ini, alasan cukup panjang")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	p := store.updateItemsParams
	var sumAlloc int64
	byLine := map[int]int64{}
	for _, it := range p.UpsertItems {
		sumAlloc += it.DiscountAmount
		byLine[it.LineNo] = it.DiscountAmount
	}
	if sumAlloc != 10_000 {
		t.Fatalf("Σ item.DiscountAmount = %d, want 10000 (orders.DiscountAmount unchanged)", sumAlloc)
	}
	if byLine[1] != 9_000 || byLine[2] != 1_000 {
		t.Errorf("allocation not re-split proportionally to new composition (90:10): got %+v, want {1:9000, 2:1000}", byLine)
	}
	// discount_amount itu sendiri TIDAK boleh masuk fields (tidak berubah).
	if _, touched := p.OrderFields["discount_amount"]; touched {
		t.Errorf("discount_amount must never be staged for edit — only its per-item split changes: %+v", p.OrderFields)
	}
}

// TestEditOrder_Items_DuplicateID_Rejected is the regression guard for
// temuan review §32.9 #1 (PALING PENTING): sending the SAME existing item ID
// twice in EditOrderInput.Items must be rejected up front. Without this
// guard, resolveItemsForEdit would add that row's Subtotal to the computed
// aggregate TWICE while the repository only UPDATEs the one physical row
// once (same ID) — Σ order_items.subtotal ends up HALF of orders.subtotal
// with no error anywhere (§32.2 invariant silently broken).
func TestEditOrder_Items_DuplicateID_Rejected(t *testing.T) {
	orderID := uuid.New()
	itemID := uuid.New()
	existing := &model.Order{
		ID: orderID, Resi: "RJK-ITEM8", Status: state.OrderMasuk,
		Subtotal: 100_000, Total: 100_000,
		Items: []model.OrderItem{{
			ID: itemID, LineNo: 1, ProductNameSnapshot: "A", MaterialNameSnapshot: "MA",
			WidthCm: 100, HeightCm: 100, Quantity: 1, UnitPrice: 100_000, Subtotal: 100_000,
			DesignSource: model.DesignSourceUpload,
		}},
	}
	store := &fakeStore{findOrder: existing}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	_, err := svc.EditOrder(context.Background(), "RJK-ITEM8", uuid.New(), EditOrderInput{
		Items: &[]EditOrderItemInput{
			{ID: &itemID, WidthCm: 100, HeightCm: 100, Quantity: 1, UnitPrice: 100_000},
			{ID: &itemID, WidthCm: 100, HeightCm: 100, Quantity: 1, UnitPrice: 100_000},
		},
	}, "coba kirim id item yang sama dua kali")
	if !errors.Is(err, orderapi.ErrOrderItemDuplicate) {
		t.Fatalf("want ErrOrderItemDuplicate, got %v", err)
	}
	if store.updateItemsCalls != 0 {
		t.Errorf("must not touch store when duplicate item id sent, calls=%d", store.updateItemsCalls)
	}
}

// TestEditOrder_Items_ReorderOnly_PersistsNewLineNo is the regression guard
// for temuan review §32.9 #6: swapping two rows' display order (nothing
// else changed) must actually persist — before the fix, itemChangeSummary
// didn't include LineNo, so a pure reorder produced an IDENTICAL summary
// before/after, EditOrder's `len(changes) == 0` short-circuit fired, and the
// whole request was silently discarded (200 OK, nothing written, no audit
// row).
func TestEditOrder_Items_ReorderOnly_PersistsNewLineNo(t *testing.T) {
	orderID := uuid.New()
	itemA, itemB := uuid.New(), uuid.New()
	existing := &model.Order{
		ID: orderID, Resi: "RJK-ITEM9", Status: state.OrderMasuk,
		Subtotal: 80_000, Total: 80_000,
		Items: []model.OrderItem{
			{ID: itemA, LineNo: 1, ProductNameSnapshot: "A", MaterialNameSnapshot: "MA",
				WidthCm: 100, HeightCm: 100, Quantity: 1, UnitPrice: 50_000, Subtotal: 50_000,
				DesignSource: model.DesignSourceUpload},
			{ID: itemB, LineNo: 2, ProductNameSnapshot: "B", MaterialNameSnapshot: "MB",
				WidthCm: 100, HeightCm: 100, Quantity: 1, UnitPrice: 30_000, Subtotal: 30_000,
				DesignSource: model.DesignSourceUpload},
		},
	}
	updated := &model.Order{ID: orderID, Resi: "RJK-ITEM9", Status: state.OrderMasuk, Subtotal: 80_000, Total: 80_000}
	store := &fakeStore{findOrders: []*model.Order{existing, updated}}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	// Swap display order only: B first, then A. No width/height/qty/price
	// changes at all.
	_, err := svc.EditOrder(context.Background(), "RJK-ITEM9", uuid.New(), EditOrderInput{
		Items: &[]EditOrderItemInput{
			{ID: &itemB, WidthCm: 100, HeightCm: 100, Quantity: 1, UnitPrice: 30_000},
			{ID: &itemA, WidthCm: 100, HeightCm: 100, Quantity: 1, UnitPrice: 50_000},
		},
	}, "tukar urutan tampil supaya sesuai urutan produksi")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if store.updateItemsCalls != 1 {
		t.Fatalf("reorder-only edit must actually persist (want 1 update items call), got %d — request was silently discarded", store.updateItemsCalls)
	}
	p := store.updateItemsParams
	byID := map[uuid.UUID]int{}
	for _, it := range p.UpsertItems {
		byID[it.ID] = it.LineNo
	}
	if byID[itemB] != 1 || byID[itemA] != 2 {
		t.Errorf("new line_no not persisted correctly: %+v, want {B:1, A:2}", byID)
	}
	if len(p.Audit.Changes) == 0 {
		t.Errorf("reorder must be recorded in the audit log, got empty changes")
	}
}

// TestEditOrder_SubtotalFieldEchoedUnchanged_NotRejected is the #1 fix
// regression guard: the admin frontend always echoes back the subtotal it
// fetched in EVERY PATCH body, even ones that only correct
// shipping_address — EditOrder must reject ONLY when the value differs from
// the order's current subtotal, not merely because the field is present.
func TestEditOrder_SubtotalFieldEchoedUnchanged_NotRejected(t *testing.T) {
	orderID := uuid.New()
	existing := &model.Order{
		ID: orderID, Resi: "RJK-ITEM7", Status: state.OrderMasuk,
		Subtotal: 100_000, Total: 100_000,
		ShippingAddress: strPtr("Jl. Lama"),
	}
	updated := &model.Order{
		ID: orderID, Resi: "RJK-ITEM7", Status: state.OrderMasuk,
		Subtotal: 100_000, Total: 100_000,
		ShippingAddress: strPtr("Jl. Baru"),
	}
	store := &fakeStore{findOrders: []*model.Order{existing, updated}}
	svc := New(store, &fakeCatalog{}, &fakeCustomers{})

	echoedSubtotal := int64(100_000) // SAMA dengan existing.Subtotal — bukan permintaan ubah
	newAddr := "Jl. Baru"
	got, err := svc.EditOrder(context.Background(), "RJK-ITEM7", uuid.New(), EditOrderInput{
		ShippingAddress: &newAddr,
		Subtotal:        &echoedSubtotal,
	}, "koreksi alamat, subtotal ikut ke-echo dari form")
	if err != nil {
		t.Fatalf("unexpected: %v (bug §1 — subtotal echoed unchanged must NOT be rejected)", err)
	}
	if got.ShippingAddress == nil || *got.ShippingAddress != "Jl. Baru" {
		t.Errorf("address not corrected: %+v", got.ShippingAddress)
	}
	if store.updateFieldsCalls != 1 {
		t.Errorf("want 1 update fields call, got %d", store.updateFieldsCalls)
	}
}
