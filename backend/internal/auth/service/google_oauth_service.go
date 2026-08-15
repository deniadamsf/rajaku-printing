package service

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/auth/model"
	"github.com/rajaku-printing/backend/internal/auth/oauth"
	"github.com/rajaku-printing/backend/internal/auth/repository"
	"github.com/rajaku-printing/backend/internal/auth/token"
	"github.com/rajaku-printing/backend/internal/notification/notificationapi"
	"github.com/rajaku-printing/backend/internal/pkg/phone"
)

// Handoff code TTLs (§ Google OAuth brief). Deliberately short — these codes
// only need to survive one browser round-trip (callback redirect → frontend
// POST /exchange) or one short form-fill (registration → phone number →
// POST /complete).
const (
	googleSessionCodeTTL      = 2 * time.Minute
	googleRegistrationCodeTTL = 15 * time.Minute
)

// googleOAuthUserStore narrows repository.UserRepository to what
// GoogleOAuthService needs — keeps the service unit-testable with an
// in-memory fake instead of a real *gorm.DB (§22 test requirement), same
// pattern as userLookup in guest_order_service.go.
type googleOAuthUserStore interface {
	FindByID(ctx context.Context, id uuid.UUID) (*model.User, error)
	FindByOAuth(ctx context.Context, provider, subject string) (*model.User, error)
	FindByEmail(ctx context.Context, email string) (*model.User, error)
	FindByPhone(ctx context.Context, phone string) (*model.User, error)
	Create(ctx context.Context, u *model.User) error
	UpdateLastLogin(ctx context.Context, id uuid.UUID) error
	ExistsByEmail(ctx context.Context, email string) (bool, error)
	LinkOAuth(ctx context.Context, id uuid.UUID, provider, subject string) error
	UpgradeGuestToRegistered(ctx context.Context, id uuid.UUID, email *string, name, provider, subject string) error
}

// googleOAuthCodeStore narrows repository.OAuthLoginCodeRepository.
type googleOAuthCodeStore interface {
	Create(ctx context.Context, c *model.OAuthLoginCode) error
	FindActiveByCode(ctx context.Context, codeHash string) (*model.OAuthLoginCode, error)
	MarkUsed(ctx context.Context, id uuid.UUID) error
}

// googleOTPStore narrows repository.PhoneVerificationRepository — WhatsApp
// OTP challenges backing phone-ownership verification (migration 000013).
type googleOTPStore interface {
	Create(ctx context.Context, pv *model.PhoneVerification) error
	FindActive(ctx context.Context, handoffID uuid.UUID, phone string) (*model.PhoneVerification, error)
	FindLatest(ctx context.Context, handoffID uuid.UUID, phone string) (*model.PhoneVerification, error)
	CancelPendingForHandoff(ctx context.Context, handoffID uuid.UUID) error
	IncrementAttempts(ctx context.Context, id uuid.UUID) (int, error)
	MarkVerified(ctx context.Context, id uuid.UUID) error
	MarkConsumed(ctx context.Context, id uuid.UUID) error
}

// googleExchanger narrows oauth.GoogleClient — lets tests fake the Google
// HTTP round-trip without a real network call (the HTTP-level behavior of
// GoogleClient itself is covered by internal/auth/oauth/google_test.go).
type googleExchanger interface {
	AuthCodeURL(state string) string
	Exchange(ctx context.Context, code string) (*oauth.GoogleProfile, error)
}

// Compile-time assertions: concrete types satisfy the narrowed interfaces.
var (
	_ googleOAuthUserStore = (*repository.UserRepository)(nil)
	_ googleOAuthCodeStore = (*repository.OAuthLoginCodeRepository)(nil)
	_ googleOTPStore       = (*repository.PhoneVerificationRepository)(nil)
	_ googleExchanger      = (*oauth.GoogleClient)(nil)
)

// OTPConfig — policy for the WhatsApp OTP challenge gating Google OAuth
// registration (nomor WA bukan rahasia, jadi kepemilikan wajib dibuktikan
// sebelum akun dibuat/di-upgrade dari guest). Populated from
// config.OTPConfig at wiring time (router.go) — see config.go for the
// fail-fast validation.
type OTPConfig struct {
	TTL            time.Duration
	MaxAttempts    int
	ResendCooldown time.Duration
	CodeLength     int
}

