package scheduler

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/service/checkin"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

// ---- checkin_test.go: interval-mode filterDue, stopLocked close-once,
// and runIntervalPassLocked end-to-end with a mock checkin function. ----

// strPtr returns a pointer to s (helper for intervalCandidate.lastCheckinAt).
func strPtr(s string) *string { return &s }

// newTestCheckinScheduler builds a CheckinScheduler in interval mode with the
// given interval hours. The checkinAll hook is left at its production default
// (checkin.CheckinAll); tests that need to inject a mock override it after
// construction.
func newTestCheckinScheduler(t *testing.T, intervalHours int) *CheckinScheduler {
	t.Helper()
	config.SetRuntime(&config.RuntimeSettings{
		CheckinScheduleMode:  "interval",
		CheckinIntervalHours: intervalHours,
	})
	t.Cleanup(func() { config.SetRuntime(nil) })
	return NewCheckinScheduler(&config.Config{})
}

// equalIDSets reports whether a and b contain the same IDs regardless of order.
func equalIDSets(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[int64]bool, len(a))
	for _, v := range a {
		seen[v] = true
	}
	for _, v := range b {
		if !seen[v] {
			return false
		}
	}
	return true
}

// ---- filterDue table-driven tests ----
//
// filterDue is the core interval-mode decision function. It must:
//   - Mark accounts due when last_checkin_at is nil/empty/invalid or older
//     than the interval.
//   - Suppress accounts whose last attempt is within the interval (attempt-map
//     backoff), but only when the attempt is at or after the last known
//     checkin (mid-flight suppression).

func TestFilterDue_TableDriven(t *testing.T) {
	// Fixed "now" so timestamp math is deterministic. 2026-08-15 12:00 UTC.
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	const intervalHours = 6

	tests := []struct {
		name       string
		candidates []intervalCandidate
		attempts   map[int64]int64 // pre-seeded attempt timestamps (ms since epoch)
		wantDueIDs []int64
	}{
		{
			name:       "nil last_checkin_at no attempt -> due",
			candidates: []intervalCandidate{{id: 1, lastCheckinAt: nil}},
			wantDueIDs: []int64{1},
		},
		{
			name:       "empty last_checkin_at string no attempt -> due",
			candidates: []intervalCandidate{{id: 2, lastCheckinAt: strPtr("")}},
			wantDueIDs: []int64{2},
		},
		{
			name:       "invalid timestamp treated as never checked -> due",
			candidates: []intervalCandidate{{id: 3, lastCheckinAt: strPtr("not-a-time")}},
			wantDueIDs: []int64{3},
		},
		{
			name:       "recent checkin within interval -> not due",
			candidates: []intervalCandidate{{id: 4, lastCheckinAt: strPtr(now.Add(-2 * time.Hour).Format(time.RFC3339))}},
			wantDueIDs: nil,
		},
		{
			name:       "old checkin beyond interval -> due",
			candidates: []intervalCandidate{{id: 5, lastCheckinAt: strPtr(now.Add(-8 * time.Hour).Format(time.RFC3339))}},
			wantDueIDs: []int64{5},
		},
		{
			name:       "nil checkin + recent attempt within interval -> not due (attempt-map suppression)",
			candidates: []intervalCandidate{{id: 6, lastCheckinAt: nil}},
			attempts:   map[int64]int64{6: now.Add(-1 * time.Hour).UnixMilli()},
			wantDueIDs: nil,
		},
		{
			name:       "nil checkin + old attempt beyond interval -> due",
			candidates: []intervalCandidate{{id: 7, lastCheckinAt: nil}},
			attempts:   map[int64]int64{7: now.Add(-8 * time.Hour).UnixMilli()},
			wantDueIDs: []int64{7},
		},
		{
			name:       "old checkin + recent attempt within interval and attempt >= checkin -> not due (mid-flight)",
			candidates: []intervalCandidate{{id: 8, lastCheckinAt: strPtr(now.Add(-8 * time.Hour).Format(time.RFC3339))}},
			attempts:   map[int64]int64{8: now.Add(-1 * time.Hour).UnixMilli()},
			wantDueIDs: nil,
		},
		{
			name:       "old checkin + old attempt beyond interval -> due",
			candidates: []intervalCandidate{{id: 9, lastCheckinAt: strPtr(now.Add(-10 * time.Hour).Format(time.RFC3339))}},
			attempts:   map[int64]int64{9: now.Add(-8 * time.Hour).UnixMilli()},
			wantDueIDs: []int64{9},
		},
		{
			name:       "old checkin + attempt older than checkin -> due (attempt doesn't suppress, checkin is newer)",
			candidates: []intervalCandidate{{id: 10, lastCheckinAt: strPtr(now.Add(-8 * time.Hour).Format(time.RFC3339))}},
			attempts:   map[int64]int64{10: now.Add(-12 * time.Hour).UnixMilli()},
			wantDueIDs: []int64{10},
		},
		{
			name: "mixed: nil + recent + old + suppressed -> only nil and old are due",
			candidates: []intervalCandidate{
				{id: 11, lastCheckinAt: nil},                                                   // due
				{id: 12, lastCheckinAt: strPtr(now.Add(-2 * time.Hour).Format(time.RFC3339))},  // not due (recent)
				{id: 13, lastCheckinAt: strPtr(now.Add(-8 * time.Hour).Format(time.RFC3339))},  // due (old)
				{id: 14, lastCheckinAt: strPtr(now.Add(-8 * time.Hour).Format(time.RFC3339))},  // not due (suppressed)
			},
			attempts:   map[int64]int64{14: now.Add(-1 * time.Hour).UnixMilli()},
			wantDueIDs: []int64{11, 13},
		},
		{
			name:       "no candidates -> empty",
			candidates: []intervalCandidate{},
			wantDueIDs: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestCheckinScheduler(t, intervalHours)
			if tt.attempts != nil {
				s.mu.Lock()
				for k, v := range tt.attempts {
					s.attemptByAccount[k] = v
				}
				s.mu.Unlock()
			}
			got := s.filterDue(tt.candidates, now)
			if !equalIDSets(got, tt.wantDueIDs) {
				t.Errorf("filterDue got %v, want %v", got, tt.wantDueIDs)
			}
		})
	}
}

