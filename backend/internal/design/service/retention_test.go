package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/design/model"
	"github.com/rajaku-printing/backend/internal/notification/notificationapi"
	"github.com/rajaku-printing/backend/internal/order/orderapi"
	"github.com/rajaku-printing/backend/internal/pkg/filestore"
	"github.com/rajaku-printing/backend/internal/settings/settingsapi"
)

// ---------- fakes khusus retention ----------

type fakeSettings struct {
	days     int
	err      error
	lastKey  string
	getCalls int
}

func (f *fakeSettings) GetInt(_ context.Context, key string) (int, error) {
	f.getCalls++
	f.lastKey = key
	if f.err != nil {
		return 0, f.err
	}
	return f.days, nil
}

type alertCall struct {
	kind     notificationapi.Kind
	orderID  *uuid.UUID
	extras   map[string]any
	dedupKey string
}

type fakeAlerter struct {
	calls []alertCall
	err   error
}

func (f *fakeAlerter) EnqueueInternalAlert(
	_ context.Context,
	kind notificationapi.Kind,
	orderID *uuid.UUID,
	extras map[string]any,
	dedupKey string,
) error {
	f.calls = append(f.calls, alertCall{kind: kind, orderID: orderID, extras: extras, dedupKey: dedupKey})
	return f.err
}

// ---------- helpers ----------

var testNow = time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)

func designFile(uploadedDaysAgo int, size int64) model.DesignFile {
	id := uuid.New()
	return model.DesignFile{
		ID:               id,
		OrderID:          uuid.New(),
		Role:             model.RoleCustomerUpload,
		FilePath:         "design/" + id.String() + ".pdf",
		FileOriginalName: "banner-final.pdf",
		FileSizeBytes:    size,
		UploadedAt:       testNow.AddDate(0, 0, -uploadedDaysAgo),
	}
}

// ---------- sweep: happy path ----------

