package scheduler

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

// openSettingsTestDB opens a fresh in-memory SQLite DB with AutoMigrate.
func openSettingsTestDB(t *testing.T) *store.DB {
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

// insertSetting inserts a JSON-encoded setting value for the given key.
func insertSetting(t *testing.T, db *store.DB, key, jsonValue string) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO settings (key, value) VALUES (?, ?)`, key, jsonValue,
	); err != nil {
		t.Fatalf("insert setting %s: %v", key, err)
	}
}

// TestResolveCronSetting_StoredValue verifies that resolveCronSetting reads
// a valid cron expression from the DB settings table and returns it instead
// of the fallback. This covers the Get → Unmarshal → Validate → return path.
func TestResolveCronSetting_StoredValue(t *testing.T) {
	db := openSettingsTestDB(t)
	store.OverrideDB(db)
	t.Cleanup(func() { store.OverrideDB(nil) })

	insertSetting(t, db, "balance_refresh_cron", `"0 0 6 * * *"`)

	got := resolveCronSetting("balance_refresh_cron", "0 0 0 * * *")
	if got != "0 0 6 * * *" {
		t.Errorf("resolveCronSetting = %q, want %q", got, "0 0 6 * * *")
	}
}

// TestResolveCronSetting_FallbackOnNilDB verifies that resolveCronSetting
// returns the fallback when the DB is nil.
func TestResolveCronSetting_FallbackOnNilDB(t *testing.T) {
	store.OverrideDB(nil)
	t.Cleanup(func() { store.OverrideDB(nil) })

	got := resolveCronSetting("any_key", "0 0 0 * * *")
	if got != "0 0 0 * * *" {
		t.Errorf("resolveCronSetting with nil DB = %q, want fallback", got)
	}
}

// TestResolveCronSetting_FallbackOnMissingKey verifies that resolveCronSetting
// returns the fallback when the key is not in the settings table.
func TestResolveCronSetting_FallbackOnMissingKey(t *testing.T) {
	db := openSettingsTestDB(t)
	store.OverrideDB(db)
	t.Cleanup(func() { store.OverrideDB(nil) })

	got := resolveCronSetting("nonexistent_key", "0 0 0 * * *")
	if got != "0 0 0 * * *" {
		t.Errorf("resolveCronSetting for missing key = %q, want fallback", got)
	}
}

// TestResolveCronSetting_FallbackOnInvalidCron verifies that resolveCronSetting
// returns the fallback when the stored value is not a valid cron expression.
func TestResolveCronSetting_FallbackOnInvalidCron(t *testing.T) {
	db := openSettingsTestDB(t)
	store.OverrideDB(db)
	t.Cleanup(func() { store.OverrideDB(nil) })

	insertSetting(t, db, "bad_cron", `"not-a-cron"`)

	got := resolveCronSetting("bad_cron", "0 0 0 * * *")
	if got != "0 0 0 * * *" {
		t.Errorf("resolveCronSetting with invalid cron = %q, want fallback", got)
	}
}

// TestResolveBooleanSetting_StoredValue verifies that resolveBooleanSetting
// reads a boolean from the DB settings table and returns it.
func TestResolveBooleanSetting_StoredValue(t *testing.T) {
	db := openSettingsTestDB(t)
	store.OverrideDB(db)
	t.Cleanup(func() { store.OverrideDB(nil) })

	insertSetting(t, db, "my_bool", "true")

	got := resolveBooleanSetting("my_bool", false)
	if !got {
		t.Errorf("resolveBooleanSetting = false, want true")
	}
}

// TestResolveBooleanSetting_FallbackOnNilDB verifies the nil DB fallback.
func TestResolveBooleanSetting_FallbackOnNilDB(t *testing.T) {
	store.OverrideDB(nil)
	t.Cleanup(func() { store.OverrideDB(nil) })

	got := resolveBooleanSetting("any_key", true)
	if !got {
		t.Error("resolveBooleanSetting with nil DB should return fallback true")
	}
}

// TestResolveStringSetting_StoredValue verifies that resolveStringSetting
// reads a string from the DB and returns it.
func TestResolveStringSetting_StoredValue(t *testing.T) {
	db := openSettingsTestDB(t)
	store.OverrideDB(db)
	t.Cleanup(func() { store.OverrideDB(nil) })

	insertSetting(t, db, "my_string", `"hello"`)

	got := resolveStringSetting("my_string", "fallback")
	if got != "hello" {
		t.Errorf("resolveStringSetting = %q, want hello", got)
	}
}

// TestResolvePositiveIntegerSetting_StoredValue verifies that
// resolvePositiveIntegerSetting reads a positive integer from the DB.
func TestResolvePositiveIntegerSetting_StoredValue(t *testing.T) {
	db := openSettingsTestDB(t)
	store.OverrideDB(db)
	t.Cleanup(func() { store.OverrideDB(nil) })

	insertSetting(t, db, "my_int", "42")

	got := resolvePositiveIntegerSetting("my_int", 7)
	if got != 42 {
		t.Errorf("resolvePositiveIntegerSetting = %d, want 42", got)
	}
}

// TestResolvePositiveIntegerSetting_FallbackOnZero verifies that a stored
// value of 0 falls back to the default (value < 1 guard).
func TestResolvePositiveIntegerSetting_FallbackOnZero(t *testing.T) {
	db := openSettingsTestDB(t)
	store.OverrideDB(db)
	t.Cleanup(func() { store.OverrideDB(nil) })

	insertSetting(t, db, "zero_int", "0")

	got := resolvePositiveIntegerSetting("zero_int", 7)
	if got != 7 {
		t.Errorf("resolvePositiveIntegerSetting with stored 0 = %d, want fallback 7", got)
	}
}
