package admin

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// seedDashboardFailOpenData inserts one active site + one active account +
// one proxy log so the dashboard's COUNT/SUM aggregation queries return
// non-zero values. Mirrors the seeding in the existing
// TestStats_SQLiteDashboardProxy24hTokens test.
func seedDashboardFailOpenData(t *testing.T, db *store.DB) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO sites (name, url, platform, status, created_at, updated_at)
		VALUES ('dash-fo-site', 'https://dash-fo.example.test', 'openai', 'active', ?, ?)`, now, now); err != nil {
		t.Fatalf("insert site: %v", err)
	}
	var siteID int64
	if err := db.Get(&siteID, "SELECT id FROM sites WHERE name = 'dash-fo-site'"); err != nil {
		t.Fatalf("load site id: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO accounts
		(site_id, username, access_token, status, balance, balance_used, checkin_enabled, created_at, updated_at)
		VALUES (?, 'dash-fo-user', 'sk-dash-fo', 'active', 100, 5, TRUE, ?, ?)`, siteID, now, now); err != nil {
		t.Fatalf("insert account: %v", err)
	}
	var accountID int64
	if err := db.Get(&accountID, "SELECT id FROM accounts WHERE username = 'dash-fo-user'"); err != nil {
		t.Fatalf("load account id: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO proxy_logs
		(account_id, model_requested, model_actual, status, total_tokens, estimated_cost, created_at)
		VALUES (?, 'gpt-fo', 'gpt-fo', 'success', 42, 0.1, ?)`, accountID, now); err != nil {
		t.Fatalf("insert proxy log: %v", err)
	}
}

// TestDashboardFailOpenOnCheckinLogsMissing verifies that dropping the
// checkin_logs table (which makes CollectDailySummaryMetrics fail) does NOT
// 500 the dashboard. Instead, the response should be 200 with
// dashboardStatus=partial, the failed metric nulled, and all other metrics
// (siteCount, accountCount, proxy24h, performance, siteAvailability) intact.
func TestDashboardFailOpenOnCheckinLogsMissing(t *testing.T) {
	db, r := setupStatsSQLiteTest(t)
	seedDashboardFailOpenData(t, db)

	// Drop checkin_logs so CollectDailySummaryMetrics returns an error
	// early ("load local-day checkin logs"). Every other dashboard query
	// (sites, accounts, proxy_logs, performance, availability) is
	// unaffected because none of them touch checkin_logs.
	if _, err := db.Exec("DROP TABLE checkin_logs"); err != nil {
		t.Fatalf("drop checkin_logs: %v", err)
	}

	resp := doGet(t, r, "/api/stats/dashboard?view=full&force=1")
	if resp.Code != http.StatusOK {
		t.Fatalf("dashboard status = %d, want %d (partial must not 500). Body: %s",
			resp.Code, http.StatusOK, resp.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal dashboard response: %v", err)
	}

	// dashboardStatus must report partial.
	dashStatus, ok := body["dashboardStatus"].(map[string]any)
	if !ok {
		t.Fatalf("dashboardStatus missing or wrong type: %v", body["dashboardStatus"])
	}
	if dashStatus["status"] != "partial" {
		t.Fatalf("dashboardStatus.status = %v, want \"partial\"", dashStatus["status"])
	}

	// todayMetricStatus must be in the failed list.
	failed, _ := dashStatus["failed"].([]any)
	foundFailed := false
	for _, f := range failed {
		if f == "todayMetricStatus" {
			foundFailed = true
			break
		}
	}
	if !foundFailed {
		t.Fatalf("todayMetricStatus not in failed list: %v", failed)
	}

	// todayMetricStatus should be a partial marker, not a full metrics block.
	tms, ok := body["todayMetricStatus"].(map[string]any)
	if !ok {
		t.Fatalf("todayMetricStatus missing or wrong type: %v", body["todayMetricStatus"])
	}
	if tms["status"] != "partial" {
		t.Fatalf("todayMetricStatus.status = %v, want \"partial\"", tms["status"])
	}

	// todaySpend/todayReward/todayCheckin should be null (degraded).
	if body["todaySpend"] != nil {
		t.Fatalf("todaySpend = %v, want nil (degraded)", body["todaySpend"])
	}
	if body["todayReward"] != nil {
		t.Fatalf("todayReward = %v, want nil (degraded)", body["todayReward"])
	}
	if body["todayCheckin"] != nil {
		t.Fatalf("todayCheckin = %v, want nil (degraded)", body["todayCheckin"])
	}

	// Other metrics must still be present with real values.
	if body["siteCount"] == nil {
		t.Fatalf("siteCount is nil; should survive checkin_logs failure")
	}
	if body["accountCount"] == nil {
		t.Fatalf("accountCount is nil; should survive checkin_logs failure")
	}
	if body["proxy24h"] == nil {
		t.Fatalf("proxy24h is nil; should survive checkin_logs failure")
	}
	if body["performance"] == nil {
		t.Fatalf("performance is nil; should survive checkin_logs failure")
	}
	if body["siteAvailability"] == nil {
		t.Fatalf("siteAvailability is nil; should survive checkin_logs failure")
	}
}

// TestDashboardCompleteWhenAllQueriesSucceed verifies the happy path: with
// all tables intact and data seeded, dashboardStatus should be "complete"
// and no fields should be null.
func TestDashboardCompleteWhenAllQueriesSucceed(t *testing.T) {
	db, r := setupStatsSQLiteTest(t)
	seedDashboardFailOpenData(t, db)

	resp := doGet(t, r, "/api/stats/dashboard?view=full&force=1")
	if resp.Code != http.StatusOK {
		t.Fatalf("dashboard status = %d, want %d. Body: %s",
			resp.Code, http.StatusOK, resp.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal dashboard response: %v", err)
	}

	dashStatus, ok := body["dashboardStatus"].(map[string]any)
	if !ok {
		t.Fatalf("dashboardStatus missing or wrong type: %v", body["dashboardStatus"])
	}
	if dashStatus["status"] != "complete" {
		t.Fatalf("dashboardStatus.status = %v, want \"complete\"", dashStatus["status"])
	}

	failed, _ := dashStatus["failed"].([]any)
	if len(failed) != 0 {
		t.Fatalf("expected no failed metrics, got: %v", failed)
	}

	// Spot-check that key fields are non-null.
	for _, key := range []string{
		"siteCount", "accountCount", "activeAccounts",
		"totalBalance", "totalUsed", "proxy24h",
		"todaySpend", "todayReward", "todayCheckin",
		"todayMetricStatus", "performance",
		"totalTokens", "totalCost", "siteAvailability",
	} {
		if body[key] == nil {
			t.Fatalf("field %q is nil on the happy path", key)
		}
	}
}

// TestDashboardSummaryViewFailOpen verifies the summary-only view also
// degrades gracefully when one query fails.
func TestDashboardSummaryViewFailOpen(t *testing.T) {
	db, r := setupStatsSQLiteTest(t)
	seedDashboardFailOpenData(t, db)

	if _, err := db.Exec("DROP TABLE checkin_logs"); err != nil {
		t.Fatalf("drop checkin_logs: %v", err)
	}

	resp := doGet(t, r, "/api/stats/dashboard?view=summary&force=1")
	if resp.Code != http.StatusOK {
		t.Fatalf("dashboard status = %d, want %d", resp.Code, http.StatusOK)
	}

	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal dashboard response: %v", err)
	}

	dashStatus, ok := body["dashboardStatus"].(map[string]any)
	if !ok {
		t.Fatalf("dashboardStatus missing: %v", body["dashboardStatus"])
	}
	if dashStatus["status"] != "partial" {
		t.Fatalf("dashboardStatus.status = %v, want \"partial\"", dashStatus["status"])
	}

	// Summary view should not have insights-only fields.
	if _, exists := body["siteAvailability"]; exists {
		t.Fatalf("siteAvailability should not exist in summary view")
	}
}
