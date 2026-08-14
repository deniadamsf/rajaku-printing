package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/rajaku-printing/backend/internal/invoice/invoiceapi"
	"github.com/rajaku-printing/backend/internal/notification/notificationapi"
	"github.com/rajaku-printing/backend/internal/order/orderapi"
	paymodel "github.com/rajaku-printing/backend/internal/payment/model"
	"github.com/rajaku-printing/backend/internal/payment/paymentapi"
	payrepo "github.com/rajaku-printing/backend/internal/payment/repository"
)

// ProofStore — narrowed storage contract for testability. *payrepo.ProofRepository
// satisfies this.
type ProofStore interface {
	Create(ctx context.Context, p *paymodel.PaymentProof) error
	FindByID(ctx context.Context, id uuid.UUID) (*paymodel.PaymentProof, error)
	FindPendingByOrder(ctx context.Context, orderID uuid.UUID) (*paymodel.PaymentProof, error)
	List(ctx context.Context, f payrepo.ListFilter) (*payrepo.ListResult, error)
	Review(ctx context.Context, p payrepo.ReviewParams) error
	DeleteRow(ctx context.Context, id uuid.UUID) error
}

// FileSaver — narrowed filestore contract. *filestore.Store satisfies this.
type FileSaver interface {
	Save(ctx context.Context, subpath string, r io.Reader) (int64, error)
	Delete(ctx context.Context, subpath string) error
	// AbsPath resolves a stored subpath to an absolute filesystem path (with
	// path-traversal guarded). Used by GetProofFile to hand off to
	// http.ServeFile without leaking storage layout into handlers.
	AbsPath(subpath string) (string, error)
}

// Allowed mime types for payment proofs — customers usually upload photos
// (screenshot / kamera) or PDF struk. Anything else likely a bug or attack.
var allowedMimeTypes = map[string]string{
	"image/jpeg":      ".jpg",
	"image/png":       ".png",
	"image/webp":      ".webp",
	"application/pdf": ".pdf",
}

// Service coordinates payment_proof storage with order state transitions.
type Service struct {
	proofs      ProofStore
	files       FileSaver
	orderCmd    orderapi.OrderCommandService
	notifier    notificationapi.Enqueuer // opsional; nil = tidak trigger WA
	invoiceGen  invoiceapi.Generator     // opsional; nil = tidak auto-generate invoice
	maxSizeByte int64
	nowFn       func() time.Time // injectable for tests
}

// SetNotifier wires optional notification enqueuer. Dipanggil sekali dari
// router.go setelah notification.Service dibuat. Best-effort — kalau nil,
// approve/reject tetap jalan tanpa kirim WA.
func (s *Service) SetNotifier(n notificationapi.Enqueuer) { s.notifier = n }

// SetInvoiceGenerator wires optional invoice generator. Dipanggil sekali dari
// router.go. Kalau nil, ApproveProof tetap jalan tanpa auto-generate PDF
// invoice (spec §12 auto-send WA).
func (s *Service) SetInvoiceGenerator(g invoiceapi.Generator) { s.invoiceGen = g }

type Config struct {
	MaxUploadMB int
}

func New(proofs ProofStore, files FileSaver, orderCmd orderapi.OrderCommandService, cfg Config) *Service {
	maxMB := cfg.MaxUploadMB
	if maxMB <= 0 {
		maxMB = 5
	}
	return &Service{
		proofs:      proofs,
		files:       files,
		orderCmd:    orderCmd,
		maxSizeByte: int64(maxMB) * 1024 * 1024,
		nowFn:       time.Now,
	}
}

