package service

import (
	"context"

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
