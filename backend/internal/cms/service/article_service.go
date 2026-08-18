// Package service — business logic modul CMS artikel (§14).
package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/rajaku-printing/backend/internal/cms/cmsapi"
	"github.com/rajaku-printing/backend/internal/cms/model"
	cmsrepo "github.com/rajaku-printing/backend/internal/cms/repository"
	"github.com/rajaku-printing/backend/internal/pkg/webp"
)

// FileStore — narrowed filestore contract (sama shape dengan modul lain).
type FileStore interface {
	Save(ctx context.Context, subpath string, r io.Reader) (int64, error)
	Delete(ctx context.Context, subpath string) error
	AbsPath(subpath string) (string, error)
}

// ArticleStore — narrowed repository contract untuk testability.
type ArticleStore interface {
	CreateArticle(ctx context.Context, a *model.Article) error
	UpdateArticle(ctx context.Context, p cmsrepo.UpdateArticleParams) error
	SetStatus(ctx context.Context, id uuid.UUID, s model.Status, at *time.Time) error
	DeleteArticle(ctx context.Context, id uuid.UUID) error
	FindArticleByID(ctx context.Context, id uuid.UUID) (*model.Article, error)
	FindArticleBySlug(ctx context.Context, slug string) (*model.Article, error)
	ListArticles(ctx context.Context, p cmsrepo.ListParams) ([]model.Article, int64, error)

	CreateImage(ctx context.Context, img *model.ArticleImage) error
	FindImageByID(ctx context.Context, id uuid.UUID) (*model.ArticleImage, error)
	AttachImageToArticle(ctx context.Context, imageID, articleID uuid.UUID) error
}

type Config struct {
	// MaxImageUploadMB — batas ukuran raw upload sebelum diconvert (defense
	// in depth; reverse proxy juga sebaiknya cap).
	MaxImageUploadMB int
	// DefaultListLimit — dipakai kalau caller tidak set limit.
	DefaultListLimit int
	// MaxListLimit — batas atas paginasi.
	MaxListLimit int
}

type Service struct {
	store       ArticleStore
	blobs       FileStore
	maxImgBytes int64
	defLimit    int
	maxLimit    int
	nowFn       func() time.Time
}

func New(store ArticleStore, blobs FileStore, cfg Config) *Service {
	maxMB := cfg.MaxImageUploadMB
	if maxMB <= 0 {
		maxMB = 8
	}
	defLim := cfg.DefaultListLimit
	if defLim <= 0 {
		defLim = 10
	}
	maxLim := cfg.MaxListLimit
	if maxLim <= 0 {
		maxLim = 50
	}
	return &Service{
		store:       store,
		blobs:       blobs,
		maxImgBytes: int64(maxMB) * 1024 * 1024,
		defLimit:    defLim,
		maxLimit:    maxLim,
		nowFn:       time.Now,
	}
}

// ---- Article CRUD ----

// CreateArticle — start baru sebagai draft. Slug auto-generate dari title kalau
// kosong; kalau ada, divalidasi format & unik.
func (s *Service) CreateArticle(ctx context.Context, in CreateArticleInput) (*model.Article, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return nil, cmsapi.ErrTitleRequired
	}
	content := strings.TrimSpace(in.ContentMD)
	if content == "" {
		return nil, cmsapi.ErrContentRequired
	}
	slug := strings.TrimSpace(in.Slug)
	if slug == "" {
		slug = SlugifyTitle(title)
	}
	if !isValidSlug(slug) {
		return nil, cmsapi.ErrInvalidSlug
	}

	article := &model.Article{
		ID:              uuid.New(),
		Slug:            slug,
		Title:           title,
		ContentMD:       content,
		Status:          model.StatusDraft,
		AuthorID:        in.AuthorID,
		CoverImageID:    in.CoverImageID,
		MetaTitle:       nilIfEmpty(in.MetaTitle),
		MetaDescription: nilIfEmpty(in.MetaDescription),
		Excerpt:         nilIfEmpty(in.Excerpt),
		CreatedAt:       s.nowFn().UTC(),
		UpdatedAt:       s.nowFn().UTC(),
	}
	if err := s.store.CreateArticle(ctx, article); err != nil {
		if errors.Is(err, cmsrepo.ErrSlugTaken) {
			return nil, cmsapi.ErrSlugTaken
		}
		return nil, fmt.Errorf("create article: %w", err)
	}

	// Kalau cover image di-set, attach sebagai back-reference (article_images.article_id).
	if in.CoverImageID != nil {
		if err := s.store.AttachImageToArticle(ctx, *in.CoverImageID, article.ID); err != nil &&
			!errors.Is(err, cmsrepo.ErrNotFound) {
			log.Ctx(ctx).Warn().Err(err).
				Str("article_id", article.ID.String()).
				Str("image_id", in.CoverImageID.String()).
				Msg("attach cover image ke article gagal — non-fatal, artikel tetap tersimpan")
		}
	}
	return article, nil
}

