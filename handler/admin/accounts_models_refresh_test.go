package admin

// Focused tests for the Wave 12 account Models surface (#998):
//   - manual upstream refresh persists availability into the existing owner
//     (model_availability) — proven by store read-back;
//   - each refresh action triggers exactly one route rebuild + one routing
//     cache invalidation (call-count assertions via the seam vars);
//   - manual add/remove is explicit: remove only ever deletes is_manual rows;
//   - upstream failure never fakes success and triggers no side effects;
//   - SQLite + PostgreSQL dialect safety (PG gated on PG_TEST_DSN, wired in CI).

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

// withRefreshSideEffectCounters swaps the rebuild/invalidate seams for
// counting fakes and restores the production owners on cleanup.
// Wave 15 (#1005) moved the refresh core to service.RefreshAccountModels, so
// the "exactly one route rebuild + one routing-cache invalidation per refresh
// action" contract is asserted against the service seams; the manual-models
// endpoint keeps using the handler-level invalidateRoutingCache seam, which is
// folded into the same invalidate counter.
func withRefreshSideEffectCounters(t *testing.T) (rebuilds, invalidates *int) {
	t.Helper()
	var rebuildCount, invalidateCount int
	restore := service.SetModelRefreshSideEffectsForTest(
		func(ctx context.Context, db *sqlx.DB) (service.RouteRebuildStats, error) {
			rebuildCount++
			return service.RouteRebuildStats{}, nil
		},
		func() { invalidateCount++ },
	)
	// The manual-models endpoint (accounts_models.go) still invalidates the
	// routing cache through the handler-level seam; route it to the same
	// counter so per-mutating-action accounting stays exact.
	prevHandlerInvalidate := invalidateRoutingCache
	invalidateRoutingCache = func() { invalidateCount++ }
	t.Cleanup(func() {
		invalidateRoutingCache = prevHandlerInvalidate
		restore()
	})
	return &rebuildCount, &invalidateCount
}

