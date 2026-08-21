package service

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/catalog/model"
	"github.com/rajaku-printing/backend/internal/catalog/repository"
)

func TestValidateProductImageInput_Valid(t *testing.T) {
	in := ProductImageInput{
		FileReader: bytes.NewReader([]byte("fake-bytes")),
		FileSize:   1024,
		MimeType:   "image/png",
	}
	if err := validateProductImageInput(in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateProductImageInput_RejectsEmpty(t *testing.T) {
	in := ProductImageInput{FileSize: 0, MimeType: "image/png"}
	if err := validateProductImageInput(in); !errors.Is(err, ErrImageEmpty) {
		t.Fatalf("want ErrImageEmpty, got %v", err)
	}
}

func TestValidateProductImageInput_RejectsOversized(t *testing.T) {
	in := ProductImageInput{FileSize: MaxProductImageUploadBytes + 1, MimeType: "image/png"}
	if err := validateProductImageInput(in); !errors.Is(err, ErrImageTooLarge) {
		t.Fatalf("want ErrImageTooLarge, got %v", err)
	}
}

func TestValidateProductImageInput_RejectsInvalidMime(t *testing.T) {
	in := ProductImageInput{FileSize: 1024, MimeType: "application/pdf"}
	if err := validateProductImageInput(in); !errors.Is(err, ErrImageInvalidType) {
		t.Fatalf("want ErrImageInvalidType, got %v", err)
	}
}

func TestNewProductImageSubpath_RandomPerCall(t *testing.T) {
	id := uuid.New()
	a := newProductImageSubpath(id)
	b := newProductImageSubpath(id)
	if a == b {
		t.Fatalf("subpath harus UUID acak per unggahan, dapat sama: %q", a)
	}
	prefix := "product_images/" + id.String() + "/"
	if !strings.HasPrefix(a, prefix) || !strings.HasSuffix(a, ".webp") {
		t.Fatalf("subpath want prefix %q + suffix .webp, got %q", prefix, a)
	}
}

func TestProductImageURL_BuiltFromBaseURL(t *testing.T) {
	svc := &Service{baseURL: "https://rajaku.test"}
	id := uuid.New()
	want := "https://rajaku.test/api/v1/catalog/product-images/" + id.String()
	if got := svc.ProductImageURL(id); got != want {
		t.Fatalf("url want %q got %q", want, got)
	}
}

// ---- fakes untuk jalur ganti-gambar (upload) -----------------------------

type fakeProductStore struct {
	product   *model.Product
	updateErr error

	lastUpdateID    uuid.UUID
	lastUpdatePatch map[string]any
}

func (f *fakeProductStore) ListActive(context.Context) ([]model.Product, error) { return nil, nil }
func (f *fakeProductStore) ListAll(context.Context) ([]model.Product, error)    { return nil, nil }
func (f *fakeProductStore) FindBySlug(context.Context, string) (*model.Product, error) {
	return nil, nil
}
func (f *fakeProductStore) FindByID(_ context.Context, id uuid.UUID) (*model.Product, error) {
	if f.product == nil {
		return nil, repository.ErrNotFound
	}
	cp := *f.product
	return &cp, nil
}
func (f *fakeProductStore) FindByIDWithPricings(ctx context.Context, id uuid.UUID) (*model.Product, error) {
	return f.FindByID(ctx, id)
}
func (f *fakeProductStore) Create(context.Context, *model.Product) error { return nil }
func (f *fakeProductStore) Update(_ context.Context, id uuid.UUID, patch map[string]any) error {
	f.lastUpdateID = id
	f.lastUpdatePatch = patch
	if f.updateErr != nil {
		return f.updateErr
	}
	if v, ok := patch["image_path"]; ok {
		if v == nil {
			f.product.ImagePath = nil
		} else {
			s := v.(string)
			f.product.ImagePath = &s
		}
	}
	return nil
}
func (f *fakeProductStore) SetActive(context.Context, uuid.UUID, bool) error { return nil }
func (f *fakeProductStore) HasAnyPricings(context.Context, uuid.UUID) (bool, error) {
	return false, nil
}

var _ productStore = (*fakeProductStore)(nil)

type fakeBlobStore struct {
	saveErr   error
	deleteErr error

	saved   map[string][]byte
	deleted []string
}

func newFakeBlobStore() *fakeBlobStore {
	return &fakeBlobStore{saved: make(map[string][]byte)}
}

func (f *fakeBlobStore) Save(_ context.Context, subpath string, r io.Reader) (int64, error) {
	if f.saveErr != nil {
		return 0, f.saveErr
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return 0, err
	}
	f.saved[subpath] = data
	return int64(len(data)), nil
}

func (f *fakeBlobStore) Delete(_ context.Context, subpath string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	delete(f.saved, subpath)
	f.deleted = append(f.deleted, subpath)
	return nil
}

func (f *fakeBlobStore) AbsPath(subpath string) (string, error) {
	return "/blobs/" + subpath, nil
}

var _ FileStore = (*fakeBlobStore)(nil)

// tinyPNG — 1x1 pixel PNG valid ter-generate via image/png (bukan bytes
// tangan), cukup untuk lolos webp.Encode.
func tinyPNGBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 200, G: 30, B: 30, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("gagal generate tiny PNG untuk fixture test: %v", err)
	}
	return buf.Bytes()
}