// UpdateArticle — patch semantics; hanya field non-nil yang diubah.
func (s *Service) UpdateArticle(ctx context.Context, in UpdateArticleInput) (*model.Article, error) {
	existing, err := s.store.FindArticleByID(ctx, in.ID)
	if err != nil {
		if errors.Is(err, cmsrepo.ErrNotFound) {
			return nil, cmsapi.ErrArticleNotFound
		}
		return nil, fmt.Errorf("update: lookup: %w", err)
	}

	patch := cmsrepo.UpdateArticleParams{ID: in.ID}
	if in.Title != nil {
		t := strings.TrimSpace(*in.Title)
		if t == "" {
			return nil, cmsapi.ErrTitleRequired
		}
		patch.Title = &t
	}
	if in.Slug != nil {
		slug := strings.TrimSpace(*in.Slug)
		if !isValidSlug(slug) {
			return nil, cmsapi.ErrInvalidSlug
		}
		patch.Slug = &slug
	}
	if in.ContentMD != nil {
		c := strings.TrimSpace(*in.ContentMD)
		if c == "" {
			return nil, cmsapi.ErrContentRequired
		}
		patch.ContentMD = &c
	}
	if in.Excerpt != nil {
		patch.Excerpt = in.Excerpt
	}
	if in.MetaTitle != nil {
		patch.MetaTitle = in.MetaTitle
	}
	if in.MetaDescription != nil {
		patch.MetaDescription = in.MetaDescription
	}
	if in.CoverImageID != nil {
		patch.CoverImageID = in.CoverImageID
	}

	if err := s.store.UpdateArticle(ctx, patch); err != nil {
		if errors.Is(err, cmsrepo.ErrSlugTaken) {
			return nil, cmsapi.ErrSlugTaken
		}
		if errors.Is(err, cmsrepo.ErrNotFound) {
			return nil, cmsapi.ErrArticleNotFound
		}
		return nil, fmt.Errorf("update article: %w", err)
	}
	// Attach new cover image kalau berubah non-nil.
	if in.CoverImageID != nil && *in.CoverImageID != uuid.Nil && (existing.CoverImageID == nil || *existing.CoverImageID != *in.CoverImageID) {
		if err := s.store.AttachImageToArticle(ctx, *in.CoverImageID, existing.ID); err != nil &&
			!errors.Is(err, cmsrepo.ErrNotFound) {
			log.Ctx(ctx).Warn().Err(err).Msg("attach new cover image gagal — non-fatal")
		}
	}

	return s.store.FindArticleByID(ctx, in.ID)
}

// PublishArticle — set status=published, published_at=now (kalau belum).
// Draft & archived boleh publish; kalau sudah published, no-op sukses.
func (s *Service) PublishArticle(ctx context.Context, id uuid.UUID) (*model.Article, error) {
	a, err := s.store.FindArticleByID(ctx, id)
	if err != nil {
		if errors.Is(err, cmsrepo.ErrNotFound) {
			return nil, cmsapi.ErrArticleNotFound
		}
		return nil, fmt.Errorf("publish: lookup: %w", err)
	}
	if strings.TrimSpace(a.ContentMD) == "" {
		return nil, cmsapi.ErrNotPublishable
	}
	if a.Status == model.StatusPublished {
		return a, nil // idempotent
	}
	// Kalau pertama kali publish, set published_at=now; kalau sebelumnya
	// pernah published lalu diarchive, tetap pakai published_at lama supaya
	// urutan feed konsisten.
	var pubAt *time.Time
	if a.PublishedAt == nil {
		now := s.nowFn().UTC()
		pubAt = &now
	}
	if err := s.store.SetStatus(ctx, id, model.StatusPublished, pubAt); err != nil {
		if errors.Is(err, cmsrepo.ErrNotFound) {
			return nil, cmsapi.ErrArticleNotFound
		}
		return nil, fmt.Errorf("publish: set status: %w", err)
	}
	return s.store.FindArticleByID(ctx, id)
}

