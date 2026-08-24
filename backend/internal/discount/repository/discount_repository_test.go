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
// atomicity (temuan review #1): if UpdateWithProducts ever regressed back
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

// TestUpdateWithProducts_HappyPath_OneTransaction — field update AND product
// scope replacement commit together, in ONE Begin/Commit pair.
func TestUpdateWithProducts_HappyPath_OneTransaction(t *testing.T) {
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

	err := repo.UpdateWithProducts(context.Background(), id,
		map[string]any{"name": "Promo Baru"}, true, []uuid.UUID{productID})
	if err != nil {
		t.Fatalf("UpdateWithProducts() error = %v, want nil", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}

// TestUpdateWithProducts_ProductReplaceFails_RollsBackFieldUpdateToo —
// temuan review #1: kalau langkah penggantian cakupan produk gagal (di sini
// disimulasikan lewat kegagalan INSERT), perubahan field yang SUDAH
// dieksekusi sebelumnya (UPDATE discounts) HARUS ikut di-ROLLBACK, bukan
// ter-commit sendirian. Test ini hanya expect SATU Begin/Rollback — kalau
// implementasi kembali memecah jadi dua transaksi (bug lama), mock akan
// melihat Commit di antaranya dan ExpectationsWereMet() gagal.
func TestUpdateWithProducts_ProductReplaceFails_RollsBackFieldUpdateToo(t *testing.T) {
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

	err := repo.UpdateWithProducts(context.Background(), id,
		map[string]any{"name": "Promo Baru"}, true, []uuid.UUID{productID})
	if err == nil {
		t.Fatalf("UpdateWithProducts() error = nil, want non-nil (insert failure must propagate)")
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

// TestUpdateWithProducts_UnknownProductID_ReturnsSentinel_NotRawFKError —
// temuan review #3: product_ids berisi UUID yang tidak ada di `products`
// harus ditolak dengan ErrProductNotFound (dipetakan ke 400 di service/
// handler), BUKAN raw FK violation yang berakhir 500. Bulk existence check
// menangkap ini SEBELUM insert pernah dicoba.
func TestUpdateWithProducts_UnknownProductID_ReturnsSentinel_NotRawFKError(t *testing.T) {
	repo, mock := newMockRepo(t)
	id := uuid.New()
	unknownProductID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM discount_products WHERE discount_id = \$1`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT count\(\*\) FROM "products" WHERE id IN \(\$1\)`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0)) // produk tidak ditemukan
	mock.ExpectRollback()

	err := repo.UpdateWithProducts(context.Background(), id, nil, true, []uuid.UUID{unknownProductID})
	if err != ErrProductNotFound {
		t.Fatalf("UpdateWithProducts() error = %v, want ErrProductNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}

// TestUpdateWithProducts_ForeignKeyViolationRace_MapsToSameSentinel —
// jaring pengaman temuan #3: kalaupun bulk existence check lolos (produk
// masih ada saat dicek) tapi INSERT tetap kena FK violation (race — produk
// dihapus tepat di antara keduanya), repository tetap memetakan ke
// ErrProductNotFound yang sama, bukan raw error 500.
func TestUpdateWithProducts_ForeignKeyViolationRace_MapsToSameSentinel(t *testing.T) {
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

	err := repo.UpdateWithProducts(context.Background(), id, nil, true, []uuid.UUID{productID})
	if err != ErrProductNotFound {
		t.Fatalf("UpdateWithProducts() error = %v, want ErrProductNotFound (FK violation fallback)", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}

// TestUpdateWithProducts_NotFound_NoFieldsNoProducts_IsNoOp — mirrors the
// existing Update() no-op behavior: an empty fields map AND
// replaceProducts=false must not touch the DB at all.
func TestUpdateWithProducts_NoFieldsNoReplace_IsNoOp(t *testing.T) {
	repo, mock := newMockRepo(t)
	err := repo.UpdateWithProducts(context.Background(), uuid.New(), nil, false, nil)
	if err != nil {
		t.Fatalf("UpdateWithProducts() error = %v, want nil (no-op)", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations (expected NO queries at all): %v", err)
	}
}
