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
	created            *model.DesignFile
	createErr          error
	createCalls        int
	findByID           *model.DesignFile
	findByIDErr        error
	pendingDraft       *model.DesignFile
	pendingDraftErr    error
	listByOrder        []model.DesignFile
	listByOrderErr     error
	reviewCalls        int
	reviewErr          error
	reviewLastParams   designrepo.ReviewDraftParams
	markPurgedCalls    int
	deleteCalls        int

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
	summary                  *orderapi.OrderSummary
	summaryErr               error
	dikerjaknCalls           int
	dikerjaknErr             error
	menungguApprovalCalls    int
	menungguApprovalErr      error
	diverifikasiCalls        int
	diverifikasiErr          error
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
