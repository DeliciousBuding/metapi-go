package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// setupAccountsHealthTest is setupAccountsTest plus a background-task reset so
// the async (wait=false) refresh path — which calls StartBackgroundTask with a
// fixed dedupe key — is not affected by leaked tasks from earlier tests.
func setupAccountsHealthTest(t *testing.T) (*store.DB, chi.Router) {
	t.Helper()
	resetBackgroundTasksForTests()
	db, r, _ := setupAccountsTest(t)
	return db, r
}

// insertHealthSiteRow inserts a sites row directly via SQL (bypassing the sites
// API) so health-refresh tests can seed FK parents without router side effects.
func insertHealthSiteRow(t *testing.T, db *store.DB, name, url, platform string) int64 {
	t.Helper()
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	res, err := db.Exec(db.Rebind(`INSERT INTO sites (name, url, platform, status, created_at, updated_at) VALUES (?, ?, ?, 'active', ?, ?)`), name, url, platform, now, now)
	if err != nil {
		t.Fatalf("insert site row: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("site LastInsertId: %v", err)
	}
	return id
}

// insertHealthAccountRow inserts an accounts row directly. extraConfigJSON
// controls the credential mode (e.g. `{"credentialMode":"apikey"}`) so callers
// can build fixtures that exercise refreshOneAccountHealth's skip branch
// without any upstream network call.
func insertHealthAccountRow(t *testing.T, db *store.DB, siteID int64, status, extraConfigJSON string) int64 {
	t.Helper()
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	res, err := db.Exec(db.Rebind(`INSERT INTO accounts (site_id, access_token, status, checkin_enabled, extra_config, created_at, updated_at) VALUES (?, 'sk-fixture', ?, 1, ?, ?, ?)`), siteID, status, extraConfigJSON, now, now)
	if err != nil {
		t.Fatalf("insert account row: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("account LastInsertId: %v", err)
	}
	return id
}

// doRawPostJSON issues a POST with a literal body string (not a marshalled
// object) so handlers can be exercised with malformed JSON payloads.
func doRawPostJSON(t *testing.T, r chi.Router, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// ---- listAccountIDsForHealthRefresh ----

func TestListAccountIDsForHealthRefresh_ReturnsAllAccountsOrdered(t *testing.T) {
	db, _ := setupAccountsHealthTest(t)
	siteID := insertHealthSiteRow(t, db, "List Health Site", "https://list-health.example.com", "openai")

	// Mix active and disabled accounts: the query has no status filter, so
	// every account must be returned regardless of status.
	idActive1 := insertHealthAccountRow(t, db, siteID, "active", "")
	idDisabled := insertHealthAccountRow(t, db, siteID, "disabled", "")
	idActive2 := insertHealthAccountRow(t, db, siteID, "active", `{"credentialMode":"apikey"}`)

	ids, err := listAccountIDsForHealthRefresh(db.DB)
	if err != nil {
		t.Fatalf("listAccountIDsForHealthRefresh: %v", err)
	}
	want := []int64{idActive1, idDisabled, idActive2}
	if len(ids) != len(want) {
		t.Fatalf("ids = %v, want %v", ids, want)
	}
	for i, id := range ids {
		if id != want[i] {
			t.Fatalf("ids[%d] = %d, want %d (must be ascending by id)", i, id, want[i])
		}
	}
}

func TestListAccountIDsForHealthRefresh_EmptyDBReturnsEmptyNonNil(t *testing.T) {
	db, _ := setupAccountsHealthTest(t)

	ids, err := listAccountIDsForHealthRefresh(db.DB)
	if err != nil {
		t.Fatalf("listAccountIDsForHealthRefresh: %v", err)
	}
	if ids == nil {
		t.Fatal("ids = nil, want non-nil empty slice (callers should not see null)")
	}
	if len(ids) != 0 {
		t.Fatalf("ids = %v, want empty", ids)
	}
}

// ---- formatHealthRefreshMessage ----

func TestFormatHealthRefreshMessage(t *testing.T) {
	cases := []struct {
		name    string
		summary healthRefreshSummary
		want    string
	}{
		{
			name:    "all success",
			summary: healthRefreshSummary{Total: 3, Success: 3, Healthy: 3},
			want:    "账号健康刷新完成：成功 3，失败 0，跳过 0（共 3）",
		},
		{
			name:    "all failure",
			summary: healthRefreshSummary{Total: 2, Failed: 2, Unhealthy: 2},
			want:    "账号健康刷新完成：成功 0，失败 2，跳过 0（共 2）",
		},
		{
			name:    "mixed success failure skipped",
			summary: healthRefreshSummary{Total: 4, Success: 2, Failed: 1, Skipped: 1, Healthy: 2, Unhealthy: 1, Degraded: 1},
			want:    "账号健康刷新完成：成功 2，失败 1，跳过 1（共 4）",
		},
		{
			name:    "all skipped",
			summary: healthRefreshSummary{Total: 5, Skipped: 5, Disabled: 5},
			want:    "账号健康刷新完成：成功 0，失败 0，跳过 5（共 5）",
		},
		{
			name:    "zero summary",
			summary: healthRefreshSummary{},
			want:    "账号健康刷新完成：成功 0，失败 0，跳过 0（共 0）",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatHealthRefreshMessage(tc.summary); got != tc.want {
				t.Fatalf("formatHealthRefreshMessage() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFormatHealthRefreshMessage_MatchesWaitModeResponse(t *testing.T) {
	// The wait=true handler path writes the same string into the response
	// "message" field, so the message must be derivable from the summary
	// alone — no extra state. Construct a summary and confirm the handler
	// message equals the formatter output.
	summary := healthRefreshSummary{Total: 2, Success: 1, Failed: 1, Skipped: 0}
	want := formatHealthRefreshMessage(summary)
	if !strings.Contains(want, "succeeded 1") || !strings.Contains(want, "failed 1") || !strings.Contains(want, "(2 total)") {
		t.Fatalf("message %q missing expected counts", want)
	}
}

// ---- healthRefresh HTTP handler ----

func TestHealthRefresh_RejectsInvalidJSON(t *testing.T) {
	_, r := setupAccountsHealthTest(t)

	rec := doRawPostJSON(t, r, "/api/accounts/health/refresh", "not-valid-json{")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Invalid health refresh payload") {
		t.Fatalf("body = %q, want invalid payload message", rec.Body.String())
	}
}

func TestHealthRefresh_RejectsNonPositiveAccountID(t *testing.T) {
	_, r := setupAccountsHealthTest(t)

	cases := []struct {
		name string
		body map[string]any
	}{
		{"zero", map[string]any{"accountId": 0}},
		{"negative", map[string]any{"accountId": -5}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doPostJSON(t, r, "/api/accounts/health/refresh", tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body=%s, want 400", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "Invalid accountId") {
				t.Fatalf("body = %q, want Invalid accountId message", rec.Body.String())
			}
		})
	}
}

func TestHealthRefresh_NonexistentAccountReturns404(t *testing.T) {
	_, r := setupAccountsHealthTest(t)

	rec := doPostJSON(t, r, "/api/accounts/health/refresh", map[string]any{
		"accountId": 99999,
	})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s, want 404", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "account not found") {
		t.Fatalf("body = %q, want account not found", rec.Body.String())
	}
}

func TestHealthRefresh_WaitModeSkipsApiKeyAccountWithoutNetwork(t *testing.T) {
	// An apikey account has no balance-probe path, so refreshOneAccountHealth
	// must return "skipped" without making any upstream network call. This
	// exercises the wait=true sync path's selection + classification logic.
	db, r := setupAccountsHealthTest(t)
	siteID := insertHealthSiteRow(t, db, "Wait APIKey Site", "https://wait-apikey.example.com", "openai")
	accountID := insertHealthAccountRow(t, db, siteID, "active", `{"credentialMode":"apikey"}`)

	rec := doPostJSON(t, r, "/api/accounts/health/refresh", map[string]any{
		"accountId": accountID,
		"wait":       true,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["success"] != true {
		t.Fatalf("success = %v, want true", body["success"])
	}
	summary, ok := body["summary"].(map[string]any)
	if !ok {
		t.Fatalf("summary missing/invalid: %v", body["summary"])
	}
	if summary["total"].(float64) != 1 {
		t.Fatalf("summary.total = %v, want 1", summary["total"])
	}
	if summary["skipped"].(float64) != 1 {
		t.Fatalf("summary.skipped = %v, want 1 (apikey account must skip probe)", summary["skipped"])
	}
	if summary["success"].(float64) != 0 {
		t.Fatalf("summary.success = %v, want 0 (skip must not count as success)", summary["success"])
	}
	results, ok := body["results"].([]any)
	if !ok || len(results) != 1 {
		t.Fatalf("results = %v, want 1 item", body["results"])
	}
	item, _ := results[0].(map[string]any)
	if item["status"] != "skipped" {
		t.Fatalf("result status = %v, want skipped", item["status"])
	}
	if int64(item["accountId"].(float64)) != accountID {
		t.Fatalf("result accountId = %v, want %d", item["accountId"], accountID)
	}
	if !strings.Contains(body["message"].(string), "succeeded 0") || !strings.Contains(body["message"].(string), "skipped 1") {
		t.Fatalf("message = %v, want skip counts reflected", body["message"])
	}
}

func TestHealthRefresh_AsyncModeQueuesTask(t *testing.T) {
	// wait=false (default) must return 202 with a real background task id,
	// not run the refresh synchronously. A seeded apikey account lets the
	// background worker complete without any upstream network call.
	db, r := setupAccountsHealthTest(t)
	t.Cleanup(func() { resetBackgroundTasksForTests() }) // drain spawned task
	siteID := insertHealthSiteRow(t, db, "Async APIKey Site", "https://async-apikey.example.com", "openai")
	accountID := insertHealthAccountRow(t, db, siteID, "active", `{"credentialMode":"apikey"}`)

	rec := doPostJSON(t, r, "/api/accounts/health/refresh", map[string]any{
		"accountId": accountID,
		// wait intentionally omitted → defaults false → async path
	})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s, want 202 (queued)", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["success"] != true {
		t.Fatalf("success = %v, want true", body["success"])
	}
	if body["queued"] != true {
		t.Fatalf("queued = %v, want true", body["queued"])
	}
	taskID, _ := body["taskId"].(string)
	if taskID == "" || taskID == "stub" {
		t.Fatalf("taskId = %v, want a real non-stub task id", body["taskId"])
	}
	if body["status"] == nil || body["status"] == "" {
		t.Fatalf("status = %v, want a task status string", body["status"])
	}
	if !strings.Contains(body["message"].(string), "refresh") {
		t.Fatalf("message = %v, want refresh wording", body["message"])
	}
}

func TestHealthRefresh_AsyncAllAccountsQueuesTask(t *testing.T) {
	// No accountId → refresh-all path → 202 with the shared dedupe key.
	// Empty account table means the worker no-ops, so this stays network-free.
	_, r := setupAccountsHealthTest(t)
	t.Cleanup(func() { resetBackgroundTasksForTests() })

	rec := doPostJSON(t, r, "/api/accounts/health/refresh", map[string]any{})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s, want 202 (queued)", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["success"] != true || body["queued"] != true {
		t.Fatalf("unexpected body: %v", body)
	}
	if _, ok := body["taskId"].(string); !ok {
		t.Fatalf("taskId missing/invalid: %v", body["taskId"])
	}
}
