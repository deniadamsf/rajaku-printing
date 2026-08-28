package repository

import (
	"context"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// newMockRepo wires a *DiscountRepository to a sqlmock-backed *sql.DB, so
// tests can assert the EXACT sequence of statements (Begin/Exec/Query/
// Commit/Rollback) a repository method issues — this is what proves
// atomicity (temuan review #1): if UpdateWithScopes ever regressed back
// into two separate transactions, this harness would show TWO Begin/Commit
// pairs instead of one, and mock.ExpectationsWereMet() would fail.
func newMockRepo(t *testing.T) (*DiscountRepository, sqlmock.Sqlmock) {
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
	return NewDiscountRepository(gdb), mock
}

// TestUpdateWithScopes_HappyPath_OneTransaction — field update AND product
// scope replacement commit together, in ONE Begin/Commit pair.
func TestUpdateWithScopes_HappyPath_OneTransaction(t *testing.T) {
	repo, mock := newMockRepo(t)
	id := uuid.New()
	productID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "discounts" SET`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM discount_products WHERE discount_id = \$1`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT count\(\*\) FROM "products" WHERE id IN \(\$1\)`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectExec(`INSERT INTO discount_products`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := repo.UpdateWithScopes(context.Background(), id,
		map[string]any{"name": "Promo Baru"},
		ScopeReplace{Replace: true, IDs: []uuid.UUID{productID}},
		ScopeReplace{})
	if err != nil {
		t.Fatalf("UpdateWithScopes() error = %v, want nil", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}

// TestUpdateWithScopes_ProductReplaceFails_RollsBackFieldUpdateToo —
// temuan review #1: kalau langkah penggantian cakupan produk gagal (di sini
// disimulasikan lewat kegagalan INSERT), perubahan field yang SUDAH
// dieksekusi sebelumnya (UPDATE discounts) HARUS ikut di-ROLLBACK, bukan
// ter-commit sendirian. Test ini hanya expect SATU Begin/Rollback — kalau
// implementasi kembali memecah jadi dua transaksi (bug lama), mock akan
// melihat Commit di antaranya dan ExpectationsWereMet() gagal.
func TestUpdateWithScopes_ProductReplaceFails_RollsBackFieldUpdateToo(t *testing.T) {
	repo, mock := newMockRepo(t)
	id := uuid.New()
	productID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "discounts" SET`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM discount_products WHERE discount_id = \$1`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT count\(\*\) FROM "products" WHERE id IN \(\$1\)`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectExec(`INSERT INTO discount_products`).
		WillReturnError(&pgconn.PgError{Code: "08006", Message: "connection failure"})
	mock.ExpectRollback()

	err := repo.UpdateWithScopes(context.Background(), id,
		map[string]any{"name": "Promo Baru"},
		ScopeReplace{Replace: true, IDs: []uuid.UUID{productID}},
		ScopeReplace{})
	if err == nil {
		t.Fatalf("UpdateWithScopes() error = nil, want non-nil (insert failure must propagate)")
	}
	// Yang paling penting: TIDAK ada ExpectCommit di atas. Kalau field
	// update dan product replace masih dua transaksi terpisah, mock ini
	// akan gagal karena Commit pertama tidak pernah "dipanggil" sesuai
	// urutan yang di-set — justru sebaliknya, kalau kode PRODUCTION salah
	// (commit dulu baru replace), sqlmock akan melaporkan "call to
	// Commit transaction was not expected" karena kita tidak
	// mendaftarkan ExpectCommit sama sekali.
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations (field update was NOT rolled back atomically with product scope): %v", err)
	}
}

