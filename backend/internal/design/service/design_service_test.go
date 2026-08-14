package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/design/designapi"
	"github.com/rajaku-printing/backend/internal/design/model"
	designrepo "github.com/rajaku-printing/backend/internal/design/repository"
	"github.com/rajaku-printing/backend/internal/order/orderapi"
)

// ---------- fakes ----------

type fakeStore struct {
	created          *model.DesignFile
	createErr        error
	createCalls      int
	findByID         *model.DesignFile
	findByIDErr      error
	pendingDraft     *model.DesignFile
	pendingDraftErr  error
	listByOrder      []model.DesignFile
	listByOrderErr   error
	reviewCalls      int
	reviewErr        error
	reviewLastParams designrepo.ReviewDraftParams
	markPurgedCalls  int
	deleteCalls      int

	// Retention (§19)
	purgeCandidates       []model.DesignFile
	purgeCandidatesErr    error
	purgeCutoff           time.Time
	reminderCandidates    []model.DesignFile
	reminderCandidatesErr error
	reminderWindow        [2]time.Time
	markPurgedIDs         []uuid.UUID
	markPurgedErr         error
	reminderSentIDs       []uuid.UUID
	reminderSentErr       error
}

func (f *fakeStore) Create(_ context.Context, x *model.DesignFile) error {
	f.createCalls++
	if f.createErr != nil {
		return f.createErr
	}
	if x.ID == uuid.Nil {
		x.ID = uuid.New()
	}
	x.UploadedAt = time.Now().UTC()
	f.created = x
	return nil
}
func (f *fakeStore) FindByID(_ context.Context, _ uuid.UUID) (*model.DesignFile, error) {
	return f.findByID, f.findByIDErr
}
func (f *fakeStore) FindPendingDraftByOrder(_ context.Context, _ uuid.UUID) (*model.DesignFile, error) {
	return f.pendingDraft, f.pendingDraftErr
}
func (f *fakeStore) ListByOrder(_ context.Context, _ uuid.UUID) ([]model.DesignFile, error) {
	return f.listByOrder, f.listByOrderErr
}
func (f *fakeStore) ReviewDraft(_ context.Context, p designrepo.ReviewDraftParams) error {
	f.reviewCalls++
	f.reviewLastParams = p
	return f.reviewErr
}
func (f *fakeStore) MarkPurged(_ context.Context, id uuid.UUID, _ time.Time) error {
	f.markPurgedCalls++
	if f.markPurgedErr != nil {
		return f.markPurgedErr
	}
	f.markPurgedIDs = append(f.markPurgedIDs, id)
	return nil
}
func (f *fakeStore) DeleteRow(_ context.Context, _ uuid.UUID) error {
	f.deleteCalls++
	return nil
}
func (f *fakeStore) ListPurgeCandidates(_ context.Context, cutoff time.Time, _ int) ([]model.DesignFile, error) {
	f.purgeCutoff = cutoff
	return f.purgeCandidates, f.purgeCandidatesErr
}
func (f *fakeStore) ListReminderCandidates(_ context.Context, purgeCutoff, reminderCutoff time.Time, _ int) ([]model.DesignFile, error) {
	f.reminderWindow = [2]time.Time{purgeCutoff, reminderCutoff}
	return f.reminderCandidates, f.reminderCandidatesErr
}
func (f *fakeStore) MarkReminderSent(_ context.Context, id uuid.UUID, _ time.Time) error {
	if f.reminderSentErr != nil {
		return f.reminderSentErr
	}
	f.reminderSentIDs = append(f.reminderSentIDs, id)
	return nil
}

type fakeBlobs struct {
	saveErr     error
	saveWritten int64
	saveCalls   int
	saveSubpath string
	deleteCalls int
	absErr      error
	deleteErr   error
	deleted     []string
}

func (f *fakeBlobs) Save(_ context.Context, subpath string, r io.Reader) (int64, error) {
	f.saveCalls++
	f.saveSubpath = subpath
	if f.saveErr != nil {
		return 0, f.saveErr
	}
	n, _ := io.Copy(io.Discard, r)
	if f.saveWritten > 0 {
		return f.saveWritten, nil
	}
	return n, nil
}
func (f *fakeBlobs) Delete(_ context.Context, subpath string) error {
	f.deleteCalls++
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleted = append(f.deleted, subpath)
	return nil
}
func (f *fakeBlobs) AbsPath(subpath string) (string, error) {
	if f.absErr != nil {
		return "", f.absErr
	}
	return "/mock/" + subpath, nil
}

