// Package pgtest resets shared PostgreSQL fixtures between tests. Packages must
// run serially because they share the database selected by PG_TEST_DSN:
//
//	PG_TEST_DSN=... go test ./... -count=1 -tags=integration -p 1
package pgtest

import (
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
// CASCADE is required because the schema is a web of foreign keys (sites →
// accounts → account_tokens → route_channels → …); enumerating a deletion order
// per test would be a second source of truth about the schema. Only tables
// outside pg_catalog/information_schema are touched, and the statement is built
// from quoted identifiers returned by PostgreSQL itself.
func Reset(t testing.TB, db *sqlx.DB) {
	t.Helper()
	if db == nil {
		t.Fatal("pgtest.Reset: nil database handle")
	}

	var tables []string
	const listTables = `SELECT format('%I.%I', schemaname, tablename)
		FROM pg_tables
		WHERE schemaname NOT IN ('pg_catalog', 'information_schema')
		  AND tablename <> $1
		ORDER BY tablename`
	if err := db.Select(&tables, listTables, schemaMigrationsTable); err != nil {
		t.Fatalf("pgtest.Reset: list tables: %v", err)
	}
	if len(tables) == 0 {
		// Fresh database: AutoMigrate has not created anything yet.
		return
	}
	if _, err := db.Exec("TRUNCATE " + strings.Join(tables, ", ") + " RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("pgtest.Reset: truncate %d tables: %v", len(tables), err)
	}
}
