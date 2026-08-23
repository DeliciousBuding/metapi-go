package scheduler

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
)

// seedRouteChannel inserts site -> account -> token route -> channel used by
// the probe-target loaders (mirrors channel_recovery_test seeding).
func seedRouteChannel(t *testing.T, db *store.DB, suffix, model string, enabled bool) (channelID int64) {
	t.Helper()
	now := "2026-01-01T00:00:00Z"
	res, err := db.Exec(`INSERT INTO sites (name, url, platform, status, created_at, updated_at) VALUES (?, ?, 'openai', 'active', ?, ?)`, "s-"+suffix, "https://s-"+suffix+".example.test", now, now)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	siteID, _ := res.LastInsertId()
	res, err = db.Exec(`INSERT INTO accounts (site_id, username, access_token, status, checkin_enabled, created_at, updated_at) VALUES (?, ?, 'tok', 'active', 1, ?, ?)`, siteID, "u-"+suffix, now, now)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	accountID, _ := res.LastInsertId()
	res, err = db.Exec(`INSERT INTO token_routes (model_pattern, route_mode, routing_strategy, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`, "all", "pattern", "weighted", 1, now, now)
	if err != nil {
		t.Fatalf("insert route: %v", err)
	}
	routeID, _ := res.LastInsertId()
	res, err = db.Exec(`INSERT INTO route_channels (route_id, account_id, source_model, priority, weight, enabled, cooldown_until) VALUES (?, ?, ?, 0, 10, ?, NULL)`, routeID, accountID, model, enabled)
	if err != nil {
		t.Fatalf("insert channel: %v", err)
	}
	channelID, _ = res.LastInsertId()
	return channelID
}

// TestModelProbeScheduler_RecentRunSummaries covers the ring buffer behind
// the scheduler-status "last few runs" view: newest-first ordering, honest
// success/failure counts, StartedAtMs populated, and the depth cap.
func TestModelProbeScheduler_RecentRunSummaries(t *testing.T) {
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store.OverrideDB(db)
	t.Cleanup(func() { store.OverrideDB(nil) })

	okChannel := seedRouteChannel(t, db, "ok", "gpt-ok", true)
	failChannel := seedRouteChannel(t, db, "fail", "gpt-fail", true)

	s := NewModelProbeScheduler(&config.Config{ModelAvailabilityProbeEnabled: true})
	s.SetProbeExecutor(&fakeProbe{outcomes: map[int64]ProbeOutcome{
		okChannel:   {Status: "success", LatencyMs: 21},
		failChannel: {Status: "failure", HTTPStatus: 502, ErrorText: "bad gateway"},
	}})
	s.SetHealthRecorder(&fakeRecorder{})

	// A single pass: both channels probed, one success one failure.
	s.TriggerNow(true)
	runs := s.RecentRunSummaries()
	if len(runs) != 1 {
		t.Fatalf("runs after first pass = %d, want 1", len(runs))
	}
	if runs[0].Success != 1 || runs[0].Failed != 1 {
		t.Fatalf("first run counts = success=%d failed=%d, want 1/1", runs[0].Success, runs[0].Failed)
	}
	if runs[0].TargetsScanned != 2 {
		t.Fatalf("first run targets = %d, want 2", runs[0].TargetsScanned)
	}
	if runs[0].StartedAtMs == 0 || runs[0].CompletedAtMs < runs[0].StartedAtMs {
		t.Fatalf("timestamps not honest: started=%d completed=%d", runs[0].StartedAtMs, runs[0].CompletedAtMs)
	}
	if last := s.LastRunSummary(); last.CompletedAtMs != runs[0].CompletedAtMs {
		t.Fatalf("LastRunSummary diverged from RecentRunSummaries[0]")
	}

	// Passes accumulate newest-first and the buffer is depth-capped.
	for i := 0; i < 12; i++ {
		s.TriggerNow(true)
	}
	runs = s.RecentRunSummaries()
	if len(runs) != probeRunHistoryDepth {
		t.Fatalf("runs after 13 passes = %d, want capped at %d", len(runs), probeRunHistoryDepth)
	}
	for i := 1; i < len(runs); i++ {
		if runs[i].CompletedAtMs > runs[i-1].CompletedAtMs {
			t.Fatalf("run %d not newest-first: %d > %d", i, runs[i-1].CompletedAtMs, runs[i].CompletedAtMs)
		}
	}
}
