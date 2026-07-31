package admin

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/tokendancelab/metapi-go/store"
)

// ---- K1a (all-api-hub borrow): model name redirects ----

func setupRedirectTest(t *testing.T) (*store.DB, chi.Router, int64, int64) {
	t.Helper()
	db, _ := setupStatsSQLiteTest(t)
	now := time.Now().UTC().Format(time.RFC3339)

	// Site + account.
	res, err := db.Exec(`INSERT INTO sites (name, url, platform, status, created_at, updated_at)
		VALUES (?, ?, ?, 'active', ?, ?)`, "RedirectSite", "https://redirect.example.test", "anthropic", now, now)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	siteID, _ := res.LastInsertId()
	res, err = db.Exec(`INSERT INTO accounts (site_id, username, access_token, api_token, status, checkin_enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'active', 0, ?, ?)`, siteID, "redirect-user", "sess", "sk-redirect", now, now)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	accountID, _ := res.LastInsertId()

	// Canonical candidates: allowed models + an exact route pattern.
	_, _ = db.Exec(`INSERT INTO settings (key, value) VALUES ('global_allowed_models', ?)`,
		`["claude-3-5-sonnet","claude-3-5-haiku","gpt-4o"]`)
	_, _ = db.Exec(`INSERT INTO token_routes (model_pattern, display_name, route_mode, routing_strategy, enabled, created_at, updated_at)
		VALUES (?, ?, 'standard', 'weighted', 1, ?, ?)`, "claude-3-5-sonnet", "Sonnet Route", now, now)

	// Available actual models (date-suffixed names).
	_, _ = db.Exec(`INSERT INTO model_availability (account_id, model_name, available, is_manual, checked_at)
		VALUES (?, ?, 1, 0, ?)`, accountID, "claude-3-5-sonnet-20241022", now)

	r := chi.NewRouter()
	RegisterModelRedirectRoutes(r, db.DB)
	return db, r, siteID, accountID
}

func TestRedirects_GenerateListApply(t *testing.T) {
	db, r, siteID, accountID := setupRedirectTest(t)

	// Manual generate for the account.
	resp := doPostJSON(t, r, "/api/model-redirects/generate", map[string]any{"accountId": accountID})
	if resp.Code != 200 {
		t.Fatalf("generate returned %d: %s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal generate: %v", err)
	}
	if body["success"] != true {
		t.Fatalf("generate success = %v", body["success"])
	}

	// List shows the mapping: canonical claude-3-5-sonnet → actual with date suffix.
	resp = doGet(t, r, "/api/model-redirects")
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	items := body["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("items = %#v, want 1 mapping", items)
	}
	m := items[0].(map[string]any)
	if m["canonical"] != "claude-3-5-sonnet" || m["actual"] != "claude-3-5-sonnet-20241022" {
		t.Fatalf("mapping = %#v, want canonical claude-3-5-sonnet", m)
	}
	if m["source"] != "sync" {
		t.Fatalf("source = %v, want sync", m["source"])
	}
	if m["siteName"] != "RedirectSite" {
		t.Fatalf("siteName = %v", m["siteName"])
	}

	// Regeneration is idempotent.
	resp = doPostJSON(t, r, "/api/model-redirects/generate", map[string]any{"accountId": accountID})
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal regen: %v", err)
	}
	if body["created"].(float64) != 0 {
		t.Fatalf("regen created = %v, want 0 (idempotent)", body["created"])
	}

	// Promote to manual, then regenerate → still not overwritten.
	redirectID := int64(m["id"].(float64))
	resp = doPutJSON(t, r, "/api/model-redirects/"+itoa(redirectID), map[string]any{"source": "manual"})
	if resp.Code != 200 {
		t.Fatalf("promote returned %d", resp.Code)
	}
	resp = doPostJSON(t, r, "/api/model-redirects/generate", map[string]any{"accountId": accountID})
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal regen2: %v", err)
	}
	if body["created"].(float64) != 0 {
		t.Fatalf("regen after manual created = %v, want 0", body["created"])
	}

	// Disabled-model fix flow: add a disabled entry for the canonical.
	_, err := db.Exec(`INSERT INTO site_disabled_models (site_id, model_name, created_at)
		VALUES (?, ?, ?)`, siteID, "claude-3-5-sonnet", time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert disabled: %v", err)
	}

	// Dry-run lists the candidate without deleting.
	resp = doPostJSON(t, r, "/api/model-redirects/apply", map[string]any{"dryRun": true})
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal dry-run: %v", err)
	}
	if body["dryRun"] != true || body["count"].(float64) != 1 {
		t.Fatalf("dry-run = %#v, want count 1", body)
	}
	var disabledCount int
	if err := db.Get(&disabledCount, "SELECT COUNT(*) FROM site_disabled_models"); err != nil {
		t.Fatalf("count disabled: %v", err)
	}
	if disabledCount != 1 {
		t.Fatalf("disabled count after dry-run = %d, want 1 (not deleted)", disabledCount)
	}

	// Real apply deletes it and records an event.
	resp = doPostJSON(t, r, "/api/model-redirects/apply", map[string]any{"dryRun": false})
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal apply: %v", err)
	}
	if body["dryRun"] != false || body["removed"].(float64) != 1 {
		t.Fatalf("apply = %#v, want removed 1", body)
	}
	if err := db.Get(&disabledCount, "SELECT COUNT(*) FROM site_disabled_models"); err != nil {
		t.Fatalf("count after apply: %v", err)
	}
	if disabledCount != 0 {
		t.Fatalf("disabled count after apply = %d, want 0", disabledCount)
	}
	var eventCount int
	if err := db.Get(&eventCount, "SELECT COUNT(*) FROM events WHERE type = 'model_redirect_applied'"); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if eventCount != 1 {
		t.Fatalf("events = %d, want 1", eventCount)
	}

	// Delete the redirect.
	resp = doDelete(t, r, "/api/model-redirects/"+itoa(redirectID))
	if resp.Code != 200 {
		t.Fatalf("delete returned %d", resp.Code)
	}
	resp = doGet(t, r, "/api/model-redirects")
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal after delete: %v", err)
	}
	if len(body["items"].([]any)) != 0 {
		t.Fatalf("after delete = %#v, want 0", body["items"])
	}
}

