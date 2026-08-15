package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// setupMonitorHealthTest opens an in-memory SQLite database, runs AutoMigrate,
// and registers the monitor health route on a standalone Chi router.
func setupMonitorHealthTest(t *testing.T) (*store.DB, chi.Router, *config.Config) {
	t.Helper()
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("failed to open SQLite: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := store.AutoMigrate(db); err != nil {
		db.Close()
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	cfg := &config.Config{
		TokenRouterFailureCooldownMaxSec: 30 * 24 * 60 * 60,
	}

	r := chi.NewRouter()
	RegisterMonitorHealthRoute(r, db.DB, cfg)
	return db, r, cfg
}

// insertTestSite inserts a minimal site row and returns its id.
func insertTestSite(t *testing.T, db *store.DB, name, status string) int64 {
	t.Helper()
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	query := db.Rebind("INSERT INTO sites (name, url, platform, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)")
	res, err := db.Exec(query, name, "https://"+name+".example.com", "openai", status, now, now)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("site LastInsertId: %v", err)
	}
	return id
}

// insertTestAccount inserts a minimal account row and returns its id.
func insertTestAccount(t *testing.T, db *store.DB, siteID int64, username, status string) int64 {
	t.Helper()
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	query := db.Rebind("INSERT INTO accounts (site_id, username, access_token, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)")
	res, err := db.Exec(query, siteID, username, "sk-test-token", status, now, now)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("account LastInsertId: %v", err)
	}
	return id
}

// insertTestTokenRoute inserts a minimal token_routes row and returns its id.
// Required as the FK parent of route_channels.
func insertTestTokenRoute(t *testing.T, db *store.DB) int64 {
	t.Helper()
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	query := db.Rebind("INSERT INTO token_routes (model_pattern, created_at, updated_at) VALUES (?, ?, ?)")
	res, err := db.Exec(query, "gpt-4o", now, now)
	if err != nil {
		t.Fatalf("insert token_route: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("token_route LastInsertId: %v", err)
	}
	return id
}

// insertTestRouteChannel inserts a route_channels row with the given fail/cooldown
// state. Creates the required token_routes FK parent automatically.
func insertTestRouteChannel(t *testing.T, db *store.DB, accountID int64, failCount int, lastFailAt, cooldownUntil *string) {
	t.Helper()
	routeID := insertTestTokenRoute(t, db)
	query := db.Rebind("INSERT INTO route_channels (route_id, account_id, fail_count, last_fail_at, cooldown_until) VALUES (?, ?, ?, ?, ?)")
	var failArg any = failCount
	var lastFailArg any
	if lastFailAt != nil {
		lastFailArg = *lastFailAt
	}
	var cooldownArg any
	if cooldownUntil != nil {
		cooldownArg = *cooldownUntil
	}
	if _, err := db.Exec(query, routeID, accountID, failArg, lastFailArg, cooldownArg); err != nil {
		t.Fatalf("insert route_channel: %v", err)
	}
}

// doHealthGet performs a GET /api/monitor/health request.
func doHealthGet(t *testing.T, r chi.Router) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/monitor/health", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestMonitorHealth_EmptyDatabase(t *testing.T) {
	_, r, _ := setupMonitorHealthTest(t)

	resp := doHealthGet(t, r)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var result map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if result["generatedAt"] == nil {
		t.Fatal("expected generatedAt field")
	}

	cooldown, ok := result["cooldown"].(map[string]any)
	if !ok {
		t.Fatalf("expected cooldown map, got %T", result["cooldown"])
	}
	if cooldown["channelsCooling"] != float64(0) {
		t.Errorf("channelsCooling = %v, want 0", cooldown["channelsCooling"])
	}
	if cooldown["channelsWithFailures"] != float64(0) {
		t.Errorf("channelsWithFailures = %v, want 0", cooldown["channelsWithFailures"])
	}
	if cooldown["channelsRecentlyFailed"] != float64(0) {
		t.Errorf("channelsRecentlyFailed = %v, want 0", cooldown["channelsRecentlyFailed"])
	}

	sites, ok := result["sites"].(map[string]any)
	if !ok {
		t.Fatalf("expected sites map, got %T", result["sites"])
	}
	if sites["total"] != float64(0) {
		t.Errorf("sites total = %v, want 0", sites["total"])
	}

	accounts, ok := result["accounts"].(map[string]any)
	if !ok {
		t.Fatalf("expected accounts map, got %T", result["accounts"])
	}
	if accounts["total"] != float64(0) {
		t.Errorf("accounts total = %v, want 0", accounts["total"])
	}
}