// GoogleOAuthService implements the Google login/registration flow. Kept
// separate from Service (email+password) and GuestOrderService — different
// concern, avoids a god-service (§22).
type GoogleOAuthService struct {
	users     googleOAuthUserStore
	codes     googleOAuthCodeStore
	otps      googleOTPStore
	google    googleExchanger
	issuer    *token.Issuer
	otpCfg    OTPConfig
	otpSender notificationapi.OTPSender

	// frontendURL — kept for parity with how other services in this module
	// receive their outward-facing base URL (§2 "satu sumber APP_BASE_URL"),
	// even though today's flow builds every redirect URL in the handler
	// layer (it owns the http.ResponseWriter). Reserved for future service-
	// level use (e.g. building URLs embedded in notification payloads).
	frontendURL string
}

func NewGoogleOAuthService(
	users *repository.UserRepository,
	codes *repository.OAuthLoginCodeRepository,
	otps *repository.PhoneVerificationRepository,
	google *oauth.GoogleClient,
	issuer *token.Issuer,
	frontendURL string,
	otpCfg OTPConfig,
) *GoogleOAuthService {
	return &GoogleOAuthService{
		users: users, codes: codes, otps: otps, google: google,
		issuer: issuer, frontendURL: frontendURL, otpCfg: otpCfg,
	}
}

// SetOTPSender wires the notification module's OTP-sending capability.
// Setter-injected (not a constructor param) because notification.Service is
// built AFTER auth in router.go's composition root — same pattern as
// design.Service.SetNotifier.
func (s *GoogleOAuthService) SetOTPSender(sender notificationapi.OTPSender) {
	s.otpSender = sender
}

// GoogleExchangeOutput is the result of exchanging/completing a handoff code.
type GoogleExchangeOutput struct {
	Status string // "session" | "need_phone"

	// Populated when Status == "session".
	Token *TokenPair
	User  *model.User

	// Populated when Status == "need_phone".
	Code         string // the same handoff code, to be echoed back to /complete
	Email        string
	Name         string
	RedirectPath string
}

// CompleteGoogleInput — body of POST /auth/google/complete.
type CompleteGoogleInput struct {
	Code  string
	Phone string
	Name  string
	OTP   string
}

// RequestOTPInput — body of POST /auth/google/request-otp.
type RequestOTPInput struct {
	Code  string
	Phone string
	Name  string
}

// RequestOTPOutput — response contract for POST /auth/google/request-otp.
// Deliberately identical in shape no matter whether the phone belongs to
// nobody, a guest, a registered customer, or staff — the endpoint must not
// leak that information at this stage (anti-enumeration; the ONLY place
// phone-conflict is surfaced is Complete(), after the caller has actually
// proven ownership of the number via the code sent here).
type RequestOTPOutput struct {
	PhoneMasked       string
	ExpiresIn         int // seconds
	ResendAvailableIn int // seconds
}

// StartURL builds the Google consent-screen URL for the given anti-CSRF
// state nonce.
func (s *GoogleOAuthService) StartURL(state string) string {
	return s.google.AuthCodeURL(state)
}

// HandleCallback exchanges the authorization `code` for the caller's Google
// profile, resolves it against the local `users` table, and returns an
// opaque one-time handoff code the handler redirects the browser to the
// frontend with (see migration 000012 for why we don't hand back the JWT
// directly here).
func (s *GoogleOAuthService) HandleCallback(ctx context.Context, code, redirectPath string) (string, error) {
	profile, err := s.google.Exchange(ctx, code)
	if err != nil {
		return "", fmt.Errorf("google oauth callback: %w", err)
	}
	email := strings.ToLower(strings.TrimSpace(profile.Email))

	existing, err := s.users.FindByOAuth(ctx, "google", profile.Subject)
	switch {
	case err == nil:
		return s.handoffForExistingOAuthUser(ctx, existing, redirectPath)
	case !errors.Is(err, repository.ErrNotFound):
		return "", fmt.Errorf("google oauth callback: lookup by oauth subject: %w", err)
	}

	if profile.EmailVerified && email != "" {
		byEmail, err := s.users.FindByEmail(ctx, email)
		switch {
		case err == nil:
			return s.handoffForEmailMatch(ctx, byEmail, profile.Subject, redirectPath)
		case !errors.Is(err, repository.ErrNotFound):
			return "", fmt.Errorf("google oauth callback: lookup by email: %w", err)
		}
	}

	// No existing account matched at all — about to start a brand-new
	// registration. Customer accounts require a non-null, unique email
	// (CHECK users_email_required); an unverified Google email can't be
	// trusted to actually belong to the caller, so reject outright rather
	// than let it get persisted (review finding #6 — email squatting).
	if !profile.EmailVerified {
		return "", authapi.ErrOAuthEmailUnverified
	}

	return s.issueRegistrationHandoff(ctx, profile, email, redirectPath)
}

