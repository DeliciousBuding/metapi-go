package balance

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
)

// ---- IsSiteDisabled Tests ----

func TestIsSiteDisabled(t *testing.T) {
	tests := []struct {
		status string
		want   bool
	}{
		{"disabled", true},
		{" disabled ", true},
		{"active", false},
		{"", false},
		{"  ", false},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			got := IsSiteDisabled(tt.status)
			if got != tt.want {
				t.Errorf("IsSiteDisabled(%q) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestRefreshBalance_UsesSiteCustomHeaders(t *testing.T) {
	headerSeen := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/user/self" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("X-Metapi-Site") == "site-header" {
			headerSeen = true
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"id":         42,
				"username":   "anyrouter-user",
				"quota":      1_000_000,
				"used_quota": 250_000,
			},
		})
	}))
	defer server.Close()

	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	siteRes, err := db.Exec(
		"INSERT INTO sites (name, url, platform, custom_headers, status, created_at, updated_at) VALUES (?, ?, ?, ?, 'active', ?, ?)",
		"AnyRouter balance headers", server.URL, "anyrouter", `{"X-Metapi-Site":"site-header"}`, now, now,
	)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	siteID, err := siteRes.LastInsertId()
	if err != nil {
		t.Fatalf("site LastInsertId: %v", err)
	}
	accountRes, err := db.Exec(
		"INSERT INTO accounts (site_id, username, access_token, status, checkin_enabled, created_at, updated_at) VALUES (?, ?, ?, 'active', ?, ?, ?)",
		siteID, "anyrouter-user", "session-token", true, now, now,
	)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	accountID, err := accountRes.LastInsertId()
	if err != nil {
		t.Fatalf("account LastInsertId: %v", err)
	}

	result, err := RefreshBalance(&config.Config{}, db.DB, accountID)
	if err != nil {
		t.Fatalf("RefreshBalance: %v", err)
	}
	if result == nil || result.Balance != 2 || result.Used != 0.5 || result.Quota != 2.5 {
		t.Fatalf("balance result = %+v, want (2,0.5,2.5)", result)
	}
	if !headerSeen {
		t.Fatal("site custom header was not sent to /api/user/self")
	}
}

func TestRefreshBalance_DisabledAnyRouterAccountIsSkippedWithoutUpstream(t *testing.T) {
	upstreamCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		http.Error(w, "unexpected upstream call", http.StatusInternalServerError)
	}))
	defer server.Close()

	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	siteRes, err := db.Exec(
		"INSERT INTO sites (name, url, platform, status, created_at, updated_at) VALUES (?, ?, ?, 'active', ?, ?)",
		"AnyRouter disabled balance", server.URL, "anyrouter", now, now,
	)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	siteID, err := siteRes.LastInsertId()
	if err != nil {
		t.Fatalf("site LastInsertId: %v", err)
	}
	accountRes, err := db.Exec(
		"INSERT INTO accounts (site_id, username, access_token, status, checkin_enabled, balance, balance_used, quota, created_at, updated_at) VALUES (?, ?, ?, 'disabled', ?, ?, ?, ?, ?, ?)",
		siteID, "anyrouter-user", "session-token", true, 7.0, 2.0, 9.0, now, now,
	)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	accountID, err := accountRes.LastInsertId()
	if err != nil {
		t.Fatalf("account LastInsertId: %v", err)
	}
	if _, err := db.Exec("UPDATE accounts SET status = 'DISABLED' WHERE id = ?", accountID); err != nil {
		t.Fatalf("upper-case disable account: %v", err)
	}

	result, err := RefreshBalance(&config.Config{}, db.DB, accountID)
	if err != nil {
		t.Fatalf("RefreshBalance: %v", err)
	}
	if result == nil || !result.Skipped || result.Reason != "account_disabled" {
		t.Fatalf("balance result = %+v, want account_disabled skip", result)
	}
	if result.Balance != 7.0 || result.Used != 2.0 || result.Quota != 9.0 {
		t.Fatalf("balance fields = (%v,%v,%v), want original (7,2,9)", result.Balance, result.Used, result.Quota)
	}
	if upstreamCalls != 0 {
		t.Fatalf("upstreamCalls = %d, want 0", upstreamCalls)
	}

	var extraConfig *string
	if err := db.Get(&extraConfig, "SELECT extra_config FROM accounts WHERE id = ?", accountID); err != nil {
		t.Fatalf("read extra_config: %v", err)
	}
	if extraConfig == nil {
		t.Fatal("extra_config is nil; want runtimeHealth")
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(*extraConfig), &cfg); err != nil {
		t.Fatalf("unmarshal extra_config: %v", err)
	}
	health, ok := cfg["runtimeHealth"].(map[string]any)
	if !ok || health["state"] != "disabled" || health["source"] != "balance" {
		t.Fatalf("runtimeHealth = %#v, want disabled/balance", cfg["runtimeHealth"])
	}
}

