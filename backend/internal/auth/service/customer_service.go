package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/auth/model"
	"github.com/rajaku-printing/backend/internal/auth/repository"
	"github.com/rajaku-printing/backend/internal/pkg/phone"
)

// CustomerService implements authapi.CustomerService. Terpisah dari Service
// (yang urus auth/token) supaya tanggung jawab jelas — hindari god-service
// yg pegang auth + customer lookup + registration + dll.
type CustomerService struct {
	users *repository.UserRepository
}

var _ authapi.CustomerService = (*CustomerService)(nil)

func NewCustomerService(users *repository.UserRepository) *CustomerService {
	return &CustomerService{users: users}
}

// ResolveOrCreateGuest returns an existing customer identity matched by phone,
// or creates a new guest customer if none exists (spec section 11 — nomor WA
// sebagai matching key lintas channel).
//
// phoneRaw MUST valid (Normalize di dalam) — kalau kotor akan error.
func (s *CustomerService) ResolveOrCreateGuest(ctx context.Context, phoneRaw, name string) (*authapi.Identity, error) {
	normalized, err := phone.Normalize(phoneRaw)
	if err != nil {
		return nil, fmt.Errorf("normalize phone: %w", err)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("name required for guest customer")
	}

	existing, err := s.users.FindByPhone(ctx, normalized)
	if err == nil {
		return userToIdentity(existing), nil
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return nil, fmt.Errorf("lookup by phone: %w", err)
	}

	// Not found — create guest.
	guestType := model.CustomerTypeGuest
	u := &model.User{
		Phone:        normalized,
		Name:         name,
		UserType:     model.UserTypeCustomer,
		CustomerType: &guestType,
		IsActive:     true,
	}
	if err := s.users.Create(ctx, u); err != nil {
		// Race: another request may have created the customer with same phone
		// between our FindByPhone and Create. Re-fetch as fallback.
		if again, ferr := s.users.FindByPhone(ctx, normalized); ferr == nil {
			return userToIdentity(again), nil
		}
		return nil, fmt.Errorf("create guest customer: %w", err)
	}
	return userToIdentity(u), nil
}

// FindByID implements authapi.CustomerService — lookup by primary key.
func (s *CustomerService) FindByID(ctx context.Context, id uuid.UUID) (*authapi.Identity, error) {
	u, err := s.users.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, authapi.ErrCustomerNotFound
		}
		return nil, fmt.Errorf("find user by id %s: %w", id, err)
	}
	return userToIdentity(u), nil
}

func userToIdentity(u *model.User) *authapi.Identity {
	id := &authapi.Identity{
		UserID:   u.ID,
		UserType: authapi.UserType(u.UserType),
		Phone:    u.Phone,
		Name:     u.Name,
	}
	if u.Email != nil {
		id.Email = *u.Email
	}
	for _, r := range u.Roles {
		id.Roles = append(id.Roles, r.Name)
		for _, p := range r.Permissions {
			id.Permissions = append(id.Permissions, p.Code)
		}
	}
	return id
}
