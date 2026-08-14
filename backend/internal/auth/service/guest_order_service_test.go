package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/auth/model"
	"github.com/rajaku-printing/backend/internal/auth/repository"
	"github.com/rajaku-printing/backend/internal/auth/token"
	"github.com/rajaku-printing/backend/internal/order/orderapi"
)

// ---------- fakes ----------

type fakeUserLookup struct {
	byID map[uuid.UUID]*model.User
	err  error
}

func (f *fakeUserLookup) FindByID(_ context.Context, id uuid.UUID) (*model.User, error) {
	if f.err != nil {
		return nil, f.err
	}
	u, ok := f.byID[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return u, nil
}

// fakeGuestOrderCmd implements the full orderapi.OrderCommandService — only
// FindSummaryByResi matters for GuestOrderService, the rest are unused no-ops
// (same pattern as fakeOrderCmd in design/service tests).
type fakeGuestOrderCmd struct {
	summary    *orderapi.OrderSummary
	summaryErr error
}

func (f *fakeGuestOrderCmd) FindSummaryByResi(context.Context, string) (*orderapi.OrderSummary, error) {
	return f.summary, f.summaryErr
}
func (f *fakeGuestOrderCmd) FindSummaryByID(context.Context, uuid.UUID) (*orderapi.OrderSummary, error) {
	return f.summary, f.summaryErr
}
func (f *fakeGuestOrderCmd) FindInvoiceViewByID(context.Context, uuid.UUID) (*orderapi.OrderInvoiceView, error) {
	return nil, nil
}
func (f *fakeGuestOrderCmd) MarkPendingVerification(context.Context, uuid.UUID, *uuid.UUID, string) error {
	return nil
}
func (f *fakeGuestOrderCmd) MarkDibayar(context.Context, uuid.UUID, string, *uuid.UUID, string) error {
	return nil
}
func (f *fakeGuestOrderCmd) MarkDitolak(context.Context, uuid.UUID, *uuid.UUID, string) error {
	return nil
}
func (f *fakeGuestOrderCmd) MarkDesainDikerjakan(context.Context, uuid.UUID, *uuid.UUID, string) error {
	return nil
}
func (f *fakeGuestOrderCmd) MarkMenungguApprovalDesain(context.Context, uuid.UUID, *uuid.UUID, string) error {
	return nil
}
func (f *fakeGuestOrderCmd) MarkDesainDiverifikasi(context.Context, uuid.UUID, *uuid.UUID, string) error {
	return nil
}
func (f *fakeGuestOrderCmd) MarkProsesCetak(context.Context, uuid.UUID, *uuid.UUID, string) error {
	return nil
}
func (f *fakeGuestOrderCmd) MarkQC(context.Context, uuid.UUID, *uuid.UUID, string) error {
	return nil
}
func (f *fakeGuestOrderCmd) MarkSiapKirimAtauAmbil(context.Context, uuid.UUID, *uuid.UUID, string) error {
	return nil
}
func (f *fakeGuestOrderCmd) MarkDikirim(context.Context, uuid.UUID, *uuid.UUID, string, string, string) error {
	return nil
}
func (f *fakeGuestOrderCmd) MarkSelesai(context.Context, uuid.UUID, *uuid.UUID, string) error {
	return nil
}
func (f *fakeGuestOrderCmd) CreatePOSOrder(context.Context, orderapi.POSCreateOrderInput) (*orderapi.OrderSummary, error) {
	return nil, nil
}
func (f *fakeGuestOrderCmd) ListPOSOrdersByDate(context.Context, time.Time) ([]orderapi.OrderSummary, error) {
	return nil, nil
}

// ---------- helper ----------

func newTestGuestOrderService(t *testing.T, users *fakeUserLookup, cmd *fakeGuestOrderCmd) *GuestOrderService {
	t.Helper()
	iss, err := token.NewIssuer("test-secret-32-chars-minimum-abcdef", 30*time.Minute, "rajaku-test")
	if err != nil {
		t.Fatalf("NewIssuer: %v", err)
	}
	s := &GuestOrderService{users: users, issuer: iss}
	s.SetOrderCommandService(cmd)
	return s
}

// ---------- tests ----------

func TestGuestOrderService_VerifyOwnership_HappyPath_AcceptsLocalAndInternationalFormat(t *testing.T) {
	custID := uuid.New()
	orderID := uuid.New()
	guest := model.CustomerTypeGuest
	owner := &model.User{
		ID:           custID,
		Phone:        "6281234567890",
		Name:         "Budi Guest",
		UserType:     model.UserTypeCustomer,
		CustomerType: &guest,
		IsActive:     true,
	}
	users := &fakeUserLookup{byID: map[uuid.UUID]*model.User{custID: owner}}
	cmd := &fakeGuestOrderCmd{summary: &orderapi.OrderSummary{
		ID: orderID, Resi: "RJK-ABC123", CustomerID: custID, Status: "dibayar",
	}}
	svc := newTestGuestOrderService(t, users, cmd)

	for _, phoneInput := range []string{"081234567890", "6281234567890", "+6281234567890"} {
		out, err := svc.VerifyOwnership(context.Background(), "RJK-ABC123", phoneInput)
		if err != nil {
			t.Fatalf("phone=%q: unexpected err: %v", phoneInput, err)
		}
		if out.AccessToken == "" {
			t.Fatalf("phone=%q: expected non-empty token", phoneInput)
		}

		claims, err := svc.issuer.Verify(out.AccessToken)
		if err != nil {
			t.Fatalf("phone=%q: verify issued token: %v", phoneInput, err)
		}
		if claims.Scope != authapi.ScopeGuestOrder {
			t.Fatalf("phone=%q: expected scope %q, got %q", phoneInput, authapi.ScopeGuestOrder, claims.Scope)
		}
		if claims.UserID != custID.String() {
			t.Fatalf("phone=%q: expected uid %s, got %s", phoneInput, custID, claims.UserID)
		}
		if out.ExpiresAt.Before(time.Now().UTC()) {
			t.Fatalf("phone=%q: expected ExpiresAt in the future, got %v", phoneInput, out.ExpiresAt)
		}
	}
}

func TestGuestOrderService_VerifyOwnership_PhoneMismatch_ReturnsGenericSentinel(t *testing.T) {
	custID := uuid.New()
	owner := &model.User{ID: custID, Phone: "6281111111111", UserType: model.UserTypeCustomer}
	users := &fakeUserLookup{byID: map[uuid.UUID]*model.User{custID: owner}}
	cmd := &fakeGuestOrderCmd{summary: &orderapi.OrderSummary{
		ID: uuid.New(), Resi: "RJK-XYZ999", CustomerID: custID, Status: "dibayar",
	}}
	svc := newTestGuestOrderService(t, users, cmd)

	_, err := svc.VerifyOwnership(context.Background(), "RJK-XYZ999", "089999999999")
	if !errors.Is(err, authapi.ErrGuestVerificationFailed) {
		t.Fatalf("expected ErrGuestVerificationFailed, got %v", err)
	}
}

func TestGuestOrderService_VerifyOwnership_ResiNotFound_SameSentinelAsMismatch(t *testing.T) {
	users := &fakeUserLookup{byID: map[uuid.UUID]*model.User{}}
	cmd := &fakeGuestOrderCmd{summaryErr: orderapi.ErrOrderNotFound}
	svc := newTestGuestOrderService(t, users, cmd)

	_, err := svc.VerifyOwnership(context.Background(), "RJK-NOPE0000", "081234567890")
	if !errors.Is(err, authapi.ErrGuestVerificationFailed) {
		t.Fatalf("expected ErrGuestVerificationFailed (identical to phone-mismatch case), got %v", err)
	}
}

func TestGuestOrderService_VerifyOwnership_StaffOwner_RejectedSameSentinel(t *testing.T) {
	// §1 review finding: FindByID does not filter by user_type, so a POS
	// order created against a staff member's WA number must NOT hand out a
	// token carrying typ=staff.
	staffID := uuid.New()
	owner := &model.User{
		ID: staffID, Phone: "6281234567890", UserType: model.UserTypeStaff, IsActive: true,
	}
	users := &fakeUserLookup{byID: map[uuid.UUID]*model.User{staffID: owner}}
	cmd := &fakeGuestOrderCmd{summary: &orderapi.OrderSummary{
		ID: uuid.New(), Resi: "RJK-STAFF01", CustomerID: staffID, Status: "dibayar",
	}}
	svc := newTestGuestOrderService(t, users, cmd)

	_, err := svc.VerifyOwnership(context.Background(), "RJK-STAFF01", "081234567890")
	if !errors.Is(err, authapi.ErrGuestVerificationFailed) {
		t.Fatalf("expected ErrGuestVerificationFailed (identical to phone-mismatch case) for staff owner, got %v", err)
	}
}

func TestGuestOrderService_VerifyOwnership_RegisteredCustomerOwner_RejectedSameSentinel(t *testing.T) {
	// Registered customers already have a password + /akun/pesanan/:resi;
	// letting a bare WA number authenticate into their account would
	// downgrade account security, so guest verification must reject them too.
	custID := uuid.New()
	registered := model.CustomerTypeRegistered
	owner := &model.User{
		ID: custID, Phone: "6281234567890", UserType: model.UserTypeCustomer,
		CustomerType: &registered, IsActive: true,
	}
	users := &fakeUserLookup{byID: map[uuid.UUID]*model.User{custID: owner}}
	cmd := &fakeGuestOrderCmd{summary: &orderapi.OrderSummary{
		ID: uuid.New(), Resi: "RJK-REG01", CustomerID: custID, Status: "dibayar",
	}}
	svc := newTestGuestOrderService(t, users, cmd)

	_, err := svc.VerifyOwnership(context.Background(), "RJK-REG01", "081234567890")
	if !errors.Is(err, authapi.ErrGuestVerificationFailed) {
		t.Fatalf("expected ErrGuestVerificationFailed (identical to phone-mismatch case) for registered owner, got %v", err)
	}
}

func TestGuestOrderService_VerifyOwnership_InactiveOwner_RejectedSameSentinel(t *testing.T) {
	custID := uuid.New()
	guest := model.CustomerTypeGuest
	owner := &model.User{
		ID: custID, Phone: "6281234567890", UserType: model.UserTypeCustomer,
		CustomerType: &guest, IsActive: false,
	}
	users := &fakeUserLookup{byID: map[uuid.UUID]*model.User{custID: owner}}
	cmd := &fakeGuestOrderCmd{summary: &orderapi.OrderSummary{
		ID: uuid.New(), Resi: "RJK-INACTIVE01", CustomerID: custID, Status: "dibayar",
	}}
	svc := newTestGuestOrderService(t, users, cmd)

	_, err := svc.VerifyOwnership(context.Background(), "RJK-INACTIVE01", "081234567890")
	if !errors.Is(err, authapi.ErrGuestVerificationFailed) {
		t.Fatalf("expected ErrGuestVerificationFailed (identical to phone-mismatch case) for inactive owner, got %v", err)
	}
}

func TestGuestOrderService_VerifyOwnership_ActiveGuestOwner_TokenCarriesCustomerTypeAndOrderID(t *testing.T) {
	custID := uuid.New()
	orderID := uuid.New()
	guest := model.CustomerTypeGuest
	owner := &model.User{
		ID: custID, Phone: "6281234567890", UserType: model.UserTypeCustomer,
		CustomerType: &guest, IsActive: true,
	}
	users := &fakeUserLookup{byID: map[uuid.UUID]*model.User{custID: owner}}
	cmd := &fakeGuestOrderCmd{summary: &orderapi.OrderSummary{
		ID: orderID, Resi: "RJK-GUESTOK", CustomerID: custID, Status: "dibayar",
	}}
	svc := newTestGuestOrderService(t, users, cmd)

	out, err := svc.VerifyOwnership(context.Background(), "RJK-GUESTOK", "081234567890")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	claims, err := svc.issuer.Verify(out.AccessToken)
	if err != nil {
		t.Fatalf("verify issued token: %v", err)
	}
	if claims.UserType != string(model.UserTypeCustomer) {
		t.Fatalf("expected typ=customer, got %q", claims.UserType)
	}
	if claims.Scope != authapi.ScopeGuestOrder {
		t.Fatalf("expected scp=%q, got %q", authapi.ScopeGuestOrder, claims.Scope)
	}
	if claims.OrderID != orderID.String() {
		t.Fatalf("expected oid=%s, got %q", orderID, claims.OrderID)
	}
}

func TestGuestOrderService_VerifyOwnership_InvalidPhoneFormat_NotGenericSentinel(t *testing.T) {
	// Malformed input is a client validation error (mapped to 400 by handler),
	// distinct from the 401 "resi/phone don't match" case — must NOT be
	// ErrGuestVerificationFailed.
	custID := uuid.New()
	owner := &model.User{ID: custID, Phone: "6281234567890", UserType: model.UserTypeCustomer}
	users := &fakeUserLookup{byID: map[uuid.UUID]*model.User{custID: owner}}
	cmd := &fakeGuestOrderCmd{summary: &orderapi.OrderSummary{
		ID: uuid.New(), Resi: "RJK-BADPHONE", CustomerID: custID, Status: "dibayar",
	}}
	svc := newTestGuestOrderService(t, users, cmd)

	_, err := svc.VerifyOwnership(context.Background(), "RJK-BADPHONE", "not-a-phone")
	if err == nil {
		t.Fatal("expected error for malformed phone, got nil")
	}
	if errors.Is(err, authapi.ErrGuestVerificationFailed) {
		t.Fatalf("malformed phone should not map to ErrGuestVerificationFailed, got %v", err)
	}
}
