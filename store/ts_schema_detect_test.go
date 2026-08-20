package store

import (
	"fmt"
	"testing"
)

// createDrizzleMigrations replicates the TypeScript __drizzle_migrations
// journal table (metapi-ts/src/server/db/migrate.ts ensureDrizzleMigrationsTable).
func createDrizzleMigrations(t *testing.T, db *DB) {
	t.Helper()
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS "__drizzle_migrations" (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			hash text NOT NULL,
			created_at numeric
		)`); err != nil {
		t.Fatalf("create __drizzle_migrations: %v", err)
	}
}

// TestDetectTSSchemaDriftNewerTSJournal pins the journal-age half of the
// reverse-drift scan: a __drizzle_migrations max(created_at) above
// knownLatestTSMigrationWhen means the database was produced by a newer
// TypeScript build.
func TestDetectTSSchemaDriftNewerTSJournal(t *testing.T) {
	db, err := Open(DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	createDrizzleMigrations(t, db)
	for i, when := range []int64{
		knownLatestTSMigrationWhen - 1000,
		knownLatestTSMigrationWhen + 1000, // newer than Go knows
	} {
		if _, err := db.Exec(
			`INSERT INTO "__drizzle_migrations" (hash, created_at) VALUES (?, ?)`,
			fmt.Sprintf("hash-%d", i), when,
		); err != nil {
			t.Fatalf("insert journal row %d: %v", i, err)
		}
	}

	result, err := detectTSSchemaDrift(db)
	if err != nil {
		t.Fatalf("detectTSSchemaDrift: %v", err)
	}
	if result == nil {
		t.Fatal("detectTSSchemaDrift returned nil result for SQLite")
	}
	if !result.TSJournalNewer {
		t.Fatal("TSJournalNewer = false, want true (journal newer than knownLatestTSMigrationWhen)")
	}
	if result.TSJournalMaxWhen != knownLatestTSMigrationWhen+1000 {
		t.Fatalf("TSJournalMaxWhen = %d, want %d", result.TSJournalMaxWhen, knownLatestTSMigrationWhen+1000)
	}
}

// TestDetectTSSchemaDriftJournalWithinRange pins the no-warning side: a
// journal at or below the known latest `when` is not flagged.
func TestDetectTSSchemaDriftJournalWithinRange(t *testing.T) {
	db, err := Open(DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	createDrizzleMigrations(t, db)
	if _, err := db.Exec(
		`INSERT INTO "__drizzle_migrations" (hash, created_at) VALUES (?, ?)`,
		"hash-0", knownLatestTSMigrationWhen,
	); err != nil {
		t.Fatalf("insert journal row: %v", err)
	}

	result, err := detectTSSchemaDrift(db)
	if err != nil {
		t.Fatalf("detectTSSchemaDrift: %v", err)
	}
	if result.TSJournalNewer {
		t.Fatal("TSJournalNewer = true, want false (journal at known latest when)")
	}
	if len(result.UnknownColumns) != 0 {
		t.Fatalf("UnknownColumns = %v, want none", result.UnknownColumns)
	}
}

// TestDetectTSSchemaDriftUnknownColumn pins the unknown-column half: a
// future TS migration adding sites.future_column must be listed.
func TestDetectTSSchemaDriftUnknownColumn(t *testing.T) {
	db := openTestSQLite(t)

	if _, err := db.Exec(`ALTER TABLE sites ADD COLUMN future_column TEXT`); err != nil {
		t.Fatalf("add future_column: %v", err)
	}

	result, err := detectTSSchemaDrift(db)
	if err != nil {
		t.Fatalf("detectTSSchemaDrift: %v", err)
	}
	found := false
	for _, unknown := range result.UnknownColumns {
		if unknown == "sites.future_column" {
			found = true
		}
	}
	if !found {
		t.Fatalf("UnknownColumns = %v, want it to contain sites.future_column", result.UnknownColumns)
	}
}

// TestDetectTSSchemaDriftCleanDatabase pins the quiet case: a fresh Go
// database (empty DB after AutoMigrate) produces no journal warning and no
// unknown columns — including the __drizzle_migrations and sqlite_* tables,
// which are excluded from the scan.
func TestDetectTSSchemaDriftCleanDatabase(t *testing.T) {
	db := openTestSQLite(t)

	// The TS journal marker and SQLite internals must never be reported as
	// unknown columns, even when present.
	createDrizzleMigrations(t, db)
	if _, err := db.Exec(
		`INSERT INTO "__drizzle_migrations" (hash, created_at) VALUES (?, ?)`,
		"hash-0", knownLatestTSMigrationWhen,
	); err != nil {
		t.Fatalf("insert journal row: %v", err)
	}

	result, err := detectTSSchemaDrift(db)
	if err != nil {
		t.Fatalf("detectTSSchemaDrift: %v", err)
	}
	if result.TSJournalNewer {
		t.Fatal("TSJournalNewer = true on a clean Go database, want false")
	}
	if len(result.UnknownColumns) != 0 {
		t.Fatalf("UnknownColumns = %v on a clean Go database, want none", result.UnknownColumns)
	}
}