func TestMonitorHealth_SiteStatusCounts(t *testing.T) {
	db, r, _ := setupMonitorHealthTest(t)

	insertTestSite(t, db, "active-site-1", "active")
	insertTestSite(t, db, "active-site-2", "active")
	insertTestSite(t, db, "disabled-site", "disabled")
	insertTestSite(t, db, "other-site", "maintenance")

	resp := doHealthGet(t, r)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var result map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	sites, ok := result["sites"].(map[string]any)
	if !ok {
		t.Fatalf("expected sites map, got %T", result["sites"])
	}
	if sites["total"] != float64(4) {
		t.Errorf("sites total = %v, want 4", sites["total"])
	}
	if sites["active"] != float64(2) {
		t.Errorf("sites active = %v, want 2", sites["active"])
	}
	if sites["disabled"] != float64(1) {
		t.Errorf("sites disabled = %v, want 1", sites["disabled"])
	}
	if sites["other"] != float64(1) {
		t.Errorf("sites other = %v, want 1", sites["other"])
	}
}

func TestMonitorHealth_AccountStatusCounts(t *testing.T) {
	db, r, _ := setupMonitorHealthTest(t)

	siteID := insertTestSite(t, db, "acct-test-site", "active")
	insertTestAccount(t, db, siteID, "user1", "active")
	insertTestAccount(t, db, siteID, "user2", "active")
	insertTestAccount(t, db, siteID, "user3", "disabled")
	insertTestAccount(t, db, siteID, "user4", "expired")

	resp := doHealthGet(t, r)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var result map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	accounts, ok := result["accounts"].(map[string]any)
	if !ok {
		t.Fatalf("expected accounts map, got %T", result["accounts"])
	}
	if accounts["total"] != float64(4) {
		t.Errorf("accounts total = %v, want 4", accounts["total"])
	}
	if accounts["active"] != float64(2) {
		t.Errorf("accounts active = %v, want 2", accounts["active"])
	}
	if accounts["disabled"] != float64(1) {
		t.Errorf("accounts disabled = %v, want 1", accounts["disabled"])
	}
	if accounts["other"] != float64(1) {
		t.Errorf("accounts other = %v, want 1", accounts["other"])
	}
}

func TestMonitorHealth_CooldownAggregation(t *testing.T) {
	db, r, _ := setupMonitorHealthTest(t)

	siteID := insertTestSite(t, db, "cooldown-site", "active")
	accountID := insertTestAccount(t, db, siteID, "cooldown-user", "active")

	// Channel with active cooldown (future timestamp) and failures.
	// lastFailAt must be within the Fibonacci backoff window (15s for
	// failCount=3 → 30s backoff) so IsChannelRecentlyFailed returns true.
	futureCooldown := time.Now().Add(30 * time.Minute).UTC().Format(time.RFC3339)
	recentFail := time.Now().Add(-1 * time.Second).UTC().Format(time.RFC3339)
	insertTestRouteChannel(t, db, accountID, 3, &recentFail, &futureCooldown)

	// Channel with failures but no cooldown. lastFailAt within 15s backoff.
	recentFail2 := time.Now().Add(-1 * time.Second).UTC().Format(time.RFC3339)
	insertTestRouteChannel(t, db, accountID, 1, &recentFail2, nil)

	resp := doHealthGet(t, r)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var result map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	cooldown, ok := result["cooldown"].(map[string]any)
	if !ok {
		t.Fatalf("expected cooldown map, got %T", result["cooldown"])
	}

	if cooldown["channelsCooling"] != float64(1) {
		t.Errorf("channelsCooling = %v, want 1", cooldown["channelsCooling"])
	}
	if cooldown["channelsWithFailures"] != float64(2) {
		t.Errorf("channelsWithFailures = %v, want 2", cooldown["channelsWithFailures"])
	}
	if cooldown["channelsRecentlyFailed"] != float64(2) {
		t.Errorf("channelsRecentlyFailed = %v, want 2", cooldown["channelsRecentlyFailed"])
	}

	cooling, ok := cooldown["cooling"].([]any)
	if !ok {
		t.Fatalf("expected cooling array, got %T", cooldown["cooling"])
	}
	if len(cooling) != 1 {
		t.Fatalf("cooling length = %d, want 1", len(cooling))
	}

	entry, ok := cooling[0].(map[string]any)
	if !ok {
		t.Fatalf("expected cooling entry map, got %T", cooling[0])
	}
	if entry["failCount"] != float64(3) {
		t.Errorf("cooling entry failCount = %v, want 3", entry["failCount"])
	}
	if entry["siteName"] != "cooldown-site" {
		t.Errorf("cooling entry siteName = %v, want cooldown-site", entry["siteName"])
	}
}

