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

	"github.com/rajaku-printing/backend/internal/cms/cmsapi"
	"github.com/rajaku-printing/backend/internal/cms/model"
	cmsrepo "github.com/rajaku-printing/backend/internal/cms/repository"
)

// ---------- fakes ----------

type fakeStore struct {
	articles map[uuid.UUID]*model.Article
	slugToID map[string]uuid.UUID
	images   map[uuid.UUID]*model.ArticleImage

	createArticleErr error
	updateArticleErr error
	setStatusErr     error
	deleteErr        error
	findByIDErr      error
	findBySlugErr    error
	listErr          error

	createImageErr   error
	findImageErr     error
	attachImageErr   error
	attachImageCalls int
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		articles: map[uuid.UUID]*model.Article{},
		slugToID: map[string]uuid.UUID{},
		images:   map[uuid.UUID]*model.ArticleImage{},
	}
}

func (f *fakeStore) CreateArticle(_ context.Context, a *model.Article) error {
	if f.createArticleErr != nil {
		return f.createArticleErr
	}
	if _, dup := f.slugToID[a.Slug]; dup {
		return cmsrepo.ErrSlugTaken
	}
	f.articles[a.ID] = a
	f.slugToID[a.Slug] = a.ID
	return nil
}
func (f *fakeStore) UpdateArticle(_ context.Context, p cmsrepo.UpdateArticleParams) error {
	if f.updateArticleErr != nil {
		return f.updateArticleErr
	}
	a, ok := f.articles[p.ID]
	if !ok {
		return cmsrepo.ErrNotFound
	}
	if p.Slug != nil {
		if id, dup := f.slugToID[*p.Slug]; dup && id != a.ID {
			return cmsrepo.ErrSlugTaken
		}
		delete(f.slugToID, a.Slug)
		a.Slug = *p.Slug
		f.slugToID[a.Slug] = a.ID
	}
	if p.Title != nil {
		a.Title = *p.Title
	}
	if p.ContentMD != nil {
		a.ContentMD = *p.ContentMD
	}
	return nil
}
func (f *fakeStore) SetStatus(_ context.Context, id uuid.UUID, s model.Status, at *time.Time) error {
	if f.setStatusErr != nil {
		return f.setStatusErr
	}
	a, ok := f.articles[id]
	if !ok {
		return cmsrepo.ErrNotFound
	}
	a.Status = s
	if at != nil {
		a.PublishedAt = at
	}
	return nil
}
func (f *fakeStore) DeleteArticle(_ context.Context, id uuid.UUID) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	a, ok := f.articles[id]
	if !ok {
		return cmsrepo.ErrNotFound
	}
	delete(f.slugToID, a.Slug)
	delete(f.articles, id)
	return nil
}
func (f *fakeStore) FindArticleByID(_ context.Context, id uuid.UUID) (*model.Article, error) {
	if f.findByIDErr != nil {
		return nil, f.findByIDErr
	}
	a, ok := f.articles[id]
	if !ok {
		return nil, cmsrepo.ErrNotFound
	}
	return a, nil
}
func (f *fakeStore) FindArticleBySlug(_ context.Context, slug string) (*model.Article, error) {
	if f.findBySlugErr != nil {
		return nil, f.findBySlugErr
	}
	id, ok := f.slugToID[slug]
	if !ok {
		return nil, cmsrepo.ErrNotFound
	}
	return f.articles[id], nil
}
func (f *fakeStore) ListArticles(_ context.Context, p cmsrepo.ListParams) ([]model.Article, int64, error) {
	if f.listErr != nil {
		return nil, 0, f.listErr
	}
	items := make([]model.Article, 0, len(f.articles))
	for _, a := range f.articles {
		if p.PublishedOnly && a.Status != model.StatusPublished {
			continue
		}
		if p.Status != "" && a.Status != p.Status {
			continue
		}
		items = append(items, *a)
	}
	return items, int64(len(items)), nil
}
func (f *fakeStore) CreateImage(_ context.Context, img *model.ArticleImage) error {
	if f.createImageErr != nil {
		return f.createImageErr
	}
	f.images[img.ID] = img
	return nil
}
func (f *fakeStore) FindImageByID(_ context.Context, id uuid.UUID) (*model.ArticleImage, error) {
	if f.findImageErr != nil {
		return nil, f.findImageErr
	}
	img, ok := f.images[id]
	if !ok {
		return nil, cmsrepo.ErrNotFound
	}
	return img, nil
}
func (f *fakeStore) AttachImageToArticle(_ context.Context, imageID, articleID uuid.UUID) error {
	f.attachImageCalls++
	if f.attachImageErr != nil {
		return f.attachImageErr
	}
	img, ok := f.images[imageID]
	if !ok {
		return cmsrepo.ErrNotFound
	}
	img.ArticleID = &articleID
	return nil
}

