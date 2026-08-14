// Package service holds design module business logic (§6, §11, §19).
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

	"github.com/rajaku-printing/backend/internal/design/designapi"
	"github.com/rajaku-printing/backend/internal/design/model"
	designrepo "github.com/rajaku-printing/backend/internal/design/repository"
	"github.com/rajaku-printing/backend/internal/notification/notificationapi"
	"github.com/rajaku-printing/backend/internal/order/orderapi"
	"github.com/rajaku-printing/backend/internal/settings/settingsapi"
)

// FileStore — narrowed filestore contract. *filestore.Store satisfies this.
type FileStore interface {
	Save(ctx context.Context, subpath string, r io.Reader) (int64, error)
	Delete(ctx context.Context, subpath string) error
	AbsPath(subpath string) (string, error)
}

// DesignStore — narrowed repository contract untuk testability.
type DesignStore interface {
	Create(ctx context.Context, f *model.DesignFile) error
	FindByID(ctx context.Context, id uuid.UUID) (*model.DesignFile, error)
	FindPendingDraftByOrder(ctx context.Context, orderID uuid.UUID) (*model.DesignFile, error)
	ListByOrder(ctx context.Context, orderID uuid.UUID) ([]model.DesignFile, error)
	ReviewDraft(ctx context.Context, p designrepo.ReviewDraftParams) error
	MarkPurged(ctx context.Context, id uuid.UUID, at time.Time) error
	DeleteRow(ctx context.Context, id uuid.UUID) error

	// Retention (§19) — dipakai job sweep & reminder di retention.go.
	ListPurgeCandidates(ctx context.Context, cutoff time.Time, limit int) ([]model.DesignFile, error)
	ListReminderCandidates(ctx context.Context, purgeCutoff, reminderCutoff time.Time, limit int) ([]model.DesignFile, error)
	MarkReminderSent(ctx context.Context, id uuid.UUID, at time.Time) error
}

// allowedMimeTypes — file yg boleh untuk cetak (§6).
//
// Daftar ini SENDIRIAN tidak cukup sebagai gerbang: "application/octet-stream"
// ada di sini karena browser sering mengirimnya untuk CDR/AI yang tidak dikenal,
// dan tanpa pengecekan kedua nilai itu meloloskan file jenis APA PUN. Karena
// itu setiap upload juga wajib lolos allowedExtensions di bawah — lihat
// resolveFileSpec.
var allowedMimeTypes = map[string]struct{}{
	"image/jpeg":      {},
	"image/png":       {},
	"image/webp":      {},
	"application/pdf": {},
	// CorelDRAW — vendor mime string bervariasi, terima yang umum.
	"application/x-cdr":     {},
	"application/cdr":       {},
	"application/coreldraw": {},
	"image/x-coreldraw":     {},
	// Adobe Illustrator
	"application/postscript":  {},
	"application/illustrator": {},
	// Generic — browser tanpa mime tepat. Hanya lolos kalau ekstensinya sah.
	"application/octet-stream": {},
}

// allowedExtensions — sumber kebenaran untuk jenis file yang boleh disimpan,
// beserta apakah browser bisa mem-preview-nya. Ekstensi dipakai (bukan mime)
// karena mime datang dari client dan bisa dipalsukan jadi nilai generik.
// CDR & AI tidak previewable — staff harus download.
// canonicalMime — mime yang DISIMPAN dan disajikan kembali di header
// Content-Type, diturunkan dari ekstensi. Mime mentah dari client tidak boleh
// dipakai untuk itu: browser sering mengirim "application/octet-stream" untuk
// PNG/PDF yang sah, sehingga file yang previewable justru gagal dirender staff
// (atau malah memicu unduhan) karena Content-Type-nya generik.
var allowedExtensions = map[string]struct {
	previewable   bool
	canonicalMime string
}{
	".jpg":  {true, "image/jpeg"},
	".jpeg": {true, "image/jpeg"},
	".png":  {true, "image/png"},
	".webp": {true, "image/webp"},
	".pdf":  {true, "application/pdf"},
	".cdr":  {false, "application/x-cdr"},
	".ai":   {false, "application/postscript"},
}

