package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/auth/model"
	"github.com/rajaku-printing/backend/internal/auth/oauth"
	"github.com/rajaku-printing/backend/internal/auth/repository"
	"github.com/rajaku-printing/backend/internal/auth/token"
)

// ---------- fakes ----------

// fakeGoogleUserStore is an in-memory stand-in for repository.UserRepository
// (narrowed to googleOAuthUserStore) — keeps GoogleOAuthService tests DB-free
// per §22.
type fakeGoogleUserStore struct {
	byID    map[uuid.UUID]*model.User
	byPhone map[string]*model.User
	byEmail map[string]*model.User
	byOAuth map[string]*model.User // key: provider+"|"+subject
}

func newFakeGoogleUserStore() *fakeGoogleUserStore {
	return &fakeGoogleUserStore{
		byID:    map[uuid.UUID]*model.User{},
		byPhone: map[string]*model.User{},
		byEmail: map[string]*model.User{},
		byOAuth: map[string]*model.User{},
	}
}

func (f *fakeGoogleUserStore) index(u *model.User) {
	f.byID[u.ID] = u
	if u.Phone != nil && *u.Phone != "" {
		f.byPhone[*u.Phone] = u
	}
	if u.Email != nil {
		f.byEmail[*u.Email] = u
	}
	if u.OAuthProvider != nil && u.OAuthSubject != nil {
		f.byOAuth[*u.OAuthProvider+"|"+*u.OAuthSubject] = u
	}
}

func (f *fakeGoogleUserStore) FindByID(_ context.Context, id uuid.UUID) (*model.User, error) {
	u, ok := f.byID[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return u, nil
}

func (f *fakeGoogleUserStore) FindByOAuth(_ context.Context, provider, subject string) (*model.User, error) {
	u, ok := f.byOAuth[provider+"|"+subject]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return u, nil
}

func (f *fakeGoogleUserStore) FindByEmail(_ context.Context, email string) (*model.User, error) {
	u, ok := f.byEmail[email]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return u, nil
}

func (f *fakeGoogleUserStore) FindByPhone(_ context.Context, phone string) (*model.User, error) {
	u, ok := f.byPhone[phone]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return u, nil
}

func (f *fakeGoogleUserStore) Create(_ context.Context, u *model.User) error {
	u.ID = uuid.New()
	f.index(u)
	return nil
}

func (f *fakeGoogleUserStore) UpdateLastLogin(_ context.Context, id uuid.UUID) error {
	if _, ok := f.byID[id]; !ok {
		return repository.ErrNotFound
	}
	return nil
}

func (f *fakeGoogleUserStore) ExistsByEmail(_ context.Context, email string) (bool, error) {
	_, ok := f.byEmail[email]
	return ok, nil
}

func (f *fakeGoogleUserStore) ExistsByPhone(_ context.Context, phone string) (bool, error) {
	_, ok := f.byPhone[phone]
	return ok, nil
}

// SetPhone mirrors the real repository's overwrite semantics — nil `phone`
// clears the index entry too, so a subsequent FindByPhone correctly misses.
func (f *fakeGoogleUserStore) SetPhone(_ context.Context, id uuid.UUID, phone *string, verifiedAt *time.Time) error {
	u, ok := f.byID[id]
	if !ok {
		return repository.ErrNotFound
	}
	if u.Phone != nil {
		delete(f.byPhone, *u.Phone)
	}
	u.Phone = phone
	u.PhoneVerifiedAt = verifiedAt
	if phone != nil && *phone != "" {
		f.byPhone[*phone] = u
	}
	return nil
}

func (f *fakeGoogleUserStore) LinkOAuth(_ context.Context, id uuid.UUID, provider, subject string) error {
	u, ok := f.byID[id]
	if !ok {
		return repository.ErrNotFound
	}
	u.OAuthProvider = &provider
	u.OAuthSubject = &subject
	f.byOAuth[provider+"|"+subject] = u
	return nil
}

// UpgradeGuestToRegistered mirrors the real repository's CASE-WHEN semantics:
// email/name only overwritten when currently empty.
func (f *fakeGoogleUserStore) UpgradeGuestToRegistered(_ context.Context, id uuid.UUID, email *string, name, provider, subject string) error {
	u, ok := f.byID[id]
	if !ok {
		return repository.ErrNotFound
	}
	// Mirrors migration 000001's `users_email_required` CHECK constraint:
	// customer_type='registered' requires email IS NOT NULL. A real Postgres
	// would reject this write; the fake must reject it too so tests exercise
	// the same failure mode the service is supposed to prevent (review
	// finding #2).
	if u.Email == nil && email == nil {
		return errEmailRequiredConstraint
	}
	registered := model.CustomerTypeRegistered
	u.CustomerType = &registered
	u.OAuthProvider = &provider
	u.OAuthSubject = &subject
	f.byOAuth[provider+"|"+subject] = u
	if u.Email == nil && email != nil {
		u.Email = email
		f.byEmail[*email] = u
	}
	if u.Name == "" {
		u.Name = name
	}
	// Mirrors UserRepository.UpgradeGuestToRegistered — ALWAYS stamps
	// phone_verified_at (this is only ever called after a verified OTP proved
	// ownership of the guest's phone, migration 000017).
	now := time.Now().UTC()
	u.PhoneVerifiedAt = &now
	return nil
}

// errEmailRequiredConstraint stands in for the Postgres CHECK violation a
// real DB would raise — the service must never actually trigger this (it's
// supposed to reject with authapi.ErrEmailAlreadyUsed BEFORE calling
// UpgradeGuestToRegistered in the conflicting-email case).
var errEmailRequiredConstraint = errors.New("fake db: users_email_required constraint violated (email IS NULL for customer_type=registered)")

// snapshot/restore — give fakeTxRunner (below) real DB-transaction-like
// rollback semantics so tests can exercise review finding #4 ("resolve fails
// mid-transaction → handoff/user state reverts") without a real Postgres.
// Shallow-copying each *model.User value is enough: every mutation in this
// file reassigns a struct FIELD (mis. `u.Email = &email`), never dereferences
// through an existing pointer to mutate shared data in place, so a snapshot
// taken before fn runs is unaffected by whatever fn does to the live maps
// afterward.
func (f *fakeGoogleUserStore) snapshot() map[uuid.UUID]model.User {
	snap := make(map[uuid.UUID]model.User, len(f.byID))
	for k, v := range f.byID {
		snap[k] = *v
	}
	return snap
}

func (f *fakeGoogleUserStore) restore(snap map[uuid.UUID]model.User) {
	f.byID = map[uuid.UUID]*model.User{}
	f.byPhone = map[string]*model.User{}
	f.byEmail = map[string]*model.User{}
	f.byOAuth = map[string]*model.User{}
	for _, v := range snap {
		val := v
		f.index(&val)
	}
}

// fakeCodeStore is an in-memory stand-in for repository.OAuthLoginCodeRepository.
type fakeCodeStore struct {
	byHash map[string]*model.OAuthLoginCode
	// createErr — test hook (review finding #3): when set, every Create call
	// fails. Lets tests simulate a reissue that fails AFTER the old handoff
	// code was already claimed, to prove claim+reissue roll back together.
	createErr error
}

func newFakeCodeStore() *fakeCodeStore {
	return &fakeCodeStore{byHash: map[string]*model.OAuthLoginCode{}}
}

func (f *fakeCodeStore) Create(_ context.Context, c *model.OAuthLoginCode) error {
	if f.createErr != nil {
		return f.createErr
	}
	c.ID = uuid.New()
	f.byHash[c.CodeHash] = c
	return nil
}

func (f *fakeCodeStore) FindActiveByCode(_ context.Context, codeHash string) (*model.OAuthLoginCode, error) {
	c, ok := f.byHash[codeHash]
	if !ok || c.UsedAt != nil || !time.Now().UTC().Before(c.ExpiresAt) {
		return nil, repository.ErrNotFound
	}
	return c, nil
}

// MarkUsed mirrors the real repository's atomic "UPDATE ... WHERE used_at IS
// NULL" semantics (review finding #4) — a second call against an
// already-used code must fail, not silently succeed again.
func (f *fakeCodeStore) MarkUsed(_ context.Context, id uuid.UUID) error {
	for _, c := range f.byHash {
		if c.ID == id {
			if c.UsedAt != nil {
				return repository.ErrNotFound
			}
			now := time.Now().UTC()
			c.UsedAt = &now
			return nil
		}
	}
	return repository.ErrNotFound
}

// snapshot/restore — see fakeGoogleUserStore's doc above for why a shallow
// per-value copy is sufficient.
func (f *fakeCodeStore) snapshot() map[string]model.OAuthLoginCode {
	snap := make(map[string]model.OAuthLoginCode, len(f.byHash))
	for k, v := range f.byHash {
		snap[k] = *v
	}
	return snap
}

func (f *fakeCodeStore) restore(snap map[string]model.OAuthLoginCode) {
	f.byHash = make(map[string]*model.OAuthLoginCode, len(snap))
	for k, v := range snap {
		val := v
		f.byHash[k] = &val
	}
}

// fakeOTPStore is an in-memory stand-in for repository.PhoneVerificationRepository.
// fakeOTPStore is guarded by `mu` (unlike the other fakes in this file) —
// unlike everything else here, it's exercised by an actual concurrency test
// (TestGoogleOAuthService_Complete_ConcurrentWrongGuesses_AttemptsCeilingNeverBypassed)
// that fires many goroutines at it, so it has to be safe under `go test -race`.
type fakeOTPStore struct {
	mu   sync.Mutex
	byID map[uuid.UUID]*model.PhoneVerification
	// beforeIncrement — test hook (review finding #5b): if set, invoked right
	// before IncrementAttemptsIfAllowed's own gated check runs, WITHOUT
	// holding f.mu (avoids self-deadlock if the hook itself calls back into
	// this store) — lets a test simulate exactly the race finding #5b closes:
	// another concurrent write (mis. a fresh RequestOTP's
	// CancelPendingForHandoff) landing in the window between verifyOTP's
	// FindActive call and the gated increment that follows it.
	beforeIncrement func()
}

func newFakeOTPStore() *fakeOTPStore {
	return &fakeOTPStore{byID: map[uuid.UUID]*model.PhoneVerification{}}
}

func (f *fakeOTPStore) Create(_ context.Context, pv *model.PhoneVerification) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	pv.ID = uuid.New()
	pv.CreatedAt = time.Now().UTC()
	f.byID[pv.ID] = pv
	return nil
}