// UnpublishArticle — set status=archived. Draft & published bisa. Idempotent.
func (s *Service) UnpublishArticle(ctx context.Context, id uuid.UUID) (*model.Article, error) {
	a, err := s.store.FindArticleByID(ctx, id)
	if err != nil {
		if errors.Is(err, cmsrepo.ErrNotFound) {
			return nil, cmsapi.ErrArticleNotFound
		}
		return nil, fmt.Errorf("unpublish: lookup: %w", err)
	}
	if a.Status == model.StatusArchived {
		return a, nil
	}
	if err := s.store.SetStatus(ctx, id, model.StatusArchived, nil); err != nil {
		if errors.Is(err, cmsrepo.ErrNotFound) {
			return nil, cmsapi.ErrArticleNotFound
		}
		return nil, fmt.Errorf("unpublish: set status: %w", err)
	}
	return s.store.FindArticleByID(ctx, id)
}

func (s *Service) DeleteArticle(ctx context.Context, id uuid.UUID) error {
	if err := s.store.DeleteArticle(ctx, id); err != nil {
		if errors.Is(err, cmsrepo.ErrNotFound) {
			return cmsapi.ErrArticleNotFound
		}
		return fmt.Errorf("delete article: %w", err)
	}
	return nil
}

// ---- Read ----

// GetArticleByID — admin path (any status).
func (s *Service) GetArticleByID(ctx context.Context, id uuid.UUID) (*model.Article, error) {
	a, err := s.store.FindArticleByID(ctx, id)
	if err != nil {
		if errors.Is(err, cmsrepo.ErrNotFound) {
			return nil, cmsapi.ErrArticleNotFound
		}
		return nil, fmt.Errorf("get article: %w", err)
	}
	return a, nil
}

// GetPublishedBySlug — public path. Return ErrArticleNotFound kalau draft/archived
// (jangan bocorkan keberadaannya).
func (s *Service) GetPublishedBySlug(ctx context.Context, slug string) (*model.Article, error) {
	slug = strings.TrimSpace(slug)
	if !isValidSlug(slug) {
		return nil, cmsapi.ErrArticleNotFound
	}
	a, err := s.store.FindArticleBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, cmsrepo.ErrNotFound) {
			return nil, cmsapi.ErrArticleNotFound
		}
		return nil, fmt.Errorf("get by slug: %w", err)
	}
	if a.Status != model.StatusPublished || a.PublishedAt == nil || a.PublishedAt.After(s.nowFn().UTC()) {
		return nil, cmsapi.ErrArticleNotFound
	}
	return a, nil
}

