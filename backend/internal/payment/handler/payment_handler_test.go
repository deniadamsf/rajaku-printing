package handler

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	paymodel "github.com/rajaku-printing/backend/internal/payment/model"
	"github.com/rajaku-printing/backend/internal/payment/service"
)

// fakePaymentService is a test double for the paymentService interface
// declared in payment_handler.go. It captures the arguments the handler
// passed through — in particular ScopedOrderID/isStaff — so tests can lock
// the handler->service wiring (§3/§1 review findings): the ENTIRE
// enforcement of guest-token scope and the staff permission gate lives in a
// few lines inside the handler that thread id.OrderID / id.HasPermission(...)
// through to the service call. A service-layer test alone (which calls the
// service directly) cannot catch a regression that silently drops one of
// those lines — only a test that drives the real gin handler can.
type fakePaymentService struct {
	gotScopedOrderID *uuid.UUID
	gotIsStaff       bool
	called           bool

	proof  *paymodel.PaymentProof
	items  []paymodel.PaymentProof
	handle *service.ProofFileHandle
	err    error
}

func (f *fakePaymentService) UploadProof(_ context.Context, in service.UploadProofInput) (*paymodel.PaymentProof, error) {
	f.called = true
	f.gotScopedOrderID = in.ScopedOrderID
	f.gotIsStaff = in.IsStaff
	if f.err != nil {
		return nil, f.err
	}
	if f.proof != nil {
		return f.proof, nil
	}
	return &paymodel.PaymentProof{ID: uuid.New(), Status: paymodel.ProofPending}, nil
}

func (f *fakePaymentService) ListForOrder(_ context.Context, _ string, _ uuid.UUID, isStaff bool, scopedOrderID *uuid.UUID) ([]paymodel.PaymentProof, error) {
	f.called = true
	f.gotScopedOrderID = scopedOrderID
	f.gotIsStaff = isStaff
	return f.items, f.err
}

func (f *fakePaymentService) ListProofs(_ context.Context, _ service.ListInput) (*service.ListPage, error) {
	return &service.ListPage{}, f.err
}

func (f *fakePaymentService) GetProofFile(_ context.Context, _ uuid.UUID, _ uuid.UUID, isStaff bool, scopedOrderID *uuid.UUID) (*service.ProofFileHandle, error) {
	f.called = true
	f.gotScopedOrderID = scopedOrderID
	f.gotIsStaff = isStaff
	if f.err != nil {
		return nil, f.err
	}
	if f.handle != nil {
		return f.handle, nil
	}
	return &service.ProofFileHandle{
		Proof:   &paymodel.PaymentProof{FileMimeType: "image/png", FileOriginalName: "x.png"},
		AbsPath: "",
	}, nil
}

func (f *fakePaymentService) ApproveProof(_ context.Context, _ service.ReviewInput) (*paymodel.PaymentProof, error) {
	return f.proof, f.err
}

func (f *fakePaymentService) RejectProof(_ context.Context, _ service.ReviewInput) (*paymodel.PaymentProof, error) {
	return f.proof, f.err
}

// withIdentity mimics what authapi.RequireAuth/RequireAuthAllowScope does
// after verifying a token — attach Identity to the request context — without
// needing a real authapi.Service/token in these tests.
func withIdentity(id *authapi.Identity) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := authapi.WithIdentity(c.Request.Context(), id)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

func newTestRouter(h *Handler, id *authapi.Identity) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	mw := withIdentity(id)
	r.POST("/orders/:resi/payment-proof", mw, h.UploadProof)
	r.GET("/orders/:resi/payment-proofs", mw, h.ListForOrder)
	r.GET("/payment-proofs/:id/file", mw, h.GetFile)
	return r
}