type fakeOrderCmd struct {
	summary               *orderapi.OrderSummary
	summaryErr            error
	dikerjaknCalls        int
	dikerjaknErr          error
	menungguApprovalCalls int
	menungguApprovalErr   error
	diverifikasiCalls     int
	diverifikasiErr       error
}

func (f *fakeOrderCmd) FindSummaryByResi(context.Context, string) (*orderapi.OrderSummary, error) {
	return f.summary, f.summaryErr
}
func (f *fakeOrderCmd) FindSummaryByID(context.Context, uuid.UUID) (*orderapi.OrderSummary, error) {
	return f.summary, f.summaryErr
}
func (f *fakeOrderCmd) MarkPendingVerification(context.Context, uuid.UUID, *uuid.UUID, string) error {
	return nil
}
func (f *fakeOrderCmd) MarkDibayar(context.Context, uuid.UUID, string, *uuid.UUID, string) error {
	return nil
}
func (f *fakeOrderCmd) MarkDitolak(context.Context, uuid.UUID, *uuid.UUID, string) error {
	return nil
}
func (f *fakeOrderCmd) MarkDesainDikerjakan(context.Context, uuid.UUID, *uuid.UUID, string) error {
	f.dikerjaknCalls++
	return f.dikerjaknErr
}
func (f *fakeOrderCmd) MarkMenungguApprovalDesain(context.Context, uuid.UUID, *uuid.UUID, string) error {
	f.menungguApprovalCalls++
	return f.menungguApprovalErr
}
func (f *fakeOrderCmd) MarkDesainDiverifikasi(context.Context, uuid.UUID, *uuid.UUID, string) error {
	f.diverifikasiCalls++
	return f.diverifikasiErr
}
func (f *fakeOrderCmd) MarkProsesCetak(context.Context, uuid.UUID, *uuid.UUID, string) error {
	return nil
}
func (f *fakeOrderCmd) MarkQC(context.Context, uuid.UUID, *uuid.UUID, string) error {
	return nil
}
func (f *fakeOrderCmd) MarkSiapKirimAtauAmbil(context.Context, uuid.UUID, *uuid.UUID, string) error {
	return nil
}
func (f *fakeOrderCmd) MarkDikirim(context.Context, uuid.UUID, *uuid.UUID, string, string, string) error {
	return nil
}
func (f *fakeOrderCmd) MarkSelesai(context.Context, uuid.UUID, *uuid.UUID, string) error {
	return nil
}
func (f *fakeOrderCmd) FindInvoiceViewByID(context.Context, uuid.UUID) (*orderapi.OrderInvoiceView, error) {
	return nil, nil
}
func (f *fakeOrderCmd) CreatePOSOrder(context.Context, orderapi.POSCreateOrderInput) (*orderapi.OrderSummary, error) {
	return nil, nil
}
func (f *fakeOrderCmd) ListPOSOrdersByDate(context.Context, time.Time) ([]orderapi.OrderSummary, error) {
	return nil, nil
}

// ---------- helpers ----------

func newSvc(store *fakeStore, blobs *fakeBlobs, cmd *fakeOrderCmd) *Service {
	s := New(store, blobs, cmd, Config{MaxUploadMB: 5})
	s.nowFn = func() time.Time { return time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC) }
	return s
}

func newUpload(orderResi string, caller uuid.UUID, staff bool, body string, mime string) UploadInput {
	return UploadInput{
		Resi:         orderResi,
		CallerID:     caller,
		IsStaff:      staff,
		FileReader:   bytes.NewReader([]byte(body)),
		FileSize:     int64(len(body)),
		MimeType:     mime,
		OriginalName: "design.pdf",
	}
}

// ---------- tests ----------

func TestUploadCustomerFile_HappyPath_UploadSource(t *testing.T) {
	cust := uuid.New()
	orderID := uuid.New()
	store := &fakeStore{}
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: orderID, Resi: "RJK-ABC", CustomerID: cust,
		Status: "dibayar", DesignSource: "upload",
	}}
	svc := newSvc(store, &fakeBlobs{}, cmd)

	saved, err := svc.UploadCustomerFile(context.Background(), newUpload("RJK-ABC", cust, false, "PDFDATA", "application/pdf"))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if saved.Role != model.RoleCustomerUpload {
		t.Errorf("role want customer_upload, got %s", saved.Role)
	}
	if !saved.IsPreviewable {
		t.Errorf("pdf should be previewable")
	}
	if store.createCalls != 1 {
		t.Errorf("create called %d times, want 1", store.createCalls)
	}
	// Upload path tidak advance state — staff yg akan verify.
	if cmd.dikerjaknCalls != 0 {
		t.Errorf("upload-source path should NOT auto-advance to dikerjakan")
	}
}

