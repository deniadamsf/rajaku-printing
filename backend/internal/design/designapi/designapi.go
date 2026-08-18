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
)
