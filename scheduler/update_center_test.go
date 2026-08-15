package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// openUpdateCenterTestDB opens a fresh in-memory SQLite DB with AutoMigrate.
func openUpdateCenterTestDB(t *testing.T) *store.DB {
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

// TestUpdateCenterScheduler_runSyncLocked_NoOp verifies that runSyncLocked
// is a log-only no-op: it does not modify the database or panic. The update-
// center scheduler is intentionally inert (no remote registry polling, no
// version discovery, no updateAvailable invention).
func TestUpdateCenterScheduler_runSyncLocked_NoOp(t *testing.T) {
	db := openUpdateCenterTestDB(t)
	cfg := testConfig()
	s := NewUpdateCenterScheduler(cfg)

	// Snapshot the row count of schema_migrations before and after to prove
	// runSyncLocked does not write anything.
	var beforeCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&beforeCount); err != nil {
		t.Fatalf("count before: %v", err)
	}

	s.runSyncLocked(db)

	var afterCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&afterCount); err != nil {
		t.Fatalf("count after: %v", err)
	}
	if afterCount != beforeCount {
		t.Errorf("runSyncLocked modified the DB: schema_migrations count %d → %d (should be a no-op)",
			beforeCount, afterCount)
	}
}

// TestUpdateCenterScheduler_runSync_WithDB exercises the full runSync flow:
// inFlight guard, store.GetDB lookup, context creation, lease acquisition,
// and runSyncLocked. On a fresh DB this completes without error.
func TestUpdateCenterScheduler_runSync_WithDB(t *testing.T) {
	ResetLeasePressureForTest()
	db := openUpdateCenterTestDB(t)
	store.OverrideDB(db)
	t.Cleanup(func() { store.OverrideDB(nil) })

	cfg := testConfig()
	s := NewUpdateCenterScheduler(cfg)

	s.runSync()

	// After runSync completes, inFlight must be false (the defer resets it).
	s.mu.Lock()
	inFlight := s.inFlight
	s.mu.Unlock()
	if inFlight {
		t.Error("inFlight should be false after runSync completes")
	}
}

// TestUpdateCenterScheduler_runSync_NilDB verifies that runSync handles a
// nil DB gracefully (returns without panic after the inFlight guard).
func TestUpdateCenterScheduler_runSync_NilDB(t *testing.T) {
	ResetLeasePressureForTest()
	store.OverrideDB(nil)
	t.Cleanup(func() { store.OverrideDB(nil) })

	cfg := testConfig()
	s := NewUpdateCenterScheduler(cfg)

	s.runSync()

	// inFlight should be false — runSync set it true, then the defer reset it
	// even though dbw was nil (the return happens before lease acquisition,
	// but after the inFlight guard).
	s.mu.Lock()
	inFlight := s.inFlight
	s.mu.Unlock()
	if inFlight {
		t.Error("inFlight should be false after runSync with nil DB")
	}
}

// TestUpdateCenterScheduler_runSync_InFlightGuard verifies that when
// inFlight is already true, runSync returns immediately without touching
// the inFlight flag's deferred reset (the guard short-circuits before the
// defer is registered).
func TestUpdateCenterScheduler_runSync_InFlightGuard(t *testing.T) {
	ResetLeasePressureForTest()
	store.OverrideDB(nil)
	t.Cleanup(func() { store.OverrideDB(nil) })

	cfg := testConfig()
	s := NewUpdateCenterScheduler(cfg)

	// Simulate an in-flight pass.
	s.mu.Lock()
	s.inFlight = true
	s.mu.Unlock()

	s.runSync()

	// inFlight must still be true — the guard returned early and never
	// registered the defer that resets it.
	s.mu.Lock()
	inFlight := s.inFlight
	s.mu.Unlock()
	if !inFlight {
		t.Error("inFlight should still be true when the guard short-circuited")
	}
}

// TestUpdateCenterScheduler_StartStop_Lifecycle verifies the interval-runner
// wrapper: Start begins the ticker; Stop halts it. Both are idempotent.
func TestUpdateCenterScheduler_StartStop_Lifecycle(t *testing.T) {
	cfg := testConfig()
	s := NewUpdateCenterScheduler(cfg)

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
