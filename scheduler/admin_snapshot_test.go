package scheduler

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

// openAdminSnapshotTestDB opens a fresh in-memory SQLite DB with AutoMigrate.
func openAdminSnapshotTestDB(t *testing.T) *store.DB {
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

// TestAdminSnapshotScheduler_WarmOnce_WithDB exercises WarmOnce → runWarm
// with a DB override and nil aggregation. On an empty DB the warm targets
// stub-complete and pruneExpired runs a no-op DELETE. This covers the main
// runWarm body (warmTarget, pruneExpired, passCount increment).
func TestAdminSnapshotScheduler_WarmOnce_WithDB(t *testing.T) {
	ResetLeasePressureForTest()
	db := openAdminSnapshotTestDB(t)
	store.OverrideDB(db)
	t.Cleanup(func() { store.OverrideDB(nil) })

	cfg := testConfig()
	// Pass nil aggregation so RunProjectionPass is skipped.
	s := NewAdminSnapshotScheduler(cfg, nil)

	s.WarmOnce()

	// After the pass, inFlight should be false (defer resets it) and
	// passCount should be 1.
	s.mu.Lock()
	inFlight := s.inFlight
	passCount := s.passCount
	s.mu.Unlock()

	if inFlight {
		t.Error("inFlight should be false after WarmOnce completes")
	}
	if passCount != 1 {
		t.Errorf("passCount = %d, want 1", passCount)
	}
}

// TestAdminSnapshotScheduler_WarmOnce_NilDB verifies that WarmOnce handles
// a nil DB gracefully (runWarm returns after the DB nil-check).
func TestAdminSnapshotScheduler_WarmOnce_NilDB(t *testing.T) {
	ResetLeasePressureForTest()
	store.OverrideDB(nil)
	t.Cleanup(func() { store.OverrideDB(nil) })

	cfg := testConfig()
	s := NewAdminSnapshotScheduler(cfg, nil)

	s.WarmOnce()

	// inFlight should be false (defer resets it) but passCount should be 0
	// (the pass returned early before incrementing).
	s.mu.Lock()
	inFlight := s.inFlight
	passCount := s.passCount
	s.mu.Unlock()

	if inFlight {
		t.Error("inFlight should be false")
	}
	if passCount != 0 {
		t.Errorf("passCount = %d, want 0 (nil DB returns before increment)", passCount)
	}
}

// TestAdminSnapshotScheduler_WarmOnce_InFlightGuard verifies that WarmOnce
// returns immediately when a warm pass is already in flight.
func TestAdminSnapshotScheduler_WarmOnce_InFlightGuard(t *testing.T) {
	ResetLeasePressureForTest()
	store.OverrideDB(nil)
	t.Cleanup(func() { store.OverrideDB(nil) })

	cfg := testConfig()
	s := NewAdminSnapshotScheduler(cfg, nil)

	// Simulate an in-flight pass.
	s.mu.Lock()
	s.inFlight = true
	s.mu.Unlock()

	s.WarmOnce()

	// passCount should still be 0 — the guard short-circuited.
	s.mu.Lock()
	passCount := s.passCount
	s.mu.Unlock()
	if passCount != 0 {
		t.Errorf("passCount = %d, want 0 (inFlight guard short-circuited)", passCount)
	}
}

// TestAdminSnapshotScheduler_pruneExpired_EmptyDB verifies that pruneExpired
// executes the DELETE query on a fresh DB without error. On an empty
// admin_snapshots table, 0 rows are deleted.
func TestAdminSnapshotScheduler_pruneExpired_EmptyDB(t *testing.T) {
	ResetLeasePressureForTest()
	db := openAdminSnapshotTestDB(t)
	store.OverrideDB(db)
	t.Cleanup(func() { store.OverrideDB(nil) })

	cfg := testConfig()
	s := NewAdminSnapshotScheduler(cfg, nil)

	// Direct call — should not panic. The admin_snapshots table exists from
	// AutoMigrate and the DELETE affects 0 rows.
	s.pruneExpired()
}

// TestAdminSnapshotScheduler_pruneExpired_NilDB verifies that pruneExpired
// handles a nil DB gracefully.
func TestAdminSnapshotScheduler_pruneExpired_NilDB(t *testing.T) {
	ResetLeasePressureForTest()
	store.OverrideDB(nil)
	t.Cleanup(func() { store.OverrideDB(nil) })

	cfg := testConfig()
	s := NewAdminSnapshotScheduler(cfg, nil)

	// Should not panic.
	s.pruneExpired()
}

// TestAdminSnapshotScheduler_warmTarget_NilDB verifies that warmTarget
// handles a nil DB gracefully.
func TestAdminSnapshotScheduler_warmTarget_NilDB(t *testing.T) {
	ResetLeasePressureForTest()
	store.OverrideDB(nil)
	t.Cleanup(func() { store.OverrideDB(nil) })

	cfg := testConfig()
	s := NewAdminSnapshotScheduler(cfg, nil)

	// Should not panic (stub path returns on nil DB).
	s.warmTarget("dashboard-summary")
}
