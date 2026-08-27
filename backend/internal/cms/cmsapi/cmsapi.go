// Package cmsapi — public contract modul CMS (§22). Untuk sekarang berisi
// sentinel error yang di-map handler ke HTTP status. Belum ada modul lain
// yang meng-consume interface CMS.
package cmsapi

import "errors"

var (
	ErrArticleNotFound = errors.New("cmsapi: article not found")
	ErrSlugTaken       = errors.New("cmsapi: slug already used by another article")
	ErrInvalidSlug     = errors.New("cmsapi: slug format invalid (a-z0-9 + dash)")
	ErrTitleRequired   = errors.New("cmsapi: title is required")
	ErrContentRequired = errors.New("cmsapi: content is required")
	ErrInvalidStatus   = errors.New("cmsapi: invalid status")
	ErrNotPublishable  = errors.New("cmsapi: article cannot be published (already archived / missing content)")
	ErrInvalidSeoScore = errors.New("cmsapi: seo_score must be between 0 and 100")

	ErrFocusKeywordTooLong      = errors.New("cmsapi: focus_keyword max 100 karakter")
	ErrSecondaryKeywordsTooLong = errors.New("cmsapi: secondary_keywords max 300 karakter")
	ErrAltTextTooLong           = errors.New("cmsapi: alt_text max 255 karakter")

	ErrImageNotFound     = errors.New("cmsapi: image not found")
	ErrImageEmpty        = errors.New("cmsapi: image file is empty")
	ErrImageTooLarge     = errors.New("cmsapi: image exceeds size limit")
	ErrImageInvalidType  = errors.New("cmsapi: image mime type not allowed (png/jpg/webp only)")
	ErrImageDecodeFailed = errors.New("cmsapi: image decode failed (corrupt file?)")
)