// resolveFileSpec memvalidasi mime DAN ekstensi nama file asli, lalu
// mengembalikan ekstensi penyimpanan + flag previewable.
//
// Keduanya wajib lolos. Mime sendiri tidak cukup (client bisa mengirim
// "application/octet-stream" untuk file apa pun); ekstensi sendiri juga tidak
// cukup (nama file sepenuhnya dikendalikan client). File tanpa ekstensi yang
// dikenal ditolak — bukan regresi yang berarti karena unggahan asli selalu
// membawa ekstensi, dan menyimpan blob tak dikenal di disk VPS adalah persis
// yang ingin dicegah (§19).
func resolveFileSpec(mimeType, originalName string) (ext string, previewable bool, canonicalMime string, err error) {
	m := strings.ToLower(strings.TrimSpace(mimeType))
	if _, ok := allowedMimeTypes[m]; !ok {
		return "", false, "", designapi.ErrInvalidMimeType
	}
	e := strings.ToLower(path.Ext(strings.TrimSpace(originalName)))
	spec, ok := allowedExtensions[e]
	if !ok {
		return "", false, "", designapi.ErrInvalidMimeType
	}
	return e, spec.previewable, spec.canonicalMime, nil
}

type Service struct {
	files       DesignStore
	blobs       FileStore
	orderCmd    orderapi.OrderCommandService
	notifier    notificationapi.Enqueuer
	maxSizeByte int64
	nowFn       func() time.Time

	// Retention (§19) — di-inject setelah konstruksi lewat setter di
	// retention.go, sama pola dengan notifier (dependency-nya baru ada
	// belakangan di urutan wiring).
	settings settingsapi.Reader
	alerter  notificationapi.InternalAlerter

	retentionDefaultDays  int
	retentionReminderDays int
	retentionBatchLimit   int
}

type Config struct {
	// MaxUploadMB — batas ukuran file design (biasanya CDR bisa besar,
	// naikkan dari default payment). Divalidasi handler juga.
	MaxUploadMB int

	// RetentionDefaultDays — fallback masa retensi kalau setting DB
	// (`design_retention_days`) tidak terbaca. Nilai runtime yang sebenarnya
	// datang dari admin panel, bukan dari sini (§19 configurable).
	RetentionDefaultDays int
	// RetentionReminderDays — H-berapa reminder dikirim ke ops sebelum file
	// dihapus. Default 3 (§19).
	RetentionReminderDays int
	// RetentionBatchLimit — maksimum row yang diproses per run job.
	RetentionBatchLimit int
}

func New(files DesignStore, blobs FileStore, orderCmd orderapi.OrderCommandService, cfg Config) *Service {
	maxMB := cfg.MaxUploadMB
	if maxMB <= 0 {
		maxMB = 25
	}
	return &Service{
		files:                 files,
		blobs:                 blobs,
		orderCmd:              orderCmd,
		maxSizeByte:           int64(maxMB) * 1024 * 1024,
		nowFn:                 time.Now,
		retentionDefaultDays:  cfg.RetentionDefaultDays,
		retentionReminderDays: cfg.RetentionReminderDays,
		retentionBatchLimit:   cfg.RetentionBatchLimit,
	}
}

// SetNotifier — best-effort trigger. Dipanggil router.go setelah notification.Service dibuat.
func (s *Service) SetNotifier(n notificationapi.Enqueuer) { s.notifier = n }

// ---- Customer flow ----

