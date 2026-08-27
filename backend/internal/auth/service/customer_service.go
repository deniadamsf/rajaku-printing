package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/auth/model"
	"github.com/rajaku-printing/backend/internal/auth/repository"
	"github.com/rajaku-printing/backend/internal/pkg/phone"
)

// customerUserStore narrows repository.UserRepository to what CustomerService
// needs — keeps the service unit-testable with an in-memory fake instead of a
// real *gorm.DB (§22 test requirement), same pattern as googleOAuthUserStore
// in google_oauth_service.go and userLookup in guest_order_service.go.
type customerUserStore interface {
	FindByPhone(ctx context.Context, phone string) (*model.User, error)
	Create(ctx context.Context, u *model.User) error
	FindByID(ctx context.Context, id uuid.UUID) (*model.User, error)
	SearchCustomers(ctx context.Context, q string, limit int) ([]model.User, error)
}

// CustomerService implements authapi.CustomerService. Terpisah dari Service
// (yang urus auth/token) supaya tanggung jawab jelas — hindari god-service
// yg pegang auth + customer lookup + registration + dll.
type CustomerService struct {
	users customerUserStore
}

var (
	_ authapi.CustomerService = (*CustomerService)(nil)
	_ customerUserStore       = (*repository.UserRepository)(nil)
)

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
		Phone:        &normalized,
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

// minSearchQueryLen — panjang minimum query pencarian pelanggan. Mencegah
// query kosong/1-karakter dijawab dengan sebuah listing pelanggan (endpoint
// pencarian, bukan endpoint daftar) — dicek di sini (bukan cuma di frontend)
// supaya pemanggil API langsung tidak bisa melewati batasnya.
//
// maxSearchQueryLen — batas atas panjang query. Endpoint ini tidak punya rate
// limit sendiri (dilindungi oleh permission staff), dan tidak ada kebutuhan
// bisnis untuk query nama/nomor WA sepanjang itu — dijepit di sini supaya
// query absurd panjang tidak diteruskan mentah-mentah ke ILIKE di DB.
//
// Panjang dihitung pakai rune (utf8.RuneCountInString), bukan len(), supaya
// nama pelanggan berkarakter multi-byte (mis. emoji/aksen) tidak salah
// terhitung lebih panjang dari yang sebenarnya diketik kasir.
const (
	minSearchQueryLen = 2
	maxSearchQueryLen = 100
)

// SearchCustomers implements authapi.CustomerService — cari customer existing
// by nama/WA (§11), dipakai layar kasir supaya tidak input ulang data
// pelanggan yang sudah pernah order (online maupun walk-in).
func (s *CustomerService) SearchCustomers(ctx context.Context, query string, limit int) ([]authapi.Identity, error) {
	query = strings.TrimSpace(query)
	queryLen := utf8.RuneCountInString(query)
	if queryLen < minSearchQueryLen || queryLen > maxSearchQueryLen {
		return []authapi.Identity{}, nil
	}
	users, err := s.users.SearchCustomers(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("search customers: %w", err)
	}
	out := make([]authapi.Identity, 0, len(users))
	for i := range users {
		out = append(out, *userToIdentity(&users[i]))
	}
	return out, nil
}

func userToIdentity(u *model.User) *authapi.Identity {
	id := &authapi.Identity{
		UserID:   u.ID,
		UserType: authapi.UserType(u.UserType),
		Name:     u.Name,
	}
	if u.Phone != nil {
		id.Phone = *u.Phone
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