// TestFilterDue_ClampsIntervalHours ensures config.ClampInt is applied so a
// zero or negative interval hours doesn't cause a divide-by-zero or negative
// interval (which would mark everything due immediately).
func TestFilterDue_ClampsIntervalHours(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	// Checkin 1 minute ago. With intervalHours=0, ClampInt(0,1,24)=1 so the
	// account is NOT due (1h interval, checkin 1m ago).
	recent := now.Add(-1 * time.Minute).Format(time.RFC3339)

	config.SetRuntime(&config.RuntimeSettings{CheckinIntervalHours: 0})
	t.Cleanup(func() { config.SetRuntime(nil) })
	s := &CheckinScheduler{
		cfg:              &config.Config{},
		attemptByAccount: make(map[int64]int64),
		checkinAll:       checkin.CheckinAll,
	}
	got := s.filterDue([]intervalCandidate{{id: 1, lastCheckinAt: strPtr(recent)}}, now)
	if len(got) != 0 {
		t.Errorf("with clamped interval=1h, recent checkin should not be due, got %v", got)
	}
}

// ---- stopLocked close-once semantics ----
//
// stopLocked must be idempotent: calling it twice must not panic (no
// "close of closed channel"). The close-once guard uses a select on the
// already-closed channel to detect the prior close.

func TestStopLocked_IntervalModeDoubleStopDoesNotPanic(t *testing.T) {
	s := newTestCheckinScheduler(t, 6)
	// Simulate interval-mode start: create intervalStop and a ticker.
	s.intervalStop = make(chan struct{})
	s.intervalTimer = time.NewTicker(time.Duration(checkinPollMs) * time.Millisecond)
	defer s.intervalTimer.Stop()

	s.stopLocked()
	// Second call must not panic — the close-once guard detects the prior close.
	s.stopLocked()

	// Verify intervalStop is closed.
	select {
	case <-s.intervalStop:
	default:
		t.Fatal("intervalStop should be closed after stopLocked")
	}
}

func TestStopLocked_CronModeDoubleStopDoesNotPanic(t *testing.T) {
	s := newTestCheckinScheduler(t, 6)
	// Cron mode: cronRunner is set, intervalStop is nil.
	s.cronRunner = newCronRunner()
	s.cronRunner.start()

	s.stopLocked()
	// Second call: cronRunner is now nil, intervalStop is nil — no panic.
	s.stopLocked()
}

func TestStopLocked_NilEverythingDoesNotPanic(t *testing.T) {
	s := newTestCheckinScheduler(t, 6)
	// Nothing initialized — all fields nil/zero. stopLocked must handle this.
	s.stopLocked()
	s.stopLocked()
}

// ---- RandomCronInWindow window-bounds spot check ----
//
// RandomCronInWindow is exhaustively tested in cron_window_test.go. This
// adds the specific 06:00-10:00 example called out in the P2 spec so a future
// regression that breaks the morning-window case is caught here too.

