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
	"gorm.io/gorm"

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
	// ExistsByPhone — used to decide whether a phone number needs an OTP
	// round-trip at all (§ business rule: OTP hanya untuk tabrakan identitas,
	// bukan setiap registrasi/klaim nomor — lihat RequestOTP/Complete).
	ExistsByPhone(ctx context.Context, phone string) (bool, error)
	LinkOAuth(ctx context.Context, id uuid.UUID, provider, subject string) error
	UpgradeGuestToRegistered(ctx context.Context, id uuid.UUID, email *string, name, provider, subject string) error
	// SetPhone — used by PhoneClaimService (authenticated users adding/
	// changing their own number, and releasing a phone from an absorbed
	// guest row). Not used by GoogleOAuthService itself, but living on the
	// same shared interface avoids a second near-identical user-store
	// narrowing in phone_claim_service.go (§22 least duplication).
	SetPhone(ctx context.Context, id uuid.UUID, phone *string, verifiedAt *time.Time) error
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
	// FindActiveByUser / FindLatestByUser / CancelPendingForUser — user_id-
	// keyed counterparts of the three handoff-keyed methods above (migration
	// 000017), used by PhoneClaimService (authenticated users proving
	// ownership of their own phone, not a Google registration handoff).
	// Living on this shared interface (rather than a second near-identical
	// one) lets PhoneClaimService reuse phoneLockTxRunner as-is.
	FindActiveByUser(ctx context.Context, userID uuid.UUID, phone string) (*model.PhoneVerification, error)
	FindLatestByUser(ctx context.Context, userID uuid.UUID, phone string) (*model.PhoneVerification, error)
	CancelPendingForUser(ctx context.Context, userID uuid.UUID) error
	IncrementAttemptsIfAllowed(ctx context.Context, id uuid.UUID, maxAttempts int) (int, error)
	MarkVerified(ctx context.Context, id uuid.UUID) error
	MarkConsumed(ctx context.Context, id uuid.UUID) error
	// CountAndOldestSince / SumAttemptsSince — cross-handoff, per-phone rate
	// limiting (review finding #2). See repository.PhoneVerificationRepository
	// for the exact semantics.
	CountAndOldestSince(ctx context.Context, phone string, since time.Time) (int64, *time.Time, error)
	SumAttemptsSince(ctx context.Context, phone string, since time.Time) (int64, error)
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
	// MaxPerPhoneHour / MaxFailedPerPhoneHour — cross-handoff, per-phone rate
	// limits (review finding #2). See config.OTPConfig for the full doc.
	MaxPerPhoneHour       int
	MaxFailedPerPhoneHour int
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
	// txRunner — opens the single DB transaction Complete() needs across
	// users/codes/otps (review finding #4). See google_oauth_tx.go.
	txRunner googleOAuthTxRunner
	// phoneLockRunner — opens a per-phone-number-scoped transaction (Postgres
	// advisory lock) around the check-then-mutate sequences that gate OTP
	// issuance and OTP verify attempts (review finding #1). See
	// google_oauth_tx.go.
	phoneLockRunner phoneLockTxRunner
}

