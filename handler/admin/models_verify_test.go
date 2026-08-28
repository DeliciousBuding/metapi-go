package admin

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/scheduler"
	"github.com/deliciousbuding/metapi-go/store"
)

// ---- G1: batch model verification + history ----

type fakeVerifyProbe struct {
	mu       sync.Mutex
	outcomes map[int64]scheduler.ProbeOutcome
}

func (f *fakeVerifyProbe) ProbeChannel(_ context.Context, target scheduler.ProbeTarget) (scheduler.ProbeOutcome, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if o, ok := f.outcomes[target.ChannelID]; ok {
		return o, nil
	}
	return scheduler.ProbeOutcome{Status: "success", LatencyMs: 42}, nil
}

// setupVerifyHarness creates a site + account + route with two enabled
// channels (verify-model-a / verify-model-b), wires a fake probe executor
// into the global scheduler, and returns the stats router + ids.
func setupVerifyHarness(t *testing.T) (chi.Router, *store.DB, int64) {
	t.Helper()
	db, _ := setupStatsSQLiteTest(t)

	now := time.Now().UTC().Format(time.RFC3339)

	res, err := db.Exec(`INSERT INTO sites (name, url, platform, status, created_at, updated_at)
		VALUES (?, ?, ?, 'active', ?, ?)`, "VerifySite", "https://verify.example.test", "openai", now, now)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	siteID, _ := res.LastInsertId()

	res, err = db.Exec(`INSERT INTO accounts (site_id, username, access_token, api_token, status, checkin_enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'active', FALSE, ?, ?)`, siteID, "verify-user", "sess", "sk-verify", now, now)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	accountID, _ := res.LastInsertId()

	res, err = db.Exec(`INSERT INTO token_routes (model_pattern, display_name, route_mode, routing_strategy, enabled, created_at, updated_at)
		VALUES (?, ?, 'standard', 'weighted', TRUE, ?, ?)`, "verify-*", "Verify Route", now, now)
	if err != nil {
		t.Fatalf("insert route: %v", err)
	}
	routeID, _ := res.LastInsertId()

	// Two channels with different models on the same account.
	for _, m := range []string{"verify-model-a", "verify-model-b"} {
		_, err = db.Exec(`INSERT INTO route_channels (route_id, account_id, source_model, priority, weight, enabled)
			VALUES (?, ?, ?, 10, 10, TRUE)`, routeID, accountID, m)
		if err != nil {
			t.Fatalf("insert channel %s: %v", m, err)
		}
	}

	sched := scheduler.NewModelProbeScheduler(&config.Config{})
	sched.SetProbeExecutor(&fakeVerifyProbe{})
	scheduler.SetGlobalModelProbeScheduler(sched)
	t.Cleanup(func() { scheduler.SetGlobalModelProbeScheduler(nil) })

	// Rebuild the router on the harness db so routes include verify-batch.
	r2 := chi.NewRouter()
	RegisterStatsRoutes(r2, db.DB)
	return r2, db, accountID
}

func TestVerifyBatch_NoSchedulerReturns503(t *testing.T) {
	scheduler.SetGlobalModelProbeScheduler(nil)
	db, r := setupStatsSQLiteTest(t)
	_ = db

	resp := doPostJSON(t, r, "/api/models/verify-batch", map[string]any{"models": []string{"x"}})
	if resp.Code != 503 {
		t.Fatalf("expected 503 without scheduler, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestVerifyBatch_EndToEnd(t *testing.T) {
	r, db, accountID := setupVerifyHarness(t)

	// Second inserted channel (id 2) fails with 429; others succeed.
	sched := scheduler.GetGlobalModelProbeScheduler()
	sched.SetProbeExecutor(&fakeVerifyProbe{outcomes: map[int64]scheduler.ProbeOutcome{
		2: {Status: "failure", LatencyMs: 15, HTTPStatus: 429, ErrorText: "rate limited"},
	}})

	resp := doPostJSON(t, r, "/api/models/verify-batch", map[string]any{
		"models": []string{"verify-model-a", "verify-model-b"},
	})
	if resp.Code != 200 {
		t.Fatalf("verify-batch returned %d: %s", resp.Code, resp.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["success"] != true {
		t.Fatalf("success = %v, want true", body["success"])
	}
	items, ok := body["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("items = %#v, want 2 rows", body["items"])
	}

	byModel := make(map[string]map[string]any)
	for _, it := range items {
		m := it.(map[string]any)
		byModel[m["model"].(string)] = m
	}
	okA := byModel["verify-model-a"]
	if okA == nil {
		t.Fatalf("missing verify-model-a row: %#v", byModel)
	}
	if okA["status"] != "success" {
		t.Fatalf("model-a status = %v, want success", okA["status"])
	}
	failB := byModel["verify-model-b"]
	if failB == nil {
		t.Fatalf("missing verify-model-b row: %#v", byModel)
	}
	if failB["status"] != "failure" {
		t.Fatalf("model-b status = %v, want failure", failB["status"])
	}
	if failB["httpStatus"].(float64) != 429 {
		t.Fatalf("model-b httpStatus = %v, want 429", failB["httpStatus"])
	}
	if failB["errorText"] != "rate limited" {
		t.Fatalf("model-b errorText = %v", failB["errorText"])
	}

	summary := body["summary"].(map[string]any)
	if summary["success"].(float64) != 1 || summary["failure"].(float64) != 1 {
		t.Fatalf("summary = %#v, want 1 success + 1 failure", summary)
	}

	// History was persisted and is queryable with site name.
	resp = doGet(t, r, "/api/models/verify-history?limit=10")
	if resp.Code != 200 {
		t.Fatalf("verify-history returned %d: %s", resp.Code, resp.Body.String())
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal history: %v", err)
	}
	hist, ok := body["items"].([]any)
	if !ok || len(hist) != 2 {
		t.Fatalf("history items = %#v, want 2", body["items"])
	}
	first := hist[0].(map[string]any)
	if first["model"] != "verify-model-b" { // newest first; id DESC breaks ties
		t.Fatalf("history[0].model = %v, want verify-model-b", first["model"])
	}
	if first["siteName"] != "VerifySite" {
		t.Fatalf("history[0].siteName = %v, want VerifySite", first["siteName"])
	}
	if first["batchId"] == "" {
		t.Fatalf("history[0].batchId empty")
	}

	// accountId filter narrows the probe set.
	resp = doPostJSON(t, r, "/api/models/verify-batch", map[string]any{"accountId": accountID})
	if resp.Code != 200 {
		t.Fatalf("account-scoped verify-batch returned %d: %s", resp.Code, resp.Body.String())
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal account-scoped: %v", err)
	}
	if body["probed"].(float64) != 2 {
		t.Fatalf("account-scoped probed = %v, want 2", body["probed"])
	}

	// model filter that matches nothing yields an empty success.
	resp = doPostJSON(t, r, "/api/models/verify-batch", map[string]any{"models": []string{"no-such-model"}})
	if resp.Code != 200 {
		t.Fatalf("empty-match verify-batch returned %d: %s", resp.Code, resp.Body.String())
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal empty-match: %v", err)
	}
	if body["probed"].(float64) != 0 {
		t.Fatalf("empty-match probed = %v, want 0", body["probed"])
	}

	// History survives on disk across requests (query again after more rows).
	var count int
	if err := db.Get(&count, "SELECT COUNT(*) FROM model_verify_history"); err != nil {
		t.Fatalf("count history: %v", err)
	}
	if count != 4 {
		t.Fatalf("history rows = %d, want 4 (2 + 2)", count)
	}
}
