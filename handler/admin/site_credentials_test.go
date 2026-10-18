package admin

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestListSites_MasksCredentials is the HTTP-level guard for M18: the site list
// may describe whether a credential exists, but it must not echo the apiKey or
// the cf_clearance cookie, and it must not be cacheable.
func TestListSites_MasksCredentials(t *testing.T) {
	db, r := setupSitesTest(t)

	created := newSiteFixture(t, r, "Credential Site", "https://api.openai.com")
	siteID := int64(created["id"].(float64))

	const apiKey = "sk-SITE-PLAINTEXT-1234"
	const cfClearance = "cf-CLEARANCE-PLAINTEXT-5678"
	if _, err := db.Exec(
		"UPDATE sites SET api_key = ?, cf_clearance = ? WHERE id = ?",
		apiKey, cfClearance, siteID,
	); err != nil {
		t.Fatalf("seed site credentials: %v", err)
	}

	resp := doGet(t, r, "/api/sites")
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /api/sites: %d %s", resp.Code, resp.Body.String())
	}
	if got := resp.Header().Get("Cache-Control"); got != noStoreCacheControl {
		t.Fatalf("Cache-Control = %q, want %q", got, noStoreCacheControl)
	}

	var sites []map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &sites); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(sites) != 1 {
		t.Fatalf("sites = %d, want 1", len(sites))
	}
	site := sites[0]
	if got, ok := site["apiKey"]; ok {
		t.Fatalf("plaintext apiKey returned: %v", got)
	}
	if got, ok := site["cfClearance"]; ok {
		t.Fatalf("cfClearance returned: %v", got)
	}
	if site["hasApiKey"] != true {
		t.Fatalf("hasApiKey = %v, want true", site["hasApiKey"])
	}
	if got := site["apiKeyMasked"]; got != "sk-S****1234" {
		t.Fatalf("apiKeyMasked = %v, want sk-S****1234", got)
	}
	body := resp.Body.String()
	if strings.Contains(body, apiKey) || strings.Contains(body, cfClearance) {
		t.Fatalf("site list leaked plaintext credentials: %s", body)
	}
}

// TestAccountsSnapshotSitesMaskCredentials guards the embedded site array on
// GET /api/accounts: it used to marshal []store.Site, whose JSON tags expose
// the plaintext site apiKey and cf_clearance even though GET /api/sites masks
// them.
func TestAccountsSnapshotSitesMaskCredentials(t *testing.T) {
	db, r, _ := setupAccountsTest(t)

	created := newSiteFixture(t, r, "Embedded Site", "https://api.openai.com")
	siteID := int64(created["id"].(float64))

	const apiKey = "sk-SITE-PLAINTEXT-1234"
	const cfClearance = "cf-CLEARANCE-PLAINTEXT-5678"
	if _, err := db.Exec(
		"UPDATE sites SET api_key = ?, cf_clearance = ? WHERE id = ?",
		apiKey, cfClearance, siteID,
	); err != nil {
		t.Fatalf("seed site credentials: %v", err)
	}

	resp := doGet(t, r, "/api/accounts")
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /api/accounts: %d %s", resp.Code, resp.Body.String())
	}
	if got := resp.Header().Get("Cache-Control"); got != noStoreCacheControl {
		t.Fatalf("Cache-Control = %q, want %q", got, noStoreCacheControl)
	}
	if body := resp.Body.String(); strings.Contains(body, apiKey) || strings.Contains(body, cfClearance) {
		t.Fatalf("accounts snapshot leaked site credentials: %s", body)
	}

	var snapshot struct {
		Sites []map[string]any `json:"sites"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(snapshot.Sites) != 1 {
		t.Fatalf("sites = %d, want 1", len(snapshot.Sites))
	}
	site := snapshot.Sites[0]
	if _, ok := site["apiKey"]; ok {
		t.Fatalf("embedded plaintext apiKey: %v", site["apiKey"])
	}
	if _, ok := site["cfClearance"]; ok {
		t.Fatalf("embedded cfClearance: %v", site["cfClearance"])
	}
	if site["hasApiKey"] != true || site["apiKeyMasked"] != "sk-S****1234" {
		t.Fatalf("embedded site projection = %#v", site)
	}
}
