package service

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/sitemedia/model"
	sitemediarepo "github.com/rajaku-printing/backend/internal/sitemedia/repository"
	"github.com/rajaku-printing/backend/internal/sitemedia/sitemediaapi"
)

// ---------- fakes ----------

type fakeStore struct {
	rows map[string]model.SiteMedia

	getErr    error
	listErr   error
	upsertErr error
	deleteErr error

	upsertCalls int
	deleteCalls int
}

func newFakeStore() *fakeStore {
	return &fakeStore{rows: map[string]model.SiteMedia{}}
}

func (f *fakeStore) Get(_ context.Context, slot string) (*model.SiteMedia, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	row, ok := f.rows[slot]
	if !ok {
		return nil, sitemediarepo.ErrNotFound
	}
	return &row, nil
}

func (f *fakeStore) List(_ context.Context) ([]model.SiteMedia, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	items := make([]model.SiteMedia, 0, len(f.rows))
	for _, row := range f.rows {
		items = append(items, row)
	}
	return items, nil
}

func (f *fakeStore) Upsert(_ context.Context, row *model.SiteMedia) error {
	f.upsertCalls++
	if f.upsertErr != nil {
		return f.upsertErr
	}
	f.rows[row.Slot] = *row
	return nil
}

func (f *fakeStore) Delete(_ context.Context, slot string) error {
	f.deleteCalls++
	if f.deleteErr != nil {
		return f.deleteErr
	}
	if _, ok := f.rows[slot]; !ok {
		return sitemediarepo.ErrNotFound
	}
	delete(f.rows, slot)
	return nil
}

type fakeBlobs struct {
	saved       map[string][]byte
	saveErr     error
	deleteCalls []string
	absErr      error
}

func newFakeBlobs() *fakeBlobs { return &fakeBlobs{saved: map[string][]byte{}} }

func (b *fakeBlobs) Save(_ context.Context, subpath string, r io.Reader) (int64, error) {
	if b.saveErr != nil {
		return 0, b.saveErr
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return 0, err
	}
	b.saved[subpath] = data
	return int64(len(data)), nil
}

func (b *fakeBlobs) Delete(_ context.Context, subpath string) error {
	b.deleteCalls = append(b.deleteCalls, subpath)
	delete(b.saved, subpath)
	return nil
}

func (b *fakeBlobs) AbsPath(subpath string) (string, error) {
	if b.absErr != nil {
		return "", b.absErr
	}
	return "/root/" + subpath, nil
}

// ---------- factories ----------

func newSvc() (*Service, *fakeStore, *fakeBlobs) {
	store := newFakeStore()
	blobs := newFakeBlobs()
	svc := New(store, blobs, Config{BaseURL: "https://rajaku.test"})
	svc.nowFn = func() time.Time { return time.Date(2026, 8, 18, 10, 0, 0, 0, time.UTC) }
	return svc, store, blobs
}

// tiny 4x4 red PNG — nativewebp will encode this to WebP.
func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func uploadInput(slot string, body []byte) UploadInput {
	return UploadInput{
		Slot:         slot,
		FileReader:   bytes.NewReader(body),
		FileSize:     int64(len(body)),
		MimeType:     "image/png",
		OriginalName: "gambar.png",
		UploaderID:   uuid.New(),
	}
}

// ---------- happy path ----------

