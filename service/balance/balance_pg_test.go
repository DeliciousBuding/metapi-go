package balance

import (
	"os"
	"testing"

	"github.com/deliciousbuding/metapi-go/internal/pgtest"
	"github.com/deliciousbuding/metapi-go/store"
)

// pgDSN returns the PostgreSQL DSN from PG_TEST_DSN (empty when unset).
func pgDSN() string {
	return os.Getenv("PG_TEST_DSN")
}

// TestRecordBalanceSnapshotPostgres
//
// Regression for the 2026-08-02 v0.8.47 deployment: the balance_history
// snapshot UPSERT ran with raw SQLite `?` placeholders, which PostgreSQL
// rejects with SQLSTATE 42601 — the A1 feature was only ever tested on
// SQLite. The query must be dialect-rebound so it executes on PG.
func TestRecordBalanceSnapshotPostgres(t *testing.T) {
	if pgDSN() == "" {
		t.Skip("PG_TEST_DSN not set; skipping PostgreSQL integration test")
	}

	db, err := store.Open(store.DialectPostgres, pgDSN(), false)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Empty the database before migrating so this test starts from the same
	// state CI gives it: a local loop reuses one PG database, and the
	// previous run's rows turn fixed-identity fixtures into duplicate-key
	// failures and whole-table counts into "want 3, got 4".
	pgtest.Reset(t, db.DB)
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("auto-migrate: %v", err)
	}

	// Seed a site + minimal account so the snapshot has an owner.
	if _, err := db.Exec(db.Rebind(
		`INSERT INTO sites (name, url, platform, status, created_at, updated_at)
		 VALUES ('pg-snapshot-probe', 'https://probe.example.test', 'openai', 'active', NOW(), NOW())`)); err != nil {
		t.Fatalf("seed site: %v", err)
	}
	var siteID int64
	if err := db.Get(&siteID, "SELECT id FROM sites WHERE name = 'pg-snapshot-probe'"); err != nil {
		t.Fatalf("load site id: %v", err)
	}
	if _, err := db.Exec(db.Rebind(
		`INSERT INTO accounts (site_id, username, access_token, status, created_at, updated_at)
		 VALUES ($1, 'pg-snapshot-probe', 'sk-probe', 'active', NOW(), NOW())`), siteID); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	var accountID int64
	if err := db.Get(&accountID, "SELECT id FROM accounts WHERE username = 'pg-snapshot-probe'"); err != nil {
		t.Fatalf("load account id: %v", err)
	}

	// First insert, then a conflicting second insert (UPSERT path).
	recordBalanceSnapshot(db.DB, accountID, 10.5, 1.25, 0)
	recordBalanceSnapshot(db.DB, accountID, 9.75, 2.5, 0)

	var rows int
	if err := db.Get(&rows,
		"SELECT COUNT(*) FROM balance_history WHERE account_id = $1", accountID); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if rows != 1 {
		t.Fatalf("balance_history rows = %d, want 1 (UPSERT)", rows)
	}

	var balance float64
	if err := db.Get(&balance,
		"SELECT balance FROM balance_history WHERE account_id = $1", accountID); err != nil {
		t.Fatalf("read balance: %v", err)
	}
	if balance != 9.75 {
		t.Fatalf("balance = %v, want 9.75 (second snapshot wins)", balance)
	}
}
