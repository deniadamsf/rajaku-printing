// Package designapi is the PUBLIC contract of the design module (§22 —
// cross-module import allowed hanya lewat package api). Saat ini belum ada
// modul lain yg consume interface design; package ini terutama mendefinisikan
// sentinel errors yg handler map ke HTTP.
package designapi

import "errors"

var (
	ErrOrderNotFound        = errors.New("designapi: order not found")
	ErrNotOrderOwner        = errors.New("designapi: caller does not own this order")
	ErrOrderNotDesignReady  = errors.New("designapi: order status doesn't allow design upload (must be dibayar or later design states)")
	ErrDesignSourceMismatch = errors.New("designapi: file role doesn't match order.design_source")
	ErrInvalidRole          = errors.New("designapi: invalid role")
	ErrInvalidMimeType      = errors.New("designapi: file mime type not allowed")
	ErrFileTooLarge         = errors.New("designapi: uploaded file exceeds size limit")
	ErrFileEmpty            = errors.New("designapi: uploaded file is empty")

	ErrFileNotFound = errors.New("designapi: design file not found")
	ErrFilePurged   = errors.New("designapi: design file already purged from disk (retention §19)")

	// Staff draft flow
	ErrPendingDraftExists    = errors.New("designapi: another staff draft is still pending customer response")
	ErrDraftAlreadyReviewed  = errors.New("designapi: draft already approved / revision-requested")
	ErrRevisionNotesRequired = errors.New("designapi: revision notes required")

	ErrWalkinOnlyForPOS  = errors.New("designapi: instant walk-in approval only valid for POS orders with design_approval_mode=instant_walkin")
	ErrOrderStateChanged = errors.New("designapi: order state changed concurrently — refresh & retry")

	// Skip-upload flow (§11 POS shortcut) — staff lewati upload file karena
	// desainer sudah punya filenya di luar sistem.
	ErrSkipUploadOnlyForPOS = errors.New("designapi: skip upload only valid for POS walk-in orders")
	ErrSkipNoteRequired     = errors.New("designapi: note required when skipping design upload")

	// ErrOrderItemMismatch — order_item_id yang dikirim caller tidak
	// ditemukan pada daftar order_items milik order yang dituju (§32.5).
	// Design service HANYA bisa melihat order.Items — proyeksi dari
	// orderapi.OrderSummary yang sudah dibatasi ke SATU order (dari resi/ID
	// yang diminta) — jadi "item tidak ada sama sekali" dan "item ada tapi
	// milik order LAIN" tidak bisa dibedakan dari sisi sini, dan keduanya
	// wajib ditolak sama tegasnya.
	ErrOrderItemMismatch = errors.New("designapi: order_item_id tidak ditemukan pada order ini")
)
