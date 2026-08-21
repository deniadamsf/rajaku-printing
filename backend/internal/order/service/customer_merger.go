package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/rajaku-printing/backend/internal/order/orderapi"
	"github.com/rajaku-printing/backend/internal/order/repository"
)

// customerMergeStore narrows OrderStore to the single operation CustomerMerger
// needs — kept separate from OrderStore so a tx-scoped CustomerMerger
// (constructed fresh per phone-claim absorption) doesn't drag in catalog/
// customer wiring that ReassignCustomer never touches (§22 satu tanggung
// jawab).
type customerMergeStore interface {
	ReassignCustomer(ctx context.Context, fromCustomerID, toCustomerID uuid.UUID) ([]uuid.UUID, error)
}

// Compile-time assertion — concrete repo satisfies the narrowed contract.
var _ customerMergeStore = (*repository.OrderRepository)(nil)

// CustomerMerger is a thin, tx-scoped implementation of orderapi.CustomerMerger
// — deliberately a SEPARATE type from Service, not a method on it: the
// phone-claim absorption transaction (auth module) only ever needs this one
// operation, so wiring it doesn't require the full Service's catalog/customer
// dependencies. Constructed straight from a *gorm.DB — in production that's
// the *gorm.DB tx the auth module's phoneClaimTx is already running inside,
// so the reassignment commits/rolls back atomically with releasing the
// guest's phone number and marking the OTP consumed.
type CustomerMerger struct {
	orders customerMergeStore
}

// NewCustomerMerger builds a CustomerMerger bound to db (ordinary handle or,
// as router.go wires it, a live transaction). Service layer only — never
// touches GORM directly beyond constructing the repository (§22).
func NewCustomerMerger(db *gorm.DB) *CustomerMerger {
	return &CustomerMerger{orders: repository.NewOrderRepository(db)}
}

// Compile-time assertion — CustomerMerger satisfies the cross-module contract.
var _ orderapi.CustomerMerger = (*CustomerMerger)(nil)

// ReassignCustomer moves every order owned by fromCustomerID to
// toCustomerID and returns the IDs of every order that moved. See
// orderapi.CustomerMerger's doc for the calling context (§11 phone-claim
// absorbing a guest identity). The caller (auth module's phone-claim flow)
// is the one that persists the durable customer_merges audit row — this log
// line is a secondary breadcrumb only, not the record of truth.
func (m *CustomerMerger) ReassignCustomer(ctx context.Context, fromCustomerID, toCustomerID uuid.UUID) ([]uuid.UUID, error) {
	ids, err := m.orders.ReassignCustomer(ctx, fromCustomerID, toCustomerID)
	if err != nil {
		return nil, fmt.Errorf("reassign customer orders from %s to %s: %w", fromCustomerID, toCustomerID, err)
	}
	if len(ids) > 0 {
		log.Ctx(ctx).Info().
			Str("from_customer_id", fromCustomerID.String()).
			Str("to_customer_id", toCustomerID.String()).
			Int("orders_moved", len(ids)).
			Msg("phone claim: absorbed guest's order history into registered account")
	}
	return ids, nil
}
