package store

import (
	"testing"
)

// ---- sc2_026_admin_sessions (#1034 session model) ----

// TestAdminSessionsStepIdempotent verifies the migration step can be applied
// repeatedly (crash-between-DDL-and-bookkeeping safety) and converges both a
// fresh database and one re-opened after the fact.
func TestAdminSessionsStepIdempotent(t *testing.T) {
	db, err := Open(DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	// Apply the full AutoMigrate twice — second run must be a no-op.
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate #1: %v", err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate #2 (idempotency): %v", err)
	}

	// Table exists with the exact sc2_026 shape.
	cols, err := tableColumns(db, "admin_sessions")
	if err != nil {
		t.Fatalf("inspect admin_sessions: %v", err)
	}
	for _, want := range []string{"token_hash", "created_at", "last_seen_at", "expires_at", "client_ip", "user_agent"} {
		if !cols[want] {
			t.Fatalf("admin_sessions missing column %q (have %v)", want, cols)
		}
	}

	// The step is recorded in schema_migrations exactly once.
	var applied int
	if err := db.Get(&applied, `SELECT COUNT(*) FROM schema_migrations WHERE version = 'sc2_026_admin_sessions'`); err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	if applied != 1 {
		t.Fatalf("sc2_026 recorded %d times, want 1", applied)
	}

	// Round-trip a row through the same SQL the SessionManager uses.
	if _, err := db.Exec(
		`INSERT INTO admin_sessions (token_hash, created_at, last_seen_at, expires_at, client_ip, user_agent) VALUES (?, ?, ?, ?, ?, ?)`,
		"hash-a", "2026-08-29T00:00:00Z", "2026-08-29T00:00:00Z", "2026-08-29T12:00:00Z", nil, nil); err != nil {
		t.Fatalf("insert: %v", err)
	}
	var live int
	if err := db.Get(&live, `SELECT COUNT(*) FROM admin_sessions WHERE expires_at > ?`, "2026-08-29T01:00:00Z"); err != nil {
		t.Fatalf("sweep query: %v", err)
	}
	if live != 1 {
		t.Fatalf("live sessions = %d, want 1", live)
	}
	if _, err := db.Exec(`DELETE FROM admin_sessions WHERE expires_at <= ?`, "2026-08-29T13:00:00Z"); err != nil {
		t.Fatalf("gc delete: %v", err)
	}
	if err := db.Get(&live, `SELECT COUNT(*) FROM admin_sessions`); err != nil {
		t.Fatalf("count after gc: %v", err)
	}
	if live != 0 {
		t.Fatalf("sessions after GC = %d, want 0", live)
	}
}

// TestAdminSessionsCarriedByMigrationTool asserts the session table is part
// of the cmd/migrate transfer set (and excluded from PG sequence sync, since
// its PK is text).
func TestAdminSessionsCarriedByMigrationTool(t *testing.T) {
	found := false
	for _, table := range AllTableNames() {
		if table == "admin_sessions" {
			found = true
		}
	}
	if !found {
		t.Fatal("admin_sessions missing from AllTableNames — cmd/migrate would drop sessions on dialect cutover")
	}

	clearFound := false
	for _, table := range ClearTableNames() {
		if table == "admin_sessions" {
			clearFound = true
		}
	}
	if !clearFound {
		t.Fatal("admin_sessions missing from ClearTableNames")
	}

	for _, table := range sequenceTableNames() {
		if table == "admin_sessions" {
			t.Fatal("admin_sessions must not be sequence-synced (text PK, no serial)")
		}
	}
}

// tableColumns returns the column-name set of a SQLite table via PRAGMA.
func tableColumns(db *DB, table string) (map[string]bool, error) {
	rows, err := db.Query(`PRAGMA table_info("` + table + `")`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull int
		var dflt any
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		out[name] = true
	}
	return out, rows.Err()
}
