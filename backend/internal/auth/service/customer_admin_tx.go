// customer_admin_tx.go — transaction runner for CustomerAdminService writes
// that must commit/rollback together with their customer_admin_logs audit
// row (temuan review #7/#9): block/unblock and profile updates were
// previously two separate statements, so a failed audit-log insert left the
// user row changed with NO audit trail explaining who/why — exactly the
// guarantee that table exists to provide. Pola sama phone_claim_tx.go
// (gormPhoneClaimTxRunner), dipilih karena paling sedikit merusak interface
// sempit customerAdminUserStore/customerAdminLogStore yang sudah bisa
// di-fake di test (customer_admin_service_test.go) — kode produksi cukup
// menyuntik implementasi txRunner yang berbeda dari fake, tanpa mengubah
// bentuk interface itu sendiri.
package service

import (
	"context"

	"gorm.io/gorm"

	"github.com/rajaku-printing/backend/internal/auth/repository"
)

// customerAdminTx bundles the tx-scoped narrow stores for one atomic write —
// a user field/status change PLUS the customer_admin_logs row describing it.
type customerAdminTx struct {
	Users customerAdminUserStore
	Logs  customerAdminLogStore
}

// customerAdminTxRunner runs fn inside one DB transaction.
type customerAdminTxRunner interface {
	RunInTx(ctx context.Context, fn func(tx customerAdminTx) error) error
}

// gormCustomerAdminTxRunner is the production customerAdminTxRunner.
type gormCustomerAdminTxRunner struct {
	db *gorm.DB
}

func newGormCustomerAdminTxRunner(db *gorm.DB) *gormCustomerAdminTxRunner {
	return &gormCustomerAdminTxRunner{db: db}
}

func (r *gormCustomerAdminTxRunner) RunInTx(ctx context.Context, fn func(tx customerAdminTx) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(customerAdminTx{
			Users: repository.NewUserRepository(tx),
			Logs:  repository.NewCustomerAdminLogRepository(tx),
		})
	})
}

var _ customerAdminTxRunner = (*gormCustomerAdminTxRunner)(nil)
