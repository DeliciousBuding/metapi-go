package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
)

// openModelSyncTestDB opens a fresh in-memory SQLite DB with AutoMigrate.
func openModelSyncTestDB(t *testing.T) *store.DB {
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

// TestModelSyncScheduler_runJobLocked_EmptyDB verifies that runJobLocked
// completes without error on a fresh DB with no candidate accounts.
func TestModelSyncScheduler_runJobLocked_EmptyDB(t *testing.T) {
	ResetLeasePressureForTest()
	db := openModelSyncTestDB(t)
	testConfig() // publishes the runtime baseline
	s := NewModelSyncScheduler()

	// Direct call to runJobLocked — no lease acquisition, no cron wrapper.
	s.runJobLocked(context.Background(), db)
}

// TestModelSyncScheduler_runJob_WithOverrideDB exercises the full runJob flow:
// store.GetDB lookup, context creation, lease acquisition, and runJobLocked.
func TestModelSyncScheduler_runJob_WithOverrideDB(t *testing.T) {
	ResetLeasePressureForTest()
	db := openModelSyncTestDB(t)
	store.OverrideDB(db)
	t.Cleanup(func() { store.OverrideDB(nil) })

	testConfig() // publishes the runtime baseline
	s := NewModelSyncScheduler()

	// runJob calls store.GetDB() -> overridden DB, acquires a local lease
	// (SQLite uses process-local exclusion), then calls runJobLocked.
	s.runJob()
}

// TestModelSyncScheduler_runJob_NilDB verifies that runJob handles a nil DB
// gracefully (logs "database not available" and returns without panic).
func TestModelSyncScheduler_runJob_NilDB(t *testing.T) {
	ResetLeasePressureForTest()
	store.OverrideDB(nil)
	t.Cleanup(func() { store.OverrideDB(nil) })

	testConfig() // publishes the runtime baseline
	s := NewModelSyncScheduler()

	// Should not panic when GetDB returns nil.
	s.runJob()
}

// TestModelSyncScheduler_StartStop_Lifecycle verifies the cron-job wrapper:
// Start registers a cron job and begins the runner; Stop halts it. The cron
// expression never fires during the test.
func TestModelSyncScheduler_StartStop_Lifecycle(t *testing.T) {
	testConfig() // publishes the runtime baseline
	config.UpdateRuntime(func(r *config.RuntimeSettings) { r.ModelSyncCron = "0 0 4 * * *" }) // 04:00 daily — won't fire during test
	s := NewModelSyncScheduler()

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

// TestModelSyncScheduler_UpdateCron_InvalidAndValid exercises the UpdateCron
// path. An invalid expression must return an error; a valid expression must
// update the runtime ModelSyncCron snapshot and restart the cron runner without error.
func TestModelSyncScheduler_UpdateCron_InvalidAndValid(t *testing.T) {
	config.SetRuntime(&config.RuntimeSettings{ModelSyncCron: "0 0 4 * * *"})
	t.Cleanup(func() { config.SetRuntime(nil) })
	s := NewModelSyncScheduler()

	if err := s.UpdateCron("not-a-cron"); err == nil {
		t.Error("expected error for invalid cron expression")
	}

	valid := "30 4 */2 * * *"
	if err := s.UpdateCron(valid); err != nil {
		t.Errorf("UpdateCron with valid expression failed: %v", err)
	}
	if config.Runtime().ModelSyncCron != valid {
		t.Errorf("config.Runtime().ModelSyncCron = %q, want %q", config.Runtime().ModelSyncCron, valid)
	}

	// Clean up the started cron runner.
	if err := s.Stop(); err != nil {
		t.Errorf("Stop after UpdateCron failed: %v", err)
	}
}

// TestModelSyncScheduler_StartResolvesDBSetting verifies the DB settings key
// model_sync_cron overrides the config/env default at Start, matching the
// balance_refresh_cron resolveCronSetting pattern.
func TestModelSyncScheduler_StartResolvesDBSetting(t *testing.T) {
	db := openModelSyncTestDB(t)
	store.OverrideDB(db)
	t.Cleanup(func() { store.OverrideDB(nil) })

	// Persist a valid override + an invalid value scenario.
	if _, err := db.Exec(`INSERT INTO settings (key, value) VALUES ('model_sync_cron', '"15 3 * * 1"')`); err != nil {
		t.Fatalf("insert setting: %v", err)
	}

	config.SetRuntime(&config.RuntimeSettings{ModelSyncCron: config.DefaultModelSyncCron})
	t.Cleanup(func() { config.SetRuntime(nil) })
	s := NewModelSyncScheduler()
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	t.Cleanup(func() { _ = s.Stop() })

	if config.Runtime().ModelSyncCron != "15 3 * * 1" {
		t.Fatalf("config.Runtime().ModelSyncCron = %q, want DB setting override 15 3 * * 1", config.Runtime().ModelSyncCron)
	}
}

// TestModelSyncScheduler_StartFallsBackOnInvalidDBSetting verifies an invalid
// persisted value falls back to the config default instead of failing Start.
func TestModelSyncScheduler_StartFallsBackOnInvalidDBSetting(t *testing.T) {
	db := openModelSyncTestDB(t)
	store.OverrideDB(db)
	t.Cleanup(func() { store.OverrideDB(nil) })

	if _, err := db.Exec(`INSERT INTO settings (key, value) VALUES ('model_sync_cron', '"not a cron"')`); err != nil {
		t.Fatalf("insert setting: %v", err)
	}

	config.SetRuntime(&config.RuntimeSettings{ModelSyncCron: config.DefaultModelSyncCron})
	t.Cleanup(func() { config.SetRuntime(nil) })
	s := NewModelSyncScheduler()
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	t.Cleanup(func() { _ = s.Stop() })

	if config.Runtime().ModelSyncCron != config.DefaultModelSyncCron {
		t.Fatalf("config.Runtime().ModelSyncCron = %q, want fallback default %q", config.Runtime().ModelSyncCron, config.DefaultModelSyncCron)
	}
}