func (f *fakeOTPStore) FindActive(_ context.Context, handoffID uuid.UUID, phone string) (*model.PhoneVerification, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var best *model.PhoneVerification
	now := time.Now().UTC()
	for _, pv := range f.byID {
		if pv.OAuthLoginCodeID == nil || *pv.OAuthLoginCodeID != handoffID || pv.Phone != phone {
			continue
		}
		if pv.ConsumedAt != nil || !now.Before(pv.ExpiresAt) {
			continue
		}
		if best == nil || pv.CreatedAt.After(best.CreatedAt) {
			best = pv
		}
	}
	if best == nil {
		return nil, repository.ErrNotFound
	}
	// Return a COPY, not the live pointer — verifyOTP reads pv.CodeHash after
	// this call returns, potentially concurrently with another goroutine's
	// IncrementAttemptsIfAllowed mutating the SAME underlying struct's
	// Attempts field. CodeHash itself is never mutated post-creation so this
	// is belt-and-suspenders, but returning a snapshot removes any doubt
	// under `go test -race`.
	cp := *best
	return &cp, nil
}

func (f *fakeOTPStore) FindLatest(_ context.Context, handoffID uuid.UUID, phone string) (*model.PhoneVerification, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var best *model.PhoneVerification
	for _, pv := range f.byID {
		if pv.OAuthLoginCodeID == nil || *pv.OAuthLoginCodeID != handoffID || pv.Phone != phone {
			continue
		}
		if best == nil || pv.CreatedAt.After(best.CreatedAt) {
			best = pv
		}
	}
	if best == nil {
		return nil, repository.ErrNotFound
	}
	cp := *best
	return &cp, nil
}

func (f *fakeOTPStore) CancelPendingForHandoff(_ context.Context, handoffID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now().UTC()
	for _, pv := range f.byID {
		if pv.OAuthLoginCodeID != nil && *pv.OAuthLoginCodeID == handoffID && pv.ConsumedAt == nil {
			pv.ConsumedAt = &now
		}
	}
	return nil
}

// FindActiveByUser / FindLatestByUser / CancelPendingForUser — user_id-keyed
// counterparts used by PhoneClaimService tests.
func (f *fakeOTPStore) FindActiveByUser(_ context.Context, userID uuid.UUID, phone string) (*model.PhoneVerification, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var best *model.PhoneVerification
	now := time.Now().UTC()
	for _, pv := range f.byID {
		if pv.UserID == nil || *pv.UserID != userID || pv.Phone != phone {
			continue
		}
		if pv.ConsumedAt != nil || !now.Before(pv.ExpiresAt) {
			continue
		}
		if best == nil || pv.CreatedAt.After(best.CreatedAt) {
			best = pv
		}
	}
	if best == nil {
		return nil, repository.ErrNotFound
	}
	cp := *best
	return &cp, nil
}

func (f *fakeOTPStore) FindLatestByUser(_ context.Context, userID uuid.UUID, phone string) (*model.PhoneVerification, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var best *model.PhoneVerification
	for _, pv := range f.byID {
		if pv.UserID == nil || *pv.UserID != userID || pv.Phone != phone {
			continue
		}
		if best == nil || pv.CreatedAt.After(best.CreatedAt) {
			best = pv
		}
	}
	if best == nil {
		return nil, repository.ErrNotFound
	}
	cp := *best
	return &cp, nil
}

func (f *fakeOTPStore) CancelPendingForUser(_ context.Context, userID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now().UTC()
	for _, pv := range f.byID {
		if pv.UserID != nil && *pv.UserID == userID && pv.ConsumedAt == nil {
			pv.ConsumedAt = &now
		}
	}
	return nil
}

// IncrementAttemptsIfAllowed mirrors repository.PhoneVerificationRepository's
// gated-atomic-UPDATE semantics (review finding #1): the ceiling check and
// the increment happen as a single indivisible step from the caller's point
// of view — there is no separate "read attempts, then decide" step a racing
// caller could observe a stale value from. It also mirrors the WHERE clause
// review finding #5b added — a row that's been consumed or expired since it
// was loaded via FindActive can never have its attempts bumped, same as
// FindActive would refuse to return it at all.
func (f *fakeOTPStore) IncrementAttemptsIfAllowed(_ context.Context, id uuid.UUID, maxAttempts int) (int, error) {
	if f.beforeIncrement != nil {
		f.beforeIncrement()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	pv, ok := f.byID[id]
	if !ok {
		return 0, repository.ErrNotFound
	}
	now := time.Now().UTC()
	if pv.ConsumedAt != nil || !pv.ExpiresAt.After(now) {
		return 0, repository.ErrNotFound
	}
	if pv.Attempts >= maxAttempts {
		return 0, repository.ErrAttemptsExceeded
	}
	pv.Attempts++
	return pv.Attempts, nil
}

// CountAndOldestSince / SumAttemptsSince — fake equivalents of the
// repository's per-phone rate-limit aggregate queries (review finding #2).
func (f *fakeOTPStore) CountAndOldestSince(_ context.Context, phone string, since time.Time) (int64, *time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var count int64
	var oldest *time.Time
	for _, pv := range f.byID {
		if pv.Phone != phone || !pv.CreatedAt.After(since) {
			continue
		}
		count++
		if oldest == nil || pv.CreatedAt.Before(*oldest) {
			t := pv.CreatedAt
			oldest = &t
		}
	}
	return count, oldest, nil
}

func (f *fakeOTPStore) SumAttemptsSince(_ context.Context, phone string, since time.Time) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var total int64
	for _, pv := range f.byID {
		if pv.Phone != phone || !pv.CreatedAt.After(since) {
			continue
		}
		total += int64(pv.Attempts)
	}
	return total, nil
}

// snapshot/restore — see fakeGoogleUserStore's doc above for why a shallow
// per-value copy is sufficient.
func (f *fakeOTPStore) snapshot() map[uuid.UUID]model.PhoneVerification {
	f.mu.Lock()
	defer f.mu.Unlock()
	snap := make(map[uuid.UUID]model.PhoneVerification, len(f.byID))
	for k, v := range f.byID {
		snap[k] = *v
	}
	return snap
}

func (f *fakeOTPStore) restore(snap map[uuid.UUID]model.PhoneVerification) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.byID = make(map[uuid.UUID]*model.PhoneVerification, len(snap))
	for k, v := range snap {
		val := v
		f.byID[k] = &val
	}
}

func (f *fakeOTPStore) MarkVerified(_ context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	pv, ok := f.byID[id]
	if !ok {
		return repository.ErrNotFound
	}
	now := time.Now().UTC()
	pv.VerifiedAt = &now
	return nil
}

func (f *fakeOTPStore) MarkConsumed(_ context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	pv, ok := f.byID[id]
	if !ok {
		return repository.ErrNotFound
	}
	now := time.Now().UTC()
	pv.ConsumedAt = &now
	return nil
}

// fakeOTPSender is an in-memory stand-in for notificationapi.OTPSender —
// captures the last enqueued message so tests can pull the raw OTP code back
// out (the code never appears anywhere else — it's hashed at rest, per
// migration 000013).
// Pengiriman OTP terjadi DI LUAR transaksi advisory-lock per-nomor (lock
// dilepas begitu challenge tersimpan — sengaja, supaya lock DB tidak ditahan
// selama enqueue). Akibatnya test konkurensi memanggil EnqueueOTP dari banyak
// goroutine sekaligus, jadi fake ini wajib aman-konkurensi. Implementasi
// aslinya menulis ke Postgres, bukan ke struct bersama, jadi mutex ini murni
// kebutuhan test double.
type fakeOTPSender struct {
	mu          sync.Mutex
	lastPhone   string
	lastMessage string
	lastDedup   string
	calls       int
	err         error
}

func (f *fakeOTPSender) EnqueueOTP(_ context.Context, phone, message, dedupKey string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls++
	if f.err != nil {
		return f.err
	}
	f.lastPhone = phone
	f.lastMessage = message
	f.lastDedup = dedupKey
	return nil
}

// fakeGoogleExchanger stands in for oauth.GoogleClient — the real HTTP
// exchange is covered by internal/auth/oauth/google_test.go.
type fakeGoogleExchanger struct {
	profile *oauth.GoogleProfile
	err     error
}

func (f *fakeGoogleExchanger) AuthCodeURL(state string) string {
	return "https://accounts.google.com/o/oauth2/v2/auth?state=" + state
}

func (f *fakeGoogleExchanger) Exchange(_ context.Context, _ string) (*oauth.GoogleProfile, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.profile, nil
}

// fakeTxRunner is the test stand-in for googleOAuthTxRunner. It runs fn
// directly against the harness's own fakes (no real gorm.DB), but adds
// snapshot/rollback around the call so it exercises the same contract a real
// DB transaction provides (review finding #4): if fn returns an error, every
// write it made through the bundled stores is undone, as if it never ran.
type fakeTxRunner struct {
	users *fakeGoogleUserStore
	codes *fakeCodeStore
	otps  *fakeOTPStore
}

