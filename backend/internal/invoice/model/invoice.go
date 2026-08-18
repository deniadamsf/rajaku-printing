// Package model contains GORM entities for the invoice module.
package model

import (
	"time"

	"github.com/google/uuid"
)

type Invoice struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	OrderID        uuid.UUID `gorm:"type:uuid;not null;uniqueIndex"                 json:"order_id"`
	InvoiceNumber  string    `gorm:"size:30;not null;uniqueIndex"                   json:"invoice_number"`
	PDFPath        string    `gorm:"not null;column:pdf_path"                       json:"-"`
	PDFSizeBytes   int64     `gorm:"not null;column:pdf_size_bytes"                 json:"pdf_size_bytes"`
	Version        int       `gorm:"not null;default:1"                             json:"version"`
	GeneratedAt    time.Time `gorm:"not null;default:now()"                         json:"generated_at"`
	UpdatedAt      time.Time `gorm:"not null;default:now()"                         json:"updated_at"`
	GeneratedBy    *uuid.UUID `gorm:"type:uuid"                                     json:"generated_by,omitempty"`
}

func (Invoice) TableName() string { return "invoices" }

// InvoiceCounter — internal counter table (per year). Bukan expose ke luar
// service; hanya repository yg baca/tulis.
type InvoiceCounter struct {
	Year       int `gorm:"primaryKey"`
	LastNumber int `gorm:"not null;default:0"`
}

func (InvoiceCounter) TableName() string { return "invoice_counters" }