// handoffForExistingOAuthUser — subject already linked to a local user.
func (s *GoogleOAuthService) handoffForExistingOAuthUser(ctx context.Context, u *model.User, redirectPath string) (string, error) {
	if !u.IsActive {
		return "", authapi.ErrUserInactive
	}
	if u.UserType == model.UserTypeStaff {
		return "", authapi.ErrOAuthStaffNotAllowed
	}
	if err := s.users.UpdateLastLogin(ctx, u.ID); err != nil {
		return "", fmt.Errorf("google oauth callback: update last login: %w", err)
	}
	return s.issueSessionHandoff(ctx, u.ID, redirectPath)
}

// handoffForEmailMatch — no linked subject yet, but Google's verified email
// matches an existing local account. Staff must never authenticate this way
// (§10: staff wajib email+password). A registered customer whose Google
// subject differs from what's already linked is a conflict, not a silent
// re-link (defense against account takeover via a compromised/aliased email).
func (s *GoogleOAuthService) handoffForEmailMatch(ctx context.Context, u *model.User, subject, redirectPath string) (string, error) {
	if u.UserType == model.UserTypeStaff {
		return "", authapi.ErrOAuthStaffNotAllowed
	}
	if u.OAuthSubject != nil && *u.OAuthSubject != subject {
		return "", authapi.ErrOAuthAccountConflict
	}
	if !u.IsActive {
		return "", authapi.ErrUserInactive
	}
	if u.OAuthSubject == nil {
		if err := s.users.LinkOAuth(ctx, u.ID, "google", subject); err != nil {
			return "", fmt.Errorf("google oauth callback: link oauth: %w", err)
		}
	}
	if err := s.users.UpdateLastLogin(ctx, u.ID); err != nil {
		return "", fmt.Errorf("google oauth callback: update last login: %w", err)
	}
	return s.issueSessionHandoff(ctx, u.ID, redirectPath)
}

// issueRegistrationHandoff persists a 'registration' handoff code carrying
// the Google identity — the frontend will prompt for a phone number, drive
// the OTP request/verify round-trip, and call Complete().
func (s *GoogleOAuthService) issueRegistrationHandoff(ctx context.Context, profile *oauth.GoogleProfile, email, redirectPath string) (string, error) {
	if profile.Subject == "" || email == "" {
		// Should not happen — scope always includes email/sub — but the DB
		// CHECK constraint requires both non-null for kind='registration', so
		// fail loudly rather than let the insert bounce with an opaque error.
		return "", fmt.Errorf("google oauth callback: profile missing subject/email (subject=%q email=%q)", profile.Subject, email)
	}
	subject := profile.Subject
	rec := &model.OAuthLoginCode{
		Kind:         model.OAuthLoginCodeKindRegistration,
		Provider:     "google",
		Subject:      &subject,
		Email:        &email,
		RedirectPath: optionalStr(redirectPath),
		ExpiresAt:    time.Now().UTC().Add(googleRegistrationCodeTTL),
	}
	if name := strings.TrimSpace(profile.Name); name != "" {
		rec.Name = &name
	}
	raw, hash, err := generateToken()
	if err != nil {
		return "", fmt.Errorf("google oauth callback: generate handoff code: %w", err)
	}
	rec.CodeHash = hash
	if err := s.codes.Create(ctx, rec); err != nil {
		return "", fmt.Errorf("google oauth callback: create registration handoff: %w", err)
	}
	return raw, nil
}

