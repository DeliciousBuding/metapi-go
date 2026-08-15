package admin

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// ---- proxyLogDetail ----

func TestStats_SQLiteProxyLogDetail_Found(t *testing.T) {
	db, r := setupStatsSQLiteTest(t)
	now := time.Now().UTC().Format(time.RFC3339)

	siteID, accountID := insertSiteAccountForStats(t, db, "detail-site", "detail-user", now)

	res, err := db.Exec(`INSERT INTO proxy_logs (account_id, model_requested, model_actual, status, total_tokens, estimated_cost, latency_ms, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, "gpt-detail", "gpt-detail", "success", 42, 0.123, 250, now)
	if err != nil {
		t.Fatalf("insert proxy log: %v", err)
	}
	logID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("log id: %v", err)
	}

	resp := doGet(t, r, "/api/stats/proxy-logs/"+itoa(logID))
	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got, _ := body["modelRequested"].(string); got != "gpt-detail" {
		t.Fatalf("modelRequested=%v, want gpt-detail", body["modelRequested"])
	}
	if got, _ := body["siteName"].(string); got != "detail-site" {
		t.Fatalf("siteName=%v, want detail-site", body["siteName"])
	}
	if got, _ := body["username"].(string); got != "detail-user" {
		t.Fatalf("username=%v, want detail-user", body["username"])
	}
	if got := int64(body["totalTokens"].(float64)); got != 42 {
		t.Fatalf("totalTokens=%d, want 42", got)
	}
	if got := int64(body["siteId"].(float64)); got != siteID {
		t.Fatalf("siteId=%d, want %d", got, siteID)
	}
}

func TestStats_SQLiteProxyLogDetail_NotFound(t *testing.T) {
	_, r := setupStatsSQLiteTest(t)

	resp := doGet(t, r, "/api/stats/proxy-logs/999999")
	if resp.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s, want 404", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if msg, _ := body["message"].(string); msg != "proxy log not found" {
		t.Fatalf("message=%q, want 'proxy log not found'", msg)
	}
}

func TestStats_SQLiteProxyLogDetail_ParsesBillingDetails(t *testing.T) {
	db, r := setupStatsSQLiteTest(t)
	now := time.Now().UTC().Format(time.RFC3339)

	_, accountID := insertSiteAccountForStats(t, db, "billing-site", "billing-user", now)

	// Insert a proxy log whose billing_details column holds a JSON object as a
	// raw string. The handler must parse it into a structured object in the
	// response rather than returning the opaque string.
	billingDetails := `{"total":"0.05","currency":"USD"}`
	res, err := db.Exec(`INSERT INTO proxy_logs (account_id, model_requested, model_actual, status, billing_details, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		accountID, "gpt-billing", "gpt-billing", "success", billingDetails, now)
	if err != nil {
		t.Fatalf("insert proxy log: %v", err)
	}
	logID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("log id: %v", err)
	}

	resp := doGet(t, r, "/api/stats/proxy-logs/"+itoa(logID))
	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	bd, ok := body["billingDetails"].(map[string]any)
	if !ok {
		t.Fatalf("billingDetails should be a parsed object, got %T: %#v", body["billingDetails"], body["billingDetails"])
	}
	if bd["currency"] != "USD" {
		t.Fatalf("billingDetails.currency=%v, want USD", bd["currency"])
	}
}

func TestStats_SQLiteProxyLogDetail_UnlinkedLog(t *testing.T) {
	db, r := setupStatsSQLiteTest(t)
	now := time.Now().UTC().Format(time.RFC3339)

	// A proxy log with no account_id: the LEFT JOINs yield NULL site/user
	// fields but the log itself is still returned.
	res, err := db.Exec(`INSERT INTO proxy_logs (model_requested, model_actual, status, total_tokens, created_at)
		VALUES (?, ?, ?, ?, ?)`, "gpt-unlinked", "gpt-unlinked", "success", 5, now)
	if err != nil {
		t.Fatalf("insert proxy log: %v", err)
	}
	logID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("log id: %v", err)
	}

	resp := doGet(t, r, "/api/stats/proxy-logs/"+itoa(logID))
	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got, _ := body["modelRequested"].(string); got != "gpt-unlinked" {
		t.Fatalf("modelRequested=%v, want gpt-unlinked", body["modelRequested"])
	}
	// siteName is absent (nil) because the LEFT JOIN found no site.
	if body["siteName"] != nil {
		t.Fatalf("siteName=%v, want nil for unlinked log", body["siteName"])
	}
}

// ---- debugTraces ----

