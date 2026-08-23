package service

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

// ListSites must not fabricate $0.00 for sites whose account balances are
// all NULL ("unknown"): those report totalBalance=nil (JSON null). A site
// with no accounts at all is a genuine zero. Known balances still sum.
func TestListSites_NullBalanceReportsUnknown(t *testing.T) {
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	create := func(name string) int64 {
		id, err := CreateSite(db.DB, map[string]any{
			"name":                       name,
			"url":                        "https://" + name + ".example.com",
			"platform":                   "openai",
			"status":                     "active",
			"isPinned":                   false,
			"globalWeight":               1.0,
			"maxConcurrency":             int64(0),
			"proxyUrl":                   nil,
			"useSystemProxy":             false,
			"customHeaders":              nil,
			"externalCheckinUrl":         nil,
			"postRefreshProbeEnabled":    false,
			"postRefreshProbeModel":      "",
			"postRefreshProbeScope":      "single",
			"postRefreshProbeLatencyThresholdMs": 0,
		})
		if err != nil {
			t.Fatalf("CreateSite(%s): %v", name, err)
		}
		return id
	}

	knownID := create("known")   // accounts with real balances
	unknownID := create("unknown") // accounts with NULL balances
	create("noaccounts") // no accounts at all

	if _, err := db.DB.Exec(
		`INSERT INTO accounts (site_id, username, access_token, balance, extra_config) VALUES (?, ?, ?, ?, ?)`,
		knownID, "a1", "tok-a1", 10.0, ""); err != nil {
		t.Fatalf("insert known a1: %v", err)
	}
	if _, err := db.DB.Exec(
		`INSERT INTO accounts (site_id, username, access_token, balance, extra_config) VALUES (?, ?, ?, ?, ?)`,
		knownID, "a2", "tok-a2", 2.34, ""); err != nil {
		t.Fatalf("insert known a2: %v", err)
	}
	// NULL balance — must not 500 and must not aggregate to 0.
	if _, err := db.DB.Exec(
		`INSERT INTO accounts (site_id, username, access_token, balance, extra_config) VALUES (?, ?, ?, NULL, ?)`,
		unknownID, "u1", "tok-u1", ""); err != nil {
		t.Fatalf("insert unknown: %v", err)
	}

	sites, err := ListSites(db.DB)
	if err != nil {
		t.Fatalf("ListSites: %v", err)
	}
	got := make(map[string]any)
	for _, site := range sites {
		got[site["name"].(string)] = site["totalBalance"]
	}

	if v, ok := got["known"].(float64); !ok || v != 12.34 {
		t.Errorf("known totalBalance = %v (%T), want float64 12.34", got["known"], got["known"])
	}
	if got["unknown"] != nil {
		t.Errorf("unknown totalBalance = %v, want nil (JSON null)", got["unknown"])
	}
	if v, ok := got["noaccounts"].(float64); !ok || v != 0 {
		t.Errorf("noaccounts totalBalance = %v (%T), want float64 0", got["noaccounts"], got["noaccounts"])
	}
}
