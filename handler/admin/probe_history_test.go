package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

// setupProbeHistoryHarness wires the two probe-history endpoints onto a fresh
// in-memory sqlite DB. The probe scheduler is the production writer; tests
// insert model_probe_results rows directly.
func setupProbeHistoryHarness(t *testing.T) (*store.DB, chi.Router) {
	t.Helper()
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	r := chi.NewRouter()
	channels := &tokenRoutesHandler{db: db.DB}
	accounts := &accountsHandler{db: db.DB}
	r.Get("/api/channels/probe-history", channels.channelProbeHistory)
	r.Get("/api/accounts/probe-history", accounts.accountProbeHistory)
	return db, r
}

// seedProbeHistoryFixtures creates one site, two accounts, one route and two
// route channels, mirroring insertHarnessFixtures shapes.
func seedProbeHistoryFixtures(t *testing.T, db *store.DB) (account1, account2, channel1, channel2 int64) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)

	res, err := db.Exec(`INSERT INTO sites (name, url, platform, status, created_at, updated_at)
		VALUES ('ProbeSite', 'https://upstream.example.test', 'openai', 'active', ?, ?)`, now, now)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	siteID, _ := res.LastInsertId()

	insertAccount := func(username string) int64 {
		res, err := db.Exec(`INSERT INTO accounts (site_id, username, access_token, api_token, status, checkin_enabled, created_at, updated_at)
			VALUES (?, ?, 'session-token', 'sk-probe-api-token', 'active', FALSE, ?, ?)`, siteID, username, now, now)
		if err != nil {
			t.Fatalf("insert account: %v", err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	account1 = insertAccount("probe-user-1")
	account2 = insertAccount("probe-user-2")

	res, err = db.Exec(`INSERT INTO token_routes (model_pattern, display_name, route_mode, routing_strategy, enabled, created_at, updated_at)
		VALUES ('gpt-*', 'Probe Route', 'standard', 'weighted', TRUE, ?, ?)`, now, now)
	if err != nil {
		t.Fatalf("insert route: %v", err)
	}
	routeID, _ := res.LastInsertId()

	insertChannel := func(accountID int64, model string) int64 {
		res, err := db.Exec(`INSERT INTO route_channels (route_id, account_id, source_model, priority, weight, enabled)
			VALUES (?, ?, ?, 10, 10, TRUE)`, routeID, accountID, model)
		if err != nil {
			t.Fatalf("insert channel: %v", err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	channel1 = insertChannel(account1, "gpt-4o")
	channel2 = insertChannel(account2, "gpt-4o-mini")
	return account1, account2, channel1, channel2
}

// insertProbeResult appends one model_probe_results row; id order is the
// recency order the endpoints must respect (newest id first).
func insertProbeResult(t *testing.T, db *store.DB, channelID any, accountID, siteID int64, model, status string, latencyMs any, httpStatus any, errorText any) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(`INSERT INTO model_probe_results
		(channel_id, account_id, site_id, model_name, status, latency_ms, http_status, error_text, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		channelID, accountID, siteID, model, status, latencyMs, httpStatus, errorText, now)
	if err != nil {
		t.Fatalf("insert probe result: %v", err)
	}
}

func decodeProbeHistoryEnvelope(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var envelope map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	return envelope
}

func TestProbeHistory_EmptyDB(t *testing.T) {
	_, r := setupProbeHistoryHarness(t)

	for _, path := range []string{"/api/channels/probe-history", "/api/accounts/probe-history"} {
		envelope := decodeProbeHistoryEnvelope(t, doGet(t, r, path))
		if envelope["limit"] != float64(probeHistoryDefaultLimit) {
			t.Fatalf("%s: expected default limit %d, got %v", path, probeHistoryDefaultLimit, envelope["limit"])
		}
		items, ok := envelope["items"].([]any)
		if !ok || len(items) != 0 {
			t.Fatalf("%s: expected empty items array, got %v", path, envelope["items"])
		}
	}
}

func TestProbeHistory_ChannelsGroupingOrderAndLimit(t *testing.T) {
	db, r := setupProbeHistoryHarness(t)
	account1, account2, channel1, channel2 := seedProbeHistoryFixtures(t, db)

	// channel1: three results (oldest → newest): success, failure, success.
	insertProbeResult(t, db, channel1, account1, 1, "gpt-4o", "success", 120.5, 200, nil)
	insertProbeResult(t, db, channel1, account1, 1, "gpt-4o", "failure", nil, 500, "upstream 500")
	insertProbeResult(t, db, channel1, account1, 1, "gpt-4o", "success", 90.0, 200, nil)
	// channel2: one inconclusive result.
	insertProbeResult(t, db, channel2, account2, 1, "gpt-4o-mini", "inconclusive", nil, nil, "timeout waiting for response")
	// Account-only row (channel_id NULL): must NOT appear on the channels
	// endpoint but MUST still count for the accounts endpoint.
	insertProbeResult(t, db, nil, account2, 1, "gpt-4o-mini", "skipped", nil, nil, nil)

	// Default limit returns every row, newest first per channel.
	envelope := decodeProbeHistoryEnvelope(t, doGet(t, r, "/api/channels/probe-history"))
	items := envelope["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("expected 2 channel groups, got %d: %s", len(items), envelope)
	}
	first := items[0].(map[string]any)
	if first["channelId"] != float64(channel1) {
		t.Fatalf("expected first group channelId=%d, got %v", channel1, first["channelId"])
	}
	results := first["results"].([]any)
	if len(results) != 3 {
		t.Fatalf("expected 3 results for channel1, got %d", len(results))
	}
	newest := results[0].(map[string]any)
	if newest["status"] != "success" || newest["latencyMs"] != 90.0 {
		t.Fatalf("expected newest result success/90ms, got %v", newest)
	}
	oldest := results[2].(map[string]any)
	if oldest["status"] != "success" || oldest["latencyMs"] != 120.5 {
		t.Fatalf("expected oldest result success/120.5ms, got %v", oldest)
	}
	failed := results[1].(map[string]any)
	if failed["status"] != "failure" || failed["errorText"] != "upstream 500" || failed["httpStatus"] != float64(500) {
		t.Fatalf("expected failure row with error text, got %v", failed)
	}
	// camelCase contract spot-check.
	for _, key := range []string{"id", "status", "latencyMs", "httpStatus", "errorText", "modelName", "createdAt"} {
		if _, ok := newest[key]; !ok {
			t.Fatalf("result missing camelCase key %q: %v", key, newest)
		}
	}

	// limit=1 keeps only the newest per channel.
	envelope = decodeProbeHistoryEnvelope(t, doGet(t, r, "/api/channels/probe-history?limit=1"))
	if envelope["limit"] != float64(1) {
		t.Fatalf("expected limit echo 1, got %v", envelope["limit"])
	}
	items = envelope["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("expected 2 channel groups under limit=1, got %d", len(items))
	}
	for _, item := range items {
		group := item.(map[string]any)
		if got := len(group["results"].([]any)); got != 1 {
			t.Fatalf("expected exactly 1 result per channel, got %d", got)
		}
	}

	// The NULL-channel row never leaks into the channels endpoint.
	for _, item := range items {
		group := item.(map[string]any)
		id := int64(group["channelId"].(float64))
		if id != channel1 && id != channel2 {
			t.Fatalf("unexpected channelId %d in response", id)
		}
	}
}

func TestProbeHistory_AccountsGroupingIncludesNullChannelRows(t *testing.T) {
	db, r := setupProbeHistoryHarness(t)
	account1, account2, channel1, _ := seedProbeHistoryFixtures(t, db)

	insertProbeResult(t, db, channel1, account1, 1, "gpt-4o", "success", 100.0, 200, nil)
	insertProbeResult(t, db, nil, account2, 1, "gpt-4o-mini", "failure", nil, 401, "unauthorized")

	envelope := decodeProbeHistoryEnvelope(t, doGet(t, r, "/api/accounts/probe-history"))
	items := envelope["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("expected 2 account groups, got %d: %s", len(items), envelope)
	}
	byAccount := map[int64]map[string]any{}
	for _, item := range items {
		group := item.(map[string]any)
		id, ok := group["accountId"].(float64)
		if !ok {
			t.Fatalf("accounts endpoint must group by accountId, got %v", group)
		}
		byAccount[int64(id)] = group
	}
	if got := len(byAccount[account2]["results"].([]any)); got != 1 {
		t.Fatalf("account2 should carry its NULL-channel row, got %d results", got)
	}
	row := byAccount[account2]["results"].([]any)[0].(map[string]any)
	if row["status"] != "failure" || row["httpStatus"] != float64(401) {
		t.Fatalf("unexpected account2 row: %v", row)
	}
	if got := len(byAccount[account1]["results"].([]any)); got != 1 {
		t.Fatalf("account1 should carry 1 result, got %d", got)
	}
}

func TestProbeHistory_LimitClamping(t *testing.T) {
	db, r := setupProbeHistoryHarness(t)
	account1, _, channel1, _ := seedProbeHistoryFixtures(t, db)
	insertProbeResult(t, db, channel1, account1, 1, "gpt-4o", "success", 100.0, 200, nil)

	// limit=0 clamps to 1, oversized limit clamps to the documented max.
	envelope := decodeProbeHistoryEnvelope(t, doGet(t, r, "/api/channels/probe-history?limit=0"))
	if envelope["limit"] != float64(1) {
		t.Fatalf("expected limit=0 to clamp to 1, got %v", envelope["limit"])
	}
	envelope = decodeProbeHistoryEnvelope(t, doGet(t, r, "/api/channels/probe-history?limit=5000"))
	if envelope["limit"] != float64(probeHistoryMaxLimit) {
		t.Fatalf("expected limit=5000 to clamp to %d, got %v", probeHistoryMaxLimit, envelope["limit"])
	}
}

// Compile-time guard: queryProbeHistory must reject unknown entity columns
// instead of interpolating them into SQL.
func TestProbeHistory_RejectsUnknownEntityColumn(t *testing.T) {
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	var sqlxDB *sqlx.DB = db.DB
	if _, err := queryProbeHistory(sqlxDB, "account_id; DROP TABLE accounts", 5); err == nil {
		t.Fatal("expected error for unsupported entity column")
	}
}
