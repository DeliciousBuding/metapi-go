package store

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
)

// The TS→Go migration compatibility suite. It verifies the two ways a real
// TypeScript-version database lands in the Go runtime:
//
//  1. Direct takeover (issue #849): the Go server boots on the TS database
//     in place; AutoMigrate adds the Go-only columns without touching data.
//  2. metapi-migrate copy: RunMigration moves the TS database into a fresh
//     SQLite/PostgreSQL target and --verify confirms the checksums.
//
// The source is a GOLDEN FIXTURE (testdata/ts-source/hub.db): a real
// cita-777/metapi database — schema built by its own drizzle migrations and
// data written through its admin API. All data is dummy. The fixture is
// committed so the tests are reusable without Node or a TS checkout; rebuild
// it with scripts/regen-ts-fixture.sh when the TS schema evolves.

const tsFixtureDir = "testdata/ts-source"

// tsFixtureManifest mirrors manifest.json, written by regen-ts-fixture.sh
// straight from the fixture database.
type tsFixtureManifest struct {
	Description        string `json:"description"`
	GeneratedAt        string `json:"generated_at"`
	TableCount         int    `json:"table_count"`
	SiteCount          int    `json:"site_count"`
	AccountCount       int    `json:"account_count"`
	DownstreamKeyCount int    `json:"downstream_key_count"`
	CheckSites         []struct {
		Name     string `json:"name"`
		URL      string `json:"url"`
		Platform string `json:"platform"`
	} `json:"check_sites"`
	CheckAccounts []struct {
		Username string `json:"username"`
		SiteName string `json:"site_name"`
	} `json:"check_accounts"`
	CheckDownstreamKeys []struct {
		Name string `json:"name"`
	} `json:"check_downstream_keys"`
}

func loadTSFixtureManifest(t *testing.T) *tsFixtureManifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(tsFixtureDir, "manifest.json"))
	if err != nil {
		t.Fatalf("read fixture manifest: %v (run scripts/regen-ts-fixture.sh)", err)
	}
	var manifest tsFixtureManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("parse fixture manifest: %v", err)
	}
	return &manifest
}

// copyTSFixture copies the golden fixture into a fresh temp dir so tests never
// mutate the committed database.
func copyTSFixture(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile(filepath.Join(tsFixtureDir, "hub.db"))
	if err != nil {
		t.Fatalf("read golden fixture: %v (run scripts/regen-ts-fixture.sh)", err)
	}
	dir := t.TempDir()
	dst := filepath.Join(dir, "hub.db")
	if err := os.WriteFile(dst, src, 0o600); err != nil {
		t.Fatalf("copy golden fixture: %v", err)
	}
	return dst
}

// assertFixtureData checks the migrated/taken-over database against the
// manifest: table counts plus identity spot-checks (sites, accounts linked
// to the right site, downstream keys). Works on both dialects (Exec/Query
// rebind placeholders automatically).
func assertFixtureData(t *testing.T, db *DB, manifest *tsFixtureManifest) {
	t.Helper()

	counts := map[string]int{
		"sites":               manifest.SiteCount,
		"accounts":            manifest.AccountCount,
		"downstream_api_keys": manifest.DownstreamKeyCount,
	}
	for table, want := range counts {
		var got int
		if err := db.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM "%s"`, table)).Scan(&got); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if got != want {
			t.Errorf("table %s has %d rows, manifest says %d", table, got, want)
		}
	}

	for _, site := range manifest.CheckSites {
		var found int
		err := db.QueryRow(
			`SELECT COUNT(*) FROM sites WHERE name = ? AND url = ? AND platform = ?`,
			site.Name, site.URL, site.Platform,
		).Scan(&found)
		if err != nil {
			t.Fatalf("spot-check site %q: %v", site.Name, err)
		}
		if found == 0 {
			t.Errorf("site %q (%s) missing after migration", site.Name, site.URL)
		}
	}

	for _, account := range manifest.CheckAccounts {
		var found int
		err := db.QueryRow(
			`SELECT COUNT(*) FROM accounts a JOIN sites s ON s.id = a.site_id WHERE a.username = ? AND s.name = ?`,
			account.Username, account.SiteName,
		).Scan(&found)
		if err != nil {
			t.Fatalf("spot-check account %q: %v", account.Username, err)
		}
		if found == 0 {
			t.Errorf("account %q (site %q) missing after migration", account.Username, account.SiteName)
		}
	}

	for _, key := range manifest.CheckDownstreamKeys {
		var found int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM downstream_api_keys WHERE name = ?`,
			key.Name,
		).Scan(&found); err != nil {
			t.Fatalf("spot-check downstream key %q: %v", key.Name, err)
		}
		if found == 0 {
			t.Errorf("downstream key %q missing after migration", key.Name)
		}
	}
}

