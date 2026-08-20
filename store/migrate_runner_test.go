package store

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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

// TestRunMigrationVerifyFailureReturnsError pins the CLI-honesty fix: a
// checksum mismatch must surface as an error (and a nil summary) so
// cmd/migrate exits non-zero instead of printing a "warning" and claiming
// success. The mismatch is forced by pre-seeding the target with a row the
// (empty) source lacks; Overwrite=false keeps the seed alive while passing
// the target-state gate (which only inspects sites).
func TestRunMigrationVerifyFailureReturnsError(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "source.db")
	targetPath := filepath.Join(t.TempDir(), "target.db")

	// 1. Source: canonical schema, zero rows.
	srcDB, err := Open(DialectSQLite, sourcePath, false)
	if err != nil {
		t.Fatalf("open source: %v", err)
	}
	if err := AutoMigrate(srcDB); err != nil {
		t.Fatalf("auto-migrate source: %v", err)
	}
	if err := srcDB.Close(); err != nil {
		t.Fatalf("close source: %v", err)
	}

	// 2. Target: canonical schema plus one settings row the source lacks.
	tgtSeed, err := Open(DialectSQLite, targetPath, false)
	if err != nil {
		t.Fatalf("open target seed: %v", err)
	}
	if err := AutoMigrate(tgtSeed); err != nil {
		t.Fatalf("auto-migrate target seed: %v", err)
	}
	if _, err := tgtSeed.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)`, "extra_key", "extra"); err != nil {
		t.Fatalf("seed target settings: %v", err)
	}
	if err := tgtSeed.Close(); err != nil {
		t.Fatalf("close target seed: %v", err)
	}

	// 3. Migrate with --verify: the target's extra settings row must trip the
	// checksum comparison and RunMigration must fail.
	summary, err := RunMigration(RunMigrationOptions{
		FromPath:  sourcePath,
		ToURL:     targetPath,
		Overwrite: false,
		Verify:    true,
		LogWriter: io.Discard,
	})
	if err == nil {
		t.Fatal("RunMigration with --verify succeeded despite a checksum mismatch, want error")
	}
	if summary != nil {
		t.Fatalf("summary = %+v, want nil when verification fails", summary)
	}
}

// TestBuildSummaryReportsTargetDialect pins the dialect fix: the summary must
// describe the TARGET's real dialect (SQLite targets were previously
// mislabeled "postgres").
func TestBuildSummaryReportsTargetDialect(t *testing.T) {
	empty := map[string][]map[string]interface{}{}

	sqliteSummary := buildSummary(empty, "data/hub.db", true)
	if sqliteSummary.Dialect != "sqlite" {
		t.Errorf("SQLite target summary dialect = %q, want sqlite", sqliteSummary.Dialect)
	}

	sqliteURLSummary := buildSummary(empty, "sqlite://data/hub.db", true)
	if sqliteURLSummary.Dialect != "sqlite" {
		t.Errorf("sqlite:// target summary dialect = %q, want sqlite", sqliteURLSummary.Dialect)
	}

	pgSummary := buildSummary(empty, "postgres://host:5432/db", true)
	if pgSummary.Dialect != "postgres" {
		t.Errorf("PostgreSQL target summary dialect = %q, want postgres", pgSummary.Dialect)
	}
}

// TestRunMigrationBatchInsertMatchesRowByRow pins the --batch-size
// implementation: BatchSize=3 must land the same 10 rows a row-by-row
// migration lands (same counts, same hashed content).
func TestRunMigrationBatchInsertMatchesRowByRow(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "source.db")

	srcDB, err := Open(DialectSQLite, sourcePath, false)
	if err != nil {
		t.Fatalf("open source: %v", err)
	}
	if err := AutoMigrate(srcDB); err != nil {
		t.Fatalf("auto-migrate source: %v", err)
	}
	for i := 1; i <= 10; i++ {
		// sites carries a UNIQUE (platform, url) constraint, so every row
		// needs a distinct url.
		url := fmt.Sprintf("https://batch-%02d.example.com", i)
		if _, err := srcDB.Exec(
			`INSERT INTO sites (name, url, platform, status, global_weight) VALUES (?, ?, ?, ?, ?)`,
			fmt.Sprintf("site-%02d", i), url, "openai", "active", float64(i),
		); err != nil {
			t.Fatalf("seed site %d: %v", i, err)
		}
	}
	if err := srcDB.Close(); err != nil {
		t.Fatalf("close source: %v", err)
	}

	batchTarget := filepath.Join(t.TempDir(), "batch-target.db")
	rowTarget := filepath.Join(t.TempDir(), "row-target.db")

	batchSummary, err := RunMigration(RunMigrationOptions{
		FromPath:  sourcePath,
		ToURL:     batchTarget,
		Overwrite: true,
		BatchSize: 3,
		LogWriter: io.Discard,
	})
	if err != nil {
		t.Fatalf("batched RunMigration: %v", err)
	}
	rowSummary, err := RunMigration(RunMigrationOptions{
		FromPath:  sourcePath,
		ToURL:     rowTarget,
		Overwrite: true,
		BatchSize: 0,
		LogWriter: io.Discard,
	})
	if err != nil {
		t.Fatalf("row-by-row RunMigration: %v", err)
	}
	if batchSummary == nil || batchSummary.Rows["sites"] != 10 {
		t.Fatalf("batched summary sites rows = %+v, want 10", batchSummary)
	}
	if rowSummary == nil || rowSummary.Rows["sites"] != 10 {
		t.Fatalf("row-by-row summary sites rows = %+v, want 10", rowSummary)
	}

	// Both targets must contain the identical sites content (hashed under the
	// same canonical serialization verifyChecksums uses).
	batchDB, err := Open(DialectSQLite, batchTarget, false)
	if err != nil {
		t.Fatalf("open batch target: %v", err)
	}
	defer batchDB.Close()
	rowDB, err := Open(DialectSQLite, rowTarget, false)
	if err != nil {
		t.Fatalf("open row target: %v", err)
	}
	defer rowDB.Close()

	batchSnapshot, err := readAllTables(batchDB.DB.DB)
	if err != nil {
		t.Fatalf("read batch target: %v", err)
	}
	rowSnapshot, err := readAllTables(rowDB.DB.DB)
	if err != nil {
		t.Fatalf("read row target: %v", err)
	}
	if !bytes.Equal(hashRows(batchSnapshot["sites"]), hashRows(rowSnapshot["sites"])) {
		t.Fatal("batched migration content differs from row-by-row migration")
	}
}

// TestGroupInsertStatements pins the batching shape: BatchSize splits runs,
// a column-list change breaks a run, and BatchSize <= 0 yields single rows.
func TestGroupInsertStatements(t *testing.T) {
	mk := func(table string, cols []string, n int) []insertStmt {
		var out []insertStmt
		for i := 0; i < n; i++ {
			out = append(out, insertStmt{table: table, columns: cols, values: []interface{}{i}})
		}
		return out
	}
	inserts := append(mk("sites", []string{"id", "name"}, 5),
		mk("accounts", []string{"id", "name"}, 1)...)

	groups := groupInsertStatements(inserts, 3)
	if len(groups) != 3 {
		t.Fatalf("groups = %d, want 3 (3+2 sites, 1 accounts)", len(groups))
	}
	if len(groups[0]) != 3 || len(groups[1]) != 2 || len(groups[2]) != 1 {
		t.Fatalf("group sizes = %d/%d/%d, want 3/2/1", len(groups[0]), len(groups[1]), len(groups[2]))
	}
	if groups[2][0].table != "accounts" {
		t.Fatalf("third group table = %q, want accounts (column-list change must split)", groups[2][0].table)
	}

	single := groupInsertStatements(inserts, 0)
	if len(single) != 6 {
		t.Fatalf("row-by-row groups = %d, want 6", len(single))
	}
	for _, g := range single {
		if len(g) != 1 {
			t.Fatalf("row-by-row group size = %d, want 1", len(g))
		}
	}
}

// TestBuildInsertBatchPGNumbering pins the PostgreSQL batch builder: $N
// placeholders must count across all rows in order.
func TestBuildInsertBatchPGNumbering(t *testing.T) {
	stmts := []insertStmt{
		{table: "sites", columns: []string{"id", "name", "weight"}, values: []interface{}{int64(1), "a", 2.5}},
		{table: "sites", columns: []string{"id", "name", "weight"}, values: []interface{}{int64(2), "b", 3.5}},
	}
	sqlText, args := buildInsertBatchPG(stmts)
	wantTuple := "($1, $2, $3), ($4, $5, $6)"
	if !strings.Contains(sqlText, wantTuple) {
		t.Errorf("batch PG SQL = %q, want it to contain %q", sqlText, wantTuple)
	}
	if len(args) != 6 {
		t.Fatalf("batch PG args = %d, want 6", len(args))
	}
}

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