func TestRefreshBalance_LegacyMirroredAPIKeyAccountIsSkippedWithoutUpstream(t *testing.T) {
	upstreamCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		http.Error(w, "unexpected upstream call", http.StatusInternalServerError)
	}))
	defer server.Close()

	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	siteRes, err := db.Exec(
		"INSERT INTO sites (name, url, platform, status, created_at, updated_at) VALUES (?, ?, ?, 'active', ?, ?)",
		"AnyRouter legacy api key balance", server.URL, "anyrouter", now, now,
	)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	siteID, err := siteRes.LastInsertId()
	if err != nil {
		t.Fatalf("site LastInsertId: %v", err)
	}
	accountRes, err := db.Exec(
		"INSERT INTO accounts (site_id, username, access_token, api_token, status, checkin_enabled, balance, balance_used, quota, created_at, updated_at) VALUES (?, ?, ?, ?, 'active', ?, ?, ?, ?, ?, ?)",
		siteID, nil, "legacy-api-key", "legacy-api-key", true, 3.0, 1.0, 4.0, now, now,
	)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	accountID, err := accountRes.LastInsertId()
	if err != nil {
		t.Fatalf("account LastInsertId: %v", err)
	}

	result, err := RefreshBalance(&config.Config{}, db.DB, accountID)
	if err != nil {
		t.Fatalf("RefreshBalance: %v", err)
	}
	if result == nil || !result.Skipped || result.Reason != "proxy_only" {
		t.Fatalf("balance result = %+v, want proxy_only skip", result)
	}
	if result.Balance != 3.0 || result.Used != 1.0 || result.Quota != 4.0 {
		t.Fatalf("balance fields = (%v,%v,%v), want original (3,1,4)", result.Balance, result.Used, result.Quota)
	}
	if upstreamCalls != 0 {
		t.Fatalf("upstreamCalls = %d, want 0", upstreamCalls)
	}
}

// ---- shouldAttemptAutoReloginBalance Tests ----
// Extra patterns beyond checkin version: unauthorized, forbidden, not login, not logged

func TestShouldAttemptAutoReloginBalance_Positive(t *testing.T) {
	// Same as checkin version
	checkinCases := []string{
		"jwt expired",
		"token expired",
		"invalid access token",
		"new-api-user header required",
		"access token is missing",
	}
	// Extra patterns for balance
	balanceExtras := []string{
		"unauthorized",
		"forbidden",
		"not login",
		"not logged in",
		"user not logged in",
	}

	allCases := append(checkinCases, balanceExtras...)
	for _, msg := range allCases {
		t.Run(msg, func(t *testing.T) {
			if !shouldAttemptAutoReloginBalance(msg) {
				t.Errorf("expected true for: %q", msg)
			}
		})
	}
}

func TestShouldAttemptAutoReloginBalance_Negative(t *testing.T) {
	negativeCases := []string{
		"",
		"some random error",
		"connection timeout",
		"server error",
		"not found",
	}
	for _, msg := range negativeCases {
		t.Run(msg, func(t *testing.T) {
			if shouldAttemptAutoReloginBalance(msg) {
				t.Errorf("expected false for: %q", msg)
			}
		})
	}
}

func TestShouldAttemptAutoReloginBalance_BalanceExtraPatterns(t *testing.T) {
	// These should ONLY match in balance version, not checkin version
	// "unauthorized" → true in balance
	if !shouldAttemptAutoReloginBalance("unauthorized") {
		t.Error("expected true for 'unauthorized' in balance version")
	}
	// "forbidden" → true in balance
	if !shouldAttemptAutoReloginBalance("forbidden") {
		t.Error("expected true for 'forbidden' in balance version")
	}
	// "not login" → true in balance
	if !shouldAttemptAutoReloginBalance("not login") {
		t.Error("expected true for 'not login' in balance version")
	}
	// "not logged" → true in balance
	if !shouldAttemptAutoReloginBalance("not logged") {
		t.Error("expected true for 'not logged' in balance version")
	}
}