func TestUploadCustomerFile_HappyPath_RequestSource_AutoAdvance(t *testing.T) {
	cust := uuid.New()
	orderID := uuid.New()
	store := &fakeStore{}
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: orderID, Resi: "RJK-DEF", CustomerID: cust,
		Status: "dibayar", DesignSource: "request",
	}}
	svc := newSvc(store, &fakeBlobs{}, cmd)

	saved, err := svc.UploadCustomerFile(context.Background(), newUpload("RJK-DEF", cust, false, "LOGO", "image/png"))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if saved.Role != model.RoleCustomerAsset {
		t.Errorf("role want customer_asset, got %s", saved.Role)
	}
	if cmd.dikerjaknCalls != 1 {
		t.Errorf("request-source path should auto-advance dibayar → desain_dikerjakan, got %d calls", cmd.dikerjaknCalls)
	}
}

func TestUploadCustomerFile_NotOwner_Rejected(t *testing.T) {
	cust := uuid.New()
	other := uuid.New()
	store := &fakeStore{}
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: uuid.New(), CustomerID: cust, Status: "dibayar", DesignSource: "upload",
	}}
	svc := newSvc(store, &fakeBlobs{}, cmd)

	_, err := svc.UploadCustomerFile(context.Background(), newUpload("RJK-X", other, false, "PDF", "application/pdf"))
	if !errors.Is(err, designapi.ErrNotOrderOwner) {
		t.Fatalf("want ErrNotOrderOwner, got %v", err)
	}
	if store.createCalls != 0 {
		t.Errorf("must not create when not owner")
	}
}

// TestUploadCustomerFile_ScopedToDifferentOrder_Rejected is a regression test
// for a review finding (§3): a guest_order token minted for resi A carries
// the customer's REAL user_id, so the plain ownership check (order.CustomerID
// == callerID) alone would keep passing for every OTHER order owned by that
// same customer/phone number. ScopedOrderID closes that gap.
func TestUploadCustomerFile_ScopedToDifferentOrder_Rejected(t *testing.T) {
	cust := uuid.New()
	orderA := uuid.New()
	orderB := uuid.New()
	store := &fakeStore{}
	// Caller's token was verified against orderA, but they're trying to
	// upload to orderB — same owner (cust), different order.
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: orderB, Resi: "RJK-ORDERB", CustomerID: cust,
		Status: "dibayar", DesignSource: "upload",
	}}
	svc := newSvc(store, &fakeBlobs{}, cmd)

	in := newUpload("RJK-ORDERB", cust, false, "PDF", "application/pdf")
	in.ScopedOrderID = &orderA
	_, err := svc.UploadCustomerFile(context.Background(), in)
	if !errors.Is(err, designapi.ErrNotOrderOwner) {
		t.Fatalf("want ErrNotOrderOwner for cross-order scoped token, got %v", err)
	}
	if store.createCalls != 0 {
		t.Errorf("must not create file when scoped token targets a different order")
	}
}

// TestUploadCustomerFile_ScopedToSameOrder_Allowed is the companion happy
// path: a scoped token targeting the SAME order it's used against must still
// work normally.
func TestUploadCustomerFile_ScopedToSameOrder_Allowed(t *testing.T) {
	cust := uuid.New()
	orderID := uuid.New()
	store := &fakeStore{}
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: orderID, Resi: "RJK-SAME", CustomerID: cust,
		Status: "dibayar", DesignSource: "upload",
	}}
	svc := newSvc(store, &fakeBlobs{}, cmd)

	in := newUpload("RJK-SAME", cust, false, "PDF", "application/pdf")
	in.ScopedOrderID = &orderID
	_, err := svc.UploadCustomerFile(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected err for scoped token targeting its own order: %v", err)
	}
	if store.createCalls != 1 {
		t.Errorf("expected file to be created, createCalls=%d", store.createCalls)
	}
}

func TestUploadCustomerFile_WrongStatus_Rejected(t *testing.T) {
	cust := uuid.New()
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: uuid.New(), CustomerID: cust, Status: "menunggu_pembayaran", DesignSource: "upload",
	}}
	store := &fakeStore{}
	svc := newSvc(store, &fakeBlobs{}, cmd)

	_, err := svc.UploadCustomerFile(context.Background(), newUpload("RJK-X", cust, false, "PDF", "application/pdf"))
	if !errors.Is(err, designapi.ErrOrderNotDesignReady) {
		t.Fatalf("want ErrOrderNotDesignReady, got %v", err)
	}
}