// startModelsUpstream serves the OpenAI-compatible /v1/models contract the
// openai adapter fetches during refresh.
func startModelsUpstream(t *testing.T, expectToken string, status int, models ...string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		if expectToken != "" && r.Header.Get("Authorization") != "Bearer "+expectToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		data := make([]map[string]string, 0, len(models))
		for _, m := range models {
			data = append(data, map[string]string{"id": m})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": data})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func pointAccountSiteAt(t *testing.T, db *sqlx.DB, accountID int64, url string) {
	t.Helper()
	if _, err := db.Exec(db.Rebind("UPDATE sites SET url = ? WHERE id = (SELECT site_id FROM accounts WHERE id = ?)"), url, accountID); err != nil {
		t.Fatalf("point site url: %v", err)
	}
}

type availReadBack struct {
	ModelName string `db:"model_name"`
	Available int    `db:"available"`
	IsManual  int    `db:"is_manual"`
}

func readAvailability(t *testing.T, db *sqlx.DB, accountID int64) map[string]availReadBack {
	t.Helper()
	var rows []availReadBack
	if err := db.Select(&rows, db.Rebind(
		"SELECT model_name, CASE WHEN available THEN 1 ELSE 0 END AS available, CASE WHEN is_manual THEN 1 ELSE 0 END AS is_manual FROM model_availability WHERE account_id = ? ORDER BY model_name ASC",
	), accountID); err != nil {
		t.Fatalf("read model_availability: %v", err)
	}
	out := map[string]availReadBack{}
	for _, r := range rows {
		out[r.ModelName] = r
	}
	return out
}

func TestRefreshAccountModels_PersistsAvailability_RebuildAndInvalidateOnce(t *testing.T) {
	db := openModelRefreshTestDB(t)
	accountID, _ := seedAccountAndToken(t, db, "sk-lane-g-refresh")

	upstream := startModelsUpstream(t, "sk-lane-g-refresh", http.StatusOK, "gpt-4o", "gpt-4o-mini")
	pointAccountSiteAt(t, db, accountID, upstream.URL)

	now := time.Now().UTC().Format(time.RFC3339)
	// Prior state: an operator-pinned manual row and a stale auto row.
	if _, err := db.Exec(db.Rebind("INSERT INTO model_availability (account_id, model_name, available, is_manual, checked_at) VALUES (?, 'manual-pin', TRUE, TRUE, ?)"), accountID, now); err != nil {
		t.Fatalf("seed manual row: %v", err)
	}
	if _, err := db.Exec(db.Rebind("INSERT INTO model_availability (account_id, model_name, available, is_manual, checked_at) VALUES (?, 'stale-auto', TRUE, FALSE, ?)"), accountID, now); err != nil {
		t.Fatalf("seed stale row: %v", err)
	}

	rebuilds, invalidates := withRefreshSideEffectCounters(t)

	result := accountModelRefresher(context.Background(), db, accountID, false)
	if ok, _ := result["success"].(bool); !ok {
		t.Fatalf("refresh failed: %#v", result)
	}
	refresh, _ := result["refresh"].(map[string]any)
	if refresh == nil || refresh["status"] != "success" {
		t.Fatalf("refresh payload = %#v", result)
	}
	if mc, _ := refresh["modelCount"].(int); mc != 2 {
		t.Fatalf("modelCount = %#v, want 2", refresh["modelCount"])
	}

	// Store read-back: availability truly persisted in the existing owner.
	rows := readAvailability(t, db, accountID)
	if len(rows) != 4 {
		t.Fatalf("row count = %d, want 4: %#v", len(rows), rows)
	}
	for name, want := range map[string]availReadBack{
		"gpt-4o":      {ModelName: "gpt-4o", Available: 1, IsManual: 0},
		"gpt-4o-mini": {ModelName: "gpt-4o-mini", Available: 1, IsManual: 0},
		"manual-pin":  {ModelName: "manual-pin", Available: 1, IsManual: 1}, // pinned manual row survives
		"stale-auto":  {ModelName: "stale-auto", Available: 0, IsManual: 0}, // vanished upstream → unavailable
	} {
		got, ok := rows[name]
		if !ok {
			t.Fatalf("missing row %q", name)
		}
		if got.Available != want.Available || got.IsManual != want.IsManual {
			t.Errorf("%s = (available=%d, manual=%d), want (available=%d, manual=%d)",
				name, got.Available, got.IsManual, want.Available, want.IsManual)
		}
	}

	// Exactly one rebuild + one invalidation per refresh action.
	if *rebuilds != 1 {
		t.Errorf("rebuild calls = %d, want 1", *rebuilds)
	}
	if *invalidates != 1 {
		t.Errorf("invalidate calls = %d, want 1", *invalidates)
	}
}

func TestRefreshAccountModels_UpstreamFailure_NoFakeSuccessNoSideEffects(t *testing.T) {
	db := openModelRefreshTestDB(t)
	accountID, _ := seedAccountAndToken(t, db, "sk-lane-g-fail")

	upstream := startModelsUpstream(t, "", http.StatusInternalServerError)
	pointAccountSiteAt(t, db, accountID, upstream.URL)

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(db.Rebind("INSERT INTO model_availability (account_id, model_name, available, is_manual, checked_at) VALUES (?, 'keep-me', TRUE, FALSE, ?)"), accountID, now); err != nil {
		t.Fatalf("seed row: %v", err)
	}

	rebuilds, invalidates := withRefreshSideEffectCounters(t)

	result := accountModelRefresher(context.Background(), db, accountID, false)
	if ok, _ := result["success"].(bool); ok {
		t.Fatalf("expected honest failure, got %#v", result)
	}
	refresh, _ := result["refresh"].(map[string]any)
	if refresh == nil || refresh["status"] != "failed" {
		t.Fatalf("refresh payload = %#v, want status=failed", result)
	}

	// Store untouched: no availability invented from a failed upstream.
	rows := readAvailability(t, db, accountID)
	if got := rows["keep-me"]; got.Available != 1 || got.IsManual != 0 {
		t.Fatalf("keep-me mutated on failure: %#v", got)
	}
	if len(rows) != 1 {
		t.Fatalf("row count = %d, want 1", len(rows))
	}
	if *rebuilds != 0 || *invalidates != 0 {
		t.Errorf("side effects on failure: rebuilds=%d invalidates=%d, want 0/0", *rebuilds, *invalidates)
	}
}

func TestManualModels_AddRemove_ReadBack(t *testing.T) {
	db, r, _ := setupAccountsTest(t)
	_, accountID := setupAccountFixtureWithSite(t, db, r, "LaneGManual", "https://api.openai.com")
	rebuilds, invalidates := withRefreshSideEffectCounters(t)

	url := "/api/accounts/" + itoa(accountID) + "/models/manual"

	// Add two manual models.
	resp := doPostJSON(t, r, url, map[string]any{"models": []string{"manual-a", "manual-b"}})
	if resp.Code != http.StatusOK {
		t.Fatalf("add: %d %s", resp.Code, resp.Body.String())
	}
	var added map[string]any
	_ = json.Unmarshal(resp.Body.Bytes(), &added)
	if added["success"] != true || added["added"] != float64(2) || added["removed"] != float64(0) {
		t.Fatalf("add response = %#v", added)
	}

	// Seed an auto row that a remove attempt must never touch.
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(db.Rebind("INSERT INTO model_availability (account_id, model_name, available, is_manual, checked_at) VALUES (?, 'auto-keep', TRUE, FALSE, ?)"), accountID, now); err != nil {
		t.Fatalf("seed auto row: %v", err)
	}

	// Remove one manual row + one auto row (no-op) + one unknown (no-op).
	resp = doPostJSON(t, r, url, map[string]any{"models": []string{}, "remove": []string{"manual-a", "auto-keep", "ghost"}})
	if resp.Code != http.StatusOK {
		t.Fatalf("remove: %d %s", resp.Code, resp.Body.String())
	}
	var removed map[string]any
	_ = json.Unmarshal(resp.Body.Bytes(), &removed)
	if removed["success"] != true || removed["added"] != float64(0) || removed["removed"] != float64(1) {
		t.Fatalf("remove response = %#v", removed)
	}

	// Store read-back: only manual-b (manual) and auto-keep (auto) survive.
	rows := readAvailability(t, db.DB, accountID)
	if len(rows) != 2 {
		t.Fatalf("row count = %d, want 2: %#v", len(rows), rows)
	}
	if got := rows["manual-b"]; got.Available != 1 || got.IsManual != 1 {
		t.Fatalf("manual-b = %#v, want available+manual", got)
	}
	if got := rows["auto-keep"]; got.Available != 1 || got.IsManual != 0 {
		t.Fatalf("auto-keep = %#v, want available+auto (remove must not delete auto rows)", got)
	}

	// Both lists empty → 400.
	resp = doPostJSON(t, r, url, map[string]any{"models": []string{}, "remove": []string{}})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty add+remove, got %d", resp.Code)
	}

	// Manual mutations invalidate the routing cache exactly once per action
	// and never rebuild routes (rebuild belongs to the refresh owner).
	if *invalidates != 2 {
		t.Errorf("invalidate calls = %d, want 2 (one per mutating action)", *invalidates)
	}
	if *rebuilds != 0 {
		t.Errorf("rebuild calls = %d, want 0 on manual mutations", *rebuilds)
	}
}