// assertAdditiveMigrationsRecorded checks that every registered additive
// step version appears in schema_migrations — the AutoMigrate takeover path
// must converge an old TS schema onto the current Go schema.
func assertAdditiveMigrationsRecorded(t *testing.T, db *DB) {
	t.Helper()
	for _, step := range enterpriseAdditiveSteps {
		var found int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`,
			step.Version,
		).Scan(&found); err != nil {
			t.Fatalf("check schema_migrations for %s: %v", step.Version, err)
		}
		if found == 0 {
			t.Errorf("additive migration %s not recorded in schema_migrations", step.Version)
		}
	}
}

// TestTSTakeoverAutoMigrate is the issue #849 scenario: the Go server boots
// directly on a TypeScript-version database and AutoMigrate converges the
// schema in place without touching the data.
func TestTSTakeoverAutoMigrate(t *testing.T) {
	manifest := loadTSFixtureManifest(t)
	fixture := copyTSFixture(t)
	t.Cleanup(func() { _ = CloseDatabase() })

	cfg := &config.Config{DataDir: filepath.Dir(fixture)}
	rt := &config.RuntimeSettings{DbType: DialectSQLite}
	if err := EnsureRuntimeDatabase(cfg, rt); err != nil {
		t.Fatalf("EnsureRuntimeDatabase on TS fixture failed: %v", err)
	}

	db := GetDB()
	if db == nil {
		t.Fatal("GetDB() returned nil after takeover boot")
	}
	assertAdditiveMigrationsRecorded(t, db)
	assertFixtureData(t, db, manifest)
}

// TestTSMigrateVerifySQLite runs metapi-migrate's Go API against the golden
// TS fixture into a fresh SQLite target with checksum verification.
func TestTSMigrateVerifySQLite(t *testing.T) {
	manifest := loadTSFixtureManifest(t)
	fixture := copyTSFixture(t)
	target := filepath.Join(t.TempDir(), "target.db")

	var logLines strings.Builder
	summary, err := RunMigration(RunMigrationOptions{
		FromPath:  fixture,
		ToURL:     target,
		Overwrite: true,
		Verify:    true,
		LogWriter: &logLines,
	})
	if err != nil {
		t.Fatalf("RunMigration SQLite failed: %v\nlog:\n%s", err, logLines.String())
	}

	if got := summary.Rows["sites"]; got != manifest.SiteCount {
		t.Errorf("summary sites rows = %d, want %d", got, manifest.SiteCount)
	}
	if got := summary.Rows["accounts"]; got != manifest.AccountCount {
		t.Errorf("summary accounts rows = %d, want %d", got, manifest.AccountCount)
	}

	db, err := Open(DialectSQLite, target, false)
	if err != nil {
		t.Fatalf("open migrated SQLite target: %v", err)
	}
	defer db.Close()
	assertFixtureData(t, db, manifest)
}

// pgCompatDSN derives a dedicated database from PG_TEST_DSN so the migration
// test's Overwrite=true never truncates the shared integration database used
// by the rest of the PG suite.
func pgCompatDSN(t *testing.T) string {
	t.Helper()
	skipIfNoPG(t)

	base := pgDSN()
	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse PG_TEST_DSN: %v", err)
	}
	dbName := strings.TrimPrefix(u.Path, "/") + "_ts_compat"
	if strings.ContainsAny(dbName, `"`+"`"+"'") {
		t.Fatalf("unsafe derived db name %q", dbName)
	}

	admin, err := Open(DialectPostgres, base, false)
	if err != nil {
		t.Fatalf("open admin PG connection: %v", err)
	}
	defer admin.Close()

	var exists bool
	if err := admin.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = ?)`,
		dbName,
	).Scan(&exists); err != nil {
		t.Fatalf("check pg_database for %s: %v", dbName, err)
	}
	if !exists {
		if _, err := admin.Exec(fmt.Sprintf(`CREATE DATABASE "%s"`, dbName)); err != nil {
			t.Fatalf(
				"create compat database %s: %v — the PG_TEST_DSN user needs CREATEDB "+
					"(CI's postgres superuser has it), or pre-create the database manually",
				dbName, err,
			)
		}
	}

	u.Path = "/" + dbName
	return u.String()
}

// TestTSMigrateVerifyPG runs metapi-migrate's Go API against the golden TS
// fixture into a dedicated PostgreSQL database with checksum verification.
// Skipped when PG_TEST_DSN is unset (local SQLite-only runs and CI shards).
func TestTSMigrateVerifyPG(t *testing.T) {
	manifest := loadTSFixtureManifest(t)
	fixture := copyTSFixture(t)
	targetDSN := pgCompatDSN(t)

	summary, err := RunMigration(RunMigrationOptions{
		FromPath:  fixture,
		ToURL:     targetDSN,
		Overwrite: true,
		Verify:    true,
		LogWriter: io.Discard,
	})
	if err != nil {
		t.Fatalf("RunMigration PG failed: %v", err)
	}

	if got := summary.Rows["sites"]; got != manifest.SiteCount {
		t.Errorf("summary sites rows = %d, want %d", got, manifest.SiteCount)
	}
	if got := summary.Rows["accounts"]; got != manifest.AccountCount {
		t.Errorf("summary accounts rows = %d, want %d", got, manifest.AccountCount)
	}

	db, err := Open(DialectPostgres, targetDSN, false)
	if err != nil {
		t.Fatalf("open migrated PG target: %v", err)
	}
	defer db.Close()
	assertFixtureData(t, db, manifest)
}
