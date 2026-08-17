package service

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

// baseSiteData returns the minimal siteData map required by CreateSite,
// with nullable override fields (resinEnabled, useUtls) omitted so the
// caller can add only the ones it wants to exercise.
func baseSiteData(name string) map[string]any {
	return map[string]any{
		"name":                  name,
		"url":                   "https://" + name + ".example.com",
		"platform":              "openai",
		"status":                "active",
		"globalWeight":          1.0,
		"maxConcurrency":        int64(0),
		"postRefreshProbeScope": "single",
	}
}

// resinEnabledFromSite extracts the *bool from the map returned by
// LoadSiteWithEndpoints, failing the test if the type assertion fails.
func resinEnabledFromSite(t *testing.T, site map[string]any) *bool {
	t.Helper()
	v, ok := site["resinEnabled"].(*bool)
	if !ok {
		t.Fatalf("resinEnabled = %v, expected *bool", site["resinEnabled"])
	}
	return v
}

func useUtlsFromSite(t *testing.T, site map[string]any) *bool {
	t.Helper()
	v, ok := site["useUtls"].(*bool)
	if !ok {
		t.Fatalf("useUtls = %v, expected *bool", site["useUtls"])
	}
	return v
}

// ---- CreateSite round-trip tests ----

func TestCreateSite_ResinEnabledTrue_ReadsBack(t *testing.T) {
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	data := baseSiteData("ResinTrue")
	data["resinEnabled"] = true

	id, err := CreateSite(db.DB, data)
	if err != nil {
		t.Fatalf("CreateSite: %v", err)
	}

	site, err := LoadSiteWithEndpoints(db.DB, id)
	if err != nil {
		t.Fatalf("LoadSiteWithEndpoints: %v", err)
	}
	if site == nil {
		t.Fatal("site not found after create")
	}

	resin := resinEnabledFromSite(t, site)
	if resin == nil || !*resin {
		t.Fatalf("resinEnabled = %v, want *bool(true)", resin)
	}
}

func TestCreateSite_UseUtlsTrue_ReadsBack(t *testing.T) {
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	data := baseSiteData("UtlsTrue")
	data["useUtls"] = true

	id, err := CreateSite(db.DB, data)
	if err != nil {
		t.Fatalf("CreateSite: %v", err)
	}

	site, err := LoadSiteWithEndpoints(db.DB, id)
	if err != nil {
		t.Fatalf("LoadSiteWithEndpoints: %v", err)
	}
	if site == nil {
		t.Fatal("site not found after create")
	}

	utls := useUtlsFromSite(t, site)
	if utls == nil || !*utls {
		t.Fatalf("useUtls = %v, want *bool(true)", utls)
	}
}

func TestCreateSite_ResinAndUtlsOmitted_ReadsBackNil(t *testing.T) {
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	data := baseSiteData("ResinNil")

	id, err := CreateSite(db.DB, data)
	if err != nil {
		t.Fatalf("CreateSite: %v", err)
	}

	site, err := LoadSiteWithEndpoints(db.DB, id)
	if err != nil {
		t.Fatalf("LoadSiteWithEndpoints: %v", err)
	}
	if site == nil {
		t.Fatal("site not found after create")
	}

	resin := resinEnabledFromSite(t, site)
	if resin != nil {
		t.Fatalf("resinEnabled = %v, want nil (inherit global)", resin)
	}
	utls := useUtlsFromSite(t, site)
	if utls != nil {
		t.Fatalf("useUtls = %v, want nil (inherit global)", utls)
	}
}

// ---- UpdateSite tests ----

func TestUpdateSite_ResinEnabledFalse_UpdatesValue(t *testing.T) {
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	data := baseSiteData("ResinUpdate")
	data["resinEnabled"] = true

	id, err := CreateSite(db.DB, data)
	if err != nil {
		t.Fatalf("CreateSite: %v", err)
	}

	if err := UpdateSite(db.DB, id, map[string]any{"resinEnabled": false}); err != nil {
		t.Fatalf("UpdateSite: %v", err)
	}

	site, err := LoadSiteWithEndpoints(db.DB, id)
	if err != nil {
		t.Fatalf("LoadSiteWithEndpoints: %v", err)
	}
	if site == nil {
		t.Fatal("site not found after update")
	}

	resin := resinEnabledFromSite(t, site)
	if resin == nil || *resin {
		t.Fatalf("resinEnabled = %v, want *bool(false)", resin)
	}
}

func TestUpdateSite_UseUtlsFalse_UpdatesValue(t *testing.T) {
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	data := baseSiteData("UtlsUpdate")
	data["useUtls"] = true

	id, err := CreateSite(db.DB, data)
	if err != nil {
		t.Fatalf("CreateSite: %v", err)
	}

	if err := UpdateSite(db.DB, id, map[string]any{"useUtls": false}); err != nil {
		t.Fatalf("UpdateSite: %v", err)
	}

	site, err := LoadSiteWithEndpoints(db.DB, id)
	if err != nil {
		t.Fatalf("LoadSiteWithEndpoints: %v", err)
	}
	if site == nil {
		t.Fatal("site not found after update")
	}

	utls := useUtlsFromSite(t, site)
	if utls == nil || *utls {
		t.Fatalf("useUtls = %v, want *bool(false)", utls)
	}
}

func TestUpdateSite_ResinEnabledOmitted_DoesNotTouchColumn(t *testing.T) {
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	data := baseSiteData("ResinOmitted")
	data["resinEnabled"] = true

	id, err := CreateSite(db.DB, data)
	if err != nil {
		t.Fatalf("CreateSite: %v", err)
	}

	// Update an unrelated field; resinEnabled is omitted from the updates map.
	if err := UpdateSite(db.DB, id, map[string]any{"status": "disabled"}); err != nil {
		t.Fatalf("UpdateSite: %v", err)
	}

	site, err := LoadSiteWithEndpoints(db.DB, id)
	if err != nil {
		t.Fatalf("LoadSiteWithEndpoints: %v", err)
	}
	if site == nil {
		t.Fatal("site not found after update")
	}

	resin := resinEnabledFromSite(t, site)
	if resin == nil || !*resin {
		t.Fatalf("resinEnabled = %v, want *bool(true) (untouched)", resin)
	}
}

func TestUpdateSite_UseUtlsOmitted_DoesNotTouchColumn(t *testing.T) {
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	data := baseSiteData("UtlsOmitted")
	data["useUtls"] = true

	id, err := CreateSite(db.DB, data)
	if err != nil {
		t.Fatalf("CreateSite: %v", err)
	}

	// Update an unrelated field; useUtls is omitted from the updates map.
	if err := UpdateSite(db.DB, id, map[string]any{"status": "disabled"}); err != nil {
		t.Fatalf("UpdateSite: %v", err)
	}

	site, err := LoadSiteWithEndpoints(db.DB, id)
	if err != nil {
		t.Fatalf("LoadSiteWithEndpoints: %v", err)
	}
	if site == nil {
		t.Fatal("site not found after update")
	}

	utls := useUtlsFromSite(t, site)
	if utls == nil || !*utls {
		t.Fatalf("useUtls = %v, want *bool(true) (untouched)", utls)
	}
}
