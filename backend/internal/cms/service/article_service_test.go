package service

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"strconv"
	"strings"
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

	createImageErr        error
	findImageErr          error
	attachImageErr        error
	attachImageCalls      int
	updateImageAltTextErr error
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
	if p.FocusKeyword != nil {
		a.FocusKeyword = p.FocusKeyword
	}
	if p.SecondaryKeywords != nil {
		a.SecondaryKeywords = p.SecondaryKeywords
	}
	if p.SeoScore != nil {
		v := int16(*p.SeoScore)
		a.SeoScore = &v
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
func (f *fakeStore) UpdateImageAltText(_ context.Context, id uuid.UUID, altText string) error {
	if f.updateImageAltTextErr != nil {
		return f.updateImageAltTextErr
	}
	img, ok := f.images[id]
	if !ok {
		return cmsrepo.ErrNotFound
	}
	if altText == "" {
		img.AltText = nil
	} else {
		img.AltText = &altText
	}
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

// ---------- SEO panel ----------

func TestCreateAndUpdateArticle_SeoFieldsRoundTrip(t *testing.T) {
	svc, _, _ := newSvc()
	author := uuid.New()
	score := 75

	a, err := svc.CreateArticle(context.Background(), CreateArticleInput{
		AuthorID:          author,
		Title:             "Panduan Lengkap Ukuran Banner Outdoor",
		ContentMD:         "isi artikel",
		FocusKeyword:      "banner outdoor",
		SecondaryKeywords: "banner, spanduk, outdoor",
		SeoScore:          &score,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if a.FocusKeyword == nil || *a.FocusKeyword != "banner outdoor" {
		t.Errorf("focus_keyword tidak tersimpan: %+v", a.FocusKeyword)
	}
	if a.SecondaryKeywords == nil || *a.SecondaryKeywords != "banner, spanduk, outdoor" {
		t.Errorf("secondary_keywords tidak tersimpan: %+v", a.SecondaryKeywords)
	}
	if a.SeoScore == nil || *a.SeoScore != 75 {
		t.Errorf("seo_score tidak tersimpan: %+v", a.SeoScore)
	}

	newKeyword := "spanduk outdoor tahan air"
	newScore := 90
	updated, err := svc.UpdateArticle(context.Background(), UpdateArticleInput{
		ID:           a.ID,
		FocusKeyword: &newKeyword,
		SeoScore:     &newScore,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.FocusKeyword == nil || *updated.FocusKeyword != newKeyword {
		t.Errorf("focus_keyword tidak terupdate: %+v", updated.FocusKeyword)
	}
	if updated.SeoScore == nil || *updated.SeoScore != 90 {
		t.Errorf("seo_score tidak terupdate: %+v", updated.SeoScore)
	}
}

func TestCreateArticle_SeoScoreOutOfRange(t *testing.T) {
	svc, _, _ := newSvc()
	badScore := 150
	_, err := svc.CreateArticle(context.Background(), CreateArticleInput{
		AuthorID:  uuid.New(),
		Title:     "Judul Valid",
		ContentMD: "isi",
		SeoScore:  &badScore,
	})
	if !errors.Is(err, cmsapi.ErrInvalidSeoScore) {
		t.Errorf("expected ErrInvalidSeoScore, got %v", err)
	}
}

func TestUpdateArticle_SeoScoreOutOfRange(t *testing.T) {
	svc, _, _ := newSvc()
	a, err := svc.CreateArticle(context.Background(), CreateArticleInput{
		AuthorID:  uuid.New(),
		Title:     "Judul Valid Lainnya",
		ContentMD: "isi",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	badScore := -1
	_, err = svc.UpdateArticle(context.Background(), UpdateArticleInput{
		ID:       a.ID,
		SeoScore: &badScore,
	})
	if !errors.Is(err, cmsapi.ErrInvalidSeoScore) {
		t.Errorf("expected ErrInvalidSeoScore, got %v", err)
	}
}

func TestCreateArticle_SeoScoreBoundaryAccepted(t *testing.T) {
	for _, score := range []int{0, 100} {
		score := score
		t.Run(strconv.Itoa(score), func(t *testing.T) {
			svc, _, _ := newSvc()
			a, err := svc.CreateArticle(context.Background(), CreateArticleInput{
				AuthorID:  uuid.New(),
				Title:     "Judul Boundary " + strconv.Itoa(score),
				ContentMD: "isi",
				SeoScore:  &score,
			})
			if err != nil {
				t.Fatalf("unexpected err untuk score=%d: %v", score, err)
			}
			if a.SeoScore == nil || int(*a.SeoScore) != score {
				t.Errorf("seo_score tidak tersimpan sesuai batas: %+v", a.SeoScore)
			}
		})
	}
}

func TestUpdateArticle_SeoScoreBoundaryAccepted(t *testing.T) {
	for _, score := range []int{0, 100} {
		score := score
		t.Run(strconv.Itoa(score), func(t *testing.T) {
			svc, _, _ := newSvc()
			a, err := svc.CreateArticle(context.Background(), CreateArticleInput{
				AuthorID:  uuid.New(),
				Title:     "Judul Update Boundary " + strconv.Itoa(score),
				ContentMD: "isi",
			})
			if err != nil {
				t.Fatalf("create: %v", err)
			}
			updated, err := svc.UpdateArticle(context.Background(), UpdateArticleInput{
				ID:       a.ID,
				SeoScore: &score,
			})
			if err != nil {
				t.Fatalf("unexpected err untuk score=%d: %v", score, err)
			}
			if updated.SeoScore == nil || int(*updated.SeoScore) != score {
				t.Errorf("seo_score tidak terupdate sesuai batas: %+v", updated.SeoScore)
			}
		})
	}
}

// ---------- SEO panel — length validation ----------

func TestCreateArticle_FocusKeywordTooLong(t *testing.T) {
	svc, _, _ := newSvc()
	_, err := svc.CreateArticle(context.Background(), CreateArticleInput{
		AuthorID:     uuid.New(),
		Title:        "Judul Valid",
		ContentMD:    "isi",
		FocusKeyword: strings.Repeat("a", 101),
	})
	if !errors.Is(err, cmsapi.ErrFocusKeywordTooLong) {
		t.Errorf("expected ErrFocusKeywordTooLong, got %v", err)
	}
}

func TestCreateArticle_SecondaryKeywordsTooLong(t *testing.T) {
	svc, _, _ := newSvc()
	_, err := svc.CreateArticle(context.Background(), CreateArticleInput{
		AuthorID:          uuid.New(),
		Title:             "Judul Valid Lain",
		ContentMD:         "isi",
		SecondaryKeywords: strings.Repeat("a", 301),
	})
	if !errors.Is(err, cmsapi.ErrSecondaryKeywordsTooLong) {
		t.Errorf("expected ErrSecondaryKeywordsTooLong, got %v", err)
	}
}

func TestUpdateArticle_FocusKeywordTooLong(t *testing.T) {
	svc, _, _ := newSvc()
	a, err := svc.CreateArticle(context.Background(), CreateArticleInput{
		AuthorID:  uuid.New(),
		Title:     "Judul Untuk Update",
		ContentMD: "isi",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	tooLong := strings.Repeat("b", 101)
	_, err = svc.UpdateArticle(context.Background(), UpdateArticleInput{
		ID:           a.ID,
		FocusKeyword: &tooLong,
	})
	if !errors.Is(err, cmsapi.ErrFocusKeywordTooLong) {
		t.Errorf("expected ErrFocusKeywordTooLong, got %v", err)
	}
}

func TestCreateArticle_FocusKeywordTrimmedBeforeLengthCheck(t *testing.T) {
	svc, _, _ := newSvc()
	// 100 huruf asli + padding spasi di kedua sisi — harus lolos karena
	// panjang dihitung SETELAH trim.
	padded := "  " + strings.Repeat("c", 100) + "  "
	a, err := svc.CreateArticle(context.Background(), CreateArticleInput{
		AuthorID:     uuid.New(),
		Title:        "Judul Trim",
		ContentMD:    "isi",
		FocusKeyword: padded,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if a.FocusKeyword == nil || *a.FocusKeyword != strings.Repeat("c", 100) {
		t.Errorf("focus_keyword tidak ter-trim dengan benar: %+v", a.FocusKeyword)
	}
}

// ---------- UpdateImageAltText / GetImageMeta ----------

func TestUpdateImageAltText_HappyPath(t *testing.T) {
	svc, store, _ := newSvc()
	imgID := uuid.New()
	store.images[imgID] = &model.ArticleImage{ID: imgID, MimeType: "image/webp"}

	updated, err := svc.UpdateImageAltText(context.Background(), imgID, "  banner outdoor merah  ")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if updated.AltText == nil || *updated.AltText != "banner outdoor merah" {
		t.Errorf("alt_text tidak terupdate (trimmed): %+v", updated.AltText)
	}
}

func TestUpdateImageAltText_NotFound(t *testing.T) {
	svc, store, _ := newSvc()
	imgID := uuid.New()
	store.updateImageAltTextErr = cmsrepo.ErrNotFound

	_, err := svc.UpdateImageAltText(context.Background(), imgID, "alt")
	if !errors.Is(err, cmsapi.ErrImageNotFound) {
		t.Errorf("expected ErrImageNotFound, got %v", err)
	}
}

func TestUpdateImageAltText_TooLong(t *testing.T) {
	svc, store, _ := newSvc()
	imgID := uuid.New()
	store.images[imgID] = &model.ArticleImage{ID: imgID, MimeType: "image/webp"}

	_, err := svc.UpdateImageAltText(context.Background(), imgID, strings.Repeat("x", 256))
	if !errors.Is(err, cmsapi.ErrAltTextTooLong) {
		t.Errorf("expected ErrAltTextTooLong, got %v", err)
	}
}

func TestGetImageMeta_HappyPath(t *testing.T) {
	svc, store, _ := newSvc()
	imgID := uuid.New()
	alt := "logo rajaku"
	store.images[imgID] = &model.ArticleImage{ID: imgID, MimeType: "image/webp", AltText: &alt}

	img, err := svc.GetImageMeta(context.Background(), imgID)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if img.AltText == nil || *img.AltText != alt {
		t.Errorf("alt_text tidak sesuai: %+v", img.AltText)
	}
}

func TestGetImageMeta_NotFound(t *testing.T) {
	svc, _, _ := newSvc()
	_, err := svc.GetImageMeta(context.Background(), uuid.New())
	if !errors.Is(err, cmsapi.ErrImageNotFound) {
		t.Errorf("expected ErrImageNotFound, got %v", err)
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
