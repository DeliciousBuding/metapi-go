package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

func TestAutomaticModelRoutesAdminLifecycleSQLite(t *testing.T) {
	db, r, cfg := setupAccountsTest(t)
	exerciseAutomaticModelRoutesAdmin(t, db, r, cfg)
}

func TestAutomaticModelRoutesAdminLifecyclePostgres(t *testing.T) {
	db, r, cfg := setupAccountsPostgresTest(t)
	exerciseAutomaticModelRoutesAdmin(t, db, r, cfg)
}

// Same HTTP settings + real model-discovery + route rebuild workflow on both
// supported dialects. No bypass route inserts stand in for the feature itself.
func exerciseAutomaticModelRoutesAdmin(t *testing.T, db *store.DB, r chi.Router, cfg *config.Config) {
	t.Helper()
	config.Set(cfg)
	config.SetRuntime(&config.RuntimeSettings{})
	t.Cleanup(func() { config.SetRuntime(nil) })
	RegisterSettingsRoutes(r, db.DB, cfg)
	RegisterTokenRoutesWithDeps(r, db.DB, TokenRoutesDeps{})
	var mu sync.Mutex
	models := []string{"lifecycle-alpha", "lifecycle-beta"}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/v1/models" {
			http.NotFound(w, req)
			return
		}
		if req.Header.Get("Authorization") != "Bearer sk-route-lifecycle-fixture" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		mu.Lock()
		current := append([]string(nil), models...)
		mu.Unlock()
		data := make([]map[string]string, 0, len(current))
		for _, model := range current {
			data = append(data, map[string]string{"id": model})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	t.Cleanup(upstream.Close)
	now := time.Now().UTC().Format(time.RFC3339)
	siteID, err := execInsertID(db.DB, "INSERT INTO sites (name,url,platform,status,created_at,updated_at) VALUES ('route lifecycle',?,'openai','active',?,?)", upstream.URL, now, now)
	if err != nil {
		t.Fatal(err)
	}
	accountID, err := execInsertID(db.DB, "INSERT INTO accounts (site_id,username,access_token,api_token,status,checkin_enabled,created_at,updated_at) VALUES (?,'lifecycle','sk-route-lifecycle-fixture','sk-route-lifecycle-fixture','active',?,?,?)", siteID, false, now, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := execInsertID(db.DB, "INSERT INTO account_tokens (account_id,name,token,value_status,enabled,is_default) VALUES (?,'default','sk-route-lifecycle-fixture','ready',?,?)", accountID, true, true); err != nil {
		t.Fatal(err)
	}
	rebuild := func() map[string]any {
		t.Helper()
		rec := doPostJSON(t, r, "/api/routes/rebuild", map[string]any{"refreshModels": true, "wait": true})
		if rec.Code != http.StatusOK {
			t.Fatalf("rebuild status=%d %s", rec.Code, rec.Body.String())
		}
		return readRebuildEnvelope(t, rec)
	}
	setAutomatic := func(enabled bool) {
		t.Helper()
		rec := doPutJSON(t, r, "/api/settings/runtime", map[string]any{"autoCreateModelRoutes": enabled})
		if rec.Code != http.StatusOK {
			t.Fatalf("settings status=%d %s", rec.Code, rec.Body.String())
		}
		get := httptest.NewRecorder()
		r.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/settings/runtime", nil))
		if readRebuildEnvelope(t, get)["autoCreateModelRoutes"] != enabled {
			t.Fatalf("setting not readable: %s", get.Body.String())
		}
		var persisted string
		if err := db.Get(&persisted, "SELECT value FROM settings WHERE key = ?", "auto_create_model_routes"); err != nil {
			t.Fatal(err)
		}
		if persisted != strconv.FormatBool(enabled) {
			t.Fatalf("setting not persisted: %q", persisted)
		}
		hydrated := &config.RuntimeSettings{}
		store.ApplyRuntimeSettings(cfg, hydrated, map[string]string{"auto_create_model_routes": persisted})
		if hydrated.AutoCreateModelRoutes != enabled {
			t.Fatal("setting lost on startup hydration")
		}
	}
	if first := rebuild(); first["routesCreated"] != float64(0) {
		t.Fatalf("default creates routes: %v", first)
	}
	setAutomatic(true)
	result := rebuild()
	if result["routesCreated"] != float64(2) || result["channelsInserted"] != float64(4) {
		t.Fatalf("enabled setting did not create routes and channels: %v", result)
	}
	var alphaID, betaID int64
	if err := db.Get(&alphaID, "SELECT id FROM token_routes WHERE model_pattern = 'lifecycle-alpha'"); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&betaID, "SELECT id FROM token_routes WHERE model_pattern = 'lifecycle-beta'"); err != nil {
		t.Fatal(err)
	}
	// A manual disable stays authoritative even when the model reappears.
	rec := doPutJSON(t, r, fmt.Sprintf("/api/routes/%d", betaID), map[string]any{"enabled": false})
	if rec.Code != http.StatusOK {
		t.Fatalf("disable route: %s", rec.Body.String())
	}
	mu.Lock()
	models = []string{"lifecycle-beta", "lifecycle-gamma"}
	mu.Unlock()
	result = rebuild()
	if result["routesCreated"] != float64(1) {
		t.Fatalf("model sync did not create new gamma route: %v", result)
	}
	var count int
	if err := db.Get(&count, "SELECT COUNT(*) FROM route_channels WHERE route_id = ?", alphaID); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("delisted alpha retained %d channels", count)
	}
	if err := db.Get(&count, "SELECT COUNT(*) FROM token_routes WHERE id = ? AND NOT enabled", betaID); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("model sync reenabled a manually disabled route")
	}
	// A real empty snapshot removes automatic channels but not route IDs.
	mu.Lock()
	models = []string{}
	mu.Unlock()
	empty := rebuild()
	if empty["modelRefresh"].(map[string]any)["failed"] != float64(1) {
		t.Fatalf("empty-model outcome hidden: %v", empty)
	}
	if err := db.Get(&count, "SELECT COUNT(*) FROM route_channels"); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("empty upstream left %d channels", count)
	}
	if err := db.Get(&count, "SELECT COUNT(*) FROM token_routes"); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatal("empty snapshot deleted operator-visible route IDs")
	}
	setAutomatic(false)
	mu.Lock()
	models = []string{"lifecycle-delta", "lifecycle-alpha"}
	mu.Unlock()
	after := rebuild()
	if after["routesCreated"] != float64(0) {
		t.Fatalf("disabled feature kept creating routes: %v", after)
	}
	var alphaAfter int64
	if err := db.Get(&alphaAfter, "SELECT id FROM token_routes WHERE model_pattern='lifecycle-alpha'"); err != nil {
		t.Fatal(err)
	}
	if alphaAfter != alphaID {
		t.Fatal("route identity not retained after delisting/reappearance")
	}
	invalid := doPutJSON(t, r, "/api/settings/runtime", map[string]any{"autoCreateModelRoutes": map[string]any{"invalid": true}})
	if invalid.Code != http.StatusBadRequest || config.Runtime().AutoCreateModelRoutes {
		t.Fatalf("invalid setting accepted: %d %s", invalid.Code, invalid.Body.String())
	}
}