// ListPublished — public list, only status=published & published_at<=now.
func (s *Service) ListPublished(ctx context.Context, page, limit int, search string) (*ListResult, error) {
	page, limit = s.normalizePagination(page, limit)
	items, total, err := s.store.ListArticles(ctx, cmsrepo.ListParams{
		PublishedOnly: true,
		Search:        search,
		Limit:         limit,
		Offset:        (page - 1) * limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list published: %w", err)
	}
	return &ListResult{Items: items, Total: total, Limit: limit, Page: page}, nil
}

// ListForAdmin — admin list dengan filter status opsional.
func (s *Service) ListForAdmin(ctx context.Context, page, limit int, status, search string) (*ListResult, error) {
	page, limit = s.normalizePagination(page, limit)
	var st model.Status
	if status != "" {
		switch model.Status(status) {
		case model.StatusDraft, model.StatusPublished, model.StatusArchived:
			st = model.Status(status)
		default:
			return nil, cmsapi.ErrInvalidStatus
		}
	}
	items, total, err := s.store.ListArticles(ctx, cmsrepo.ListParams{
		Status: st,
		Search: search,
		Limit:  limit,
		Offset: (page - 1) * limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list admin: %w", err)
	}
	return &ListResult{Items: items, Total: total, Limit: limit, Page: page}, nil
}

// ---- Image upload ----

// UploadImage — decode raw upload, convert ke WebP, simpan ke disk & DB.
// Return row article_images (id → dipakai handler bikin URL).
func (s *Service) UploadImage(ctx context.Context, in UploadImageInput) (*model.ArticleImage, error) {
	if in.FileSize <= 0 {
		return nil, cmsapi.ErrImageEmpty
	}
	if in.FileSize > s.maxImgBytes {
		return nil, cmsapi.ErrImageTooLarge
	}
	if !webp.IsAllowedMime(in.MimeType) {
		return nil, cmsapi.ErrImageInvalidType
	}

	// Batasi read agar tidak overshoot (defense terhadap MIME lie).
	limitedR := io.LimitReader(in.FileReader, s.maxImgBytes+1)
	encoded, err := webp.Encode(limitedR)
	if err != nil {
		if errors.Is(err, webp.ErrDecodeFailed) {
			return nil, cmsapi.ErrImageDecodeFailed
		}
		return nil, fmt.Errorf("encode webp: %w", err)
	}

	now := s.nowFn().UTC()
	imgID := uuid.New()
	subpath := path.Join(
		"cms_images",
		fmt.Sprintf("%04d", now.Year()),
		fmt.Sprintf("%02d", now.Month()),
		imgID.String()+".webp",
	)
	written, err := s.blobs.Save(ctx, subpath, bytes.NewReader(encoded.Data))
	if err != nil {
		return nil, fmt.Errorf("save webp blob: %w", err)
	}

	row := &model.ArticleImage{
		ID:           imgID,
		ArticleID:    in.ArticleID,
		StoragePath:  subpath,
		OriginalName: safeOriginalName(in.OriginalName),
		MimeType:     "image/webp",
		SizeBytes:    written,
		WidthPx:      encoded.Width,
		HeightPx:     encoded.Height,
		UploadedBy:   &in.UploaderID,
		UploadedAt:   now,
	}
	if in.AltText != "" {
		alt := in.AltText
		row.AltText = &alt
	}

	if err := s.store.CreateImage(ctx, row); err != nil {
		_ = s.blobs.Delete(ctx, subpath)
		return nil, fmt.Errorf("insert article_image row: %w", err)
	}
	return row, nil
}

// GetImage — serve image publicly (artikel img embed di halaman public).
func (s *Service) GetImage(ctx context.Context, id uuid.UUID) (*ImageHandle, error) {
	img, err := s.store.FindImageByID(ctx, id)
	if err != nil {
		if errors.Is(err, cmsrepo.ErrNotFound) {
			return nil, cmsapi.ErrImageNotFound
		}
		return nil, fmt.Errorf("get image: %w", err)
	}
	abs, err := s.blobs.AbsPath(img.StoragePath)
	if err != nil {
		log.Ctx(ctx).Error().Err(err).
			Str("image_id", img.ID.String()).
			Str("subpath", img.StoragePath).
			Msg("cms image path rejected — cek DB")
		return nil, fmt.Errorf("resolve path: %w", err)
	}
	return &ImageHandle{Image: img, AbsPath: abs}, nil
}

// ---- Helpers ----

func (s *Service) normalizePagination(page, limit int) (int, int) {
	if page < 1 {
		page = 1
	}
	if limit <= 0 {
		limit = s.defLimit
	}
	if limit > s.maxLimit {
		limit = s.maxLimit
	}
	return page, limit
}

// slugRE — cek final slug format (a-z, 0-9, dash, no leading/trailing dash,
// no double-dash). Match constraint di migration 000009.
var slugRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func isValidSlug(s string) bool {
	if s == "" || len(s) > 200 {
		return false
	}
	return slugRE.MatchString(s)
}

// SlugifyTitle — konversi title ke slug SEO. Exported supaya test/handler
// bisa preview slug sebelum submit.
func SlugifyTitle(title string) string {
	// Lower, ganti spasi/tanda baca dengan dash, buang non-alnum.
	var b strings.Builder
	prevDash := true
	for _, r := range strings.ToLower(strings.TrimSpace(title)) {
		switch {
		case unicode.IsLetter(r) && r < 128, unicode.IsDigit(r):
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	slug := strings.Trim(b.String(), "-")
	if len(slug) > 200 {
		slug = strings.Trim(slug[:200], "-")
	}
	if slug == "" {
		// Fallback — pakai timestamp epoch supaya tetap unik-ish.
		slug = "artikel"
	}
	return slug
}

func nilIfEmpty(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	v := s
	return &v
}

// safeOriginalName — buang path separator supaya nama file yg disimpan aman
// untuk display (bukan untuk fs path — storage_path digenerate service).
func safeOriginalName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "file"
	}
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, "/", "_")
	if len(name) > 255 {
		name = name[:255]
	}
	return name
}