func (f *fakeTxRunner) RunInTx(ctx context.Context, fn func(tx googleOAuthCompletionTx) error) error {
	usersSnap := f.users.snapshot()
	codesSnap := f.codes.snapshot()
	otpsSnap := f.otps.snapshot()
	err := fn(googleOAuthCompletionTx{Users: f.users, Codes: f.codes, OTPs: f.otps})
	if err != nil {
		f.users.restore(usersSnap)
		f.codes.restore(codesSnap)
		f.otps.restore(otpsSnap)
	}
	return err
}

// fakePhoneLockTxRunner is the test stand-in for phoneLockTxRunner (review
// finding #1). Unlike fakeTxRunner above (which only needs rollback
// semantics), this one has to actually SERIALIZE concurrent calls for the
// SAME phone number — that's the entire property under test: without real
// serialization, a check-then-mutate sequence built on top of fakeOTPStore
// (whose own methods each lock/unlock f.mu individually, not held across
// calls) would still be racy, same as the pre-fix production code was
// against Postgres. A per-phone sync.Mutex is a faithful-enough stand-in for
// pg_advisory_xact_lock's per-key serialization for test purposes.
type fakePhoneLockTxRunner struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
	otps  *fakeOTPStore
}

func newFakePhoneLockTxRunner(otps *fakeOTPStore) *fakePhoneLockTxRunner {
	return &fakePhoneLockTxRunner{locks: map[string]*sync.Mutex{}, otps: otps}
}

func (f *fakePhoneLockTxRunner) lockFor(phone string) *sync.Mutex {
	f.mu.Lock()
	defer f.mu.Unlock()
	l, ok := f.locks[phone]
	if !ok {
		l = &sync.Mutex{}
		f.locks[phone] = l
	}
	return l
}

func (f *fakePhoneLockTxRunner) RunInTx(_ context.Context, normalizedPhone string, fn func(tx googleOTPStore) error) error {
	l := f.lockFor(normalizedPhone)
	l.Lock()
	defer l.Unlock()
	return fn(f.otps)
}

// ---------- helper ----------

// testHarness bundles the fakes so tests can both build the service AND
// drive the OTP round-trip (RequestOTP → pull the code out of the fake
// sender → Complete) without repeating the wiring in every test.
type testHarness struct {
	users  *fakeGoogleUserStore
	codes  *fakeCodeStore
	otps   *fakeOTPStore
	sender *fakeOTPSender
	svc    *GoogleOAuthService
}

// newTestHarness fills in sane defaults for any UNSET field of otpCfg
// (per-field, not "all-or-nothing") — tests that only care about customizing
// MaxAttempts (say) shouldn't have to also spell out the per-phone rate-limit
// fields just to avoid tripping them accidentally.
func newTestHarness(t *testing.T, exch googleExchanger, otpCfg OTPConfig) *testHarness {
	t.Helper()
	iss, err := token.NewIssuer("test-secret-32-chars-minimum-abcdef", time.Hour, "rajaku-test")
	if err != nil {
		t.Fatalf("NewIssuer: %v", err)
	}
	if otpCfg.TTL == 0 {
		otpCfg.TTL = 5 * time.Minute
	}
	if otpCfg.MaxAttempts == 0 {
		otpCfg.MaxAttempts = 5
	}
	if otpCfg.ResendCooldown == 0 {
		otpCfg.ResendCooldown = 60 * time.Second
	}
	if otpCfg.CodeLength == 0 {
		otpCfg.CodeLength = 6
	}
	if otpCfg.MaxPerPhoneHour == 0 {
		otpCfg.MaxPerPhoneHour = 1000
	}
	if otpCfg.MaxFailedPerPhoneHour == 0 {
		otpCfg.MaxFailedPerPhoneHour = 1000
	}
	users := newFakeGoogleUserStore()
	codes := newFakeCodeStore()
	otps := newFakeOTPStore()
	sender := &fakeOTPSender{}
	svc := &GoogleOAuthService{
		users: users, codes: codes, otps: otps, google: exch,
		issuer: iss, otpCfg: otpCfg, otpSender: sender,
		txRunner:        &fakeTxRunner{users: users, codes: codes, otps: otps},
		phoneLockRunner: newFakePhoneLockTxRunner(otps),
	}
	return &testHarness{users: users, codes: codes, otps: otps, sender: sender, svc: svc}
}

// requestAndReadOTP drives RequestOTP and returns the raw code the fake
// sender captured (equivalent to "reading the WhatsApp message" in a test).
func (h *testHarness) requestAndReadOTP(t *testing.T, handoffCode, rawPhone string) string {
	t.Helper()
	if _, err := h.svc.RequestOTP(context.Background(), RequestOTPInput{Code: handoffCode, Phone: rawPhone}); err != nil {
		t.Fatalf("RequestOTP: unexpected err: %v", err)
	}
	code := extractOTPCode(h.sender.lastMessage)
	if code == "" {
		t.Fatalf("could not extract OTP code from message: %q", h.sender.lastMessage)
	}
	return code
}

// extractOTPCode pulls the digits out of "Kode verifikasi Rajaku Printing:
// 123456. Berlaku ...".
func extractOTPCode(msg string) string {
	const prefix = "Kode verifikasi Rajaku Printing: "
	if len(msg) < len(prefix) {
		return ""
	}
	rest := msg[len(prefix):]
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	return rest[:end]
}

// ---------- tests ----------

// (a) Happy path: user already has a linked Google identity → callback +
// exchange yields a session immediately (no OTP involved — this is a
// returning login, not a registration).
func TestGoogleOAuthService_HandleCallbackThenExchange_ExistingOAuthUser_ReturnsSession(t *testing.T) {
	provider := "google"
	subject := "sub-existing-1"
	custType := model.CustomerTypeRegistered
	existing := &model.User{
		ID: uuid.New(), Phone: strp("6281200000001"), Name: "Budi Existing",
		UserType: model.UserTypeCustomer, CustomerType: &custType,
		OAuthProvider: &provider, OAuthSubject: &subject, IsActive: true,
	}

	exch := &fakeGoogleExchanger{profile: &oauth.GoogleProfile{
		Subject: subject, Email: "budi@example.com", EmailVerified: true, Name: "Budi Existing",
	}}
	h := newTestHarness(t, exch, OTPConfig{})
	h.users.index(existing)

	handoff, err := h.svc.HandleCallback(context.Background(), "any-code", "/akun/pesanan")
	if err != nil {
		t.Fatalf("HandleCallback: unexpected err: %v", err)
	}
	if handoff == "" {
		t.Fatal("expected non-empty handoff code")
	}

	out, err := h.svc.Exchange(context.Background(), handoff)
	if err != nil {
		t.Fatalf("Exchange: unexpected err: %v", err)
	}
	if out.Status != "session" {
		t.Fatalf("expected status=session, got %q", out.Status)
	}
	if out.User == nil || out.User.ID != existing.ID {
		t.Fatalf("expected user id %s, got %+v", existing.ID, out.User)
	}
	if out.Token == nil || out.Token.AccessToken == "" {
		t.Fatal("expected non-empty access token")
	}
	if out.RedirectPath != "/akun/pesanan" {
		t.Fatalf("expected redirect_path preserved, got %q", out.RedirectPath)
	}

	// Handoff code is single-use.
	if _, err := h.svc.Exchange(context.Background(), handoff); !errors.Is(err, authapi.ErrOAuthCodeInvalid) {
		t.Fatalf("expected ErrOAuthCodeInvalid on replay, got %v", err)
	}
}

// (b) Brand-new Google identity: HandleCallback → need_phone, RequestOTP
// sends a code, and Complete with the correct code creates a new registered
// customer. This is the full happy path for the OTP security fix.
//
// (b) is now split into several tests matching the business rule (OTP hanya
// untuk tabrakan identitas, bukan setiap registrasi):
//   - EmptyPhone: no phone at all → no OTP, ever.
//   - FreshUnclaimedPhone: phone nobody owns → no OTP, account created
//     unverified.
//   - PhoneTaken_NoOTP: phone already claimed, otp omitted →
//     ErrPhoneVerificationRequired (PHONE_ALREADY_IN_USE).
//   - PhoneTaken_OTPRoundTrip: phone already claimed (by a guest — the
//     realistic "identity collision" case), full OTP round-trip completes
//     registration AND stamps phone_verified_at (§11 upgrade-in-place is
//     covered separately below by TestGoogleOAuthService_Complete_GuestPhoneMatch...).

func TestGoogleOAuthService_Complete_EmptyPhone_CreatesAccountWithoutOTP(t *testing.T) {
	exch := &fakeGoogleExchanger{profile: &oauth.GoogleProfile{
		Subject: "sub-empty-phone", Email: "emptyphone@example.com", EmailVerified: true, Name: "Tanpa Nomor",
	}}
	h := newTestHarness(t, exch, OTPConfig{})

	handoff, err := h.svc.HandleCallback(context.Background(), "any-code", "")
	if err != nil {
		t.Fatalf("HandleCallback: unexpected err: %v", err)
	}
	out, err := h.svc.Exchange(context.Background(), handoff)
	if err != nil {
		t.Fatalf("Exchange: unexpected err: %v", err)
	}

	completed, err := h.svc.Complete(context.Background(), CompleteGoogleInput{
		Code: out.Code, Phone: "", Name: "",
	})
	if err != nil {
		t.Fatalf("Complete: unexpected err: %v", err)
	}
	if completed.User.Phone != nil {
		t.Fatalf("expected nil phone, got %q", *completed.User.Phone)
	}
	if completed.User.PhoneVerifiedAt != nil {
		t.Fatalf("expected phone_verified_at nil when no phone given, got %v", *completed.User.PhoneVerifiedAt)
	}
	if h.sender.calls != 0 {
		t.Fatalf("expected NO otp ever sent for an empty-phone registration, got %d calls", h.sender.calls)
	}
}