func TestSub2APIRefreshPromiseBroadcastsResultToAllWaiters(t *testing.T) {
	p := newSub2APIRefreshPromise()

	done := make(chan sub2apiRefreshResult, 2)
	go func() { done <- p.wait() }()
	go func() { done <- p.wait() }()

	want := sub2apiRefreshResult{accessToken: "shared-sub2api-token"}
	p.resolve(want)

	for i := 0; i < 2; i++ {
		select {
		case got := <-done:
			if got.accessToken != want.accessToken || got.err != want.err {
				t.Fatalf("waiter %d got %+v, want %+v", i, got, want)
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("waiter %d did not receive broadcast result", i)
		}
	}
}

// ---- supportsTodayIncomeLogFallback Tests ----

func TestSupportsTodayIncomeLogFallback(t *testing.T) {
	tests := []struct {
		platform string
		want     bool
	}{
		{"new-api", true},
		{"new-api", true},
		{"anyrouter", true},
		{"AnyRouter", true},
		{"one-api", true},
		{"veloera", true},
		{"Veloera", true},
		{"openai", false},
		{"anthropic", false},
		{"gemini", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.platform, func(t *testing.T) {
			got := supportsTodayIncomeLogFallback(tt.platform)
			if got != tt.want {
				t.Errorf("supportsTodayIncomeLogFallback(%q) = %v, want %v", tt.platform, got, tt.want)
			}
		})
	}
}

// ---- resolveQuotaConversionFactor Tests ----

func TestResolveQuotaConversionFactor(t *testing.T) {
	tests := []struct {
		platform string
		want     float64
	}{
		{"veloera", 1_000_000},
		{"Veloera", 1_000_000},
		{"new-api", 500_000},
		{"anyrouter", 500_000},
		{"one-api", 500_000},
		{"openai", 500_000},  // default
		{"unknown", 500_000}, // default
		{"", 500_000},        // default
	}
	for _, tt := range tests {
		t.Run(tt.platform, func(t *testing.T) {
			got := resolveQuotaConversionFactor(tt.platform)
			if got != tt.want {
				t.Errorf("resolveQuotaConversionFactor(%q) = %v, want %v", tt.platform, got, tt.want)
			}
		})
	}
}

// ---- parsePositiveNumberAny Tests ----

func TestParsePositiveNumberAny(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  float64
	}{
		{"positive float64", float64(100), 100},
		{"zero float64", float64(0), 0},
		{"negative float64", float64(-5), 0},
		{"NaN float64", math.NaN(), 0},
		{"positive string", "50", 50},
		{"zero string", "0", 0},
		{"negative string", "-10", 0},
		{"empty string", "", 0},
		{"invalid string", "abc", 0},
		{"nil", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parsePositiveNumberAny(tt.input)
			if got != tt.want {
				t.Errorf("parsePositiveNumberAny(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// ---- BalanceResult Tests ----

func TestBalanceResult_Success(t *testing.T) {
	r := BalanceResult{
		Balance: 100.5,
		Used:    50.2,
		Quota:   200,
		Skipped: false,
	}
	if r.Skipped {
		t.Error("success result should not be skipped")
	}
	if r.Balance != 100.5 {
		t.Errorf("Balance = %v, want 100.5", r.Balance)
	}
}

func TestBalanceResult_Skipped(t *testing.T) {
	r := BalanceResult{
		Balance: 100,
		Used:    50,
		Quota:   200,
		Skipped: true,
		Reason:  "site_disabled",
	}
	if !r.Skipped {
		t.Error("skipped result should have Skipped=true")
	}
	if r.Reason != "site_disabled" {
		t.Errorf("Reason = %q, want 'site_disabled'", r.Reason)
	}
}

func TestBalanceResult_APIKeyProxy(t *testing.T) {
	r := BalanceResult{
		Balance: 0,
		Used:    0,
		Quota:   0,
		Skipped: true,
		Reason:  "proxy_only",
	}
	if r.Reason != "proxy_only" {
		t.Errorf("Reason = %q, want 'proxy_only'", r.Reason)
	}
}

func TestDecodeIncomeLogPayloadRejectsOversizedResponse(t *testing.T) {
	var payload map[string]any
	err := decodeIncomeLogPayload(strings.NewReader(`{"data":[]}`+strings.Repeat(" ", incomeLogResponseBodyLimit+1)), &payload)
	if err == nil {
		t.Fatal("decodeIncomeLogPayload succeeded, want oversized response error")
	}
	if !strings.Contains(err.Error(), "income log response exceeds") {
		t.Fatalf("error = %v, want size limit", err)
	}
}

func TestDecodeIncomeLogPayloadAcceptsNormalResponse(t *testing.T) {
	var payload map[string]any
	if err := decodeIncomeLogPayload(strings.NewReader(`{"data":{"items":[{"quota":500000}]}}`), &payload); err != nil {
		t.Fatalf("decodeIncomeLogPayload: %v", err)
	}
	items := extractLogItems(payload)
	if len(items) != 1 || parsePositiveNumberAny(items[0]["quota"]) != 500000 {
		t.Fatalf("items = %#v, want one quota item", items)
	}
}

// ---- extractLogItems Tests ----

func TestExtractLogItems_DataItems(t *testing.T) {
	payload := map[string]any{
		"data": map[string]any{
			"items": []any{
				map[string]any{"quota": float64(100)},
				map[string]any{"quota": float64(200)},
			},
		},
	}
	items := extractLogItems(payload)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
}

func TestExtractLogItems_DirectItems(t *testing.T) {
	payload := map[string]any{
		"items": []any{
			map[string]any{"quota": float64(50)},
		},
	}
	items := extractLogItems(payload)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
}

func TestExtractLogItems_DataArray(t *testing.T) {
	payload := map[string]any{
		"data": []any{
			map[string]any{"quota": float64(300)},
		},
	}
	items := extractLogItems(payload)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
}

func TestExtractLogItems_Empty(t *testing.T) {
	items := extractLogItems(map[string]any{})
	if items != nil {
		t.Errorf("expected nil for empty payload, got %v", items)
	}
}

// ---- extractLogTotal Tests ----

func TestExtractLogTotal(t *testing.T) {
	tests := []struct {
		name    string
		payload map[string]any
		want    int
	}{
		{
			name:    "nested data.total as float64",
			payload: map[string]any{"data": map[string]any{"total": float64(42)}},
			want:    42,
		},
		{
			name:    "direct total as float64",
			payload: map[string]any{"total": float64(100)},
			want:    100,
		},
		{
			name:    "total as string",
			payload: map[string]any{"total": "50"},
			want:    50,
		},
		{
			name:    "no total",
			payload: map[string]any{},
			want:    -1, // returns nil, we check for nil
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractLogTotal(tt.payload)
			if tt.want == -1 {
				if got != nil {
					t.Errorf("expected nil, got %v", *got)
				}
				return
			}
			if got == nil {
				t.Errorf("expected %d, got nil", tt.want)
				return
			}
			if *got != tt.want {
				t.Errorf("extractLogTotal() = %d, want %d", *got, tt.want)
			}
		})
	}
}

// ---- parseIncomeFromContent Tests ----

func TestParseIncomeFromContent(t *testing.T) {
	// Current implementation is simplified; tests verify it doesn't panic
	_ = parseIncomeFromContent("some content with 100")
	_ = parseIncomeFromContent("")
	_ = parseIncomeFromContent("no numbers here")
}

// ---- A1: balance_history snapshot Tests ----

// TestRecordBalanceSnapshot_UPSERT verifies that RefreshBalance's success
// path writes a balance_history row, and a same-day re-refresh overwrites it
// (latest-known balance of the day, not a duplicate row).
func TestRecordBalanceSnapshot_UPSERT(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/user/self" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"id":         7,
				"username":   "snap-user",
				"quota":      1_000_000,
				"used_quota": 250_000,
			},
		})
	}))
	defer server.Close()

	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	siteRes, err := db.Exec(
		"INSERT INTO sites (name, url, platform, status, created_at, updated_at) VALUES (?, ?, ?, 'active', ?, ?)",
		"Snap Test", server.URL, "new-api", now, now,
	)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	siteID, _ := siteRes.LastInsertId()
	accRes, err := db.Exec(
		"INSERT INTO accounts (site_id, username, access_token, status, checkin_enabled, created_at, updated_at) VALUES (?, ?, ?, 'active', ?, ?, ?)",
		siteID, "snap-user", "tok", true, now, now,
	)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	accountID, _ := accRes.LastInsertId()

	// First refresh → one snapshot row.
	if _, err := RefreshBalance(&config.Config{}, db.DB, accountID); err != nil {
		t.Fatalf("first RefreshBalance: %v", err)
	}
	count1 := countBalanceHistoryRows(t, db.DB, accountID)
	if count1 != 1 {
		t.Fatalf("after first refresh: balance_history rows = %d, want 1", count1)
	}

	// Second refresh same day → UPSERT overwrites, still one row.
	if _, err := RefreshBalance(&config.Config{}, db.DB, accountID); err != nil {
		t.Fatalf("second RefreshBalance: %v", err)
	}
	count2 := countBalanceHistoryRows(t, db.DB, accountID)
	if count2 != 1 {
		t.Fatalf("after second refresh: balance_history rows = %d, want 1 (UPSERT)", count2)
	}
}

