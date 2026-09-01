// Tests for OrderRepository.UpdateItems (§32.9 super admin item-level
// correction) — sqlmock-backed so the exact sequence of statements inside
// the transaction can be asserted (same harness pattern as
// discount/repository/discount_repository_test.go).
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

	"github.com/rajaku-printing/backend/internal/order/model"
)

func newMockOrderRepo(t *testing.T) (*OrderRepository, sqlmock.Sqlmock) {
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
	return NewOrderRepository(gdb), mock
}

// TestUpdateItems_DeleteRowWithDesignFiles_RejectedBeforeDelete is the
// regression guard for temuan review §32.9 #2: deleting an order_items row
// still referenced by design_files (migration 000034, FK is RESTRICT — no
// ON DELETE CASCADE, §19) must be refused with ErrItemHasDesignFiles BEFORE
// the DELETE is ever attempted, and the whole transaction must roll back —
// not surface as a raw Postgres FK-violation wrapped in a generic 500.
func TestUpdateItems_DeleteRowWithDesignFiles_RejectedBeforeDelete(t *testing.T) {
	repo, mock := newMockOrderRepo(t)
	orderID := uuid.New()
	itemID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT "status" FROM "orders"`).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("order_masuk"))
	mock.ExpectQuery(`SELECT DISTINCT "order_items"\."line_no" FROM "order_items" JOIN design_files`).
		WillReturnRows(sqlmock.NewRows([]string{"line_no"}).AddRow(2))
	mock.ExpectRollback()

	err := repo.UpdateItems(context.Background(), UpdateItemsParams{
		OrderID:        orderID,
		ExpectedStatus: "order_masuk",
		DeleteItemIDs:  []uuid.UUID{itemID},
		Audit:          &model.AdminAuditLog{ID: uuid.New()},
	})
	if !errors.Is(err, ErrItemHasDesignFiles) {
		t.Fatalf("UpdateItems() error = %v, want ErrItemHasDesignFiles", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}

// TestUpdateItems_DeleteRowWithoutDesignFiles_ProceedsToDelete is the happy
// path counterpart: a row with NO design_files reference is deleted
// normally, and the transaction commits.
func TestUpdateItems_DeleteRowWithoutDesignFiles_ProceedsToDelete(t *testing.T) {
	repo, mock := newMockOrderRepo(t)
	orderID := uuid.New()
	itemID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT "status" FROM "orders"`).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("order_masuk"))
	mock.ExpectQuery(`SELECT DISTINCT "order_items"\."line_no" FROM "order_items" JOIN design_files`).
		WillReturnRows(sqlmock.NewRows([]string{"line_no"}))
	mock.ExpectExec(`DELETE FROM "order_items"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`INSERT INTO "admin_audit_log"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(uuid.New(), time.Now()))
	mock.ExpectCommit()

	err := repo.UpdateItems(context.Background(), UpdateItemsParams{
		OrderID:        orderID,
		ExpectedStatus: "order_masuk",
		DeleteItemIDs:  []uuid.UUID{itemID},
		Audit:          &model.AdminAuditLog{ID: uuid.New()},
	})
	if err != nil {
		t.Fatalf("UpdateItems() error = %v, want nil", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}

// TestUpdateItems_ConcurrentlyDeletedRow_ReturnsErrStaleState is the
// regression guard for temuan review §32.9 #3: Phase 3's final Updates()
// call matching ZERO rows (because a concurrent admin already deleted that
// row) is NOT a GORM error — without an explicit RowsAffected check it
// would silently proceed to write orders.subtotal/total computed against an
// item that never actually got its new values persisted. Must abort with
// ErrStaleState instead.
func TestUpdateItems_ConcurrentlyDeletedRow_ReturnsErrStaleState(t *testing.T) {
	repo, mock := newMockOrderRepo(t)
	orderID := uuid.New()
	itemID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT "status" FROM "orders"`).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("order_masuk"))
	// Phase 1 — bump to temp offset: succeeds (row still existed a moment
	// ago from THIS call's point of view up to here).
	mock.ExpectExec(`UPDATE "order_items" SET`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// Phase 3 — final write: 0 rows affected, simulating the row having
	// vanished between Phase 1 and Phase 3 (concurrent delete by another
	// admin session).
	mock.ExpectExec(`UPDATE "order_items" SET`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	err := repo.UpdateItems(context.Background(), UpdateItemsParams{
		OrderID:        orderID,
		ExpectedStatus: "order_masuk",
		UpsertItems: []model.OrderItem{
			{ID: itemID, LineNo: 1, ProductNameSnapshot: "A", MaterialNameSnapshot: "MA",
				WidthCm: 100, HeightCm: 100, Quantity: 1, UnitPrice: 10_000, Subtotal: 10_000},
		},
		Audit: &model.AdminAuditLog{ID: uuid.New()},
	})
	if !errors.Is(err, ErrStaleState) {
		t.Fatalf("UpdateItems() error = %v, want ErrStaleState", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}