// UploadCustomerFile — customer upload file. Service derive role dari
// order.design_source:
//   - design_source=upload  → role=customer_upload (siap-cetak; staff akan verifikasi)
//   - design_source=request → role=customer_asset  (aset/brief; staff akan kerjakan)
//
// Ownership: caller wajib pemilik order (kecuali staff bypass, mis. kasir POS
// upload atas nama customer).
//
// State guard:
//   - upload path: order harus di `dibayar` (siap terima file cetak).
//   - request path: order boleh di `dibayar` (initial asset) atau
//     `desain_dikerjakan` / `menunggu_approval_desain` (extra asset saat revisi).
func (s *Service) UploadCustomerFile(ctx context.Context, in UploadInput) (*model.DesignFile, error) {
	order, err := s.orderCmd.FindSummaryByResi(ctx, in.Resi)
	if err != nil {
		if errors.Is(err, orderapi.ErrOrderNotFound) {
			return nil, designapi.ErrOrderNotFound
		}
		return nil, fmt.Errorf("upload: lookup order: %w", err)
	}
	if !in.IsStaff && order.CustomerID != in.CallerID {
		return nil, designapi.ErrNotOrderOwner
	}
	if err := checkScopedOrder(in.ScopedOrderID, order.ID); err != nil {
		return nil, err
	}

	// Derive role + state guard.
	var role model.Role
	switch order.DesignSource {
	case "upload":
		role = model.RoleCustomerUpload
		if order.Status != "dibayar" {
			return nil, designapi.ErrOrderNotDesignReady
		}
	case "request":
		role = model.RoleCustomerAsset
		switch order.Status {
		case "dibayar", "desain_dikerjakan", "menunggu_approval_desain":
			// OK
		default:
			return nil, designapi.ErrOrderNotDesignReady
		}
	default:
		return nil, designapi.ErrDesignSourceMismatch
	}

	saved, err := s.persistBlobAndRow(ctx, order.ID, role, in /* approvalPending */, false, nil /* notes */, in.Notes)
	if err != nil {
		return nil, err
	}

	// Kalau ini asset pertama untuk request-desain di status dibayar,
	// otomatis advance ke desain_dikerjakan biar staff tahu bisa mulai kerja.
	if role == model.RoleCustomerAsset && order.Status == "dibayar" {
		actor := in.CallerID
		if err := s.orderCmd.MarkDesainDikerjakan(ctx, order.ID, &actor, "customer asset diterima"); err != nil {
			log.Ctx(ctx).Error().Err(err).
				Str("order_id", order.ID.String()).
				Msg("advance to desain_dikerjakan failed setelah asset upload — cek manual")
			// Non-fatal — file sudah tersimpan.
		}
	}

	return saved, nil
}

// ApproveDraft — customer approve staff draft; advance ke desain_diverifikasi.
func (s *Service) ApproveDraft(ctx context.Context, in ApproveInput) (*model.DesignFile, error) {
	draft, err := s.files.FindByID(ctx, in.DraftID)
	if err != nil {
		if errors.Is(err, designrepo.ErrNotFound) {
			return nil, designapi.ErrFileNotFound
		}
		return nil, fmt.Errorf("approve: lookup draft: %w", err)
	}
	if draft.Role != model.RoleStaffDraft {
		return nil, designapi.ErrInvalidRole
	}
	if draft.ApprovalStatus == nil || *draft.ApprovalStatus != model.ApprovalPending {
		return nil, designapi.ErrDraftAlreadyReviewed
	}

	order, err := s.orderCmd.FindSummaryByID(ctx, draft.OrderID)
	if err != nil {
		return nil, fmt.Errorf("approve: lookup order: %w", err)
	}
	if !in.IsStaff && order.CustomerID != in.CallerID {
		return nil, designapi.ErrNotOrderOwner
	}
	if err := checkScopedOrder(in.ScopedOrderID, order.ID); err != nil {
		return nil, err
	}

	now := s.nowFn().UTC()
	if err := s.files.ReviewDraft(ctx, designrepo.ReviewDraftParams{
		ID:         in.DraftID,
		NewStatus:  model.ApprovalApproved,
		ReviewedBy: in.CallerID,
		ReviewedAt: now,
	}); err != nil {
		if errors.Is(err, designrepo.ErrDraftAlreadyReviewed) {
			return nil, designapi.ErrDraftAlreadyReviewed
		}
		if errors.Is(err, designrepo.ErrNotFound) {
			return nil, designapi.ErrFileNotFound
		}
		return nil, fmt.Errorf("approve: mark reviewed: %w", err)
	}

	actor := in.CallerID
	if err := s.orderCmd.MarkDesainDiverifikasi(ctx, order.ID, &actor, "customer approved draft"); err != nil {
		if errors.Is(err, orderapi.ErrOrderStateChanged) {
			return nil, designapi.ErrOrderStateChanged
		}
		log.Ctx(ctx).Error().Err(err).
			Str("order_id", order.ID.String()).
			Str("draft_id", draft.ID.String()).
			Msg("approve draft: order transition failed — manual reconciliation")
		return nil, fmt.Errorf("approve: advance order: %w", err)
	}

	s.enqueueNotif(ctx, notificationapi.KindDesignApproved, order.ID, nil)

	// Refresh local copy.
	approved := model.ApprovalApproved
	draft.ApprovalStatus = &approved
	draft.ReviewedAt = &now
	draft.ReviewedBy = &in.CallerID
	return draft, nil
}

