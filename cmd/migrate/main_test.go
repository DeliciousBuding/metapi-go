package main

import "testing"

// ---- Reverse migration (PG → SQLite) helpers, 2026-08-01 ----

func TestIsPostgresURL(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{"postgres://u:p@h:5432/db", true},
		{"postgresql://u:p@h/db", true},
		{"C:/tmp/hub.db", false},
		{"sqlite://data/hub.db", false},
		{"data/hub.db", false},
	}
	for _, c := range cases {
		if got := isPostgresURL(c.raw); got != c.want {
			t.Errorf("isPostgresURL(%q) = %v, want %v", c.raw, got, c.want)
		}
	}
}

func TestSQLiteDDLFromPG(t *testing.T) {
	pg := `CREATE TABLE IF NOT EXISTS "sites" (id BIGSERIAL PRIMARY KEY, name TEXT, use_system_proxy BOOLEAN DEFAULT FALSE, sort_order BIGINT DEFAULT 0, global_weight DOUBLE PRECISION DEFAULT 1, custom_headers JSONB, enabled BOOLEAN DEFAULT TRUE, created_at TEXT)`
	got := sqliteDDLFromPG(pg)
	want := `CREATE TABLE IF NOT EXISTS "sites" (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT, use_system_proxy INTEGER DEFAULT 0, sort_order INTEGER DEFAULT 0, global_weight REAL DEFAULT 1, custom_headers TEXT, enabled INTEGER DEFAULT 1, created_at TEXT)`
	if got != want {
		t.Fatalf("sqliteDDLFromPG mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestSQLiteDDLFromPG_NoBogusReplacement(t *testing.T) {
	// BIGINT must not corrupt other tokens; JSONB→TEXT must not touch TEXT columns.
	pg := `CREATE TABLE IF NOT EXISTS "proxy_logs" (id BIGSERIAL PRIMARY KEY, prompt_tokens BIGINT, billing_details JSONB, created_at TEXT)`
	got := sqliteDDLFromPG(pg)
	for _, bad := range []string{"BIGSERIAL", "BIGINT", "JSONB", "BOOLEAN", "DOUBLE PRECISION"} {
		if contains(got, bad) {
			t.Errorf("sqliteDDLFromPG still contains %q: %s", bad, got)
		}
	}
	if !contains(got, "INTEGER PRIMARY KEY AUTOINCREMENT") {
		t.Errorf("missing AUTOINCREMENT mapping: %s", got)
	}
	if !contains(got, "TEXT") {
		t.Errorf("JSONB→TEXT mapping failed: %s", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
