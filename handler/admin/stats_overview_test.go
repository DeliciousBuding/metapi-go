package admin

import (
	"encoding/json"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
	"testing"
	"time"
)

func TestStats_OverviewWindows(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*testing.T) (*store.DB, chi.Router)
	}{{"sqlite", setupStatsSQLiteTest}, {"postgres", setupStatsPostgresTest}} {
		t.Run(tc.name, func(t *testing.T) {
			db, r := tc.setup(t)
			now := time.Now().UTC()
			// Unassigned failures must be included, even without active upstreams.
			for i, days := range []int{0, 3, 20, 120} {
				status := "success"
				if i == 0 {
					status = "failed"
				}
				insertStatsProxyLog(t, db, "window-model", 1, 100, status, now.AddDate(0, 0, -days).Format(time.RFC3339))
			}
			// Future records are outside the frozen upper bound.
			insertStatsProxyLog(t, db, "future", 99, 100, "success", now.Add(time.Hour).Format(time.RFC3339))
			for _, rangeTest := range []struct {
				period string
				count  float64
			}{{"24h", 1}, {"7d", 2}, {"30d", 3}, {"all", 4}} {
				response := doGet(t, r, "/api/stats/overview?period="+rangeTest.period)
				if response.Code != 200 {
					t.Fatalf("%s: %d %s", rangeTest.period, response.Code, response.Body.String())
				}
				var result map[string]any
				if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				summary := result["summary"].(map[string]any)
				if summary["totalCount"] != rangeTest.count || summary["totalCost"] != rangeTest.count || summary["failedCount"] != float64(1) {
					t.Fatalf("%s summary=%v", rangeTest.period, summary)
				}
				models := result["models"].([]any)
				if len(models) != 1 || models[0].(map[string]any)["calls"] != rangeTest.count {
					t.Fatalf("model scope differs: %v", models)
				}
				var requests float64
				for _, point := range result["points"].([]any) {
					requests += point.(map[string]any)["requests"].(float64)
				}
				if requests != rangeTest.count {
					t.Fatalf("trend scope differs: %v", requests)
				}
				window := result["window"].(map[string]any)
				_, hasFrom := window["from"]
				if hasFrom == (rangeTest.period == "all") {
					t.Fatalf("invalid from bound: %v", window)
				}
			}
			if response := doGet(t, r, "/api/stats/overview?period=invalid"); response.Code != 400 {
				t.Fatalf("invalid period accepted: %d", response.Code)
			}
		})
	}
}

func TestStats_OverviewUpstreamAndOther(t *testing.T) {
	db, r := setupStatsSQLiteTest(t)
	seedDashboardFailOpenData(t, db)
	now := time.Now().UTC().Format(time.RFC3339)
	for _, model := range []string{"a", "b", "c", "d", "e", "f"} {
		insertStatsProxyLog(t, db, model, 1, 50, "success", now)
	}
	response := doGet(t, r, "/api/stats/overview?period=all")
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	sites := result["siteAvailability"].([]any)
	if len(sites) != 1 || sites[0].(map[string]any)["totalRequests"] != float64(1) {
		t.Fatalf("upstream aggregation: %v", sites)
	}
	models := result["models"].([]any)
	if len(models) != 6 || models[5].(map[string]any)["model"] != "other" || models[5].(map[string]any)["calls"] != float64(2) {
		t.Fatalf("Other bucket lost: %v", models)
	}
}
