// Package model contains GORM entities for the payment module.
package model

import (
	"time"

	"github.com/google/uuid"
)

type ProofStatus string

const (
	ProofPending  ProofStatus = "pending"
	ProofApproved ProofStatus = "approved"
	ProofRejected ProofStatus = "rejected"
)

type MetodeBayar string

const (
	MetodeBayarTransfer MetodeBayar = "transfer"
	MetodeBayarQRIS     MetodeBayar = "qris"
)

type PaymentProof struct {
	ID          uuid.UUID   `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	OrderID     uuid.UUID   `gorm:"type:uuid;not null;index"                       json:"order_id"`
	MetodeBayar MetodeBayar `gorm:"size:20;not null"                               json:"metode_bayar"`

	FilePath         string `gorm:"not null"                                json:"-"` // internal only; served via signed endpoint
	FileOriginalName string `gorm:"size:255;not null"                       json:"file_original_name"`
	FileSizeBytes    int64  `gorm:"not null"                                json:"file_size_bytes"`
	FileMimeType     string `gorm:"size:100;not null"                       json:"file_mime_type"`

	AmountClaimed *int64 `                                             json:"amount_claimed,omitempty"`

	UploadedAt time.Time  `gorm:"not null;default:now()"                json:"uploaded_at"`
	UploadedBy *uuid.UUID `gorm:"type:uuid"                             json:"uploaded_by,omitempty"`

	Status       ProofStatus `gorm:"size:20;not null"                      json:"status"`
	ReviewedAt   *time.Time  `                                             json:"reviewed_at,omitempty"`
	ReviewedBy   *uuid.UUID  `gorm:"type:uuid"                             json:"reviewed_by,omitempty"`
	RejectReason *string     `                                             json:"reject_reason,omitempty"`
}

func (PaymentProof) TableName() string { return "payment_proofs" }