func TestUploadCustomerFile_InvalidMime_Rejected(t *testing.T) {
	cust := uuid.New()
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: uuid.New(), CustomerID: cust, Status: "dibayar", DesignSource: "upload",
	}}
	svc := newSvc(&fakeStore{}, &fakeBlobs{}, cmd)

	_, err := svc.UploadCustomerFile(context.Background(), newUpload("RJK-X", cust, false, "DOC", "application/msword"))
	if !errors.Is(err, designapi.ErrInvalidMimeType) {
		t.Fatalf("want ErrInvalidMimeType, got %v", err)
	}
}

// TestUploadCustomerFile_GenericMimeRequiresAllowedExtension menutup lubang
// unggah file sembarangan: "application/octet-stream" ada di allowedMimeTypes
// karena browser mengirimnya untuk CDR/AI, tapi tanpa cross-check ekstensi
// nilai itu meloloskan file jenis APA PUN (.exe, .sh, …) ke disk VPS.
func TestUploadCustomerFile_GenericMimeRequiresAllowedExtension(t *testing.T) {
	cust := uuid.New()
	newCmd := func() *fakeOrderCmd {
		return &fakeOrderCmd{summary: &orderapi.OrderSummary{
			ID: uuid.New(), CustomerID: cust, Status: "dibayar", DesignSource: "upload",
		}}
	}

	t.Run("ekstensi terlarang ditolak meski mime generik", func(t *testing.T) {
		svc := newSvc(&fakeStore{}, &fakeBlobs{}, newCmd())
		in := newUpload("RJK-X", cust, false, "MZ\x90\x00", "application/octet-stream")
		in.OriginalName = "payload.exe"
		if _, err := svc.UploadCustomerFile(context.Background(), in); !errors.Is(err, designapi.ErrInvalidMimeType) {
			t.Fatalf("want ErrInvalidMimeType for .exe, got %v", err)
		}
	})

	t.Run("tanpa ekstensi ditolak", func(t *testing.T) {
		svc := newSvc(&fakeStore{}, &fakeBlobs{}, newCmd())
		in := newUpload("RJK-X", cust, false, "data", "application/octet-stream")
		in.OriginalName = "berkas-tanpa-ekstensi"
		if _, err := svc.UploadCustomerFile(context.Background(), in); !errors.Is(err, designapi.ErrInvalidMimeType) {
			t.Fatalf("want ErrInvalidMimeType for extensionless file, got %v", err)
		}
	})

	// Jalur yang HARUS tetap jalan: browser kirim octet-stream untuk .cdr —
	// justru alasan fallback generik itu ada. Jangan sampai perbaikan ini
	// mematikan unggahan CorelDRAW yang sah.
	t.Run("cdr dengan mime generik tetap diterima", func(t *testing.T) {
		store := &fakeStore{}
		svc := newSvc(store, &fakeBlobs{}, newCmd())
		in := newUpload("RJK-X", cust, false, "CDR-BODY", "application/octet-stream")
		in.OriginalName = "spanduk.CDR" // uppercase — harus case-insensitive
		got, err := svc.UploadCustomerFile(context.Background(), in)
		if err != nil {
			t.Fatalf("want success for .cdr, got %v", err)
		}
		if got.IsPreviewable {
			t.Errorf("CDR tidak boleh ditandai previewable")
		}
		if !strings.HasSuffix(got.FilePath, ".cdr") {
			t.Errorf("blob harus disimpan dengan ekstensi .cdr, got %q", got.FilePath)
		}
	})

	// Previewable diturunkan dari ekstensi, bukan dari mime yang bisa dipalsukan.
	t.Run("png dengan mime generik previewable dari ekstensi", func(t *testing.T) {
		svc := newSvc(&fakeStore{}, &fakeBlobs{}, newCmd())
		in := newUpload("RJK-X", cust, false, "PNG-BODY", "application/octet-stream")
		in.OriginalName = "logo.png"
		got, err := svc.UploadCustomerFile(context.Background(), in)
		if err != nil {
			t.Fatalf("want success for .png, got %v", err)
		}
		if !got.IsPreviewable {
			t.Errorf("PNG harus previewable meski mime-nya generik")
		}
	})
}

func TestUploadCustomerFile_TooLarge_Rejected(t *testing.T) {
	cust := uuid.New()
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: uuid.New(), CustomerID: cust, Status: "dibayar", DesignSource: "upload",
	}}
	svc := newSvc(&fakeStore{}, &fakeBlobs{}, cmd)

	// Cap = 5MB in newSvc; declare 6MB.
	in := newUpload("RJK-X", cust, false, "x", "application/pdf")
	in.FileSize = 6 * 1024 * 1024
	_, err := svc.UploadCustomerFile(context.Background(), in)
	if !errors.Is(err, designapi.ErrFileTooLarge) {
		t.Fatalf("want ErrFileTooLarge, got %v", err)
	}
}

