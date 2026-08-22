package store

import (
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

// runVerifyAgainstTarget exercises the exact checksum verification that
// RunMigration performs after commit (step 12: verifyChecksums of the source
// snapshot against the migrated target), but pointed at an already-migrated
// target so external tampering after the migration can be detected. The CLI
// has no verify-only mode; this helper keeps that machinery testable.
func runVerifyAgainstTarget(sourcePath string, targetDB *DB) error {
	sourceDB, err := openSourceDB(sourcePath)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer sourceDB.Close()

	snapshot, err := readAllTables(sourceDB)
	if err != nil {
		return fmt.Errorf("read source: %w", err)
	}
	return verifyChecksums(sourceDB, targetDB, snapshot)
}

// TestVerifyDetectsTamperedSQLiteTarget guards the --verify checksum path:
// after a verified migration succeeds, tampering with one target row must
// make the verification machinery fail with a checksum mismatch instead of
// silently passing.
func TestVerifyDetectsTamperedSQLiteTarget(t *testing.T) {
	fixture := copyTSFixture(t)
	target := filepath.Join(t.TempDir(), "target.db")

	if _, err := RunMigration(RunMigrationOptions{
		FromPath:  fixture,
		ToURL:     target,
		Overwrite: true,
		Verify:    true,
		LogWriter: io.Discard,
	}); err != nil {
		t.Fatalf("baseline migration with verify failed: %v", err)
	}

	targetDB, err := Open(DialectSQLite, target, false)
	if err != nil {
		t.Fatalf("open migrated SQLite target: %v", err)
	}
	defer targetDB.Close()

	if _, err := targetDB.Exec(`UPDATE sites SET name = ? WHERE id = 1`, "tampered-by-test"); err != nil {
		t.Fatalf("tamper target row: %v", err)
	}

	err = runVerifyAgainstTarget(fixture, targetDB)
	if err == nil {
		t.Fatal("verifyChecksums on tampered target = nil, want checksum mismatch error")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("want checksum mismatch error, got: %v", err)
	}
}

// TestVerifyDetectsTamperedPGTarget is the PostgreSQL variant: it exercises
// the cross-dialect hash path (hashPGTable) against a tampered PG target.
// Skipped when PG_TEST_DSN is unset.
func TestVerifyDetectsTamperedPGTarget(t *testing.T) {
	fixture := copyTSFixture(t)
	targetDSN := pgTamperCheckDSN(t)

	if _, err := RunMigration(RunMigrationOptions{
		FromPath:  fixture,
		ToURL:     targetDSN,
		Overwrite: true,
		Verify:    true,
		LogWriter: io.Discard,
	}); err != nil {
		t.Fatalf("baseline PG migration with verify failed: %v", err)
	}

	targetDB, err := Open(DialectPostgres, targetDSN, false)
	if err != nil {
		t.Fatalf("open migrated PG target: %v", err)
	}
	defer targetDB.Close()

	if _, err := targetDB.Exec(`UPDATE sites SET name = ? WHERE id = 1`, "tampered-by-test"); err != nil {
		t.Fatalf("tamper PG target row: %v", err)
	}

	err = runVerifyAgainstTarget(fixture, targetDB)
	if err == nil {
		t.Fatal("verifyChecksums on tampered PG target = nil, want checksum mismatch error")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("want checksum mismatch error, got: %v", err)
	}
}

// pgTamperCheckDSN derives a dedicated database from PG_TEST_DSN so the
// tamper test's Overwrite=true never truncates the shared integration
// database. The derived name differs from pgCompatDSN's suffix so the two
// migration-compat tests cannot collide.
func pgTamperCheckDSN(t *testing.T) string {
	t.Helper()
	skipIfNoPG(t)

	base := pgDSN()
	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse PG_TEST_DSN: %v", err)
	}
	dbName := strings.TrimPrefix(u.Path, "/") + "_m_tamper_check"
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
			t.Fatalf("create tamper-check database %s: %v", dbName, err)
		}
	}

	u.Path = "/" + dbName
	return u.String()
}
