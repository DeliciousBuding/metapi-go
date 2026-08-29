package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
)

// resetActiveDBForTest closes any active DB singleton so each SwitchDatabase
// test starts from a clean global state. The activeDB/initialized globals are
// package-private, so tests cannot run in parallel with each other.
func resetActiveDBForTest(t *testing.T) {
	t.Helper()
	if err := CloseDatabase(); err != nil {
		t.Logf("close before test: %v", err)
	}
}

// newSwitchTestConfig builds a Config that points EnsureRuntimeDatabase at a
// fresh SQLite file inside dir. All SwitchDatabase tests here are sqlite-only
// (no PostgreSQL dependency).
func newSwitchTestConfig(t *testing.T, dir, fileName string) (*config.Config, string) {
	t.Helper()
	dbPath := filepath.Join(dir, fileName)
	cfg := &config.Config{DataDir: dir}
	// SwitchDatabase reads and writes the connection settings through the
	// global atomic RuntimeSettings snapshot, so each test publishes its
	// baseline there (cleared again on cleanup).
	config.SetRuntime(&config.RuntimeSettings{
		DbType: DialectSQLite,
		DbUrl:  dbPath,
	})
	t.Cleanup(func() { config.SetRuntime(nil) })
	return cfg, dbPath
}

// ensureRuntimeOrFail wraps EnsureRuntimeDatabase with a fatal t helper so a
// setup failure stops the test instead of cascading into switch assertions.
func ensureRuntimeOrFail(t *testing.T, cfg *config.Config) {
	t.Helper()
	if err := EnsureRuntimeDatabase(cfg, config.Runtime()); err != nil {
		t.Fatalf("EnsureRuntimeDatabase failed: %v", err)
	}
	if GetDB() == nil {
		t.Fatal("GetDB() == nil after EnsureRuntimeDatabase")
	}
}

// TestSwitchDatabase_Success_SqliteToSqlite verifies a successful runtime
// switch from one SQLite file to another: the old pool closes, the new DB
// opens + migrates, and queries run against the new DB (the old marker is
// absent because it is a different database file).
func TestSwitchDatabase_Success_SqliteToSqlite(t *testing.T) {
	resetActiveDBForTest(t)
	// defer (not t.Cleanup) so the active DB is closed before t.TempDir's
	// RemoveAll cleanup tries to delete the still-open SQLite file on Windows.
	defer func() { _ = CloseDatabase() }()

	origDir := t.TempDir()
	cfg, origPath := newSwitchTestConfig(t, origDir, "orig.db")
	ensureRuntimeOrFail(t, cfg)

	const markerKey = "test.switch.success.marker"
	origStore := NewSettingsStore(GetDB())
	if err := origStore.Set(markerKey, "original"); err != nil {
		t.Fatalf("seed original setting: %v", err)
	}

	newDir := t.TempDir()
	newPath := filepath.Join(newDir, "new.db")
	if err := SwitchDatabase(cfg, DialectSQLite, newPath, false); err != nil {
		t.Fatalf("SwitchDatabase returned error: %v", err)
	}

	newDB := GetDB()
	if newDB == nil {
		t.Fatal("GetDB() == nil after successful switch")
	}
	if config.Runtime().DbType != DialectSQLite {
		t.Fatalf("runtime DbType = %q, want %q", config.Runtime().DbType, DialectSQLite)
	}
	if config.Runtime().DbUrl != newPath {
		t.Fatalf("runtime DbUrl = %q, want %q", config.Runtime().DbUrl, newPath)
	}

	// New DB is a fresh migrate — the old marker must NOT be present.
	newStore := NewSettingsStore(newDB)
	got, err := newStore.Get(markerKey)
	if err != nil {
		t.Fatalf("read marker on new DB: %v", err)
	}
	if got != "" {
		t.Fatalf("marker leaked into new DB: %q (expected empty — different file)", got)
	}

	// Queries work on the new DB: write + read a fresh setting.
	if err := newStore.Set("test.switch.success.new", "written"); err != nil {
		t.Fatalf("write on new DB: %v", err)
	}
	got, err = newStore.Get("test.switch.success.new")
	if err != nil || got != "written" {
		t.Fatalf("read back on new DB: got=%q err=%v", got, err)
	}

	// Sanity: the original file still exists on disk (switch closes, never deletes).
	if _, statErr := os.Stat(origPath); statErr != nil {
		t.Fatalf("original db file missing after switch: %v", statErr)
	}
}

