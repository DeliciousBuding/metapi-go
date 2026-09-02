package store

import (
	"os"
	"strings"
	"testing"
)

// TestNormalizeLegacyTimestamps_SQLite seeds TS-shaped (drizzle
// datetime('now')) values next to Go-shaped RFC3339 values in real takeover
// columns and verifies the sc2_029 rewrite: TS rows convert, Go rows stay,
// NULLs stay, re-runs are no-ops, and mixed-shape ordering becomes
// chronologically correct.
func TestNormalizeLegacyTimestamps_SQLite(t *testing.T) {
	db, err := Open(DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	// TS-era site: created_at/updated_at in drizzle shape.
	if _, err := db.Exec(`INSERT INTO sites (id, name, url, platform, status, created_at, updated_at)
		VALUES (1, 'ts-site', 'https://ts.example.com', 'openai', 'active', '2026-08-20 12:31:20', '2026-08-21 09:00:00')`); err != nil {
		t.Fatalf("seed ts site: %v", err)
	}
	// Go-era site created AFTER the TS one in real time: RFC3339 shape.
	if _, err := db.Exec(`INSERT INTO sites (id, name, url, platform, status, created_at, updated_at)
		VALUES (2, 'go-site', 'https://go.example.com', 'openai', 'active', '2026-09-01T08:00:00Z', '2026-09-01T08:00:00Z')`); err != nil {
		t.Fatalf("seed go site: %v", err)
	}
	// TS-era account with a drizzle-shaped last_checkin_at plus a NULL column.
	if _, err := db.Exec(`INSERT INTO accounts (id, site_id, username, access_token, status, created_at, updated_at, last_checkin_at)
		VALUES (1, 1, 'ts-acct', 'tok', 'active', '2026-08-20 12:31:21', '2026-08-20 12:31:21', '2026-08-30 23:59:59')`); err != nil {
		t.Fatalf("seed ts account: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO accounts (id, site_id, username, access_token, status, created_at, updated_at)
		VALUES (2, 1, 'go-acct', 'tok2', 'active', '2026-09-01T08:00:01Z', '2026-09-01T08:00:01Z')`); err != nil {
		t.Fatalf("seed go account: %v", err)
	}
	// Fractional-seconds variant (some drizzle paths wrote .SSS).
	if _, err := db.Exec(`INSERT INTO sites (id, name, url, platform, status, created_at, updated_at)
		VALUES (3, 'ts-frac', 'https://frac.example.com', 'openai', 'active', '2026-08-20 12:31:20.500', '2026-08-20 12:31:20.500')`); err != nil {
		t.Fatalf("seed frac site: %v", err)
	}

	changed, err := normalizeLegacyTimestamps(db)
	if err != nil {
		t.Fatalf("normalizeLegacyTimestamps: %v", err)
	}
	// Row-updates: sites.created_at (id 1,3) + sites.updated_at (id 1,3) = 4,
	// accounts.created_at + updated_at + last_checkin_at (id 1) = 3.
	if changed != 7 {
		t.Fatalf("changed = %d, want 7", changed)
	}

	var got string
	mustGet := func(query string, args ...any) string {
		t.Helper()
		if err := db.QueryRow(db.Rebind(query), args...).Scan(&got); err != nil {
			t.Fatalf("query %q: %v", query, err)
		}
		return got
	}

	if v := mustGet(`SELECT created_at FROM sites WHERE id = 1`); v != "2026-08-20T12:31:20Z" {
		t.Errorf("ts site created_at = %q, want RFC3339", v)
	}
	if v := mustGet(`SELECT updated_at FROM sites WHERE id = 1`); v != "2026-08-21T09:00:00Z" {
		t.Errorf("ts site updated_at = %q, want RFC3339", v)
	}
	if v := mustGet(`SELECT created_at FROM sites WHERE id = 2`); v != "2026-09-01T08:00:00Z" {
		t.Errorf("go site created_at = %q, must stay untouched", v)
	}
	if v := mustGet(`SELECT created_at FROM sites WHERE id = 3`); v != "2026-08-20T12:31:20.500Z" {
		t.Errorf("frac site created_at = %q, want RFC3339 with millis", v)
	}
	if v := mustGet(`SELECT last_checkin_at FROM accounts WHERE id = 1`); v != "2026-08-30T23:59:59Z" {
		t.Errorf("ts account last_checkin_at = %q, want RFC3339", v)
	}
	if v := mustGet(`SELECT COALESCE(last_checkin_at, 'NULL') FROM accounts WHERE id = 2`); v != "NULL" {
		t.Errorf("go account last_checkin_at = %q, NULL must stay NULL", v)
	}

	// Ordering: the Go-era site is newer in real time; DESC must list it
	// first now that both rows speak RFC3339 (before normalization the space
	// separator sorted TS rows first regardless of date).
	rows, err := db.Query(`SELECT name FROM sites ORDER BY created_at DESC LIMIT 2`)
	if err != nil {
		t.Fatalf("order query: %v", err)
	}
	defer rows.Close()
	var order []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		order = append(order, name)
	}
	if len(order) != 2 || order[0] != "go-site" {
		t.Errorf("ORDER BY created_at DESC = %v, want go-site first", order)
	}

	// Idempotent: a second pass rewrites nothing.
	changed2, err := normalizeLegacyTimestamps(db)
	if err != nil {
		t.Fatalf("second normalize: %v", err)
	}
	if changed2 != 0 {
		t.Errorf("second pass changed %d rows, want 0", changed2)
	}
}

// TestNormalizeLegacyTimestamps_SweepIsNotJournalGated pins the sweep into
// AutoMigrate itself. A journal gate decides "already applied" from the state
// of the database at the moment it runs, which is exactly wrong here: a copy
// migration books the step on an empty target and only then inserts the
// TS-shaped rows, so the gate used to guarantee the rewrite never touched
// them. The sweep must therefore run on every boot, and must pick up values
// that appear after the previous boot.
func TestNormalizeLegacyTimestamps_SweepIsNotJournalGated(t *testing.T) {
	db, err := Open(DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	for _, step := range enterpriseAdditiveSteps {
		if step.Version == "sc2_029_ts_timestamp_normalization" {
			t.Fatalf("sc2_029 is journal-gated again: the sweep must run on every boot, not once per database")
		}
	}

	// A TS-shaped value written after the first boot (what a copy migration
	// does into an already-migrated target) is normalized by the next boot.
	if _, err := db.Exec(db.Rebind(`INSERT INTO sites (id, name, url, platform, status, created_at, updated_at)
		VALUES (990002, 'late-ts', 'https://late-ts.example.com', 'openai', 'active', '2026-08-20 12:31:20', '2026-08-20 12:31:20')`)); err != nil {
		t.Fatalf("seed late TS row: %v", err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("second AutoMigrate: %v", err)
	}
	var got string
	if err := db.QueryRow(db.Rebind(`SELECT created_at FROM sites WHERE id = 990002`)).Scan(&got); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got != "2026-08-20T12:31:20Z" {
		t.Errorf("created_at after restart = %q, want RFC3339 '2026-08-20T12:31:20Z'", got)
	}
}

// TestNormalizeLegacyTimestamps_Postgres exercises the PG introspection path
// (information_schema + text-type filter). Gated like the other PG tests.
func TestNormalizeLegacyTimestamps_Postgres(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PG_TEST_DSN"))
	if dsn == "" {
		t.Skip("PG_TEST_DSN not set; skipping PostgreSQL integration test")
	}
	db, err := Open(DialectPostgres, dsn, false)
	if err != nil {
		t.Fatalf("Open pg: %v", err)
	}
	defer db.Close()
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`INSERT INTO sites (id, name, url, platform, status, created_at, updated_at)
		VALUES (990001, 'ts-pg', 'https://ts-pg.example.com', 'openai', 'active', '2026-08-20 12:31:20', '2026-08-20 12:31:20')`)); err != nil {
		t.Fatalf("seed: %v", err)
	}
	defer func() {
		if _, err := db.Exec(db.Rebind(`DELETE FROM sites WHERE id = 990001`)); err != nil {
			t.Logf("cleanup: %v", err)
		}
	}()

	if _, err := normalizeLegacyTimestamps(db); err != nil {
		t.Fatalf("normalizeLegacyTimestamps pg: %v", err)
	}
	var got string
	if err := db.QueryRow(db.Rebind(`SELECT created_at FROM sites WHERE id = 990001`)).Scan(&got); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got != "2026-08-20T12:31:20Z" {
		t.Errorf("pg created_at = %q, want RFC3339", got)
	}
}
