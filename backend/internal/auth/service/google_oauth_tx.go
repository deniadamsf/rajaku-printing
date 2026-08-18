package service

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/rajaku-printing/backend/internal/auth/repository"
)

// googleOAuthCompletionTx bundles the three narrowed stores AS THEY EXIST
// INSIDE the single DB transaction that Complete() wraps claim-handoff +
// resolve-user + consume-OTP + update-last-login in (review finding #4).
// Same interfaces used everywhere else in this file — the real runner below
// just binds them to a transaction handle instead of the base connection.
type googleOAuthCompletionTx struct {
	Users googleOAuthUserStore
	Codes googleOAuthCodeStore
	OTPs  googleOTPStore
}

// googleOAuthTxRunner runs fn inside one DB transaction: if fn returns a
// non-nil error every write it made through the bundled stores is rolled
// back, including a MarkUsed on the handoff code — so a rejected resolve
// (ErrPhoneAlreadyUsed / ErrEmailAlreadyUsed / ErrUserInactive) leaves the
// handoff exactly as usable as it was before Complete() was called, instead
// of burning it on a failed attempt.
type googleOAuthTxRunner interface {
	RunInTx(ctx context.Context, fn func(tx googleOAuthCompletionTx) error) error
}

// gormTxRunner is the production googleOAuthTxRunner. It is deliberately the
// ONLY place in this service package that touches *gorm.DB directly — every
// actual query still lives in internal/auth/repository (§22 "repository
// akses DB only"); this type exists purely to open the transaction BOUNDARY
// that Complete() needs across three different repositories, which none of
// those repositories can do on their own (each only knows about its own
// table).
type gormTxRunner struct {
	db *gorm.DB
}

func newGormTxRunner(db *gorm.DB) *gormTxRunner { return &gormTxRunner{db: db} }

func (r *gormTxRunner) RunInTx(ctx context.Context, fn func(tx googleOAuthCompletionTx) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(googleOAuthCompletionTx{
			Users: repository.NewUserRepository(tx),
			Codes: repository.NewOAuthLoginCodeRepository(tx),
			OTPs:  repository.NewPhoneVerificationRepository(tx),
		})
	})
}

var _ googleOAuthTxRunner = (*gormTxRunner)(nil)

// phoneLockTxRunner runs fn inside one DB transaction that starts by
// acquiring a Postgres advisory lock SCOPED TO `normalizedPhone`
// (pg_advisory_xact_lock(hashtext(phone)) — see
// repository.PhoneVerificationRepository.LockPhone) before doing anything
// else (review finding #1 — TOCTOU between a per-phone COUNT/SUM SELECT and
// the INSERT/UPDATE that follows it).
//
// The lock is transactional: Postgres releases it automatically at COMMIT or
// ROLLBACK, so it can never leak even if fn panics or returns an error. It
// serializes concurrent callers ONLY for the same phone number — unrelated
// phones proceed independently, so this is not a global bottleneck.
//
// Used from TWO distinct, INTENTIONALLY SEPARATE call sites (do not merge
// them into one bigger transaction):
//   - RequestOTP: lock → checkPerPhoneIssueCap → checkResendCooldown →
//     CancelPendingForHandoff → Create.
//   - verifyOTP: lock → checkPerPhoneFailedCap → IncrementAttemptsIfAllowed.
//     Deliberately NOT folded into googleOAuthTxRunner's Complete() tx — the
//     attempts counter this increments must survive even if Complete()'s
//     later claim+resolve+consume steps roll back (see verifyOTP's doc in
//     google_oauth_service.go).
type phoneLockTxRunner interface {
	RunInTx(ctx context.Context, normalizedPhone string, fn func(tx googleOTPStore) error) error
}

// gormPhoneLockTxRunner is the production phoneLockTxRunner. Like
// gormTxRunner, it's the only place in this service package that opens a
// transaction directly — the advisory lock acquisition itself lives in
// repository.PhoneVerificationRepository.LockPhone (§22 "repository akses DB
// only"; service code must never embed raw SQL).
type gormPhoneLockTxRunner struct {
	db *gorm.DB
}

func newGormPhoneLockTxRunner(db *gorm.DB) *gormPhoneLockTxRunner {
	return &gormPhoneLockTxRunner{db: db}
}

func (r *gormPhoneLockTxRunner) RunInTx(ctx context.Context, normalizedPhone string, fn func(tx googleOTPStore) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		otps := repository.NewPhoneVerificationRepository(tx)
		if err := otps.LockPhone(ctx, normalizedPhone); err != nil {
			return fmt.Errorf("google oauth: acquire per-phone advisory lock: %w", err)
		}
		return fn(otps)
	})
}

var _ phoneLockTxRunner = (*gormPhoneLockTxRunner)(nil)