// RequestRevision — customer minta revisi; state balik ke desain_dikerjakan.
func (s *Service) RequestRevision(ctx context.Context, in RevisionInput) (*model.DesignFile, error) {
	if strings.TrimSpace(in.Notes) == "" {
		return nil, designapi.ErrRevisionNotesRequired
	}
	draft, err := s.files.FindByID(ctx, in.DraftID)
	if err != nil {
		if errors.Is(err, designrepo.ErrNotFound) {
			return nil, designapi.ErrFileNotFound
		}
		return nil, fmt.Errorf("revision: lookup draft: %w", err)
	}
	if draft.Role != model.RoleStaffDraft {
		return nil, designapi.ErrInvalidRole
	}
	if draft.ApprovalStatus == nil || *draft.ApprovalStatus != model.ApprovalPending {
		return nil, designapi.ErrDraftAlreadyReviewed
	}
	order, err := s.orderCmd.FindSummaryByID(ctx, draft.OrderID)
	if err != nil {
		return nil, fmt.Errorf("revision: lookup order: %w", err)
	}
	if !in.IsStaff && order.CustomerID != in.CallerID {
		return nil, designapi.ErrNotOrderOwner
	}
	if err := checkScopedOrder(in.ScopedOrderID, order.ID); err != nil {
		return nil, err
	}

	now := s.nowFn().UTC()
	if err := s.files.ReviewDraft(ctx, designrepo.ReviewDraftParams{
		ID:            in.DraftID,
		NewStatus:     model.ApprovalRevision,
		ReviewedBy:    in.CallerID,
		ReviewedAt:    now,
		RevisionNotes: in.Notes,
	}); err != nil {
		if errors.Is(err, designrepo.ErrDraftAlreadyReviewed) {
			return nil, designapi.ErrDraftAlreadyReviewed
		}
		if errors.Is(err, designrepo.ErrNotFound) {
			return nil, designapi.ErrFileNotFound
		}
		return nil, fmt.Errorf("revision: mark reviewed: %w", err)
	}

	actor := in.CallerID
	if err := s.orderCmd.MarkDesainDikerjakan(ctx, order.ID, &actor, "customer requested revision: "+truncate(in.Notes, 200)); err != nil {
		if errors.Is(err, orderapi.ErrOrderStateChanged) {
			return nil, designapi.ErrOrderStateChanged
		}
		log.Ctx(ctx).Error().Err(err).
			Str("order_id", order.ID.String()).
			Str("draft_id", draft.ID.String()).
			Msg("revision: order transition failed — manual reconciliation")
		return nil, fmt.Errorf("revision: advance order: %w", err)
	}

	s.enqueueNotif(ctx, notificationapi.KindDesignNeedsRevision, order.ID, map[string]any{"note": in.Notes})

	revStatus := model.ApprovalRevision
	notes := in.Notes
	draft.ApprovalStatus = &revStatus
	draft.ReviewedAt = &now
	draft.ReviewedBy = &in.CallerID
	draft.RevisionNotes = &notes
	return draft, nil
}

// ---- Staff flow ----

// StaffVerifyUpload — staff verifikasi file customer_upload valid → advance
// ke desain_diverifikasi (upload path).
// Wajib: order.design_source=upload, minimal ada 1 file customer_upload,
// order status = dibayar.
func (s *Service) StaffVerifyUpload(ctx context.Context, in StaffVerifyInput) error {
	order, err := s.orderCmd.FindSummaryByResi(ctx, in.Resi)
	if err != nil {
		if errors.Is(err, orderapi.ErrOrderNotFound) {
			return designapi.ErrOrderNotFound
		}
		return fmt.Errorf("verify: lookup order: %w", err)
	}
	if order.DesignSource != "upload" {
		return designapi.ErrDesignSourceMismatch
	}
	if order.Status != "dibayar" {
		return designapi.ErrOrderNotDesignReady
	}
	// Confirm at least 1 file exists — mencegah staff verify order kosong.
	list, err := s.files.ListByOrder(ctx, order.ID)
	if err != nil {
		return fmt.Errorf("verify: list files: %w", err)
	}
	hasCustomerUpload := false
	for _, f := range list {
		if f.Role == model.RoleCustomerUpload && !f.IsPurged {
			hasCustomerUpload = true
			break
		}
	}
	if !hasCustomerUpload {
		return designapi.ErrFileNotFound
	}
	actor := in.StaffID
	if err := s.orderCmd.MarkDesainDiverifikasi(ctx, order.ID, &actor, in.Note); err != nil {
		if errors.Is(err, orderapi.ErrOrderStateChanged) {
			return designapi.ErrOrderStateChanged
		}
		return fmt.Errorf("verify: advance order: %w", err)
	}
	// Notify customer bahwa file mereka diterima & masuk cetak (upload path §6).
	s.enqueueNotif(ctx, notificationapi.KindDesignApproved, order.ID, nil)
	return nil
}

