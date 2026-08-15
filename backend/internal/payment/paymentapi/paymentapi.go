// Package paymentapi is the PUBLIC contract of the payment module — only this
// package boleh diimport oleh modul lain. Per spec §22 (komunikasi antar modul
// via interface, bukan import package internal).
package paymentapi

import "errors"

var (
	ErrProofNotFound        = errors.New("paymentapi: payment proof not found")
	ErrOrderNotPayable      = errors.New("paymentapi: order status doesn't allow proof upload (must be menunggu_pembayaran or ditolak)")
	ErrPendingProofExists   = errors.New("paymentapi: order already has a pending proof; wait for review or contact staff")
	ErrProofAlreadyReviewed = errors.New("paymentapi: proof already approved or rejected — cannot re-review")
	ErrInvalidMetodeBayar   = errors.New("paymentapi: metode_bayar must be transfer or qris")
	ErrInvalidMimeType      = errors.New("paymentapi: file mime type not allowed (jpg/png/webp/pdf only)")
	ErrFileTooLarge         = errors.New("paymentapi: uploaded file exceeds size limit")
	ErrFileEmpty            = errors.New("paymentapi: uploaded file is empty")
	ErrRejectReasonRequired = errors.New("paymentapi: reject reason must not be empty")
	ErrNotOrderOwner        = errors.New("paymentapi: caller does not own this order")
)