func TestStaffUploadDraft_HappyPath_AdvancesToApprovalState(t *testing.T) {
	staff := uuid.New()
	orderID := uuid.New()
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: orderID, Resi: "RJK-Q", CustomerID: uuid.New(),
		Status: "dibayar", DesignSource: "request",
	}}
	store := &fakeStore{}
	svc := newSvc(store, &fakeBlobs{}, cmd)

	saved, err := svc.StaffUploadDraft(context.Background(), newUpload("RJK-Q", staff, true, "DRAFT", "application/pdf"))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if saved.Role != model.RoleStaffDraft {
		t.Errorf("role want staff_draft, got %s", saved.Role)
	}
	if saved.ApprovalStatus == nil || *saved.ApprovalStatus != model.ApprovalPending {
		t.Errorf("approval status want pending, got %v", saved.ApprovalStatus)
	}
	// Dari status `dibayar`, service harus panggil dikerjakan DULU baru approval.
	if cmd.dikerjaknCalls != 1 {
		t.Errorf("want MarkDesainDikerjakan called (dari dibayar), got %d", cmd.dikerjaknCalls)
	}
	if cmd.menungguApprovalCalls != 1 {
		t.Errorf("want MarkMenungguApprovalDesain called once, got %d", cmd.menungguApprovalCalls)
	}
}

func TestStaffUploadDraft_WrongDesignSource_Rejected(t *testing.T) {
	staff := uuid.New()
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: uuid.New(), CustomerID: uuid.New(),
		Status: "dibayar", DesignSource: "upload", // wrong: staff draft hanya utk request
	}}
	svc := newSvc(&fakeStore{}, &fakeBlobs{}, cmd)

	_, err := svc.StaffUploadDraft(context.Background(), newUpload("RJK-X", staff, true, "X", "application/pdf"))
	if !errors.Is(err, designapi.ErrDesignSourceMismatch) {
		t.Fatalf("want ErrDesignSourceMismatch, got %v", err)
	}
}

func TestApproveDraft_HappyPath(t *testing.T) {
	cust := uuid.New()
	orderID := uuid.New()
	draftID := uuid.New()
	pending := model.ApprovalPending
	draft := &model.DesignFile{
		ID: draftID, OrderID: orderID, Role: model.RoleStaffDraft,
		ApprovalStatus: &pending,
	}
	store := &fakeStore{findByID: draft}
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: orderID, CustomerID: cust, Status: "menunggu_approval_desain",
	}}
	svc := newSvc(store, &fakeBlobs{}, cmd)

	out, err := svc.ApproveDraft(context.Background(), ApproveInput{
		DraftID: draftID, CallerID: cust, IsStaff: false,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if store.reviewCalls != 1 {
		t.Errorf("ReviewDraft calls: want 1 got %d", store.reviewCalls)
	}
	if store.reviewLastParams.NewStatus != model.ApprovalApproved {
		t.Errorf("review status: want approved, got %s", store.reviewLastParams.NewStatus)
	}
	if cmd.diverifikasiCalls != 1 {
		t.Errorf("want MarkDesainDiverifikasi called once, got %d", cmd.diverifikasiCalls)
	}
	if out.ApprovalStatus == nil || *out.ApprovalStatus != model.ApprovalApproved {
		t.Errorf("returned draft status not updated")
	}
}

func TestApproveDraft_NotOwner_Rejected(t *testing.T) {
	cust := uuid.New()
	other := uuid.New()
	pending := model.ApprovalPending
	store := &fakeStore{findByID: &model.DesignFile{
		ID: uuid.New(), OrderID: uuid.New(), Role: model.RoleStaffDraft, ApprovalStatus: &pending,
	}}
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{CustomerID: cust}}
	svc := newSvc(store, &fakeBlobs{}, cmd)

	_, err := svc.ApproveDraft(context.Background(), ApproveInput{
		DraftID: uuid.New(), CallerID: other, IsStaff: false,
	})
	if !errors.Is(err, designapi.ErrNotOrderOwner) {
		t.Fatalf("want ErrNotOrderOwner, got %v", err)
	}
}

