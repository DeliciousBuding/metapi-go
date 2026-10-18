package service

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

// TestSiteReadSurfacesMaskCredentials covers M18: GET /api/sites must not echo
// the site-level apiKey or the live cf_clearance cookie. The list and detail
// builders share siteToMap, so both shapes are asserted here.
func TestSiteReadSurfacesMaskCredentials(t *testing.T) {
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	siteID, err := CreateSite(db.DB, map[string]any{
		"name":                               "Credential Site",
		"url":                                "https://credential.example.com",
		"platform":                           "openai",
		"status":                             "active",
		"isPinned":                           false,
		"globalWeight":                       1.0,
		"maxConcurrency":                     int64(0),
		"proxyUrl":                           nil,
		"useSystemProxy":                     false,
		"customHeaders":                      nil,
		"externalCheckinUrl":                 nil,
		"postRefreshProbeEnabled":            false,
		"postRefreshProbeModel":              "",
		"postRefreshProbeScope":              "single",
		"postRefreshProbeLatencyThresholdMs": 0,
	})
	if err != nil {
		t.Fatalf("CreateSite: %v", err)
	}

	const apiKey = "sk-SITE-PLAINTEXT-1234"
	const cfClearance = "cf-CLEARANCE-PLAINTEXT-5678"
	if _, err := db.DB.Exec(
		"UPDATE sites SET api_key = ?, cf_clearance = ? WHERE id = ?",
		apiKey, cfClearance, siteID,
	); err != nil {
		t.Fatalf("seed site credentials: %v", err)
	}

	assertSiteMap := func(label string, site map[string]any) {
		t.Helper()
		if site == nil {
			t.Fatalf("%s: nil site map", label)
		}
		if got, ok := site["apiKey"]; ok {
			t.Fatalf("%s: plaintext apiKey returned: %v", label, got)
		}
		if got, ok := site["cfClearance"]; ok {
			t.Fatalf("%s: cfClearance returned: %v", label, got)
		}
		if site["hasApiKey"] != true {
			t.Fatalf("%s: hasApiKey = %v, want true", label, site["hasApiKey"])
		}
		if got := site["apiKeyMasked"]; got != "sk-S****1234" {
			t.Fatalf("%s: apiKeyMasked = %v, want sk-S****1234", label, got)
		}
	}

	sites, err := ListSites(db.DB)
	if err != nil {
		t.Fatalf("ListSites: %v", err)
	}
	if len(sites) != 1 {
		t.Fatalf("ListSites returned %d rows, want 1", len(sites))
	}
	assertSiteMap("list", sites[0])

	detail, err := LoadSiteWithEndpoints(db.DB, siteID)
	if err != nil {
		t.Fatalf("LoadSiteWithEndpoints: %v", err)
	}
	assertSiteMap("detail", detail)
}