func TestRunRetentionSweep_PurgesExpiredBlobs(t *testing.T) {
	a := designFile(40, 1_000)
	b := designFile(35, 2_500)

	store := &fakeStore{purgeCandidates: []model.DesignFile{a, b}}
	blobs := &fakeBlobs{}
	svc := newSvc(store, blobs, &fakeOrderCmd{})
	svc.SetSettingsReader(&fakeSettings{days: 30})

	res, err := svc.RunRetentionSweep(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Scanned != 2 || res.Purged != 2 || res.Failed != 0 {
		t.Errorf("want scanned=2 purged=2 failed=0, got %+v", res)
	}
	if res.FreedBytes != 3_500 {
		t.Errorf("freed bytes want 3500, got %d", res.FreedBytes)
	}
	if len(blobs.deleted) != 2 {
		t.Errorf("want 2 blob deletes, got %d", len(blobs.deleted))
	}
	if len(store.markPurgedIDs) != 2 {
		t.Errorf("want 2 rows marked purged, got %d", len(store.markPurgedIDs))
	}
	// Cutoff harus tepat now - retensi; salah tanda di sini artinya file baru
	// ikut kehapus (bug paling mahal di modul ini).
	wantCutoff := testNow.AddDate(0, 0, -30)
	if !store.purgeCutoff.Equal(wantCutoff) {
		t.Errorf("cutoff want %s, got %s", wantCutoff, store.purgeCutoff)
	}
}

func TestRunRetentionSweep_UsesSettingValueNotDefault(t *testing.T) {
	store := &fakeStore{}
	settings := &fakeSettings{days: 7}
	svc := newSvc(store, &fakeBlobs{}, &fakeOrderCmd{})
	svc.SetSettingsReader(settings)

	res, err := svc.RunRetentionSweep(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.RetentionDays != 7 {
		t.Errorf("retention days want 7 (dari setting), got %d", res.RetentionDays)
	}
	if settings.lastKey != settingsapi.KeyDesignRetentionDays {
		t.Errorf("key want %q, got %q", settingsapi.KeyDesignRetentionDays, settings.lastKey)
	}
	if want := testNow.AddDate(0, 0, -7); !store.purgeCutoff.Equal(want) {
		t.Errorf("cutoff want %s, got %s", want, store.purgeCutoff)
	}
}

func TestRunRetentionSweep_FallsBackWhenSettingUnreadable(t *testing.T) {
	store := &fakeStore{}
	svc := newSvc(store, &fakeBlobs{}, &fakeOrderCmd{})
	svc.retentionDefaultDays = 45
	svc.SetSettingsReader(&fakeSettings{err: settingsapi.ErrSettingNotFound})

	res, err := svc.RunRetentionSweep(context.Background())
	if err != nil {
		t.Fatalf("setting hilang tidak boleh bikin sweep gagal, got %v", err)
	}
	if res.RetentionDays != 45 {
		t.Errorf("want fallback 45 hari, got %d", res.RetentionDays)
	}
}

// ---------- sweep: failure path ----------

func TestRunRetentionSweep_BlobDeleteFails_RowStaysUnpurged(t *testing.T) {
	f := designFile(40, 900)
	store := &fakeStore{purgeCandidates: []model.DesignFile{f}}
	blobs := &fakeBlobs{deleteErr: errors.New("permission denied")}
	svc := newSvc(store, blobs, &fakeOrderCmd{})
	svc.SetSettingsReader(&fakeSettings{days: 30})

	res, err := svc.RunRetentionSweep(context.Background())
	if err != nil {
		t.Fatalf("kegagalan per-file tidak boleh menggagalkan batch, got %v", err)
	}
	if res.Failed != 1 || res.Purged != 0 {
		t.Errorf("want failed=1 purged=0, got %+v", res)
	}
	// Kritis: row TIDAK boleh ditandai purged kalau blob-nya masih ada —
	// kalau ditandai, file jadi sampah permanen yang tidak pernah di-scan lagi.
	if store.markPurgedCalls != 0 {
		t.Errorf("row tidak boleh ditandai purged saat delete blob gagal, markPurged dipanggil %d kali", store.markPurgedCalls)
	}
	if res.FreedBytes != 0 {
		t.Errorf("freed bytes want 0, got %d", res.FreedBytes)
	}
}

func TestRunRetentionSweep_MissingBlobStillMarksPurged(t *testing.T) {
	f := designFile(40, 900)
	store := &fakeStore{purgeCandidates: []model.DesignFile{f}}
	blobs := &fakeBlobs{deleteErr: filestore.ErrNotFound}
	svc := newSvc(store, blobs, &fakeOrderCmd{})
	svc.SetSettingsReader(&fakeSettings{days: 30})

	res, err := svc.RunRetentionSweep(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Blob sudah hilang duluan (mis. dihapus manual) — tetap tandai purged
	// supaya tidak ke-scan berulang selamanya.
	if res.Purged != 1 || res.Failed != 0 {
		t.Errorf("want purged=1 failed=0, got %+v", res)
	}
}

func TestRunRetentionSweep_ListCandidatesError(t *testing.T) {
	store := &fakeStore{purgeCandidatesErr: errors.New("db down")}
	svc := newSvc(store, &fakeBlobs{}, &fakeOrderCmd{})
	svc.SetSettingsReader(&fakeSettings{days: 30})

	if _, err := svc.RunRetentionSweep(context.Background()); err == nil {
		t.Fatal("kegagalan level-batch harus dikembalikan sebagai error")
	}
}

// ---------- reminder: happy path ----------

func TestRunRetentionReminder_NotifiesWhenOrderUnfinished(t *testing.T) {
	f := designFile(28, 500)
	store := &fakeStore{reminderCandidates: []model.DesignFile{f}}
	alerter := &fakeAlerter{}
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID:     f.OrderID,
		Resi:   "RJK-8F3K2A9X",
		Status: "proses_cetak",
	}}

	svc := newSvc(store, &fakeBlobs{}, cmd)
	svc.SetSettingsReader(&fakeSettings{days: 30})
	svc.SetInternalAlerter(alerter)

	res, err := svc.RunRetentionReminder(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Notified != 1 || res.Skipped != 0 || res.Failed != 0 {
		t.Errorf("want notified=1 skipped=0 failed=0, got %+v", res)
	}
	if len(alerter.calls) != 1 {
		t.Fatalf("want 1 alert, got %d", len(alerter.calls))
	}
	call := alerter.calls[0]
	if call.kind != notificationapi.KindDesignRetentionWarning {
		t.Errorf("kind want %s, got %s", notificationapi.KindDesignRetentionWarning, call.kind)
	}
	if call.dedupKey == "" {
		t.Error("dedup key wajib diisi supaya reminder tidak dobel")
	}
	if got := call.extras["days_left"]; got != 2 {
		// upload 28 hari lalu, retensi 30 → sisa 2 hari.
		t.Errorf("days_left want 2, got %v", got)
	}
	if len(store.reminderSentIDs) != 1 || store.reminderSentIDs[0] != f.ID {
		t.Errorf("file harus ditandai sudah diingatkan, got %v", store.reminderSentIDs)
	}
	// Jendela kandidat: [now-30d, now-27d) — file yang sudah lewat retensi
	// urusan sweep, bukan reminder.
	wantFrom := testNow.AddDate(0, 0, -30)
	wantTo := testNow.AddDate(0, 0, -27)
	if !store.reminderWindow[0].Equal(wantFrom) || !store.reminderWindow[1].Equal(wantTo) {
		t.Errorf("window want [%s, %s), got [%s, %s)", wantFrom, wantTo, store.reminderWindow[0], store.reminderWindow[1])
	}
}

func TestRunRetentionReminder_SkipsFinishedOrder(t *testing.T) {
	f := designFile(28, 500)
	store := &fakeStore{reminderCandidates: []model.DesignFile{f}}
	alerter := &fakeAlerter{}
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID:     f.OrderID,
		Resi:   "RJK-DONE0001",
		Status: "selesai",
	}}

	svc := newSvc(store, &fakeBlobs{}, cmd)
	svc.SetSettingsReader(&fakeSettings{days: 30})
	svc.SetInternalAlerter(alerter)

	res, err := svc.RunRetentionReminder(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Skipped != 1 || res.Notified != 0 {
		t.Errorf("want skipped=1 notified=0, got %+v", res)
	}
	if len(alerter.calls) != 0 {
		t.Errorf("order selesai tidak perlu reminder, tapi %d alert terkirim", len(alerter.calls))
	}
	// Sengaja TIDAK ditandai: kalau order dibuka lagi (komplain), run
	// berikutnya masih boleh mengingatkan selama file belum kehapus.
	if len(store.reminderSentIDs) != 0 {
		t.Errorf("order selesai tidak boleh ditandai reminded, got %v", store.reminderSentIDs)
	}
}

// ---------- reminder: failure path ----------

func TestRunRetentionReminder_StopsWhenInternalPhoneUnset(t *testing.T) {
	store := &fakeStore{reminderCandidates: []model.DesignFile{designFile(28, 1), designFile(29, 1)}}
	alerter := &fakeAlerter{err: notificationapi.ErrInternalRecipientMissing}
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{Resi: "RJK-X", Status: "qc"}}

	svc := newSvc(store, &fakeBlobs{}, cmd)
	svc.SetSettingsReader(&fakeSettings{days: 30})
	svc.SetInternalAlerter(alerter)

	res, err := svc.RunRetentionReminder(context.Background())
	if err != nil {
		t.Fatalf("nomor ops belum di-set bukan error fatal, got %v", err)
	}
	if res.Notified != 0 {
		t.Errorf("want notified=0, got %d", res.Notified)
	}
	// Berhenti setelah kegagalan pertama — sisanya pasti gagal dgn alasan sama.
	if len(alerter.calls) != 1 {
		t.Errorf("want berhenti setelah 1 percobaan, got %d", len(alerter.calls))
	}
	if len(store.reminderSentIDs) != 0 {
		t.Errorf("tidak boleh menandai reminded saat alert gagal, got %v", store.reminderSentIDs)
	}
}

