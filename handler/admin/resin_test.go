package admin

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// setupResinStatusTest boots a standalone chi router with the Resin status
// endpoint mounted against an in-memory SQLite DB. Returns the router and
// DB so the test can seed rows directly.
func setupResinStatusTest(t *testing.T) (*store.DB, chi.Router, *config.Config) {
	t.Helper()
	resetBackgroundTasksForTests()

	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	cfg := &config.Config{}
	config.Set(cfg)
	config.SetRuntime(&config.RuntimeSettings{AuthToken: "resin-admin-test-token"})
	t.Cleanup(func() { config.SetRuntime(nil) })

	r := chi.NewRouter()
	RegisterResinRoutes(r, db.DB, cfg)
	return db, r, cfg
}

func TestResinStatus_DisabledGlobalReturnsValidShape(t *testing.T) {
	_, r, _ := setupResinStatusTest(t)

	rec := doGet(t, r, "/api/admin/resin/status")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["enabled"] != false {
		t.Fatalf("enabled = %v, want false (no RESIN_ENABLED set)", body["enabled"])
	}
	if body["resinUrl"] != "" {
		t.Fatalf("resinUrl = %q, want empty when not configured", body["resinUrl"])
	}
	// Empty arrays must serialize as [] not null so frontend can map safely.
	leases, ok := body["activeLeases"].([]any)
	if !ok {
		t.Fatalf("activeLeases not an array: %T", body["activeLeases"])
	}
	if len(leases) != 0 {
		t.Fatalf("activeLeases = %v, want empty array", leases)
	}
	overrides, ok := body["perSiteOverrides"].([]any)
	if !ok {
		t.Fatalf("perSiteOverrides not an array: %T", body["perSiteOverrides"])
	}
	if len(overrides) != 0 {
		t.Fatalf("perSiteOverrides = %v, want empty array", overrides)
	}
	if body["generatedAt"] == nil || body["generatedAt"] == "" {
		t.Fatalf("generatedAt missing or empty: %v", body["generatedAt"])
	}
}

func TestResinStatus_EnabledGlobalSurfacesConfig(t *testing.T) {
	_, r, cfg := setupResinStatusTest(t)
	cfg.ResinURL = "http://resin.local:2260/my-token"
	cfg.ResinPlatformName = "metapi"
	cfg.ResinEnabled = true

	rec := doGet(t, r, "/api/admin/resin/status")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["enabled"] != true {
		t.Fatalf("enabled = %v, want true", body["enabled"])
	}
	if body["resinUrl"] != "http://resin.local:2260/my-token" {
		t.Fatalf("resinUrl = %q, want configured URL", body["resinUrl"])
	}
	if body["platformName"] != "metapi" {
		t.Fatalf("platformName = %v, want metapi", body["platformName"])
	}
}

func TestResinStatus_ActiveLeasesSurfaceFromTracker(t *testing.T) {
	_, r, _ := setupResinStatusTest(t)

	const accountID = "acc-status-test-1"
	service.TouchResinLease(accountID)
	t.Cleanup(func() { service.ClearResinLeasesForTest() })

	rec := doGet(t, r, "/api/admin/resin/status")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	leases, _ := body["activeLeases"].([]any)
	found := false
	for _, raw := range leases {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if entry["accountId"] == accountID {
			found = true
			if entry["lastUsed"] == "" {
				t.Fatalf("lastUsed empty for %q", accountID)
			}
			break
		}
	}
	if !found {
		t.Fatalf("active lease %q not surfaced: %v", accountID, leases)
	}
}

func TestResinStatus_PerSiteOverridesListed(t *testing.T) {
	db, r, _ := setupResinStatusTest(t)
	// Seed three sites: nil (inherit), explicit true, explicit false.
	now := "2026-08-15T00:00:00.000Z"
	execMulti(t, db,
		[]string{
			`INSERT INTO sites (name, url, platform, status, sort_order, created_at, updated_at) VALUES ('inherit', 'https://inherit.example.com', 'openai', 'active', 0, '` + now + `', '` + now + `')`,
			`INSERT INTO sites (name, url, platform, status, sort_order, resin_enabled, created_at, updated_at) VALUES ('opt-in', 'https://optin.example.com', 'codex', 'active', 1, 1, '` + now + `', '` + now + `')`,
			`INSERT INTO sites (name, url, platform, status, sort_order, resin_enabled, created_at, updated_at) VALUES ('opt-out', 'https://optout.example.com', 'gemini', 'active', 2, 0, '` + now + `', '` + now + `')`,
		},
	)

	rec := doGet(t, r, "/api/admin/resin/status")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	overrides, _ := body["perSiteOverrides"].([]any)
	if len(overrides) != 2 {
		t.Fatalf("perSiteOverrides len = %d, want 2 (only explicit overrides)", len(overrides))
	}
	// Order should follow sites.sort_order (opt-in before opt-out).
	first, _ := overrides[0].(map[string]any)
	if first["name"] != "opt-in" {
		t.Fatalf("first override = %v, want opt-in", first)
	}
	if first["resinEnabled"] != true {
		t.Fatalf("opt-in resinEnabled = %v, want true", first["resinEnabled"])
	}
	second, _ := overrides[1].(map[string]any)
	if second["name"] != "opt-out" {
		t.Fatalf("second override = %v, want opt-out", second)
	}
	if second["resinEnabled"] != false {
		t.Fatalf("opt-out resinEnabled = %v, want false", second["resinEnabled"])
	}
}

// execMulti runs each SQL statement in order via db.Exec, failing the test on
// the first error. Used to seed deterministic rows for the status test.
func execMulti(t *testing.T, db *store.DB, statements []string) {
	t.Helper()
	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}
}