func TestRedirects_GenerationRules(t *testing.T) {
	db, r, _, accountID := setupRedirectTest(t)

	// Add more actual names covering the rule matrix.
	now := time.Now().UTC().Format(time.RFC3339)
	for _, m := range []string{
		"claude-3-5-sonnet-20241022-v2", // date+revision suffix
		"claude-3-5-haiku-latest",        // version suffix
		"gpt-4o",                         // exact → no mapping
		"CLAUDE-3-5-SONNET-20250101",     // case-folded date suffix
	} {
		_, err := db.Exec(`INSERT INTO model_availability (account_id, model_name, available, is_manual, checked_at)
			VALUES (?, ?, 1, 0, ?)`, accountID, m, now)
		if err != nil {
			t.Fatalf("insert availability %s: %v", m, err)
		}
	}

	resp := doPostJSON(t, r, "/api/model-redirects/generate", map[string]any{"accountId": accountID})
	if resp.Code != 200 {
		t.Fatalf("generate returned %d", resp.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["created"].(float64) != 2 {
		t.Fatalf("created = %v, want 2 (sonnet + haiku-latest; sonnet-v2/CLAUDE merge into existing canonical)", body["created"])
	}

	resp = doGet(t, r, "/api/model-redirects")
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	byCanonical := make(map[string]string)
	for _, it := range body["items"].([]any) {
		m := it.(map[string]any)
		byCanonical[m["canonical"].(string)] = m["actual"].(string)
	}
	if byCanonical["claude-3-5-sonnet"] != "claude-3-5-sonnet-20241022" {
		t.Fatalf("sonnet mapping = %q (first-chosen actual is stable, later names do not overwrite)", byCanonical["claude-3-5-sonnet"])
	}
	if byCanonical["claude-3-5-haiku"] != "claude-3-5-haiku-latest" {
		t.Fatalf("haiku mapping = %q", byCanonical["claude-3-5-haiku"])
	}
	if _, ok := byCanonical["gpt-4o"]; ok {
		t.Fatalf("gpt-4o exact match must not create a mapping: %#v", byCanonical)
	}
}

func TestRedirects_Validation(t *testing.T) {
	_, r, _, _ := setupRedirectTest(t)

	resp := doPutJSON(t, r, "/api/model-redirects/999999", map[string]any{"actual": "x"})
	if resp.Code != 404 {
		t.Fatalf("unknown update returned %d, want 404", resp.Code)
	}
	resp = doDelete(t, r, "/api/model-redirects/999999")
	if resp.Code != 404 {
		t.Fatalf("unknown delete returned %d, want 404", resp.Code)
	}
	resp = doPutJSON(t, r, "/api/model-redirects/abc", map[string]any{"actual": "x"})
	if resp.Code != 400 {
		t.Fatalf("bad id returned %d, want 400", resp.Code)
	}
}
