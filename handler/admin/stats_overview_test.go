package admin

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
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

func insertStatsProxyLogAttempt(t *testing.T, db *store.DB, model, status, requestID string, retryCount int, createdAt string) {
	t.Helper()
	var nullableRequestID any
	if requestID != "" {
		nullableRequestID = requestID
	}
	_, err := db.Exec(
		`INSERT INTO proxy_logs (model_requested, model_actual, status, request_id, retry_count, total_tokens, estimated_cost, latency_ms, created_at)
		 VALUES (?, ?, ?, ?, ?, 10, 1, 100, ?)`,
		model, model, status, nullableRequestID, retryCount, createdAt,
	)
	if err != nil {
		t.Fatalf("insert proxy log attempt: %v", err)
	}
}

func TestStats_OverviewRequestLevelMetrics(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*testing.T) (*store.DB, chi.Router)
	}{{"sqlite", setupStatsSQLiteTest}, {"postgres", setupStatsPostgresTest}} {
		t.Run(tc.name, func(t *testing.T) {
			db, r := tc.setup(t)
			now := time.Now().UTC().Format(time.RFC3339)
			model := "request-overview-" + tc.name
			t.Cleanup(func() {
				_, _ = db.Exec("DELETE FROM proxy_logs WHERE model_requested = ?", model)
			})

			// Two failures followed by success is one successful client request.
			insertStatsProxyLogAttempt(t, db, model, "failed", "req-a", 0, now)
			insertStatsProxyLogAttempt(t, db, model, "failed", "req-a", 1, now)
			insertStatsProxyLogAttempt(t, db, model, "success", "req-a", 2, now)
			// Two failures with no success is one failed client request.
			insertStatsProxyLogAttempt(t, db, model, "failed", "req-b", 0, now)
			insertStatsProxyLogAttempt(t, db, model, "failed", "req-b", 1, now)
			// A single-attempt request remains one request.
			insertStatsProxyLogAttempt(t, db, model, "success", "req-c", 0, now)
			// Legacy rows cannot be correlated and must not enter the request denominator.
			insertStatsProxyLogAttempt(t, db, model, "success", "", 0, now)
			insertStatsProxyLogAttempt(t, db, model, "failed", "", 0, now)

			response := doGet(t, r, "/api/stats/overview?period=all")
			if response.Code != 200 {
				t.Fatalf("%s: %d %s", tc.name, response.Code, response.Body.String())
			}
			var result map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			requestSummary := result["requestSummary"].(map[string]any)
			if requestSummary["totalCount"] != float64(3) ||
				requestSummary["successCount"] != float64(2) ||
				requestSummary["failedCount"] != float64(1) {
				t.Fatalf("requestSummary = %#v", requestSummary)
			}
			if requestSummary["legacyAttemptCount"] != float64(2) ||
				requestSummary["partial"] != true {
				t.Fatalf("request completeness = %#v", requestSummary)
			}
			reasons, ok := requestSummary["partialReasons"].([]any)
			if !ok || len(reasons) != 1 || reasons[0] != "requestIdMissing" {
				t.Fatalf("partialReasons = %#v", requestSummary["partialReasons"])
			}

			attemptSummary := result["summary"].(map[string]any)
			if attemptSummary["totalCount"] != float64(8) {
				t.Fatalf("attempt summary = %#v", attemptSummary)
			}

			points, ok := result["requestPoints"].([]any)
			if !ok || len(points) != 1 {
				t.Fatalf("requestPoints = %#v", result["requestPoints"])
			}
			point := points[0].(map[string]any)
			if point["requests"] != float64(3) ||
				point["successCount"] != float64(2) ||
				point["successRate"] != float64(2.0/3.0) {
				t.Fatalf("request point = %#v", point)
			}
		})
	}
}

func bothStatsDatabases(t *testing.T, check func(*testing.T, *store.DB, chi.Router)) {
	t.Helper()
	for _, tc := range []struct {
		name  string
		setup func(*testing.T) (*store.DB, chi.Router)
	}{{"sqlite", setupStatsSQLiteTest}, {"postgres", setupStatsPostgresTest}} {
		t.Run(tc.name, func(t *testing.T) {
			db, r := tc.setup(t)
			check(t, db, r)
		})
	}
}

func TestOverviewObservedHours(t *testing.T) {
	bothStatsDatabases(t, func(t *testing.T, db *store.DB, r chi.Router) {
		if db.Dialect == store.DialectPostgres {
			db.SetMaxOpenConns(1)
			mustExec(t, db, `SET TIME ZONE 'Asia/Hong_Kong'`)
		}
		mustExec(t, db, `INSERT INTO sites (id, name, url, platform, status) VALUES (1,'busy','https://busy.example','openai','active'), (2,'idle','https://idle.example','openai','active')`)
		mustExec(t, db, `INSERT INTO accounts (id, site_id, access_token) VALUES (1,1,'fixture')`)
		get := func() map[string]any { return catalogCall(t, r, "GET", "/api/stats/overview?period=7d", nil, 200) }
		empty := get()["summary"].(map[string]any)
		if v, ok := empty["averageFirstByteMs"]; !ok || v != nil {
			t.Errorf("empty first byte = %#v", empty)
		}
		hour := time.Now().UTC().Truncate(time.Hour).Add(-time.Hour)
		add := func(ago int, account, firstByte, latency any, status string) {
			mustExec(t, db, `INSERT INTO proxy_logs (account_id, status, first_byte_latency_ms, latency_ms, created_at) VALUES (?,?,?,?,?)`, account, status, firstByte, latency, hour.Add(-time.Duration(ago)*time.Hour).Format(time.RFC3339))
		}
		for i := 0; i < 25; i++ {
			add(2*i, 1, nil, nil, "success")
		}
		add(0, 1, 12, 80, "failed")
		add(0, 1, 0, 40, "success")
		add(1, nil, 24, nil, "failed")
		add(200, 1, 400, 100, "failed")
		out := get()
		summary := out["summary"].(map[string]any)
		sites := out["siteAvailability"].([]any)
		busy, idle := sites[0].(map[string]any), sites[1].(map[string]any)
		if summary["averageFirstByteMs"] != float64(18) || busy["averageFirstByteMs"] != float64(12) {
			t.Errorf("first byte summary=%v busy=%v", summary, busy)
		}
		if v, ok := idle["averageFirstByteMs"]; !ok || v != nil || !reflect.DeepEqual(idle["recentSamples"], []any{}) {
			t.Errorf("idle = %v", idle)
		}
		samples, ok := busy["recentSamples"].([]any)
		if !ok || len(samples) != 24 {
			t.Fatalf("recentSamples = %v", busy["recentSamples"])
		}
		for i, raw := range samples {
			want := map[string]any{"bucketTs": hour.Add(-time.Duration(2*(23-i)) * time.Hour).Format(time.RFC3339), "total": float64(1), "success": float64(1), "avgLatencyMs": nil}
			if i == 23 {
				want["total"], want["success"], want["avgLatencyMs"] = float64(3), float64(2), float64(60)
			}
			if !reflect.DeepEqual(raw, want) {
				t.Errorf("sample %d=%v want %v", i, raw, want)
			}
		}
	})
}