func TestGoogleOAuthService_Complete_FreshUnclaimedPhone_CreatesAccountWithoutOTP(t *testing.T) {
	exch := &fakeGoogleExchanger{profile: &oauth.GoogleProfile{
		Subject: "sub-fresh-phone", Email: "freshphone@example.com", EmailVerified: true, Name: "Nomor Baru",
	}}
	h := newTestHarness(t, exch, OTPConfig{})

	handoff, err := h.svc.HandleCallback(context.Background(), "any-code", "")
	if err != nil {
		t.Fatalf("HandleCallback: unexpected err: %v", err)
	}
	out, err := h.svc.Exchange(context.Background(), handoff)
	if err != nil {
		t.Fatalf("Exchange: unexpected err: %v", err)
	}

	otpOut, err := h.svc.RequestOTP(context.Background(), RequestOTPInput{Code: out.Code, Phone: "081234500001"})
	if err != nil {
		t.Fatalf("RequestOTP: unexpected err: %v", err)
	}
	if otpOut.OTPRequired {
		t.Fatalf("expected otp_required=false for an unclaimed phone, got true")
	}
	if h.sender.calls != 0 {
		t.Fatalf("expected RequestOTP to send NOTHING for an unclaimed phone, got %d calls", h.sender.calls)
	}

	completed, err := h.svc.Complete(context.Background(), CompleteGoogleInput{
		Code: out.Code, Phone: "081234500001", Name: "",
	})
	if err != nil {
		t.Fatalf("Complete: unexpected err: %v", err)
	}
	if completed.Status != "session" {
		t.Fatalf("expected status=session, got %q", completed.Status)
	}
	if completed.User.Phone == nil || *completed.User.Phone != "6281234500001" {
		t.Fatalf("expected normalized phone, got %+v", completed.User.Phone)
	}
	if completed.User.PhoneVerifiedAt != nil {
		t.Fatalf("expected phone_verified_at nil (claimed but unproven), got %v", *completed.User.PhoneVerifiedAt)
	}
	if completed.User.Name != "Nomor Baru" {
		t.Fatalf("expected name falls back to google profile name, got %q", completed.User.Name)
	}
	if completed.User.CustomerType == nil || *completed.User.CustomerType != model.CustomerTypeRegistered {
		t.Fatalf("expected customer_type=registered, got %+v", completed.User.CustomerType)
	}
	if completed.User.OAuthSubject == nil || *completed.User.OAuthSubject != "sub-fresh-phone" {
		t.Fatalf("expected oauth_subject linked, got %+v", completed.User.OAuthSubject)
	}
	if h.sender.calls != 0 {
		t.Fatalf("expected NO otp ever sent across the whole free-phone flow, got %d calls", h.sender.calls)
	}
}

func TestGoogleOAuthService_Complete_PhoneTaken_NoOTP_ReturnsErrPhoneVerificationRequired(t *testing.T) {
	exch := &fakeGoogleExchanger{profile: &oauth.GoogleProfile{
		Subject: "sub-taken-no-otp", Email: "takennootp@example.com", EmailVerified: true, Name: "Taken No OTP",
	}}
	h := newTestHarness(t, exch, OTPConfig{})
	seedUserAtPhone(h, "6281234500099")

	handoff, err := h.svc.HandleCallback(context.Background(), "any-code", "")
	if err != nil {
		t.Fatalf("HandleCallback: unexpected err: %v", err)
	}
	out, err := h.svc.Exchange(context.Background(), handoff)
	if err != nil {
		t.Fatalf("Exchange: unexpected err: %v", err)
	}

	_, err = h.svc.Complete(context.Background(), CompleteGoogleInput{
		Code: out.Code, Phone: "081234500099", Name: "Taken No OTP",
	})
	if !errors.Is(err, authapi.ErrPhoneVerificationRequired) {
		t.Fatalf("expected ErrPhoneVerificationRequired, got %v", err)
	}
	if h.sender.calls != 0 {
		t.Fatalf("expected Complete() itself to never send an otp, got %d calls", h.sender.calls)
	}
}

func TestGoogleOAuthService_RequestOTP_PhoneAlreadyClaimed_ReturnsOTPRequiredTrue(t *testing.T) {
	exch := &fakeGoogleExchanger{profile: &oauth.GoogleProfile{
		Subject: "sub-taken-otp-req", Email: "takenotpreq@example.com", EmailVerified: true, Name: "Taken",
	}}
	h := newTestHarness(t, exch, OTPConfig{})
	seedUserAtPhone(h, "6281234500098")

	handoff, err := h.svc.HandleCallback(context.Background(), "any-code", "")
	if err != nil {
		t.Fatalf("HandleCallback: unexpected err: %v", err)
	}
	out, err := h.svc.Exchange(context.Background(), handoff)
	if err != nil {
		t.Fatalf("Exchange: unexpected err: %v", err)
	}

	otpOut, err := h.svc.RequestOTP(context.Background(), RequestOTPInput{Code: out.Code, Phone: "081234500098"})
	if err != nil {
		t.Fatalf("RequestOTP: unexpected err: %v", err)
	}
	if !otpOut.OTPRequired {
		t.Fatalf("expected otp_required=true for an already-claimed phone, got false")
	}
	if h.sender.calls != 1 {
		t.Fatalf("expected exactly 1 otp send, got %d", h.sender.calls)
	}
}

func TestGoogleOAuthService_NewIdentity_OTPRoundTrip_CompletesRegistration(t *testing.T) {
	exch := &fakeGoogleExchanger{profile: &oauth.GoogleProfile{
		Subject: "sub-new-1", Email: "newbie@example.com", EmailVerified: true, Name: "New Person",
	}}
	h := newTestHarness(t, exch, OTPConfig{})
	// Under the business rule, RequestOTP only sends anything when the
	// number is already claimed — seed a colliding (guest) owner so this
	// test still exercises the full OTP round trip end to end.
	seedUserAtPhone(h, "6281234500001")

	handoff, err := h.svc.HandleCallback(context.Background(), "any-code", "")
	if err != nil {
		t.Fatalf("HandleCallback: unexpected err: %v", err)
	}

	out, err := h.svc.Exchange(context.Background(), handoff)
	if err != nil {
		t.Fatalf("Exchange: unexpected err: %v", err)
	}
	if out.Status != "need_phone" {
		t.Fatalf("expected status=need_phone, got %q", out.Status)
	}
	if out.Email != "newbie@example.com" {
		t.Fatalf("unexpected exchange output: %+v", out)
	}
	// Exchange ROTATES the registration handoff (review finding #8) — the
	// code returned must be brand new, not an echo of the original.
	if out.Code == "" || out.Code == handoff {
		t.Fatalf("expected Exchange to rotate the registration handoff to a NEW code, got %q (original %q)", out.Code, handoff)
	}
	// The pre-rotation code (what a scraped browser-history/Referer entry
	// would contain) must be dead from this point on.
	if _, err := h.svc.RequestOTP(context.Background(), RequestOTPInput{Code: handoff, Phone: "081234500001"}); !errors.Is(err, authapi.ErrOAuthCodeInvalid) {
		t.Fatalf("expected ErrOAuthCodeInvalid using the pre-rotation code, got %v", err)
	}

	code := h.requestAndReadOTP(t, out.Code, "081234500001")
	if h.sender.lastPhone != "6281234500001" {
		t.Fatalf("expected otp sent to normalized phone, got %q", h.sender.lastPhone)
	}

	completed, err := h.svc.Complete(context.Background(), CompleteGoogleInput{
		Code: out.Code, Phone: "081234500001", Name: "", OTP: code,
	})
	if err != nil {
		t.Fatalf("Complete: unexpected err: %v", err)
	}
	if completed.Status != "session" {
		t.Fatalf("expected status=session, got %q", completed.Status)
	}
	if completed.User.Phone == nil || *completed.User.Phone != "6281234500001" {
		t.Fatalf("expected normalized phone, got %+v", completed.User.Phone)
	}
	if completed.User.PhoneVerifiedAt == nil {
		t.Fatal("expected phone_verified_at set after a valid OTP round-trip")
	}
	if completed.User.CustomerType == nil || *completed.User.CustomerType != model.CustomerTypeRegistered {
		t.Fatalf("expected customer_type=registered, got %+v", completed.User.CustomerType)
	}
	if completed.User.OAuthSubject == nil || *completed.User.OAuthSubject != "sub-new-1" {
		t.Fatalf("expected oauth_subject linked, got %+v", completed.User.OAuthSubject)
	}

	// Handoff code is single-use — a second Complete (even with a fresh OTP
	// request) must fail.
	if _, err := h.svc.Complete(context.Background(), CompleteGoogleInput{
		Code: out.Code, Phone: "081234500001", OTP: code,
	}); !errors.Is(err, authapi.ErrOAuthCodeInvalid) {
		t.Fatalf("expected ErrOAuthCodeInvalid on replay, got %v", err)
	}
}

