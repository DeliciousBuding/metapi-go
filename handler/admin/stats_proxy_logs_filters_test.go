package admin

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// seedProxyLogsForFilterTests inserts three proxy_logs rows with distinct
// latency_ms / client_family / created_at values so that every list filter
// (latencyMin, latencyMax, client, from, to) has at least one matching and
// one non-matching row. The rows are standalone (no account_id) — the LEFT
// JOINs in the handler yield NULL site/user fields, which is fine for filter
// assertions. Returns the three created_at timestamps in RFC3339 form.
//
// Row A: latency 3000, client "openai-node",   created_at = t3 (latest)
// Row B: latency  500, client "anthropic-sdk", created_at = t2 (middle)
// Row C: latency 2500, client "openai-node",   created_at = t1 (earliest)
func seedProxyLogsForFilterTests(t *testing.T, db *store.DB) (t1, t2, t3 string) {
	t.Helper()
	t1 = "2026-01-01T01:00:00Z"
	t2 = "2026-01-01T02:00:00Z"
	t3 = "2026-01-01T03:00:00Z"
	rows := []struct {
		model        string
		latencyMs    int
		clientFamily string
		createdAt    string
	}{
		{"slow-A", 3000, "openai-node", t3},
		{"fast-B", 500, "anthropic-sdk", t2},
		{"slow-C", 2500, "openai-node", t1},
	}
	for _, row := range rows {
		if _, err := db.Exec(`INSERT INTO proxy_logs (model_requested, model_actual, status, latency_ms, client_family, created_at)
			VALUES (?, ?, ?, ?, ?, ?)`,
			row.model, row.model, "success", row.latencyMs, row.clientFamily, row.createdAt); err != nil {
			t.Fatalf("insert %s: %v", row.model, err)
		}
	}
	return t1, t2, t3
}

// proxyLogsFilterResponse captures just the list-page fields the filter
// tests assert on. Decoding into a typed struct is more robust than
// map[string]any when checking nested summary counts.
type proxyLogsFilterResponse struct {
	Items   []map[string]any `json:"items"`
	Total   int              `json:"total"`
	Summary struct {
		TotalCount int `json:"totalCount"`
	} `json:"summary"`
}

func fetchProxyLogsFiltered(t *testing.T, r chi.Router, query string) proxyLogsFilterResponse {
	t.Helper()
	resp := doGet(t, r, "/api/stats/proxy-logs?view=full&"+query)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /api/stats/proxy-logs?%s status=%d body=%s", query, resp.Code, resp.Body.String())
	}
	var body proxyLogsFilterResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal %s: %v", query, err)
	}
	return body
}

// modelSet extracts the distinct model_requested values from the response
// items for concise assertion comparisons.
func modelSet(items []map[string]any) map[string]struct{} {
	out := make(map[string]struct{}, len(items))
	for _, item := range items {
		if model, ok := item["modelRequested"].(string); ok {
			out[model] = struct{}{}
		}
	}
	return out
}

func TestStats_SQLiteProxyLogsLatencyMinFilter(t *testing.T) {
	db, r := setupStatsSQLiteTest(t)
	seedProxyLogsForFilterTests(t, db)

	// latencyMin=2000 (the "Slow only" preset): rows A (3000) + C (2500)
	// match; row B (500) is filtered out. items, total, AND summary must
	// agree — this is the core regression: previously `total` used the
	// unfiltered count while items were client-side filtered.
	body := fetchProxyLogsFiltered(t, r, "latencyMin=2000")
	if body.Total != 2 {
		t.Fatalf("latencyMin=2000 total=%d, want 2", body.Total)
	}
	if len(body.Items) != 2 {
		t.Fatalf("latencyMin=2000 items=%d, want 2", len(body.Items))
	}
	if body.Summary.TotalCount != 2 {
		t.Fatalf("latencyMin=2000 summary.totalCount=%d, want 2", body.Summary.TotalCount)
	}
	got := modelSet(body.Items)
	if _, ok := got["slow-A"]; !ok {
		t.Fatalf("latencyMin=2000 missing slow-A in items: %v", got)
	}
	if _, ok := got["slow-C"]; !ok {
		t.Fatalf("latencyMin=2000 missing slow-C in items: %v", got)
	}
}