// TestUpdateWithScopes_UnknownProductID_ReturnsSentinel_NotRawFKError —
// temuan review #3: product_ids berisi UUID yang tidak ada di `products`
// harus ditolak dengan ErrProductNotFound (dipetakan ke 400 di service/
// handler), BUKAN raw FK violation yang berakhir 500. Bulk existence check
// menangkap ini SEBELUM insert pernah dicoba.
func TestUpdateWithScopes_UnknownProductID_ReturnsSentinel_NotRawFKError(t *testing.T) {
	repo, mock := newMockRepo(t)
	id := uuid.New()
	unknownProductID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM discount_products WHERE discount_id = \$1`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT count\(\*\) FROM "products" WHERE id IN \(\$1\)`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0)) // produk tidak ditemukan
	mock.ExpectRollback()

	err := repo.UpdateWithScopes(context.Background(), id, nil,
		ScopeReplace{Replace: true, IDs: []uuid.UUID{unknownProductID}}, ScopeReplace{})
	if err != ErrProductNotFound {
		t.Fatalf("UpdateWithScopes() error = %v, want ErrProductNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}

// TestUpdateWithScopes_ForeignKeyViolationRace_MapsToSameSentinel —
// jaring pengaman temuan #3: kalaupun bulk existence check lolos (produk
// masih ada saat dicek) tapi INSERT tetap kena FK violation (race — produk
// dihapus tepat di antara keduanya), repository tetap memetakan ke
// ErrProductNotFound yang sama, bukan raw error 500.
func TestUpdateWithScopes_ForeignKeyViolationRace_MapsToSameSentinel(t *testing.T) {
	repo, mock := newMockRepo(t)
	id := uuid.New()
	productID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM discount_products WHERE discount_id = \$1`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT count\(\*\) FROM "products" WHERE id IN \(\$1\)`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectExec(`INSERT INTO discount_products`).
		WillReturnError(&pgconn.PgError{Code: pgForeignKeyViolationCode, Message: "fk violation"})
	mock.ExpectRollback()

	err := repo.UpdateWithScopes(context.Background(), id, nil,
		ScopeReplace{Replace: true, IDs: []uuid.UUID{productID}}, ScopeReplace{})
	if err != ErrProductNotFound {
		t.Fatalf("UpdateWithScopes() error = %v, want ErrProductNotFound (FK violation fallback)", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}

// TestUpdateWithScopes_NoFieldsNoReplace_IsNoOp — an empty fields map AND
// both ScopeReplace.Replace=false must not touch the DB at all.
func TestUpdateWithScopes_NoFieldsNoReplace_IsNoOp(t *testing.T) {
	repo, mock := newMockRepo(t)
	err := repo.UpdateWithScopes(context.Background(), uuid.New(), nil, ScopeReplace{}, ScopeReplace{})
	if err != nil {
		t.Fatalf("UpdateWithScopes() error = %v, want nil (no-op)", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations (expected NO queries at all): %v", err)
	}
}

// ---- §30.3 — cakupan member (discount_customers) ----

// TestUpdateWithScopes_CustomerReplace_HappyPath_OneTransaction — mirror
// TestUpdateWithScopes_HappyPath_OneTransaction, but for the member-scope
// axis: field update + discount_customers replacement, one Begin/Commit.
func TestUpdateWithScopes_CustomerReplace_HappyPath_OneTransaction(t *testing.T) {
	repo, mock := newMockRepo(t)
	id := uuid.New()
	customerID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "discounts" SET`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM discount_customers WHERE discount_id = \$1`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT count\(\*\) FROM "users" WHERE id IN \(\$1\)`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectExec(`INSERT INTO discount_customers`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := repo.UpdateWithScopes(context.Background(), id,
		map[string]any{"audience_scope": "member"},
		ScopeReplace{},
		ScopeReplace{Replace: true, IDs: []uuid.UUID{customerID}})
	if err != nil {
		t.Fatalf("UpdateWithScopes() error = %v, want nil", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}

// TestUpdateWithScopes_UnknownCustomerID_ReturnsSentinel — customer_ids
// berisi UUID yang tidak ada di `users` harus ditolak dengan
// ErrCustomerNotFound, BUKAN raw FK violation 500 (mirror produk).
func TestUpdateWithScopes_UnknownCustomerID_ReturnsSentinel(t *testing.T) {
	repo, mock := newMockRepo(t)
	id := uuid.New()
	unknownCustomerID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM discount_customers WHERE discount_id = \$1`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT count\(\*\) FROM "users" WHERE id IN \(\$1\)`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectRollback()

	err := repo.UpdateWithScopes(context.Background(), id, nil,
		ScopeReplace{}, ScopeReplace{Replace: true, IDs: []uuid.UUID{unknownCustomerID}})
	if err != ErrCustomerNotFound {
		t.Fatalf("UpdateWithScopes() error = %v, want ErrCustomerNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}

// TestUpdateWithScopes_CustomerID_IsStaffAccount_ReturnsSentinel — code
// review finding (§30.3): ensureCustomersExist used to check ONLY "does this
// id exist in `users`", with no filter on `user_type`. A staff/admin UUID
// (which very much exists in `users`, just with user_type='staff') would
// silently pass, land in discount_customers, and then never match any real
// order.customer_id (orders are always created for customer accounts) — the
// discount looks "configured" but is quietly unusable. This test asserts the
// query now filters `user_type = 'customer'`, so a staff id (simulated here
// as a bulk-count of 0, exactly like TestUpdateWithScopes_UnknownCustomerID_
// ReturnsSentinel — from the DB's perspective under the fixed query, a
// staff id and a genuinely-nonexistent id are indistinguishable, which is
// the point: both are "not a valid customer scope target") is rejected with
// ErrCustomerNotFound rather than silently accepted.
func TestUpdateWithScopes_CustomerID_IsStaffAccount_ReturnsSentinel(t *testing.T) {
	repo, mock := newMockRepo(t)
	id := uuid.New()
	staffID := uuid.New() // exists in `users`, but user_type='staff'

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM discount_customers WHERE discount_id = \$1`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// The fixed query filters `AND user_type = 'customer'` — a staff row
	// exists in `users` but does NOT satisfy that predicate, so the count
	// comes back 0 even though `staffID` is a real row in the table.
	mock.ExpectQuery(`SELECT count\(\*\) FROM "users" WHERE id IN \(\$1\) AND user_type = \$2`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectRollback()

	err := repo.UpdateWithScopes(context.Background(), id, nil,
		ScopeReplace{}, ScopeReplace{Replace: true, IDs: []uuid.UUID{staffID}})
	if err != ErrCustomerNotFound {
		t.Fatalf("UpdateWithScopes() error = %v, want ErrCustomerNotFound (staff id must not be accepted as discount member scope)", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}

// TestUpdateWithScopes_CustomerForeignKeyViolation_MapsByTableName — the FK
// violation fallback (race window) must disambiguate discount_customers
// from discount_products using the driver-reported TableName, not just
// default to ErrProductNotFound.
func TestUpdateWithScopes_CustomerForeignKeyViolation_MapsByTableName(t *testing.T) {
	repo, mock := newMockRepo(t)
	id := uuid.New()
	customerID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM discount_customers WHERE discount_id = \$1`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT count\(\*\) FROM "users" WHERE id IN \(\$1\)`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectExec(`INSERT INTO discount_customers`).
		WillReturnError(&pgconn.PgError{Code: pgForeignKeyViolationCode, TableName: "discount_customers", Message: "fk violation"})
	mock.ExpectRollback()

	err := repo.UpdateWithScopes(context.Background(), id, nil,
		ScopeReplace{}, ScopeReplace{Replace: true, IDs: []uuid.UUID{customerID}})
	if err != ErrCustomerNotFound {
		t.Fatalf("UpdateWithScopes() error = %v, want ErrCustomerNotFound (FK violation fallback by table name)", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}