func TestModelRouteSettingsInvalidPatchDoesNotPartiallyEnable(t *testing.T) {
	db, r, _ := setupEdgeTest(t)
	response := doPutJSON(t, r, "/api/settings/runtime", map[string]any{"autoCreateModelRoutes": true, "modelSyncCron": "not a cron"})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d %s", response.Code, response.Body.String())
	}
	if config.Runtime().AutoCreateModelRoutes {
		t.Fatal("rejected settings patch enabled automatic route creation")
	}
	var count int
	if err := db.Get(&count, "SELECT COUNT(*) FROM settings WHERE key = ?", "auto_create_model_routes"); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("rejected patch persisted automatic route creation")
	}
}

func TestModelRouteSettingsDatabaseFailureRollsBackBothControls(t *testing.T) {
	db, r, _ := setupEdgeTest(t)
	if _, err := db.Exec("CREATE TRIGGER fail_model_schedule BEFORE INSERT ON settings WHEN NEW.key = 'model_sync_cron' BEGIN SELECT RAISE(FAIL, 'controlled settings failure'); END"); err != nil {
		t.Fatal(err)
	}
	response := doPutJSON(t, r, "/api/settings/runtime", map[string]any{"autoCreateModelRoutes": true, "modelSyncCron": "0 3 * * *"})
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d %s", response.Code, response.Body.String())
	}
	if config.Runtime().AutoCreateModelRoutes || config.Runtime().ModelSyncCron != "" {
		t.Fatal("failed transaction changed the runtime snapshot")
	}
	var count int
	if err := db.Get(&count, "SELECT COUNT(*) FROM settings WHERE key IN (?, ?)", "auto_create_model_routes", "model_sync_cron"); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("partial model sync settings persisted: %d", count)
	}
}