func countBalanceHistoryRows(t *testing.T, db *sqlx.DB, accountID int64) int {
	t.Helper()
	var n int
	if err := db.Get(&n, "SELECT COUNT(*) FROM balance_history WHERE account_id = ?", accountID); err != nil {
		t.Fatalf("count balance_history: %v", err)
	}
	return n
}

// ---- fetchTodayIncomeFromLogs Tests (P2: fallback paths) ----
//
// fetchTodayIncomeFromLogs is the income-fallback path used when
// GetBalance's response lacks today_income. It iterates log types [1,4]
// and pages 1-6, parsing quota and content fields. These tests use
// httptest.Server — no real network calls.

func TestFetchTodayIncomeFromLogs_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/log/self" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query()
		logType := q.Get("type")
		page := q.Get("p")

		// Type 1, page 1: two items — one quota-based, one content-based.
		if logType == "1" && page == "1" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"items": []any{
						map[string]any{"quota": float64(1_000_000)},
						map[string]any{"content": "10.5"},
					},
					"total": float64(2),
				},
			})
			return
		}
		// All other log types / pages: empty items.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"items": []any{},
				"total": float64(0),
			},
		})
	}))
	defer server.Close()

	// Platform "veloera" → resolveQuotaConversionFactor = 1_000_000.
	// 1_000_000/1_000_000 = 1.0 (quota) + 10.5 (content) = 11.5.
	income, err := fetchTodayIncomeFromLogs(server.URL, "test-token", "veloera", 0, nil)
	if err != nil {
		t.Fatalf("fetchTodayIncomeFromLogs: %v", err)
	}
	if income != 11.5 {
		t.Errorf("income = %v, want 11.5", income)
	}
}