func TestAdminUploadProductImage_Success_SwapsPathAndDeletesOld(t *testing.T) {
	productID := uuid.New()
	oldPath := "product_images/" + productID.String() + "/old.webp"
	store := &fakeProductStore{product: &model.Product{ID: productID, ImagePath: &oldPath}}
	blobs := newFakeBlobStore()
	blobs.saved[oldPath] = []byte("old-webp-bytes")

	svc := &Service{products: store, blobs: blobs, baseURL: "https://rajaku.test"}
	pngBytes := tinyPNGBytes(t)

	got, err := svc.AdminUploadProductImage(context.Background(), productID, ProductImageInput{
		FileReader: bytes.NewReader(pngBytes),
		FileSize:   int64(len(pngBytes)),
		MimeType:   "image/png",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ImagePath == nil || *got.ImagePath == oldPath {
		t.Fatalf("want new image_path different from old, got %v", got.ImagePath)
	}
	if _, stillThere := blobs.saved[oldPath]; stillThere {
		t.Fatalf("blob lama %q seharusnya sudah dihapus setelah DB sukses", oldPath)
	}
	if len(blobs.deleted) != 1 || blobs.deleted[0] != oldPath {
		t.Fatalf("want blob lama %q dihapus tepat sekali, got %v", oldPath, blobs.deleted)
	}
	if _, newThere := blobs.saved[*got.ImagePath]; !newThere {
		t.Fatalf("blob baru %q seharusnya tersimpan", *got.ImagePath)
	}
}

func TestAdminUploadProductImage_DBFailure_CleansNewBlobKeepsOld(t *testing.T) {
	productID := uuid.New()
	oldPath := "product_images/" + productID.String() + "/old.webp"
	store := &fakeProductStore{
		product:   &model.Product{ID: productID, ImagePath: &oldPath},
		updateErr: errors.New("db unavailable"),
	}
	blobs := newFakeBlobStore()
	blobs.saved[oldPath] = []byte("old-webp-bytes")

	svc := &Service{products: store, blobs: blobs, baseURL: "https://rajaku.test"}
	pngBytes := tinyPNGBytes(t)

	_, err := svc.AdminUploadProductImage(context.Background(), productID, ProductImageInput{
		FileReader: bytes.NewReader(pngBytes),
		FileSize:   int64(len(pngBytes)),
		MimeType:   "image/png",
	})
	if err == nil {
		t.Fatal("want error when DB update fails, got nil")
	}

	// Gambar lama tidak pernah tersentuh.
	if data, ok := blobs.saved[oldPath]; !ok || string(data) != "old-webp-bytes" {
		t.Fatalf("blob lama seharusnya tetap utuh, got present=%v data=%q", ok, data)
	}
	// Blob baru yang sempat ditulis harus dibersihkan lagi (hanya oldPath
	// tersisa di storage).
	if len(blobs.saved) != 1 {
		t.Fatalf("want hanya blob lama tersisa (blob baru dibersihkan), got %d entries: %v", len(blobs.saved), blobs.saved)
	}
	// image_path di representasi produk tidak berubah dari oldPath.
	if store.product.ImagePath == nil || *store.product.ImagePath != oldPath {
		t.Fatalf("want image_path tetap %q, got %v", oldPath, store.product.ImagePath)
	}
}