// UploadProof stores a customer's bukti transfer/QRIS and advances the order to
// menunggu_verifikasi. Ordering:
//
//  1. Lookup order (must be menunggu_pembayaran or ditolak).
//  2. Verify caller is the order owner (unless staff).
//  3. Ensure no pending proof exists yet for the order.
//  4. Validate file (mime type + size).
//  5. Save file to disk (temp+rename via filestore).
//  6. Insert proof row (status=pending).
//  7. Trigger order.MarkPendingVerification.
//
// If step 7 fails, we roll back (delete proof row + file). File is on disk BEFORE
// row insert only because we need bytes to persist before referencing them. If
// step 6 fails the file is orphaned — caller sees the error, and an eventual
// cleanup job (out of scope for MVP) can sweep unreferenced files.
func (s *Service) UploadProof(ctx context.Context, in UploadProofInput) (*paymodel.PaymentProof, error) {
	// 1. Order lookup + status guard
	order, err := s.orderCmd.FindSummaryByResi(ctx, in.Resi)
	if err != nil {
		return nil, err // orderapi errors bubble as-is (handler maps to HTTP)
	}
	switch order.Status {
	case "menunggu_pembayaran", "ditolak":
		// OK — customer can upload initial or replacement proof
	case "dibayar", "selesai", "dibatalkan":
		return nil, orderapi.ErrPaymentAlreadySettled
	default:
		return nil, paymentapi.ErrOrderNotPayable
	}

	// 2. Ownership
	if !in.IsStaff && order.CustomerID != in.CallerID {
		return nil, paymentapi.ErrNotOrderOwner
	}

	// 3. No dup pending
	if _, err := s.proofs.FindPendingByOrder(ctx, order.ID); err == nil {
		return nil, paymentapi.ErrPendingProofExists
	} else if !errors.Is(err, payrepo.ErrNotFound) {
		return nil, fmt.Errorf("check pending proof: %w", err)
	}

	// 4. Validate metode_bayar + file meta
	if in.MetodeBayar != paymodel.MetodeBayarTransfer && in.MetodeBayar != paymodel.MetodeBayarQRIS {
		return nil, paymentapi.ErrInvalidMetodeBayar
	}
	ext, mimeOK := allowedMimeTypes[strings.ToLower(in.MimeType)]
	if !mimeOK {
		return nil, paymentapi.ErrInvalidMimeType
	}
	if in.FileSize <= 0 {
		return nil, paymentapi.ErrFileEmpty
	}
	if in.FileSize > s.maxSizeByte {
		return nil, paymentapi.ErrFileTooLarge
	}

	// 5. Save file. Deterministic path so re-uploads for same proof (unused)
	// don't collide with each other's dirs.
	now := s.nowFn().UTC()
	proofID := uuid.New()
	subpath := path.Join(
		"payment_proofs",
		fmt.Sprintf("%04d", now.Year()),
		fmt.Sprintf("%02d", now.Month()),
		proofID.String()+ext,
	)
	written, err := s.files.Save(ctx, subpath, in.FileReader)
	if err != nil {
		return nil, fmt.Errorf("save file: %w", err)
	}
	if written != in.FileSize {
		// Defensive: if declared size mismatches actual, treat as invalid upload.
		_ = s.files.Delete(ctx, subpath)
		return nil, fmt.Errorf("declared size %d != written %d", in.FileSize, written)
	}

	// 6. Insert row
	proof := &paymodel.PaymentProof{
		ID:               proofID,
		OrderID:          order.ID,
		MetodeBayar:      in.MetodeBayar,
		FilePath:         subpath,
		FileOriginalName: safeOriginalName(in.OriginalName),
		FileSizeBytes:    written,
		FileMimeType:     strings.ToLower(in.MimeType),
		AmountClaimed:    in.AmountClaimed,
		UploadedAt:       now,
		UploadedBy:       &in.CallerID,
		Status:           paymodel.ProofPending,
	}
	if err := s.proofs.Create(ctx, proof); err != nil {
		_ = s.files.Delete(ctx, subpath)
		if errors.Is(err, payrepo.ErrPendingConflict) {
			return nil, paymentapi.ErrPendingProofExists
		}
		return nil, fmt.Errorf("create proof row: %w", err)
	}

	// 7. Advance order to menunggu_verifikasi
	actor := in.CallerID
	if err := s.orderCmd.MarkPendingVerification(ctx, order.ID, &actor, "customer uploaded proof"); err != nil {
		// Roll back the proof row + file so the system is consistent — customer
		// gets an error and can retry cleanly.
		if delErr := s.proofs.DeleteRow(ctx, proof.ID); delErr != nil {
			log.Ctx(ctx).Error().Err(delErr).
				Str("proof_id", proof.ID.String()).
				Msg("rollback: delete proof row failed — manual cleanup required")
		}
		if delErr := s.files.Delete(ctx, subpath); delErr != nil {
			log.Ctx(ctx).Error().Err(delErr).
				Str("subpath", subpath).
				Msg("rollback: delete proof file failed — orphaned file")
		}
		return nil, fmt.Errorf("advance order to verifikasi: %w", err)
	}
	return proof, nil
}