func NewGoogleOAuthService(
	users *repository.UserRepository,
	codes *repository.OAuthLoginCodeRepository,
	otps *repository.PhoneVerificationRepository,
	google *oauth.GoogleClient,
	issuer *token.Issuer,
	otpCfg OTPConfig,
	db *gorm.DB,
) *GoogleOAuthService {
	return &GoogleOAuthService{
		users: users, codes: codes, otps: otps, google: google,
		issuer: issuer, otpCfg: otpCfg,
		txRunner:        newGormTxRunner(db),
		phoneLockRunner: newGormPhoneLockTxRunner(db),
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

// RequestOTPInput — body of POST /auth/google/request-otp. No Name field
// (review finding #10) — the display name is only ever needed/used by
// Complete(), which already has its own fallback to the Google profile name
// carried on the handoff code; accepting-but-ignoring it here a second time
// was dead input with no documented purpose.
type RequestOTPInput struct {
	Code  string
	Phone string
}

// RequestOTPOutput — response contract for POST /auth/google/request-otp.
//
// NOT anti-enumeration anymore (owner decision, superseding this type's
// original doc): OTPRequired now DELIBERATELY reveals whether `phone` is
// already claimed by an existing `users` row (guest, registered, or staff) —
// false means "free, no OTP needed, proceed straight to Complete()"; true
// means "taken, prove ownership via the code just sent before Complete()
// will accept it". This trade-off is intentional — sending a WhatsApp OTP to
// every phone number typed into a registration form (even ones nobody has
// ever claimed) burns Baileys' rate budget for no security benefit, since an
// unclaimed number can't be used to hijack anyone's history anyway (§13:
// Baileys is unofficial WhatsApp Web automation — high volume to strangers
// risks the sending number getting banned). The endpoint is rate-limited on
// its own strict bucket (RATE_LIMIT_OTP_*, see googleOTPLimited in
// router.go) as the mitigation for the resulting enumeration surface.
type RequestOTPOutput struct {
	OTPRequired bool
	// PhoneMasked/ExpiresIn/ResendAfterSeconds are only meaningful when
	// OTPRequired is true (an OTP was actually issued and sent).
	PhoneMasked        string
	ExpiresIn          int // seconds
	ResendAfterSeconds int // seconds
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
// claimed AND ROTATED (review finding #8, see exchangeRegistrationCode) — a
// brand-new code carrying the same Google identity is returned, which the
// frontend must use for the subsequent RequestOTP/Complete calls to render
// the "enter your phone number" step.
func (s *GoogleOAuthService) Exchange(ctx context.Context, handoffCode string) (*GoogleExchangeOutput, error) {
	rec, err := s.lookupActiveCode(ctx, handoffCode)
	if err != nil {
		return nil, err
	}
	switch rec.Kind {
	case model.OAuthLoginCodeKindSession:
		return s.exchangeSessionCode(ctx, rec)
	case model.OAuthLoginCodeKindRegistration:
		return s.exchangeRegistrationCode(ctx, rec)
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
	// Re-check IsActive at consumption time (review finding #9) — an admin
	// could have deactivated the account in the up-to-2-minute window between
	// HandleCallback issuing this handoff and the frontend calling Exchange.
	// Every other resolution branch in this file already checks this; this
	// one didn't.
	if !u.IsActive {
		return nil, authapi.ErrUserInactive
	}
	pair, err := issueTokenFor(s.issuer, u)
	if err != nil {
		return nil, err
	}
	return &GoogleExchangeOutput{Status: "session", Token: pair, User: u, RedirectPath: derefStr(rec.RedirectPath)}, nil
}

// exchangeRegistrationCode rotates the registration handoff (review finding
// #8): the raw code arrives in the browser's URL query string and survives
// in history/Referer headers for the code's entire 15-minute TTL, even
// though the code itself was previously left un-consumed here on purpose (so
// the SPA could submit it again to RequestOTP/Complete). Immediately
// claiming the OLD code and minting a NEW one — same Google identity, fresh
// TTL — means anything scraped from browser history/Referer stops working
// the instant the legitimate SPA calls this endpoint (normally within a
// second of landing on the page), instead of remaining valid for up to 15
// minutes.
// exchangeRegistrationCode claims the OLD code and mints the NEW one in a
// SINGLE transaction (review finding #3). Before this fix, the claim
// (codes.MarkUsed) committed on its own; if generateToken(), codes.Create(),
// or delivering the response to the caller failed/was lost AFTER that point,
// the old code was permanently burned with no replacement anyone could use —
// forcing the user to redo the entire Google consent screen, exactly what
// this rotation was meant to avoid needing. Both operations only ever touch
// oauth_login_codes, so the transaction boundary is clean — reuses the same
// googleOAuthTxRunner Complete() uses, just with only tx.Codes exercised.
func (s *GoogleOAuthService) exchangeRegistrationCode(ctx context.Context, rec *model.OAuthLoginCode) (*GoogleExchangeOutput, error) {
	if rec.Subject == nil || rec.Email == nil {
		return nil, fmt.Errorf("google oauth exchange: registration handoff code %s missing subject/email", rec.ID)
	}
	var newCode string
	err := s.txRunner.RunInTx(ctx, func(tx googleOAuthCompletionTx) error {
		if err := claimHandoffCodeWith(ctx, tx.Codes, rec.ID); err != nil {
			return err
		}
		reissued, err := reissueRegistrationHandoffWith(ctx, tx.Codes, rec)
		if err != nil {
			return err
		}
		newCode = reissued
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &GoogleExchangeOutput{
		Status:       "need_phone",
		Code:         newCode,
		Email:        derefStr(rec.Email),
		Name:         derefStr(rec.Name),
		RedirectPath: derefStr(rec.RedirectPath),
	}, nil
}

// reissueRegistrationHandoffWith mints a brand-new registration handoff code
// carrying the SAME Google identity as `rec` — used by exchangeRegistrationCode
// to rotate the code exposed in the browser URL. Store-parameterized (like
// claimHandoffCodeWith) so it can run against the transaction-scoped codes
// store (review finding #3).
func reissueRegistrationHandoffWith(ctx context.Context, codes googleOAuthCodeStore, rec *model.OAuthLoginCode) (string, error) {
	newRec := &model.OAuthLoginCode{
		Kind:         model.OAuthLoginCodeKindRegistration,
		Provider:     rec.Provider,
		Subject:      rec.Subject,
		Email:        rec.Email,
		Name:         rec.Name,
		RedirectPath: rec.RedirectPath,
		ExpiresAt:    time.Now().UTC().Add(googleRegistrationCodeTTL),
	}
	raw, hash, err := generateToken()
	if err != nil {
		return "", fmt.Errorf("google oauth exchange: generate rotated handoff code: %w", err)
	}
	newRec.CodeHash = hash
	if err := codes.Create(ctx, newRec); err != nil {
		return "", fmt.Errorf("google oauth exchange: create rotated registration handoff: %w", err)
	}
	return raw, nil
}

// RequestOTP sends a WhatsApp OTP to prove ownership of the phone number
// supplied for a registration handoff — but ONLY when that number is already
// claimed by an existing `users` row (§ business rule: OTP hanya diterbitkan
// saat terjadi tabrakan identitas, bukan untuk setiap registrasi — lihat
// RequestOTPOutput doc for why). An unclaimed number short-circuits with
// OTPRequired=false and sends nothing at all.
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

	taken, err := s.users.ExistsByPhone(ctx, normalizedPhone)
	if err != nil {
		return nil, fmt.Errorf("google oauth request otp: check phone existence: %w", err)
	}
	if !taken {
		return &RequestOTPOutput{OTPRequired: false}, nil
	}

	code, err := generateOTPCode(s.otpCfg.CodeLength)
	if err != nil {
		return nil, fmt.Errorf("google oauth request otp: %w", err)
	}

	// Everything that reads-then-writes per-phone state runs inside ONE
	// transaction, opened under a Postgres advisory lock scoped to
	// normalizedPhone (review finding #1 — TOCTOU: without the lock, N
	// concurrent requests for the same number could all read a low
	// count/cooldown and all pass, letting the per-phone cap be bypassed
	// entirely under enough parallelism). checkResendCooldown rides along
	// here too — it has the exact same read-then-write shape and there's no
	// reason to leave it unprotected once the transaction boundary exists.
	var pv *model.PhoneVerification
	err = s.phoneLockRunner.RunInTx(ctx, normalizedPhone, func(tx googleOTPStore) error {
		if err := checkPerPhoneIssueCapWith(ctx, tx, normalizedPhone, s.otpCfg.MaxPerPhoneHour); err != nil {
			return err
		}
		if err := checkResendCooldownWith(ctx, tx, rec.ID, normalizedPhone, s.otpCfg.ResendCooldown); err != nil {
			return err
		}
		// At most one active challenge per handoff at a time — cancel
		// whatever was pending (even under a different, possibly typo'd,
		// phone number) before issuing the new one.
		if err := tx.CancelPendingForHandoff(ctx, rec.ID); err != nil {
			return fmt.Errorf("google oauth request otp: cancel previous challenge: %w", err)
		}
		newPV := &model.PhoneVerification{
			OAuthLoginCodeID: &rec.ID,
			Phone:            normalizedPhone,
			CodeHash:         hashOTP(normalizedPhone, code),
			ExpiresAt:        time.Now().UTC().Add(s.otpCfg.TTL),
		}
		if err := tx.Create(ctx, newPV); err != nil {
			return fmt.Errorf("google oauth request otp: create challenge: %w", err)
		}
		pv = newPV
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Enqueuing the WA send is deliberately OUTSIDE the transaction above —
	// it writes to a different module's table (notification_jobs) and best
	// left out of the phone-lock tx's blast radius/duration.
	if err := s.sendOTP(ctx, normalizedPhone, code, pv.ID); err != nil {
		return nil, err
	}

	return &RequestOTPOutput{
		OTPRequired:        true,
		PhoneMasked:        phone.Mask(normalizedPhone),
		ExpiresIn:          int(s.otpCfg.TTL.Seconds()),
		ResendAfterSeconds: int(s.otpCfg.ResendCooldown.Seconds()),
	}, nil
}

// sendOTP renders the fixed WA message template and enqueues it — best-
// effort via the notification module's job queue (§13: never send WA
// synchronously in the request path). Thin wrapper over sendPhoneOTP (shared
// with PhoneClaimService — §22 least duplication).
func (s *GoogleOAuthService) sendOTP(ctx context.Context, normalizedPhone, code string, challengeID uuid.UUID) error {
	return sendPhoneOTP(ctx, s.otpSender, s.otpCfg, normalizedPhone, code, challengeID)
}

// sendPhoneOTP renders the fixed WA message template and enqueues it — best-
// effort via the notification module's job queue (§13: never send WA
// synchronously in the request path). Free function so both
// GoogleOAuthService (registration) and PhoneClaimService (authenticated
// users adding/changing their own number) share the exact same message
// template instead of drifting apart.
func sendPhoneOTP(ctx context.Context, sender notificationapi.OTPSender, cfg OTPConfig, normalizedPhone, code string, challengeID uuid.UUID) error {
	if sender == nil {
		return fmt.Errorf("send phone otp: notification sender not configured")
	}
	minutes := int(cfg.TTL.Minutes())
	if minutes < 1 {
		minutes = 1
	}
	message := fmt.Sprintf(
		"Kode verifikasi Rajaku Printing: %s. Berlaku %d menit. Jangan bagikan kode ini kepada siapa pun, termasuk yang mengaku staf kami.",
		code, minutes,
	)
	dedup := "otp:" + challengeID.String()
	if err := sender.EnqueueOTP(ctx, normalizedPhone, message, dedup); err != nil {
		return fmt.Errorf("send phone otp: enqueue notification: %w", err)
	}
	return nil
}

// checkResendCooldownWith enforces OTP_RESEND_COOLDOWN for (handoffID,
// phone). Store-parameterized (review finding #1) so RequestOTP can run it
// against the phone-lock-scoped transaction store instead of s.otps directly
// — it has the exact same "read latest, then decide whether to write" shape
// as the two per-phone cap checks below, so it rides along inside the same
// advisory-lock transaction rather than being left as a separate, still-racy
// call.
func checkResendCooldownWith(ctx context.Context, otps googleOTPStore, handoffID uuid.UUID, normalizedPhone string, cooldown time.Duration) error {
	latest, err := otps.FindLatest(ctx, handoffID, normalizedPhone)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("google oauth request otp: check resend cooldown: %w", err)
	}
	return cooldownErrorFromLatest(latest.CreatedAt, cooldown)
}

// checkResendCooldownForUserWith is checkResendCooldownWith's user_id-keyed
// counterpart (PhoneClaimService — an authenticated user proving ownership of
// their own phone, not a Google registration handoff).
func checkResendCooldownForUserWith(ctx context.Context, otps googleOTPStore, userID uuid.UUID, normalizedPhone string, cooldown time.Duration) error {
	latest, err := otps.FindLatestByUser(ctx, userID, normalizedPhone)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("phone claim request otp: check resend cooldown: %w", err)
	}
	return cooldownErrorFromLatest(latest.CreatedAt, cooldown)
}

// cooldownErrorFromLatest is the shared "how much longer must the caller
// wait" computation behind both checkResendCooldownWith and
// checkResendCooldownForUserWith.
func cooldownErrorFromLatest(latestCreatedAt time.Time, cooldown time.Duration) error {
	elapsed := time.Since(latestCreatedAt)
	if elapsed >= cooldown {
		return nil
	}
	remaining := cooldown - elapsed
	// Round UP to the next whole second so the client never polls a moment
	// too early and gets rejected again.
	secs := int(remaining.Seconds())
	if remaining%time.Second != 0 {
		secs++
	}
	return &authapi.OTPCooldownError{ResendAvailableIn: secs}
}

// checkPerPhoneIssueCapWith enforces OTP_MAX_PER_PHONE_HOUR — how many OTP
// challenges may be ISSUED to one WhatsApp number per hour, across every
// registration handoff (review finding #2). Distinct from
// checkResendCooldownWith above, which only throttles resends WITHIN a
// single handoff — that alone does nothing to stop an attacker from opening
// a new handoff per request to reset the cooldown clock.
//
// Store-parameterized (review finding #1): RequestOTP calls this against the
// phone-lock-scoped transaction store so the COUNT this reads and the
// otps.Create() that follows it (if the cap isn't hit) can never be split by
// a concurrent request reading the same stale count.
func checkPerPhoneIssueCapWith(ctx context.Context, otps googleOTPStore, normalizedPhone string, maxPerPhoneHour int) error {
	since := time.Now().UTC().Add(-time.Hour)
	count, oldest, err := otps.CountAndOldestSince(ctx, normalizedPhone, since)
	if err != nil {
		return fmt.Errorf("google oauth request otp: check per-phone issue cap: %w", err)
	}
	if count < int64(maxPerPhoneHour) {
		return nil
	}
	// The cap clears exactly one hour after the oldest challenge in the
	// window ages out — report that as the resend hint.
	var resendIn int
	if oldest != nil {
		remaining := time.Hour - time.Since(*oldest)
		if remaining < 0 {
			remaining = 0
		}
		resendIn = int(remaining.Seconds())
		if remaining%time.Second != 0 {
			resendIn++
		}
	}
	return &authapi.OTPCooldownError{ResendAvailableIn: resendIn}
}

// checkPerPhoneFailedCapWith enforces OTP_MAX_FAILED_PER_PHONE_HOUR — the
// total number of verify attempts logged against one WhatsApp number per
// hour, across every registration handoff (review finding #2). Without this,
// an attacker could open several handoffs for the same victim phone, each
// carrying its own full OTP_MAX_ATTEMPTS budget, and keep guessing the code
// far past what any single handoff's ceiling allows.
//
// Store-parameterized (review finding #1): verifyOTP calls this against the
// phone-lock-scoped transaction store so the SUM this reads and the
// IncrementAttemptsIfAllowed() that follows it are atomic with respect to
// other concurrent verify attempts against the same phone.
func checkPerPhoneFailedCapWith(ctx context.Context, otps googleOTPStore, normalizedPhone string, maxFailedPerPhoneHour int) error {
	since := time.Now().UTC().Add(-time.Hour)
	total, err := otps.SumAttemptsSince(ctx, normalizedPhone, since)
	if err != nil {
		return fmt.Errorf("google oauth complete: check per-phone attempt cap: %w", err)
	}
	if total >= int64(maxFailedPerPhoneHour) {
		return authapi.ErrOTPTooManyAttempts
	}
	return nil
}

// Complete finishes the registration flow: resolves the (now optional) phone
// number against `users` and issues a real session. Phone handling (§
// business rule — OTP hanya untuk tabrakan identitas):
//   - Phone blank/absent → account created with no phone at all.
//   - Phone provided, currently unclaimed → account created with the number
//     attached but UNVERIFIED (phone_verified_at stays NULL) — no OTP needed.
//   - Phone provided, already claimed by an existing `users` row → an `otp`
//     MUST be supplied and verified (authapi.ErrPhoneVerificationRequired
//     otherwise). Once verified: a GUEST owner is upgraded in place (§11: 1
//     nomor WA = 1 identitas, riwayat gabung); a STAFF or already-REGISTERED
//     owner is never merged into — OTP proves WA ownership, not that
//     account's password (authapi.ErrPhoneAlreadyUsed, a dead end requiring a
//     different number).
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

	var normalizedPhone *string
	if trimmed := strings.TrimSpace(in.Phone); trimmed != "" {
		p, err := phone.Normalize(trimmed)
		if err != nil {
			return nil, fmt.Errorf("phone invalid: %w", err)
		}
		normalizedPhone = &p
	}

	// Resolve + validate the display name BEFORE touching the OTP challenge.
	// This can't move all the way out to the HTTP handler (§23 edge
	// validation) — the fallback source, rec.Name (the name Google supplied
	// at /exchange time), is only known once the handoff code has been
	// looked up above — but doing it here, before verifyOTP's DB round-trip,
	// means a request that was always going to be rejected for a missing
	// name doesn't also burn an OTP attempt on the way to that rejection.
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = strings.TrimSpace(derefStr(rec.Name))
	}
	if name == "" {
		return nil, fmt.Errorf("name required: %w", errBadInput)
	}

	// pv stays nil unless the phone is both provided AND already claimed by
	// someone — that's the ONLY case an OTP round-trip is required. verifyOTP
	// (when it runs) is deliberately OUTSIDE the transaction below: its
	// attempts counter must stay incremented even if everything after it
	// rolls back (that's the entire point of counting attempts — see
	// verifyOTP's doc).
	var pv *model.PhoneVerification
	if normalizedPhone != nil {
		taken, err := s.users.ExistsByPhone(ctx, *normalizedPhone)
		if err != nil {
			return nil, fmt.Errorf("google oauth complete: check phone existence: %w", err)
		}
		if taken {
			if strings.TrimSpace(in.OTP) == "" {
				return nil, authapi.ErrPhoneVerificationRequired
			}
			pv, err = s.verifyOTP(ctx, rec.ID, *normalizedPhone, in.OTP)
			if err != nil {
				return nil, err
			}
		}
	}

	// claim handoff + resolve user (create/upgrade) + consume OTP + update
	// last login all commit or all roll back TOGETHER (review finding #4).
	// Before this fix, a rejected resolve (ErrPhoneAlreadyUsed /
	// ErrEmailAlreadyUsed / ErrUserInactive) still left the handoff code
	// permanently burned — the caller had no way to retry with a corrected
	// phone number without redoing the entire Google consent screen. Worse,
	// if Create() succeeded but a later step failed, the user row stuck
	// around with a dead handoff and no token ever returned.
	var u *model.User
	err = s.txRunner.RunInTx(ctx, func(tx googleOAuthCompletionTx) error {
		if err := claimHandoffCodeWith(ctx, tx.Codes, rec.ID); err != nil {
			return err
		}
		var resolveErr error
		u, resolveErr = resolveUserForCompletion(ctx, tx.Users, normalizedPhone, pv != nil, name, *rec.Subject, *rec.Email)
		if resolveErr != nil {
			return resolveErr
		}
		if pv != nil {
			if err := tx.OTPs.MarkConsumed(ctx, pv.ID); err != nil {
				return fmt.Errorf("google oauth complete: mark otp consumed: %w", err)
			}
		}
		if err := tx.Users.UpdateLastLogin(ctx, u.ID); err != nil {
			return fmt.Errorf("google oauth complete: update last login: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	pair, err := issueTokenFor(s.issuer, u)
	if err != nil {
		return nil, err
	}
	return &GoogleExchangeOutput{Status: "session", Token: pair, User: u, RedirectPath: derefStr(rec.RedirectPath)}, nil
}

// verifyOTP looks up the active challenge for (handoffID, phone) and
// constant-time-compares the hash. The per-phone failed-attempt cap check and
// the attempts-ceiling increment run together inside ONE transaction, opened
// under a Postgres advisory lock scoped to normalizedPhone (review finding
// #1 — repository.PhoneVerificationRepository.LockPhone /
// IncrementAttemptsIfAllowed), so no window exists between reading a stale
// SUM/attempts value and deciding whether a guess is allowed — for THIS
// specific challenge row AND across every other handoff open for the same
// phone number. Every call reaching the increment counts as an attempt,
// whether the code turns out right or wrong — increment happens BEFORE the
// hash comparison, unconditionally, not just on the wrong-guess branch.
//
// This transaction is DELIBERATELY SEPARATE from Complete()'s claim+resolve+
// consume transaction (googleOAuthTxRunner) — the attempts counter bumped
// here must stay incremented even if everything Complete() does afterward
// rolls back (that's the entire point of counting attempts).
//
// Thin wrapper over verifyPhoneOTP (shared with PhoneClaimService — §22
// least duplication): only the "how do I find the active challenge" step
// differs (handoff-scoped here vs. user-scoped there).
func (s *GoogleOAuthService) verifyOTP(ctx context.Context, handoffID uuid.UUID, normalizedPhone, otp string) (*model.PhoneVerification, error) {
	return verifyPhoneOTP(ctx, s.otps, s.phoneLockRunner, s.otpCfg, normalizedPhone, otp,
		func(ctx context.Context, otps googleOTPStore) (*model.PhoneVerification, error) {
			return otps.FindActive(ctx, handoffID, normalizedPhone)
		})
}

// verifyPhoneOTP is the shared engine behind GoogleOAuthService.verifyOTP
// (Google registration handoff) and PhoneClaimService.verifyOTP
// (authenticated user proving ownership of their own number) — both need the
// identical constant-time compare + per-phone failed-attempt cap +
// gated-attempts-increment dance; they only differ in HOW the active
// challenge row is looked up (`findActive`), which the caller supplies.
func verifyPhoneOTP(
	ctx context.Context,
	otps googleOTPStore,
	lockRunner phoneLockTxRunner,
	cfg OTPConfig,
	normalizedPhone, otp string,
	findActive func(context.Context, googleOTPStore) (*model.PhoneVerification, error),
) (*model.PhoneVerification, error) {
	pv, err := findActive(ctx, otps)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, authapi.ErrOTPExpired
		}
		return nil, fmt.Errorf("verify phone otp: lookup challenge: %w", err)
	}

	var attempts int
	err = lockRunner.RunInTx(ctx, normalizedPhone, func(tx googleOTPStore) error {
		if err := checkPerPhoneFailedCapWith(ctx, tx, normalizedPhone, cfg.MaxFailedPerPhoneHour); err != nil {
			return err
		}
		a, err := tx.IncrementAttemptsIfAllowed(ctx, pv.ID, cfg.MaxAttempts)
		if err != nil {
			return err
		}
		attempts = a
		return nil
	})
	if err != nil {
		switch {
		case errors.Is(err, authapi.ErrOTPTooManyAttempts):
			return nil, err
		case errors.Is(err, repository.ErrAttemptsExceeded):
			return nil, authapi.ErrOTPTooManyAttempts
		case errors.Is(err, repository.ErrNotFound):
			// The challenge stopped being active (consumed/expired) between
			// findActive above and the gated increment — same outcome as
			// findActive itself returning ErrNotFound (review finding #5a).
			return nil, authapi.ErrOTPExpired
		default:
			return nil, fmt.Errorf("verify phone otp: increment attempts: %w", err)
		}
	}

	want := []byte(pv.CodeHash)
	got := []byte(hashOTP(normalizedPhone, otp))
	if subtle.ConstantTimeCompare(got, want) != 1 {
		left := cfg.MaxAttempts - attempts
		if left < 0 {
			left = 0
		}
		return nil, &authapi.OTPInvalidError{AttemptsLeft: left}
	}

	if err := otps.MarkVerified(ctx, pv.ID); err != nil {
		return nil, fmt.Errorf("verify phone otp: mark verified: %w", err)
	}
	return pv, nil
}

// claimHandoffCode wraps codes.MarkUsed against the service's own (non-tx)
// store — used by the two Exchange() paths, which each only ever write to
// oauth_login_codes and don't need a cross-repository transaction.
func (s *GoogleOAuthService) claimHandoffCode(ctx context.Context, id uuid.UUID) error {
	return claimHandoffCodeWith(ctx, s.codes, id)
}

// claimHandoffCodeWith is the store-parameterized version — Complete() calls
// this with the TRANSACTION-scoped codes store (see googleOAuthCompletionTx)
// so the claim participates in the same commit/rollback as the rest of that
// method's writes (review finding #4).
//
// Maps "not found / already used" to the single ErrOAuthCodeInvalid sentinel
// the handler already knows how to map to a 400 (MarkUsed is atomic).
func claimHandoffCodeWith(ctx context.Context, codes googleOAuthCodeStore, id uuid.UUID) error {
	if err := codes.MarkUsed(ctx, id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return authapi.ErrOAuthCodeInvalid
		}
		return fmt.Errorf("google oauth: mark handoff code used: %w", err)
	}
	return nil
}

// resolveUserForCompletion decides how to create/attach the user row for a
// Google registration completion (§ business rule — phone is now optional and
// OTP-conditional):
//   - phoneOptional == nil → brand-new customer created with no phone at all.
//   - phoneOptional != nil && !phoneVerified → caller either supplied a
//     number nobody has claimed (Complete()'s own ExistsByPhone check, moments
//     earlier, said so), or never went through the OTP path at all. Either
//     way, an authoritative re-check happens HERE, inside the transaction: if
//     the number turns out to belong to someone by the time THIS lookup runs
//     (race, or an OTP-less caller lying about the number being free),
//     reject — an unverified claim must NEVER silently attach to an existing
//     row, guest or not. That's exactly the account-takeover hole OTP exists
//     to close, and it can't be allowed to reopen through a TOCTOU window.
//   - phoneOptional != nil && phoneVerified → caller has JUST verified a
//     correct OTP for this number (proves ownership). Nobody owns it (yet) →
//     create fresh with phone_verified_at set. A GUEST owns it → upgrade in
//     place (merge history, §11). Staff / already-registered owners are
//     NEVER merged into, even with a valid OTP — OTP proves WA ownership, not
//     the target account's password; silently attaching would be an account
//     takeover via a different channel (authapi.ErrPhoneAlreadyUsed, a dead
//     end requiring a different number).
//
// Takes the store as a parameter (rather than a method on *GoogleOAuthService)
// so Complete() can pass either the service's own store or a
// transaction-scoped one (review finding #4) — this function has no other
// dependency on the service.
func resolveUserForCompletion(ctx context.Context, users googleOAuthUserStore, phoneOptional *string, phoneVerified bool, name, subject, email string) (*model.User, error) {
	if phoneOptional == nil {
		return createRegisteredCustomer(ctx, users, nil, false, name, subject, email)
	}

	existing, err := users.FindByPhone(ctx, *phoneOptional)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return createRegisteredCustomer(ctx, users, phoneOptional, phoneVerified, name, subject, email)
		}
		return nil, fmt.Errorf("google oauth complete: lookup phone: %w", err)
	}

	if !phoneVerified {
		// See doc above — an unproven claim must never attach to an existing
		// row, no matter what type it is.
		return nil, authapi.ErrPhoneVerificationRequired
	}

	isRegisteredCustomer := existing.CustomerType != nil && *existing.CustomerType == model.CustomerTypeRegistered
	if existing.UserType == model.UserTypeStaff || isRegisteredCustomer {
		return nil, authapi.ErrPhoneAlreadyUsed
	}

	return upgradeGuestCustomer(ctx, users, existing, name, subject, email)
}

// createRegisteredCustomer inserts a brand-new registered customer.
// `phoneOptional` nil leaves the phone column NULL; non-nil attaches it, with
// phone_verified_at set ONLY when `phoneVerified` is true (an unclaimed
// number a caller merely typed in is claimed-but-unproven, matching the
// business rule — see resolveUserForCompletion's doc).
func createRegisteredCustomer(ctx context.Context, users googleOAuthUserStore, phoneOptional *string, phoneVerified bool, name, subject, email string) (*model.User, error) {
	if used, err := users.ExistsByEmail(ctx, email); err != nil {
		return nil, fmt.Errorf("google oauth complete: check email uniqueness: %w", err)
	} else if used {
		return nil, authapi.ErrEmailAlreadyUsed
	}

	registered := model.CustomerTypeRegistered
	provider := "google"
	u := &model.User{
		Email:         &email,
		Phone:         phoneOptional,
		Name:          name,
		UserType:      model.UserTypeCustomer,
		CustomerType:  &registered,
		OAuthProvider: &provider,
		OAuthSubject:  &subject,
		IsActive:      true,
	}
	if phoneOptional != nil && phoneVerified {
		now := time.Now().UTC()
		u.PhoneVerifiedAt = &now
	}
	if err := users.Create(ctx, u); err != nil {
		return nil, fmt.Errorf("google oauth complete: create user: %w", err)
	}
	return u, nil
}

// upgradeGuestCustomer promotes a matching guest to customer_type=registered
// with a linked Google identity. Only ever called once resolveUserForCompletion
// has already confirmed phoneVerified==true, so UserRepository.UpgradeGuestToRegistered
// unconditionally stamps phone_verified_at — the guest's phone was unverified
// by construction (POS never runs an OTP round-trip) and this call is exactly
// the moment ownership got proven.
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
func upgradeGuestCustomer(ctx context.Context, users googleOAuthUserStore, existing *model.User, name, subject, email string) (*model.User, error) {
	if !existing.IsActive {
		return nil, authapi.ErrUserInactive
	}

	var emailToSet *string
	if existing.Email == nil {
		used, err := users.ExistsByEmail(ctx, email)
		if err != nil {
			return nil, fmt.Errorf("google oauth complete: check email uniqueness: %w", err)
		}
		if used {
			return nil, authapi.ErrEmailAlreadyUsed
		}
		emailToSet = &email
	}

	if err := users.UpgradeGuestToRegistered(ctx, existing.ID, emailToSet, name, "google", subject); err != nil {
		return nil, fmt.Errorf("google oauth complete: upgrade guest to registered: %w", err)
	}
	updated, err := users.FindByID(ctx, existing.ID)
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
