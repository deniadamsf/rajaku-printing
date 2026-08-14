// Package invoiceapi is the PUBLIC contract of the invoice module (§22).
// Modul lain (payment, POS) hanya boleh import package ini untuk trigger
// generate invoice.
package invoiceapi

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var (
	ErrOrderNotFound        = errors.New("invoiceapi: order not found")
	ErrInvoiceNotFound      = errors.New("invoiceapi: invoice not found")
	ErrOrderNotInvoiceable  = errors.New("invoiceapi: order not in a state to be invoiced (must be dibayar or later)")
	ErrInvoiceAlreadyExists = errors.New("invoiceapi: invoice already exists for this order (use Regenerate)")
)

// Generator — kontrak yang di-consume payment/POS untuk auto-generate
// invoice setelah payment settled. Best-effort per §13:
//   - Return nil kalau sudah ada invoice (idempotent).
//   - Return non-nil error kalau gagal — caller SEBAIKNYA log & continue
//     (jangan bikin payment approval gagal cuma karena render PDF error).
type Generator interface {
	// GenerateForOrder create-or-noop invoice untuk order. actorID nil kalau
	// system-triggered (mis. dari payment.ApproveProof); isi UserID staff
	// kalau di-trigger manual dari admin panel.
	// Return-nya invoice row (untuk logging/download link building).
	GenerateForOrder(ctx context.Context, orderID uuid.UUID, actorID *uuid.UUID) (*Info, error)
}

// Info — projection minimal invoice yg dishare ke consumer (payment, notif).
// Hindari expose full model biar cross-module coupling minim.
type Info struct {
	ID             uuid.UUID
	OrderID        uuid.UUID
	InvoiceNumber  string
	Version        int
	DownloadURL    string // absolute URL (dibangun dari config.App.BaseURL)
}