func multipartUploadBody(t *testing.T) (*bytes.Buffer, string) {
	t.Helper()
	buf := &bytes.Buffer{}
	w := multipart.NewWriter(buf)
	if err := w.WriteField("metode_bayar", "transfer"); err != nil {
		t.Fatalf("write field metode_bayar: %v", err)
	}
	fw, err := w.CreateFormFile("file", "bukti.jpg")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fw.Write([]byte("fake-image-bytes")); err != nil {
		t.Fatalf("write file bytes: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	return buf, w.FormDataContentType()
}

// ---------- §3: id.OrderID propagation from handler to service ----------

func TestUploadProof_PropagatesScopedOrderIDToService(t *testing.T) {
	orderID := uuid.New()
	id := &authapi.Identity{UserID: uuid.New(), UserType: authapi.UserTypeCustomer, OrderID: &orderID}
	fake := &fakePaymentService{}
	r := newTestRouter(New(fake), id)

	body, contentType := multipartUploadBody(t)
	req := httptest.NewRequest(http.MethodPost, "/orders/RJK-TEST/payment-proof", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if !fake.called {
		t.Fatalf("service.UploadProof was never called (status=%d body=%s)", rec.Code, rec.Body.String())
	}
	if fake.gotScopedOrderID == nil || *fake.gotScopedOrderID != orderID {
		t.Fatalf("want ScopedOrderID=%s propagated from id.OrderID, got %v (status=%d body=%s)",
			orderID, fake.gotScopedOrderID, rec.Code, rec.Body.String())
	}
}

func TestListForOrder_PropagatesScopedOrderIDToService(t *testing.T) {
	orderID := uuid.New()
	id := &authapi.Identity{UserID: uuid.New(), UserType: authapi.UserTypeCustomer, OrderID: &orderID}
	fake := &fakePaymentService{}
	r := newTestRouter(New(fake), id)

	req := httptest.NewRequest(http.MethodGet, "/orders/RJK-TEST/payment-proofs", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if !fake.called {
		t.Fatalf("service.ListForOrder was never called (status=%d body=%s)", rec.Code, rec.Body.String())
	}
	if fake.gotScopedOrderID == nil || *fake.gotScopedOrderID != orderID {
		t.Fatalf("want ScopedOrderID=%s propagated from id.OrderID, got %v (status=%d body=%s)",
			orderID, fake.gotScopedOrderID, rec.Code, rec.Body.String())
	}
}

func TestGetFile_PropagatesScopedOrderIDToService(t *testing.T) {
	orderID := uuid.New()
	id := &authapi.Identity{UserID: uuid.New(), UserType: authapi.UserTypeCustomer, OrderID: &orderID}

	tmp := filepath.Join(t.TempDir(), "proof.png")
	if err := os.WriteFile(tmp, []byte("x"), 0o600); err != nil {
		t.Fatalf("write temp proof file: %v", err)
	}
	fake := &fakePaymentService{handle: &service.ProofFileHandle{
		Proof:   &paymodel.PaymentProof{FileMimeType: "image/png", FileOriginalName: "proof.png"},
		AbsPath: tmp,
	}}
	r := newTestRouter(New(fake), id)

	req := httptest.NewRequest(http.MethodGet, "/payment-proofs/"+uuid.New().String()+"/file", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if !fake.called {
		t.Fatalf("service.GetProofFile was never called (status=%d)", rec.Code)
	}
	if fake.gotScopedOrderID == nil || *fake.gotScopedOrderID != orderID {
		t.Fatalf("want ScopedOrderID=%s propagated from id.OrderID, got %v (status=%d)",
			orderID, fake.gotScopedOrderID, rec.Code)
	}
}

// ---------- §1: staff bypass gated on "payment.verify" permission ----------

func TestListForOrder_StaffWithoutPaymentVerifyPermission_TreatedAsCustomer(t *testing.T) {
	id := &authapi.Identity{
		UserID: uuid.New(), UserType: authapi.UserTypeStaff,
		// Has SOME permission, just not payment.verify — mirrors mis. kasir
		// or admin artikel accounts, which is exactly the attack scenario
		// from the review finding.
		Permissions: []string{"order.view"},
	}
	fake := &fakePaymentService{}
	r := newTestRouter(New(fake), id)

	req := httptest.NewRequest(http.MethodGet, "/orders/RJK-TEST/payment-proofs", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if !fake.called {
		t.Fatalf("service.ListForOrder was never called (status=%d body=%s)", rec.Code, rec.Body.String())
	}
	if fake.gotIsStaff {
		t.Fatalf("staff without payment.verify must be treated as non-staff (ownership enforced downstream), got isStaff=true")
	}
}

func TestListForOrder_StaffWithPaymentVerifyPermission_Bypasses(t *testing.T) {
	id := &authapi.Identity{
		UserID: uuid.New(), UserType: authapi.UserTypeStaff,
		Permissions: []string{"payment.verify"},
	}
	fake := &fakePaymentService{}
	r := newTestRouter(New(fake), id)

	req := httptest.NewRequest(http.MethodGet, "/orders/RJK-TEST/payment-proofs", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if !fake.called {
		t.Fatalf("service.ListForOrder was never called (status=%d body=%s)", rec.Code, rec.Body.String())
	}
	if !fake.gotIsStaff {
		t.Fatalf("staff WITH payment.verify should bypass ownership check, got isStaff=false")
	}
}

func TestGetFile_StaffWithoutPaymentVerifyPermission_TreatedAsCustomer(t *testing.T) {
	id := &authapi.Identity{
		UserID: uuid.New(), UserType: authapi.UserTypeStaff,
		Permissions: []string{"design.review"},
	}
	tmp := filepath.Join(t.TempDir(), "proof.png")
	if err := os.WriteFile(tmp, []byte("x"), 0o600); err != nil {
		t.Fatalf("write temp proof file: %v", err)
	}
	fake := &fakePaymentService{handle: &service.ProofFileHandle{
		Proof:   &paymodel.PaymentProof{FileMimeType: "image/png", FileOriginalName: "proof.png"},
		AbsPath: tmp,
	}}
	r := newTestRouter(New(fake), id)

	req := httptest.NewRequest(http.MethodGet, "/payment-proofs/"+uuid.New().String()+"/file", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if !fake.called {
		t.Fatalf("service.GetProofFile was never called (status=%d)", rec.Code)
	}
	if fake.gotIsStaff {
		t.Fatalf("staff without payment.verify must be treated as non-staff (ownership enforced downstream), got isStaff=true")
	}
}

func TestGetFile_StaffWithPaymentVerifyPermission_Bypasses(t *testing.T) {
	id := &authapi.Identity{
		UserID: uuid.New(), UserType: authapi.UserTypeStaff,
		Permissions: []string{"payment.verify"},
	}
	tmp := filepath.Join(t.TempDir(), "proof.png")
	if err := os.WriteFile(tmp, []byte("x"), 0o600); err != nil {
		t.Fatalf("write temp proof file: %v", err)
	}
	fake := &fakePaymentService{handle: &service.ProofFileHandle{
		Proof:   &paymodel.PaymentProof{FileMimeType: "image/png", FileOriginalName: "proof.png"},
		AbsPath: tmp,
	}}
	r := newTestRouter(New(fake), id)

	req := httptest.NewRequest(http.MethodGet, "/payment-proofs/"+uuid.New().String()+"/file", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if !fake.called {
		t.Fatalf("service.GetProofFile was never called (status=%d)", rec.Code)
	}
	if !fake.gotIsStaff {
		t.Fatalf("staff WITH payment.verify should bypass ownership check, got isStaff=false")
	}
}

// ---------- §5: UploadProof responds with customer DTO, not admin DTO ----------

func TestUploadProof_RespondsWithCustomerDTO_NotAdminDTO(t *testing.T) {
	reviewer := uuid.New()
	orderID := uuid.New()
	fake := &fakePaymentService{proof: &paymodel.PaymentProof{
		ID:          uuid.New(),
		OrderID:     orderID, // must NOT leak into response body
		MetodeBayar: paymodel.MetodeBayarTransfer,
		Status:      paymodel.ProofPending,
		ReviewedBy:  &reviewer, // must NOT leak into response body
	}}
	callerOrderID := uuid.New()
	id := &authapi.Identity{UserID: uuid.New(), UserType: authapi.UserTypeCustomer, OrderID: &callerOrderID}
	r := newTestRouter(New(fake), id)

	body, contentType := multipartUploadBody(t)
	req := httptest.NewRequest(http.MethodPost, "/orders/RJK-TEST/payment-proof", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	respBody := rec.Body.String()
	if bytes.Contains([]byte(respBody), []byte("order_id")) {
		t.Fatalf("UploadProof response leaks order_id (admin DTO), body=%s", respBody)
	}
	if bytes.Contains([]byte(respBody), []byte("reviewed_by")) {
		t.Fatalf("UploadProof response leaks reviewed_by (admin DTO), body=%s", respBody)
	}
}
