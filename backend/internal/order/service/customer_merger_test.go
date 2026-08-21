package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// fakeCustomerMergeStore is an in-memory stand-in for the customerMergeStore
// contract (narrowed *repository.OrderRepository) — models a tiny "orders"
// table keyed by owner so ReassignCustomer's WHERE-scoping behavior (only
// `from`'s rows move, everyone else's stay put) is actually exercised, not
// just assumed.
type fakeCustomerMergeStore struct {
	ownerByOrder map[uuid.UUID]uuid.UUID // orderID -> customerID
	err          error
}

func newFakeCustomerMergeStore() *fakeCustomerMergeStore {
	return &fakeCustomerMergeStore{ownerByOrder: map[uuid.UUID]uuid.UUID{}}
}

func (f *fakeCustomerMergeStore) ReassignCustomer(_ context.Context, fromCustomerID, toCustomerID uuid.UUID) ([]uuid.UUID, error) {
	if f.err != nil {
		return nil, f.err
	}
	var moved []uuid.UUID
	for orderID, owner := range f.ownerByOrder {
		if owner == fromCustomerID {
			f.ownerByOrder[orderID] = toCustomerID
			moved = append(moved, orderID)
		}
	}
	return moved, nil
}

// (1) Happy path — only orders owned by `from` move to `to`; orders owned by
// an unrelated third customer are left untouched, and the returned count
// matches exactly how many rows moved.
func TestCustomerMerger_ReassignCustomer_MovesOnlyFromCustomersOrders(t *testing.T) {
	store := newFakeCustomerMergeStore()
	fromID, toID, otherID := uuid.New(), uuid.New(), uuid.New()

	orderA, orderB, orderC := uuid.New(), uuid.New(), uuid.New()
	store.ownerByOrder[orderA] = fromID
	store.ownerByOrder[orderB] = fromID
	store.ownerByOrder[orderC] = otherID // belongs to a different customer entirely

	m := &CustomerMerger{orders: store}

	ids, err := m.ReassignCustomer(context.Background(), fromID, toID)
	if err != nil {
		t.Fatalf("ReassignCustomer: unexpected err: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 orders moved, got %d (%v)", len(ids), ids)
	}
	gotIDs := map[uuid.UUID]bool{ids[0]: true, ids[1]: true}
	if !gotIDs[orderA] || !gotIDs[orderB] {
		t.Fatalf("expected returned IDs to be exactly {orderA, orderB}, got %v", ids)
	}
	if gotIDs[orderC] {
		t.Fatalf("expected orderC (unrelated customer) NOT to be in the returned IDs, got %v", ids)
	}
	if store.ownerByOrder[orderA] != toID || store.ownerByOrder[orderB] != toID {
		t.Fatalf("expected orderA and orderB reassigned to %s, got %+v", toID, store.ownerByOrder)
	}
	if store.ownerByOrder[orderC] != otherID {
		t.Fatalf("expected orderC (unrelated customer) untouched, got owner %s want %s", store.ownerByOrder[orderC], otherID)
	}
}

// (2) Nothing owned by `from` → zero rows moved, no error — same "already
// free" no-op shape the auth module's rollback test relies on NOT happening
// on the happy path.
func TestCustomerMerger_ReassignCustomer_NoOrders_ReturnsZero(t *testing.T) {
	store := newFakeCustomerMergeStore()
	fromID, toID := uuid.New(), uuid.New()
	m := &CustomerMerger{orders: store}

	ids, err := m.ReassignCustomer(context.Background(), fromID, toID)
	if err != nil {
		t.Fatalf("ReassignCustomer: unexpected err: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected 0 orders moved, got %d (%v)", len(ids), ids)
	}
}

// (3) FAILURE PATH — repository error is wrapped with %w (errors.Is still
// matches) and context (order IDs), not swallowed or replaced.
func TestCustomerMerger_ReassignCustomer_StoreError_WrappedAndPropagated(t *testing.T) {
	store := newFakeCustomerMergeStore()
	wantErr := errors.New("boom: db connection dropped")
	store.err = wantErr
	fromID, toID := uuid.New(), uuid.New()
	m := &CustomerMerger{orders: store}

	ids, err := m.ReassignCustomer(context.Background(), fromID, toID)
	if err == nil {
		t.Fatal("expected an error when the store fails")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected the store's error to be wrapped (%%w), got %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected 0 on error, got %d (%v)", len(ids), ids)
	}
}
