//go:build integration

package admin

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/internal/pgtest"
	"github.com/deliciousbuding/metapi-go/store"
)

// TestBackupImportResyncsPostgresSequences is the PostgreSQL half of a restore.
//
// An export carries rows with the ids they were written with, so importing them
// leaves every serial sequence pointing below the restored data. The next
// ordinary INSERT then asks the sequence for an id the restore just occupied and
// fails on a unique constraint -- an error that names a key, not a sequence, so
// nothing in it points back at the import. A restored deployment that cannot
// perform its first write is not a working restore, and it looks fine until
// somebody tries to add a site.
//
// SQLite is deliberately not covered: AUTOINCREMENT updates sqlite_sequence on an
// explicit insert and a plain INTEGER PRIMARY KEY derives max+1 by itself, so
// there is nothing to resync (store.ResyncPGIDSequences documents both).
func TestBackupImportResyncsPostgresSequences(t *testing.T) {
	dsn := os.Getenv("PG_TEST_DSN")
	if strings.TrimSpace(dsn) == "" {
		t.Skip("PG_TEST_DSN not set; skipping PostgreSQL integration test")
	}
	db, err := store.Open(store.DialectPostgres, dsn, false)
	if err != nil {
		t.Fatalf("failed to open PostgreSQL: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	pgtest.Reset(t, db.DB)
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	site := func(id int, name, url, platform string) string {
		return `{"id":` + json.Number(strings.TrimSpace(itoaPG(id))).String() +
			`,"name":"` + name + `","url":"` + url + `","platform":"` + platform +
			`","status":"active","created_at":"` + now + `","updated_at":"` + now + `"}`
	}
	// Exactly what a real export looks like: explicit, low, contiguous ids.
	payload := map[string]json.RawMessage{
		"sites": json.RawMessage("[" +
			site(1, "restored-a", "http://127.0.0.1:3000", "new-api") + "," +
			site(2, "restored-b", "http://127.0.0.1:3004", "sub2api") + "," +
			site(3, "restored-c", "http://127.0.0.1:3012", "openai") +
			"]"),
	}
	tx, err := db.Beginx()
	if err != nil {
		t.Fatalf("begin import tx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := importBackupTablesWithConn(tx, payload); err != nil {
		t.Fatalf("import: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit import tx: %v", err)
	}

	// The ordinary write path supplies no id, so PostgreSQL asks the sequence.
	if _, err := db.Exec(db.Rebind(`INSERT INTO sites (name, url, platform, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`),
		"post-restore", "http://127.0.0.1:3001", "new-api", "active", now, now); err != nil {
		t.Fatalf("first write after restore failed: %v (a duplicate key here means the import left the id sequences behind the rows it restored)", err)
	}

	var maxID, seq int64
	if err := db.QueryRow(`SELECT MAX(id) FROM sites`).Scan(&maxID); err != nil {
		t.Fatalf("read MAX(id): %v", err)
	}
	// pg_sequence_last_value takes the sequence's regclass; `FROM func()::regclass`
	// is not valid PostgreSQL, which is worth writing down because it reads fine.
	if err := db.QueryRow(`SELECT pg_sequence_last_value(pg_get_serial_sequence('sites', 'id')::regclass)`).Scan(&seq); err != nil {
		t.Fatalf("read the sites id sequence: %v", err)
	}
	if maxID != 4 {
		t.Fatalf("MAX(id) = %d, want 4 (three restored rows plus the post-restore write)", maxID)
	}
	if seq < maxID {
		t.Fatalf("sequence last_value = %d is below MAX(id) = %d, so a later insert can still collide", seq, maxID)
	}
}

// TestBackupImportCoercesSQLiteIntBooleansForPostgres proves that an export from
// SQLite (which emits 0/1 numbers for BOOLEAN columns such as sites.use_system_proxy
// and accounts.checkin_enabled) restores cleanly into PostgreSQL.
func TestBackupImportCoercesSQLiteIntBooleansForPostgres(t *testing.T) {
	dsn := os.Getenv("PG_TEST_DSN")
	if strings.TrimSpace(dsn) == "" {
		t.Skip("PG_TEST_DSN not set; skipping PostgreSQL integration test")
	}
	db, err := store.Open(store.DialectPostgres, dsn, false)
	if err != nil {
		t.Fatalf("failed to open PostgreSQL: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	pgtest.Reset(t, db.DB)
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	payload := map[string]json.RawMessage{
		"sites":    json.RawMessage(`[{"id":1,"name":"sqlite-site","url":"http://127.0.0.1:3000","platform":"codex","use_system_proxy":0,"custom_headers_override_request_headers":1,"is_pinned":0,"status":"active","created_at":"` + now + `","updated_at":"` + now + `"}]`),
		"accounts": json.RawMessage(`[{"id":1,"site_id":1,"username":"oauth-user","access_token":"tok","checkin_enabled":0,"is_pinned":1,"status":"active","oauth_provider":"codex","created_at":"` + now + `","updated_at":"` + now + `"}]`),
	}
	tx, err := db.Beginx()
	if err != nil {
		t.Fatalf("begin import tx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := importBackupTablesWithConn(tx, payload); err != nil {
		t.Fatalf("import SQLite-encoded booleans into PostgreSQL failed: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit import tx: %v", err)
	}

	var useProxy, overrideHeaders bool
	if err := db.QueryRow(`SELECT use_system_proxy, custom_headers_override_request_headers FROM sites WHERE id = 1`).Scan(&useProxy, &overrideHeaders); err != nil {
		t.Fatalf("query restored site: %v", err)
	}
	if useProxy || !overrideHeaders {
		t.Fatalf("restored site booleans = (%v, %v), want (false, true)", useProxy, overrideHeaders)
	}
}

func itoaPG(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
