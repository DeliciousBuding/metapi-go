package admin

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tokendancelab/metapi-go/store"
)

// ---- A2 (all-api-hub borrow): model cost distribution + latency gallery ----

func insertStatsProxyLog(t *testing.T, db *store.DB, model string, cost float64, latencyMs int64, status string, createdAt string) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO proxy_logs (model_requested, model_actual, status, total_tokens, estimated_cost, latency_ms, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		model, model, status, 10, cost, latencyMs, createdAt,
	)
	if err != nil {
		t.Fatalf("insert proxy log: %v", err)
	}
}

func TestStats_ModelCostDistribution_TopNWithOther(t *testing.T) {
	db, r := setupStatsSQLiteTest(t)

	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	mA := "cost-a-" + suffix
	mB := "cost-b-" + suffix
	mC := "cost-c-" + suffix
	now := time.Now().UTC().Format(time.RFC3339)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM proxy_logs WHERE model_requested IN (?, ?, ?)", mA, mB, mC)
	})

	insertStatsProxyLog(t, db, mA, 5.0, 100, "success", now)
	insertStatsProxyLog(t, db, mB, 3.0, 100, "success", now)
	insertStatsProxyLog(t, db, mC, 0.5, 100, "failed", now)

	resp := doGet(t, r, "/api/stats/model-cost-distribution?days=30&topN=2")
	if resp.Code != 200 {
		t.Fatalf("model-cost-distribution returned %d: %s", resp.Code, resp.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	items, ok := body["items"].([]any)
	if !ok || len(items) != 3 {
		t.Fatalf("items = %#v, want 3 (top2 + Other)", body["items"])
	}
	first := items[0].(map[string]any)
	if first["model"] != mA {
		t.Fatalf("items[0].model = %v, want %s", first["model"], mA)
	}
	if got := first["cost"].(float64); got != 5.0 {
		t.Fatalf("items[0].cost = %v, want 5.0", got)
	}
	last := items[2].(map[string]any)
	if last["model"] != "other" || last["label"] != "其他模型" {
		t.Fatalf("items[2] = %#v, want Other bucket", last)
	}
	if got := last["cost"].(float64); got != 0.5 {
		t.Fatalf("Other.cost = %v, want 0.5", got)
	}

	totals, ok := body["totals"].(map[string]any)
	if !ok {
		t.Fatalf("totals missing: %#v", body["totals"])
	}
	if got := totals["cost"].(float64); got != 8.5 {
		t.Fatalf("totals.cost = %v, want 8.5", got)
	}
	if got := totals["calls"].(float64); got != 3 {
		t.Fatalf("totals.calls = %v, want 3", got)
	}
	if got := totals["tokens"].(float64); got != 30 {
		t.Fatalf("totals.tokens = %v, want 30", got)
	}
}

func TestStats_LatencyHistogram_Buckets(t *testing.T) {
	db, r := setupStatsSQLiteTest(t)

	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	model := "hist-" + suffix
	now := time.Now().UTC().Format(time.RFC3339)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM proxy_logs WHERE model_requested = ?", model)
	})

	// 100ms → bucket 0, 600ms → bucket 500, 1200ms → bucket 1000 (bucketMs=500).
	insertStatsProxyLog(t, db, model, 0.1, 100, "success", now)
	insertStatsProxyLog(t, db, model, 0.1, 600, "success", now)
	insertStatsProxyLog(t, db, model, 0.1, 1200, "success", now)

	resp := doGet(t, r, "/api/stats/latency-histogram?days=7&bucketMs=500")
	if resp.Code != 200 {
		t.Fatalf("latency-histogram returned %d: %s", resp.Code, resp.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := body["total"].(float64); got != 3 {
		t.Fatalf("total = %v, want 3", got)
	}
	buckets, ok := body["buckets"].([]any)
	if !ok || len(buckets) != 3 {
		t.Fatalf("buckets = %#v, want 3 non-empty buckets", body["buckets"])
	}
	wantStarts := []float64{0, 500, 1000}
	for i, b := range buckets {
		bm := b.(map[string]any)
		if got := bm["bucketStartMs"].(float64); got != wantStarts[i] {
			t.Fatalf("buckets[%d].bucketStartMs = %v, want %v", i, got, wantStarts[i])
		}
		if got := bm["count"].(float64); got != 1 {
			t.Fatalf("buckets[%d].count = %v, want 1", i, got)
		}
		if got := bm["percent"].(float64); got < 33.3 || got > 33.4 {
			t.Fatalf("buckets[%d].percent = %v, want ~33.33", i, got)
		}
	}
}

func TestStats_LatencyTrend_PerDayProfile(t *testing.T) {
	db, r := setupStatsSQLiteTest(t)

	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	model := "trend-" + suffix
	now := time.Now().UTC()
	today := now.Format("2006-01-02")
	yesterday := now.Add(-24 * time.Hour).Format("2006-01-02")
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM proxy_logs WHERE model_requested = ?", model)
	})

	// Today: 2 successful calls at 100ms and 200ms. Yesterday: 1 failed call at 3000ms.
	insertStatsProxyLog(t, db, model, 0.1, 100, "success", now.Format(time.RFC3339))
	insertStatsProxyLog(t, db, model, 0.2, 200, "success", now.Format(time.RFC3339))
	insertStatsProxyLog(t, db, model, 0.3, 3000, "failed", now.Add(-24*time.Hour).Format(time.RFC3339))

	resp := doGet(t, r, "/api/stats/latency-trend?days=7")
	if resp.Code != 200 {
		t.Fatalf("latency-trend returned %d: %s", resp.Code, resp.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	points, ok := body["points"].([]any)
	if !ok || len(points) < 2 {
		t.Fatalf("points = %#v, want at least 2 days", body["points"])
	}

	byDay := make(map[string]map[string]any)
	for _, p := range points {
		pm := p.(map[string]any)
		byDay[pm["date"].(string)] = pm
	}

	tp, ok := byDay[today]
	if !ok {
		t.Fatalf("today %s missing from points %#v", today, byDay)
	}
	if got := tp["requests"].(float64); got != 2 {
		t.Fatalf("today.requests = %v, want 2", got)
	}
	if got := tp["avgLatencyMs"].(float64); got != 150 {
		t.Fatalf("today.avgLatencyMs = %v, want 150", got)
	}
	if got := tp["maxLatencyMs"].(float64); got != 200 {
		t.Fatalf("today.maxLatencyMs = %v, want 200", got)
	}
	// p95 of [100, 200] with n=2 → floor(0.05*2)=0 → descending sample [200,100][0].
	if got := tp["p95LatencyMs"].(float64); got != 200 {
		t.Fatalf("today.p95LatencyMs = %v, want 200", got)
	}
	if got := tp["successRate"].(float64); got != 1.0 {
		t.Fatalf("today.successRate = %v, want 1.0", got)
	}

	yp, ok := byDay[yesterday]
	if !ok {
		t.Fatalf("yesterday %s missing from points %#v", yesterday, byDay)
	}
	if got := yp["requests"].(float64); got != 1 {
		t.Fatalf("yesterday.requests = %v, want 1", got)
	}
	if got := yp["avgLatencyMs"].(float64); got != 3000 {
		t.Fatalf("yesterday.avgLatencyMs = %v, want 3000", got)
	}
	if got := yp["p95LatencyMs"].(float64); got != 3000 {
		t.Fatalf("yesterday.p95LatencyMs = %v, want 3000", got)
	}
	if got := yp["successRate"].(float64); got != 0.0 {
		t.Fatalf("yesterday.successRate = %v, want 0.0", got)
	}
}

// PostgreSQL dialect parity for the A2 gallery (skipped without PG_TEST_DSN).
func TestStats_PostgresModelCostDistributionAndLatency(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PG_TEST_DSN"))
	if dsn == "" {
		t.Skip("PG_TEST_DSN not set; skipping PostgreSQL integration test")
	}
	db, r := setupStatsPostgresTest(t)

	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	mA := "pg-gallery-a-" + suffix
	mB := "pg-gallery-b-" + suffix
	now := time.Now().UTC().Format(time.RFC3339)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM proxy_logs WHERE model_requested IN (?, ?)", mA, mB)
	})

	insertStatsProxyLog(t, db, mA, 2.5, 250, "success", now)
	insertStatsProxyLog(t, db, mB, 1.0, 1300, "success", now)

	resp := doGet(t, r, "/api/stats/model-cost-distribution?days=30&topN=1")
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal distribution: %v", err)
	}
	items := body["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("items = %#v, want top1 + Other", items)
	}
	if items[0].(map[string]any)["model"] != mA {
		t.Fatalf("items[0] = %#v, want %s", items[0], mA)
	}
	if items[1].(map[string]any)["model"] != "other" {
		t.Fatalf("items[1] = %#v, want Other", items[1])
	}

	resp = doGet(t, r, "/api/stats/latency-trend?days=7")
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal trend: %v", err)
	}
	points := body["points"].([]any)
	if len(points) < 1 {
		t.Fatalf("points = %#v, want at least one day", points)
	}
	// Both logs are in today's window: avg = (250+1300)/2 = 775.
	found := false
	for _, p := range points {
		if pm := p.(map[string]any); pm["requests"].(float64) == 2 {
			found = true
			if got := pm["avgLatencyMs"].(float64); got != 775 {
				t.Fatalf("avgLatencyMs = %v, want 775", got)
			}
		}
	}
	if !found {
		t.Fatalf("no day with 2 requests in %#v", points)
	}
}