// TestApproveDraft_ScopedToDifferentOrder_Rejected mirrors the upload-path
// regression test above for the approve-draft path (§3 review finding).
func TestApproveDraft_ScopedToDifferentOrder_Rejected(t *testing.T) {
	cust := uuid.New()
	orderA := uuid.New()
	orderB := uuid.New()
	draftID := uuid.New()
	pending := model.ApprovalPending
	draft := &model.DesignFile{
		ID: draftID, OrderID: orderB, Role: model.RoleStaffDraft, ApprovalStatus: &pending,
	}
	store := &fakeStore{findByID: draft}
	// Draft belongs to orderB, owned by the same customer — but the caller's
	// token was verified against orderA.
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: orderB, CustomerID: cust, Status: "menunggu_approval_desain",
	}}
	svc := newSvc(store, &fakeBlobs{}, cmd)

	_, err := svc.ApproveDraft(context.Background(), ApproveInput{
		DraftID: draftID, CallerID: cust, IsStaff: false, ScopedOrderID: &orderA,
	})
	if !errors.Is(err, designapi.ErrNotOrderOwner) {
		t.Fatalf("want ErrNotOrderOwner for cross-order scoped token, got %v", err)
	}
	if store.reviewCalls != 0 {
		t.Errorf("must not review draft when scoped token targets a different order")
	}
}

// TestApproveDraft_ScopedToSameOrder_Allowed is the companion happy path.
func TestApproveDraft_ScopedToSameOrder_Allowed(t *testing.T) {
	cust := uuid.New()
	orderID := uuid.New()
	draftID := uuid.New()
	pending := model.ApprovalPending
	draft := &model.DesignFile{
		ID: draftID, OrderID: orderID, Role: model.RoleStaffDraft, ApprovalStatus: &pending,
	}
	store := &fakeStore{findByID: draft}
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: orderID, CustomerID: cust, Status: "menunggu_approval_desain",
	}}
	svc := newSvc(store, &fakeBlobs{}, cmd)

	_, err := svc.ApproveDraft(context.Background(), ApproveInput{
		DraftID: draftID, CallerID: cust, IsStaff: false, ScopedOrderID: &orderID,
	})
	if err != nil {
		t.Fatalf("unexpected err for scoped token targeting its own order: %v", err)
	}
	if store.reviewCalls != 1 {
		t.Errorf("expected draft to be reviewed, reviewCalls=%d", store.reviewCalls)
	}
}

func TestApproveDraft_NotStaffDraft_Rejected(t *testing.T) {
	store := &fakeStore{findByID: &model.DesignFile{
		Role: model.RoleCustomerUpload,
	}}
	svc := newSvc(store, &fakeBlobs{}, &fakeOrderCmd{})

	_, err := svc.ApproveDraft(context.Background(), ApproveInput{
		DraftID: uuid.New(), CallerID: uuid.New(),
	})
	if !errors.Is(err, designapi.ErrInvalidRole) {
		t.Fatalf("want ErrInvalidRole, got %v", err)
	}
}

func TestRequestRevision_RequiresNotes(t *testing.T) {
	svc := newSvc(&fakeStore{}, &fakeBlobs{}, &fakeOrderCmd{})
	_, err := svc.RequestRevision(context.Background(), RevisionInput{
		DraftID: uuid.New(), CallerID: uuid.New(), Notes: "   ",
	})
	if !errors.Is(err, designapi.ErrRevisionNotesRequired) {
		t.Fatalf("want ErrRevisionNotesRequired, got %v", err)
	}
}

func TestRequestRevision_HappyPath_TransitionsBackToDikerjakan(t *testing.T) {
	cust := uuid.New()
	orderID := uuid.New()
	draftID := uuid.New()
	pending := model.ApprovalPending
	draft := &model.DesignFile{
		ID: draftID, OrderID: orderID, Role: model.RoleStaffDraft, ApprovalStatus: &pending,
	}
	store := &fakeStore{findByID: draft}
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: orderID, CustomerID: cust, Status: "menunggu_approval_desain",
	}}
	svc := newSvc(store, &fakeBlobs{}, cmd)

	out, err := svc.RequestRevision(context.Background(), RevisionInput{
		DraftID: draftID, CallerID: cust, Notes: "warnanya terlalu redup",
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if store.reviewLastParams.NewStatus != model.ApprovalRevision {
		t.Errorf("review status: want revision_requested, got %s", store.reviewLastParams.NewStatus)
	}
	if !strings.Contains(store.reviewLastParams.RevisionNotes, "warnanya") {
		t.Errorf("revision_notes tidak persisted")
	}
	if cmd.dikerjaknCalls != 1 {
		t.Errorf("expected MarkDesainDikerjakan called, got %d", cmd.dikerjaknCalls)
	}
	if out.ApprovalStatus == nil || *out.ApprovalStatus != model.ApprovalRevision {
		t.Errorf("returned draft status not updated")
	}
}

func TestStaffVerifyUpload_HappyPath(t *testing.T) {
	staff := uuid.New()
	orderID := uuid.New()
	store := &fakeStore{listByOrder: []model.DesignFile{
		{ID: uuid.New(), OrderID: orderID, Role: model.RoleCustomerUpload},
	}}
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: orderID, Resi: "RJK-Y", CustomerID: uuid.New(),
		Status: "dibayar", DesignSource: "upload",
	}}
	svc := newSvc(store, &fakeBlobs{}, cmd)

	err := svc.StaffVerifyUpload(context.Background(), StaffVerifyInput{
		Resi: "RJK-Y", StaffID: staff, Note: "file OK",
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if cmd.diverifikasiCalls != 1 {
		t.Errorf("want MarkDesainDiverifikasi called, got %d", cmd.diverifikasiCalls)
	}
}

func TestStaffVerifyUpload_NoFileYet_Rejected(t *testing.T) {
	staff := uuid.New()
	store := &fakeStore{listByOrder: []model.DesignFile{}}
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: uuid.New(), CustomerID: uuid.New(),
		Status: "dibayar", DesignSource: "upload",
	}}
	svc := newSvc(store, &fakeBlobs{}, cmd)

	err := svc.StaffVerifyUpload(context.Background(), StaffVerifyInput{
		Resi: "RJK-Y", StaffID: staff,
	})
	if !errors.Is(err, designapi.ErrFileNotFound) {
		t.Fatalf("want ErrFileNotFound (no files uploaded), got %v", err)
	}
	if cmd.diverifikasiCalls != 0 {
		t.Errorf("must not advance without file")
	}
}