// StaffUploadDraft — staff upload draft desain (request path). Insert row
// dgn approval_status=pending; advance order ke menunggu_approval_desain.
// Guard: order.design_source=request; status = dibayar (initial) atau
// desain_dikerjakan (revisi berikutnya). Cegah double-pending via unique index.
func (s *Service) StaffUploadDraft(ctx context.Context, in UploadInput) (*model.DesignFile, error) {
	order, err := s.orderCmd.FindSummaryByResi(ctx, in.Resi)
	if err != nil {
		if errors.Is(err, orderapi.ErrOrderNotFound) {
			return nil, designapi.ErrOrderNotFound
		}
		return nil, fmt.Errorf("staff upload: lookup order: %w", err)
	}
	if order.DesignSource != "request" {
		return nil, designapi.ErrDesignSourceMismatch
	}
	switch order.Status {
	case "dibayar", "desain_dikerjakan":
		// OK
	default:
		return nil, designapi.ErrOrderNotDesignReady
	}

	pending := model.ApprovalPending
	draft, err := s.persistBlobAndRow(ctx, order.ID, model.RoleStaffDraft, in, true, &pending, in.Notes)
	if err != nil {
		return nil, err
	}

	// Advance order state (dari dibayar atau desain_dikerjakan → menunggu_approval_desain).
	// Kalau order masih di `dibayar`, transisi butuh 2 hops:
	// dibayar → desain_dikerjakan → menunggu_approval_desain. Panggil MarkDesainDikerjakan
	// dulu kalau perlu (idempotent-ish: transition guard di state machine akan tolak kalau
	// bukan di dibayar).
	actor := in.CallerID
	if order.Status == "dibayar" {
		if err := s.orderCmd.MarkDesainDikerjakan(ctx, order.ID, &actor, "staff mulai kerja"); err != nil {
			s.rollbackFile(ctx, draft)
			return nil, fmt.Errorf("staff upload: advance to dikerjakan: %w", err)
		}
	}
	if err := s.orderCmd.MarkMenungguApprovalDesain(ctx, order.ID, &actor, "staff upload draft"); err != nil {
		s.rollbackFile(ctx, draft)
		if errors.Is(err, orderapi.ErrOrderStateChanged) {
			return nil, designapi.ErrOrderStateChanged
		}
		return nil, fmt.Errorf("staff upload: advance to approval: %w", err)
	}

	// Notify customer "desain butuh approval" — pakai kind design_needs_revision
	// tidak cocok karena bukan revisi. Untuk MVP kita reuse trigger design_approved?
	// Bukan juga — sengaja tidak notify di draft-upload; frontend bisa polling
	// atau nanti tambah kind baru "design_ready_for_review". Cukup log.
	log.Ctx(ctx).Info().
		Str("order_id", order.ID.String()).
		Str("draft_id", draft.ID.String()).
		Msg("staff draft uploaded — customer bisa review di halaman lacak")

	return draft, nil
}

