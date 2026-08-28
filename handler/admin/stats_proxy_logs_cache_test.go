package admin

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// seedProxyLogsCacheFixture inserts three standalone proxy_logs rows (two
// success, one failed) with distinct costs/token counts so summary
// aggregates have non-trivial expected values:
//
//	totalCount=3, successCount=2, failedCount=1,
//	totalCost=0.30, totalTokensAll=100+25+15=140 (the third row exercises the
//	prompt+completion fallback of EffectiveProxyTokensSQL).
func seedProxyLogsCacheFixture(t *testing.T, db *store.DB) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	rows := []struct {
		model              string
		status             string
		prompt, completion int
		totalTokens        any
		cost               float64
	}{
		{"cache-a", "success", 0, 0, 100, 0.10},
		{"cache-b", "success", 25, 15, nil, 0.05},
		{"cache-c", "failed", 10, 10, 50, 0.15},
	}
	for _, row := range rows {
		if row.totalTokens == nil {
			mustExec(t, db, `INSERT INTO proxy_logs (model_requested, model_actual, status, prompt_tokens, completion_tokens, estimated_cost, created_at)
				VALUES (?, ?, ?, ?, ?, ?, ?)`, row.model, row.model, row.status, row.prompt, row.completion, row.cost, now)
		} else {
			mustExec(t, db, `INSERT INTO proxy_logs (model_requested, model_actual, status, prompt_tokens, completion_tokens, total_tokens, estimated_cost, created_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, row.model, row.model, row.status, row.prompt, row.completion, row.totalTokens, row.cost, now)
		}
	}
}

// proxyLogsSummaryBody decodes the summary block of a meta/full response.
type proxyLogsSummaryBody struct {
	TotalCount     int     `json:"totalCount"`
	SuccessCount   int     `json:"successCount"`
	FailedCount    int     `json:"failedCount"`
	TotalCost      float64 `json:"totalCost"`
	TotalTokensAll int64   `json:"totalTokensAll"`
}

func fetchProxyLogsSummaryBody(t *testing.T, r chi.Router, query string) (proxyLogsSummaryBody, map[string]any) {
	t.Helper()
	resp := doGet(t, r, "/api/stats/proxy-logs?"+query)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /api/stats/proxy-logs?%s status=%d body=%s", query, resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal %s: %v", query, err)
	}
	raw, ok := body["summary"].(map[string]any)
	if !ok {
		t.Fatalf("GET /api/stats/proxy-logs?%s missing summary: %s", query, resp.Body.String())
	}
	rawJSON, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("re-marshal summary: %v", err)
	}
	var summary proxyLogsSummaryBody
	if err := json.Unmarshal(rawJSON, &summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	return summary, body
}

// TestStats_SQLiteProxyLogsQueryViewOmitsSummary locks in the view=query
// contract the list page relies on after the perf split: the list-only view
// returns items/total/page/pageSize and NOTHING else — no summary, sites, or
// clientOptions — so a page load never computes the five-way summary twice
// (once in the list fetch, once in the meta fetch).
func TestStats_SQLiteProxyLogsQueryViewOmitsSummary(t *testing.T) {
	db, r := setupStatsSQLiteTest(t)
	seedProxyLogsCacheFixture(t, db)

	resp := doGet(t, r, "/api/stats/proxy-logs?view=query")
	if resp.Code != http.StatusOK {
		t.Fatalf("view=query status=%d body=%s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, want := range []string{"items", "total", "page", "pageSize"} {
		if _, ok := body[want]; !ok {
			t.Fatalf("view=query response missing %q: %v", want, body)
		}
	}
	for _, absent := range []string{"summary", "sites", "clientOptions"} {
		if _, ok := body[absent]; ok {
			t.Fatalf("view=query response must NOT contain %q (summary travels via view=meta): %v", absent, body)
		}
	}
	if got := int(body["total"].(float64)); got != 3 {
		t.Fatalf("view=query total = %d, want 3", got)
	}
	if items, ok := body["items"].([]any); !ok || len(items) != 3 {
		t.Fatalf("view=query items = %#v, want 3 rows", body["items"])
	}
}

// TestStats_ProxyLogsSummaryCache_MissThenHit verifies the core dedup
// contract: the first meta request misses (and populates), the second
// identical request hits and returns the same summary values.
func TestStats_ProxyLogsSummaryCache_MissThenHit(t *testing.T) {
	db, r := setupStatsSQLiteTest(t)
	seedProxyLogsCacheFixture(t, db)

	first := doGet(t, r, "/api/stats/proxy-logs?view=meta")
	if first.Code != http.StatusOK {
		t.Fatalf("first meta: %d %s", first.Code, first.Body.String())
	}
	if got := first.Header().Get("x-proxy-logs-summary-cache"); got != "miss" {
		t.Fatalf("first request cache header = %q, want miss", got)
	}

	second := doGet(t, r, "/api/stats/proxy-logs?view=meta")
	if got := second.Header().Get("x-proxy-logs-summary-cache"); got != "hit" {
		t.Fatalf("second request cache header = %q, want hit", got)
	}
	if first.Body.String() != second.Body.String() {
		t.Fatalf("cache hit body differs from miss body.\nmiss: %s\nhit:  %s", first.Body.String(), second.Body.String())
	}
}

// TestStats_ProxyLogsSummaryCache_StaleThenForce verifies the TTL semantics:
// a data change within the TTL window is NOT visible to plain requests (the
// cached aggregate serves), but ?force=1 (and the meta client's ?refresh=1)
// invalidate and recompute. Mirrors TestDashboardCache_ForceRefreshBypassesAndInvalidates.
func TestStats_ProxyLogsSummaryCache_StaleThenForce(t *testing.T) {
	db, r := setupStatsSQLiteTest(t)
	seedProxyLogsCacheFixture(t, db)

	if got := doGet(t, r, "/api/stats/proxy-logs?view=meta").Header().Get("x-proxy-logs-summary-cache"); got != "miss" {
		t.Fatalf("first request should miss, got %q", got)
	}

	// Mutate the data: a fourth log makes a fresh compute differ.
	now := time.Now().UTC().Format(time.RFC3339)
	mustExec(t, db, `INSERT INTO proxy_logs (model_requested, model_actual, status, total_tokens, estimated_cost, created_at)
		VALUES ('cache-d', 'cache-d', 'success', 7, 0.01, ?)`, now)

	// A plain request still hits the stale cache.
	stale := doGet(t, r, "/api/stats/proxy-logs?view=meta")
	if stale.Header().Get("x-proxy-logs-summary-cache") != "hit" {
		t.Fatalf("expected hit from stale cache, got %q", stale.Header().Get("x-proxy-logs-summary-cache"))
	}
	var staleBody map[string]any
	if err := json.Unmarshal(stale.Body.Bytes(), &staleBody); err != nil {
		t.Fatalf("unmarshal stale: %v", err)
	}
	staleSummary := staleBody["summary"].(map[string]any)
	if got := int(staleSummary["totalCount"].(float64)); got != 3 {
		t.Fatalf("stale cached totalCount = %d, want 3 (cache served pre-insert aggregate)", got)
	}

	// force=1 bypasses + invalidates, recomputing from the DB.
	forced := doGet(t, r, "/api/stats/proxy-logs?view=meta&force=1")
	if got := forced.Header().Get("x-proxy-logs-summary-cache"); got != "miss" {
		t.Fatalf("force=1 should report miss, got %q", got)
	}
	var forcedBody map[string]any
	if err := json.Unmarshal(forced.Body.Bytes(), &forcedBody); err != nil {
		t.Fatalf("unmarshal forced: %v", err)
	}
	forcedSummary := forcedBody["summary"].(map[string]any)
	if got := int(forcedSummary["totalCount"].(float64)); got != 4 {
		t.Fatalf("forced totalCount = %d, want 4 (force did not recompute)", got)
	}

	// A subsequent plain request hits the freshly-populated cache.
	after := doGet(t, r, "/api/stats/proxy-logs?view=meta")
	if after.Header().Get("x-proxy-logs-summary-cache") != "hit" {
		t.Fatalf("post-force request should hit repopulated cache, got %q", after.Header().Get("x-proxy-logs-summary-cache"))
	}
	if after.Body.String() != forced.Body.String() {
		t.Fatal("post-force cache hit did not return the freshly-populated body")
	}

	// The meta client speaks `refresh` rather than `force`; it must invalidate
	// exactly the same way. Mutate again so a fresh compute differs.
	mustExec(t, db, `INSERT INTO proxy_logs (model_requested, model_actual, status, total_tokens, estimated_cost, created_at)
		VALUES ('cache-e', 'cache-e', 'failed', 3, 0.02, ?)`, now)
	refreshed := doGet(t, r, "/api/stats/proxy-logs?view=meta&refresh=1")
	if got := refreshed.Header().Get("x-proxy-logs-summary-cache"); got != "miss" {
		t.Fatalf("refresh=1 should report miss, got %q", got)
	}
	var refreshedBody map[string]any
	if err := json.Unmarshal(refreshed.Body.Bytes(), &refreshedBody); err != nil {
		t.Fatalf("unmarshal refreshed: %v", err)
	}
	refreshedSummary := refreshedBody["summary"].(map[string]any)
	if got := int(refreshedSummary["totalCount"].(float64)); got != 5 {
		t.Fatalf("refresh=1 totalCount = %d, want 5", got)
	}
}

// TestStats_ProxyLogsSummaryCache_FingerprintKeying verifies the cache is
// keyed by the filter fingerprint: a different filter is a different entry,
// and the split view=query + view=meta page load shares one entry (view/query
// params like limit/offset do not fork the key).
func TestStats_ProxyLogsSummaryCache_FingerprintKeying(t *testing.T) {
	db, r := setupStatsSQLiteTest(t)
	seedProxyLogsCacheFixture(t, db)

	// Populate the status=success fingerprint.
	if got := doGet(t, r, "/api/stats/proxy-logs?view=meta&status=success").Header().Get("x-proxy-logs-summary-cache"); got != "miss" {
		t.Fatalf("status=success first should miss, got %q", got)
	}
	// The unfiltered fingerprint is a different key: must miss.
	if got := doGet(t, r, "/api/stats/proxy-logs?view=meta").Header().Get("x-proxy-logs-summary-cache"); got != "miss" {
		t.Fatalf("unfiltered first should miss (fingerprint-keyed cache), got %q", got)
	}
	// Both now hit independently.
	if got := doGet(t, r, "/api/stats/proxy-logs?view=meta&status=success").Header().Get("x-proxy-logs-summary-cache"); got != "hit" {
		t.Fatal("status=success should hit after population")
	}
	if got := doGet(t, r, "/api/stats/proxy-logs?view=meta").Header().Get("x-proxy-logs-summary-cache"); got != "hit" {
		t.Fatal("unfiltered should hit after population")
	}

	// view=full with the same filters shares the meta fingerprint — the page's
	// legacy full-view load must not recompute a summary the meta fetch cached.
	if got := doGet(t, r, "/api/stats/proxy-logs?view=full").Header().Get("x-proxy-logs-summary-cache"); got != "hit" {
		t.Fatal("view=full should hit the fingerprint cached by view=meta")
	}
	// Pagination params (limit/offset) are part of the list query but NOT the
	// fingerprint: page 2 of the same filters reuses the cached summary.
	if got := doGet(t, r, "/api/stats/proxy-logs?view=full&limit=1&offset=1").Header().Get("x-proxy-logs-summary-cache"); got != "hit" {
		t.Fatal("page-turn (limit/offset change) should hit the same fingerprint")
	}
}

// TestStats_ProxyLogsSummaryCache_SiteIdFilterStillAggregates guards the
// conditional-join path: with siteId set the aggregate FROM keeps the
// accounts/sites join (the WHERE references s.id) and the summary must still
// count only the filtered site's logs.
func TestStats_ProxyLogsSummaryCache_SiteIdFilterStillAggregates(t *testing.T) {
	db, r := setupStatsSQLiteTest(t)
	now := time.Now().UTC().Format(time.RFC3339)
	mustExec(t, db, `INSERT INTO sites (name, url, platform, status, created_at, updated_at)
		VALUES (?, ?, ?, 'active', ?, ?)`, "agg-site", "https://agg.example.test", "openai", now, now)
	var siteID int64
	if err := db.Get(&siteID, "SELECT id FROM sites WHERE name = ?", "agg-site"); err != nil {
		t.Fatalf("site id: %v", err)
	}
	mustExec(t, db, `INSERT INTO accounts (site_id, username, access_token, status, balance, checkin_enabled, created_at, updated_at)
		VALUES (?, ?, ?, 'active', ?, FALSE, ?, ?)`, siteID, "agg-user", "sk-agg", 1.0, now, now)
	var accountID int64
	if err := db.Get(&accountID, "SELECT id FROM accounts WHERE username = ?", "agg-user"); err != nil {
		t.Fatalf("account id: %v", err)
	}
	mustExec(t, db, `INSERT INTO proxy_logs (account_id, model_requested, model_actual, status, total_tokens, estimated_cost, created_at)
		VALUES (?, 'agg-linked', 'agg-linked', 'success', 30, 0.2, ?)`, accountID, now)
	// A standalone row (no account) must NOT count toward the site filter.
	mustExec(t, db, `INSERT INTO proxy_logs (model_requested, model_actual, status, total_tokens, estimated_cost, created_at)
		VALUES ('agg-orphan', 'agg-orphan', 'success', 99, 0.9, ?)`, now)

	summary, _ := fetchProxyLogsSummaryBody(t, r, "view=full&siteId="+itoa(siteID))
	if summary.TotalCount != 1 {
		t.Fatalf("siteId summary.totalCount = %d, want 1 (only the linked row)", summary.TotalCount)
	}
	if summary.TotalTokensAll != 30 {
		t.Fatalf("siteId summary.totalTokensAll = %d, want 30", summary.TotalTokensAll)
	}
}

// TestStats_ProxyLogsAggregateSQL_NoJoinWithoutSiteID asserts the join-free
// aggregate shape at the SQL level (the regression the 500k-row audit
// measured: the LEFT JOINs added ~90ms to the summary and ~100ms to the
// COUNT). Pure string assertions — no database needed.
func TestStats_ProxyLogsAggregateSQL_NoJoinWithoutSiteID(t *testing.T) {
	from := proxyLogsAggregateFrom(0, "")
	if strings.Contains(from, "JOIN") {
		t.Fatalf("aggregate FROM without siteId must be join-free, got %q", from)
	}
	if !strings.Contains(from, "FROM proxy_logs pl") {
		t.Fatalf("aggregate FROM without siteId must scan proxy_logs directly, got %q", from)
	}

	countQuery := "SELECT COUNT(*) " + proxyLogsAggregateFrom(0, "") + " WHERE pl.status = 'success'"
	if strings.Contains(countQuery, "JOIN") {
		t.Fatalf("COUNT query without siteId must be join-free, got %q", countQuery)
	}

	summaryQuery := proxyLogsSummaryQuerySQL(proxyLogsAggregateFrom(0, ""), "")
	if strings.Contains(summaryQuery, "JOIN") {
		t.Fatalf("summary query without siteId must be join-free, got %q", summaryQuery)
	}
	for _, alias := range []string{"total_count", "success_count", "failed_count", "total_cost", "total_tokens_all"} {
		if !strings.Contains(summaryQuery, alias) {
			t.Fatalf("summary query missing aggregate %q: %s", alias, summaryQuery)
		}
	}

	// With a siteId the WHERE references s.id, so the accounts/sites joins
	// must stay — as INNER joins (Wave 18 index audit): s.id = ? already
	// discards non-matching rows, and INNER lets the planner drive from the
	// site's accounts into the proxy_logs indexes instead of scanning
	// proxy_logs first. The dk join stays LEFT because only the search filter
	// reads it and it must not drop rows.
	fromSite := proxyLogsAggregateFrom(3, "")
	if !strings.Contains(fromSite, "INNER JOIN accounts a ON pl.account_id = a.id") {
		t.Fatalf("aggregate FROM with siteId must inner-join accounts, got %q", fromSite)
	}
	if !strings.Contains(fromSite, "INNER JOIN sites s ON a.site_id = s.id") {
		t.Fatalf("aggregate FROM with siteId must inner-join sites, got %q", fromSite)
	}
	if !strings.Contains(fromSite, "LEFT JOIN downstream_api_keys dk ON pl.downstream_api_key_id = dk.id") {
		t.Fatalf("aggregate FROM with siteId must keep the LEFT downstream_api_keys join, got %q", fromSite)
	}
	if strings.Contains(fromSite, "LEFT JOIN accounts") || strings.Contains(fromSite, "LEFT JOIN sites") {
		t.Fatalf("aggregate FROM with siteId must not LEFT JOIN accounts/sites, got %q", fromSite)
	}
	summarySite := proxyLogsSummaryQuerySQL(fromSite, " WHERE s.id = ?")
	if !strings.Contains(summarySite, "JOIN") {
		t.Fatalf("summary query with siteId must keep the join, got %q", summarySite)
	}

	// With a search active the WHERE references a.username / dk.key / dk.name,
	// so the accounts sites AND downstream_api_keys joins must all stay.
	fromSearch := proxyLogsAggregateFrom(0, "oneapi")
	for _, fragment := range []string{
		"LEFT JOIN accounts a ON pl.account_id = a.id",
		"LEFT JOIN sites s ON a.site_id = s.id",
		"LEFT JOIN downstream_api_keys dk ON pl.downstream_api_key_id = dk.id",
	} {
		if !strings.Contains(fromSearch, fragment) {
			t.Fatalf("aggregate FROM with search must contain %q, got %q", fragment, fromSearch)
		}
	}
	summarySearch := proxyLogsSummaryQuerySQL(fromSearch, " WHERE dk.key LIKE ?")
	if !strings.Contains(summarySearch, "LEFT JOIN downstream_api_keys dk") {
		t.Fatalf("summary query with search must keep the key join, got %q", summarySearch)
	}
}

// TestStats_SQLiteProxyLogsAggregatePlan_NoJoinWithoutSiteID strengthens the
// string assertion with SQLite's query planner: the executed COUNT and summary
// plans must not touch accounts/sites when no siteId filter is set.
func TestStats_SQLiteProxyLogsAggregatePlan_NoJoinWithoutSiteID(t *testing.T) {
	db, _ := setupStatsSQLiteTest(t)
	seedProxyLogsCacheFixture(t, db)

	for label, sql := range map[string]string{
		"count":   "SELECT COUNT(*) " + proxyLogsAggregateFrom(0, ""),
		"summary": proxyLogsSummaryQuerySQL(proxyLogsAggregateFrom(0, ""), ""),
	} {
		planRows, err := queryRowsErr(db.DB, "EXPLAIN QUERY PLAN "+sql)
		if err != nil {
			t.Fatalf("explain %s: %v", label, err)
		}
		if len(planRows) == 0 {
			t.Fatalf("explain %s returned no plan rows", label)
		}
		for _, row := range planRows {
			detail, _ := row["detail"].(string)
			// Joined tables appear as "SEARCH a/s ... LEFT-JOIN" (alias-based);
			// also guard against full table names in case the planner spells
			// them out on another SQLite version.
			if strings.Contains(detail, "LEFT-JOIN") || strings.Contains(detail, "accounts") || strings.Contains(detail, "sites") {
				t.Fatalf("%s plan touches accounts/sites without a siteId filter: %v", label, planRows)
			}
		}
	}

	// Sanity check the other direction: with a siteId the plan joins
	// accounts/sites (INNER since the Wave 18 index audit) and keeps the
	// LEFT downstream_api_keys join.
	planRows, err := queryRowsErr(db.DB, "EXPLAIN QUERY PLAN "+proxyLogsSummaryQuerySQL(proxyLogsAggregateFrom(1, ""), " WHERE s.id = 1"))
	if err != nil {
		t.Fatalf("explain siteId summary: %v", err)
	}
	joined := false
	touchesAccounts := false
	for _, row := range planRows {
		detail, _ := row["detail"].(string)
		if strings.Contains(detail, "LEFT-JOIN") {
			joined = true
		}
		if strings.Contains(detail, "accounts") || strings.HasPrefix(detail, "SEARCH a ") || strings.HasPrefix(detail, "SCAN a ") {
			touchesAccounts = true
		}
	}
	if !joined {
		t.Fatalf("siteId summary plan should keep the LEFT downstream_api_keys join: %v", planRows)
	}
	if !touchesAccounts {
		t.Fatalf("siteId summary plan should join accounts: %v", planRows)
	}
}

// TestStats_PostgresProxyLogsSummaryCache_MissThenHit is the PostgreSQL
// dialect twin of the miss→hit contract (skipped without PG_TEST_DSN).
func TestStats_PostgresProxyLogsSummaryCache_MissThenHit(t *testing.T) {
	db, r := setupStatsPostgresTest(t)

	suffix := "-" + time.Now().UTC().Format("150405.000000000")
	now := time.Now().UTC().Format(time.RFC3339)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM proxy_logs WHERE model_requested LIKE 'pg-proxylogs-cache%'")
	})
	for _, row := range []struct {
		model  string
		status string
	}{
		{"pg-proxylogs-cache-a" + suffix, "success"},
		{"pg-proxylogs-cache-b" + suffix, "failed"},
	} {
		if _, err := db.Exec(`INSERT INTO proxy_logs (model_requested, model_actual, status, total_tokens, estimated_cost, created_at)
			VALUES (?, ?, ?, 10, 0.05, ?)`, row.model, row.model, row.status, now); err != nil {
			t.Fatalf("insert %s: %v", row.model, err)
		}
	}

	// Filter by the unique model so concurrent CI runs on the shared database
	// don't perturb the fingerprint's aggregate.
	q := "view=meta&search=pg-proxylogs-cache-a" + suffix
	first := doGet(t, r, "/api/stats/proxy-logs?"+q)
	if first.Code != http.StatusOK {
		t.Fatalf("first meta: %d %s", first.Code, first.Body.String())
	}
	if got := first.Header().Get("x-proxy-logs-summary-cache"); got != "miss" {
		t.Fatalf("first request cache header = %q, want miss", got)
	}
	second := doGet(t, r, "/api/stats/proxy-logs?"+q)
	if got := second.Header().Get("x-proxy-logs-summary-cache"); got != "hit" {
		t.Fatalf("second request cache header = %q, want hit", got)
	}
	if first.Body.String() != second.Body.String() {
		t.Fatalf("cache hit body differs from miss body.\nmiss: %s\nhit:  %s", first.Body.String(), second.Body.String())
	}

	// The join-free aggregate must also hold on PG: no siteId → no join in the
	// executed SQL (string-level check is dialect-neutral).
	if strings.Contains(proxyLogsAggregateFrom(0, ""), "JOIN") {
		t.Fatal("aggregate FROM without siteId must be join-free on every dialect")
	}
}
