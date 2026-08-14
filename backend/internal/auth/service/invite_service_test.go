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
)

// ---------- fakes ----------

type fakeInviteStore struct {
	created   *model.StaffInvite
	createErr error
	byHash    *model.StaffInvite
	byHashErr error
	markedID  uuid.UUID
	markErr   error
}

func (f *fakeInviteStore) Create(_ context.Context, inv *model.StaffInvite) error {
	if f.createErr != nil {
		return f.createErr
	}
	if inv.ID == uuid.Nil {
		inv.ID = uuid.New()
	}
	f.created = inv
	return nil
}
func (f *fakeInviteStore) FindByTokenHash(_ context.Context, _ string) (*model.StaffInvite, error) {
	return f.byHash, f.byHashErr
}
func (f *fakeInviteStore) MarkUsed(_ context.Context, id uuid.UUID, _ time.Time) error {
	f.markedID = id
	return f.markErr
}

type fakeUserStore struct {
	user           *model.User
	userErr        error
	setPasswordID  uuid.UUID
	setPasswordArg string
	setPasswordErr error
}

func (f *fakeUserStore) FindByID(_ context.Context, _ uuid.UUID) (*model.User, error) {
	return f.user, f.userErr
}
func (f *fakeUserStore) SetPassword(_ context.Context, id uuid.UUID, hash string) error {
	f.setPasswordID = id
	f.setPasswordArg = hash
	return f.setPasswordErr
}

// ---------- tests ----------

func newInviteSvc(inv *fakeInviteStore, usr *fakeUserStore, ttl time.Duration) *InviteService {
	s := NewInviteService(inv, usr, InviteConfig{TTL: ttl})
	s.nowFn = func() time.Time { return time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC) }
	return s
}

func TestCreateInvite_HappyPath_ReturnsRawTokenAndInsertsHash(t *testing.T) {
	userID := uuid.New()
	inviterID := uuid.New()
	inv := &fakeInviteStore{}
	svc := newInviteSvc(inv, &fakeUserStore{}, 24*time.Hour)

	raw, row, err := svc.CreateInvite(context.Background(), userID, inviterID)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(raw) < 30 {
		t.Errorf("raw token suspiciously short: %d", len(raw))
	}
	if inv.created == nil {
		t.Fatal("create not called")
	}
	if inv.created.TokenHash == raw {
		t.Error("token_hash should NOT equal raw token — must be sha256 hash")
	}
	if inv.created.UserID != userID {
		t.Errorf("user_id mismatch")
	}
	if inv.created.CreatedBy != inviterID {
		t.Errorf("created_by mismatch")
	}
	// Expiry ~ now + 24h.
	wantExp := time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
	if !inv.created.ExpiresAt.Equal(wantExp) {
		t.Errorf("expires_at: want %s got %s", wantExp, inv.created.ExpiresAt)
	}
	if row.UserID != userID {
		t.Errorf("returned row user_id mismatch")
	}
}

func TestVerifyInvite_HappyPath(t *testing.T) {
	userID := uuid.New()
	// Precompute a token + hash — call CreateInvite via svc so hashing consistent.
	invSt := &fakeInviteStore{}
	email := "ali@x.co"
	usrSt := &fakeUserStore{user: &model.User{ID: userID, Name: "Ali", Email: &email}}
	svc := newInviteSvc(invSt, usrSt, 24*time.Hour)

	raw, _, err := svc.CreateInvite(context.Background(), userID, uuid.New())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// Wire the "byHash" field so VerifyInvite finds it. The service stores
	// hash of raw; we replay that lookup.
	invSt.byHash = invSt.created

	v, err := svc.VerifyInvite(context.Background(), raw)
	if err != nil {
		t.Fatalf("unexpected verify: %v", err)
	}
	if v.UserID != userID {
		t.Errorf("verify user_id mismatch")
	}
	if v.Email != "ali@x.co" {
		t.Errorf("verify email mismatch")
	}
}

func TestVerifyInvite_UnknownToken(t *testing.T) {
	svc := newInviteSvc(&fakeInviteStore{byHashErr: repository.ErrNotFound}, &fakeUserStore{}, time.Hour)
	_, err := svc.VerifyInvite(context.Background(), "some-random-token")
	if !errors.Is(err, authapi.ErrInviteInvalid) {
		t.Fatalf("want ErrInviteInvalid, got %v", err)
	}
}

func TestVerifyInvite_Expired(t *testing.T) {
	past := time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC) // 2 days ago
	svc := newInviteSvc(&fakeInviteStore{byHash: &model.StaffInvite{
		ID: uuid.New(), UserID: uuid.New(), ExpiresAt: past,
	}}, &fakeUserStore{}, time.Hour)
	_, err := svc.VerifyInvite(context.Background(), "anything")
	if !errors.Is(err, authapi.ErrInviteExpired) {
		t.Fatalf("want ErrInviteExpired, got %v", err)
	}
}

func TestVerifyInvite_AlreadyUsed(t *testing.T) {
	usedAt := time.Now()
	future := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	svc := newInviteSvc(&fakeInviteStore{byHash: &model.StaffInvite{
		ID: uuid.New(), UserID: uuid.New(),
		ExpiresAt: future, UsedAt: &usedAt,
	}}, &fakeUserStore{}, time.Hour)
	_, err := svc.VerifyInvite(context.Background(), "x")
	if !errors.Is(err, authapi.ErrInviteAlreadyUsed) {
		t.Fatalf("want ErrInviteAlreadyUsed, got %v", err)
	}
}

func TestAcceptInvite_HappyPath_HashesPasswordAndMarksUsed(t *testing.T) {
	userID := uuid.New()
	invSt := &fakeInviteStore{}
	usrSt := &fakeUserStore{user: &model.User{ID: userID, Name: "X"}}
	svc := newInviteSvc(invSt, usrSt, time.Hour)
	raw, _, err := svc.CreateInvite(context.Background(), userID, uuid.New())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	invSt.byHash = invSt.created

	if err := svc.AcceptInvite(context.Background(), raw, "strong-password-1234"); err != nil {
		t.Fatalf("accept: %v", err)
	}
	if invSt.markedID != invSt.created.ID {
		t.Errorf("invite not marked used")
	}
	if usrSt.setPasswordID != userID {
		t.Errorf("set password not called for right user")
	}
	if usrSt.setPasswordArg == "strong-password-1234" {
		t.Errorf("password should be hashed, not raw")
	}
	if len(usrSt.setPasswordArg) < 20 {
		t.Errorf("hashed password suspiciously short: %d", len(usrSt.setPasswordArg))
	}
}

func TestAcceptInvite_WeakPassword_Rejected(t *testing.T) {
	svc := newInviteSvc(&fakeInviteStore{}, &fakeUserStore{}, time.Hour)
	err := svc.AcceptInvite(context.Background(), "any-token", "short")
	if err == nil {
		t.Fatal("expected error for weak password, got nil")
	}
}