func TestStaffApproveWalkinInstant_HappyPath(t *testing.T) {
	mode := "instant_walkin"
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: uuid.New(), Channel: "pos", Status: "desain_dikerjakan",
		DesignApprovalMode: &mode,
	}}
	svc := newSvc(&fakeStore{}, &fakeBlobs{}, cmd)

	err := svc.StaffApproveWalkinInstant(context.Background(), WalkinInstantApproveInput{
		Resi: "RJK-Z", StaffID: uuid.New(),
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if cmd.diverifikasiCalls != 1 {
		t.Errorf("want diverifikasi called, got %d", cmd.diverifikasiCalls)
	}
}

func TestStaffApproveWalkinInstant_NotPOS_Rejected(t *testing.T) {
	mode := "instant_walkin"
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		Channel: "online", // wrong
		Status:  "desain_dikerjakan", DesignApprovalMode: &mode,
	}}
	svc := newSvc(&fakeStore{}, &fakeBlobs{}, cmd)

	err := svc.StaffApproveWalkinInstant(context.Background(), WalkinInstantApproveInput{
		Resi: "RJK-Z", StaffID: uuid.New(),
	})
	if !errors.Is(err, designapi.ErrWalkinOnlyForPOS) {
		t.Fatalf("want ErrWalkinOnlyForPOS, got %v", err)
	}
}

// ---------- Regresi batas scope: 3 jalur yang sebelumnya tidak terkunci ----------
//
// Review putaran kedua menemukan penegakan `ScopedOrderID` hanya diuji di jalur
// upload & approve. Tanpa test di bawah, menghapus `checkScopedOrder` dari
// RequestRevision / ListForOrder / GetFile membuat SELURUH suite tetap hijau
// sementara token guest resi A kembali bisa membaca metadata, mengunduh blob,
// dan memaksa revisi di order lain — persis kelas kerentanan yang baru ditambal.

func TestRequestRevision_ScopedToDifferentOrder_Rejected(t *testing.T) {
	cust := uuid.New()
	orderA, orderB := uuid.New(), uuid.New()
	draftID := uuid.New()
	pending := model.ApprovalPending
	store := &fakeStore{findByID: &model.DesignFile{
		ID: draftID, OrderID: orderB, Role: model.RoleStaffDraft, ApprovalStatus: &pending,
	}}
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: orderB, CustomerID: cust, Status: "menunggu_approval_desain",
	}}
	svc := newSvc(store, &fakeBlobs{}, cmd)

	_, err := svc.RequestRevision(context.Background(), RevisionInput{
		DraftID: draftID, CallerID: cust, IsStaff: false,
		Notes: "tolong warnanya dinaikkan", ScopedOrderID: &orderA,
	})
	if !errors.Is(err, designapi.ErrNotOrderOwner) {
		t.Fatalf("want ErrNotOrderOwner for cross-order scoped token, got %v", err)
	}
	if store.reviewCalls != 0 {
		t.Errorf("must not touch draft when scoped token targets a different order")
	}
}

