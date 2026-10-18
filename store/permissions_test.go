package store

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func requireUnixModes(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no Unix permission bits")
	}
}

// TestEnsureSQLiteFilePermissionsCreatesOwnerOnly pins audit L6: a fresh
// SQLite file must not be readable by other local users.
func TestEnsureSQLiteFilePermissionsCreatesOwnerOnly(t *testing.T) {
	requireUnixModes(t)
	dbPath := filepath.Join(t.TempDir(), "hub.db")
	if err := ensureSQLiteFilePermissions(dbPath); err != nil {
		t.Fatalf("ensureSQLiteFilePermissions: %v", err)
	}
	info, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("new SQLite file mode = %04o, want 0600", got)
	}
}

// TestEnsureSQLiteFilePermissionsTightensLooseFile covers databases created by
// an older build under a permissive umask.
func TestEnsureSQLiteFilePermissionsTightensLooseFile(t *testing.T) {
	requireUnixModes(t)
	dbPath := filepath.Join(t.TempDir(), "hub.db")
	if err := os.WriteFile(dbPath, []byte("stub"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureSQLiteFilePermissions(dbPath); err != nil {
		t.Fatalf("ensureSQLiteFilePermissions: %v", err)
	}
	info, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("tightened SQLite file mode = %04o, want 0600", got)
	}
}

// TestEnsureSQLiteFilePermissionsKeepsStricterFile proves the helper only
// removes group/other bits and never widens an operator's stricter mode.
func TestEnsureSQLiteFilePermissionsKeepsStricterFile(t *testing.T) {
	requireUnixModes(t)
	dbPath := filepath.Join(t.TempDir(), "hub.db")
	if err := os.WriteFile(dbPath, []byte("stub"), 0o400); err != nil {
		t.Fatal(err)
	}
	if err := ensureSQLiteFilePermissions(dbPath); err != nil {
		t.Fatalf("ensureSQLiteFilePermissions: %v", err)
	}
	info, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o400 {
		t.Fatalf("stricter SQLite file mode = %04o, want 0400 (must not widen)", got)
	}
}

// TestProbeSQLiteWritableCreatesOwnerOnlyDirectory checks the data directory
// created during bootstrap/probing is owner-only.
func TestProbeSQLiteWritableCreatesOwnerOnlyDirectory(t *testing.T) {
	requireUnixModes(t)
	dir := filepath.Join(t.TempDir(), "data")
	if err := probeSQLiteWritable(filepath.Join(dir, "hub.db")); err != nil {
		t.Fatalf("probeSQLiteWritable: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("data directory mode = %04o, want 0700", got)
	}
}
