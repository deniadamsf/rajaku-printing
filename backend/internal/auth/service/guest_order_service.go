package service

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/auth/model"
	"github.com/rajaku-printing/backend/internal/auth/repository"
	"github.com/rajaku-printing/backend/internal/auth/token"
	"github.com/rajaku-printing/backend/internal/order/orderapi"
	"github.com/rajaku-printing/backend/internal/pkg/phone"
)

// userLookup narrows repository.UserRepository to what GuestOrderService
// needs — keeps the service testable with an in-memory fake (§22 test
// requirement) instead of a real *gorm.DB.
type userLookup interface {
	FindByID(ctx context.Context, id uuid.UUID) (*model.User, error)
}

// GuestOrderService implements the guest-checkout ownership-proof flow: a
// customer without a password proves control of an order by supplying its
// resi + the WhatsApp number tied to the order, and receives a short-TTL,
// scope-limited access token (authapi.ScopeGuestOrder) usable ONLY on
// endpoints that opt into that scope (see authapi.RequireAuthAllowScope).
//
// Kept separate from Service (login/register/me) — different concern, avoids
// a god-service (§22).
type GuestOrderService struct {
	users  userLookup
	issuer *token.Issuer // short-TTL issuer (JWT_GUEST_ORDER_TTL), same secret/issuer as the main Issuer so authapi.Service.VerifyToken can verify it.

	// orderCmd is injected via setter AFTER construction — router.go builds
	// authSvc/GuestOrderService BEFORE orderSvc exists (order module needs
	// authapi.CustomerService first). Same pattern as order.Service.SetNotifier.
	orderCmd orderapi.OrderCommandService
}

func NewGuestOrderService(users *repository.UserRepository, issuer *token.Issuer) *GuestOrderService {
	return &GuestOrderService{users: users, issuer: issuer}
}

// SetOrderCommandService wires the order module's command interface. MUST be
// called before VerifyOwnership is ever invoked in production (router.go does
// this at composition-root time, right after orderSvc is constructed).
func (s *GuestOrderService) SetOrderCommandService(oc orderapi.OrderCommandService) {
	s.orderCmd = oc
}

// VerifyOwnership checks that `phoneRaw` (any accepted format — 08xx / 62xxx
// / +62xxx, normalized internally) matches the WhatsApp number of the
// customer who owns the order identified by `resi`. On success it issues a
// scope-limited token whose `uid` is that customer's real user_id — so every
// existing ownership check elsewhere (mis. `order.CustomerID != caller.UserID`
// in the design service) keeps working unchanged.
//
// The token is issued when the order's owner is an ACTIVE user (is_active=true)
// and the provided phone matches the customer's phone (or the order's shipping
// recipient phone). Both guest and registered customers (as well as staff who placed
// an order) can verify via this endpoint. Defense in depth: the issued token always
// carries `UserType: customer` (never staff) and has `Scope: ScopeGuestOrder` strictly
// bound to `OrderID: summary.ID`.
//
// "resi not found", "phone mismatch", and "inactive account"
// all return the exact same sentinel (authapi.ErrGuestVerificationFailed) —
// callers MUST NOT branch differently on them, to avoid leaking which resi
// numbers are valid, and to avoid leaking whether a given resi belongs to
// a deactivated account.
func (s *GuestOrderService) VerifyOwnership(ctx context.Context, resi, phoneRaw string) (*GuestOrderToken, error) {
	if s.orderCmd == nil {
		return nil, fmt.Errorf("verify guest ownership resi %s: order command service not wired", resi)
	}

	normalizedPhone, err := phone.Normalize(phoneRaw)
	if err != nil {
		return nil, fmt.Errorf("normalize phone: %w", err)
	}

	summary, err := s.orderCmd.FindSummaryByResi(ctx, resi)
	if err != nil {
		if errors.Is(err, orderapi.ErrOrderNotFound) {
			s.dummyOwnerLookup(ctx)
			return nil, authapi.ErrGuestVerificationFailed
		}
		return nil, fmt.Errorf("verify guest ownership resi %s: find order: %w", resi, err)
	}

	owner, err := s.users.FindByID(ctx, summary.CustomerID)
	if err != nil {
		// Order exists but its customer_id has no row — data integrity bug,
		// not a client input problem. Do NOT map to the generic 401 (that
		// sentinel is reserved for "resi/phone don't match"); let it surface
		// as a 500 so it gets logged & investigated.
		return nil, fmt.Errorf("verify guest ownership resi %s: find owner user %s: %w", resi, summary.CustomerID, err)
	}

	// Constant-time compare — defense against timing side-channels on phone
	// guessing (spec requirement). Differing lengths already yield "no match"
	// without a data-dependent branch. ownerPhone == "" (nil Phone) can never
	// match — phone.Normalize above always yields a non-empty string.
	ownerPhone := ""
	if owner.Phone != nil {
		ownerPhone = *owner.Phone
	}
	phoneMatches := subtle.ConstantTimeCompare([]byte(ownerPhone), []byte(normalizedPhone)) == 1
	if !phoneMatches && summary.ShippingRecipientPhone != nil {
		if shipNorm, err := phone.Normalize(*summary.ShippingRecipientPhone); err == nil {
			phoneMatches = subtle.ConstantTimeCompare([]byte(shipNorm), []byte(normalizedPhone)) == 1
		}
	}
	if !phoneMatches || !owner.IsActive {
		return nil, authapi.ErrGuestVerificationFailed
	}

	claims := token.Claims{
		UserID: owner.ID.String(),
		// Deliberately the literal customer constant, NOT owner.UserType —
		// defense in depth: even if the gate above is ever loosened, the
		// issued token can never claim staff identity.
		UserType: string(model.UserTypeCustomer),
		Phone:    ownerPhone,
		Scope:    authapi.ScopeGuestOrder,
		// Ties this token to the ONE order just verified — without this, a
		// guest token from one resi would work for every other order owned
		// by the same phone number (§3 review finding). Enforced downstream
		// by the design service against designapi.ErrNotOrderOwner.
		OrderID: summary.ID.String(),
	}
	signed, err := s.issuer.Sign(claims)
	if err != nil {
		return nil, fmt.Errorf("sign guest order token resi %s: %w", resi, err)
	}

	return &GuestOrderToken{
		AccessToken: signed,
		ExpiresAt:   time.Now().UTC().Add(s.issuer.TTL()),
	}, nil
}

// dummyOwnerLookup performs a lookup shaped exactly like the real second
// roundtrip (order found → fetch its owner by ID) but against a random,
// virtually-guaranteed-absent UUID, purely to burn a comparable amount of
// time/DB work. See the timing note in VerifyOwnership for what this does
// and does not guarantee.
func (s *GuestOrderService) dummyOwnerLookup(ctx context.Context) {
	// Hasilnya memang tidak dipakai, tapi error-nya tidak boleh ditelan diam-diam
	// (§22). ErrNotFound adalah hasil yang DIHARAPKAN — UUID acak. Selain itu
	// berarti DB bermasalah: kalau tidak dicatat, jalur ini diam sementara jalur
	// lain balas 500, dan justru itu jadi sinyal baru untuk membedakan
	// "resi ada / tidak ada" — kebalikan dari tujuan fungsi ini.
	if _, err := s.users.FindByID(ctx, uuid.New()); err != nil && !errors.Is(err, repository.ErrNotFound) {
		log.Ctx(ctx).Error().Err(err).Msg("guest verify: dummy owner lookup gagal — cek koneksi DB")
	}
}
