package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/auth/model"
	"github.com/rajaku-printing/backend/internal/auth/password"
	"github.com/rajaku-printing/backend/internal/auth/repository"
	"github.com/rajaku-printing/backend/internal/auth/token"
	"github.com/rajaku-printing/backend/internal/pkg/phone"
)

// Service implements the business logic of the auth module. It implements
// authapi.Service so other modules can depend on the interface, not this
// struct.
type Service struct {
	users  *repository.UserRepository
	roles  *repository.RoleRepository
	issuer *token.Issuer
}

// Compile-time assertion: Service must satisfy authapi.Service.
var _ authapi.Service = (*Service)(nil)

func New(users *repository.UserRepository, roles *repository.RoleRepository, issuer *token.Issuer) *Service {
	return &Service{users: users, roles: roles, issuer: issuer}
}

// RegisterCustomer creates a new registered customer (user_type=customer,
// customer_type=registered) with email + password. Phone dinormalisasi ke 62xxx.
//
// Returns TokenPair so client bisa langsung login-in setelah register (skip
// round-trip ke /login).
func (s *Service) RegisterCustomer(ctx context.Context, in RegisterCustomerInput) (*TokenPair, *model.User, error) {
	email := strings.ToLower(strings.TrimSpace(in.Email))
	name := strings.TrimSpace(in.Name)

	if email == "" || !strings.Contains(email, "@") {
		return nil, nil, fmt.Errorf("email invalid: %w", errBadInput)
	}
	if name == "" {
		return nil, nil, fmt.Errorf("name required: %w", errBadInput)
	}
	if err := password.ValidateStrength(in.Password); err != nil {
		return nil, nil, err
	}

	normalizedPhone, err := phone.Normalize(in.Phone)
	if err != nil {
		return nil, nil, fmt.Errorf("phone invalid: %w", err)
	}

	// Uniqueness check — race-safe karena unique index di DB tetap jadi backstop.
	if exists, err := s.users.ExistsByEmail(ctx, email); err != nil {
		return nil, nil, fmt.Errorf("check email uniqueness: %w", err)
	} else if exists {
		return nil, nil, authapi.ErrEmailAlreadyUsed
	}
	if exists, err := s.users.ExistsByPhone(ctx, normalizedPhone); err != nil {
		return nil, nil, fmt.Errorf("check phone uniqueness: %w", err)
	} else if exists {
		return nil, nil, authapi.ErrPhoneAlreadyUsed
	}

	hash, err := password.Hash(in.Password)
	if err != nil {
		return nil, nil, fmt.Errorf("hash password: %w", err)
	}

	custType := model.CustomerTypeRegistered
	u := &model.User{
		Email:        &email,
		Phone:        &normalizedPhone,
		Name:         name,
		PasswordHash: &hash,
		UserType:     model.UserTypeCustomer,
		CustomerType: &custType,
		IsActive:     true,
	}
	if err := s.users.Create(ctx, u); err != nil {
		return nil, nil, fmt.Errorf("create customer: %w", err)
	}

	pair, err := s.issueTokenFor(u)
	if err != nil {
		return nil, nil, err
	}
	return pair, u, nil
}

// Login validates email+password and issues an access token.
//
// Failure modes intentionally return the same generic ErrInvalidPassword —
// jangan bocorkan apakah email atau password yang salah (defense against
// account-enumeration).
func (s *Service) Login(ctx context.Context, in LoginInput) (*TokenPair, *model.User, error) {
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if email == "" || in.Password == "" {
		return nil, nil, authapi.ErrInvalidPassword
	}

	u, err := s.users.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, nil, authapi.ErrInvalidPassword
		}
		return nil, nil, fmt.Errorf("lookup user: %w", err)
	}
	if !u.IsActive {
		return nil, nil, authapi.ErrUserInactive
	}
	if u.PasswordHash == nil {
		// OAuth-only user tanpa password lokal
		return nil, nil, authapi.ErrInvalidPassword
	}
	if err := password.Verify(*u.PasswordHash, in.Password); err != nil {
		if errors.Is(err, password.ErrMismatch) {
			return nil, nil, authapi.ErrInvalidPassword
		}
		return nil, nil, fmt.Errorf("verify password: %w", err)
	}

	if err := s.users.UpdateLastLogin(ctx, u.ID); err != nil {
		// Non-fatal — log tapi jangan block login. Untuk sekarang wrap saja.
		return nil, nil, fmt.Errorf("update last_login: %w", err)
	}

	pair, err := s.issueTokenFor(u)
	if err != nil {
		return nil, nil, err
	}
	return pair, u, nil
}

