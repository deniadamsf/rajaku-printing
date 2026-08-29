package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/rajaku-printing/backend/internal/auth/model"
)

// newMockUserRepo — same sqlmock+gorm harness as
// discount/repository/discount_repository_test.go's newMockRepo, so this
// test can assert the EXACT sequence of SELECT statements (and their bind
// args) StreamCustomers issues — this is what proves the offset/limit fix
// (temuan review #1): if StreamCustomers ever regressed back to
// FindInBatches, the OFFSET args asserted below wouldn't appear at all (GORM
// would instead paginate by primary key), and this test would fail loudly
// instead of silently reintroducing the missing/duplicate-rows CSV bug.
func newMockUserRepo(t *testing.T) (*UserRepository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	gdb, err := gorm.Open(postgres.New(postgres.Config{Conn: db}), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	return NewUserRepository(gdb), mock
}

// TestStreamCustomers_MultipleBatches_EveryRowExactlyOnceInOrder — temuan
// review #1: 7 rows, batch size 3 (3 batches: 3+3+1), asserting (a) each
// batch query uses OFFSET/LIMIT advancing deterministically — NOT GORM's
// FindInBatches primary-key pagination — and (b) every one of the 7 rows
// reaches the caller EXACTLY once, in the order the repository returned
// them, proving nothing is skipped or duplicated across batch boundaries.
func TestStreamCustomers_MultipleBatches_EveryRowExactlyOnceInOrder(t *testing.T) {
	repo, mock := newMockUserRepo(t)

	ids := make([]uuid.UUID, 7)
	for i := range ids {
		ids[i] = uuid.New()
	}
	baseTime := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	cols := []string{"id", "created_at"}

	// Batch 1 — offset=0 limit=3. GORM omits the OFFSET clause entirely when
	// offset is 0 (no "OFFSET $n" arg at all), so this first query only
	// binds 2 args — user_type and limit.
	mock.ExpectQuery(`SELECT \* FROM "users" WHERE user_type = \$1.*ORDER BY created_at DESC, id DESC.*LIMIT \$2`).
		WithArgs("customer", 3).
		WillReturnRows(sqlmock.NewRows(cols).
			AddRow(ids[0].String(), baseTime.Add(-1*time.Minute)).
			AddRow(ids[1].String(), baseTime.Add(-2*time.Minute)).
			AddRow(ids[2].String(), baseTime.Add(-3*time.Minute)))

	// Batch 2 — offset=3 limit=3.
	mock.ExpectQuery(`SELECT \* FROM "users" WHERE user_type = \$1.*ORDER BY created_at DESC, id DESC.*LIMIT \$2.*OFFSET \$3`).
		WithArgs("customer", 3, 3).
		WillReturnRows(sqlmock.NewRows(cols).
			AddRow(ids[3].String(), baseTime.Add(-4*time.Minute)).
			AddRow(ids[4].String(), baseTime.Add(-5*time.Minute)).
			AddRow(ids[5].String(), baseTime.Add(-6*time.Minute)))

	// Batch 3 — offset=6 limit=3, only 1 row left (< batchSize) => loop stops
	// WITHOUT issuing a 4th query.
	mock.ExpectQuery(`SELECT \* FROM "users" WHERE user_type = \$1.*ORDER BY created_at DESC, id DESC.*LIMIT \$2.*OFFSET \$3`).
		WithArgs("customer", 3, 6).
		WillReturnRows(sqlmock.NewRows(cols).
			AddRow(ids[6].String(), baseTime.Add(-7*time.Minute)))

	var seen []uuid.UUID
	err := repo.StreamCustomers(context.Background(), ListCustomerFilter{}, 3, func(batch []model.User) error {
		for _, u := range batch {
			seen = append(seen, u.ID)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("StreamCustomers() error = %v, want nil", err)
	}

	if len(seen) != len(ids) {
		t.Fatalf("expected exactly %d rows total, got %d: %v", len(ids), len(seen), seen)
	}
	seenCount := map[uuid.UUID]int{}
	for _, id := range seen {
		seenCount[id]++
	}
	for i, id := range ids {
		if seenCount[id] != 1 {
			t.Fatalf("row %d (%s) seen %d times, want exactly 1 (seen order: %v)", i, id, seenCount[id], seen)
		}
	}
	// Order must be preserved exactly as the (deterministically-ordered)
	// repository query returned it — no reordering/interleaving across
	// batch boundaries.
	for i, id := range ids {
		if seen[i] != id {
			t.Fatalf("row at position %d = %s, want %s (order not preserved across batches)", i, seen[i], id)
		}
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}

// TestStreamCustomers_CallbackError_StopsIteration — fn returning an error
// must stop the loop immediately (no further batch queries issued) and
// propagate that exact error.
func TestStreamCustomers_CallbackError_StopsIteration(t *testing.T) {
	repo, mock := newMockUserRepo(t)

	id := uuid.New()
	cols := []string{"id", "created_at"}
	mock.ExpectQuery(`SELECT \* FROM "users" WHERE user_type = \$1.*LIMIT \$2`).
		WithArgs("customer", 3).
		WillReturnRows(sqlmock.NewRows(cols).AddRow(id.String(), time.Now()))

	wantErr := errors.New("boom: client disconnected mid-export")
	err := repo.StreamCustomers(context.Background(), ListCustomerFilter{}, 3, func(_ []model.User) error {
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("StreamCustomers() error = %v, want %v", err, wantErr)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations (expected exactly ONE batch query before stopping): %v", err)
	}
}
