package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/auth/model"
)

// ---------------------------------------------------------------------------
// fakeCustomerUserStore — minimal in-memory stand-in for customerUserStore,
// exercised only through SearchCustomers here (§ code review: the
// minSearchQueryLen guard was completely untested — nothing stopped it from
// silently regressing).
// ---------------------------------------------------------------------------

type fakeCustomerUserStore struct {
	searchCalls int
	lastQuery   string
	lastLimit   int

	searchResult []model.User
	searchErr    error
}

func (f *fakeCustomerUserStore) FindByPhone(_ context.Context, _ string) (*model.User, error) {
	return nil, errors.New("fakeCustomerUserStore.FindByPhone: not used by these tests")
}

func (f *fakeCustomerUserStore) Create(_ context.Context, _ *model.User) error {
	return errors.New("fakeCustomerUserStore.Create: not used by these tests")
}

func (f *fakeCustomerUserStore) FindByID(_ context.Context, _ uuid.UUID) (*model.User, error) {
	return nil, errors.New("fakeCustomerUserStore.FindByID: not used by these tests")
}

func (f *fakeCustomerUserStore) SearchCustomers(_ context.Context, q string, limit int) ([]model.User, error) {
	f.searchCalls++
	f.lastQuery = q
	f.lastLimit = limit
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	return f.searchResult, nil
}

// (1) Happy path: a query >= minSearchQueryLen reaches the repository and its
// results map through to authapi.Identity correctly.
func TestCustomerService_SearchCustomers_HappyPath_MapsResults(t *testing.T) {
	registered := model.CustomerTypeRegistered
	phoneStr := "6281234500001"
	store := &fakeCustomerUserStore{
		searchResult: []model.User{
			{ID: uuid.New(), Name: "Budi Santoso", Phone: &phoneStr, UserType: model.UserTypeCustomer, CustomerType: &registered, IsActive: true},
		},
	}
	svc := &CustomerService{users: store}

	got, err := svc.SearchCustomers(context.Background(), "Budi", 10)
	if err != nil {
		t.Fatalf("SearchCustomers: unexpected err: %v", err)
	}
	if store.searchCalls != 1 {
		t.Fatalf("expected the repository to be called exactly once, got %d calls", store.searchCalls)
	}
	if store.lastQuery != "Budi" {
		t.Fatalf("expected the repository to receive the trimmed query %q, got %q", "Budi", store.lastQuery)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
	if got[0].Name != "Budi Santoso" {
		t.Fatalf("expected mapped Name=%q, got %q", "Budi Santoso", got[0].Name)
	}
	if got[0].Phone != phoneStr {
		t.Fatalf("expected mapped Phone=%q, got %q", phoneStr, got[0].Phone)
	}
}

// (2) SECURITY-RELEVANT: a query below minSearchQueryLen must be rejected
// BEFORE the repository is ever called — this is the guard standing between
// a 1-character wildcard query and a full customer-table scan.
func TestCustomerService_SearchCustomers_QueryTooShort_RejectedBeforeRepositoryCalled(t *testing.T) {
	cases := []struct {
		name  string
		query string
	}{
		{"empty", ""},
		{"whitespace only", "   "},
		{"single char", "a"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeCustomerUserStore{}
			svc := &CustomerService{users: store}

			got, err := svc.SearchCustomers(context.Background(), tc.query, 10)
			if err != nil {
				t.Fatalf("SearchCustomers: unexpected err: %v", err)
			}
			if len(got) != 0 {
				t.Fatalf("expected empty result, got %d", len(got))
			}
			if store.searchCalls != 0 {
				t.Fatalf("expected the repository to NEVER be called for a too-short query, got %d calls", store.searchCalls)
			}
		})
	}
}

// (3) The max-length guard: a query far longer than any legitimate name/WA
// number has no business use case here (no rate limiting on this admin
// endpoint) — it must be rejected the same way, before the repository call.
func TestCustomerService_SearchCustomers_QueryTooLong_RejectedBeforeRepositoryCalled(t *testing.T) {
	store := &fakeCustomerUserStore{}
	svc := &CustomerService{users: store}

	tooLong := strings.Repeat("a", maxSearchQueryLen+1)
	got, err := svc.SearchCustomers(context.Background(), tooLong, 10)
	if err != nil {
		t.Fatalf("SearchCustomers: unexpected err: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty result, got %d", len(got))
	}
	if store.searchCalls != 0 {
		t.Fatalf("expected the repository to NEVER be called for an overly long query, got %d calls", store.searchCalls)
	}
}

// (3b) Exactly maxSearchQueryLen characters is still accepted (boundary).
func TestCustomerService_SearchCustomers_QueryAtMaxLength_Accepted(t *testing.T) {
	store := &fakeCustomerUserStore{}
	svc := &CustomerService{users: store}

	exact := strings.Repeat("a", maxSearchQueryLen)
	_, err := svc.SearchCustomers(context.Background(), exact, 10)
	if err != nil {
		t.Fatalf("SearchCustomers: unexpected err: %v", err)
	}
	if store.searchCalls != 1 {
		t.Fatalf("expected the repository to be called exactly once for a query at the max length boundary, got %d calls", store.searchCalls)
	}
}

// (4) Repository error is wrapped (%w) and propagated, not swallowed or
// re-stringified.
func TestCustomerService_SearchCustomers_RepositoryError_WrappedAndPropagated(t *testing.T) {
	wantErr := errors.New("boom: db down")
	store := &fakeCustomerUserStore{searchErr: wantErr}
	svc := &CustomerService{users: store}

	_, err := svc.SearchCustomers(context.Background(), "Budi", 10)
	if err == nil {
		t.Fatal("expected an error when the repository fails")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected the repository's error to be wrapped (%%w) into the returned error, got %v", err)
	}
}