// (c) Phone matches an existing GUEST customer → upgraded in place, same
// user_id — proof that order history stays attached (§11).
func TestGoogleOAuthService_Complete_GuestPhoneMatch_UpgradesInPlace_SameUserID(t *testing.T) {
	guestType := model.CustomerTypeGuest
	guest := &model.User{
		ID: uuid.New(), Phone: strp("6281234500002"), Name: "Guest Existing",
		UserType: model.UserTypeCustomer, CustomerType: &guestType, IsActive: true,
	}
	originalID := guest.ID

	exch := &fakeGoogleExchanger{profile: &oauth.GoogleProfile{
		Subject: "sub-guest-upgrade", Email: "guest-google@example.com", EmailVerified: true, Name: "Guest Google",
	}}
	h := newTestHarness(t, exch, OTPConfig{})
	h.users.index(guest)

	handoff, err := h.svc.HandleCallback(context.Background(), "any-code", "")
	if err != nil {
		t.Fatalf("HandleCallback: unexpected err: %v", err)
	}
	code := h.requestAndReadOTP(t, handoff, "081234500002")

	completed, err := h.svc.Complete(context.Background(), CompleteGoogleInput{
		Code: handoff, Phone: "081234500002", OTP: code,
	})
	if err != nil {
		t.Fatalf("Complete: unexpected err: %v", err)
	}
	if completed.User.ID != originalID {
		t.Fatalf("expected SAME user_id after upgrade (§11 riwayat gabung), got %s want %s", completed.User.ID, originalID)
	}
	if completed.User.CustomerType == nil || *completed.User.CustomerType != model.CustomerTypeRegistered {
		t.Fatalf("expected customer_type upgraded to registered, got %+v", completed.User.CustomerType)
	}
	if completed.User.Email == nil || *completed.User.Email != "guest-google@example.com" {
		t.Fatalf("expected email filled in from google (was nil), got %+v", completed.User.Email)
	}
	if completed.User.OAuthSubject == nil || *completed.User.OAuthSubject != "sub-guest-upgrade" {
		t.Fatalf("expected oauth_subject linked, got %+v", completed.User.OAuthSubject)
	}
}

// (c2) Guest upgrade where the Google email is ALREADY claimed by a
// different user → must reject explicitly with ErrEmailAlreadyUsed, never
// silently upgrade with email left NULL (review finding #2 — that used to
// violate the users_email_required CHECK and bubble up as an opaque 500).
func TestGoogleOAuthService_Complete_GuestUpgrade_EmailAlreadyUsedByAnotherUser_ReturnsErrEmailAlreadyUsed(t *testing.T) {
	guestType := model.CustomerTypeGuest
	guest := &model.User{
		ID: uuid.New(), Phone: strp("6281234500003"), Name: "Guest Conflict",
		UserType: model.UserTypeCustomer, CustomerType: &guestType, IsActive: true,
	}
	takenEmail := "taken@example.com"
	registeredType := model.CustomerTypeRegistered
	otherUser := &model.User{
		ID: uuid.New(), Phone: strp("6281200099999"), Name: "Sudah Ada Email Ini",
		Email: &takenEmail, UserType: model.UserTypeCustomer, CustomerType: &registeredType, IsActive: true,
	}

	h := newTestHarness(t, &fakeGoogleExchanger{}, OTPConfig{})
	h.users.index(guest)
	h.users.index(otherUser)

	// Seed the registration handoff DIRECTLY (bypass HandleCallback): if the
	// Google email matched an existing account, HandleCallback's own
	// email-match branch would short-circuit into a SESSION handoff for that
	// other account before registration is ever reached — so the only way
	// this conflict can surface is if the email became taken by someone else
	// AFTER the registration handoff was already issued (e.g. that other
	// person registered in the few minutes the guest was filling in the
	// phone-number form). Constructing the handoff directly isolates
	// Complete()'s guest-upgrade conflict handling from that race.
	subject := "sub-guest-email-conflict"
	email := takenEmail
	raw, hash, err := generateToken()
	if err != nil {
		t.Fatalf("generateToken: %v", err)
	}
	rec := &model.OAuthLoginCode{
		CodeHash: hash, Kind: model.OAuthLoginCodeKindRegistration,
		Subject: &subject, Email: &email, ExpiresAt: time.Now().UTC().Add(15 * time.Minute),
	}
	if err := h.codes.Create(context.Background(), rec); err != nil {
		t.Fatalf("seed registration handoff: %v", err)
	}
	handoff := raw
	code := h.requestAndReadOTP(t, handoff, "081234500003")

	_, err = h.svc.Complete(context.Background(), CompleteGoogleInput{
		Code: handoff, Phone: "081234500003", Name: "Guest Conflict", OTP: code,
	})
	if !errors.Is(err, authapi.ErrEmailAlreadyUsed) {
		t.Fatalf("expected ErrEmailAlreadyUsed, got %v", err)
	}
	// Guest must be untouched — still a guest, not silently promoted.
	if *guest.CustomerType != model.CustomerTypeGuest {
		t.Fatalf("guest customer_type must be untouched after rejected upgrade, got %v", *guest.CustomerType)
	}
	// Review finding #4: a rejected resolve must roll back the handoff claim
	// too — the caller can retry (mis. with a corrected phone number)
	// WITHOUT redoing the whole Google consent screen. Check via the store's
	// own FindActiveByCode (not the `rec`/`raw` variables directly) — those
	// point at the LIVE object mutated during the (rolled-back) transaction,
	// not the post-rollback state of the store.
	if _, err := h.codes.FindActiveByCode(context.Background(), hashToken(raw)); err != nil {
		t.Fatalf("expected handoff code to remain usable after rolled-back Complete (review finding #4), got %v", err)
	}
}

// (c3) Guest upgrade where the guest account was deactivated by an admin →
// ErrUserInactive (review finding #7 — this branch used to skip the
// IsActive check entirely).
func TestGoogleOAuthService_Complete_GuestUpgrade_InactiveGuest_ReturnsErrUserInactive(t *testing.T) {
	guestType := model.CustomerTypeGuest
	guest := &model.User{
		ID: uuid.New(), Phone: strp("6281234500004"), Name: "Guest Nonaktif",
		UserType: model.UserTypeCustomer, CustomerType: &guestType, IsActive: false,
	}

	exch := &fakeGoogleExchanger{profile: &oauth.GoogleProfile{
		Subject: "sub-guest-inactive", Email: "guest-inactive@example.com", EmailVerified: true, Name: "Guest Nonaktif",
	}}
	h := newTestHarness(t, exch, OTPConfig{})
	h.users.index(guest)

	handoff, err := h.svc.HandleCallback(context.Background(), "any-code", "")
	if err != nil {
		t.Fatalf("HandleCallback: unexpected err: %v", err)
	}
	code := h.requestAndReadOTP(t, handoff, "081234500004")

	_, err = h.svc.Complete(context.Background(), CompleteGoogleInput{
		Code: handoff, Phone: "081234500004", OTP: code,
	})
	if !errors.Is(err, authapi.ErrUserInactive) {
		t.Fatalf("expected ErrUserInactive, got %v", err)
	}
}

// (d) Invalid/expired handoff code → ErrOAuthCodeInvalid.
func TestGoogleOAuthService_Exchange_InvalidCode_ReturnsErrOAuthCodeInvalid(t *testing.T) {
	h := newTestHarness(t, &fakeGoogleExchanger{}, OTPConfig{})

	if _, err := h.svc.Exchange(context.Background(), "never-issued-code"); !errors.Is(err, authapi.ErrOAuthCodeInvalid) {
		t.Fatalf("expected ErrOAuthCodeInvalid for unknown code, got %v", err)
	}

	// Expired code.
	expiredHash := hashToken("expired-raw-code")
	h.codes.byHash[expiredHash] = &model.OAuthLoginCode{
		ID: uuid.New(), CodeHash: expiredHash, Kind: model.OAuthLoginCodeKindSession,
		UserID: uuidPtr(uuid.New()), ExpiresAt: time.Now().UTC().Add(-time.Minute),
	}
	if _, err := h.svc.Exchange(context.Background(), "expired-raw-code"); !errors.Is(err, authapi.ErrOAuthCodeInvalid) {
		t.Fatalf("expected ErrOAuthCodeInvalid for expired code, got %v", err)
	}
}

// (e) Verified email matches an existing STAFF account → ErrOAuthStaffNotAllowed
// (§10: staff wajib login email+password, tidak boleh via Google).
func TestGoogleOAuthService_HandleCallback_EmailMatchesStaff_ReturnsErrOAuthStaffNotAllowed(t *testing.T) {
	staffEmail := "staff@rajakuprinting.id"
	staff := &model.User{
		ID: uuid.New(), Phone: strp("6281200000099"), Name: "Staff Verifikasi",
		Email: &staffEmail, UserType: model.UserTypeStaff, IsActive: true,
	}

	exch := &fakeGoogleExchanger{profile: &oauth.GoogleProfile{
		Subject: "sub-staff-attempt", Email: staffEmail, EmailVerified: true, Name: "Staff Verifikasi",
	}}
	h := newTestHarness(t, exch, OTPConfig{})
	h.users.index(staff)

	_, err := h.svc.HandleCallback(context.Background(), "any-code", "")
	if !errors.Is(err, authapi.ErrOAuthStaffNotAllowed) {
		t.Fatalf("expected ErrOAuthStaffNotAllowed, got %v", err)
	}
}

