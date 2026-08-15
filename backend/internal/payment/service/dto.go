// Package service — payment business logic.
package service

import (
	"io"

	"github.com/google/uuid"

	paymodel "github.com/rajaku-printing/backend/internal/payment/model"
)

// UploadProofInput — everything the service needs to accept a customer's bukti
// transfer/QRIS. Caller (handler) is responsible for extracting file bytes from
// multipart form and passing an io.Reader here.
type UploadProofInput struct {
	Resi          string    // order tracker
	CallerID      uuid.UUID // authenticated user (customer OR staff on behalf of)
	IsStaff       bool      // if true, ownership check is skipped
	MetodeBayar   paymodel.MetodeBayar
	AmountClaimed *int64
	FileReader    io.Reader
	FileSize      int64 // MUST be pre-computed by handler (Content-Length or io.Copy tee) so service can validate before streaming to disk
	OriginalName  string
	MimeType      string // as reported by client; service will re-check the allowlist

	// ScopedOrderID — non-nil kalau caller memakai token guest_order
	// (authapi.Identity.OrderID, minted by POST /lacak/:resi/verify). nil untuk
	// sesi penuh. Diteruskan ke checkScopedOrder di service supaya token yang
	// terbit untuk satu order tidak bisa dipakai upload bukti ke order lain
	// milik customer_id yang sama.
	ScopedOrderID *uuid.UUID
}

// ReviewInput — approve/reject payload.
type ReviewInput struct {
	ProofID uuid.UUID
	StaffID uuid.UUID
	Reason  string // required when rejecting
}

// ListInput — filter for staff dashboard.
type ListInput struct {
	Status   paymodel.ProofStatus
	OrderID  *uuid.UUID
	Page     int
	PageSize int
}

// ListPage — paginated result.
type ListPage struct {
	Items    []paymodel.PaymentProof
	Total    int64
	Page     int
	PageSize int
}
