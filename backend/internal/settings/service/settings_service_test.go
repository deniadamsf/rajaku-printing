package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/settings/model"
	settingsrepo "github.com/rajaku-printing/backend/internal/settings/repository"
	"github.com/rajaku-printing/backend/internal/settings/settingsapi"
)

type fakeStore struct {
	row        *model.AppSetting
	getErr     error
	updateErr  error
	updated    []updateCall
	listResult []model.AppSetting
	listErr    error
}

type updateCall struct {
	key   string
	value string
	actor *uuid.UUID
}

func (f *fakeStore) Get(_ context.Context, _ string) (*model.AppSetting, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.row, nil
}

func (f *fakeStore) List(_ context.Context) ([]model.AppSetting, error) {
	return f.listResult, f.listErr
}

func (f *fakeStore) Update(_ context.Context, key, value string, actor *uuid.UUID, _ time.Time) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	f.updated = append(f.updated, updateCall{key: key, value: value, actor: actor})
	if f.row != nil {
		f.row.Value = value
	}
	return nil
}

func newTestSvc(store *fakeStore) *Service {
	s := New(store)
	s.nowFn = func() time.Time { return time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC) }
	return s
}

// ---------- happy path ----------

func TestGetInt_ReturnsStoredValue(t *testing.T) {
	svc := newTestSvc(&fakeStore{row: &model.AppSetting{
		Key:   settingsapi.KeyDesignRetentionDays,
		Value: "45",
	}})

	got, err := svc.GetInt(context.Background(), settingsapi.KeyDesignRetentionDays)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 45 {
		t.Errorf("want 45, got %d", got)
	}
}

func TestUpdate_PersistsValidValue(t *testing.T) {
	store := &fakeStore{row: &model.AppSetting{
		Key:   settingsapi.KeyDesignRetentionDays,
		Value: "30",
	}}
	svc := newTestSvc(store)
	actor := uuid.New()

	row, err := svc.Update(context.Background(), settingsapi.KeyDesignRetentionDays, " 14 ", actor)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if row.Value != "14" {
		t.Errorf("value want 14, got %q", row.Value)
	}
	if len(store.updated) != 1 {
		t.Fatalf("want 1 update call, got %d", len(store.updated))
	}
	if store.updated[0].actor == nil || *store.updated[0].actor != actor {
		t.Error("actor harus tercatat untuk audit siapa yang ubah kebijakan")
	}
}

// ---------- failure path ----------

func TestUpdate_RejectsNonInteger(t *testing.T) {
	store := &fakeStore{row: &model.AppSetting{Key: settingsapi.KeyDesignRetentionDays, Value: "30"}}
	svc := newTestSvc(store)

	_, err := svc.Update(context.Background(), settingsapi.KeyDesignRetentionDays, "tigapuluh", uuid.New())
	if !errors.Is(err, settingsapi.ErrInvalidValue) {
		t.Fatalf("want ErrInvalidValue, got %v", err)
	}
	if len(store.updated) != 0 {
		t.Error("nilai invalid tidak boleh sampai ke store")
	}
}

func TestUpdate_RejectsOutOfRange(t *testing.T) {
	store := &fakeStore{row: &model.AppSetting{Key: settingsapi.KeyDesignRetentionDays, Value: "30"}}
	svc := newTestSvc(store)

	// 0 hari = file kehapus seketika setelah upload; harus ditolak.
	if _, err := svc.Update(context.Background(), settingsapi.KeyDesignRetentionDays, "0", uuid.New()); !errors.Is(err, settingsapi.ErrInvalidValue) {
		t.Errorf("retensi 0 hari harus ditolak, got %v", err)
	}
	if _, err := svc.Update(context.Background(), settingsapi.KeyDesignRetentionDays, "9999", uuid.New()); !errors.Is(err, settingsapi.ErrInvalidValue) {
		t.Errorf("retensi 9999 hari harus ditolak, got %v", err)
	}
	if len(store.updated) != 0 {
		t.Error("nilai di luar rentang tidak boleh sampai ke store")
	}
}

// ---------- pos.receipt_width_mm (§11/§12 — enum 58/80) ----------

func TestUpdate_AcceptsAllowedReceiptWidths(t *testing.T) {
	for _, value := range []string{"58", "80"} {
		store := &fakeStore{row: &model.AppSetting{Key: settingsapi.KeyPOSReceiptWidthMM, Value: "58"}}
		svc := newTestSvc(store)

		row, err := svc.Update(context.Background(), settingsapi.KeyPOSReceiptWidthMM, value, uuid.New())
		if err != nil {
			t.Fatalf("value %q should be accepted, got error: %v", value, err)
		}
		if row.Value != value {
			t.Errorf("want value %q, got %q", value, row.Value)
		}
	}
}