func TestStats_SQLiteDebugTraces_Empty(t *testing.T) {
	_, r := setupStatsSQLiteTest(t)

	resp := doGet(t, r, "/api/stats/proxy-debug/traces")
	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	items, ok := body["items"].([]any)
	if !ok {
		t.Fatalf("items not an array: %#v", body["items"])
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 traces on empty DB, got %d", len(items))
	}
}

func TestStats_SQLiteDebugTraces_WithTraces(t *testing.T) {
	db, r := setupStatsSQLiteTest(t)
	now := time.Now().UTC().Format(time.RFC3339)

	insertDebugTrace(t, db, "/v1/chat/completions", now)
	insertDebugTrace(t, db, "/v1/responses", now)

	resp := doGet(t, r, "/api/stats/proxy-debug/traces")
	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	items, ok := body["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("expected 2 traces, got %#v", body["items"])
	}

	// limit=1 returns only the most recent trace.
	respLimit := doGet(t, r, "/api/stats/proxy-debug/traces?limit=1")
	if respLimit.Code != http.StatusOK {
		t.Fatalf("limit status=%d body=%s", respLimit.Code, respLimit.Body.String())
	}
	var limitBody map[string]any
	if err := json.Unmarshal(respLimit.Body.Bytes(), &limitBody); err != nil {
		t.Fatalf("decode limit: %v", err)
	}
	limitItems, _ := limitBody["items"].([]any)
	if len(limitItems) != 1 {
		t.Fatalf("expected 1 trace with limit=1, got %d", len(limitItems))
	}
}

// ---- debugTraceDetail ----

func TestStats_SQLiteDebugTraceDetail_FoundWithAttempts(t *testing.T) {
	db, r := setupStatsSQLiteTest(t)
	now := time.Now().UTC().Format(time.RFC3339)

	traceID := insertDebugTrace(t, db, "/v1/chat/completions", now)
	insertDebugAttempt(t, db, traceID, 0, now)
	insertDebugAttempt(t, db, traceID, 1, now)

	resp := doGet(t, r, "/api/stats/proxy-debug/traces/"+itoa(traceID))
	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got, _ := body["downstreamPath"].(string); got != "/v1/chat/completions" {
		t.Fatalf("downstreamPath=%v, want /v1/chat/completions", body["downstreamPath"])
	}
	attempts, ok := body["attempts"].([]any)
	if !ok || len(attempts) != 2 {
		t.Fatalf("expected 2 attempts, got %#v", body["attempts"])
	}
}

func TestStats_SQLiteDebugTraceDetail_NotFound(t *testing.T) {
	_, r := setupStatsSQLiteTest(t)

	resp := doGet(t, r, "/api/stats/proxy-debug/traces/999999")
	if resp.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s, want 404", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if msg, _ := body["message"].(string); msg != "proxy debug trace not found" {
		t.Fatalf("message=%q, want 'proxy debug trace not found'", msg)
	}
}

func TestStats_SQLiteDebugTraceDetail_NoAttempts(t *testing.T) {
	db, r := setupStatsSQLiteTest(t)
	now := time.Now().UTC().Format(time.RFC3339)

	traceID := insertDebugTrace(t, db, "/v1/responses", now)

	resp := doGet(t, r, "/api/stats/proxy-debug/traces/"+itoa(traceID))
	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	attempts, ok := body["attempts"].([]any)
	if !ok || len(attempts) != 0 {
		t.Fatalf("expected 0 attempts, got %#v", body["attempts"])
	}
}

// ---- siteDistribution ----