func TestUpload_ValidSlot_SavesAndReturnsRow(t *testing.T) {
	svc, store, blobs := newSvc()
	body := tinyPNG(t)

	row, err := svc.Upload(context.Background(), uploadInput(string(model.SlotOGImage), body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if row.Slot != string(model.SlotOGImage) {
		t.Errorf("slot want %q, got %q", model.SlotOGImage, row.Slot)
	}
	if row.MimeType != "image/webp" {
		t.Errorf("mime want image/webp, got %q", row.MimeType)
	}
	if _, ok := store.rows[string(model.SlotOGImage)]; !ok {
		t.Error("row harus tersimpan di store")
	}
	if len(blobs.saved) != 1 {
		t.Errorf("want 1 blob tersimpan, got %d", len(blobs.saved))
	}

	// ListPublicMap harus memuat slot ini dengan URL yang dibangun dari BaseURL.
	m, err := svc.ListPublicMap(context.Background())
	if err != nil {
		t.Fatalf("ListPublicMap: %v", err)
	}
	wantURL := "https://rajaku.test/api/v1/site-media/file/" + string(model.SlotOGImage)
	if m[string(model.SlotOGImage)] != wantURL {
		t.Errorf("url want %q, got %q", wantURL, m[string(model.SlotOGImage)])
	}
}

func TestUpload_OverwritesSlot_DeletesOldBlobAfterDBUpdated(t *testing.T) {
	svc, store, blobs := newSvc()
	body := tinyPNG(t)

	first, err := svc.Upload(context.Background(), uploadInput(string(model.SlotHeroPosterDesktop), body))
	if err != nil {
		t.Fatalf("upload pertama: %v", err)
	}
	oldPath := first.StoragePath
	if _, ok := blobs.saved[oldPath]; !ok {
		t.Fatal("blob pertama harus tersimpan")
	}

	second, err := svc.Upload(context.Background(), uploadInput(string(model.SlotHeroPosterDesktop), body))
	if err != nil {
		t.Fatalf("upload kedua: %v", err)
	}
	if second.StoragePath == oldPath {
		t.Fatal("path kedua harus berbeda dari path pertama (unique per upload)")
	}
	// Blob lama harus terhapus SETELAH blob baru & DB diperbarui.
	if _, stillThere := blobs.saved[oldPath]; stillThere {
		t.Error("blob lama harus sudah dihapus setelah slot ditimpa")
	}
	if _, ok := blobs.saved[second.StoragePath]; !ok {
		t.Error("blob baru harus tersimpan")
	}
	// Hanya satu baris tersisa di DB untuk slot ini (bukan dua).
	if len(store.rows) != 1 {
		t.Errorf("want 1 row di store, got %d", len(store.rows))
	}
}

func TestDelete_ClearsSlotAndRemovesBlob(t *testing.T) {
	svc, store, blobs := newSvc()
	body := tinyPNG(t)
	row, err := svc.Upload(context.Background(), uploadInput(string(model.SlotBrandLogoMark), body))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	if err := svc.Delete(context.Background(), string(model.SlotBrandLogoMark)); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok := store.rows[string(model.SlotBrandLogoMark)]; ok {
		t.Error("row harus terhapus dari store")
	}
	found := false
	for _, p := range blobs.deleteCalls {
		if p == row.StoragePath {
			found = true
		}
	}
	if !found {
		t.Error("blob harus dihapus dari filestore")
	}

	// Slot sekarang kosong lagi di admin view.
	views, err := svc.ListAdmin(context.Background())
	if err != nil {
		t.Fatalf("ListAdmin: %v", err)
	}
	for _, v := range views {
		if v.Slot == string(model.SlotBrandLogoMark) && v.URL != nil {
			t.Error("slot yang sudah dihapus tidak boleh punya URL")
		}
	}
}

func TestListAdmin_IncludesEmptySlots(t *testing.T) {
	svc, _, _ := newSvc()
	views, err := svc.ListAdmin(context.Background())
	if err != nil {
		t.Fatalf("ListAdmin: %v", err)
	}
	if len(views) != len(model.Registry) {
		t.Fatalf("want %d slot registry, got %d", len(model.Registry), len(views))
	}
	for _, v := range views {
		if v.URL != nil {
			t.Errorf("slot %q belum diisi harusnya URL nil", v.Slot)
		}
	}
}

// ---------- failure path ----------

func TestUpload_RejectsUnknownSlot(t *testing.T) {
	svc, store, blobs := newSvc()
	body := tinyPNG(t)

	_, err := svc.Upload(context.Background(), uploadInput("slot_karangan", body))
	if !errors.Is(err, sitemediaapi.ErrUnknownSlot) {
		t.Fatalf("want ErrUnknownSlot, got %v", err)
	}
	if len(store.rows) != 0 || len(blobs.saved) != 0 {
		t.Error("slot tak dikenal tidak boleh menyentuh store/filestore")
	}
}

func TestUpload_RejectsInvalidMimeType(t *testing.T) {
	svc, _, _ := newSvc()
	in := uploadInput(string(model.SlotOGImage), []byte("not an image"))
	in.MimeType = "application/pdf"

	_, err := svc.Upload(context.Background(), in)
	if !errors.Is(err, sitemediaapi.ErrImageInvalidType) {
		t.Fatalf("want ErrImageInvalidType, got %v", err)
	}
}

func TestUpload_RejectsOversizedFile(t *testing.T) {
	svc, _, _ := newSvc()
	in := uploadInput(string(model.SlotOGImage), tinyPNG(t))
	in.FileSize = MaxUploadBytes + 1

	_, err := svc.Upload(context.Background(), in)
	if !errors.Is(err, sitemediaapi.ErrImageTooLarge) {
		t.Fatalf("want ErrImageTooLarge, got %v", err)
	}
}

func TestUpload_RejectsCorruptImage(t *testing.T) {
	svc, _, _ := newSvc()
	in := uploadInput(string(model.SlotOGImage), []byte("bukan gambar sama sekali"))

	_, err := svc.Upload(context.Background(), in)
	if !errors.Is(err, sitemediaapi.ErrImageDecodeFailed) {
		t.Fatalf("want ErrImageDecodeFailed, got %v", err)
	}
}

func TestDelete_UnknownSlotRejected(t *testing.T) {
	svc, _, _ := newSvc()
	err := svc.Delete(context.Background(), "slot_karangan")
	if !errors.Is(err, sitemediaapi.ErrUnknownSlot) {
		t.Fatalf("want ErrUnknownSlot, got %v", err)
	}
}

func TestDelete_EmptySlotReturnsErrSlotEmpty(t *testing.T) {
	svc, _, _ := newSvc()
	err := svc.Delete(context.Background(), string(model.SlotOGImage))
	if !errors.Is(err, sitemediaapi.ErrSlotEmpty) {
		t.Fatalf("want ErrSlotEmpty, got %v", err)
	}
}

func TestGetFile_EmptySlotReturnsErrSlotEmpty(t *testing.T) {
	svc, _, _ := newSvc()
	_, err := svc.GetFile(context.Background(), string(model.SlotProses1))
	if !errors.Is(err, sitemediaapi.ErrSlotEmpty) {
		t.Fatalf("want ErrSlotEmpty, got %v", err)
	}
}