// issueSessionHandoff persists a 'session' handoff code for an already-
// resolved user.
func (s *GoogleOAuthService) issueSessionHandoff(ctx context.Context, userID uuid.UUID, redirectPath string) (string, error) {
	raw, hash, err := generateToken()
	if err != nil {
		return "", fmt.Errorf("google oauth callback: generate handoff code: %w", err)
	}
	rec := &model.OAuthLoginCode{
		CodeHash:     hash,
		Kind:         model.OAuthLoginCodeKindSession,
		UserID:       &userID,
		Provider:     "google",
		RedirectPath: optionalStr(redirectPath),
		ExpiresAt:    time.Now().UTC().Add(googleSessionCodeTTL),
	}
	if err := s.codes.Create(ctx, rec); err != nil {
		return "", fmt.Errorf("google oauth callback: create session handoff: %w", err)
	}
	return raw, nil
}

// Exchange consumes a handoff code: 'session' codes are marked used
// immediately and turned into a real access token; 'registration' codes are
// deliberately LEFT UNUSED (the frontend still needs to submit it again to
// RequestOTP/Complete) and just echoed back with the Google identity data
// needed to render the "enter your phone number" step.
func (s *GoogleOAuthService) Exchange(ctx context.Context, handoffCode string) (*GoogleExchangeOutput, error) {
	rec, err := s.lookupActiveCode(ctx, handoffCode)
	if err != nil {
		return nil, err
	}
	switch rec.Kind {
	case model.OAuthLoginCodeKindSession:
		return s.exchangeSessionCode(ctx, rec)
	case model.OAuthLoginCodeKindRegistration:
		return &GoogleExchangeOutput{
			Status:       "need_phone",
			Code:         handoffCode,
			Email:        derefStr(rec.Email),
			Name:         derefStr(rec.Name),
			RedirectPath: derefStr(rec.RedirectPath),
		}, nil
	default:
		return nil, fmt.Errorf("google oauth exchange: unknown handoff code kind %q", rec.Kind)
	}
}

func (s *GoogleOAuthService) exchangeSessionCode(ctx context.Context, rec *model.OAuthLoginCode) (*GoogleExchangeOutput, error) {
	if rec.UserID == nil {
		return nil, fmt.Errorf("google oauth exchange: session handoff code %s missing user_id", rec.ID)
	}
	if err := s.claimHandoffCode(ctx, rec.ID); err != nil {
		return nil, err
	}
	u, err := s.users.FindByID(ctx, *rec.UserID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, authapi.ErrUserNotFound
		}
		return nil, fmt.Errorf("google oauth exchange: load user %s: %w", *rec.UserID, err)
	}
	pair, err := issueTokenFor(s.issuer, u)
	if err != nil {
		return nil, err
	}
	return &GoogleExchangeOutput{Status: "session", Token: pair, User: u, RedirectPath: derefStr(rec.RedirectPath)}, nil
}

// RequestOTP sends a WhatsApp OTP to prove ownership of the phone number
// supplied for a registration handoff (§ security review finding A — WA
// numbers aren't secret, so this MUST happen before any user row is
// created/upgraded). Response shape is deliberately identical no matter the
// phone's status (unclaimed / guest / registered / staff) — see
// RequestOTPOutput doc.
func (s *GoogleOAuthService) RequestOTP(ctx context.Context, in RequestOTPInput) (*RequestOTPOutput, error) {
	rec, err := s.lookupActiveCode(ctx, in.Code)
	if err != nil {
		return nil, err
	}
	if rec.Kind != model.OAuthLoginCodeKindRegistration {
		return nil, authapi.ErrOAuthCodeInvalid
	}

	normalizedPhone, err := phone.Normalize(in.Phone)
	if err != nil {
		return nil, fmt.Errorf("phone invalid: %w", err)
	}

	if err := s.checkResendCooldown(ctx, rec.ID, normalizedPhone); err != nil {
		return nil, err
	}

	// At most one active challenge per handoff at a time — cancel whatever
	// was pending (even under a different, possibly typo'd, phone number)
	// before issuing the new one.
	if err := s.otps.CancelPendingForHandoff(ctx, rec.ID); err != nil {
		return nil, fmt.Errorf("google oauth request otp: cancel previous challenge: %w", err)
	}

	code, err := generateOTPCode(s.otpCfg.CodeLength)
	if err != nil {
		return nil, fmt.Errorf("google oauth request otp: %w", err)
	}
	pv := &model.PhoneVerification{
		OAuthLoginCodeID: rec.ID,
		Phone:            normalizedPhone,
		CodeHash:         hashOTP(normalizedPhone, code),
		ExpiresAt:        time.Now().UTC().Add(s.otpCfg.TTL),
	}
	if err := s.otps.Create(ctx, pv); err != nil {
		return nil, fmt.Errorf("google oauth request otp: create challenge: %w", err)
	}

	if err := s.sendOTP(ctx, normalizedPhone, code, pv.ID); err != nil {
		return nil, err
	}

	return &RequestOTPOutput{
		PhoneMasked:       phone.Mask(normalizedPhone),
		ExpiresIn:         int(s.otpCfg.TTL.Seconds()),
		ResendAvailableIn: int(s.otpCfg.ResendCooldown.Seconds()),
	}, nil
}