// GetMe returns the current user's profile projection.
func (s *Service) GetMe(ctx context.Context, id uuid.UUID) (*MeOutput, error) {
	u, err := s.users.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, authapi.ErrUserNotFound
		}
		return nil, fmt.Errorf("get me: %w", err)
	}
	out := &MeOutput{
		UserID:        u.ID,
		UserType:      string(u.UserType),
		Name:          u.Name,
		Roles:         roleNames(u.Roles),
		Permissions:   flattenPermissions(u.Roles),
		PhoneVerified: u.PhoneVerifiedAt != nil,
	}
	if u.Phone != nil {
		out.Phone = *u.Phone
	}
	if u.Email != nil {
		out.Email = *u.Email
	}
	return out, nil
}

// VerifyToken implements authapi.Service — dipanggil middleware RequireAuth.
func (s *Service) VerifyToken(ctx context.Context, raw string) (*authapi.Identity, error) {
	claims, err := s.issuer.Verify(raw)
	if err != nil {
		if errors.Is(err, token.ErrExpiredToken) {
			return nil, authapi.ErrExpiredToken
		}
		return nil, authapi.ErrInvalidToken
	}
	uid, err := uuid.Parse(claims.UserID)
	if err != nil {
		return nil, authapi.ErrInvalidToken
	}
	var orderID *uuid.UUID
	if claims.OrderID != "" {
		oid, err := uuid.Parse(claims.OrderID)
		if err != nil {
			return nil, authapi.ErrInvalidToken
		}
		orderID = &oid
	}
	return &authapi.Identity{
		UserID:      uid,
		UserType:    authapi.UserType(claims.UserType),
		Email:       claims.Email,
		Phone:       claims.Phone,
		Roles:       claims.Roles,
		Permissions: claims.Permissions,
		Scope:       claims.Scope,
		OrderID:     orderID,
	}, nil
}

func (s *Service) issueTokenFor(u *model.User) (*TokenPair, error) {
	return issueTokenFor(s.issuer, u)
}

// issueTokenFor is a package-level helper (not a Service method) so both
// Service (email+password login/register) and GoogleOAuthService (Google
// login/registration) can share the exact same claim-building logic without
// GoogleOAuthService needing to embed/depend on Service — avoids a
// god-service (§22) while keeping token issuance in one place.
func issueTokenFor(issuer *token.Issuer, u *model.User) (*TokenPair, error) {
	claims := token.Claims{
		UserID:      u.ID.String(),
		UserType:    string(u.UserType),
		Roles:       roleNames(u.Roles),
		Permissions: flattenPermissions(u.Roles),
	}
	if u.Phone != nil {
		claims.Phone = *u.Phone
	}
	if u.Email != nil {
		claims.Email = *u.Email
	}
	// Namanya di JWT dihilangkan (bukan info otorisasi; hemat byte).
	tok, err := issuer.Sign(claims)
	if err != nil {
		return nil, fmt.Errorf("sign token: %w", err)
	}
	return &TokenPair{
		AccessToken: tok,
		TokenType:   "Bearer",
		ExpiresIn:   issuer.TTL(),
	}, nil
}

func roleNames(rs []model.Role) []string {
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.Name)
	}
	return out
}

func flattenPermissions(rs []model.Role) []string {
	seen := make(map[string]struct{})
	out := []string{}
	for _, r := range rs {
		for _, p := range r.Permissions {
			if _, ok := seen[p.Code]; ok {
				continue
			}
			seen[p.Code] = struct{}{}
			out = append(out, p.Code)
		}
	}
	return out
}

// errBadInput is a sentinel wrapped by input-validation errors — handler maps
// to 400. Kept private; not part of the public authapi errors set.
var errBadInput = errors.New("service: bad input")

// IsBadInput reports whether err came from input validation.
func IsBadInput(err error) bool { return errors.Is(err, errBadInput) }
