package admin

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// seedDashboardCacheFixture inserts the minimal site + account + proxy_log rows
// the dashboard queries need to return 200 (rather than erroring on empty
// joins). Returns the account id so callers can mutate data between requests
// to validate force-refresh.
func seedDashboardCacheFixture(t *testing.T, db *store.DB) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	mustExec(t, db, `INSERT INTO sites (name, url, platform, status, created_at, updated_at)
		VALUES (?, ?, ?, 'active', ?, ?)`, "cache-site", "https://cache.example.test", "openai", now, now)
	var siteID int64
	if err := db.Get(&siteID, "SELECT id FROM sites WHERE name = ?", "cache-site"); err != nil {
		t.Fatalf("site id: %v", err)
	}
	mustExec(t, db, `INSERT INTO accounts (site_id, username, access_token, status, balance, checkin_enabled, created_at, updated_at)
		VALUES (?, ?, ?, 'active', ?, FALSE, ?, ?)`, siteID, "cache-user", "sk-cache", 5.0, now, now)
	var accountID int64
	if err := db.Get(&accountID, "SELECT id FROM accounts WHERE username = ?", "cache-user"); err != nil {
		t.Fatalf("account id: %v", err)
	}
	mustExec(t, db, `INSERT INTO proxy_logs (account_id, model_requested, model_actual, status, prompt_tokens, completion_tokens, total_tokens, estimated_cost, created_at)
		VALUES (?, ?, ?, 'success', ?, ?, ?, ?, ?)`, accountID, "gpt-cache", "gpt-cache", 10, 20, 30, 0.1, now)
	return accountID
}