func TestStats_SQLiteProxyLogsLatencyMaxFilter(t *testing.T) {
	db, r := setupStatsSQLiteTest(t)
	seedProxyLogsForFilterTests(t, db)

	// latencyMax=1000: only row B (500) matches.
	body := fetchProxyLogsFiltered(t, r, "latencyMax=1000")
	if body.Total != 1 {
		t.Fatalf("latencyMax=1000 total=%d, want 1", body.Total)
	}
	if len(body.Items) != 1 {
		t.Fatalf("latencyMax=1000 items=%d, want 1", len(body.Items))
	}
	got := modelSet(body.Items)
	if _, ok := got["fast-B"]; !ok {
		t.Fatalf("latencyMax=1000 expected fast-B, got %v", got)
	}
}

func TestStats_SQLiteProxyLogsLatencyRangeFilter(t *testing.T) {
	db, r := setupStatsSQLiteTest(t)
	seedProxyLogsForFilterTests(t, db)

	// Both bounds: latencyMin=2000&latencyMax=3000 → rows A (3000) + C (2500).
	// Row A is exactly at the max boundary (inclusive, `<=`).
	body := fetchProxyLogsFiltered(t, r, "latencyMin=2000&latencyMax=3000")
	if body.Total != 2 {
		t.Fatalf("latency range total=%d, want 2", body.Total)
	}
	if len(body.Items) != 2 {
		t.Fatalf("latency range items=%d, want 2", len(body.Items))
	}
}

func TestStats_SQLiteProxyLogsClientFilter(t *testing.T) {
	db, r := setupStatsSQLiteTest(t)
	seedProxyLogsForFilterTests(t, db)

	// client=openai-node matches rows A + C; row B (anthropic-sdk) excluded.
	body := fetchProxyLogsFiltered(t, r, "client=openai-node")
	if body.Total != 2 {
		t.Fatalf("client=openai-node total=%d, want 2", body.Total)
	}
	if len(body.Items) != 2 {
		t.Fatalf("client=openai-node items=%d, want 2", len(body.Items))
	}
	if body.Summary.TotalCount != 2 {
		t.Fatalf("client=openai-node summary.totalCount=%d, want 2", body.Summary.TotalCount)
	}
	got := modelSet(body.Items)
	if _, ok := got["fast-B"]; ok {
		t.Fatalf("client=openai-node should NOT include fast-B: %v", got)
	}
}

func TestStats_SQLiteProxyLogsFromToFilters(t *testing.T) {
	db, r := setupStatsSQLiteTest(t)
	_, t2, t3 := seedProxyLogsForFilterTests(t, db)

	// from=t2&to=t3 → rows A (t3) + B (t2) match; row C (t1) is before the
	// window. RFC3339 strings compare lexicographically, so `>=` / `<=` are
	// chronologically correct (the column stores time.Now().UTC().Format
	// (RFC3339) per handler/proxy/proxy_log.go).
	body := fetchProxyLogsFiltered(t, r, "from="+t2+"&to="+t3)
	if body.Total != 2 {
		t.Fatalf("from/to total=%d, want 2", body.Total)
	}
	if len(body.Items) != 2 {
		t.Fatalf("from/to items=%d, want 2", len(body.Items))
	}
	got := modelSet(body.Items)
	if _, ok := got["slow-C"]; ok {
		t.Fatalf("from/to should NOT include slow-C (before window): %v", got)
	}

	// Lower bound only: from=t2 → rows A (t3) + B (t2), excludes C (t1).
	bodyFrom := fetchProxyLogsFiltered(t, r, "from="+t2)
	if bodyFrom.Total != 2 {
		t.Fatalf("from=t2 total=%d, want 2", bodyFrom.Total)
	}

	// Upper bound only: to=t2 → rows B (t2) + C (t1), excludes A (t3).
	bodyTo := fetchProxyLogsFiltered(t, r, "to="+t2)
	if bodyTo.Total != 2 {
		t.Fatalf("to=t2 total=%d, want 2", bodyTo.Total)
	}
	gotTo := modelSet(bodyTo.Items)
	if _, ok := gotTo["slow-A"]; ok {
		t.Fatalf("to=t2 should NOT include slow-A (after window): %v", gotTo)
	}
}

func TestStats_SQLiteProxyLogsCombinedFilters(t *testing.T) {
	db, r := setupStatsSQLiteTest(t)
	seedProxyLogsForFilterTests(t, db)

	// client=openai-node AND latencyMin=2000 → rows A + C (both openai-node
	// AND latency >= 2000). Row B is both wrong client and too fast.
	body := fetchProxyLogsFiltered(t, r, "client=openai-node&latencyMin=2000")
	if body.Total != 2 {
		t.Fatalf("combined total=%d, want 2", body.Total)
	}
	if len(body.Items) != 2 {
		t.Fatalf("combined items=%d, want 2", len(body.Items))
	}
}
