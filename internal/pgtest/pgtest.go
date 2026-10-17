// Package pgtest resets shared PostgreSQL fixtures between tests. Packages must
// run serially because they share the database selected by PG_TEST_DSN:
//
//	PG_TEST_DSN=... go test ./... -count=1 -tags=integration -p 1
package pgtest

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
)

// schemaMigrationsTable is the additive-migration bookkeeping table written by
// store.AutoMigrate. It is deliberately preserved: emptying it would make every
// test re-apply the whole additive migration ladder, and the ladder is not the
// subject of these tests.
const schemaMigrationsTable = "schema_migrations"

// Reset empties every user table in the current schema — except the migration
// bookkeeping table — and restarts identity sequences, so fixture primary keys
// are deterministic across runs. Call it right after opening the database and
// before AutoMigrate.
//
// All tables are truncated together so intra-schema foreign keys need no
// deletion order. Do not use CASCADE: a foreign key from another schema must
// fail the reset, not silently clear another fixture. ONLY likewise prevents
// PostgreSQL from implicitly including descendants in another schema. Table
// identifiers are quoted by PostgreSQL itself.
func Reset(t testing.TB, db *sqlx.DB) {
	t.Helper()
	if db == nil {
		t.Fatal("pgtest.Reset: nil database handle")
	}
	if err := reset(db); err != nil {
		t.Fatalf("pgtest.Reset: %v", err)
	}
}

func reset(db *sqlx.DB) error {
	var tables []string
	const listTables = `SELECT format('ONLY %I.%I', schemaname, tablename)
		FROM pg_tables
		WHERE schemaname = current_schema()
		  AND schemaname NOT IN ('pg_catalog', 'information_schema')
		  AND tablename <> $1
		ORDER BY tablename`
	if err := db.Select(&tables, listTables, schemaMigrationsTable); err != nil {
		return fmt.Errorf("list tables: %w", err)
	}
	if len(tables) == 0 {
		// Fresh database: AutoMigrate has not created anything yet.
		return nil
	}
	if _, err := db.Exec("TRUNCATE " + strings.Join(tables, ", ") + " RESTART IDENTITY"); err != nil {
		return fmt.Errorf("truncate %d tables: %w", len(tables), err)
	}
	return nil
}