// StaffApproveWalkinInstant — POS shortcut (§11): staff langsung mark draft
// approved di tempat tanpa loop menunggu_approval_desain + tanpa notif WA
// (karena approval sudah terjadi tatap muka).
// Wajib: order.channel=pos, design_approval_mode=instant_walkin, status=desain_dikerjakan.
func (s *Service) StaffApproveWalkinInstant(ctx context.Context, in WalkinInstantApproveInput) error {
	order, err := s.orderCmd.FindSummaryByResi(ctx, in.Resi)
	if err != nil {
		if errors.Is(err, orderapi.ErrOrderNotFound) {
			return designapi.ErrOrderNotFound
		}
		return fmt.Errorf("walkin approve: lookup order: %w", err)
	}
	if order.Channel != "pos" ||
		order.DesignApprovalMode == nil ||
		*order.DesignApprovalMode != "instant_walkin" {
		return designapi.ErrWalkinOnlyForPOS
	}
	if order.Status != "desain_dikerjakan" {
		return designapi.ErrOrderNotDesignReady
	}
	actor := in.StaffID
	if err := s.orderCmd.MarkDesainDiverifikasi(ctx, order.ID, &actor, in.Note); err != nil {
		if errors.Is(err, orderapi.ErrOrderStateChanged) {
			return designapi.ErrOrderStateChanged
		}
		return fmt.Errorf("walkin approve: advance order: %w", err)
	}
	// Sengaja NO notif — §11 approval sudah tatap muka.
	return nil
}

// ---- Read paths ----

// ListForOrder returns all design files for an order (customer sees own,
// staff sees anything). No file bytes — hanya metadata.
//
// scopedOrderID — non-nil kalau caller memakai token guest_order
// (authapi.Identity.OrderID); lihat dokumentasi UploadInput.ScopedOrderID.
func (s *Service) ListForOrder(ctx context.Context, resi string, callerID uuid.UUID, isStaff bool, scopedOrderID *uuid.UUID) ([]model.DesignFile, error) {
	order, err := s.orderCmd.FindSummaryByResi(ctx, resi)
	if err != nil {
		if errors.Is(err, orderapi.ErrOrderNotFound) {
			return nil, designapi.ErrOrderNotFound
		}
		return nil, fmt.Errorf("list: lookup order: %w", err)
	}
	if !isStaff && order.CustomerID != callerID {
		return nil, designapi.ErrNotOrderOwner
	}
	if err := checkScopedOrder(scopedOrderID, order.ID); err != nil {
		return nil, err
	}
	return s.files.ListByOrder(ctx, order.ID)
}

// GetFile authorizes access & returns filesystem handle to stream via
// http.ServeFile. Purged files return ErrFilePurged (§19: metadata masih ada,
// blob sudah dihapus).
//
// scopedOrderID — non-nil kalau caller memakai token guest_order
// (authapi.Identity.OrderID); lihat dokumentasi UploadInput.ScopedOrderID.
func (s *Service) GetFile(ctx context.Context, id uuid.UUID, callerID uuid.UUID, isStaff bool, scopedOrderID *uuid.UUID) (*FileHandle, error) {
	f, err := s.files.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, designrepo.ErrNotFound) {
			return nil, designapi.ErrFileNotFound
		}
		return nil, fmt.Errorf("get file: %w", err)
	}
	if f.IsPurged {
		return nil, designapi.ErrFilePurged
	}
	// Batas scope token ditegakkan TANPA syarat, sejajar dengan empat jalur
	// lain (upload/approve/revision/list). Sebelumnya cek ini bersarang di
	// dalam `if !isStaff`, sehingga kalau suatu saat ada token ber-scope milik
	// user bertipe staff, GetFile jadi satu-satunya jalur yang melewatkan
	// pembatasan per-order dan bisa mengunduh blob order mana pun.
	if err := checkScopedOrder(scopedOrderID, f.OrderID); err != nil {
		return nil, err
	}
	if !isStaff {
		order, err := s.orderCmd.FindSummaryByID(ctx, f.OrderID)
		if err != nil {
			return nil, designapi.ErrFileNotFound
		}
		if order.CustomerID != callerID {
			return nil, designapi.ErrNotOrderOwner
		}
	}
	abs, err := s.blobs.AbsPath(f.FilePath)
	if err != nil {
		log.Ctx(ctx).Error().Err(err).
			Str("file_id", f.ID.String()).
			Str("subpath", f.FilePath).
			Msg("design file path rejected — cek DB")
		return nil, fmt.Errorf("resolve path: %w", err)
	}
	return &FileHandle{File: f, AbsPath: abs}, nil
}

// ---- Internal helpers ----

