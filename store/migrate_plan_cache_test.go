package store

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

// TestClearPreparedPlanCachePostgresIssuesStatements verifies that the
// PostgreSQL branch issues DEALLOCATE ALL and DISCARD ALL as two separate
// executions. The pgx driver speaks the extended protocol, which rejects
// multiple commands inside a single prepared statement, so a combined
// "DEALLOCATE ALL; DISCARD ALL" string must never be sent.
func TestClearPreparedPlanCachePostgresIssuesStatements(t *testing.T) {
	mockSQLDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer mockSQLDB.Close()

	// sqlmock.New() defaults to ordered expectations, so this also pins the
	// exact statement sequence.
	mock.ExpectExec("DEALLOCATE ALL").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DISCARD ALL").WillReturnResult(sqlmock.NewResult(0, 0))

	db := &DB{DB: sqlx.NewDb(mockSQLDB, "sqlmock"), Dialect: DialectPostgres}
	if err := ClearPreparedPlanCache(db); err != nil {
		t.Fatalf("ClearPreparedPlanCache(postgres): %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestClearPreparedPlanCacheSQLiteNoop verifies the SQLite dialect
// short-circuits without issuing any statement (SQLite has no server-side
// prepared plan cache).
func TestClearPreparedPlanCacheSQLiteNoop(t *testing.T) {
	db, err := Open(DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("Open(sqlite): %v", err)
	}
	defer db.Close()

	if err := ClearPreparedPlanCache(db); err != nil {
		t.Fatalf("ClearPreparedPlanCache(sqlite): %v", err)
	}
}

// TestClearPreparedPlanCacheNilDB verifies a nil DB is a safe no-op.
func TestClearPreparedPlanCacheNilDB(t *testing.T) {
	if err := ClearPreparedPlanCache(nil); err != nil {
		t.Fatalf("ClearPreparedPlanCache(nil): %v", err)
	}
}

// TestAutoMigrateDoesNotRunPlanCacheCleanupOnSQLite verifies the
// AutoMigrate integration stays SQLite-clean: the cleanup hook must not leak
// PostgreSQL-only statements into the SQLite path.
func TestAutoMigrateDoesNotRunPlanCacheCleanupOnSQLite(t *testing.T) {
	db, err := Open(DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("Open(sqlite): %v", err)
	}
	defer db.Close()

	if err := AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate(sqlite): %v", err)
	}
}