type fakeBlobs struct {
	saved       map[string][]byte
	saveErr     error
	deleteCalls int
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
	b.deleteCalls++
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
	svc := New(store, blobs, Config{MaxImageUploadMB: 2})
	svc.nowFn = func() time.Time { return time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC) }
	return svc, store, blobs
}

// tiny 4x4 red PNG for image tests — nativewebp will encode this to WebP.
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

// ---------- CreateArticle ----------

func TestCreateArticle_HappyPath(t *testing.T) {
	svc, store, _ := newSvc()
	author := uuid.New()

	a, err := svc.CreateArticle(context.Background(), CreateArticleInput{
		AuthorID:  author,
		Title:     "Cara Pilih Bahan Banner Outdoor Tahan Cuaca",
		ContentMD: "# Hello\n\nIsi artikel.",
		Excerpt:   "Ringkasan pendek",
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if a.Slug != "cara-pilih-bahan-banner-outdoor-tahan-cuaca" {
		t.Errorf("slug auto-generate salah: %q", a.Slug)
	}
	if a.Status != model.StatusDraft {
		t.Errorf("expected draft, got %s", a.Status)
	}
	if _, ok := store.articles[a.ID]; !ok {
		t.Error("article not persisted")
	}
}

func TestCreateArticle_MissingTitle(t *testing.T) {
	svc, _, _ := newSvc()
	_, err := svc.CreateArticle(context.Background(), CreateArticleInput{
		AuthorID:  uuid.New(),
		Title:     "   ",
		ContentMD: "isi",
	})
	if !errors.Is(err, cmsapi.ErrTitleRequired) {
		t.Errorf("expected ErrTitleRequired, got %v", err)
	}
}

func TestCreateArticle_InvalidExplicitSlug(t *testing.T) {
	svc, _, _ := newSvc()
	_, err := svc.CreateArticle(context.Background(), CreateArticleInput{
		AuthorID:  uuid.New(),
		Title:     "OK",
		Slug:      "Bad Slug!",
		ContentMD: "isi",
	})
	if !errors.Is(err, cmsapi.ErrInvalidSlug) {
		t.Errorf("expected ErrInvalidSlug, got %v", err)
	}
}

func TestCreateArticle_SlugTaken(t *testing.T) {
	svc, store, _ := newSvc()
	store.slugToID["ada"] = uuid.New()
	_, err := svc.CreateArticle(context.Background(), CreateArticleInput{
		AuthorID:  uuid.New(),
		Title:     "Ada",
		ContentMD: "isi",
	})
	if !errors.Is(err, cmsapi.ErrSlugTaken) {
		t.Errorf("expected ErrSlugTaken, got %v", err)
	}
}

// ---------- Publish / GetPublishedBySlug ----------

func TestPublishAndGetPublishedBySlug(t *testing.T) {
	svc, _, _ := newSvc()
	a, err := svc.CreateArticle(context.Background(), CreateArticleInput{
		AuthorID:  uuid.New(),
		Title:     "Panduan Ukuran Banner",
		ContentMD: "body",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	pub, err := svc.PublishArticle(context.Background(), a.ID)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if pub.Status != model.StatusPublished || pub.PublishedAt == nil {
		t.Errorf("publish state salah: %+v", pub)
	}

	got, err := svc.GetPublishedBySlug(context.Background(), a.Slug)
	if err != nil {
		t.Fatalf("get published: %v", err)
	}
	if got.ID != a.ID {
		t.Error("slug lookup return artikel salah")
	}
}

func TestGetPublishedBySlug_DraftHidden(t *testing.T) {
	svc, _, _ := newSvc()
	a, _ := svc.CreateArticle(context.Background(), CreateArticleInput{
		AuthorID:  uuid.New(),
		Title:     "Draft Aja",
		ContentMD: "body",
	})
	_, err := svc.GetPublishedBySlug(context.Background(), a.Slug)
	if !errors.Is(err, cmsapi.ErrArticleNotFound) {
		t.Errorf("draft harus 404, got %v", err)
	}
}

// ---------- UploadImage ----------

func TestUploadImage_HappyConvertsToWebP(t *testing.T) {
	svc, store, blobs := newSvc()
	pngBytes := tinyPNG(t)

	img, err := svc.UploadImage(context.Background(), UploadImageInput{
		UploaderID:   uuid.New(),
		FileReader:   bytes.NewReader(pngBytes),
		FileSize:     int64(len(pngBytes)),
		MimeType:     "image/png",
		OriginalName: "cover.png",
	})
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if img.MimeType != "image/webp" {
		t.Errorf("expected image/webp, got %s", img.MimeType)
	}
	if img.WidthPx != 4 || img.HeightPx != 4 {
		t.Errorf("dimensions salah: %dx%d", img.WidthPx, img.HeightPx)
	}
	if _, ok := blobs.saved[img.StoragePath]; !ok {
		t.Error("blob tidak tersimpan di storage_path")
	}
	if _, ok := store.images[img.ID]; !ok {
		t.Error("row image tidak persisted")
	}
}

func TestUploadImage_InvalidMime(t *testing.T) {
	svc, _, _ := newSvc()
	_, err := svc.UploadImage(context.Background(), UploadImageInput{
		UploaderID: uuid.New(),
		FileReader: bytes.NewReader([]byte("not-an-image")),
		FileSize:   12,
		MimeType:   "application/pdf",
	})
	if !errors.Is(err, cmsapi.ErrImageInvalidType) {
		t.Errorf("expected ErrImageInvalidType, got %v", err)
	}
}

func TestUploadImage_CorruptDecodeFails(t *testing.T) {
	svc, _, _ := newSvc()
	// Mime declared png tapi bytes bukan png valid → decode gagal.
	garbage := []byte("this is not a png at all, hanya bytes acak-acak")
	_, err := svc.UploadImage(context.Background(), UploadImageInput{
		UploaderID: uuid.New(),
		FileReader: bytes.NewReader(garbage),
		FileSize:   int64(len(garbage)),
		MimeType:   "image/png",
	})
	if !errors.Is(err, cmsapi.ErrImageDecodeFailed) {
		t.Errorf("expected ErrImageDecodeFailed, got %v", err)
	}
}

// ---------- Slugify ----------

func TestSlugifyTitle(t *testing.T) {
	cases := map[string]string{
		"Halo Dunia":               "halo-dunia",
		"  Ada  Spasi  Berlebih  ": "ada-spasi-berlebih",
		"Rp10.000/m² Banner!":      "rp10-000-m-banner",
		"":                         "artikel",
		"###":                      "artikel",
	}
	for in, want := range cases {
		if got := SlugifyTitle(in); got != want {
			t.Errorf("SlugifyTitle(%q) = %q, want %q", in, got, want)
		}
	}
}