// checkScopedOrder enforces that a caller using a scope-limited token (mis.
// ScopeGuestOrder, minted by POST /lacak/:resi/verify) only ever touches the
// ONE order that token was verified against. `scopedOrderID` is
// authapi.Identity.OrderID, threaded through as UploadInput/ApproveInput/
// RevisionInput.ScopedOrderID (or a direct parameter for the read paths) —
// nil means a full session, which has no extra restriction beyond the
// ownership check that already ran at the call site.
//
// Without this, a guest token issued for resi A would still pass the plain
// `order.CustomerID == callerID` ownership check for resi B, C, … — every
// OTHER order owned by the same phone number — because the token's uid is
// the real (shared) customer_id, not something scoped per-order.
func checkScopedOrder(scopedOrderID *uuid.UUID, orderID uuid.UUID) error {
	if scopedOrderID != nil && *scopedOrderID != orderID {
		return designapi.ErrNotOrderOwner
	}
	return nil
}

func (s *Service) persistBlobAndRow(
	ctx context.Context,
	orderID uuid.UUID,
	role model.Role,
	in UploadInput,
	setApproval bool,
	approvalStatus *model.ApprovalStatus,
	notes string,
) (*model.DesignFile, error) {
	// Validate mime + ekstensi + size.
	ext, previewable, canonicalMime, err := resolveFileSpec(in.MimeType, in.OriginalName)
	if err != nil {
		return nil, err
	}
	if in.FileSize <= 0 {
		return nil, designapi.ErrFileEmpty
	}
	if in.FileSize > s.maxSizeByte {
		return nil, designapi.ErrFileTooLarge
	}

	now := s.nowFn().UTC()
	fileID := uuid.New()
	// Storage layout: design_files/<year>/<month>/<file_id>.<ext>
	subpath := path.Join(
		"design_files",
		fmt.Sprintf("%04d", now.Year()),
		fmt.Sprintf("%02d", now.Month()),
		fileID.String()+ext,
	)
	written, err := s.blobs.Save(ctx, subpath, in.FileReader)
	if err != nil {
		return nil, fmt.Errorf("save blob: %w", err)
	}
	if written != in.FileSize {
		_ = s.blobs.Delete(ctx, subpath)
		return nil, fmt.Errorf("declared size %d != written %d", in.FileSize, written)
	}

	row := &model.DesignFile{
		ID:               fileID,
		OrderID:          orderID,
		Role:             role,
		FilePath:         subpath,
		FileOriginalName: safeOriginalName(in.OriginalName),
		FileSizeBytes:    written,
		// Mime kanonik dari ekstensi, BUKAN nilai mentah client — nilai ini
		// disajikan kembali sebagai Content-Type saat file di-stream.
		FileMimeType:     canonicalMime,
		IsPreviewable:    previewable,
		UploadedAt:       now,
		UploadedBy:       &in.CallerID,
	}
	if notes != "" {
		n := notes
		row.Notes = &n
	}
	if setApproval {
		row.ApprovalStatus = approvalStatus
	}

	if err := s.files.Create(ctx, row); err != nil {
		_ = s.blobs.Delete(ctx, subpath)
		if errors.Is(err, designrepo.ErrPendingDraftExists) {
			return nil, designapi.ErrPendingDraftExists
		}
		return nil, fmt.Errorf("insert design_file row: %w", err)
	}
	return row, nil
}

func (s *Service) rollbackFile(ctx context.Context, f *model.DesignFile) {
	if f == nil {
		return
	}
	if err := s.files.DeleteRow(ctx, f.ID); err != nil {
		log.Ctx(ctx).Error().Err(err).
			Str("file_id", f.ID.String()).
			Msg("rollback: delete design_file row failed")
	}
	if err := s.blobs.Delete(ctx, f.FilePath); err != nil {
		log.Ctx(ctx).Error().Err(err).
			Str("subpath", f.FilePath).
			Msg("rollback: delete design blob failed — orphaned file")
	}
}

func (s *Service) enqueueNotif(ctx context.Context, kind notificationapi.Kind, orderID uuid.UUID, extras map[string]any) {
	if s.notifier == nil {
		return
	}
	if err := s.notifier.EnqueueOrderEvent(ctx, kind, orderID, extras); err != nil {
		log.Ctx(ctx).Error().Err(err).
			Str("order_id", orderID.String()).
			Str("kind", string(kind)).
			Msg("enqueue design notification failed — WA tidak terkirim, cek manual")
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// safeOriginalName strips path separators + control chars.
func safeOriginalName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "upload"
	}
	name = path.Base(name)
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