func TestFetchTodayIncomeFromLogs_Non2xxResponseSurfacesError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer server.Close()

	income, err := fetchTodayIncomeFromLogs(server.URL, "test-token", "veloera", 0, nil)
	if err == nil {
		t.Fatal("expected error for non-2xx response, got nil")
	}
	if income != 0 {
		t.Errorf("income = %v, want 0 on error", income)
	}
	if !strings.Contains(err.Error(), "no log responses") {
		t.Errorf("error = %v, want 'no log responses'", err)
	}
}

func TestFetchTodayIncomeFromLogs_MalformedJSONSurfacesError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Invalid JSON — opening brace without closing.
		_, _ = w.Write([]byte(`{invalid json`))
	}))
	defer server.Close()

	income, err := fetchTodayIncomeFromLogs(server.URL, "test-token", "veloera", 0, nil)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
	if income != 0 {
		t.Errorf("income = %v, want 0 on error", income)
	}
	// decodeIncomeLogPayload fails → break → hasAnyResponse=false → "no log responses".
	if !strings.Contains(err.Error(), "no log responses") {
		t.Errorf("error = %v, want 'no log responses'", err)
	}
}

func TestFetchTodayIncomeFromLogs_OversizedBodyHandledGracefully(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Body exceeds incomeLogResponseBodyLimit (1 MiB). The LimitedReader
		// in decodeIncomeLogPayload will cap and reject.
		_, _ = w.Write([]byte(`{"data":{"items":[` + strings.Repeat(",", incomeLogResponseBodyLimit+1) + `]}}`))
	}))
	defer server.Close()

	income, err := fetchTodayIncomeFromLogs(server.URL, "test-token", "veloera", 0, nil)
	if err == nil {
		t.Fatal("expected error for oversized body, got nil")
	}
	if income != 0 {
		t.Errorf("income = %v, want 0 on error", income)
	}
	// decodeIncomeLogPayload returns a size-limit error → break → "no log responses".
	if !strings.Contains(err.Error(), "no log responses") {
		t.Errorf("error = %v, want 'no log responses'", err)
	}
}

func TestFetchTodayIncomeFromLogs_EmptyBaseURL(t *testing.T) {
	_, err := fetchTodayIncomeFromLogs("", "token", "veloera", 0, nil)
	if err == nil {
		t.Fatal("expected error for empty baseURL")
	}
	if !strings.Contains(err.Error(), "empty baseURL or accessToken") {
		t.Errorf("error = %v, want 'empty baseURL or accessToken'", err)
	}
}

func TestFetchTodayIncomeFromLogs_EmptyAccessToken(t *testing.T) {
	_, err := fetchTodayIncomeFromLogs("http://example.com", "", "veloera", 0, nil)
	if err == nil {
		t.Fatal("expected error for empty accessToken")
	}
	if !strings.Contains(err.Error(), "empty baseURL or accessToken") {
		t.Errorf("error = %v, want 'empty baseURL or accessToken'", err)
	}
}

func TestFetchTodayIncomeFromLogs_VeloeraConversionFactor(t *testing.T) {
	// Veloera platform uses 1_000_000 as the conversion factor.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Type 1 page 1: one item with quota=1_000_000.
		if r.URL.Query().Get("type") == "1" && r.URL.Query().Get("p") == "1" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"items": []any{
						map[string]any{"quota": float64(1_000_000)},
					},
					"total": float64(1),
				},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"items": []any{}, "total": float64(0)},
		})
	}))
	defer server.Close()

	// 1_000_000 / 1_000_000 = 1.0.
	income, err := fetchTodayIncomeFromLogs(server.URL, "tok", "veloera", 0, nil)
	if err != nil {
		t.Fatalf("fetchTodayIncomeFromLogs: %v", err)
	}
	if income != 1.0 {
		t.Errorf("income = %v, want 1.0 (veloera factor 1_000_000)", income)
	}
}

