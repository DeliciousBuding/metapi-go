package store

import (
	"bytes"
	"io"
	"path/filepath"
	"testing"
)

// TestHashRowsOrderIndependent guards against the --verify false mismatch
// class found while validating the TS→Go migration: hashRows must be
// deterministic regardless of the source query's row order.
func TestHashRowsOrderIndependent(t *testing.T) {
	a := []map[string]interface{}{
		{"id": int64(1), "name": "first", "enabled": int64(1)},
		{"id": int64(2), "name": "second", "enabled": int64(0)},
		{"id": int64(3), "name": "third", "enabled": int64(1)},
	}
	reversed := []map[string]interface{}{a[2], a[0], a[1]}
	shuffled := []map[string]interface{}{a[1], a[2], a[0]}

	h1 := hashRows(a)
	if !bytes.Equal(h1, hashRows(reversed)) {
		t.Fatal("hashRows differs under reversed row order")
	}
	if !bytes.Equal(h1, hashRows(shuffled)) {
		t.Fatal("hashRows differs under shuffled row order")
	}
}

// TestHashRowsNormalizesBooleans guards the cross-dialect boolean rule:
// SQLite stores 0/1, PostgreSQL scans true/false — the hash must agree.
func TestHashRowsNormalizesBooleans(t *testing.T) {
	sqliteLike := []map[string]interface{}{
		{"id": int64(1), "active": int64(1), "note": "x"},
	}
	pgLike := []map[string]interface{}{
		{"id": int64(1), "active": true, "note": "x"},
	}
	if !bytes.Equal(hashRows(sqliteLike), hashRows(pgLike)) {
		t.Fatal("hashRows should normalize booleans across dialects")
	}
}

// TestRunMigrationVerifyTSSchemaOrder reproduces the real-world regression:
// a source whose columns are laid out in the TypeScript drizzle order (with
// the additive Go columns appended) migrated to a Go-created target must pass
// --verify. This failed on three distinct bugs: column-order hashing,
// settings runtime-key filtering, and row-order hashing.
func TestRunMigrationVerifyTSSchemaOrder(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "source.db")
	targetPath := filepath.Join(t.TempDir(), "target.db")

	// Source mirrors a TS-version database: sites columns in the drizzle
	// order with the Go additive columns appended at the end.
	srcDB, err := Open(DialectSQLite, sourcePath, false)
	if err != nil {
		t.Fatalf("open source: %v", err)
	}
	if _, err := srcDB.Exec(`CREATE TABLE sites (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		url TEXT NOT NULL,
		platform TEXT NOT NULL,
		status TEXT NOT NULL,
		api_key TEXT,
		created_at TEXT DEFAULT (datetime('now')),
		updated_at TEXT DEFAULT (datetime('now')),
		global_weight REAL,
		max_concurrency INTEGER,
		tags TEXT
	)`); err != nil {
		t.Fatalf("create ts-order sites: %v", err)
	}
	if _, err := srcDB.Exec(`CREATE TABLE settings (
		key TEXT PRIMARY KEY,
		value TEXT
	)`); err != nil {
		t.Fatalf("create settings: %v", err)
	}
	if _, err := srcDB.Exec(`INSERT INTO sites (name, url, platform, status, api_key, global_weight, max_concurrency, tags) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"新API站A", "https://a.example.com", "new-api", "active", "sk-a", 1.0, 0, `["prod"]`); err != nil {
		t.Fatalf("seed sites: %v", err)
	}
	if _, err := srcDB.Exec(`INSERT INTO sites (name, url, platform, status, api_key, global_weight, max_concurrency) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"站B", "https://b.example.com", "one-api", "active", "sk-b", 1.0, 0); err != nil {
		t.Fatalf("seed sites 2: %v", err)
	}
	// Runtime DB settings keys are documented as filtered out of migrations;
	// verify must not flag the resulting count difference.
	for _, kv := range [][2]string{{"db_type", "sqlite"}, {"db_url", "data/hub.db"}, {"site_name", "我的中转"}} {
		if _, err := srcDB.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)`, kv[0], kv[1]); err != nil {
			t.Fatalf("seed settings: %v", err)
		}
	}
	if err := srcDB.Close(); err != nil {
		t.Fatalf("close source: %v", err)
	}

	summary, err := RunMigration(RunMigrationOptions{
		FromPath:  sourcePath,
		ToURL:     targetPath,
		Overwrite: true,
		Verify:    true,
		LogWriter: io.Discard,
	})
	if err != nil {
		t.Fatalf("RunMigration with --verify failed on a TS-order source: %v", err)
	}
	if summary == nil || summary.Rows["sites"] != 2 {
		t.Fatalf("summary sites rows = %+v, want 2", summary)
	}
	// summary.Rows reports the SOURCE snapshot counts; the runtime-key filter
	// shows up on the target side, which is what --verify actually checks.
	if got := summary.Rows["settings"]; got != 3 {
		t.Fatalf("summary settings rows = %d, want 3 (source count)", got)
	}
	tgtDB, err := Open(DialectSQLite, targetPath, false)
	if err != nil {
		t.Fatalf("open target for inspection: %v", err)
	}
	defer tgtDB.Close()
	var targetSettings int
	if err := tgtDB.QueryRow(`SELECT COUNT(*) FROM settings`).Scan(&targetSettings); err != nil {
		t.Fatalf("count target settings: %v", err)
	}
	if targetSettings != 1 {
		t.Fatalf("target settings rows = %d, want 1 (runtime keys filtered)", targetSettings)
	}
}
