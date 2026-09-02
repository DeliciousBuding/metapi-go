// Package pgtest is a test-only helper for the PostgreSQL-gated suites
// (PG_TEST_DSN). It is imported exclusively from _test.go files, so it never
// becomes part of the production dependency graph, and it depends on nothing
// but sqlx so even the store package's own tests can use it.
//
// Why it exists: CI runs every PG-gated test against a freshly created
// database, while a local development loop reuses one. Fixtures with a fixed
// identity — a site keyed on the unique (platform, url) pair, a downstream key
// with a constant value — then collide with the previous run's leftovers, and
// whole-table assertions count rows nobody seeded in this run. The failures are
// real red herrings: they say "duplicate key" or "total = 4, want 3" about
// state, not about the code under test. The documented escape hatch was "drop
// and recreate the database by hand before trusting a result", which is exactly
// the kind of tribal knowledge that makes a suite nobody dares to run twice.
//
// Reset makes the local loop match CI: every PG-gated test starts from an empty
// database, so a suite is repeatable by construction and a second run of the
// same command is evidence rather than noise.
//
// Run the PG suite the way CI does — serialized, one database:
//
//	PG_TEST_DSN=... go test ./... -count=1 -tags=integration -p 1
//
// -p 1 is not optional: packages share one database, so parallel packages race
// each other on AutoMigrate and on this truncation (the CI workflow carries the
// same note). Tests inside a package already run sequentially.
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
