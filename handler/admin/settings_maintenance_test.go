package admin

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

func setupMaintenanceTest(t *testing.T) (*store.DB, chi.Router, *routing.RouteCache) {
	t.Helper()
	resetBackgroundTasksForTests()
	// Keep the process-lifetime singleton; only clear contents (#328).
	globalAccountsCache.clear()

	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Seed a warm accounts snapshot so clear-cache can prove invalidation.
	globalAccountsCache.set([]byte(`{"accounts":[],"sites":[]}`))

	// Seed a warm route cache so clear-cache can prove invalidation.
	rc := routing.NewRouteCache(5_000)
	rc.SetRoutes([]store.TokenRoute{{ID: 1, ModelPattern: "gpt-*"}})
	routing.SetGlobalCache(rc)
	t.Cleanup(func() { routing.SetGlobalCache(nil) })

	r := chi.NewRouter()
	RegisterMaintenanceRoutes(r, db.DB)
	RegisterTasksRoutes(r, db.DB)
	return db, r, rc
}

func TestMaintenanceClearCache_RealInvalidationAndRealJob(t *testing.T) {
	db, r, rc := setupMaintenanceTest(t)

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO token_routes (model_pattern, enabled, created_at, updated_at)
		VALUES ('gpt-*', TRUE, ?, ?)`, now, now); err != nil {
		t.Fatalf("seed token_routes: %v", err)
	}

	if _, hit := globalAccountsCache.get(); !hit {
		t.Fatal("accounts snapshot cache should be warm before clear-cache")
	}
	if rc.GetRoutes() == nil {
		t.Fatal("route cache should be warm before clear-cache")
	}

	resp := doPostJSON(t, r, "/api/settings/maintenance/clear-cache", map[string]any{})
	if resp.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s, want 202", resp.Code, resp.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["success"] != true {
		t.Fatalf("success=%v, want true", body["success"])
	}
	if body["queued"] != true {
		t.Fatalf("queued=%v, want true", body["queued"])
	}

	deletedRoutes, _ := body["deletedTokenRoutes"].(float64)
	if deletedRoutes < 1 {
		t.Fatalf("deletedTokenRoutes=%v, want >= 1", body["deletedTokenRoutes"])
	}

	jobID, _ := body["jobId"].(string)
	if jobID == "" || jobID == "stub-clear-cache" {
		t.Fatalf("jobId still stub/empty: %v", body["jobId"])
	}
	if taskID, _ := body["taskId"].(string); taskID != jobID {
		t.Fatalf("taskId=%v jobId=%v, want match", body["taskId"], jobID)
	}

	// In-process caches must be invalidated immediately.
	if _, hit := globalAccountsCache.get(); hit {
		t.Fatal("accounts snapshot cache should be cleared")
	}
	if routes := rc.GetRoutes(); routes != nil {
		t.Fatalf("route cache should be invalidated, got %d routes", len(routes))
	}

	// Durable rows wiped.
	var routeCount int64
	if err := db.Get(&routeCount, "SELECT COUNT(*) FROM token_routes"); err != nil {
		t.Fatalf("count token_routes: %v", err)
	}
	if routeCount != 0 {
		t.Fatalf("token_routes count=%d, want 0", routeCount)
	}

	// Real background task completes.
	deadline := time.Now().Add(2 * time.Second)
	var task map[string]any
	for time.Now().Before(deadline) {
		getResp := doGet(t, r, "/api/tasks/"+jobID)
		if getResp.Code != http.StatusOK {
			time.Sleep(20 * time.Millisecond)
			continue
		}
		var getBody map[string]any
		if err := json.Unmarshal(getResp.Body.Bytes(), &getBody); err != nil {
			t.Fatalf("decode task: %v", err)
		}
		task, _ = getBody["task"].(map[string]any)
		if task != nil {
			if task["status"] == "succeeded" || task["status"] == "failed" {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if task == nil {
		t.Fatal("clear-cache task not found in registry")
	}
	if task["status"] != "succeeded" {
		t.Fatalf("task status=%v body=%v, want succeeded", task["status"], task)
	}
	if task["type"] != clearCacheTaskType {
		t.Fatalf("task type=%v, want %s", task["type"], clearCacheTaskType)
	}
}

func TestMaintenanceClearCache_NoFakeStubJobID(t *testing.T) {
	_, r, _ := setupMaintenanceTest(t)

	resp := doPostJSON(t, r, "/api/settings/maintenance/clear-cache", nil)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	raw := resp.Body.String()
	if strings.Contains(raw, "stub-clear-cache") {
		t.Fatalf("response still contains stub job id: %s", raw)
	}
	if strings.Contains(raw, `"success":false`) {
		t.Fatalf("unexpected failure: %s", raw)
	}
}

// TestMaintenanceFactoryResetWipesTheRegistryNotAHandCopiedList seeds rows into
// tables the endpoint's previous 28-name list did not contain — admin_sessions
// above all, because a reset that left it standing kept every pre-reset admin
// cookie valid against an otherwise empty database — and asserts the endpoint
// empties them and reports the registry's whole table set. The coverage
// assertion is derived from store.FactoryResetTableNames(), so a table added to
// the schema later is automatically required here instead of silently escaping
// the reset the way those 9 did.
func TestMaintenanceFactoryResetWipesTheRegistryNotAHandCopiedList(t *testing.T) {
	db, r, _ := setupMaintenanceTest(t)
	now := time.Now().UTC().Format(time.RFC3339)

	if _, err := db.Exec(`INSERT INTO sites (name, url, platform, status, created_at, updated_at)
		VALUES ('Reset Site', 'https://reset.example.com', 'openai', 'active', ?, ?)`, now, now); err != nil {
		t.Fatalf("seed sites: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO accounts (site_id, username, access_token, status, checkin_enabled, created_at, updated_at)
		VALUES (1, 'reset-user', 'sk-reset-fixture', 'active', FALSE, ?, ?)`, now, now); err != nil {
		t.Fatalf("seed accounts: %v", err)
	}
	// Two of the nine tables the hand-copied list missed: the security-bearing
	// one and the highest-volume one.
	if _, err := db.Exec(`INSERT INTO admin_sessions (token_hash, created_at, last_seen_at, expires_at, client_ip, user_agent)
		VALUES ('hash-issued-before-reset', ?, ?, ?, '203.0.113.9', 'test-agent')`, now, now, now); err != nil {
		t.Fatalf("seed admin_sessions: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`INSERT INTO model_probe_results (account_id, site_id, model_name, status, created_at)
		VALUES (1, 1, 'gpt-4o', 'success', ?)`), now); err != nil {
		t.Fatalf("seed model_probe_results: %v", err)
	}

	resp := doPostJSON(t, r, "/api/settings/maintenance/factory-reset", map[string]any{"confirm": true})
	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["success"] != true {
		t.Fatalf("success=%v, want true", body["success"])
	}
	deleted, ok := body["deleted"].(map[string]any)
	if !ok {
		t.Fatalf("deleted map missing: %s", resp.Body.String())
	}
	registry := store.FactoryResetTableNames()
	if len(deleted) != len(registry) {
		t.Fatalf("deleted reports %d tables, want the registry's %d", len(deleted), len(registry))
	}
	for _, table := range registry {
		if _, reported := deleted[table]; !reported {
			t.Fatalf("table %q missing from the deleted report", table)
		}
	}

	for _, table := range []string{"sites", "accounts", "admin_sessions", "model_probe_results"} {
		var rows int64
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&rows); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if rows != 0 {
			t.Fatalf("%s holds %d row(s) after factory reset", table, rows)
		}
	}
}

// TestMaintenanceFactoryResetRequiresConfirmation keeps the guard that stops an
// empty or unconfirmed request from wiping the database.
func TestMaintenanceFactoryResetRequiresConfirmation(t *testing.T) {
	_, r, _ := setupMaintenanceTest(t)

	resp := doPostJSON(t, r, "/api/settings/maintenance/factory-reset", map[string]any{})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("unconfirmed reset status=%d, want 400", resp.Code)
	}
	if !strings.Contains(resp.Body.String(), "confirm") {
		t.Fatalf("unconfirmed reset body should name the confirm flag: %s", resp.Body.String())
	}
}