func TestListForOrder_ScopedToDifferentOrder_Rejected(t *testing.T) {
	cust := uuid.New()
	orderA, orderB := uuid.New(), uuid.New()
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: orderB, Resi: "RJK-B", CustomerID: cust, Status: "dibayar", DesignSource: "upload",
	}}
	store := &fakeStore{listByOrder: []model.DesignFile{{ID: uuid.New(), OrderID: orderB}}}
	svc := newSvc(store, &fakeBlobs{}, cmd)

	if _, err := svc.ListForOrder(context.Background(), "RJK-B", cust, false, &orderA); !errors.Is(err, designapi.ErrNotOrderOwner) {
		t.Fatalf("want ErrNotOrderOwner listing another order with scoped token, got %v", err)
	}
}

func TestListForOrder_ScopedToSameOrder_Allowed(t *testing.T) {
	cust := uuid.New()
	orderB := uuid.New()
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: orderB, Resi: "RJK-B", CustomerID: cust, Status: "dibayar", DesignSource: "upload",
	}}
	store := &fakeStore{listByOrder: []model.DesignFile{{ID: uuid.New(), OrderID: orderB}}}
	svc := newSvc(store, &fakeBlobs{}, cmd)

	items, err := svc.ListForOrder(context.Background(), "RJK-B", cust, false, &orderB)
	if err != nil {
		t.Fatalf("scoped token for its own order must be allowed, got %v", err)
	}
	if len(items) != 1 {
		t.Errorf("want 1 item, got %d", len(items))
	}
}

func TestGetFile_ScopedToDifferentOrder_Rejected(t *testing.T) {
	cust := uuid.New()
	orderA, orderB := uuid.New(), uuid.New()
	fileID := uuid.New()
	store := &fakeStore{findByID: &model.DesignFile{
		ID: fileID, OrderID: orderB, Role: model.RoleCustomerUpload, FilePath: "design_files/2026/08/x.pdf",
	}}
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{ID: orderB, CustomerID: cust}}
	svc := newSvc(store, &fakeBlobs{}, cmd)

	if _, err := svc.GetFile(context.Background(), fileID, cust, false, &orderA); !errors.Is(err, designapi.ErrNotOrderOwner) {
		t.Fatalf("want ErrNotOrderOwner downloading another order's file, got %v", err)
	}
}

// Batas scope harus berlaku juga untuk caller bertipe staff — cek ini dulunya
// bersarang di dalam `if !isStaff`, sehingga GetFile jadi satu-satunya jalur
// yang bocor kalau suatu saat ada token ber-scope milik staff.
func TestGetFile_ScopedEnforcedEvenForStaffCaller(t *testing.T) {
	orderA, orderB := uuid.New(), uuid.New()
	fileID := uuid.New()
	store := &fakeStore{findByID: &model.DesignFile{
		ID: fileID, OrderID: orderB, Role: model.RoleCustomerUpload, FilePath: "design_files/2026/08/x.pdf",
	}}
	svc := newSvc(store, &fakeBlobs{}, &fakeOrderCmd{})

	if _, err := svc.GetFile(context.Background(), fileID, uuid.New(), true, &orderA); !errors.Is(err, designapi.ErrNotOrderOwner) {
		t.Fatalf("scoped token must be order-bound even for staff caller, got %v", err)
	}
}

func TestGetFile_ScopedToSameOrder_Allowed(t *testing.T) {
	cust := uuid.New()
	orderB := uuid.New()
	fileID := uuid.New()
	store := &fakeStore{findByID: &model.DesignFile{
		ID: fileID, OrderID: orderB, Role: model.RoleCustomerUpload, FilePath: "design_files/2026/08/x.pdf",
	}}
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{ID: orderB, CustomerID: cust}}
	svc := newSvc(store, &fakeBlobs{}, cmd)

	if _, err := svc.GetFile(context.Background(), fileID, cust, false, &orderB); err != nil {
		t.Fatalf("scoped token for its own file must be allowed, got %v", err)
	}
}

// Mime yang DISIMPAN harus kanonik dari ekstensi, bukan nilai mentah client:
// nilai ini disajikan kembali sebagai Content-Type, jadi kalau ikut generik,
// file previewable gagal dirender staff di admin panel.
func TestUploadCustomerFile_StoresCanonicalMimeNotClientMime(t *testing.T) {
	cust := uuid.New()
	store := &fakeStore{}
	cmd := &fakeOrderCmd{summary: &orderapi.OrderSummary{
		ID: uuid.New(), CustomerID: cust, Status: "dibayar", DesignSource: "upload",
	}}
	svc := newSvc(store, &fakeBlobs{}, cmd)

	in := newUpload("RJK-X", cust, false, "PNGDATA", "application/octet-stream")
	in.OriginalName = "logo.png"
	got, err := svc.UploadCustomerFile(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.FileMimeType != "image/png" {
		t.Errorf("want canonical image/png, got %q", got.FileMimeType)
	}
	if !got.IsPreviewable {
		t.Errorf("png must stay previewable")
	}
}
