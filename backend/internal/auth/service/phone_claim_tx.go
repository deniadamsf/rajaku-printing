package service

import (
	"context"

	"gorm.io/gorm"

	"github.com/rajaku-printing/backend/internal/auth/repository"
	"github.com/rajaku-printing/backend/internal/order/orderapi"
)

// phoneClaimTx bundles the narrowed stores AS THEY EXIST INSIDE the single DB
// transaction PhoneClaimService.Claim wraps its writes in — same pattern as
// googleOAuthCompletionTx, just without a `Codes` store (this flow has no
// OAuth handoff code to claim; the caller is already authenticated).
//
// Orders is the order module's cross-module port (§22 no cross-module
// internal imports — orderapi is the only order package auth may import).
// It's here, bound to the SAME tx, so that when a guest identity is absorbed
// (see phone_claim_service.go's claimOther), the guest's order history moves
// to the caller atomically with releasing the phone and consuming the OTP —
// no window where the phone has moved but the history hasn't.
//
// CustomerMerges writes the durable customer_merges audit row (§ migration
// 000019) — also bound to the SAME tx, so the audit row and the order
// reassignment/tombstone it describes can never separate: either both
// commit or both roll back together.
type phoneClaimTx struct {
	Users          googleOAuthUserStore
	OTPs           googleOTPStore
	Orders         orderapi.CustomerMerger
	CustomerMerges customerMergeAuditStore
}

// phoneClaimTxRunner runs fn inside one DB transaction — needed because
// Claim() writes to `users` (release the old owner's phone, set the caller's,
// tombstone the absorbed guest row), `phone_verifications` (mark the
// challenge consumed), and — when absorbing a guest — `orders` (reassign
// customer_id), which no single repository can do atomically on its own
// (§22 "repository akses DB only").
type phoneClaimTxRunner interface {
	RunInTx(ctx context.Context, fn func(tx phoneClaimTx) error) error
}

// gormPhoneClaimTxRunner is the production phoneClaimTxRunner.
type gormPhoneClaimTxRunner struct {
	db *gorm.DB
	// newOrderMerger builds the order module's tx-scoped CustomerMerger from
	// the LIVE transaction handle — supplied by the composition root
	// (router.go), the only place allowed to import both auth and order
	// packages. A nil factory is a wiring bug, not a silently-skipped merge
	// (§22 no-silent-stub) — see newGormPhoneClaimTxRunner.
	newOrderMerger func(tx *gorm.DB) orderapi.CustomerMerger
}

// newGormPhoneClaimTxRunner requires a non-nil newOrderMerger factory — a nil
// factory would mean guest order history silently never gets reassigned
// during absorption, which is exactly the bug this whole feature exists to
// fix. Panicking here is startup-time config validation (§22 panic only for
// invalid config at startup), not an in-request failure mode.
func newGormPhoneClaimTxRunner(db *gorm.DB, newOrderMerger func(tx *gorm.DB) orderapi.CustomerMerger) *gormPhoneClaimTxRunner {
	if newOrderMerger == nil {
		panic("auth/service: newGormPhoneClaimTxRunner requires a non-nil order CustomerMerger factory (wiring bug in router.go)")
	}
	return &gormPhoneClaimTxRunner{db: db, newOrderMerger: newOrderMerger}
}

func (r *gormPhoneClaimTxRunner) RunInTx(ctx context.Context, fn func(tx phoneClaimTx) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(phoneClaimTx{
			Users:          repository.NewUserRepository(tx),
			OTPs:           repository.NewPhoneVerificationRepository(tx),
			Orders:         r.newOrderMerger(tx),
			CustomerMerges: repository.NewCustomerMergeRepository(tx),
		})
	})
}

var _ phoneClaimTxRunner = (*gormPhoneClaimTxRunner)(nil)