// sendOTP renders the fixed WA message template and enqueues it — best-
// effort via the notification module's job queue (§13: never send WA
// synchronously in the request path).
func (s *GoogleOAuthService) sendOTP(ctx context.Context, normalizedPhone, code string, challengeID uuid.UUID) error {
	if s.otpSender == nil {
		return fmt.Errorf("google oauth request otp: notification sender not configured")
	}
	minutes := int(s.otpCfg.TTL.Minutes())
	if minutes < 1 {
		minutes = 1
	}
	message := fmt.Sprintf(
		"Kode verifikasi Rajaku Printing: %s. Berlaku %d menit. Jangan bagikan kode ini kepada siapa pun, termasuk yang mengaku staf kami.",
		code, minutes,
	)
	dedup := "otp:" + challengeID.String()
	if err := s.otpSender.EnqueueOTP(ctx, normalizedPhone, message, dedup); err != nil {
		return fmt.Errorf("google oauth request otp: enqueue notification: %w", err)
	}
	return nil
}

// checkResendCooldown enforces OTP_RESEND_COOLDOWN for (handoffID, phone).
func (s *GoogleOAuthService) checkResendCooldown(ctx context.Context, handoffID uuid.UUID, normalizedPhone string) error {
	latest, err := s.otps.FindLatest(ctx, handoffID, normalizedPhone)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("google oauth request otp: check resend cooldown: %w", err)
	}
	elapsed := time.Since(latest.CreatedAt)
	if elapsed >= s.otpCfg.ResendCooldown {
		return nil
	}
	remaining := s.otpCfg.ResendCooldown - elapsed
	// Round UP to the next whole second so the client never polls a moment
	// too early and gets rejected again.
	secs := int(remaining.Seconds())
	if remaining%time.Second != 0 {
		secs++
	}
	return &authapi.OTPCooldownError{ResendAvailableIn: secs}
}

// Complete finishes the registration flow: verifies the OTP sent by
// RequestOTP, claims the handoff code, resolves the phone number against
// `users` (create new, or upgrade a matching guest — §11: 1 nomor WA = 1
// identitas lintas channel), links the Google identity, and issues a real
// session.
func (s *GoogleOAuthService) Complete(ctx context.Context, in CompleteGoogleInput) (*GoogleExchangeOutput, error) {
	rec, err := s.lookupActiveCode(ctx, in.Code)
	if err != nil {
		return nil, err
	}
	if rec.Kind != model.OAuthLoginCodeKindRegistration {
		return nil, authapi.ErrOAuthCodeInvalid
	}
	if rec.Subject == nil || rec.Email == nil {
		return nil, fmt.Errorf("google oauth complete: registration handoff code %s missing subject/email", rec.ID)
	}

	normalizedPhone, err := phone.Normalize(in.Phone)
	if err != nil {
		return nil, fmt.Errorf("phone invalid: %w", err)
	}

	pv, err := s.verifyOTP(ctx, rec.ID, normalizedPhone, in.OTP)
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = strings.TrimSpace(derefStr(rec.Name))
	}
	if name == "" {
		return nil, fmt.Errorf("name required: %w", errBadInput)
	}

	// Claim the handoff code BEFORE mutating the user (review finding #4) —
	// atomic single-use, so two concurrent Complete() calls for the same
	// code can never both create/upgrade a user.
	if err := s.claimHandoffCode(ctx, rec.ID); err != nil {
		return nil, err
	}

	u, err := s.resolveUserForCompletion(ctx, normalizedPhone, name, *rec.Subject, *rec.Email)
	if err != nil {
		return nil, err
	}

	if err := s.otps.MarkConsumed(ctx, pv.ID); err != nil {
		return nil, fmt.Errorf("google oauth complete: mark otp consumed: %w", err)
	}
	if err := s.users.UpdateLastLogin(ctx, u.ID); err != nil {
		return nil, fmt.Errorf("google oauth complete: update last login: %w", err)
	}
	pair, err := issueTokenFor(s.issuer, u)
	if err != nil {
		return nil, err
	}
	return &GoogleExchangeOutput{Status: "session", Token: pair, User: u, RedirectPath: derefStr(rec.RedirectPath)}, nil
}