// ---- extractLogItems edge shapes (P2) ----
//
// The existing tests cover data.items, direct items, data array, and empty
// payload. These add: empty array in data.items, nil payload, items with
// missing fields, items with extra fields, and non-map items being skipped.

func TestExtractLogItems_EmptyArrayInDataItems(t *testing.T) {
	payload := map[string]any{
		"data": map[string]any{
			"items": []any{},
			"total": float64(0),
		},
	}
	items := extractLogItems(payload)
	// Should be a non-nil empty slice (make with cap 0), not nil.
	if items == nil {
		t.Fatal("expected non-nil empty slice for empty data.items array")
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestExtractLogItems_NilPayload(t *testing.T) {
	items := extractLogItems(nil)
	if items != nil {
		t.Errorf("expected nil for nil payload, got %v", items)
	}
}

func TestExtractLogItems_MissingFieldsReturned(t *testing.T) {
	// Items without quota or content — they are returned as-is; income
	// processing happens downstream and handles missing fields gracefully.
	payload := map[string]any{
		"data": map[string]any{
			"items": []any{
				map[string]any{"id": float64(1)},
				map[string]any{"model": "gpt-4"},
			},
		},
	}
	items := extractLogItems(payload)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if _, ok := items[0]["id"]; !ok {
		t.Error("missing 'id' field in first item")
	}
}

func TestExtractLogItems_ExtraFieldsPreserved(t *testing.T) {
	payload := map[string]any{
		"data": map[string]any{
			"items": []any{
				map[string]any{
					"quota":   float64(500000),
					"content": "10.5",
					"extra1":  "ignored",
					"extra2":  float64(42),
				},
			},
		},
	}
	items := extractLogItems(payload)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0]["quota"] != float64(500000) {
		t.Errorf("quota = %v, want 500000", items[0]["quota"])
	}
	if items[0]["extra1"] != "ignored" {
		t.Errorf("extra1 = %v, want 'ignored'", items[0]["extra1"])
	}
}

func TestExtractLogItems_NonMapItemsSkipped(t *testing.T) {
	// Non-map items (strings, numbers) in the items array must be silently
	// skipped — only map[string]any entries are returned.
	payload := map[string]any{
		"data": map[string]any{
			"items": []any{
				"string-item",
				float64(42),
				map[string]any{"quota": float64(100)},
			},
		},
	}
	items := extractLogItems(payload)
	if len(items) != 1 {
		t.Fatalf("expected 1 item (non-maps skipped), got %d", len(items))
	}
	if items[0]["quota"] != float64(100) {
		t.Errorf("quota = %v, want 100", items[0]["quota"])
	}
}

// ---- RefreshAllBalances Tests (P2: aggregation + error collection) ----
//
// RefreshAllBalances queries all active accounts and refreshes them
// concurrently. Errors from individual accounts must be collected (nil
// balance) rather than aborting the whole pass.

func TestRefreshAllBalances_ErrorsCollectedNotFatal(t *testing.T) {
	// Success server factory: returns valid /api/user/self balance.
	// Each success site needs its own httptest.Server URL because the sites
	// table has a UNIQUE(platform, url) constraint.
	newSuccessServer := func() *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Path != "/api/user/self" {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"id":         float64(1),
					"username":   "success-user",
					"quota":      float64(1_000_000),
					"used_quota": float64(250_000),
				},
			})
		}))
	}
	successServer1 := newSuccessServer()
	defer successServer1.Close()
	successServer3 := newSuccessServer()
	defer successServer3.Close()

	// Error server: returns 500 for all paths. The NewApiAdapter exhausts
	// all fallback paths and returns an error — RefreshBalance propagates it.
	errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer errorServer.Close()

	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Three active sites: success, error, success (different URLs).
	// Using "new-api" platform: NewApiAdapter returns nil+error on 500
	// (unlike VeloeraAdapter which returns &BalanceInfo{}, nil on error).
	site1Res, err := db.Exec(
		"INSERT INTO sites (name, url, platform, status, created_at, updated_at) VALUES (?, ?, ?, 'active', ?, ?)",
		"success-site-1", successServer1.URL, "new-api", now, now,
	)
	if err != nil {
		t.Fatalf("insert site 1: %v", err)
	}
	site1ID, _ := site1Res.LastInsertId()

	site2Res, err := db.Exec(
		"INSERT INTO sites (name, url, platform, status, created_at, updated_at) VALUES (?, ?, ?, 'active', ?, ?)",
		"error-site-2", errorServer.URL, "new-api", now, now,
	)
	if err != nil {
		t.Fatalf("insert site 2: %v", err)
	}
	site2ID, _ := site2Res.LastInsertId()

	site3Res, err := db.Exec(
		"INSERT INTO sites (name, url, platform, status, created_at, updated_at) VALUES (?, ?, ?, 'active', ?, ?)",
		"success-site-3", successServer3.URL, "new-api", now, now,
	)
	if err != nil {
		t.Fatalf("insert site 3: %v", err)
	}
	site3ID, _ := site3Res.LastInsertId()

	// Three active accounts, one per site.
	acc1Res, err := db.Exec(
		"INSERT INTO accounts (site_id, username, access_token, status, checkin_enabled, created_at, updated_at) VALUES (?, ?, ?, 'active', ?, ?, ?)",
		site1ID, "user1", "tok1", true, now, now,
	)
	if err != nil {
		t.Fatalf("insert account 1: %v", err)
	}
	acc1ID, _ := acc1Res.LastInsertId()

	acc2Res, err := db.Exec(
		"INSERT INTO accounts (site_id, username, access_token, status, checkin_enabled, created_at, updated_at) VALUES (?, ?, ?, 'active', ?, ?, ?)",
		site2ID, "user2", "tok2", true, now, now,
	)
	if err != nil {
		t.Fatalf("insert account 2: %v", err)
	}
	acc2ID, _ := acc2Res.LastInsertId()

	acc3Res, err := db.Exec(
		"INSERT INTO accounts (site_id, username, access_token, status, checkin_enabled, created_at, updated_at) VALUES (?, ?, ?, 'active', ?, ?, ?)",
		site3ID, "user3", "tok3", true, now, now,
	)
	if err != nil {
		t.Fatalf("insert account 3: %v", err)
	}
	acc3ID, _ := acc3Res.LastInsertId()

	results := RefreshAllBalances(&config.Config{}, db.DB)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	balanceByID := make(map[int64]*float64, len(results))
	for _, r := range results {
		balanceByID[r.AccountID] = r.Balance
	}

	// Account 1 (success server): non-nil balance = 2.0.
	b1, ok := balanceByID[acc1ID]
	if !ok {
		t.Fatalf("account %d missing from results", acc1ID)
	}
	if b1 == nil {
		t.Errorf("account %d balance is nil, want non-nil (success path)", acc1ID)
	} else if *b1 != 2.0 {
		t.Errorf("account %d balance = %v, want 2.0", acc1ID, *b1)
	}

	// Account 2 (error server): nil balance (error collected, not fatal).
	b2, ok := balanceByID[acc2ID]
	if !ok {
		t.Fatalf("account %d missing from results", acc2ID)
	}
	if b2 != nil {
		t.Errorf("account %d balance = %v, want nil (error path)", acc2ID, *b2)
	}

	// Account 3 (success server): non-nil balance = 2.0.
	b3, ok := balanceByID[acc3ID]
	if !ok {
		t.Fatalf("account %d missing from results", acc3ID)
	}
	if b3 == nil {
		t.Errorf("account %d balance is nil, want non-nil (success path)", acc3ID)
	} else if *b3 != 2.0 {
		t.Errorf("account %d balance = %v, want 2.0", acc3ID, *b3)
	}
}

