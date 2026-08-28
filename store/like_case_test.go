package store

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// Dialect divergence under test (w18-pg-dialect audit): SQLite LIKE matches
// case-insensitively for ASCII, PostgreSQL LIKE is case-sensitive. Shared
// filters therefore must normalize both sides with LOWER() — otherwise the
// same admin query returns rows on SQLite and nothing on PostgreSQL.

// likeProbeSeedSite inserts a site whose name embeds probe (mixed case) so
// each test owns a unique row even on the shared CI test-pg database.
func likeProbeSeedSite(t *testing.T, db *DB, probe string) {
	t.Helper()
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	name := "OpenAI " + probe
	if _, err := db.Exec(db.Rebind(
		"INSERT INTO sites (name, url, platform, status, created_at, updated_at) VALUES (?, ?, 'openai', 'active', ?, ?)"),
		name, "https://"+strings.ToLower(probe)+".example.com", now, now); err != nil {
		t.Fatalf("seed site: %v", err)
	}
}

// TestLikeCaseInsensitiveOnSQLite documents the SQLite half: a lowercase
// pattern matches a mixed-case name without LOWER() normalization.
func TestLikeCaseInsensitiveOnSQLite(t *testing.T) {
	db, err := Open(DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	probe := fmt.Sprintf("CaseProbe-sqlite-%d", time.Now().UnixNano())
	likeProbeSeedSite(t, db, probe)

	var n int
	if err := db.Get(&n, db.Rebind("SELECT COUNT(*) FROM sites WHERE name LIKE ?"), "%"+strings.ToLower(probe)+"%"); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("SQLite LIKE matched %d rows, want 1 (case-insensitive ASCII LIKE)", n)
	}
}

// TestLikeCaseSensitiveOnPostgres proves the divergent half on a real
// PostgreSQL server (CI test-pg sets PG_TEST_DSN; skipped locally without
// one): the identical un-normalized predicate returns zero rows.
func TestLikeCaseSensitiveOnPostgres(t *testing.T) {
	db := openTestPG(t)
	probe := fmt.Sprintf("CaseProbe-pg-%d", time.Now().UnixNano())
	likeProbeSeedSite(t, db, probe)

	var n int
	if err := db.Get(&n, db.Rebind("SELECT COUNT(*) FROM sites WHERE name LIKE ?"), "%"+strings.ToLower(probe)+"%"); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Fatalf("PostgreSQL LIKE matched %d rows, want 0 (case-sensitive LIKE)", n)
	}
}

// TestLowerNormalizedLikeMatchesBothDialects verifies the fix shape used by
// the shared search/filter queries: LOWER(column) LIKE lowercased-pattern
// matches on both dialects.
func TestLowerNormalizedLikeMatchesBothDialects(t *testing.T) {
	run := func(t *testing.T, db *DB) {
		t.Helper()
		probe := fmt.Sprintf("CaseProbe-%s-%d", strings.ReplaceAll(t.Name(), "/", "-"), time.Now().UnixNano())
		likeProbeSeedSite(t, db, probe)
		var n int
		if err := db.Get(&n, db.Rebind("SELECT COUNT(*) FROM sites WHERE LOWER(name) LIKE ?"), "%"+strings.ToLower(probe)+"%"); err != nil {
			t.Fatalf("count: %v", err)
		}
		if n != 1 {
			t.Fatalf("LOWER-normalized LIKE matched %d rows, want 1", n)
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
