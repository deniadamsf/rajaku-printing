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
// The token is ONLY issued when the order's owner is an ACTIVE, GUEST
// customer (user_type=customer AND customer_type=guest AND is_active=true).
// `users` table is shared across staff + customer (phone is the unique
// matching key, §11) and FindByID does not filter by user_type — so without
// this gate, a POS order created against a staff member's WA number would
// hand out a token carrying that staff's real identity (typ=staff), and the
// design service's IsStaff bypass would let the caller reach ANY order's
// files. Registered customers are excluded too: they already have a
// password + `/akun/pesanan/:resi` — letting a bare WA number authenticate
// into a password-protected account would be a downgrade of their account
// security, not a convenience.
//
// "resi not found", "phone mismatch", and "owner fails the guest/active gate"
// all return the exact same sentinel (authapi.ErrGuestVerificationFailed) —
// callers MUST NOT branch differently on them, to avoid leaking which resi
// numbers are valid, and to avoid leaking whether a given resi belongs to
// staff / a registered account / a deactivated account.
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
			// Timing note: the success/mismatch path below always performs a
			// SECOND lookup (owner by ID) after this one. Without doing
			// equivalent work here, "resi not found" would return after only
			// one DB roundtrip while every other outcome takes two — a gap
			// large enough to fingerprint valid resi numbers by latency
			// alone. dummyOwnerLookup pays that same second roundtrip before
			// returning.
			//
			// Batas jaminannya, supaya tidak ada yang menganggap ini beres:
			// FindByID memakai Preload("Roles.Permissions"), dan GORM MELEWATI
			// query preload kalau query utamanya tidak menemukan row. Karena
			// UUID acak di bawah dijamin miss, jalur ini membakar 1 query
			// sementara jalur "resi ada, nomor salah" membakar 2. Jadi
			// selisihnya mengecil dari "1 vs 2 roundtrip" menjadi "1 vs 2
			// query dengan yang kedua ringan" — bukan nol. Menutupnya sampai
			// benar-benar setara butuh lookup dummy yang meniru preload juga,
			// atau menyamakan durasi respons di lapisan handler.
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
	isActiveGuestCustomer := owner.UserType == model.UserTypeCustomer &&
		owner.CustomerType != nil && *owner.CustomerType == model.CustomerTypeGuest &&
		owner.IsActive
	if !phoneMatches || !isActiveGuestCustomer {
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