func TestRunRetentionReminder_OrderLookupFails(t *testing.T) {
	store := &fakeStore{reminderCandidates: []model.DesignFile{designFile(28, 1)}}
	alerter := &fakeAlerter{}
	cmd := &fakeOrderCmd{summaryErr: orderapi.ErrOrderNotFound}

	svc := newSvc(store, &fakeBlobs{}, cmd)
	svc.SetSettingsReader(&fakeSettings{days: 30})
	svc.SetInternalAlerter(alerter)

	res, err := svc.RunRetentionReminder(context.Background())
	if err != nil {
		t.Fatalf("kegagalan per-file tidak boleh menggagalkan batch, got %v", err)
	}
	if res.Failed != 1 || res.Notified != 0 {
		t.Errorf("want failed=1 notified=0, got %+v", res)
	}
	if len(alerter.calls) != 0 {
		t.Errorf("tidak boleh kirim alert tanpa konteks order, got %d", len(alerter.calls))
	}
}

func TestRunRetentionReminder_NoAlerterConfigured(t *testing.T) {
	store := &fakeStore{reminderCandidates: []model.DesignFile{designFile(28, 1)}}
	svc := newSvc(store, &fakeBlobs{}, &fakeOrderCmd{})
	svc.SetSettingsReader(&fakeSettings{days: 30})

	res, err := svc.RunRetentionReminder(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Scanned != 0 || res.Notified != 0 {
		t.Errorf("tanpa alerter, reminder harus no-op, got %+v", res)
	}
}