// (f) Phone number in the completion form already belongs to a REGISTERED
// customer → ErrPhoneAlreadyUsed (phone is the unique matching key, §11).
// Note this is only revealed AFTER a valid OTP (anti-enumeration — RequestOTP
// itself never leaks phone status, per spec A3 point 7).
func TestGoogleOAuthService_Complete_PhoneBelongsToRegisteredCustomer_ReturnsErrPhoneAlreadyUsed(t *testing.T) {
	registeredType := model.CustomerTypeRegistered
	other := &model.User{
		ID: uuid.New(), Phone: strp("6281234599999"), Name: "Sudah Terdaftar",
		UserType: model.UserTypeCustomer, CustomerType: &registeredType, IsActive: true,
	}

	exch := &fakeGoogleExchanger{profile: &oauth.GoogleProfile{
		Subject: "sub-conflict-phone", Email: "freshgoogle@example.com", EmailVerified: true, Name: "Fresh Google",
	}}
	h := newTestHarness(t, exch, OTPConfig{})
	h.users.index(other)

	handoff, err := h.svc.HandleCallback(context.Background(), "any-code", "")
	if err != nil {
		t.Fatalf("HandleCallback: unexpected err: %v", err)
	}
	code := h.requestAndReadOTP(t, handoff, "081234599999")

	_, err = h.svc.Complete(context.Background(), CompleteGoogleInput{
		Code: handoff, Phone: "081234599999", OTP: code,
	})
	if !errors.Is(err, authapi.ErrPhoneAlreadyUsed) {
		t.Fatalf("expected ErrPhoneAlreadyUsed, got %v", err)
	}
}

// (g) Google reports an unverified email for a brand-new registration →
// ErrOAuthEmailUnverified (review finding #6 — email squatting defense).
func TestGoogleOAuthService_HandleCallback_NewRegistration_UnverifiedEmail_ReturnsErrOAuthEmailUnverified(t *testing.T) {
	exch := &fakeGoogleExchanger{profile: &oauth.GoogleProfile{
		Subject: "sub-unverified-1", Email: "maybe-fake@example.com", EmailVerified: false, Name: "Unverified Person",
	}}
	h := newTestHarness(t, exch, OTPConfig{})

	_, err := h.svc.HandleCallback(context.Background(), "any-code", "")
	if !errors.Is(err, authapi.ErrOAuthEmailUnverified) {
		t.Fatalf("expected ErrOAuthEmailUnverified, got %v", err)
	}
}

// ---------- OTP-specific tests ----------

// (h) Wrong OTP guess increments attempts and reports how many attempts
// remain; the correct code afterwards still works (attempts isn't exhausted
// yet).
func TestGoogleOAuthService_Complete_WrongOTP_IncrementsAttempts(t *testing.T) {
	exch := &fakeGoogleExchanger{profile: &oauth.GoogleProfile{
		Subject: "sub-wrong-otp", Email: "wrongotp@example.com", EmailVerified: true, Name: "Wrong OTP",
	}}
	h := newTestHarness(t, exch, OTPConfig{TTL: 5 * time.Minute, MaxAttempts: 3, ResendCooldown: time.Minute, CodeLength: 6})
	// Business rule: RequestOTP only sends anything for an ALREADY-claimed
	// phone — seed a colliding owner so this OTP-mechanics test still gets a
	// real challenge to exercise.
	seedUserAtPhone(h, "6281234511111")

	handoff, err := h.svc.HandleCallback(context.Background(), "any-code", "")
	if err != nil {
		t.Fatalf("HandleCallback: unexpected err: %v", err)
	}
	correctCode := h.requestAndReadOTP(t, handoff, "081234511111")

	_, err = h.svc.Complete(context.Background(), CompleteGoogleInput{
		Code: handoff, Phone: "081234511111", OTP: "000000",
	})
	var invalidErr *authapi.OTPInvalidError
	if !errors.As(err, &invalidErr) {
		t.Fatalf("expected *authapi.OTPInvalidError, got %v", err)
	}
	if invalidErr.AttemptsLeft != 2 {
		t.Fatalf("expected 2 attempts left after 1 wrong guess (max 3), got %d", invalidErr.AttemptsLeft)
	}
	if !errors.Is(err, authapi.ErrOTPInvalid) {
		t.Fatalf("expected errors.Is match against ErrOTPInvalid, got %v", err)
	}

	// Correct code still works afterwards.
	completed, err := h.svc.Complete(context.Background(), CompleteGoogleInput{
		Code: handoff, Phone: "081234511111", OTP: correctCode,
	})
	if err != nil {
		t.Fatalf("Complete with correct code: unexpected err: %v", err)
	}
	if completed.Status != "session" {
		t.Fatalf("expected status=session, got %q", completed.Status)
	}
}

// (i) OTP challenge exhausted its attempts → ErrOTPTooManyAttempts, even
// with the eventually-correct code.
func TestGoogleOAuthService_Complete_OTPMaxAttemptsExceeded(t *testing.T) {
	exch := &fakeGoogleExchanger{profile: &oauth.GoogleProfile{
		Subject: "sub-max-attempts", Email: "maxattempts@example.com", EmailVerified: true, Name: "Max Attempts",
	}}
	h := newTestHarness(t, exch, OTPConfig{TTL: 5 * time.Minute, MaxAttempts: 2, ResendCooldown: time.Minute, CodeLength: 6})
	seedUserAtPhone(h, "6281234522222")

	handoff, err := h.svc.HandleCallback(context.Background(), "any-code", "")
	if err != nil {
		t.Fatalf("HandleCallback: unexpected err: %v", err)
	}
	correctCode := h.requestAndReadOTP(t, handoff, "081234522222")

	for i := 0; i < 2; i++ {
		_, err = h.svc.Complete(context.Background(), CompleteGoogleInput{
			Code: handoff, Phone: "081234522222", OTP: "000000",
		})
		if !errors.Is(err, authapi.ErrOTPInvalid) {
			t.Fatalf("attempt %d: expected ErrOTPInvalid, got %v", i, err)
		}
	}

	_, err = h.svc.Complete(context.Background(), CompleteGoogleInput{
		Code: handoff, Phone: "081234522222", OTP: correctCode,
	})
	if !errors.Is(err, authapi.ErrOTPTooManyAttempts) {
		t.Fatalf("expected ErrOTPTooManyAttempts even with correct code once exhausted, got %v", err)
	}
}

// (j) OTP challenge has expired (or was never requested) → ErrOTPExpired.
func TestGoogleOAuthService_Complete_OTPExpired(t *testing.T) {
	exch := &fakeGoogleExchanger{profile: &oauth.GoogleProfile{
		Subject: "sub-expired-otp", Email: "expiredotp@example.com", EmailVerified: true, Name: "Expired OTP",
	}}
	h := newTestHarness(t, exch, OTPConfig{})
	// Phone must already be claimed, otherwise Complete() would treat it as
	// free and never even look for a challenge (business rule).
	seedUserAtPhone(h, "6281234533333")

	handoff, err := h.svc.HandleCallback(context.Background(), "any-code", "")
	if err != nil {
		t.Fatalf("HandleCallback: unexpected err: %v", err)
	}

	// Never called RequestOTP — no challenge exists at all.
	_, err = h.svc.Complete(context.Background(), CompleteGoogleInput{
		Code: handoff, Phone: "081234533333", OTP: "123456",
	})
	if !errors.Is(err, authapi.ErrOTPExpired) {
		t.Fatalf("expected ErrOTPExpired when no challenge was ever requested, got %v", err)
	}
}

// (k) Requesting a second OTP before the resend cooldown elapses →
// ErrOTPCooldown, with the exact remaining seconds attached.
func TestGoogleOAuthService_RequestOTP_ResendCooldown(t *testing.T) {
	exch := &fakeGoogleExchanger{profile: &oauth.GoogleProfile{
		Subject: "sub-cooldown", Email: "cooldown@example.com", EmailVerified: true, Name: "Cooldown",
	}}
	h := newTestHarness(t, exch, OTPConfig{TTL: 5 * time.Minute, MaxAttempts: 5, ResendCooldown: time.Hour, CodeLength: 6})
	seedUserAtPhone(h, "6281234544444")

	handoff, err := h.svc.HandleCallback(context.Background(), "any-code", "")
	if err != nil {
		t.Fatalf("HandleCallback: unexpected err: %v", err)
	}
	h.requestAndReadOTP(t, handoff, "081234544444")

	_, err = h.svc.RequestOTP(context.Background(), RequestOTPInput{Code: handoff, Phone: "081234544444"})
	var cooldownErr *authapi.OTPCooldownError
	if !errors.As(err, &cooldownErr) {
		t.Fatalf("expected *authapi.OTPCooldownError, got %v", err)
	}
	if cooldownErr.ResendAvailableIn <= 0 || cooldownErr.ResendAvailableIn > 3600 {
		t.Fatalf("expected resend_available_in within (0, 3600], got %d", cooldownErr.ResendAvailableIn)
	}
	if !errors.Is(err, authapi.ErrOTPCooldown) {
		t.Fatalf("expected errors.Is match against ErrOTPCooldown, got %v", err)
	}
}

// (l) An OTP code correctly guessed for handoff A must NOT verify against
// handoff B, even for the identical phone number — the OTP is scoped to one
// specific registration handoff (migration 000013 FK).
func TestGoogleOAuthService_Complete_OTPScopedToItsOwnHandoff_RejectedForDifferentHandoff(t *testing.T) {
	exchA := &fakeGoogleExchanger{profile: &oauth.GoogleProfile{
		Subject: "sub-handoff-a", Email: "handoffa@example.com", EmailVerified: true, Name: "Handoff A",
	}}
	h := newTestHarness(t, exchA, OTPConfig{})
	seedUserAtPhone(h, "6281234555555")

	handoffA, err := h.svc.HandleCallback(context.Background(), "any-code", "")
	if err != nil {
		t.Fatalf("HandleCallback A: unexpected err: %v", err)
	}
	codeA := h.requestAndReadOTP(t, handoffA, "081234555555")

	// Same service/store, second independent registration handoff (different
	// Google subject/email), SAME phone number.
	h.svc.google = &fakeGoogleExchanger{profile: &oauth.GoogleProfile{
		Subject: "sub-handoff-b", Email: "handoffb@example.com", EmailVerified: true, Name: "Handoff B",
	}}
	handoffB, err := h.svc.HandleCallback(context.Background(), "any-code", "")
	if err != nil {
		t.Fatalf("HandleCallback B: unexpected err: %v", err)
	}

	// Try to complete B using the OTP code that was actually sent for A.
	_, err = h.svc.Complete(context.Background(), CompleteGoogleInput{
		Code: handoffB, Phone: "081234555555", OTP: codeA,
	})
	if !errors.Is(err, authapi.ErrOTPExpired) {
		t.Fatalf("expected ErrOTPExpired (no active challenge under handoff B), got %v", err)
	}
}