func TestRefreshAllBalances_EmptyDBReturnsNil(t *testing.T) {
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	results := RefreshAllBalances(&config.Config{}, db.DB)
	// No active accounts → nil or empty slice.
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty DB, got %d", len(results))
	}
}

// ---- RefreshBalance site-disabled skip ----

func TestRefreshBalance_DisabledSiteIsSkippedWithoutUpstream(t *testing.T) {
	upstreamCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalls++
		http.Error(w, "unexpected upstream call", http.StatusInternalServerError)
	}))
	defer server.Close()

	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	siteRes, err := db.Exec(
		"INSERT INTO sites (name, url, platform, status, created_at, updated_at) VALUES (?, ?, ?, 'disabled', ?, ?)",
		"Disabled site balance", server.URL, "veloera", now, now,
	)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	siteID, _ := siteRes.LastInsertId()
	accRes, err := db.Exec(
		"INSERT INTO accounts (site_id, username, access_token, status, checkin_enabled, balance, balance_used, quota, created_at, updated_at) VALUES (?, ?, ?, 'active', ?, ?, ?, ?, ?, ?)",
		siteID, "disabled-site-user", "tok", true, 5.0, 1.5, 6.5, now, now,
	)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	accountID, _ := accRes.LastInsertId()

	result, err := RefreshBalance(&config.Config{}, db.DB, accountID)
	if err != nil {
		t.Fatalf("RefreshBalance: %v", err)
	}
	if result == nil || !result.Skipped || result.Reason != "site_disabled" {
		t.Fatalf("balance result = %+v, want site_disabled skip", result)
	}
	if result.Balance != 5.0 || result.Used != 1.5 || result.Quota != 6.5 {
		t.Errorf("balance fields = (%v,%v,%v), want original (5,1.5,6.5)", result.Balance, result.Used, result.Quota)
	}
	if upstreamCalls != 0 {
		t.Fatalf("upstreamCalls = %d, want 0 (site disabled)", upstreamCalls)
	}
}

