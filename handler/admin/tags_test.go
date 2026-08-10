package admin

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/deliciousbuding/metapi-go/store"
)

// ---- I1: accounts/sites tag system ----

func setupTagsTest(t *testing.T) (*store.DB, chi.Router, int64, int64) {
	t.Helper()
	db, _ := setupStatsSQLiteTest(t)
	now := time.Now().UTC().Format(time.RFC3339)

	res, err := db.Exec(`INSERT INTO sites (name, url, platform, status, created_at, updated_at)
		VALUES (?, ?, ?, 'active', ?, ?)`, "TagSite", "https://tags.example.test", "openai", now, now)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	siteID, _ := res.LastInsertId()

	res, err = db.Exec(`INSERT INTO accounts (site_id, username, access_token, api_token, status, checkin_enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'active', 0, ?, ?)`, siteID, "tag-user", "sess", "sk-tag", now, now)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	accountID, _ := res.LastInsertId()

	r := chi.NewRouter()
	RegisterTagsRoutes(r, db.DB)
	return db, r, siteID, accountID
}

func TestTags_UpdateAndAggregate(t *testing.T) {
	db, r, siteID, accountID := setupTagsTest(t)

	// Empty tag list is accepted and stores empty.
	resp := doPutJSON(t, r, "/api/accounts/"+itoa(accountID)+"/tags", map[string]any{"tags": []string{}})
	if resp.Code != 200 {
		t.Fatalf("empty tags returned %d: %s", resp.Code, resp.Body.String())
	}

	// Set account tags + site tags.
	resp = doPutJSON(t, r, "/api/accounts/"+itoa(accountID)+"/tags", map[string]any{"tags": []string{"prod", "priority", "prod"}})
	if resp.Code != 200 {
		t.Fatalf("account tags returned %d: %s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	tags := body["tags"].([]any)
	if len(tags) != 2 { // dedup
		t.Fatalf("account tags = %#v, want 2 deduped", tags)
	}

	resp = doPutJSON(t, r, "/api/sites/"+itoa(siteID)+"/tags", map[string]any{"tags": []string{"prod", "alpha"}})
	if resp.Code != 200 {
		t.Fatalf("site tags returned %d: %s", resp.Code, resp.Body.String())
	}

	// Aggregate index: prod 2 (1 account + 1 site), priority 1, alpha 1.
	resp = doGet(t, r, "/api/tags")
	if resp.Code != 200 {
		t.Fatalf("tags index returned %d: %s", resp.Code, resp.Body.String())
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal index: %v", err)
	}
	items := body["items"].([]any)
	byName := make(map[string]map[string]any)
	for _, it := range items {
		m := it.(map[string]any)
		byName[m["name"].(string)] = m
	}
	prod := byName["prod"]
	if prod == nil {
		t.Fatalf("prod tag missing from %#v", byName)
	}
	if prod["accounts"].(float64) != 1 || prod["sites"].(float64) != 1 || prod["total"].(float64) != 2 {
		t.Fatalf("prod counts = %#v, want accounts=1 sites=1 total=2", prod)
	}
	if byName["priority"] == nil || byName["alpha"] == nil {
		t.Fatalf("missing tags in index: %#v", byName)
	}
	// Sorted by total desc: prod (2) first.
	if items[0].(map[string]any)["name"] != "prod" {
		t.Fatalf("items[0] = %#v, want prod first", items[0])
	}

	// DB column round-trips the JSON array text.
	var stored *string
	if err := db.Get(&stored, "SELECT tags FROM accounts WHERE id = ?", accountID); err != nil {
		t.Fatalf("read account tags: %v", err)
	}
	if stored == nil || *stored != `["prod","priority"]` {
		t.Fatalf("stored account tags = %v, want [\"prod\",\"priority\"]", stored)
	}

	// Clearing tags removes them from the index.
	resp = doPutJSON(t, r, "/api/accounts/"+itoa(accountID)+"/tags", map[string]any{"tags": []string{}})
	if resp.Code != 200 {
		t.Fatalf("clear tags returned %d", resp.Code)
	}
	resp = doGet(t, r, "/api/tags")
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal after clear: %v", err)
	}
	for _, it := range body["items"].([]any) {
		if it.(map[string]any)["name"] == "priority" {
			t.Fatalf("priority still present after clear: %#v", body["items"])
		}
	}
}

func TestTags_Validation(t *testing.T) {
	_, r, siteID, accountID := setupTagsTest(t)

	// Missing body / wrong shape.
	resp := doPutJSON(t, r, "/api/accounts/"+itoa(accountID)+"/tags", map[string]any{"nope": 1})
	if resp.Code != 400 {
		t.Fatalf("wrong body returned %d, want 400", resp.Code)
	}

	// Unknown account.
	resp = doPutJSON(t, r, "/api/accounts/999999/tags", map[string]any{"tags": []string{"x"}})
	if resp.Code != 404 {
		t.Fatalf("unknown account returned %d, want 404", resp.Code)
	}

	// Unknown site.
	resp = doPutJSON(t, r, "/api/sites/999999/tags", map[string]any{"tags": []string{"x"}})
	if resp.Code != 404 {
		t.Fatalf("unknown site returned %d, want 404", resp.Code)
	}

	// Bad id.
	resp = doPutJSON(t, r, "/api/sites/"+itoa(siteID)+"-abc/tags", map[string]any{"tags": []string{"x"}})
	if resp.Code != 400 {
		t.Fatalf("bad id returned %d, want 400", resp.Code)
	}
}