// ---------- security review follow-up tests ----------

// (m) Concurrency: many simultaneous wrong guesses against the SAME
// challenge must never let the attempts ceiling be bypassed by a stale read
// (review finding #1 — the exact TOCTOU this closes: the pre-fix code
// checked `pv.Attempts >= MaxAttempts` from a plain SELECT done moments
// earlier, then incremented separately, so a burst of concurrent requests
// could all read a stale low count and pass the gate). Every wrong guess
// still only gets exactly MaxAttempts total ErrOTPInvalid outcomes;
// everything beyond that gets ErrOTPTooManyAttempts. Run with `-race` to
// also prove the gate itself has no data race (fakeOTPStore is mutex-guarded
// specifically so this test is meaningful).
func TestGoogleOAuthService_Complete_ConcurrentWrongGuesses_AttemptsCeilingNeverBypassed(t *testing.T) {
	const maxAttempts = 5
	const concurrency = 100

	exch := &fakeGoogleExchanger{profile: &oauth.GoogleProfile{
		Subject: "sub-concurrent", Email: "concurrent@example.com", EmailVerified: true, Name: "Concurrent",
	}}
	h := newTestHarness(t, exch, OTPConfig{TTL: 5 * time.Minute, MaxAttempts: maxAttempts, ResendCooldown: time.Minute, CodeLength: 6})
	seedUserAtPhone(h, "6281234566666")

	handoff, err := h.svc.HandleCallback(context.Background(), "any-code", "")
	if err != nil {
		t.Fatalf("HandleCallback: unexpected err: %v", err)
	}
	h.requestAndReadOTP(t, handoff, "081234566666")

	var wg sync.WaitGroup
	var mu sync.Mutex
	var invalidCount, tooManyCount, otherCount int
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := h.svc.Complete(context.Background(), CompleteGoogleInput{
				Code: handoff, Phone: "081234566666", OTP: "000000",
			})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case errors.Is(err, authapi.ErrOTPTooManyAttempts):
				tooManyCount++
			case errors.Is(err, authapi.ErrOTPInvalid):
				invalidCount++
			default:
				otherCount++
			}
		}()
	}
	wg.Wait()

	if otherCount != 0 {
		t.Fatalf("expected every call to resolve to ErrOTPInvalid or ErrOTPTooManyAttempts only, got %d other outcomes", otherCount)
	}
	if invalidCount != maxAttempts {
		t.Fatalf("expected exactly %d ErrOTPInvalid (one per allowed attempt — the ceiling must not be bypassed), got %d", maxAttempts, invalidCount)
	}
	if tooManyCount != concurrency-maxAttempts {
		t.Fatalf("expected %d ErrOTPTooManyAttempts, got %d", concurrency-maxAttempts, tooManyCount)
	}
}

// (n) Cross-handoff, per-phone ISSUE cap (review finding #2): opening a
// fresh registration handoff for every request must NOT reset the per-phone
// limit — OTP_MAX_PER_PHONE_HOUR is keyed on the phone number alone,
// spanning every handoff that targets it.
func TestGoogleOAuthService_RequestOTP_PerPhoneIssueCap_CrossHandoff(t *testing.T) {
	const maxPerPhoneHour = 3
	const victimPhone = "081234577777"
	h := newTestHarness(t, &fakeGoogleExchanger{}, OTPConfig{
		MaxPerPhoneHour: maxPerPhoneHour, MaxFailedPerPhoneHour: 1000,
	})
	seedUserAtPhone(h, "6281234577777")

	for i := 0; i < maxPerPhoneHour; i++ {
		h.svc.google = &fakeGoogleExchanger{profile: &oauth.GoogleProfile{
			Subject: fmt.Sprintf("sub-issue-cap-%d", i), Email: fmt.Sprintf("issuecap%d@example.com", i), EmailVerified: true, Name: "Issue Cap",
		}}
		handoff, err := h.svc.HandleCallback(context.Background(), "any-code", "")
		if err != nil {
			t.Fatalf("HandleCallback %d: unexpected err: %v", i, err)
		}
		if _, err := h.svc.RequestOTP(context.Background(), RequestOTPInput{Code: handoff, Phone: victimPhone}); err != nil {
			t.Fatalf("RequestOTP %d: unexpected err: %v", i, err)
		}
	}

	// One more handoff for the SAME phone — a brand-new handoff with no
	// per-handoff cooldown history of its own, yet the cross-handoff cap must
	// still reject it.
	h.svc.google = &fakeGoogleExchanger{profile: &oauth.GoogleProfile{
		Subject: "sub-issue-cap-over", Email: "issuecapover@example.com", EmailVerified: true, Name: "Issue Cap",
	}}
	handoff, err := h.svc.HandleCallback(context.Background(), "any-code", "")
	if err != nil {
		t.Fatalf("HandleCallback (over cap): unexpected err: %v", err)
	}
	_, err = h.svc.RequestOTP(context.Background(), RequestOTPInput{Code: handoff, Phone: victimPhone})
	var cooldownErr *authapi.OTPCooldownError
	if !errors.As(err, &cooldownErr) {
		t.Fatalf("expected *authapi.OTPCooldownError once the per-phone issue cap is hit, got %v", err)
	}
}

// (o) Cross-handoff, per-phone FAILED-ATTEMPT cap (review finding #2):
// opening several handoffs for the same victim phone must not multiply the
// total guessing budget available to an attacker beyond
// OTP_MAX_FAILED_PER_PHONE_HOUR, even though each handoff's own MaxAttempts
// ceiling is nowhere near exhausted.
func TestGoogleOAuthService_Complete_PerPhoneFailedCap_CrossHandoff(t *testing.T) {
	const maxFailedPerPhoneHour = 4
	const victimPhone = "081234588888"
	h := newTestHarness(t, &fakeGoogleExchanger{}, OTPConfig{
		MaxAttempts: 10, MaxPerPhoneHour: 1000, MaxFailedPerPhoneHour: maxFailedPerPhoneHour,
	})
	seedUserAtPhone(h, "6281234588888")

	// Two separate handoffs, two wrong guesses each — 4 verify calls logged
	// against the phone in total, hitting the cap exactly.
	var handoffs []string
	for i := 0; i < 2; i++ {
		h.svc.google = &fakeGoogleExchanger{profile: &oauth.GoogleProfile{
			Subject: fmt.Sprintf("sub-failed-cap-%d", i), Email: fmt.Sprintf("failedcap%d@example.com", i), EmailVerified: true, Name: "Failed Cap",
		}}
		handoff, err := h.svc.HandleCallback(context.Background(), "any-code", "")
		if err != nil {
			t.Fatalf("HandleCallback %d: unexpected err: %v", i, err)
		}
		h.requestAndReadOTP(t, handoff, victimPhone)
		handoffs = append(handoffs, handoff)
	}
	for hi, handoff := range handoffs {
		for j := 0; j < 2; j++ {
			_, err := h.svc.Complete(context.Background(), CompleteGoogleInput{
				Code: handoff, Phone: victimPhone, OTP: "000000",
			})
			if !errors.Is(err, authapi.ErrOTPInvalid) {
				t.Fatalf("handoff %d attempt %d: expected ErrOTPInvalid, got %v", hi, j, err)
			}
		}
	}

	// A THIRD handoff for the same phone — its OWN MaxAttempts budget (10) is
	// nowhere near exhausted, but the cross-handoff cap must still bite.
	h.svc.google = &fakeGoogleExchanger{profile: &oauth.GoogleProfile{
		Subject: "sub-failed-cap-3rd", Email: "failedcap3rd@example.com", EmailVerified: true, Name: "Failed Cap",
	}}
	handoff3, err := h.svc.HandleCallback(context.Background(), "any-code", "")
	if err != nil {
		t.Fatalf("HandleCallback (3rd): unexpected err: %v", err)
	}
	h.requestAndReadOTP(t, handoff3, victimPhone)
	_, err = h.svc.Complete(context.Background(), CompleteGoogleInput{
		Code: handoff3, Phone: victimPhone, OTP: "000000",
	})
	if !errors.Is(err, authapi.ErrOTPTooManyAttempts) {
		t.Fatalf("expected ErrOTPTooManyAttempts once the cross-handoff failed-attempt cap is hit, got %v", err)
	}
}

