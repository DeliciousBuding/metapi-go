package main

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

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

// TestBuildersMatchStoreSchema guards against the data-loss drift this
// refactor fixed: every per-table insert builder must carry exactly the
// columns created by store.AutoMigrate. When a future store DDL change
// diverges from cmd/migrate, this test (and the migration-time drift guard)
// fails loudly instead of silently dropping columns.
func TestBuildersMatchStoreSchema(t *testing.T) {
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("Open SQLite: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	if err := verifyBuilderColumnsMatchTarget(db); err != nil {
		t.Fatalf("cmd/migrate builders drifted from store DDL: %v", err)
	}
}

// TestBuildSitesIncludesPreviouslyDroppedColumns pins the exact regression:
// before the refactor buildSites copied only 15 columns and silently dropped
// max_concurrency, the post_refresh_probe_* group, custom_headers_override_request_headers
// and tags.
func TestBuildSitesIncludesPreviouslyDroppedColumns(t *testing.T) {
	stmts := buildSites([]map[string]interface{}{{}})
	if len(stmts) != 1 {
		t.Fatalf("buildSites produced %d stmts, want 1", len(stmts))
	}
	got := make(map[string]bool, len(stmts[0].columns))
	for _, c := range stmts[0].columns {
		got[c] = true
	}
	for _, required := range []string{
		"max_concurrency",
		"post_refresh_probe_enabled",
		"post_refresh_probe_model",
		"post_refresh_probe_scope",
		"post_refresh_probe_latency_threshold_ms",
		"custom_headers_override_request_headers",
		"tags",
	} {
		if !got[required] {
			t.Errorf("buildSites is missing column %q", required)
		}
	}
}
