package store

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

// TestRunMigration_SQLiteToSQLiteRoundTrip guards the extraction of
// runMigration into the package-level RunMigration callable: a source SQLite
// DB with a sites row + a non-runtime setting must land in a fresh target
// SQLite DB with identical row counts. The migration SQL/logic is unchanged
// from the CLI; this test only proves the callable form wires the same path.
func TestRunMigration_SQLiteToSQLiteRoundTrip(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "source.db")
	targetPath := filepath.Join(t.TempDir(), "target.db")

	// 1. Build a source DB with the canonical schema and seed two tables:
	//    one sites row and one non-runtime settings row (runtime keys
	//    db_type/db_url/db_ssl are intentionally skipped by the migration).
	srcDB, err := Open(DialectSQLite, sourcePath, false)
	if err != nil {
		t.Fatalf("open source: %v", err)
	}
	if err := AutoMigrate(srcDB); err != nil {
		t.Fatalf("auto-migrate source: %v", err)
	}
	if _, err := srcDB.Exec(`INSERT INTO sites (name, url, platform, status, global_weight) VALUES (?, ?, ?, ?, ?)`,
		"Test Site", "https://example.com", "openai", "active", 2.0); err != nil {
		t.Fatalf("seed sites: %v", err)
	}
	if _, err := srcDB.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)`,
		"system_name", `"metapi-test"`); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
	if err := srcDB.Close(); err != nil {
		t.Fatalf("close source: %v", err)
	}

	// 2. Run the callable migration (SQLite → SQLite copy).
	summary, err := RunMigration(RunMigrationOptions{
		FromPath:  sourcePath,
		ToURL:     targetPath,
		Overwrite: true,
		Progress:  true,
		Verify:    false,
		LogWriter: io.Discard,
	})
	if err != nil {
		t.Fatalf("RunMigration: %v", err)
	}
	if summary == nil {
		t.Fatal("RunMigration returned nil summary")
	}
	if got := summary.Rows["sites"]; got != 1 {
		t.Fatalf("summary sites rows = %d, want 1", got)
	}
	if got := summary.Rows["settings"]; got != 1 {
		t.Fatalf("summary settings rows = %d, want 1", got)
	}

	// 3. Open the target and assert the seeded rows landed.
	tgtDB, err := Open(DialectSQLite, targetPath, false)
	if err != nil {
		t.Fatalf("open target: %v", err)
	}
	defer tgtDB.Close()

	var (
		siteID           int
		siteName         string
		siteURL          string
		sitePlatform     string
		siteStatus       string
		siteGlobalWeight float64
	)
	if err := tgtDB.QueryRow(`SELECT id, name, url, platform, status, global_weight FROM sites WHERE name = ?`, "Test Site").
		Scan(&siteID, &siteName, &siteURL, &sitePlatform, &siteStatus, &siteGlobalWeight); err != nil {
		t.Fatalf("query migrated site: %v", err)
	}
	if siteName != "Test Site" || siteURL != "https://example.com" || sitePlatform != "openai" {
		t.Fatalf("migrated site row = %q/%q/%q, want Test Site/https://example.com/openai", siteName, siteURL, sitePlatform)
	}
	if siteStatus != "active" {
		t.Fatalf("migrated site status = %q, want active", siteStatus)
	}
	if siteGlobalWeight != 2.0 {
		t.Fatalf("migrated site global_weight = %v, want 2.0", siteGlobalWeight)
	}
	_ = siteID

	var settingsValue string
	if err := tgtDB.QueryRow(`SELECT value FROM settings WHERE key = ?`, "system_name").Scan(&settingsValue); err != nil {
		t.Fatalf("query migrated setting: %v", err)
	}
	if settingsValue != `"metapi-test"` {
		t.Fatalf("migrated settings value = %q, want %q", settingsValue, `"metapi-test"`)
	}

	// 4. Runtime DB setting keys must NOT migrate (the migration filters them).
	var runtimeCount int
	for _, key := range []string{"db_type", "db_url", "db_ssl"} {
		var n int
		if err := tgtDB.QueryRow(`SELECT COUNT(*) FROM settings WHERE key = ?`, key).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", key, err)
		}
		runtimeCount += n
	}
	if runtimeCount != 0 {
		t.Fatalf("runtime DB setting keys migrated, count=%d (should be filtered)", runtimeCount)
	}
}

// TestRunMigration_DryRunWritesNothing asserts the DryRun path returns a
// summary without creating or modifying the target DB.
func TestRunMigration_DryRunWritesNothing(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "source.db")
	targetPath := filepath.Join(t.TempDir(), "target.db")

	srcDB, err := Open(DialectSQLite, sourcePath, false)
	if err != nil {
		t.Fatalf("open source: %v", err)
	}
	if err := AutoMigrate(srcDB); err != nil {
		t.Fatalf("auto-migrate source: %v", err)
	}
	if _, err := srcDB.Exec(`INSERT INTO sites (name, url, platform) VALUES (?, ?, ?)`,
		"Dry Run Site", "https://dry.example.com", "openai"); err != nil {
		t.Fatalf("seed sites: %v", err)
	}
	if err := srcDB.Close(); err != nil {
		t.Fatalf("close source: %v", err)
	}

	summary, err := RunMigration(RunMigrationOptions{
		FromPath:  sourcePath,
		ToURL:     targetPath,
		Overwrite: true,
		DryRun:    true,
		LogWriter: io.Discard,
	})
	if err != nil {
		t.Fatalf("RunMigration dry-run: %v", err)
	}
	if summary == nil {
		t.Fatal("dry-run returned nil summary")
	}
	if got := summary.Rows["sites"]; got != 1 {
		t.Fatalf("dry-run summary sites rows = %d, want 1", got)
	}

	// The target file must not exist (dry-run opens no target).
	if _, statErr := os.Stat(targetPath); statErr == nil {
		t.Fatal("dry-run created the target file, want no writes")
	}
}

// ---- Helpers moved from cmd/migrate/main_test.go (now package store) ----
// These guard the unexported migration helpers that the CLI extraction moved
// into the store package. They run in-package so they can reach the
// unexported symbols (isPostgresURL, buildSites, verifyBuilderColumnsMatchTarget).

// TestIsPostgresURL pins the dialect-detection helper used to decide forward
// vs. reverse migration direction. Test URLs omit credentials so they don't
// trip the leak-guard credential-URL rule; isPostgresURL only checks the
// scheme prefix.
func TestIsPostgresURL(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{"postgres://host:5432/db", true},
		{"postgresql://host/db", true},
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
// columns created by AutoMigrate. When a future store DDL change diverges
// from the migration builders, this test (and the migration-time drift guard)
// fails loudly instead of silently dropping columns.
func TestBuildersMatchStoreSchema(t *testing.T) {
	db, err := Open(DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("Open SQLite: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	if err := verifyBuilderColumnsMatchTarget(db); err != nil {
		t.Fatalf("migration builders drifted from store DDL: %v", err)
	}
}

// TestBuildSitesIncludesPreviouslyDroppedColumns pins the exact regression:
// before the refactor buildSites copied only 15 columns and silently dropped
// max_concurrency, the post_refresh_probe_* group,
// custom_headers_override_request_headers and tags.
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
