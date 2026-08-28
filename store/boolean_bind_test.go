package store

import (
	"strings"
	"testing"
	"time"
)

// Incident class under test (w18-pg-dialect audit): a shared statement binds
// the INTEGER literal 0/1 to a BOOLEAN column. SQLite declares its boolean
// columns as INTEGER (see schema_ddl.go), so the literal is silently accepted;
// PostgreSQL uses native BOOLEAN and rejects the row with
// "column ... is of type boolean but expression is of type integer".
//
// These tests pin both halves of the divergence and prove the dialect-safe
// replacements used by the fixes: FALSE/TRUE literals and Go bool binds.

func boolBindProbeNow() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
}

// insertEventsReadStmt builds an INSERT into events whose read column gets
// readExpr verbatim (literal path) so each dialect sees the exact statement.
func insertEventsReadStmt(readExpr string) string {
	return "INSERT INTO events (type, title, message, level, related_type, created_at, read) " +
		"VALUES ('test', 'bool-bind-probe', 'dialect trap probe', 'info', 'test', '" + boolBindProbeNow() + "', " + readExpr + ")"
}

// insertEventsReadPlaceholderStmt is the all-placeholder twin of
// insertEventsReadStmt so the bound-bool form exercises identical columns.
func insertEventsReadPlaceholderStmt() string {
	return "INSERT INTO events (type, title, message, level, related_type, created_at, read) VALUES (?, ?, ?, ?, ?, ?, ?)"
}

// TestBooleanLiteralIntegerAcceptedBySQLite documents the hiding half of the
// incident: on SQLite a literal 0/1 for a boolean column succeeds because the
// column type is INTEGER.
func TestBooleanLiteralIntegerAcceptedBySQLite(t *testing.T) {
	db, err := Open(DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	if _, err := db.Exec(insertEventsReadStmt("0")); err != nil {
		t.Fatalf("SQLite rejected integer literal 0 for boolean column: %v", err)
	}
	var n int
	if err := db.Get(&n, "SELECT COUNT(*) FROM events WHERE title = 'bool-bind-probe'"); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if n != 1 {
		t.Fatalf("events count = %d, want 1", n)
	}
}

// TestBooleanLiteralIntegerRejectedByPostgres proves the failing half of the
// incident on a real PostgreSQL server (CI test-pg sets PG_TEST_DSN; skipped
// locally without one). The exact statement shape that several shared seeds
// and event writers used must fail with a type error on PG.
func TestBooleanLiteralIntegerRejectedByPostgres(t *testing.T) {
	db := openTestPG(t)

	_, err := db.Exec(insertEventsReadStmt("0"))
	if err == nil {
		t.Fatal("PostgreSQL accepted integer literal 0 for boolean column; want type error")
	}
	low := strings.ToLower(err.Error())
	if !strings.Contains(low, "boolean") && !strings.Contains(low, "integer") {
		t.Fatalf("expected a boolean/integer type error, got: %v", err)
	}
}

// TestEventsReadBooleanSafeFormsBothDialects verifies the dialect-safe forms
// used by the fixes: FALSE/TRUE SQL literals and Go bool parameter binds,
// including WHERE/UPDATE comparisons. Runs on SQLite locally and on
// PostgreSQL under CI test-pg.
func TestEventsReadBooleanSafeFormsBothDialects(t *testing.T) {
	run := func(t *testing.T, db *DB) {
		t.Helper()
		// CI test-pg shares one database across packages (-p 1), so assert
		// deltas on our probe title, not absolute counts.
		baseline := 0
		if err := db.Get(&baseline, db.Rebind("SELECT COUNT(*) FROM events WHERE title = ?"), "bool-bind-probe"); err != nil {
			t.Fatalf("baseline count: %v", err)
		}

		// Literal FALSE is valid in both dialects (SQLite >= 3.23).
		if _, err := db.Exec(insertEventsReadStmt("FALSE")); err != nil {
			t.Fatalf("insert with FALSE literal: %v", err)
		}
		// Go bool parameter bind (the pattern used by service.CreateEvent).
		if _, err := db.Exec(db.Rebind(insertEventsReadPlaceholderStmt()),
			"test", "bool-bind-probe", "dialect trap probe", "info", "test", boolBindProbeNow(), false); err != nil {
			t.Fatalf("insert with bound bool: %v", err)
		}

		var unread int
		if err := db.Get(&unread, db.Rebind("SELECT COUNT(*) FROM events WHERE read = ? AND title = ?"), false, "bool-bind-probe"); err != nil {
			t.Fatalf("count unread with bound bool: %v", err)
		}
		if unread != baseline+2 {
			t.Fatalf("unread = %d, want %d (baseline %d)", unread, baseline+2, baseline)
		}

		if _, err := db.Exec(db.Rebind("UPDATE events SET read = ? WHERE read = ? AND title = ?"), true, false, "bool-bind-probe"); err != nil {
			t.Fatalf("mark-read with bound bools: %v", err)
		}
		if err := db.Get(&unread, db.Rebind("SELECT COUNT(*) FROM events WHERE read = ? AND title = ?"), false, "bool-bind-probe"); err != nil {
			t.Fatalf("re-count unread: %v", err)
		}
		if unread != 0 {
			t.Fatalf("unread after mark-read = %d, want 0", unread)
		}
	}

	t.Run("sqlite", func(t *testing.T) {
		db, err := Open(DialectSQLite, ":memory:", false)
		if err != nil {
			t.Fatalf("open sqlite: %v", err)
		}
		defer db.Close()
		if err := AutoMigrate(db); err != nil {
			t.Fatalf("AutoMigrate: %v", err)
		}
		run(t, db)
	})

	t.Run("postgres", func(t *testing.T) {
		db := openTestPG(t)
		run(t, db)
	})
}
