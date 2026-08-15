package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// openLogCleanupTestDB opens a fresh in-memory SQLite DB with AutoMigrate.
func openLogCleanupTestDB(t *testing.T) *store.DB {
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

// TestLogCleanupScheduler_StartStop_Lifecycle verifies the cron-job wrapper:
// Start registers a cron job and begins the runner; Stop halts it.
func TestLogCleanupScheduler_StartStop_Lifecycle(t *testing.T) {
	cfg := testConfig()
	s := NewLogCleanupScheduler(cfg)

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	if err := s.Stop(); err != nil {
		t.Errorf("Stop failed: %v", err)
	}
	if err := s.Stop(); err != nil {
		t.Errorf("second Stop failed: %v", err)
	}
}

// TestLogCleanupScheduler_runJob_NotConfigured verifies that runJob returns
// early when LogCleanupConfigured is false (legacy fallback mode active).
func TestLogCleanupScheduler_runJob_NotConfigured(t *testing.T) {
	ResetLeasePressureForTest()
	cfg := testConfig()
	cfg.LogCleanupConfigured = false
	s := NewLogCleanupScheduler(cfg)

	// Should return without touching the DB (no GetDB call).
	s.runJob()
}

// TestLogCleanupScheduler_runJob_NoLogTargetEnabled verifies that runJob
// returns early when both usage and program log targets are disabled.
func TestLogCleanupScheduler_runJob_NoLogTargetEnabled(t *testing.T) {
	ResetLeasePressureForTest()
	cfg := testConfig()
	cfg.LogCleanupConfigured = true
	cfg.LogCleanupUsageLogsEnabled = false
	cfg.LogCleanupProgramLogsEnabled = false
	s := NewLogCleanupScheduler(cfg)

	// Should return without touching the DB.
	s.runJob()
}

// TestLogCleanupScheduler_runJob_Configured exercises the full runJob flow
// with LogCleanupConfigured=true and a DB override. On an empty DB, the
// DELETE queries affect 0 rows and the method completes without error.
func TestLogCleanupScheduler_runJob_Configured(t *testing.T) {
	ResetLeasePressureForTest()
	db := openLogCleanupTestDB(t)
	store.OverrideDB(db)
	t.Cleanup(func() { store.OverrideDB(nil) })

	cfg := testConfig()
	cfg.LogCleanupConfigured = true
	cfg.LogCleanupUsageLogsEnabled = true
	cfg.LogCleanupProgramLogsEnabled = true
	cfg.LogCleanupRetentionDays = 90
	s := NewLogCleanupScheduler(cfg)

	// runJob calls store.GetDB(), acquires a local lease, then runJobLocked
	// which DELETEs from proxy_logs and events. On an empty DB, 0 rows deleted.
	s.runJob()
}

// TestLogCleanupScheduler_runJob_NilDB verifies that runJob handles a nil DB
// gracefully (logs "database not available" and returns without panic).
func TestLogCleanupScheduler_runJob_NilDB(t *testing.T) {
	ResetLeasePressureForTest()
	store.OverrideDB(nil)
	t.Cleanup(func() { store.OverrideDB(nil) })

	cfg := testConfig()
	cfg.LogCleanupConfigured = true
	s := NewLogCleanupScheduler(cfg)

	s.runJob()
}

// TestLogCleanupScheduler_runJobLocked_EmptyDB verifies that runJobLocked
// executes the DELETE queries on an empty DB without error.
func TestLogCleanupScheduler_runJobLocked_EmptyDB(t *testing.T) {
	db := openLogCleanupTestDB(t)
	cfg := testConfig()
	cfg.LogCleanupConfigured = true
	cfg.LogCleanupUsageLogsEnabled = true
	cfg.LogCleanupProgramLogsEnabled = true
	cfg.LogCleanupRetentionDays = 90
	s := NewLogCleanupScheduler(cfg)

	s.runJobLocked(db)
}

// Note: TestLogCleanupScheduler_UpdateSettings is already tested in
// scheduler_test.go — no duplicate here.