func TestStats_SQLiteSiteDistribution(t *testing.T) {
	db, r := setupStatsSQLiteTest(t)
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)
	today := now.Format("2006-01-02")

	// site-a: one account (balance 100) + recorded spend (0.5) today.
	siteIDA, _ := insertSiteAccountForStatsWithBalance(t, db, "dist-a", "dist-user-a", nowStr, 100.0)
	_, err := db.Exec(`INSERT INTO site_day_usage (local_day, site_id, total_calls, success_calls, failed_calls, total_tokens, total_summary_spend, total_site_spend, total_latency_ms, latency_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		today, siteIDA, 10, 8, 2, 200, 0.5, 0.0, 1000, 10, nowStr, nowStr)
	if err != nil {
		t.Fatalf("insert site_day_usage: %v", err)
	}

	// site-b: no accounts, no usage.
	_, err = db.Exec(`INSERT INTO sites (name, url, platform, status, created_at, updated_at)
		VALUES (?, ?, ?, 'active', ?, ?)`, "dist-b", "https://dist-b.example.test", "openai", nowStr, nowStr)
	if err != nil {
		t.Fatalf("insert site-b: %v", err)
	}

	resp := doGet(t, r, "/api/stats/site-distribution?days=7")
	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	distribution, ok := body["distribution"].([]any)
	if !ok || len(distribution) != 2 {
		t.Fatalf("expected 2 distribution entries, got %#v", body["distribution"])
	}

	// Ordered by total_spend DESC: dist-a (0.5) first, dist-b (0) second.
	first := distribution[0].(map[string]any)
	if first["siteName"] != "dist-a" {
		t.Fatalf("first entry siteName=%v, want dist-a", first["siteName"])
	}
	if got := int(first["accountCount"].(float64)); got != 1 {
		t.Fatalf("dist-a accountCount=%d, want 1", got)
	}
	if got := first["totalBalance"].(float64); got != 100 {
		t.Fatalf("dist-a totalBalance=%v, want 100", got)
	}
	if got := first["totalSpend"].(float64); got != 0.5 {
		t.Fatalf("dist-a totalSpend=%v, want 0.5", got)
	}

	second := distribution[1].(map[string]any)
	if second["siteName"] != "dist-b" {
		t.Fatalf("second entry siteName=%v, want dist-b", second["siteName"])
	}
	if got := int(second["accountCount"].(float64)); got != 0 {
		t.Fatalf("dist-b accountCount=%d, want 0", got)
	}
	if got := second["totalSpend"].(float64); got != 0 {
		t.Fatalf("dist-b totalSpend=%v, want 0", got)
	}
}

// ---- modelProbe ----

func TestStats_SQLiteModelProbe_SchedulerNotRunning(t *testing.T) {
	_, r := setupStatsSQLiteTest(t)

	resp := doPostJSON(t, r, "/api/models/probe", map[string]any{})
	// In unit tests the global model-probe scheduler is never started, so the
	// handler must report 503 Service Unavailable rather than queueing a job.
	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("modelProbe status=%d body=%s, want 503", resp.Code, resp.Body.String())
	}
}

// ---- helpers ----

// insertSiteAccountForStats inserts a site and a linked account, returning
// (siteID, accountID). Used by proxy-log detail tests that need the JOINs.
func insertSiteAccountForStats(t *testing.T, db *store.DB, siteName, username, now string) (int64, int64) {
	t.Helper()
	return insertSiteAccountForStatsWithBalance(t, db, siteName, username, now, 0)
}

func insertSiteAccountForStatsWithBalance(t *testing.T, db *store.DB, siteName, username, now string, balance float64) (int64, int64) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO sites (name, url, platform, status, created_at, updated_at)
		VALUES (?, ?, ?, 'active', ?, ?)`, siteName, "https://"+siteName+".example.test", "openai", now, now)
	if err != nil {
		t.Fatalf("insert site %s: %v", siteName, err)
	}
	var siteID int64
	if err := db.Get(&siteID, "SELECT id FROM sites WHERE name = ?", siteName); err != nil {
		t.Fatalf("site id %s: %v", siteName, err)
	}
	_, err = db.Exec(`INSERT INTO accounts (site_id, username, access_token, status, balance, checkin_enabled, created_at, updated_at)
		VALUES (?, ?, ?, 'active', ?, FALSE, ?, ?)`, siteID, username, "sk-"+siteName, balance, now, now)
	if err != nil {
		t.Fatalf("insert account %s: %v", username, err)
	}
	var accountID int64
	if err := db.Get(&accountID, "SELECT id FROM accounts WHERE username = ?", username); err != nil {
		t.Fatalf("account id %s: %v", username, err)
	}
	return siteID, accountID
}

// insertDebugTrace inserts a minimal proxy_debug_traces row and returns its id.
func insertDebugTrace(t *testing.T, db *store.DB, downstreamPath, now string) int64 {
	t.Helper()
	res, err := db.Exec(`INSERT INTO proxy_debug_traces (downstream_path, created_at, updated_at)
		VALUES (?, ?, ?)`, downstreamPath, now, now)
	if err != nil {
		t.Fatalf("insert debug trace: %v", err)
	}
	traceID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("trace id: %v", err)
	}
	return traceID
}

// insertDebugAttempt inserts a minimal proxy_debug_attempts row linked to a trace.
func insertDebugAttempt(t *testing.T, db *store.DB, traceID, attemptIndex int64, now string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO proxy_debug_attempts (trace_id, attempt_index, endpoint, request_path, target_url, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		traceID, attemptIndex, "https://api.openai.com", "/v1/chat/completions", "https://api.openai.com/v1/chat/completions", now)
	if err != nil {
		t.Fatalf("insert debug attempt: %v", err)
	}
}