// TestSwitchDatabase_OpenFailure_RollsBack exercises the rollback path: a bad
// dialect cannot be opened, so SwitchDatabase must restore the original config
// and re-open the original DB so the caller keeps serving traffic.
func TestSwitchDatabase_OpenFailure_RollsBack(t *testing.T) {
	resetActiveDBForTest(t)
	defer func() { _ = CloseDatabase() }()

	origDir := t.TempDir()
	cfg, origPath := newSwitchTestConfig(t, origDir, "orig.db")
	ensureRuntimeOrFail(t, cfg)

	const markerKey = "test.switch.rollback.marker"
	if err := NewSettingsStore(GetDB()).Set(markerKey, "original"); err != nil {
		t.Fatalf("seed original setting: %v", err)
	}

	// An unsupported dialect fails at OpenWithPostgresSSLModeAndPool, which
	// routes through rollbackSwitch (same path as a migration failure).
	newDir := t.TempDir()
	badPath := filepath.Join(newDir, "bad.db")
	switchErr := SwitchDatabase(cfg, "baddialect", badPath, false)
	if switchErr == nil {
		t.Fatal("SwitchDatabase with bad dialect returned nil error, want rollback failure")
	}

	// Config must be restored to the original values.
	if config.Runtime().DbType != DialectSQLite {
		t.Fatalf("runtime DbType = %q, want restored %q", config.Runtime().DbType, DialectSQLite)
	}
	if config.Runtime().DbUrl != origPath {
		t.Fatalf("runtime DbUrl = %q, want restored %q", config.Runtime().DbUrl, origPath)
	}

	// Rollback must have re-opened the original DB.
	restoredDB := GetDB()
	if restoredDB == nil {
		t.Fatal("GetDB() == nil after rollback; expected original re-opened")
	}
	got, err := NewSettingsStore(restoredDB).Get(markerKey)
	if err != nil {
		t.Fatalf("read marker after rollback: %v", err)
	}
	if got != "original" {
		t.Fatalf("marker after rollback = %q, want %q (original DB must be queryable)", got, "original")
	}
}

// TestSwitchDatabase_RollbackAlsoFails verifies the worst case: both the new
// DB and the rollback DB fail to open. activeDB must be left nil (only logged)
// and the error must be wrapped so operators see both failures.
func TestSwitchDatabase_RollbackAlsoFails(t *testing.T) {
	resetActiveDBForTest(t)
	defer func() { _ = CloseDatabase() }()

	origDir := t.TempDir()
	cfg, origPath := newSwitchTestConfig(t, origDir, "orig.db")
	ensureRuntimeOrFail(t, cfg)

	// Close + remove the parent dir so BOTH the original path and a new path
	// inside the same dir cannot be opened. We close first to avoid Windows
	// file-lock on the open SQLite handle.
	if err := CloseDatabase(); err != nil {
		t.Fatalf("close original before removing dir: %v", err)
	}
	if err := os.RemoveAll(origDir); err != nil {
		t.Fatalf("remove original dir: %v", err)
	}

	// New path shares the (now-deleted) parent dir → open fails. Rollback
	// restores origPath, whose parent is also gone → rollback open also fails.
	newPath := filepath.Join(origPath, "new.db") // parent missing
	switchErr := SwitchDatabase(cfg, DialectSQLite, newPath, false)
	if switchErr == nil {
		t.Fatal("SwitchDatabase returned nil error; want wrapped switch+rollback failure")
	}
	if !strings.Contains(switchErr.Error(), "switch") || !strings.Contains(switchErr.Error(), "rollback") {
		t.Fatalf("error message = %q, want both switch and rollback context", switchErr.Error())
	}

	// activeDB must be nil — the singleton must not hold a stale/broken handle.
	if GetDB() != nil {
		t.Fatalf("GetDB() != nil after rollback failure; expected nil activeDB")
	}
	if initialized {
		t.Fatal("initialized flag still true after rollback failure; expected false")
	}

	// Config is best-effort restored to the original values by rollbackSwitch
	// even when re-open fails, so a future EnsureRuntimeDatabase retries the
	// original (operator-recoverable) rather than the broken new path.
	if config.Runtime().DbUrl != origPath {
		t.Fatalf("runtime DbUrl = %q, want restored %q", config.Runtime().DbUrl, origPath)
	}
}