func TestMonitorHealth_RuntimeHealthPresent(t *testing.T) {
	_, r, _ := setupMonitorHealthTest(t)

	resp := doHealthGet(t, r)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var result map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	runtimeHealth, ok := result["runtimeHealth"].(map[string]any)
	if !ok {
		t.Fatalf("expected runtimeHealth map, got %T", result["runtimeHealth"])
	}
	// In a fresh test with no runtime breaker state, all counts should be zero.
	if runtimeHealth["sitesTracked"] != float64(0) {
		t.Errorf("runtimeHealth sitesTracked = %v, want 0", runtimeHealth["sitesTracked"])
	}
	if runtimeHealth["sitesBreakerOpen"] != float64(0) {
		t.Errorf("runtimeHealth sitesBreakerOpen = %v, want 0", runtimeHealth["sitesBreakerOpen"])
	}
	if runtimeHealth["modelsTracked"] != float64(0) {
		t.Errorf("runtimeHealth modelsTracked = %v, want 0", runtimeHealth["modelsTracked"])
	}
	if runtimeHealth["modelsBreakerOpen"] != float64(0) {
		t.Errorf("runtimeHealth modelsBreakerOpen = %v, want 0", runtimeHealth["modelsBreakerOpen"])
	}
}

func TestMonitorHealth_ExpiredCooldownNotInCoolingList(t *testing.T) {
	db, r, _ := setupMonitorHealthTest(t)

	siteID := insertTestSite(t, db, "expired-cooldown-site", "active")
	accountID := insertTestAccount(t, db, siteID, "expired-user", "active")

	// Channel with past cooldown (already expired) but still has failures.
	pastCooldown := time.Now().Add(-30 * time.Minute).UTC().Format(time.RFC3339)
	recentFail := time.Now().Add(-1 * time.Minute).UTC().Format(time.RFC3339)
	insertTestRouteChannel(t, db, accountID, 2, &recentFail, &pastCooldown)

	resp := doHealthGet(t, r)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var result map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	cooldown, ok := result["cooldown"].(map[string]any)
	if !ok {
		t.Fatalf("expected cooldown map, got %T", result["cooldown"])
	}

	// Expired cooldown should not count as cooling.
	if cooldown["channelsCooling"] != float64(0) {
		t.Errorf("channelsCooling = %v, want 0 (expired)", cooldown["channelsCooling"])
	}
	// But the channel still has failures, so it should be counted.
	if cooldown["channelsWithFailures"] != float64(1) {
		t.Errorf("channelsWithFailures = %v, want 1", cooldown["channelsWithFailures"])
	}

	cooling, ok := cooldown["cooling"].([]any)
	if !ok {
		t.Fatalf("expected cooling array, got %T", cooldown["cooling"])
	}
	if len(cooling) != 0 {
		t.Fatalf("cooling length = %d, want 0 (expired cooldown)", len(cooling))
	}
}
