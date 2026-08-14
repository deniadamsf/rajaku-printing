// Package service — invoice generation + WA-auto-send orchestration (§12).
package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/invoice/invoiceapi"
	"github.com/rajaku-printing/backend/internal/invoice/model"
	"github.com/rajaku-printing/backend/internal/invoice/pdfrender"
	invrepo "github.com/rajaku-printing/backend/internal/invoice/repository"
	"github.com/rajaku-printing/backend/internal/notification/notificationapi"
	"github.com/rajaku-printing/backend/internal/order/orderapi"
)

// InvoiceStore — narrowed repository contract untuk testability.
type InvoiceStore interface {
	Create(ctx context.Context, inv *model.Invoice) error
	FindByID(ctx context.Context, id uuid.UUID) (*model.Invoice, error)
	FindByOrderID(ctx context.Context, orderID uuid.UUID) (*model.Invoice, error)
	NextInvoiceNumber(ctx context.Context, year int) (int, error)
}

// FileStore — narrowed filestore contract.
type FileStore interface {
	Save(ctx context.Context, subpath string, r io.Reader) (int64, error)
	Delete(ctx context.Context, subpath string) error
	AbsPath(subpath string) (string, error)
}

// Config — service config injected dari router.go.
type Config struct {
	// BaseURL untuk build download link WA. Dari config.App.BaseURL (§2).
	BaseURL string
	// Company info yg tampil di header PDF.
	Company pdfrender.CompanyInfo
}

type Service struct {
	invoices  InvoiceStore
	blobs     FileStore
	orderCmd  orderapi.OrderCommandService
	customers authapi.CustomerService
	notifier  notificationapi.Enqueuer
	cfg       Config
	nowFn     func() time.Time
}

var _ invoiceapi.Generator = (*Service)(nil)

func New(
	invoices InvoiceStore,
	blobs FileStore,
	orderCmd orderapi.OrderCommandService,
	customers authapi.CustomerService,
	cfg Config,
) *Service {
	return &Service{
		invoices:  invoices,
		blobs:     blobs,
		orderCmd:  orderCmd,
		customers: customers,
		cfg:       cfg,
		nowFn:     time.Now,
	}
}

func (s *Service) SetNotifier(n notificationapi.Enqueuer) { s.notifier = n }

// GenerateForOrder implements invoiceapi.Generator. Idempotent — kalau
// invoice sudah ada untuk order, return existing info (log info, tidak error).
func (s *Service) GenerateForOrder(ctx context.Context, orderID uuid.UUID, actorID *uuid.UUID) (*invoiceapi.Info, error) {
	// Idempotency check.
	existing, err := s.invoices.FindByOrderID(ctx, orderID)
	if err == nil {
		log.Ctx(ctx).Debug().
			Str("order_id", orderID.String()).
			Str("invoice_number", existing.InvoiceNumber).
			Msg("invoice already exists — returning existing")
		return s.toInfo(existing), nil
	}
	if !errors.Is(err, invrepo.ErrNotFound) {
		return nil, fmt.Errorf("check existing invoice: %w", err)
	}

	// Load order + customer.
	view, err := s.orderCmd.FindInvoiceViewByID(ctx, orderID)
	if err != nil {
		if errors.Is(err, orderapi.ErrOrderNotFound) {
			return nil, invoiceapi.ErrOrderNotFound
		}
		return nil, fmt.Errorf("load order for invoice: %w", err)
	}
	// State guard — hanya generate kalau order sudah dibayar minimal.
	switch view.Status {
	case "dibayar", "desain_diverifikasi", "desain_dikerjakan", "menunggu_approval_desain",
		"proses_cetak", "qc", "siap_kirim", "siap_ambil", "dikirim", "selesai":
		// OK
	default:
		return nil, invoiceapi.ErrOrderNotInvoiceable
	}

	cust, err := s.customers.FindByID(ctx, view.CustomerID)
	if err != nil {
		return nil, fmt.Errorf("load customer for invoice: %w", err)
	}

	// Assign invoice number (atomic per-year).
	now := s.nowFn().UTC()
	yearLocal := now.In(jakartaTZ()).Year()
	seq, err := s.invoices.NextInvoiceNumber(ctx, yearLocal)
	if err != nil {
		return nil, fmt.Errorf("assign invoice number: %w", err)
	}
	invoiceNumber := fmt.Sprintf("INV/%04d/%05d", yearLocal, seq)

	// Render PDF.
	meta := pdfrender.Meta{
		InvoiceNumber: invoiceNumber,
		GeneratedAt:   now,
	}
	pdfBytes, err := pdfrender.Render(s.cfg.Company, meta, view, cust.Name, cust.Phone)
	if err != nil {
		return nil, fmt.Errorf("render pdf: %w", err)
	}

	// Save to disk.
	invID := uuid.New()
	subpath := path.Join(
		"invoices",
		fmt.Sprintf("%04d", yearLocal),
		invID.String()+".pdf",
	)
	written, err := s.blobs.Save(ctx, subpath, bytes.NewReader(pdfBytes))
	if err != nil {
		return nil, fmt.Errorf("save pdf blob: %w", err)
	}
	if written != int64(len(pdfBytes)) {
		_ = s.blobs.Delete(ctx, subpath)
		return nil, fmt.Errorf("declared size %d != written %d", len(pdfBytes), written)
	}

	// Insert row.
	inv := &model.Invoice{
		ID:            invID,
		OrderID:       orderID,
		InvoiceNumber: invoiceNumber,
		PDFPath:       subpath,
		PDFSizeBytes:  written,
		Version:       1,
		GeneratedAt:   now,
		UpdatedAt:     now,
		GeneratedBy:   actorID,
	}
	if err := s.invoices.Create(ctx, inv); err != nil {
		_ = s.blobs.Delete(ctx, subpath)
		if errors.Is(err, invrepo.ErrDuplicateOrder) {
			// Race — another request created it while we were rendering.
			// Re-fetch & return that one.
			if again, ferr := s.invoices.FindByOrderID(ctx, orderID); ferr == nil {
				return s.toInfo(again), nil
			}
			return nil, invoiceapi.ErrInvoiceAlreadyExists
		}
		return nil, fmt.Errorf("insert invoice row: %w", err)
	}

	info := s.toInfo(inv)

	// Auto-send WA (§12) — best-effort, non-blocking business.
	s.enqueueInvoiceWA(ctx, orderID, info)

	return info, nil
}

