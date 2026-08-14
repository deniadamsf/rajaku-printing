package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/auth/model"
	"github.com/rajaku-printing/backend/internal/auth/password"
	"github.com/rajaku-printing/backend/internal/auth/repository"
)

// InviteConfig — knobs untuk lifecycle invite.
type InviteConfig struct {
	// TTL — berapa lama invite valid sejak dibuat. Default 24h.
	TTL time.Duration
}

// InviteStore — narrowed contract untuk testability.
type InviteStore interface {
	Create(ctx context.Context, inv *model.StaffInvite) error
	FindByTokenHash(ctx context.Context, tokenHash string) (*model.StaffInvite, error)
	MarkUsed(ctx context.Context, id uuid.UUID, at time.Time) error
}

// InviteUserStore — narrowed user ops yg dipakai invite service.
type InviteUserStore interface {
	FindByID(ctx context.Context, id uuid.UUID) (*model.User, error)
	SetPassword(ctx context.Context, id uuid.UUID, hash string) error
}

// InviteService — orchestrator staff-invite flow (§10).
type InviteService struct {
	invites InviteStore
	users   InviteUserStore
	cfg     InviteConfig
	nowFn   func() time.Time
}

// Compile-time assertion: konkret repo memenuhi interface.
var (
	_ InviteStore     = (*repository.StaffInviteRepository)(nil)
	_ InviteUserStore = (*repository.UserRepository)(nil)
)

func NewInviteService(invites InviteStore, users InviteUserStore, cfg InviteConfig) *InviteService {
	if cfg.TTL <= 0 {
		cfg.TTL = 24 * time.Hour
	}
	return &InviteService{invites: invites, users: users, cfg: cfg, nowFn: time.Now}
}

// CreateInvite — internal helper dipakai StaffAdminService.CreateStaff.
// Return raw token (untuk disematkan di URL yg dikirim ke staff) + row.
func (s *InviteService) CreateInvite(ctx context.Context, userID, createdBy uuid.UUID) (rawToken string, inv *model.StaffInvite, err error) {
	rawToken, tokenHash, err := generateToken()
	if err != nil {
		return "", nil, fmt.Errorf("generate invite token: %w", err)
	}
	now := s.nowFn().UTC()
	inv = &model.StaffInvite{
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: now.Add(s.cfg.TTL),
		CreatedBy: createdBy,
	}
	if err := s.invites.Create(ctx, inv); err != nil {
		return "", nil, fmt.Errorf("insert invite: %w", err)
	}
	return rawToken, inv, nil
}

// VerifyInvite — dipanggil handler GET /invites/verify?token=xxx. Return
// user info supaya frontend bisa preview "You are setting password for
// email@example.com". Tidak mark used_at.
type InviteVerification struct {
	UserID    uuid.UUID `json:"user_id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *InviteService) VerifyInvite(ctx context.Context, rawToken string) (*InviteVerification, error) {
	inv, err := s.lookupValid(ctx, rawToken)
	if err != nil {
		return nil, err
	}
	u, err := s.users.FindByID(ctx, inv.UserID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, authapi.ErrStaffNotFound
		}
		return nil, fmt.Errorf("load user for invite: %w", err)
	}
	email := ""
	if u.Email != nil {
		email = *u.Email
	}
	return &InviteVerification{
		UserID:    u.ID,
		Email:     email,
		Name:      u.Name,
		ExpiresAt: inv.ExpiresAt,
	}, nil
}

// AcceptInvite — dipanggil handler POST /invites/accept. Validasi, hash
// password, set active + password, mark invite used.
func (s *InviteService) AcceptInvite(ctx context.Context, rawToken, plainPassword string) error {
	if err := password.ValidateStrength(plainPassword); err != nil {
		return err // handler map ke validation error
	}
	inv, err := s.lookupValid(ctx, rawToken)
	if err != nil {
		return err
	}
	hash, err := password.Hash(plainPassword)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	// Sequence: mark used dulu (biar kalau SetPassword gagal, token tetap
	// consumed — mencegah replay). Toleransi: kalau SetPassword gagal, admin
	// harus reissue invite (manual). Ini tradeoff untuk keamanan.
	if err := s.invites.MarkUsed(ctx, inv.ID, s.nowFn().UTC()); err != nil {
		return fmt.Errorf("mark invite used: %w", err)
	}
	if err := s.users.SetPassword(ctx, inv.UserID, hash); err != nil {
		return fmt.Errorf("set password: %w", err)
	}
	return nil
}

// lookupValid — cari invite by hash, validasi expiry & used.
func (s *InviteService) lookupValid(ctx context.Context, rawToken string) (*model.StaffInvite, error) {
	hash := hashToken(rawToken)
	inv, err := s.invites.FindByTokenHash(ctx, hash)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, authapi.ErrInviteInvalid
		}
		return nil, fmt.Errorf("lookup invite: %w", err)
	}
	if inv.UsedAt != nil {
		return nil, authapi.ErrInviteAlreadyUsed
	}
	if s.nowFn().UTC().After(inv.ExpiresAt) {
		return nil, authapi.ErrInviteExpired
	}
	return inv, nil
}

// generateToken — 32 byte crypto random → base64url (43 char, tanpa padding).
// Return raw + sha256 hex hash.
func generateToken() (raw, hexHash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	raw = base64.RawURLEncoding.EncodeToString(buf)
	return raw, hashToken(raw), nil
}

// hashToken — SHA-256 hex. Deterministic, dipakai lookup.
func hashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}
