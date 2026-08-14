// Package handler — HTTP layer for payment module.
package handler

import (
	"time"

	"github.com/google/uuid"

	paymodel "github.com/rajaku-printing/backend/internal/payment/model"
)

// ---- Admin request DTOs ----

type rejectRequest struct {
	Reason string `json:"reason" binding:"required,min=3,max=500"`
}

// approveRequest — currently empty (staff clicks "approve"; metode_bayar comes
// from the proof row itself). Kept as a type so future fields (e.g. bank name
// on the receiving side) can be added without changing the endpoint signature.
type approveRequest struct{}

// ---- Response ----

type proofResponse struct {
	ID               uuid.UUID `json:"id"`
	OrderID          uuid.UUID `json:"order_id"`
	MetodeBayar      string    `json:"metode_bayar"`
	FileOriginalName string    `json:"file_original_name"`
	FileSizeBytes    int64     `json:"file_size_bytes"`
	FileMimeType     string    `json:"file_mime_type"`
	AmountClaimed    *int64    `json:"amount_claimed,omitempty"`
	UploadedAt       string    `json:"uploaded_at"`
	Status           string    `json:"status"`
	ReviewedAt       string    `json:"reviewed_at,omitempty"`
	ReviewedBy       string    `json:"reviewed_by,omitempty"`
	RejectReason     string    `json:"reject_reason,omitempty"`
}

func toProofResponse(p *paymodel.PaymentProof) proofResponse {
	r := proofResponse{
		ID:               p.ID,
		OrderID:          p.OrderID,
		MetodeBayar:      string(p.MetodeBayar),
		FileOriginalName: p.FileOriginalName,
		FileSizeBytes:    p.FileSizeBytes,
		FileMimeType:     p.FileMimeType,
		AmountClaimed:    p.AmountClaimed,
		UploadedAt:       p.UploadedAt.UTC().Format(time.RFC3339),
		Status:           string(p.Status),
	}
	if p.ReviewedAt != nil {
		r.ReviewedAt = p.ReviewedAt.UTC().Format(time.RFC3339)
	}
	if p.ReviewedBy != nil {
		r.ReviewedBy = p.ReviewedBy.String()
	}
	if p.RejectReason != nil {
		r.RejectReason = *p.RejectReason
	}
	return r
}
