// Package repository — DB access untuk modul CMS.
package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/rajaku-printing/backend/internal/cms/model"
)

var (
	ErrNotFound  = errors.New("cms/repository: not found")
	ErrSlugTaken = errors.New("cms/repository: slug already used")
)

const pgUniqueViolationCode = "23505"

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

// ----- Articles -----

func (r *Repository) CreateArticle(ctx context.Context, a *model.Article) error {
	if err := r.db.WithContext(ctx).Create(a).Error; err != nil {
		if isUniqueViolation(err) {
			return ErrSlugTaken
		}
		return fmt.Errorf("insert article: %w", err)
	}
	return nil
}

// UpdateArticleParams — fields yang dituliskan saat update. Nil-value skip
// (pakai patch-semantics dari service layer).
type UpdateArticleParams struct {
	ID              uuid.UUID
	Slug            *string
	Title           *string
	Excerpt         *string
	ContentMD       *string
	CoverImageID    *uuid.UUID // nil = tidak diubah; *uuid.Nil = clear (unset)
	MetaTitle       *string
	MetaDescription *string
}

// UpdateArticle applies patch fields. Slug conflict → ErrSlugTaken.
func (r *Repository) UpdateArticle(ctx context.Context, p UpdateArticleParams) error {
	updates := map[string]any{"updated_at": time.Now().UTC()}
	if p.Slug != nil {
		updates["slug"] = *p.Slug
	}
	if p.Title != nil {
		updates["title"] = *p.Title
	}
	if p.Excerpt != nil {
		updates["excerpt"] = strPtrOrNil(*p.Excerpt)
	}
	if p.ContentMD != nil {
		updates["content_md"] = *p.ContentMD
	}
	if p.CoverImageID != nil {
		if *p.CoverImageID == uuid.Nil {
			updates["cover_image_id"] = nil
		} else {
			updates["cover_image_id"] = *p.CoverImageID
		}
	}
	if p.MetaTitle != nil {
		updates["meta_title"] = strPtrOrNil(*p.MetaTitle)
	}
	if p.MetaDescription != nil {
		updates["meta_description"] = strPtrOrNil(*p.MetaDescription)
	}
	res := r.db.WithContext(ctx).
		Model(&model.Article{}).
		Where("id = ?", p.ID).
		Updates(updates)
	if res.Error != nil {
		if isUniqueViolation(res.Error) {
			return ErrSlugTaken
		}
		return fmt.Errorf("update article: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// SetStatus updates status + published_at atomically.
func (r *Repository) SetStatus(ctx context.Context, id uuid.UUID, status model.Status, publishedAt *time.Time) error {
	updates := map[string]any{
		"status":     status,
		"updated_at": time.Now().UTC(),
	}
	if status == model.StatusPublished {
		if publishedAt == nil {
			now := time.Now().UTC()
			publishedAt = &now
		}
		updates["published_at"] = *publishedAt
	}
	res := r.db.WithContext(ctx).
		Model(&model.Article{}).
		Where("id = ?", id).
		Updates(updates)
	if res.Error != nil {
		return fmt.Errorf("set article status: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) DeleteArticle(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Delete(&model.Article{}, "id = ?", id)
	if res.Error != nil {
		return fmt.Errorf("delete article: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) FindArticleByID(ctx context.Context, id uuid.UUID) (*model.Article, error) {
	var a model.Article
	err := r.db.WithContext(ctx).First(&a, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find article by id: %w", err)
	}
	return &a, nil
}

func (r *Repository) FindArticleBySlug(ctx context.Context, slug string) (*model.Article, error) {
	var a model.Article
	err := r.db.WithContext(ctx).First(&a, "slug = ?", slug).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find article by slug: %w", err)
	}
	return &a, nil
}

// ListParams — parameter list generic (dipakai admin & public via filter).
type ListParams struct {
	// PublishedOnly — filter status=published AND published_at <= NOW().
	// Digunakan endpoint public.
	PublishedOnly bool
	// Status filter untuk admin; kosong = semua status.
	Status model.Status
	// Search text — LIKE lower(title). Kosong = tanpa filter.
	Search string
	// Pagination — Limit wajib > 0 (default handled di service).
	Limit  int
	Offset int
}

// ListArticles returns matching articles + total count (for pagination).
func (r *Repository) ListArticles(ctx context.Context, p ListParams) ([]model.Article, int64, error) {
	q := r.db.WithContext(ctx).Model(&model.Article{})
	if p.PublishedOnly {
		q = q.Where("status = ? AND published_at <= ?", model.StatusPublished, time.Now().UTC())
	} else if p.Status != "" {
		q = q.Where("status = ?", p.Status)
	}
	if s := strings.TrimSpace(p.Search); s != "" {
		q = q.Where("lower(title) LIKE ?", "%"+strings.ToLower(s)+"%")
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count articles: %w", err)
	}

	// Public listing ordered by published_at DESC; admin fallback ke updated_at.
	orderClause := "updated_at DESC"
	if p.PublishedOnly {
		orderClause = "published_at DESC"
	}
	var items []model.Article
	err := q.Order(orderClause).Limit(p.Limit).Offset(p.Offset).Find(&items).Error
	if err != nil {
		return nil, 0, fmt.Errorf("list articles: %w", err)
	}
	return items, total, nil
}

// ----- Images -----

func (r *Repository) CreateImage(ctx context.Context, img *model.ArticleImage) error {
	if err := r.db.WithContext(ctx).Create(img).Error; err != nil {
		return fmt.Errorf("insert article_image: %w", err)
	}
	return nil
}

func (r *Repository) FindImageByID(ctx context.Context, id uuid.UUID) (*model.ArticleImage, error) {
	var img model.ArticleImage
	err := r.db.WithContext(ctx).First(&img, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find image by id: %w", err)
	}
	return &img, nil
}

// AttachImageToArticle sets article_id on the image record (used when
// an image uploaded standalone gets linked to an article on save).
func (r *Repository) AttachImageToArticle(ctx context.Context, imageID, articleID uuid.UUID) error {
	res := r.db.WithContext(ctx).
		Model(&model.ArticleImage{}).
		Where("id = ?", imageID).
		Update("article_id", articleID)
	if res.Error != nil {
		return fmt.Errorf("attach image to article: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ----- helpers -----

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == pgUniqueViolationCode
	}
	return false
}

// strPtrOrNil — string kosong dianggap null di DB (patch semantic).
func strPtrOrNil(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
