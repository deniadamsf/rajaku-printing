package service

import (
	"context"

	"gorm.io/gorm"

	"github.com/rajaku-printing/backend/internal/auth/repository"
)

// phoneClaimTx bundles the two narrowed stores AS THEY EXIST INSIDE the
// single DB transaction PhoneClaimService.Claim wraps its writes in — same
// pattern as googleOAuthCompletionTx, just without a `Codes` store (this flow
// has no OAuth handoff code to claim; the caller is already authenticated).
type phoneClaimTx struct {
	Users googleOAuthUserStore
	OTPs  googleOTPStore
}

// phoneClaimTxRunner runs fn inside one DB transaction — needed because
// Claim() writes to BOTH `users` (release the old owner's phone, set the
// caller's) and `phone_verifications` (mark the challenge consumed), which no
// single repository can do atomically on its own (§22 "repository akses DB
// only").
type phoneClaimTxRunner interface {
	RunInTx(ctx context.Context, fn func(tx phoneClaimTx) error) error
}

// gormPhoneClaimTxRunner is the production phoneClaimTxRunner.
type gormPhoneClaimTxRunner struct {
	db *gorm.DB
}

func newGormPhoneClaimTxRunner(db *gorm.DB) *gormPhoneClaimTxRunner {
	return &gormPhoneClaimTxRunner{db: db}
}

func (r *gormPhoneClaimTxRunner) RunInTx(ctx context.Context, fn func(tx phoneClaimTx) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(phoneClaimTx{
			Users: repository.NewUserRepository(tx),
			OTPs:  repository.NewPhoneVerificationRepository(tx),
		})
	})
}

var _ phoneClaimTxRunner = (*gormPhoneClaimTxRunner)(nil)