func TestUpdate_RejectsDisallowedReceiptWidth(t *testing.T) {
	store := &fakeStore{row: &model.AppSetting{Key: settingsapi.KeyPOSReceiptWidthMM, Value: "58"}}
	svc := newTestSvc(store)

	_, err := svc.Update(context.Background(), settingsapi.KeyPOSReceiptWidthMM, "70", uuid.New())
	if !errors.Is(err, settingsapi.ErrInvalidValue) {
		t.Fatalf("want ErrInvalidValue for 70mm, got %v", err)
	}
	if len(store.updated) != 0 {
		t.Error("nilai 70mm tidak boleh sampai ke store")
	}
}

func TestUpdate_RejectsNonIntegerReceiptWidth(t *testing.T) {
	store := &fakeStore{row: &model.AppSetting{Key: settingsapi.KeyPOSReceiptWidthMM, Value: "58"}}
	svc := newTestSvc(store)

	_, err := svc.Update(context.Background(), settingsapi.KeyPOSReceiptWidthMM, "abc", uuid.New())
	if !errors.Is(err, settingsapi.ErrInvalidValue) {
		t.Fatalf("want ErrInvalidValue for non-integer, got %v", err)
	}
}

func TestUpdate_RejectsUnknownKey(t *testing.T) {
	store := &fakeStore{}
	svc := newTestSvc(store)

	_, err := svc.Update(context.Background(), "retensi_karangan", "10", uuid.New())
	if !errors.Is(err, settingsapi.ErrUnknownKey) {
		t.Fatalf("want ErrUnknownKey, got %v", err)
	}
}

func TestGetInt_MissingRowSurfacesError(t *testing.T) {
	svc := newTestSvc(&fakeStore{getErr: settingsrepo.ErrNotFound})

	_, err := svc.GetInt(context.Background(), settingsapi.KeyDesignRetentionDays)
	if !errors.Is(err, settingsapi.ErrSettingNotFound) {
		t.Fatalf("want ErrSettingNotFound, got %v", err)
	}
}

func TestGetInt_CorruptValueSurfacesError(t *testing.T) {
	svc := newTestSvc(&fakeStore{row: &model.AppSetting{
		Key:   settingsapi.KeyDesignRetentionDays,
		Value: "dua puluh",
	}})

	_, err := svc.GetInt(context.Background(), settingsapi.KeyDesignRetentionDays)
	if !errors.Is(err, settingsapi.ErrInvalidValue) {
		t.Fatalf("want ErrInvalidValue, got %v", err)
	}
}

// ---------- payment settings (§7) ----------

// paymentRows — 6 row payment.* seperti hasil migration 000020, dipakai
// beberapa test GetPaymentInfo di bawah.
func paymentRows() map[string]*model.AppSetting {
	return map[string]*model.AppSetting{
		settingsapi.KeyPaymentBankName:         {Key: settingsapi.KeyPaymentBankName, Value: "BCA"},
		settingsapi.KeyPaymentAccountName:      {Key: settingsapi.KeyPaymentAccountName, Value: "CV WANSHOU NIAGA UTAMA"},
		settingsapi.KeyPaymentAccountNumber:    {Key: settingsapi.KeyPaymentAccountNumber, Value: "3245070777"},
		settingsapi.KeyPaymentQRISNote:         {Key: settingsapi.KeyPaymentQRISNote, Value: "Pindai QRIS lalu unggah bukti bayar."},
		settingsapi.KeyPaymentQRISMerchantName: {Key: settingsapi.KeyPaymentQRISMerchantName, Value: "EVENT WOWINFOOD"},
		settingsapi.KeyPaymentQRISNmid:         {Key: settingsapi.KeyPaymentQRISNmid, Value: "ID2026569993978"},
	}
}

// keyedStore — fakeStore variant yang menjawab Get per key (dipakai
// GetPaymentInfo yang query 6 key berbeda, beda dari fakeStore.row tunggal
// yang dipakai test lain di atas).
type keyedStore struct {
	fakeStore
	rows map[string]*model.AppSetting
}

func (k *keyedStore) Get(_ context.Context, key string) (*model.AppSetting, error) {
	row, ok := k.rows[key]
	if !ok {
		return nil, settingsrepo.ErrNotFound
	}
	return row, nil
}

func newKeyedTestSvc(store *keyedStore) *Service {
	s := New(store)
	s.nowFn = func() time.Time { return time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC) }
	return s
}

