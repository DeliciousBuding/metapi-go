// Test for the model-probe recentRuns extension on the unified
// scheduler-status view: a fresh background pass must surface the honest
// run summary (success/failed counts) as a recentRuns entry.

package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/scheduler"
	"github.com/deliciousbuding/metapi-go/store"
)

type statusProbeStub struct{}

func (statusProbeStub) ProbeChannel(_ context.Context, _ scheduler.ProbeTarget) (scheduler.ProbeOutcome, error) {
	return scheduler.ProbeOutcome{Status: "success", LatencyMs: 12}, nil
}

// statusRecorderStub accepts outcomes without touching routing state — the
// pass must still succeed so the run summary reports honest counts.
type statusRecorderStub struct{}

func (statusRecorderStub) RecordProbeSuccess(_ context.Context, _ int64, _ float64, _ *string, _ *int64) error {
	return nil
}

func (statusRecorderStub) RecordProbeFailure(_ context.Context, _ int64, _ *int, _ *string, _ *string, _ *int64) error {
	return nil
}

func TestSchedulerStatusModelProbeRecentRuns(t *testing.T) {
	db, r := setupSchedulerStatusTest(t)

	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(`INSERT INTO sites (name, url, platform, status, created_at, updated_at)
		VALUES ('p-site', 'https://p.example.test', 'openai', 'active', ?, ?)`, now, now)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	siteID, _ := res.LastInsertId()
	res, err = db.Exec(`INSERT INTO accounts (site_id, username, access_token, status, checkin_enabled, created_at, updated_at)
		VALUES (?, 'p-user', 'tok', 'active', TRUE, ?, ?)`, siteID, now, now)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	accountID, _ := res.LastInsertId()
	res, err = db.Exec(`INSERT INTO token_routes (model_pattern, route_mode, routing_strategy, enabled, created_at, updated_at)
		VALUES ('all', 'pattern', 'weighted', TRUE, ?, ?)`, now, now)
	if err != nil {
		t.Fatalf("insert route: %v", err)
	}
	routeID, _ := res.LastInsertId()
	_, err = db.Exec(`INSERT INTO route_channels (route_id, account_id, source_model, priority, weight, enabled, cooldown_until)
		VALUES (?, ?, 'gpt-4o', 0, 10, TRUE, NULL)`, routeID, accountID)
	if err != nil {
		t.Fatalf("insert channel: %v", err)
	}

	store.OverrideDB(db)
	t.Cleanup(func() { store.OverrideDB(nil) })

	prev := scheduler.GetGlobalModelProbeScheduler()
	t.Cleanup(func() { scheduler.SetGlobalModelProbeScheduler(prev) })
	sched := scheduler.NewModelProbeScheduler(&config.Config{})
	sched.SetProbeExecutor(statusProbeStub{})
	sched.SetHealthRecorder(statusRecorderStub{})
	scheduler.SetGlobalModelProbeScheduler(sched)

	sched.TriggerNow(true)

	resp := doGet(t, r, "/api/scheduler/status")
	if resp.Code != http.StatusOK {
		t.Fatalf("status returned %d: %s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	items, ok := body["items"].([]any)
	if !ok {
		t.Fatalf("items = %#v", body["items"])
	}
	var probeItem map[string]any
	for _, it := range items {
		item := it.(map[string]any)
		if item["job"] == "model-probe" {
			probeItem = item
		}
	}
	if probeItem == nil {
		t.Fatalf("model-probe job missing: %#v", items)
	}
	runs, ok := probeItem["recentRuns"].([]any)
	if !ok || len(runs) != 1 {
		t.Fatalf("recentRuns = %#v, want exactly 1 run", probeItem["recentRuns"])
	}
	run := runs[0].(map[string]any)
	if run["success"].(float64) != 1 || run["failed"].(float64) != 0 {
		t.Fatalf("recentRuns[0] counts = %#v, want success=1 failed=0", run)
	}
	if run["completedAt"] == "" || run["startedAt"] == "" {
		t.Fatalf("recentRuns[0] timestamps empty: %#v", run)
	}
	if probeItem["lastStatus"] != "success" {
		t.Fatalf("lastStatus = %v, want success", probeItem["lastStatus"])
	}
}