func TestGetAccountModels_HonestSourceAndAvailability(t *testing.T) {
	db, r, _ := setupAccountsTest(t)
	siteID, accountID := setupAccountFixtureWithSite(t, db, r, "LaneGGet", "https://api.openai.com")

	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	seeds := []struct {
		name      string
		available bool
		isManual  bool
	}{
		{"auto-up", true, false},
		{"auto-down", false, false},
		{"manual-up", true, true},
	}
	for _, s := range seeds {
		if _, err := db.Exec(db.Rebind("INSERT INTO model_availability (account_id, model_name, available, is_manual, checked_at) VALUES (?, ?, ?, ?, ?)"), accountID, s.name, s.available, s.isManual, now); err != nil {
			t.Fatalf("seed %s: %v", s.name, err)
		}
	}

	resp := doGet(t, r, "/api/accounts/"+itoa(accountID)+"/models")
	if resp.Code != http.StatusOK {
		t.Fatalf("get models: %d %s", resp.Code, resp.Body.String())
	}
	var result struct {
		Models []struct {
			Name      string  `json:"name"`
			Available bool    `json:"available"`
			IsManual  bool    `json:"isManual"`
			Disabled  bool    `json:"disabled"`
			CheckedAt *string `json:"checkedAt"`
		} `json:"models"`
		TotalCount int `json:"totalCount"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.TotalCount != 3 || len(result.Models) != 3 {
		t.Fatalf("expected 3 rows (including unavailable), got total=%d len=%d", result.TotalCount, len(result.Models))
	}
	byName := map[string]int{}
	for i, m := range result.Models {
		byName[m.Name] = i
	}
	if i, ok := byName["auto-down"]; !ok {
		t.Fatal("unavailable row missing from response")
	} else if result.Models[i].Available {
		t.Error("auto-down must report available=false")
	}
	if i, ok := byName["manual-up"]; !ok {
		t.Fatal("manual row missing")
	} else {
		if !result.Models[i].IsManual || !result.Models[i].Available {
			t.Error("manual-up must report isManual=true available=true")
		}
		if result.Models[i].CheckedAt == nil || *result.Models[i].CheckedAt != now {
			t.Errorf("checkedAt = %v, want %s", result.Models[i].CheckedAt, now)
		}
	}

	// Empty account: models must be [] not null. Seed a second account on the
	// same site directly (the fixture enforces one site per URL).
	emptyRes, err := db.Exec(db.Rebind("INSERT INTO accounts (site_id, username, access_token, status, checkin_enabled, sort_order, created_at, updated_at) VALUES (?, 'lane-g-empty', 'sk-empty', 'active', FALSE, 0, ?, ?)"), siteID, now, now)
	if err != nil {
		t.Fatalf("seed empty account: %v", err)
	}
	emptyID, _ := emptyRes.LastInsertId()
	resp = doGet(t, r, "/api/accounts/"+itoa(emptyID)+"/models")
	if resp.Code != http.StatusOK {
		t.Fatalf("empty get: %d", resp.Code)
	}
	if !strings.Contains(resp.Body.String(), `"models":[]`) {
		t.Errorf("expected empty models array, body=%s", resp.Body.String())
	}
}

// TestAccountModels_Postgres exercises the same handler SQL on PostgreSQL
// (dialect safety for the new CASE/DELETE/upsert read-write paths). Gated on
// PG_TEST_DSN; CI wires a postgres:16 service for it.
func TestAccountModels_Postgres(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PG_TEST_DSN"))
	if dsn == "" {
		t.Skip("PG_TEST_DSN not set; skipping PostgreSQL account-models test")
	}
	dbx, err := store.Open(store.DialectPostgres, dsn, false)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(func() { dbx.Close() })
	if err := store.AutoMigrate(dbx); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	db := dbx.DB

	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	now := time.Now().UTC().Format(time.RFC3339)

	var siteID int64
	if err := db.Get(&siteID, db.Rebind(
		"INSERT INTO sites (name, url, platform, status, use_system_proxy, sort_order, global_weight, post_refresh_probe_enabled, created_at, updated_at) VALUES (?, ?, 'openai', 'active', ?, 0, 0, ?, ?, ?) RETURNING id",
	), "lane-g-pg-"+suffix, "https://api.example.com", false, false, now, now); err != nil {
		t.Fatalf("insert site: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(db.Rebind("DELETE FROM sites WHERE id = ?"), siteID) })

	var accountID int64
	if err := db.Get(&accountID, db.Rebind(
		"INSERT INTO accounts (site_id, username, access_token, status, checkin_enabled, sort_order, created_at, updated_at) VALUES (?, ?, ?, 'active', ?, 0, ?, ?) RETURNING id",
	), siteID, "lane-g-pg-user", "sk-pg", false, now, now); err != nil {
		t.Fatalf("insert account: %v", err)
	}

	h := &accountsHandler{db: db, cfg: &config.Config{}}
	r := chi.NewRouter()
	r.Get("/api/accounts/{id}/models", h.getAccountModels)
	r.Post("/api/accounts/{id}/models/manual", h.manualModels)

	manualModel := "lane-g-manual-" + suffix
	autoModel := "lane-g-auto-" + suffix
	if _, err := db.Exec(db.Rebind("INSERT INTO model_availability (account_id, model_name, available, is_manual, checked_at) VALUES (?, ?, true, false, ?)"), accountID, autoModel, now); err != nil {
		t.Fatalf("seed auto row: %v", err)
	}

	// Add manual + remove auto (must no-op) through the real handlers.
	resp := doPostJSON(t, r, "/api/accounts/"+itoa(accountID)+"/models/manual", map[string]any{
		"models": []string{manualModel},
		"remove": []string{autoModel},
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("manual on pg: %d %s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	_ = json.Unmarshal(resp.Body.Bytes(), &body)
	if body["added"] != float64(1) || body["removed"] != float64(0) {
		t.Fatalf("pg response = %#v", body)
	}

	rows := readAvailability(t, db, accountID)
	if got, ok := rows[manualModel]; !ok || got.Available != 1 || got.IsManual != 1 {
		t.Fatalf("manual row on pg = %#v (ok=%v)", got, ok)
	}
	if got, ok := rows[autoModel]; !ok || got.Available != 1 || got.IsManual != 0 {
		t.Fatalf("auto row on pg must survive remove = %#v (ok=%v)", got, ok)
	}

	// GET read-back through the handler (CASE WHEN booleans on PG).
	getResp := doGet(t, r, "/api/accounts/"+itoa(accountID)+"/models")
	if getResp.Code != http.StatusOK {
		t.Fatalf("get on pg: %d %s", getResp.Code, getResp.Body.String())
	}
	var getResult struct {
		Models []struct {
			Name      string `json:"name"`
			Available bool   `json:"available"`
			IsManual  bool   `json:"isManual"`
		} `json:"models"`
		TotalCount int `json:"totalCount"`
	}
	if err := json.Unmarshal(getResp.Body.Bytes(), &getResult); err != nil {
		t.Fatalf("decode pg get: %v", err)
	}
	if getResult.TotalCount != 2 {
		t.Fatalf("pg totalCount = %d, want 2 (%s)", getResult.TotalCount, getResp.Body.String())
	}

	// Explicit manual row deletion on PG.
	delResp := doPostJSON(t, r, "/api/accounts/"+itoa(accountID)+"/models/manual", map[string]any{
		"remove": []string{manualModel},
	})
	if delResp.Code != http.StatusOK {
		t.Fatalf("remove on pg: %d %s", delResp.Code, delResp.Body.String())
	}
	rows = readAvailability(t, db, accountID)
	if _, ok := rows[manualModel]; ok {
		t.Fatal("manual row still present after explicit delete on pg")
	}
}