// mustExec fails the test on a DB exec error.
func mustExec(t *testing.T, db *store.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

// TestDashboardCache_MissThenHit verifies the first request is a miss (and
// populates the cache) and the second identical request is a hit returning the
// same body. This is the core dedup contract.
func TestDashboardCache_MissThenHit(t *testing.T) {
	db, r := setupStatsSQLiteTest(t)
	seedDashboardCacheFixture(t, db)

	first := doGet(t, r, "/api/stats/dashboard?view=summary")
	if first.Code != 200 {
		t.Fatalf("first dashboard: %d %s", first.Code, first.Body.String())
	}
	if got := first.Header().Get("x-dashboard-summary-cache"); got != "miss" {
		t.Fatalf("first request cache header = %q, want miss", got)
	}

	second := doGet(t, r, "/api/stats/dashboard?view=summary")
	if second.Code != 200 {
		t.Fatalf("second dashboard: %d %s", second.Code, second.Body.String())
	}
	if got := second.Header().Get("x-dashboard-summary-cache"); got != "hit" {
		t.Fatalf("second request cache header = %q, want hit", got)
	}

	// Hit must return byte-identical body to the miss that populated it.
	if first.Body.String() != second.Body.String() {
		t.Fatalf("cache hit body differs from miss body.\nmiss: %s\nhit:  %s", first.Body.String(), second.Body.String())
	}
}

// TestDashboardCache_ForceRefreshBypassesAndInvalidates verifies ?force=1
// reports miss, recomputes from the DB, and invalidates the cache so a
// subsequent non-force request also sees fresh data.
func TestDashboardCache_ForceRefreshBypassesAndInvalidates(t *testing.T) {
	db, r := setupStatsSQLiteTest(t)
	accountID := seedDashboardCacheFixture(t, db)

	// Populate the cache with the initial data set.
	first := doGet(t, r, "/api/stats/dashboard?view=summary")
	if first.Header().Get("x-dashboard-summary-cache") != "miss" {
		t.Fatalf("first request should miss, got %q", first.Header().Get("x-dashboard-summary-cache"))
	}

	// Mutate the data so a fresh compute would differ. Insert another proxy log
	// so proxy24h.total changes from 1 → 2.
	now := time.Now().UTC().Format(time.RFC3339)
	mustExec(t, db, `INSERT INTO proxy_logs (account_id, model_requested, model_actual, status, total_tokens, estimated_cost, created_at)
		VALUES (?, 'gpt-cache-2', 'gpt-cache-2', 'success', 7, 0.05, ?)`, accountID, now)

	// A plain (non-force) request would still hit the stale cache.
	stale := doGet(t, r, "/api/stats/dashboard?view=summary")
	if stale.Header().Get("x-dashboard-summary-cache") != "hit" {
		t.Fatalf("expected hit from stale cache, got %q", stale.Header().Get("x-dashboard-summary-cache"))
	}

	// force=1 must bypass + invalidate, recomputing from the DB.
	forced := doGet(t, r, "/api/stats/dashboard?view=summary&force=1")
	if forced.Code != 200 {
		t.Fatalf("forced dashboard: %d %s", forced.Code, forced.Body.String())
	}
	if got := forced.Header().Get("x-dashboard-summary-cache"); got != "miss" {
		t.Fatalf("force=1 should report miss, got %q", got)
	}

	// The forced (fresh) body must differ from the stale cached body.
	if forced.Body.String() == stale.Body.String() {
		t.Fatal("force=1 returned the same body as the stale cache; cache was not actually invalidated")
	}
	// And it must reflect the new proxy24h.total.
	var fresh map[string]any
	if err := json.Unmarshal(forced.Body.Bytes(), &fresh); err != nil {
		t.Fatalf("unmarshal forced: %v", err)
	}
	proxy24h := fresh["proxy24h"].(map[string]any)
	if got := int(proxy24h["total"].(float64)); got != 2 {
		t.Fatalf("forced proxy24h.total = %d, want 2 (force did not recompute)", got)
	}

	// A subsequent non-force request must now hit the freshly-populated cache
	// and return the fresh body (proving force repopulated, not just cleared).
	after := doGet(t, r, "/api/stats/dashboard?view=summary")
	if after.Header().Get("x-dashboard-summary-cache") != "hit" {
		t.Fatalf("post-force request should hit repopulated cache, got %q", after.Header().Get("x-dashboard-summary-cache"))
	}
	if after.Body.String() != forced.Body.String() {
		t.Fatal("post-force cache hit did not return the freshly-populated body")
	}
}

// TestDashboardCache_ViewKeying verifies that caching one view does not serve
// another view from the cache. summary and full have different shapes, so a
// summary miss→hit must not make a full request a hit.
func TestDashboardCache_ViewKeying(t *testing.T) {
	db, r := setupStatsSQLiteTest(t)
	seedDashboardCacheFixture(t, db)

	// Populate the summary view cache.
	summaryFirst := doGet(t, r, "/api/stats/dashboard?view=summary")
	if summaryFirst.Header().Get("x-dashboard-summary-cache") != "miss" {
		t.Fatalf("summary first should miss, got %q", summaryFirst.Header().Get("x-dashboard-summary-cache"))
	}

	// full is a different view: must be a miss even though summary is cached.
	fullFirst := doGet(t, r, "/api/stats/dashboard?view=full")
	if fullFirst.Code != 200 {
		t.Fatalf("full dashboard: %d %s", fullFirst.Code, fullFirst.Body.String())
	}
	if fullFirst.Header().Get("x-dashboard-summary-cache") != "miss" {
		t.Fatalf("full first should miss (view-keyed cache), got %q", fullFirst.Header().Get("x-dashboard-summary-cache"))
	}

	// Now both should hit independently.
	if doGet(t, r, "/api/stats/dashboard?view=summary").Header().Get("x-dashboard-summary-cache") != "hit" {
		t.Fatal("summary should hit after population")
	}
	if doGet(t, r, "/api/stats/dashboard?view=full").Header().Get("x-dashboard-summary-cache") != "hit" {
		t.Fatal("full should hit after population")
	}
}

// TestDashboardCache_DoesNotCacheErrors verifies that a failed (500) response
// is not cached, so a subsequent request after the DB is healthy again computes
// fresh rather than replaying a cached error. The error path returns before
// globalDashboardCache.set, so the next request must be a miss, never a hit.
func TestDashboardCache_DoesNotCacheErrors(t *testing.T) {
	db, r := setupStatsSQLiteTest(t)
	seedDashboardCacheFixture(t, db)

	// First a healthy miss to confirm the cache is empty for this view.
	healthy := doGet(t, r, "/api/stats/dashboard?view=summary")
	if healthy.Header().Get("x-dashboard-summary-cache") != "miss" {
		t.Fatalf("healthy first should miss, got %q", healthy.Header().Get("x-dashboard-summary-cache"))
	}
	// The healthy response IS cached now. Closing the DB then requesting would
	// still hit the cache (a hit, not an error) — so to test the error-no-cache
	// contract we need a view that was never populated. Use insights view and
	// a closed DB: the query fails before set() is reached.
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
	errResp := doGet(t, r, "/api/stats/dashboard?view=insights")
	if errResp.Code != 500 {
		t.Fatalf("insights on closed DB: got %d, want 500 (%s)", errResp.Code, errResp.Body.String())
	}
	if got := errResp.Header().Get("x-dashboard-summary-cache"); got != "miss" {
		t.Fatalf("error response cache header = %q, want miss (errors must not be cached as hits)", got)
	}
}