// verifyOTP looks up the active challenge for (handoffID, phone), enforces
// the attempt ceiling, and constant-time-compares the hash. A wrong guess
// atomically increments attempts before returning — this is the ONLY place
// attempts is incremented, so the counter can't be bypassed by racing
// requests against a stale read.
func (s *GoogleOAuthService) verifyOTP(ctx context.Context, handoffID uuid.UUID, normalizedPhone, otp string) (*model.PhoneVerification, error) {
	pv, err := s.otps.FindActive(ctx, handoffID, normalizedPhone)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, authapi.ErrOTPExpired
		}
		return nil, fmt.Errorf("google oauth complete: lookup otp challenge: %w", err)
	}
	if pv.Attempts >= s.otpCfg.MaxAttempts {
		return nil, authapi.ErrOTPTooManyAttempts
	}

	want := []byte(pv.CodeHash)
	got := []byte(hashOTP(normalizedPhone, otp))
	if subtle.ConstantTimeCompare(got, want) != 1 {
		attempts, err := s.otps.IncrementAttempts(ctx, pv.ID)
		if err != nil {
			return nil, fmt.Errorf("google oauth complete: increment otp attempts: %w", err)
		}
		left := s.otpCfg.MaxAttempts - attempts
		if left < 0 {
			left = 0
		}
		return nil, &authapi.OTPInvalidError{AttemptsLeft: left}
	}

	if err := s.otps.MarkVerified(ctx, pv.ID); err != nil {
		return nil, fmt.Errorf("google oauth complete: mark otp verified: %w", err)
	}
	return pv, nil
}

// claimHandoffCode wraps codes.MarkUsed, mapping "not found / already used"
// to the single ErrOAuthCodeInvalid sentinel the handler already knows how
// to map to a 400 (review finding #4 — MarkUsed is now atomic).
func (s *GoogleOAuthService) claimHandoffCode(ctx context.Context, id uuid.UUID) error {
	if err := s.codes.MarkUsed(ctx, id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return authapi.ErrOAuthCodeInvalid
		}
		return fmt.Errorf("google oauth: mark handoff code used: %w", err)
	}
	return nil
}

// resolveUserForCompletion — see spec: no user with this phone yet → create
// a brand-new registered customer; phone belongs to staff or an already-
// registered customer → reject (phone is the unique matching key, can't be
// claimed twice); phone belongs to a guest customer → upgrade in place so
// existing order history stays attached to the same user_id (§11).
func (s *GoogleOAuthService) resolveUserForCompletion(ctx context.Context, normalizedPhone, name, subject, email string) (*model.User, error) {
	existing, err := s.users.FindByPhone(ctx, normalizedPhone)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return s.createRegisteredCustomer(ctx, normalizedPhone, name, subject, email)
		}
		return nil, fmt.Errorf("google oauth complete: lookup phone: %w", err)
	}

	isRegisteredCustomer := existing.CustomerType != nil && *existing.CustomerType == model.CustomerTypeRegistered
	if existing.UserType == model.UserTypeStaff || isRegisteredCustomer {
		return nil, authapi.ErrPhoneAlreadyUsed
	}

	return s.upgradeGuestCustomer(ctx, existing, name, subject, email)
}