// ApproveProof marks the proof approved AND transitions the order to `dibayar`
// (with metode_bayar recorded on the order). Idempotency: if the proof is
// already reviewed, returns ErrProofAlreadyReviewed.
func (s *Service) ApproveProof(ctx context.Context, in ReviewInput) (*paymodel.PaymentProof, error) {
	proof, err := s.proofs.FindByID(ctx, in.ProofID)
	if err != nil {
		if errors.Is(err, payrepo.ErrNotFound) {
			return nil, paymentapi.ErrProofNotFound
		}
		return nil, fmt.Errorf("lookup proof: %w", err)
	}
	if proof.Status != paymodel.ProofPending {
		return nil, paymentapi.ErrProofAlreadyReviewed
	}

	now := s.nowFn().UTC()
	if err := s.proofs.Review(ctx, payrepo.ReviewParams{
		ID:         in.ProofID,
		NewStatus:  paymodel.ProofApproved,
		ReviewedBy: in.StaffID,
		ReviewedAt: now,
	}); err != nil {
		if errors.Is(err, payrepo.ErrAlreadyReviewed) {
			return nil, paymentapi.ErrProofAlreadyReviewed
		}
		if errors.Is(err, payrepo.ErrNotFound) {
			return nil, paymentapi.ErrProofNotFound
		}
		return nil, fmt.Errorf("mark approved: %w", err)
	}

	if err := s.orderCmd.MarkDibayar(ctx, proof.OrderID, string(proof.MetodeBayar), &in.StaffID, "payment proof approved"); err != nil {
		// Do NOT auto-rollback the proof review — the payment IS validated; the
		// order status advance failing means a race/inconsistency the staff must
		// inspect. Surface the error; keep proof.approved.
		log.Ctx(ctx).Error().Err(err).
			Str("proof_id", proof.ID.String()).
			Str("order_id", proof.OrderID.String()).
			Msg("proof approved but order MarkDibayar failed — manual reconciliation")
		return nil, fmt.Errorf("advance order to dibayar: %w", err)
	}

	proof.Status = paymodel.ProofApproved
	proof.ReviewedAt = &now
	proof.ReviewedBy = &in.StaffID

	s.enqueueNotif(ctx, notificationapi.KindPaymentVerified, proof.OrderID, nil)
	// Auto-generate invoice (§12) — best-effort. Invoice module akan enqueue
	// KindInvoiceReady WA sendiri, jadi payment tidak perlu track link download.
	s.autoGenerateInvoice(ctx, proof.OrderID, in.StaffID)
	return proof, nil
}

// autoGenerateInvoice — best-effort trigger. Payment tetap dianggap sukses
// meskipun render/save PDF gagal (§13 semua notif best-effort).
func (s *Service) autoGenerateInvoice(ctx context.Context, orderID uuid.UUID, staffID uuid.UUID) {
	if s.invoiceGen == nil {
		return
	}
	actor := staffID
	if _, err := s.invoiceGen.GenerateForOrder(ctx, orderID, &actor); err != nil {
		log.Ctx(ctx).Error().Err(err).
			Str("order_id", orderID.String()).
			Msg("auto-generate invoice failed — payment sukses, cek manual di admin panel")
	}
}

// RejectProof marks the proof rejected AND transitions the order back to
// `ditolak` so the customer can upload a new one.
func (s *Service) RejectProof(ctx context.Context, in ReviewInput) (*paymodel.PaymentProof, error) {
	if strings.TrimSpace(in.Reason) == "" {
		return nil, paymentapi.ErrRejectReasonRequired
	}
	proof, err := s.proofs.FindByID(ctx, in.ProofID)
	if err != nil {
		if errors.Is(err, payrepo.ErrNotFound) {
			return nil, paymentapi.ErrProofNotFound
		}
		return nil, fmt.Errorf("lookup proof: %w", err)
	}
	if proof.Status != paymodel.ProofPending {
		return nil, paymentapi.ErrProofAlreadyReviewed
	}

	now := s.nowFn().UTC()
	if err := s.proofs.Review(ctx, payrepo.ReviewParams{
		ID:           in.ProofID,
		NewStatus:    paymodel.ProofRejected,
		ReviewedBy:   in.StaffID,
		ReviewedAt:   now,
		RejectReason: in.Reason,
	}); err != nil {
		if errors.Is(err, payrepo.ErrAlreadyReviewed) {
			return nil, paymentapi.ErrProofAlreadyReviewed
		}
		if errors.Is(err, payrepo.ErrNotFound) {
			return nil, paymentapi.ErrProofNotFound
		}
		return nil, fmt.Errorf("mark rejected: %w", err)
	}

	if err := s.orderCmd.MarkDitolak(ctx, proof.OrderID, &in.StaffID, in.Reason); err != nil {
		log.Ctx(ctx).Error().Err(err).
			Str("proof_id", proof.ID.String()).
			Str("order_id", proof.OrderID.String()).
			Msg("proof rejected but order MarkDitolak failed — manual reconciliation")
		return nil, fmt.Errorf("advance order to ditolak: %w", err)
	}

	proof.Status = paymodel.ProofRejected
	proof.ReviewedAt = &now
	proof.ReviewedBy = &in.StaffID
	reason := in.Reason
	proof.RejectReason = &reason

	s.enqueueNotif(ctx, notificationapi.KindPaymentRejected, proof.OrderID,
		map[string]any{"reason": in.Reason})
	return proof, nil
}

