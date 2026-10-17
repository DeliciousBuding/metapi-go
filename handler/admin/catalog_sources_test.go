package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/sqlitetest"
	"github.com/deliciousbuding/metapi-go/service/catalogsync"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

func TestCatalogDisabledStatusSQLite(t *testing.T) { testCatalogDisabledStatus(t, sqlitetest.Open(t)) }

func TestCatalogDisabledStatusPostgres(t *testing.T) {
	db, _ := setupSitesPostgresTest(t)
	testCatalogDisabledStatus(t, db)
}

func testCatalogDisabledStatus(t *testing.T, db *store.DB) {
	t.Helper()
	previous := config.GetSafe()
	config.Set(&config.Config{PricingCatalogEnabled: false})
	t.Cleanup(func() { config.Set(previous) })
	r := chi.NewRouter()
	RegisterCatalogSourceRoutes(r, db.DB)
	created := doPostJSON(t, r, "/api/models/catalog-sources", map[string]any{"name": "Fixture", "url": "https://fixture.example/catalog.json"})
	if created.Code != 200 {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	get := func(want int) *httptest.ResponseRecorder {
		t.Helper()
		out := httptest.NewRecorder()
		r.ServeHTTP(out, httptest.NewRequest(http.MethodGet, "/api/models/catalog-sync", nil))
		if out.Code != want {
			t.Fatalf("status: %d %s, want %d", out.Code, out.Body.String(), want)
		}
		return out
	}
	out := get(200)
	var wire struct {
		Enabled  *bool                `json:"enabled"`
		AutoSync bool                 `json:"autoSync"`
		Sources  []catalogsync.Source `json:"sources"`
	}
	if err := json.Unmarshal(out.Body.Bytes(), &wire); err != nil {
		t.Fatal(err)
	}
	if wire.Enabled == nil || *wire.Enabled || wire.AutoSync || len(wire.Sources) != 1 || wire.Sources[0].Name != "Fixture" {
		t.Fatalf("disabled inventory: %s", out.Body.String())
	}
	id := wire.Sources[0].ID
	update := doPutJSON(t, r, "/api/models/catalog-sources/"+itoa(id), map[string]any{"name": "Renamed", "enabled": false})
	if update.Code != 200 {
		t.Fatalf("update: %d %s", update.Code, update.Body.String())
	}
	out = get(200)
	if err := json.Unmarshal(out.Body.Bytes(), &wire); err != nil {
		t.Fatal(err)
	}
	if wire.Sources[0].Name != "Renamed" || wire.Sources[0].Enabled {
		t.Fatalf("updated source missing: %s", out.Body.String())
	}
	for _, method := range []string{http.MethodPost, http.MethodPut} {
		path := "/api/models/catalog-sync"
		if method == http.MethodPut {
			path += "/config"
		}
		res := httptest.NewRecorder()
		r.ServeHTTP(res, httptest.NewRequest(method, path, nil))
		if res.Code != 409 {
			t.Fatalf("disabled mutation: %s %d", path, res.Code)
		}
	}
	deleted := httptest.NewRecorder()
	r.ServeHTTP(deleted, httptest.NewRequest(http.MethodDelete, "/api/models/catalog-sources/"+itoa(id), nil))
	if deleted.Code != 200 {
		t.Fatalf("delete: %d", deleted.Code)
	}
	out = get(200)
	if err := json.Unmarshal(out.Body.Bytes(), &wire); err != nil {
		t.Fatal(err)
	}
	if wire.Sources == nil || len(wire.Sources) != 0 {
		t.Fatalf("empty inventory must be an array: %s", out.Body.String())
	}
	config.Set(&config.Config{PricingCatalogEnabled: true})
	get(503) // A failed/unavailable runtime must not be reported as deliberately disabled.
	config.Set(&config.Config{PricingCatalogEnabled: false})
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	get(500)
}
