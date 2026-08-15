package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
)

// openBalanceTestDB opens a fresh in-memory SQLite DB with AutoMigrate.
func openBalanceTestDB(t *testing.T) *store.DB {
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

// TestBalanceScheduler_runJobLocked_EmptyDB verifies that runJobLocked
// completes without error on a fresh DB with no active accounts.
// RefreshAllBalances queries for active accounts; an empty result set means
// no HTTP calls are made, so the method returns a nil/empty slice and logs
// the completion line.
func TestBalanceScheduler_runJobLocked_EmptyDB(t *testing.T) {
	ResetLeasePressureForTest()
	db := openBalanceTestDB(t)
	cfg := testConfig()
	s := NewBalanceScheduler(cfg)

	// Direct call to runJobLocked — no lease acquisition, no cron wrapper.
	// On an empty DB, RefreshAllBalances returns nil/empty and the method
	// completes without panic.
	s.runJobLocked(db)
}

// TestBalanceScheduler_runJob_WithOverrideDB exercises the full runJob flow:
// store.GetDB lookup, context creation, lease acquisition, and runJobLocked.
// The DB is overridden via store.OverrideDB so GetDB returns the test DB.
// On an empty DB, no HTTP calls are made and the lease is released cleanly.
func TestBalanceScheduler_runJob_WithOverrideDB(t *testing.T) {
	ResetLeasePressureForTest()
	db := openBalanceTestDB(t)
	store.OverrideDB(db)
	t.Cleanup(func() { store.OverrideDB(nil) })

	cfg := testConfig()
	s := NewBalanceScheduler(cfg)

	// runJob calls store.GetDB() → overridden DB, acquires a local lease
	// (SQLite uses process-local exclusion), then calls runJobLocked.
	s.runJob()
}

// TestBalanceScheduler_runJob_NilDB verifies that runJob handles a nil DB
// gracefully (logs "database not available" and returns without panic).
func TestBalanceScheduler_runJob_NilDB(t *testing.T) {
	ResetLeasePressureForTest()
	store.OverrideDB(nil)
	t.Cleanup(func() { store.OverrideDB(nil) })

	cfg := testConfig()
	s := NewBalanceScheduler(cfg)

	// Should not panic when GetDB returns nil.
	s.runJob()
}

// TestBalanceScheduler_StartStop_Lifecycle verifies the cron-job wrapper:
// Start registers a cron job and begins the runner; Stop halts it.
// The cron expression is short enough that no tick fires during the test.
func TestBalanceScheduler_StartStop_Lifecycle(t *testing.T) {
	cfg := testConfig()
	cfg.BalanceRefreshCron = "0 0 0 * * *" // midnight daily — won't fire during test
	s := NewBalanceScheduler(cfg)

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	if err := s.Stop(); err != nil {
		t.Errorf("Stop failed: %v", err)
	}
	// Idempotent Stop.
	if err := s.Stop(); err != nil {
		t.Errorf("second Stop failed: %v", err)
	}
}

// TestBalanceScheduler_UpdateCron_InvalidAndValid exercises the UpdateCron
// path. An invalid expression must return an error; a valid expression must
// update cfg.BalanceRefreshCron and restart the cron runner without error.
func TestBalanceScheduler_UpdateCron_InvalidAndValid(t *testing.T) {
	cfg := &config.Config{BalanceRefreshCron: "0 0 0 * * *"}
	s := NewBalanceScheduler(cfg)

	if err := s.UpdateCron("not-a-cron"); err == nil {
		t.Error("expected error for invalid cron expression")
	}

	valid := "30 0 */2 * * *"
	if err := s.UpdateCron(valid); err != nil {
		t.Errorf("UpdateCron with valid expression failed: %v", err)
	}
	if cfg.BalanceRefreshCron != valid {
		t.Errorf("cfg.BalanceRefreshCron = %q, want %q", cfg.BalanceRefreshCron, valid)
	}

	// Clean up the started cron runner.
	if err := s.Stop(); err != nil {
		t.Errorf("Stop after UpdateCron failed: %v", err)
	}
}
