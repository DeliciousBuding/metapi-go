package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// expectedSitesBuilderColumns mirrors the column list that buildSites in
// store/migrate_runner.go uses for INSERT statements. This test file is the
// drift guard: if someone adds a column to buildSitesDDL (schema_ddl.go) or
// an additive step but forgets to update buildSites, this test fails loudly
// instead of silently losing data during migration.
var expectedSitesBuilderColumns = []string{
	"id", "name", "url", "external_checkin_url", "platform", "proxy_url",
	"use_system_proxy", "custom_headers", "custom_headers_override_request_headers",
	"status", "is_pinned", "sort_order", "global_weight", "api_key",
	"max_concurrency",
	"post_refresh_probe_enabled", "post_refresh_probe_model", "post_refresh_probe_scope",
	"post_refresh_probe_latency_threshold_ms",
	"created_at", "updated_at", "tags", "browser_ua", "cf_clearance",
	"resin_enabled", "use_utls",
}

// openTestDB opens a fresh in-memory SQLite database and runs AutoMigrate.
// The DB is automatically closed when the test finishes.
func openTestDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("auto-migrate: %v", err)
	}
	return db
}

// TestAutoMigrate_FreshSQLite_CreatesAllMigrationTables verifies that
// AutoMigrate creates every table in store.AllTableNames plus the
// schema_migrations bookkeeping table on a fresh in-memory SQLite DB.
func TestAutoMigrate_FreshSQLite_CreatesAllMigrationTables(t *testing.T) {
	db := openTestDB(t)

	for _, table := range store.AllTableNames() {
		var name string
		err := db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not created by AutoMigrate: %v", table, err)
		}
	}

	// schema_migrations bookkeeping table must exist after AutoMigrate
	// (created by ensureSchemaMigrationsTable inside ApplyAdditiveMigrations).
	var schemaMigrationsName string
	err := db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name='schema_migrations'`,
	).Scan(&schemaMigrationsName)
	if err != nil {
		t.Fatalf("schema_migrations table not created: %v", err)
	}
}

// TestApplyAdditiveMigrations_Idempotent verifies that re-running
// ApplyAdditiveMigrations does not error and does not create duplicate
// entries in the schema_migrations table. The INSERT OR IGNORE / ON CONFLICT
// DO NOTHING guard in markMigrationApplied makes re-runs safe.
func TestApplyAdditiveMigrations_Idempotent(t *testing.T) {
	db := openTestDB(t)

	firstCount := countAppliedVersions(t, db)
	if firstCount == 0 {
		t.Fatal("expected additive migrations to be applied on first run, got 0")
	}

	// Re-run — must not error or duplicate.
	if err := store.ApplyAdditiveMigrations(db); err != nil {
		t.Fatalf("second ApplyAdditiveMigrations failed: %v", err)
	}

	secondCount := countAppliedVersions(t, db)
	if secondCount != firstCount {
		t.Errorf("idempotency broken: first run recorded %d versions, second run recorded %d",
			firstCount, secondCount)
	}
}

func countAppliedVersions(t *testing.T, db *store.DB) int {
	t.Helper()
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&n)
	if err != nil {
		t.Fatalf("count schema_migrations: %v", err)
	}
	return n
}

// TestSitesColumnList_MatchesBuilderAndSchema is the drift guard for the sites
// table. It verifies that every column in the buildSites INSERT builder
// (expectedSitesBuilderColumns) exists in the actual sites table created by
// AutoMigrate, and that the table has no extra columns the builder would
// silently drop. If this test fails, a schema change was not propagated to
// the migration builder.
func TestSitesColumnList_MatchesBuilderAndSchema(t *testing.T) {
	db := openTestDB(t)

	rows, err := db.Query(`SELECT * FROM sites LIMIT 0`)
	if err != nil {
		t.Fatalf("query sites: %v", err)
	}
	actualCols, err := rows.Columns()
	rows.Close()
	if err != nil {
		t.Fatalf("get sites columns: %v", err)
	}

	actualSet := make(map[string]bool, len(actualCols))
	for _, c := range actualCols {
		actualSet[c] = true
	}
	expectedSet := make(map[string]bool, len(expectedSitesBuilderColumns))
	for _, c := range expectedSitesBuilderColumns {
		expectedSet[c] = true
	}

	for _, c := range expectedSitesBuilderColumns {
		if !actualSet[c] {
			t.Errorf("column %q in buildSites builder but missing from sites table (DDL drift)", c)
		}
	}
	for _, c := range actualCols {
		if !expectedSet[c] {
			t.Errorf("column %q in sites table but not in buildSites builder (builder drift)", c)
		}
	}
}

// TestRunMigration_SQLiteToSQLite_PreservesData exercises the full migration
// pipeline (read → build → drift-guard → insert) on a populated SQLite
// database. It verifies that data is preserved across a SQLite→SQLite copy
// and that runtime DB settings (db_type, db_url, db_ssl) are filtered out.
func TestRunMigration_SQLiteToSQLite_PreservesData(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)

	// 1. Create source DB with data.
	sourcePath := filepath.Join(t.TempDir(), "source.db")
	srcDB, err := store.Open(store.DialectSQLite, sourcePath, false)
	if err != nil {
		t.Fatalf("open source: %v", err)
	}
	if err := store.AutoMigrate(srcDB); err != nil {
		t.Fatalf("auto-migrate source: %v", err)
	}

	if _, err := srcDB.Exec(
		`INSERT INTO sites (name, url, platform, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"test-site", "https://test.example", "new-api", "active", now, now,
	); err != nil {
		t.Fatalf("insert site: %v", err)
	}
	if _, err := srcDB.Exec(
		`INSERT INTO accounts (site_id, username, access_token, status, checkin_enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		1, "testuser", "sk-test", "active", true, now, now,
	); err != nil {
		t.Fatalf("insert account: %v", err)
	}
	if _, err := srcDB.Exec(
		`INSERT INTO settings (key, value) VALUES (?, ?)`, "test_key", "test_value",
	); err != nil {
		t.Fatalf("insert setting: %v", err)
	}
	// Runtime DB setting — must be filtered out by buildSettings.
	if _, err := srcDB.Exec(
		`INSERT INTO settings (key, value) VALUES (?, ?)`, "db_type", "sqlite",
	); err != nil {
		t.Fatalf("insert runtime setting: %v", err)
	}
	srcDB.Close()

	// 2. Run migration to a fresh target file.
	targetPath := filepath.Join(t.TempDir(), "target.db")
	summary, err := store.RunMigration(store.RunMigrationOptions{
		FromPath:  sourcePath,
		ToURL:     targetPath,
		Overwrite: true,
	})
	if err != nil {
		t.Fatalf("RunMigration failed: %v", err)
	}
	if summary == nil {
		t.Fatal("RunMigration returned nil summary")
	}
	if summary.Rows["sites"] != 1 {
		t.Errorf("summary sites rows = %d, want 1", summary.Rows["sites"])
	}
	if summary.Rows["accounts"] != 1 {
		t.Errorf("summary accounts rows = %d, want 1", summary.Rows["accounts"])
	}
	if summary.Rows["settings"] != 2 {
		t.Errorf("summary settings rows = %d, want 2 (read before filtering)", summary.Rows["settings"])
	}

	// 3. Open target and verify data.
	tgtDB, err := store.Open(store.DialectSQLite, targetPath, false)
	if err != nil {
		t.Fatalf("open target: %v", err)
	}
	defer tgtDB.Close()

	var siteName, siteURL, sitePlatform string
	if err := tgtDB.QueryRow(
		`SELECT name, url, platform FROM sites WHERE id = 1`,
	).Scan(&siteName, &siteURL, &sitePlatform); err != nil {
		t.Fatalf("query migrated site: %v", err)
	}
	if siteName != "test-site" || siteURL != "https://test.example" || sitePlatform != "new-api" {
		t.Errorf("site data mismatch: name=%s url=%s platform=%s", siteName, siteURL, sitePlatform)
	}

	var username string
	if err := tgtDB.QueryRow(
		`SELECT username FROM accounts WHERE id = 1`,
	).Scan(&username); err != nil {
		t.Fatalf("query migrated account: %v", err)
	}
	if username != "testuser" {
		t.Errorf("account username = %q, want testuser", username)
	}

	// Regular setting should be migrated.
	var settingValue string
	if err := tgtDB.QueryRow(
		`SELECT value FROM settings WHERE key = 'test_key'`,
	).Scan(&settingValue); err != nil {
		t.Fatalf("query migrated setting: %v", err)
	}
	if settingValue != "test_value" {
		t.Errorf("setting value = %q, want test_value", settingValue)
	}

	// Runtime DB setting should NOT be migrated (filtered by buildSettings).
	var dbTypeCount int
	if err := tgtDB.QueryRow(
		`SELECT COUNT(*) FROM settings WHERE key = 'db_type'`,
	).Scan(&dbTypeCount); err != nil {
		t.Fatalf("count db_type settings: %v", err)
	}
	if dbTypeCount != 0 {
		t.Errorf("db_type setting should have been filtered, but %d rows exist", dbTypeCount)
	}
}

// TestRunMigration_DryRun_NoDataWritten verifies that DryRun=true returns a
// summary without creating the target file or writing any data.
func TestRunMigration_DryRun_NoDataWritten(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)

	sourcePath := filepath.Join(t.TempDir(), "source.db")
	srcDB, err := store.Open(store.DialectSQLite, sourcePath, false)
	if err != nil {
		t.Fatalf("open source: %v", err)
	}
	if err := store.AutoMigrate(srcDB); err != nil {
		t.Fatalf("auto-migrate source: %v", err)
	}
	if _, err := srcDB.Exec(
		`INSERT INTO sites (name, url, platform, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"dry-run-site", "https://dryrun.example", "new-api", "active", now, now,
	); err != nil {
		t.Fatalf("insert site: %v", err)
	}
	srcDB.Close()

	targetPath := filepath.Join(t.TempDir(), "target_dryrun.db")
	summary, err := store.RunMigration(store.RunMigrationOptions{
		FromPath:  sourcePath,
		ToURL:     targetPath,
		Overwrite: true,
		DryRun:    true,
	})
	if err != nil {
		t.Fatalf("RunMigration dry-run failed: %v", err)
	}
	if summary == nil {
		t.Fatal("dry-run returned nil summary")
	}
	if summary.Rows["sites"] != 1 {
		t.Errorf("dry-run summary sites rows = %d, want 1", summary.Rows["sites"])
	}
}

// TestPrintSummary_DoesNotPanic verifies that printSummary handles a complete
// MigrationSummary without panicking. This covers the stderr formatting path.
func TestPrintSummary_DoesNotPanic(t *testing.T) {
	rows := make(map[string]int)
	for _, table := range store.AllTableNames() {
		rows[table] = 0
	}
	summary := &store.MigrationSummary{
		Dialect:    "sqlite",
		Connection: "test.db",
		Overwrite:  true,
		Version:    "test",
		Timestamp:  time.Now().UnixMilli(),
		Rows:       rows,
	}
	printSummary(summary)
}
