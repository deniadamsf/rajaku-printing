package service

import (
	"io"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/cms/model"
)

// CreateArticleInput — payload create draft. Slug boleh kosong → auto-generate
// dari title. Status selalu draft saat create; publish via PublishArticle.
type CreateArticleInput struct {
	AuthorID        uuid.UUID
	Title           string
	Slug            string // optional
	Excerpt         string
	ContentMD       string
	MetaTitle       string
	MetaDescription string
	CoverImageID    *uuid.UUID // optional

	// SEO panel — nilai apa adanya dari editor. SeoScore nil = belum dihitung
	// client-side (mis. draft baru dibuat).
	FocusKeyword      string
	SecondaryKeywords string
	SeoScore          *int
}

// UpdateArticleInput — patch semantics. Pointer nil = field tidak diubah.
// Slug bisa diubah admin (SEO consideration — service tidak enforce redirect
// mapping di MVP; anggap admin sadar).
type UpdateArticleInput struct {
	ID              uuid.UUID
	Title           *string
	Slug            *string
	Excerpt         *string
	ContentMD       *string
	MetaTitle       *string
	MetaDescription *string
	// CoverImageID: nil = tidak diubah; pointer ke uuid.Nil = clear (unset).
	CoverImageID *uuid.UUID

	// SEO panel — FocusKeyword/SecondaryKeywords ikut patch semantics field
	// lain: nil = tidak diubah, pointer ke string kosong akan mengosongkan
	// kolom (di-trim & dikonversi ke NULL oleh service).
	//
	// SeoScore beda: nil = tidak diubah, tapi TIDAK ADA cara eksplisit untuk
	// mengembalikannya ke NULL lewat API (tidak ada nilai sentinel semacam
	// uuid.Nil milik CoverImageID untuk int) — kolom ini hanya NULL untuk
	// baris pre-migration 000029. Ini sengaja dibiarkan karena frontend
	// selalu mengirim skor hasil hitung, bukan mengosongkannya.
	FocusKeyword      *string
	SecondaryKeywords *string
	SeoScore          *int
}

// UploadImageInput — payload upload gambar (raw JPG/PNG/WebP). Service akan
// decode → re-encode ke WebP → simpan. Kalau input sudah WebP tetap
// re-encoded (normalisasi).
type UploadImageInput struct {
	UploaderID   uuid.UUID
	ArticleID    *uuid.UUID // optional; boleh nil (upload standalone)
	FileReader   io.Reader
	FileSize     int64
	MimeType     string
	OriginalName string
	AltText      string
}

// ImageHandle — dipakai handler untuk stream file.
type ImageHandle struct {
	Image   *model.ArticleImage
	AbsPath string
}

// ListResult — envelope hasil listing dengan total count untuk pagination.
type ListResult struct {
	Items []model.Article `json:"items"`
	Total int64           `json:"total"`
	Limit int             `json:"limit"`
	Page  int             `json:"page"`
}
