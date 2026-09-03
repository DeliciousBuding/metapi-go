package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
)

// openRetentionSchedTestDB opens a fresh in-memory SQLite DB with AutoMigrate.
func openRetentionSchedTestDB(t *testing.T) *store.DB {
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

// TestProxyLogRetention_StartStop_Lifecycle verifies the interval-runner
// wrapper: Start begins the ticker; Stop halts it.
func TestProxyLogRetention_StartStop_Lifecycle(t *testing.T) {
	cfg := testConfig()
	s := NewProxyLogRetentionScheduler(cfg)

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

// TestProxyLogRetention_StartDisabled verifies that Start returns nil without
// starting the runner when retention days is 0 (disabled).
func TestProxyLogRetention_StartDisabled(t *testing.T) {
	cfg := testConfig()
	cfg.ProxyLogRetentionDays = 0
	s := NewProxyLogRetentionScheduler(cfg)

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start should not error when disabled: %v", err)
	}
	// Stop should be a no-op (runner was never started).
	if err := s.Stop(); err != nil {
		t.Errorf("Stop failed: %v", err)
	}
}

// TestProxyLogRetention_StartDisabled_LogCleanupConfigured verifies that Start
// returns nil when the log-cleanup system is configured (the legacy fallback
// is disabled).
func TestProxyLogRetention_StartDisabled_LogCleanupConfigured(t *testing.T) {
	cfg := testConfig()
	cfg.LogCleanupConfigured = true
	s := NewProxyLogRetentionScheduler(cfg)

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start should not error when disabled: %v", err)
	}
	if err := s.Stop(); err != nil {
		t.Errorf("Stop failed: %v", err)
	}
}

// TestRetentionScheduler_runCleanup_WithDB exercises the full runCleanup flow:
// store.GetDB lookup, retention-days check, context creation, lease
// acquisition, and runCleanupLocked. On an empty DB the DELETE affects 0 rows.
func TestRetentionScheduler_runCleanup_WithDB(t *testing.T) {
	ResetLeasePressureForTest()
	db := openRetentionSchedTestDB(t)
	store.OverrideDB(db)
	t.Cleanup(func() { store.OverrideDB(nil) })

	cfg := testConfig()
	s := NewProxyLogRetentionScheduler(cfg)
	// Set lifecycle ctx so runCleanup can derive a job timeout.
	s.ctx, s.cancel = context.WithCancel(context.Background())

	s.runCleanup()
}

// TestRetentionScheduler_runCleanup_NilDB verifies that runCleanup handles a
// nil DB gracefully (returns without panic).
func TestRetentionScheduler_runCleanup_NilDB(t *testing.T) {
	ResetLeasePressureForTest()
	store.OverrideDB(nil)
	t.Cleanup(func() { store.OverrideDB(nil) })

	cfg := testConfig()
	s := NewProxyLogRetentionScheduler(cfg)

	s.runCleanup()
}

// TestRetentionScheduler_runCleanup_ZeroRetentionDays verifies that
// runCleanup returns without acquiring a lease when retention days is 0.
func TestRetentionScheduler_runCleanup_ZeroRetentionDays(t *testing.T) {
	ResetLeasePressureForTest()
	db := openRetentionSchedTestDB(t)
	store.OverrideDB(db)
	t.Cleanup(func() { store.OverrideDB(nil) })

	cfg := testConfig()
	cfg.ProxyLogRetentionDays = 0
	s := NewProxyLogRetentionScheduler(cfg)

	// Should return without panic (retentionDays <= 0 guard).
	s.runCleanup()
}

// TestRetentionScheduler_runCleanupLocked_EmptyDB verifies that
// runCleanupLocked executes the DELETE query on a fresh DB without error.
func TestRetentionScheduler_runCleanupLocked_EmptyDB(t *testing.T) {
	db := openRetentionSchedTestDB(t)
	cfg := testConfig()
	s := NewProxyLogRetentionScheduler(cfg)

	// Direct call — no lease, no GetDB. The DELETE on an empty proxy_logs
	// table affects 0 rows and the method returns without error.
	s.runCleanupLocked(db, 90)
}

// TestAdminBackgroundTaskRetention_StartStop_Lifecycle verifies the
// admin-background-task retention scheduler lifecycle.
func TestAdminBackgroundTaskRetention_StartStop_Lifecycle(t *testing.T) {
	cfg := testConfig()
	s := NewAdminBackgroundTaskRetentionScheduler(cfg)

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	if err := s.Stop(); err != nil {
		t.Errorf("Stop failed: %v", err)
	}
}

// TestAdminBackgroundTaskRetention_runCleanupLocked_EmptyDB verifies that
// runCleanupLocked executes the DELETE query (with the status filter) on a
// fresh DB without error.
func TestAdminBackgroundTaskRetention_runCleanupLocked_EmptyDB(t *testing.T) {
	db := openRetentionSchedTestDB(t)
	cfg := testConfig()
	s := NewAdminBackgroundTaskRetentionScheduler(cfg)

	s.runCleanupLocked(db, adminBackgroundTaskRetentionDays)
}

// TestProxyVideoTaskRetention_StartStop_Lifecycle verifies the
// proxy-video-task retention scheduler lifecycle.
func TestProxyVideoTaskRetention_StartStop_Lifecycle(t *testing.T) {
	cfg := &config.Config{
		ProxyVideoTaskRetentionDays:                 90,
		ProxyVideoTaskRetentionPruneIntervalMinutes: 60,
	}
	s := NewProxyVideoTaskRetentionScheduler(cfg)

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	if err := s.Stop(); err != nil {
		t.Errorf("Stop failed: %v", err)
	}
}

// TestAdminBackgroundTaskRetention_runCleanup_WithDB exercises the full
// runCleanup flow for the admin-background-task retention scheduler.
func TestAdminBackgroundTaskRetention_runCleanup_WithDB(t *testing.T) {
	ResetLeasePressureForTest()
	db := openRetentionSchedTestDB(t)
	store.OverrideDB(db)
	t.Cleanup(func() { store.OverrideDB(nil) })

	cfg := testConfig()
	s := NewAdminBackgroundTaskRetentionScheduler(cfg)
	s.ctx, s.cancel = context.WithCancel(context.Background())

	s.runCleanup()
}

// TestProxyVideoTaskRetention_runCleanupLocked_EmptyDB exercises the DELETE
// query on the proxy_video_tasks table on a fresh DB.
func TestProxyVideoTaskRetention_runCleanupLocked_EmptyDB(t *testing.T) {
	db := openRetentionSchedTestDB(t)
	cfg := &config.Config{
		ProxyVideoTaskRetentionDays: 90,
	}
	s := NewProxyVideoTaskRetentionScheduler(cfg)

	s.runCleanupLocked(db, 90)
}

// TestProxyVideoTaskRetention_StartDisabled verifies that Start returns nil
// when retention days is 0.
func TestProxyVideoTaskRetention_StartDisabled(t *testing.T) {
	cfg := &config.Config{
		ProxyVideoTaskRetentionDays: 0,
	}
	s := NewProxyVideoTaskRetentionScheduler(cfg)

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start should not error when disabled: %v", err)
	}
	if err := s.Stop(); err != nil {
		t.Errorf("Stop failed: %v", err)
	}
}