func TestGetPaymentInfo_MapsAllSixKeys(t *testing.T) {
	store := &keyedStore{rows: paymentRows()}
	svc := newKeyedTestSvc(store)

	info, err := svc.GetPaymentInfo(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.BankName != "BCA" {
		t.Errorf("bank_name want BCA, got %q", info.BankName)
	}
	if info.AccountName != "CV WANSHOU NIAGA UTAMA" {
		t.Errorf("account_name salah: %q", info.AccountName)
	}
	if info.AccountNumber != "3245070777" {
		t.Errorf("account_number salah: %q", info.AccountNumber)
	}
	if info.QRISMerchantName != "EVENT WOWINFOOD" {
		t.Errorf("qris_merchant_name salah: %q", info.QRISMerchantName)
	}
	if info.QRISNmid != "ID2026569993978" {
		t.Errorf("qris_nmid salah: %q", info.QRISNmid)
	}
	if info.QRISNote == "" {
		t.Error("qris_note tidak boleh kosong pada test ini (row-nya terisi)")
	}
}

func TestGetPaymentInfo_MissingRequiredKeySurfacesError(t *testing.T) {
	rows := paymentRows()
	delete(rows, settingsapi.KeyPaymentAccountNumber) // simulasikan migration belum jalan
	store := &keyedStore{rows: rows}
	svc := newKeyedTestSvc(store)

	_, err := svc.GetPaymentInfo(context.Background())
	if err == nil {
		t.Fatal("want error ketika key wajib hilang dari DB, got nil")
	}
}

func TestUpdate_NormalizesAccountNumber(t *testing.T) {
	store := &fakeStore{row: &model.AppSetting{Key: settingsapi.KeyPaymentAccountNumber, Value: "3245070777"}}
	svc := newTestSvc(store)

	row, err := svc.Update(context.Background(), settingsapi.KeyPaymentAccountNumber, "3245 070-777", uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if row.Value != "3245070777" {
		t.Errorf("account_number want ternormalisasi 3245070777, got %q", row.Value)
	}
}

func TestUpdate_RejectsAccountNumberWithLetters(t *testing.T) {
	store := &fakeStore{row: &model.AppSetting{Key: settingsapi.KeyPaymentAccountNumber, Value: "3245070777"}}
	svc := newTestSvc(store)

	_, err := svc.Update(context.Background(), settingsapi.KeyPaymentAccountNumber, "3245O70777", uuid.New())
	if !errors.Is(err, settingsapi.ErrInvalidValue) {
		t.Fatalf("want ErrInvalidValue, got %v", err)
	}
	if len(store.updated) != 0 {
		t.Error("nomor rekening berisi huruf tidak boleh sampai ke store")
	}
}

func TestUpdate_RejectsEmptyRequiredPaymentFields(t *testing.T) {
	for _, key := range []string{
		settingsapi.KeyPaymentBankName,
		settingsapi.KeyPaymentAccountName,
		settingsapi.KeyPaymentAccountNumber,
	} {
		store := &fakeStore{row: &model.AppSetting{Key: key, Value: "isi lama"}}
		svc := newTestSvc(store)

		_, err := svc.Update(context.Background(), key, "   ", uuid.New())
		if !errors.Is(err, settingsapi.ErrInvalidValue) {
			t.Errorf("%s: want ErrInvalidValue untuk nilai kosong, got %v", key, err)
		}
		if len(store.updated) != 0 {
			t.Errorf("%s: nilai kosong tidak boleh sampai ke store", key)
		}
	}
}

// ---------- List (§ tugas C — allowed_values proyeksi enum) ----------

func TestList_EnumKeyCarriesAllowedValues(t *testing.T) {
	store := &fakeStore{listResult: []model.AppSetting{
		{Key: settingsapi.KeyPOSReceiptWidthMM, Value: "58", DisplayName: "Lebar Struk"},
		{Key: settingsapi.KeyDesignRetentionDays, Value: "30", DisplayName: "Retensi Desain"},
	}}
	svc := newTestSvc(store)

	items, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("want 2 items, got %d", len(items))
	}

	receipt := items[0]
	if receipt.Key != settingsapi.KeyPOSReceiptWidthMM {
		t.Fatalf("unexpected order, item[0].Key=%q", receipt.Key)
	}
	if len(receipt.AllowedValues) != 2 || receipt.AllowedValues[0] != "58" || receipt.AllowedValues[1] != "80" {
		t.Errorf("pos.receipt_width_mm allowed_values want [58 80], got %v", receipt.AllowedValues)
	}

	retention := items[1]
	if retention.Key != settingsapi.KeyDesignRetentionDays {
		t.Fatalf("unexpected order, item[1].Key=%q", retention.Key)
	}
	if retention.AllowedValues != nil {
		t.Errorf("design_retention_days harus TIDAK membawa allowed_values (free-form range), got %v", retention.AllowedValues)
	}
}

func TestUpdate_AcceptsEmptyQRISFields(t *testing.T) {
	for _, key := range []string{
		settingsapi.KeyPaymentQRISNote,
		settingsapi.KeyPaymentQRISMerchantName,
		settingsapi.KeyPaymentQRISNmid,
	} {
		store := &fakeStore{row: &model.AppSetting{Key: key, Value: "isi lama"}}
		svc := newTestSvc(store)

		row, err := svc.Update(context.Background(), key, "  ", uuid.New())
		if err != nil {
			t.Errorf("%s: QRIS boleh kosong, unexpected error: %v", key, err)
			continue
		}
		if row.Value != "" {
			t.Errorf("%s: want value kosong, got %q", key, row.Value)
		}
	}
}
