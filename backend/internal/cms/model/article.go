// Package model — GORM entities untuk modul CMS artikel (§14).
package model

import (
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusDraft     Status = "draft"
	StatusPublished Status = "published"
	StatusArchived  Status = "archived"
)

type Article struct {
	ID   uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Slug string    `gorm:"size:200;uniqueIndex;not null"                  json:"slug"`

	Title      string  `gorm:"size:200;not null" json:"title"`
	Excerpt    *string `                         json:"excerpt,omitempty"`
	ContentMD  string  `gorm:"column:content_md;not null" json:"content_md"`

	CoverImageID *uuid.UUID `gorm:"type:uuid;column:cover_image_id" json:"cover_image_id,omitempty"`

	MetaTitle       *string `gorm:"size:200" json:"meta_title,omitempty"`
	MetaDescription *string `gorm:"size:320" json:"meta_description,omitempty"`

	Status      Status     `gorm:"size:20;not null;default:draft" json:"status"`
	PublishedAt *time.Time `                                       json:"published_at,omitempty"`

	AuthorID uuid.UUID `gorm:"type:uuid;not null" json:"author_id"`

	CreatedAt time.Time `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt time.Time `gorm:"not null;default:now()" json:"updated_at"`
}

func (Article) TableName() string { return "articles" }

type ArticleImage struct {
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ArticleID    *uuid.UUID `gorm:"type:uuid;index"                                 json:"article_id,omitempty"`
	StoragePath  string     `gorm:"not null"                                        json:"-"`
	OriginalName string     `gorm:"size:255;not null"                               json:"original_name"`
	MimeType     string     `gorm:"size:50;not null"                                json:"mime_type"`
	SizeBytes    int64      `gorm:"not null"                                        json:"size_bytes"`
	WidthPx      int        `gorm:"column:width_px;not null"                        json:"width_px"`
	HeightPx     int        `gorm:"column:height_px;not null"                       json:"height_px"`
	AltText      *string    `gorm:"size:255"                                        json:"alt_text,omitempty"`
	UploadedBy   *uuid.UUID `gorm:"type:uuid"                                       json:"uploaded_by,omitempty"`
	UploadedAt   time.Time  `gorm:"not null;default:now()"                          json:"uploaded_at"`
}

func (ArticleImage) TableName() string { return "article_images" }