// ---- parseIncomeFromContent additional edge cases ----

func TestParseIncomeFromContent_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    float64
	}{
		{"decimal value", "10.5", 10.5},
		{"integer value", "100", 100},
		{"zero", "0", 0},
		{"negative sign stripped by regex", "-5", 5}, // regex \d+ matches "5" ignoring "-"
		{"empty", "", 0},
		{"no numbers", "no numbers here", 0},
		{"with commas", "1,234.56", 1234.56},
		{"embedded in text", "income 42.5 usd", 42.5},
		{"inf value", "inf", 0},
		{"multiple numbers picks first", "10 20 30", 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseIncomeFromContent(tt.input)
			if got != tt.want {
				t.Errorf("parseIncomeFromContent(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// ---- platformUserIDPtr Tests ----

func TestPlatformUserIDPtr(t *testing.T) {
	if got := platformUserIDPtr(0); got != nil {
		t.Errorf("platformUserIDPtr(0) = %v, want nil", got)
	}
	if got := platformUserIDPtr(-1); got != nil {
		t.Errorf("platformUserIDPtr(-1) = %v, want nil", got)
	}
	got := platformUserIDPtr(42)
	if got == nil {
		t.Fatal("platformUserIDPtr(42) = nil, want &42")
	}
	if *got != 42 {
		t.Errorf("platformUserIDPtr(42) = &%d, want &42", *got)
	}
}

// ---- recordBalanceSnapshot nil-db guard ----

func TestRecordBalanceSnapshot_NilDBDoesNotPanic(t *testing.T) {
	recordBalanceSnapshot(nil, 1, 10.0, 2.0, 8.0)
}

// TestExplainRefreshFailure locks the operator-facing wording of a balance
// refresh failure (#1210) and the order it is decided in: the shapes the balance
// path owns are named here, everything upstream is delegated to
// platform.ExplainUpstreamFailure, and a failure that cannot be named safely
// yields "" so the caller keeps its bare stable prefix instead of inventing one.
func TestExplainRefreshFailure(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"nil says nothing", nil, ""},
		{"observed sub2api expiry", errors.New("sub2api /api/v1/auth/me: HTTP 401: Token has expired"), "upstream rejected the credential (HTTP 401)"},
		{"adapter got no balance", errors.New("failed to fetch balance"), "upstream returned no usable balance"},
		{"unsupported platform belongs to the 404", errors.New("unsupported platform: nosuch"), ""},
		{"local read failure outranks the transport wording", fmt.Errorf("%s%w", errReadAccountPrefix, errors.New("connection refused")), "the account could not be read from the database"},
		{"local save failure", fmt.Errorf("%s%w", errSaveBalancePrefix, errors.New("disk full")), "the refreshed balance could not be saved"},
		{"upstream unreachable", errors.New("request: dial tcp 127.0.0.1:1: connect: connection refused"), "upstream refused the connection"},
		{"unnameable keeps the bare prefix", errors.New("something odd"), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExplainRefreshFailure(tc.err); got != tc.want {
				t.Fatalf("ExplainRefreshFailure(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}

// TestRefreshBalanceReadFailureIsNotReportedAsMissing is the Law-3 half: a
// missing row is a legitimate "not found" (nil, nil, and the caller answers 404),
// while any other read failure must surface as an error. Collapsing the two is
// what made a dead chain look configured (#1179).
func TestRefreshBalanceReadFailureIsNotReportedAsMissing(t *testing.T) {
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	// A row that is genuinely absent is a legitimate "not found": (nil, nil), and
	// the handler answers 404 for it.
	result, err := RefreshBalance(&config.Config{}, db.DB, 999999)
	if result != nil || err != nil {
		t.Fatalf("missing account = (%v, %v), want (nil, nil)", result, err)
	}

	// A read that fails is a different statement. With the database closed the
	// lookup cannot succeed, and answering "account not found" for it is the
	// [] + nil disease: a failure made indistinguishable from a valid empty
	// answer, which is exactly what #1179 turned into a product rule.
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
	result, err = RefreshBalance(&config.Config{}, db.DB, 1)
	if err == nil {
		t.Fatalf("read failure = (%v, nil), want an error", result)
	}
	if got := ExplainRefreshFailure(err); got != "the account could not be read from the database" {
		t.Fatalf("ExplainRefreshFailure(%v) = %q, want the local read reason", err, got)
	}
}
