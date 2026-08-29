package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// openDailySummaryTestDB opens a fresh in-memory SQLite DB with AutoMigrate.
func openDailySummaryTestDB(t *testing.T) *store.DB {
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

// TestDailySummaryScheduler_runJobLocked_EmptyDB verifies that runJobLocked
// completes without panic on a fresh DB. CollectDailySummaryMetrics queries
// accounts, checkin_logs, and proxy_logs — all empty on a fresh DB — and
// returns valid zero-value metrics. SendNotification then fails because no
// notification channel is configured, but the scheduler logs the error and
// returns gracefully.
func TestDailySummaryScheduler_runJobLocked_EmptyDB(t *testing.T) {
	ResetLeasePressureForTest()
	db := openDailySummaryTestDB(t)
	testConfig() // publishes the runtime baseline
	s := NewDailySummaryScheduler()

	// Direct call to runJobLocked — exercises metric collection + notification
	// dispatch. On a fresh DB, metrics are all zeros and the notification
	// fails gracefully (no channel configured).
	s.runJobLocked(db)
}

// TestDailySummaryScheduler_runJob_WithOverrideDB exercises the full runJob
// flow: store.GetDB lookup, context creation, lease acquisition, and
// runJobLocked.
func TestDailySummaryScheduler_runJob_WithOverrideDB(t *testing.T) {
	ResetLeasePressureForTest()
	db := openDailySummaryTestDB(t)
	store.OverrideDB(db)
	t.Cleanup(func() { store.OverrideDB(nil) })

	testConfig() // publishes the runtime baseline
	s := NewDailySummaryScheduler()

	s.runJob()
}

// TestDailySummaryScheduler_runJob_NilDB verifies that runJob handles a nil
// DB gracefully (logs "database not available" and returns without panic).
func TestDailySummaryScheduler_runJob_NilDB(t *testing.T) {
	ResetLeasePressureForTest()
	store.OverrideDB(nil)
	t.Cleanup(func() { store.OverrideDB(nil) })

	testConfig() // publishes the runtime baseline
	s := NewDailySummaryScheduler()

	s.runJob()
}

// TestDailySummaryScheduler_StartStop_Lifecycle verifies the cron-job
// wrapper: Start registers a cron job and begins the runner; Stop halts it.
func TestDailySummaryScheduler_StartStop_Lifecycle(t *testing.T) {
	testConfig() // publishes the runtime baseline
	s := NewDailySummaryScheduler()

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