// enqueueNotif — best-effort WA trigger. Error di-log tapi tidak di-return
// karena payment sudah tersimpan; notif gagal tidak boleh bikin approve/reject
// keliatan gagal ke staff (§13 best-effort).
func (s *Service) enqueueNotif(ctx context.Context, kind notificationapi.Kind, orderID uuid.UUID, extras map[string]any) {
	if s.notifier == nil {
		return
	}
	if err := s.notifier.EnqueueOrderEvent(ctx, kind, orderID, extras); err != nil {
		log.Ctx(ctx).Error().Err(err).
			Str("order_id", orderID.String()).
			Str("kind", string(kind)).
			Msg("enqueue notification failed — WA tidak terkirim, cek manual")
	}
}

// ProofFileHandle — everything the handler needs to serve a proof file.
// Kept small so handler doesn't leak into filesystem details on its own.
type ProofFileHandle struct {
	Proof   *paymodel.PaymentProof
	AbsPath string
}

// GetProofFile authorizes access to a proof's underlying file and returns the
// absolute path (so handler can stream via http.ServeFile). Access rules:
//   - Staff: always allowed.
//   - Otherwise: caller must be the owner of the order the proof belongs to.
//
// Returns paymentapi.ErrProofNotFound if the proof doesn't exist, or
// paymentapi.ErrNotOrderOwner if the customer doesn't own the parent order.
func (s *Service) GetProofFile(ctx context.Context, proofID uuid.UUID, callerID uuid.UUID, isStaff bool) (*ProofFileHandle, error) {
	proof, err := s.proofs.FindByID(ctx, proofID)
	if err != nil {
		if errors.Is(err, payrepo.ErrNotFound) {
			return nil, paymentapi.ErrProofNotFound
		}
		return nil, fmt.Errorf("lookup proof: %w", err)
	}

	if !isStaff {
		summary, err := s.orderCmd.FindSummaryByID(ctx, proof.OrderID)
		if err != nil {
			// If the parent order is gone (shouldn't happen due to FK RESTRICT)
			// or lookup failed, treat as not-found to avoid leaking existence.
			return nil, paymentapi.ErrProofNotFound
		}
		if summary.CustomerID != callerID {
			return nil, paymentapi.ErrNotOrderOwner
		}
	}

	abs, err := s.files.AbsPath(proof.FilePath)
	if err != nil {
		// Programmer error / attack — path traversal or empty. Log for visibility.
		log.Ctx(ctx).Error().Err(err).
			Str("proof_id", proof.ID.String()).
			Str("subpath", proof.FilePath).
			Msg("proof file path rejected by filestore — check DB for bad row")
		return nil, fmt.Errorf("resolve file path: %w", err)
	}
	return &ProofFileHandle{Proof: proof, AbsPath: abs}, nil
}

// ListProofs paginated for staff dashboard.
func (s *Service) ListProofs(ctx context.Context, in ListInput) (*ListPage, error) {
	res, err := s.proofs.List(ctx, payrepo.ListFilter{
		Status:   in.Status,
		OrderID:  in.OrderID,
		Page:     in.Page,
		PageSize: in.PageSize,
	})
	if err != nil {
		return nil, fmt.Errorf("list proofs: %w", err)
	}
	return &ListPage{Items: res.Items, Total: res.Total, Page: res.Page, PageSize: res.PageSize}, nil
}

// safeOriginalName strips path separators & control chars from a user-supplied
// filename so we can safely echo it back in responses / notifications without
// worrying about weird UI or log injection. Not used as a filesystem path.
func safeOriginalName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "upload"
	}
	// Strip any directory components — keep the tail only.
	name = path.Base(name)
	// Replace control chars.
	var b strings.Builder
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			b.WriteRune('_')
		} else {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if len(out) > 200 {
		out = out[:200]
	}
	return out
}