func TestRandomCronInWindow_MorningWindowBounds(t *testing.T) {
	const startMin = 6 * 60 // 06:00 = 360
	const endMin = 10 * 60  // 10:00 = 600

	for i := 0; i < 100; i++ {
		expr, err := RandomCronInWindow("06:00", "10:00")
		if err != nil {
			t.Fatalf("RandomCronInWindow(06:00, 10:00): %v", err)
		}
		if !ValidateCronExpr(expr) {
			t.Fatalf("rolled cron %q is not a valid expression", expr)
		}
		var m, h int
		if _, err := fmt.Sscanf(expr, "%d %d", &m, &h); err != nil {
			t.Fatalf("unexpected cron shape %q: %v", expr, err)
		}
		rolled := h*60 + m
		if rolled < startMin || rolled > endMin {
			t.Fatalf("rolled %d min (cron %q), want within [%d,%d]", rolled, expr, startMin, endMin)
		}
	}
}

// ---- runIntervalPassLocked end-to-end with mock checkin ----
//
// This exercises the full interval pass: DB query -> filterDue -> mock
// checkinAll -> attempt-map update. The mock records which account IDs were
// passed and returns success for all, so the attempt map should be populated.

func setupIntervalTestDB(t *testing.T) *store.DB {
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

func insertIntervalTestSite(t *testing.T, db *store.DB, name, status string) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(
		"INSERT INTO sites (name, url, platform, status, created_at, updated_at) VALUES (?, ?, 'new-api', ?, ?, ?)",
		name, "https://"+name+".example.test", status, now, now,
	)
	if err != nil {
		t.Fatalf("insert site %s: %v", name, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("site id: %v", err)
	}
	return id
}

func insertIntervalTestAccount(t *testing.T, db *store.DB, siteID int64, username, status, lastCheckinAt string, enabled bool) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	enabledArg := any(1)
	if !enabled {
		enabledArg = 0
	}
	res, err := db.Exec(
		"INSERT INTO accounts (site_id, username, access_token, status, checkin_enabled, last_checkin_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		siteID, username, "sk-"+username, status, enabledArg, intervalNullableStr(lastCheckinAt), now, now,
	)
	if err != nil {
		t.Fatalf("insert account %s: %v", username, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("account id: %v", err)
	}
	return id
}

// intervalNullableStr returns the empty string as a SQL NULL (nil) and
// otherwise the literal value — mirrors nullableStr in the stale catch-up
// tests but is kept local to avoid cross-test coupling.
func intervalNullableStr(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func TestRunIntervalPassLocked_CallsCheckinForDueAccountsOnly(t *testing.T) {
	db := setupIntervalTestDB(t)
	activeSite := insertIntervalTestSite(t, db, "active-site", "active")

	// Due: nil last_checkin_at -> must be picked up.
	dueNilID := insertIntervalTestAccount(t, db, activeSite, "due-nil", "active", "", true)
	// Due: old checkin (12h ago, beyond 6h interval) -> must be picked up.
	oldCheckin := time.Now().Add(-12 * time.Hour).UTC().Format(time.RFC3339)
	dueOldID := insertIntervalTestAccount(t, db, activeSite, "due-old", "active", oldCheckin, true)
	// Not due: recent checkin (1h ago, within 6h interval) -> must be skipped.
	recentCheckin := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	insertIntervalTestAccount(t, db, activeSite, "recent", "active", recentCheckin, true)
	// Not eligible: account disabled -> skipped by the SQL WHERE clause.
	insertIntervalTestAccount(t, db, activeSite, "disabled-acct", "disabled", "", true)
	// Not eligible: checkin_enabled = false -> skipped by the SQL WHERE clause.
	insertIntervalTestAccount(t, db, activeSite, "checkin-off", "active", "", false)
	// Not eligible: disabled site -> skipped by the INNER JOIN / site status.
	disabledSite := insertIntervalTestSite(t, db, "disabled-site", "disabled")
	insertIntervalTestAccount(t, db, disabledSite, "disabled-site-acct", "active", "", true)

	config.SetRuntime(&config.RuntimeSettings{
		CheckinScheduleMode:  "interval",
		CheckinIntervalHours: 6,
	})
	t.Cleanup(func() { config.SetRuntime(nil) })
	s := NewCheckinScheduler(&config.Config{})

	var (
		calledIDs []int64
		callCount int32
	)
	s.checkinAll = func(_ *config.Config, _ *sqlx.DB, ids []int64, mode string) []checkin.CheckinAllResult {
		atomic.AddInt32(&callCount, 1)
		calledIDs = make([]int64, len(ids))
		copy(calledIDs, ids)
		if mode != "interval" {
			t.Errorf("checkinAll mode = %q, want %q", mode, "interval")
		}
		results := make([]checkin.CheckinAllResult, 0, len(ids))
		for _, id := range ids {
			results = append(results, checkin.CheckinAllResult{
				AccountID: id,
				Result:     checkin.CheckinResult{Success: true},
			})
		}
		return results
	}

	s.runIntervalPassLocked(db)

	if callCount != 1 {
		t.Fatalf("checkinAll called %d times, want 1", callCount)
	}
	wantDue := []int64{dueNilID, dueOldID}
	if !equalIDSets(calledIDs, wantDue) {
		t.Fatalf("calledIDs = %v, want %v", calledIDs, wantDue)
	}

	// Verify the attempt map was populated for every called account.
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range calledIDs {
		if _, ok := s.attemptByAccount[id]; !ok {
			t.Errorf("attempt map missing account %d after run", id)
		}
	}
}

func TestRunIntervalPassLocked_NoDueAccountsDoesNotCallCheckin(t *testing.T) {
	db := setupIntervalTestDB(t)
	activeSite := insertIntervalTestSite(t, db, "all-recent-site", "active")
	recentCheckin := time.Now().Add(-30 * time.Minute).UTC().Format(time.RFC3339)
	insertIntervalTestAccount(t, db, activeSite, "recent-a", "active", recentCheckin, true)
	insertIntervalTestAccount(t, db, activeSite, "recent-b", "active", recentCheckin, true)

	config.SetRuntime(&config.RuntimeSettings{
		CheckinScheduleMode:  "interval",
		CheckinIntervalHours: 6,
	})
	t.Cleanup(func() { config.SetRuntime(nil) })
	s := NewCheckinScheduler(&config.Config{})

	var callCount int32
	s.checkinAll = func(_ *config.Config, _ *sqlx.DB, _ []int64, _ string) []checkin.CheckinAllResult {
		atomic.AddInt32(&callCount, 1)
		return nil
	}

	s.runIntervalPassLocked(db)

	if callCount != 0 {
		t.Fatalf("checkinAll called %d times, want 0 (no due accounts)", callCount)
	}

	// Attempt map should remain empty.
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.attemptByAccount) != 0 {
		t.Errorf("attempt map = %v, want empty", s.attemptByAccount)
	}
}

func TestRunIntervalPassLocked_FailedResultsStillUpdateAttemptMap(t *testing.T) {
	db := setupIntervalTestDB(t)
	activeSite := insertIntervalTestSite(t, db, "fail-site", "active")
	dueID := insertIntervalTestAccount(t, db, activeSite, "due-fail", "active", "", true)

	config.SetRuntime(&config.RuntimeSettings{
		CheckinScheduleMode:  "interval",
		CheckinIntervalHours: 6,
	})
	t.Cleanup(func() { config.SetRuntime(nil) })
	s := NewCheckinScheduler(&config.Config{})

	s.checkinAll = func(_ *config.Config, _ *sqlx.DB, ids []int64, _ string) []checkin.CheckinAllResult {
		results := make([]checkin.CheckinAllResult, 0, len(ids))
		for _, id := range ids {
			results = append(results, checkin.CheckinAllResult{
				AccountID: id,
				Result:    checkin.CheckinResult{Success: false, Message: "upstream error"},
			})
		}
		return results
	}

	s.runIntervalPassLocked(db)

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.attemptByAccount[dueID]; !ok {
		t.Errorf("attempt map missing failed account %d — failures must still record an attempt", dueID)
	}
}

// ---- runIntervalPass via store.OverrideDB + mock ----
//
// Exercises the public runIntervalPass entry point, which acquires the
// scheduler lease and then delegates to runIntervalPassLocked. With
// store.OverrideDB pointing at an in-memory SQLite DB and the lease using
// local (non-Postgres) semantics, this path is fully testable.

func TestRunIntervalPass_OverridesDBAndCallsCheckin(t *testing.T) {
	db := setupIntervalTestDB(t)
	store.OverrideDB(db)
	t.Cleanup(func() { store.OverrideDB(nil) })

	activeSite := insertIntervalTestSite(t, db, "override-site", "active")
	dueID := insertIntervalTestAccount(t, db, activeSite, "override-due", "active", "", true)

	config.SetRuntime(&config.RuntimeSettings{
		CheckinScheduleMode:  "interval",
		CheckinIntervalHours: 6,
	})
	t.Cleanup(func() { config.SetRuntime(nil) })
	s := NewCheckinScheduler(&config.Config{})

	var calledIDs []int64
	s.checkinAll = func(_ *config.Config, _ *sqlx.DB, ids []int64, _ string) []checkin.CheckinAllResult {
		calledIDs = append(calledIDs, ids...)
		return nil
	}

	s.runIntervalPass()

	if len(calledIDs) != 1 || calledIDs[0] != dueID {
		t.Fatalf("calledIDs = %v, want [%d]", calledIDs, dueID)
	}
}