// GetFile — resolve invoice by ID + return filesystem handle. Public endpoint:
// tidak butuh auth (UUID cukup unguessable). Handler stream via http.ServeFile.
func (s *Service) GetFile(ctx context.Context, id uuid.UUID) (*FileHandle, error) {
	inv, err := s.invoices.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, invrepo.ErrNotFound) {
			return nil, invoiceapi.ErrInvoiceNotFound
		}
		return nil, fmt.Errorf("lookup invoice: %w", err)
	}
	abs, err := s.blobs.AbsPath(inv.PDFPath)
	if err != nil {
		log.Ctx(ctx).Error().Err(err).
			Str("invoice_id", inv.ID.String()).
			Str("subpath", inv.PDFPath).
			Msg("invoice path rejected — cek DB")
		return nil, fmt.Errorf("resolve invoice path: %w", err)
	}
	return &FileHandle{Invoice: inv, AbsPath: abs}, nil
}

// ---- helpers ----

func (s *Service) toInfo(inv *model.Invoice) *invoiceapi.Info {
	return &invoiceapi.Info{
		ID:            inv.ID,
		OrderID:       inv.OrderID,
		InvoiceNumber: inv.InvoiceNumber,
		Version:       inv.Version,
		DownloadURL:   s.cfg.BaseURL + "/api/v1/invoices/" + inv.ID.String() + "/pdf",
	}
}

func (s *Service) enqueueInvoiceWA(ctx context.Context, orderID uuid.UUID, info *invoiceapi.Info) {
	if s.notifier == nil {
		return
	}
	err := s.notifier.EnqueueOrderEvent(ctx, notificationapi.KindInvoiceReady, orderID, map[string]any{
		"invoice_url":    info.DownloadURL,
		"invoice_number": info.InvoiceNumber,
	})
	if err != nil {
		log.Ctx(ctx).Error().Err(err).
			Str("order_id", orderID.String()).
			Str("invoice_number", info.InvoiceNumber).
			Msg("enqueue invoice_ready WA failed — link tidak terkirim, admin bisa share manual")
	}
}

// jakartaTZ — WIB (UTC+7). Nomor invoice pakai tahun lokal Indonesia,
// bukan UTC (mencegah "1 Jan pagi WIB masih 2025 di UTC").
func jakartaTZ() *time.Location {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		// Fallback fixed offset — cukup untuk MVP kalau tzdata tidak ada.
		return time.FixedZone("WIB", 7*3600)
	}
	return loc
}