// (p) Session handoff issued while the user was active, then the account is
// deactivated by an admin BEFORE the frontend calls Exchange (mis. anywhere
// in the up-to-2-minute googleSessionCodeTTL window) → ErrUserInactive
// (review finding #9 — exchangeSessionCode used to skip this re-check;
// every other resolution branch in this file already had it).
func TestGoogleOAuthService_ExchangeSessionCode_UserDeactivatedAfterHandoffIssued_ReturnsErrUserInactive(t *testing.T) {
	provider := "google"
	subject := "sub-deactivated-after-handoff"
	custType := model.CustomerTypeRegistered
	existing := &model.User{
		ID: uuid.New(), Phone: strp("6281234599990"), Name: "Soon Deactivated",
		UserType: model.UserTypeCustomer, CustomerType: &custType,
		OAuthProvider: &provider, OAuthSubject: &subject, IsActive: true,
	}
	exch := &fakeGoogleExchanger{profile: &oauth.GoogleProfile{
		Subject: subject, Email: "deactivated@example.com", EmailVerified: true, Name: "Soon Deactivated",
	}}
	h := newTestHarness(t, exch, OTPConfig{})
	h.users.index(existing)

	handoff, err := h.svc.HandleCallback(context.Background(), "any-code", "")
	if err != nil {
		t.Fatalf("HandleCallback: unexpected err: %v", err)
	}

	// Admin deactivates the account in the window between HandleCallback and
	// Exchange.
	existing.IsActive = false

	if _, err := h.svc.Exchange(context.Background(), handoff); !errors.Is(err, authapi.ErrUserInactive) {
		t.Fatalf("expected ErrUserInactive, got %v", err)
	}
}

func uuidPtr(id uuid.UUID) *uuid.UUID { return &id }

// strp is a small test helper — model.User.Phone is *string (migration
// 000017, phone now nullable), so every literal in this file needs an
// address-of somewhere; this keeps call sites terse.
func strp(s string) *string { return &s }

// seedUserAtPhone inserts a bare, UNVERIFIED user already occupying
// `normalizedPhone` directly into the harness's fake store — used by OTP
// mechanics tests (attempts/cooldown/expiry/concurrency/cross-handoff caps)
// that, under the business rule (OTP only required when a phone is ALREADY
// claimed), need a pre-existing owner for RequestOTP to actually issue a
// challenge at all. Defaults to an active guest customer (the most common
// "taken" case); pass a different CustomerType/UserType via the returned
// pointer if a test needs to seed staff/registered instead.
func seedUserAtPhone(h *testHarness, normalizedPhone string) *model.User {
	guestType := model.CustomerTypeGuest
	u := &model.User{
		ID: uuid.New(), Phone: strp(normalizedPhone), Name: "",
		UserType: model.UserTypeCustomer, CustomerType: &guestType, IsActive: true,
	}
	h.users.index(u)
	return u
}

// ---------- security review follow-up tests (finding #1, #3, #5) ----------

// (q) Review finding #1 (issue-cap TOCTOU): many concurrent RequestOTP calls,
// each carrying its OWN fresh registration handoff (so the per-handoff resend
// cooldown never engages) but the SAME target phone number, must never let
// more than OTP_MAX_PER_PHONE_HOUR challenges actually get created. Before
// the advisory-lock transaction fix, checkPerPhoneIssueCap's COUNT and
// otps.Create() were two separate statements — under this exact concurrency
// pattern every goroutine could read the same low count and all pass the
// gate. fakePhoneLockTxRunner gives per-phone serialization a real mutex
// enforces (see its doc), so this test is only meaningful because that fake
// exists — without it (or without the production advisory lock it stands in
// for), this assertion would be flaky/fail under load.
func TestGoogleOAuthService_RequestOTP_ConcurrentRequests_PerPhoneIssueCapNeverBypassed(t *testing.T) {
	const maxPerPhoneHour = 5
	const concurrency = 40
	const victimPhone = "081234599222"

	h := newTestHarness(t, &fakeGoogleExchanger{}, OTPConfig{
		MaxPerPhoneHour: maxPerPhoneHour, MaxFailedPerPhoneHour: 100000,
	})
	seedUserAtPhone(h, "6281234599222")

	handoffs := make([]string, concurrency)
	for i := 0; i < concurrency; i++ {
		h.svc.google = &fakeGoogleExchanger{profile: &oauth.GoogleProfile{
			Subject:       fmt.Sprintf("sub-concurrent-issue-%d", i),
			Email:         fmt.Sprintf("concurrentissue%d@example.com", i),
			EmailVerified: true, Name: "Concurrent Issue",
		}}
		handoff, err := h.svc.HandleCallback(context.Background(), "any-code", "")
		if err != nil {
			t.Fatalf("HandleCallback %d: unexpected err: %v", i, err)
		}
		handoffs[i] = handoff
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var successCount, cooldownCount, otherCount int
	for _, handoff := range handoffs {
		wg.Add(1)
		go func(handoff string) {
			defer wg.Done()
			_, err := h.svc.RequestOTP(context.Background(), RequestOTPInput{Code: handoff, Phone: victimPhone})
			mu.Lock()
			defer mu.Unlock()
			var cooldownErr *authapi.OTPCooldownError
			switch {
			case err == nil:
				successCount++
			case errors.As(err, &cooldownErr):
				cooldownCount++
			default:
				otherCount++
			}
		}(handoff)
	}
	wg.Wait()

	if otherCount != 0 {
		t.Fatalf("expected every call to resolve to success or *authapi.OTPCooldownError only, got %d other outcomes", otherCount)
	}
	if successCount != maxPerPhoneHour {
		t.Fatalf("expected exactly %d successful OTP issues under the per-phone cap despite %d concurrent requests for the same phone (review finding #1 — TOCTOU), got %d",
			maxPerPhoneHour, concurrency, successCount)
	}
	if cooldownCount != concurrency-maxPerPhoneHour {
		t.Fatalf("expected %d rejections once the per-phone issue cap is hit, got %d", concurrency-maxPerPhoneHour, cooldownCount)
	}
}

// (r) Review finding #3: exchangeRegistrationCode claims the OLD handoff code
// and mints the NEW one atomically. If the reissue half fails (mis. a
// transient DB error), the claim of the old code must roll back too — the
// caller can retry Exchange with the SAME (still-unclaimed) code, instead of
// the old code being permanently burned with no replacement anyone can use.
func TestGoogleOAuthService_ExchangeRegistrationCode_ReissueFails_ClaimRolledBack(t *testing.T) {
	exch := &fakeGoogleExchanger{profile: &oauth.GoogleProfile{
		Subject: "sub-reissue-fail", Email: "reissuefail@example.com", EmailVerified: true, Name: "Reissue Fail",
	}}
	h := newTestHarness(t, exch, OTPConfig{})

	handoff, err := h.svc.HandleCallback(context.Background(), "any-code", "")
	if err != nil {
		t.Fatalf("HandleCallback: unexpected err: %v", err)
	}

	// Force the SECOND Create call — the rotated replacement code minted
	// inside exchangeRegistrationCode — to fail.
	sentinel := errors.New("boom: reissue create failed")
	h.codes.createErr = sentinel

	if _, err := h.svc.Exchange(context.Background(), handoff); !errors.Is(err, sentinel) {
		t.Fatalf("expected wrapped sentinel error from failed reissue, got %v", err)
	}

	// Review finding #3: a failed reissue must roll back the claim too —
	// the original handoff must remain exactly as usable as it was before
	// this failed Exchange call.
	h.codes.createErr = nil
	out, err := h.svc.Exchange(context.Background(), handoff)
	if err != nil {
		t.Fatalf("expected original handoff to remain usable after rolled-back Exchange, got %v", err)
	}
	if out.Status != "need_phone" {
		t.Fatalf("expected status=need_phone on retried exchange, got %q", out.Status)
	}
	if out.Code == "" || out.Code == handoff {
		t.Fatalf("expected a freshly rotated code on the successful retry, got %q", out.Code)
	}
}

// (s) Review finding #5 (a+b): a wrong-guess verify's gated increment must
// distinguish "row no longer active" from "attempts ceiling hit", and the
// WHERE clause backing it must itself re-check consumed_at/expires_at — not
// just rely on the caller's earlier FindActive. Simulated here via
// fakeOTPStore.beforeIncrement: a concurrent write (a fresh RequestOTP
// cancelling the previous pending challenge) lands in the exact window
// between verifyOTP's FindActive (which still saw the challenge as active)
// and the gated increment that follows. Before the fix, the increment
// statement only checked `attempts < maxAttempts` and would have happily
// incremented (and compared against) a challenge that's no longer usable,
// then reported a misleading ErrOTPTooManyAttempts once attempts everntually
// crossed the ceiling instead of the accurate ErrOTPExpired.
func TestGoogleOAuthService_Complete_ChallengeCancelledBetweenFindActiveAndIncrement_ReturnsErrOTPExpired(t *testing.T) {
	exch := &fakeGoogleExchanger{profile: &oauth.GoogleProfile{
		Subject: "sub-cancel-race", Email: "cancelrace@example.com", EmailVerified: true, Name: "Cancel Race",
	}}
	h := newTestHarness(t, exch, OTPConfig{})
	seedUserAtPhone(h, "6281234599333")

	handoff, err := h.svc.HandleCallback(context.Background(), "any-code", "")
	if err != nil {
		t.Fatalf("HandleCallback: unexpected err: %v", err)
	}
	code := h.requestAndReadOTP(t, handoff, "081234599333")

	rec, err := h.codes.FindActiveByCode(context.Background(), hashToken(handoff))
	if err != nil {
		t.Fatalf("lookup handoff record: %v", err)
	}

	// Fires exactly once, right before the gated increment runs — simulates
	// a concurrent RequestOTP for the same handoff cancelling this challenge
	// in the race window review finding #5b closes.
	fired := false
	h.otps.beforeIncrement = func() {
		if fired {
			return
		}
		fired = true
		if err := h.otps.CancelPendingForHandoff(context.Background(), rec.ID); err != nil {
			t.Fatalf("simulate concurrent cancel: %v", err)
		}
	}

	_, err = h.svc.Complete(context.Background(), CompleteGoogleInput{
		Code: handoff, Phone: "081234599333", OTP: code,
	})
	if !errors.Is(err, authapi.ErrOTPExpired) {
		t.Fatalf("expected ErrOTPExpired for a challenge cancelled concurrently mid-verify (review finding #5), got %v", err)
	}
}