func (s *GoogleOAuthService) createRegisteredCustomer(ctx context.Context, normalizedPhone, name, subject, email string) (*model.User, error) {
	if used, err := s.users.ExistsByEmail(ctx, email); err != nil {
		return nil, fmt.Errorf("google oauth complete: check email uniqueness: %w", err)
	} else if used {
		return nil, authapi.ErrEmailAlreadyUsed
	}

	registered := model.CustomerTypeRegistered
	provider := "google"
	u := &model.User{
		Email:         &email,
		Phone:         normalizedPhone,
		Name:          name,
		UserType:      model.UserTypeCustomer,
		CustomerType:  &registered,
		OAuthProvider: &provider,
		OAuthSubject:  &subject,
		IsActive:      true,
	}
	if err := s.users.Create(ctx, u); err != nil {
		return nil, fmt.Errorf("google oauth complete: create user: %w", err)
	}
	return u, nil
}

// upgradeGuestCustomer promotes a matching guest to customer_type=registered
// with a linked Google identity.
//
//   - Staff-only accounts and IsActive checks mirror the other resolution
//     branches in this file (review finding #7 — this branch used to skip
//     the IsActive check entirely).
//   - A guest always has email IS NULL. Registered customers require
//     email IS NOT NULL (CHECK users_email_required). If the Google email is
//     already claimed by a DIFFERENT user we CANNOT silently upgrade with
//     email left NULL (that violates the CHECK and used to bounce as an
//     opaque 500 — review finding #2) nor steal the other account's email —
//     reject explicitly instead.
func (s *GoogleOAuthService) upgradeGuestCustomer(ctx context.Context, existing *model.User, name, subject, email string) (*model.User, error) {
	if !existing.IsActive {
		return nil, authapi.ErrUserInactive
	}

	var emailToSet *string
	if existing.Email == nil {
		used, err := s.users.ExistsByEmail(ctx, email)
		if err != nil {
			return nil, fmt.Errorf("google oauth complete: check email uniqueness: %w", err)
		}
		if used {
			return nil, authapi.ErrEmailAlreadyUsed
		}
		emailToSet = &email
	}

	if err := s.users.UpgradeGuestToRegistered(ctx, existing.ID, emailToSet, name, "google", subject); err != nil {
		return nil, fmt.Errorf("google oauth complete: upgrade guest to registered: %w", err)
	}
	updated, err := s.users.FindByID(ctx, existing.ID)
	if err != nil {
		return nil, fmt.Errorf("google oauth complete: reload upgraded user %s: %w", existing.ID, err)
	}
	return updated, nil
}

// lookupActiveCode hashes `raw` and looks it up — any miss (not found, used,
// expired) maps to the single ErrOAuthCodeInvalid sentinel.
func (s *GoogleOAuthService) lookupActiveCode(ctx context.Context, raw string) (*model.OAuthLoginCode, error) {
	rec, err := s.codes.FindActiveByCode(ctx, hashToken(raw))
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, authapi.ErrOAuthCodeInvalid
		}
		return nil, fmt.Errorf("lookup oauth handoff code: %w", err)
	}
	return rec, nil
}

// generateOTPCode returns a `length`-digit numeric code using crypto/rand,
// one uniformly-distributed digit at a time (rand.Int with a base-10 bound)
// — deliberately NOT math/rand (predictable) and NOT a single rand.Int
// modulo 10^length (would introduce modulo bias for non-power-of-10 bounds
// on some digit positions when reduced digit-by-digit from a larger value).
func generateOTPCode(length int) (string, error) {
	ten := big.NewInt(10)
	digits := make([]byte, length)
	for i := range digits {
		n, err := rand.Int(rand.Reader, ten)
		if err != nil {
			return "", fmt.Errorf("generate otp digit: %w", err)
		}
		digits[i] = '0' + byte(n.Int64())
	}
	return string(digits), nil
}

// hashOTP mirrors the migration 000013 comment: sha256 hex of
// "<phone>:<code>" (phone included so the same numeric code sent to two
// different numbers never hashes to the same value). Reuses hashToken
// (invite_service.go) — same sha256-hex primitive, just fed a composite
// string instead of a single random token.
func hashOTP(normalizedPhone, code string) string {
	return hashToken(normalizedPhone + ":" + code)
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func optionalStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
