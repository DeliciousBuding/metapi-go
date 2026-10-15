package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

func TestQueryRowErrorContract(t *testing.T) {
	for _, fixture := range []struct {
		name  string
		setup func(*testing.T) (*store.DB, chi.Router)
	}{
		{"SQLite", setupStatsSQLiteTest},
		{"Postgres", setupStatsPostgresTest},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			db, router := fixture.setup(t)
			RegisterDownstreamKeysRoutes(router, db.DB)
			RegisterTokenRoutesWithDeps(router, db.DB, TokenRoutesDeps{})

			row, err := queryRow(db.DB, "SELECT ? AS site_name, NULL AS optional_value", "example")
			if err != nil || row["siteName"] != "example" || row["optionalValue"] != nil {
				t.Fatalf("camelCase/null projection: row=%v err=%v", row, err)
			}
			row, err = queryRow(db.DB, "SELECT id FROM downstream_api_keys WHERE id = ?", -1)
			if err != nil || row != nil {
				t.Fatalf("missing record: row=%v err=%v", row, err)
			}
			if _, err := queryRow(db.DB, "SELECT * FROM missing_query_row_fixture"); err == nil {
				t.Fatal("invalid SQL must not look like a missing record")
			}

			requests := []struct{ method, path string }{
				{http.MethodGet, "/api/downstream-keys/1/export"},
				{http.MethodGet, "/api/downstream-keys/1/overview"},
				{http.MethodGet, "/api/downstream-keys/1/trend"},
				{http.MethodPut, "/api/downstream-keys/1"},
				{http.MethodPut, "/api/routes/1"},
				{http.MethodGet, "/api/stats/proxy-logs/1"},
			}
			for _, request := range requests {
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, httptest.NewRequest(request.method, request.path, strings.NewReader("{}")))
				if rec.Code != http.StatusNotFound {
					t.Fatalf("missing %s: %d %s", request.path, rec.Code, rec.Body.String())
				}
			}

			h := &downstreamKeysHandler{db: db.DB}
			for _, kind := range []string{"account_token", "default_api_key"} {
				refs := []any{map[string]any{"kind": kind, "tokenId": 1, "accountId": 1, "siteId": 1}}
				message, err := h.validateDownstreamPolicyReferences(nil, nil, nil, nil, nil, refs)
				if err != nil || message == "" {
					t.Fatalf("missing %s reference: message=%q err=%v", kind, message, err)
				}
			}

			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := queryRow(db.DB, "SELECT 1"); err == nil {
				t.Fatal("closed database must return an error")
			}
			for _, request := range requests {
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, httptest.NewRequest(request.method, request.path, strings.NewReader("{}")))
				body := decodeGrantBody(t, rec.Body.Bytes())
				if rec.Code != http.StatusInternalServerError || body["errorCode"] != ErrorCodeResourceLoadFailed {
					t.Fatalf("unavailable DB %s: %d %s", request.path, rec.Code, rec.Body.String())
				}
				if strings.Contains(rec.Body.String(), "database is closed") {
					t.Fatalf("driver error leaked: %s", rec.Body.String())
				}
			}
			for _, kind := range []string{"account_token", "default_api_key"} {
				refs := []any{map[string]any{"kind": kind, "tokenId": 1, "accountId": 1, "siteId": 1}}
				message, err := h.validateDownstreamPolicyReferences(nil, nil, nil, nil, nil, refs)
				if err == nil || message != "" {
					t.Fatalf("DB failure misclassified as unknown %s: message=%q err=%v", kind, message, err)
				}
			}
			rec := doPostJSON(t, router, "/api/downstream-keys/batch", map[string]any{"action": "enable", "ids": []int64{1, 2}})
			body := decodeGrantBody(t, rec.Body.Bytes())
			failures, ok := body["failedItems"].([]any)
			if rec.Code != http.StatusOK || !ok || len(failures) != 2 {
				t.Fatalf("batch lost item results: %d %s", rec.Code, rec.Body.String())
			}
			for _, failure := range failures {
				if failure.(map[string]any)["message"] != "failed to load API key" {
					t.Fatalf("batch outage misclassified as missing: %v", failure)
				}
			}
		})
	}
}

func TestRouteReadbackFailureDoesNotPanicOrUndoWrite(t *testing.T) {
	db, router := setupTokenRoutesTest(t)
	routeID, _, _ := seedRouteChannelRefs(t, db)
	requireCacheState(t, router, "/api/channels", "miss")
	requireCacheState(t, router, "/api/channels", "hit")
	_, err := db.Exec(`CREATE TRIGGER remove_updated_route AFTER UPDATE OF display_name ON token_routes
		BEGIN DELETE FROM token_routes WHERE id = NEW.id; END`)
	if err != nil {
		t.Fatal(err)
	}
	rec := doPutJSON(t, router, "/api/routes/"+itoa(routeID), map[string]any{"displayName": "changed"})
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), ErrorCodeResourceLoadFailed) {
		t.Fatalf("readback failure: %d %s", rec.Code, rec.Body.String())
	}
	var count int
	if err := db.Get(&count, "SELECT COUNT(*) FROM token_routes WHERE id = ?", routeID); err != nil || count != 0 {
		t.Fatalf("write must remain committed: count=%d err=%v", count, err)
	}
	requireCacheState(t, router, "/api/channels", "miss")
}

func TestChannelBatchReadbackFailurePreservesWriteOutcomes(t *testing.T) {
	db, router := setupTokenRoutesTest(t)
	routeID, accountID, tokenID := seedRouteChannelRefs(t, db)
	ids := seedBatchUpdateChannels(t, db, routeID, accountID, tokenID, 2)
	requireCacheState(t, router, "/api/channels", "miss")
	requireCacheState(t, router, "/api/channels", "hit")
	_, err := db.Exec(`CREATE TRIGGER remove_updated_channel AFTER UPDATE ON route_channels
		BEGIN DELETE FROM route_channels WHERE id = NEW.id; END`)
	if err != nil {
		t.Fatal(err)
	}
	rec := doPutJSON(t, router, "/api/channels/batch", map[string]any{
		"updates": []map[string]any{{"id": ids[0], "enabled": false}, {"id": ids[1], "enabled": false}, {"id": -1, "enabled": false}},
	})
	body := decodeGrantBody(t, rec.Body.Bytes())
	if rec.Code != http.StatusInternalServerError || body["errorCode"] != ErrorCodeResourceLoadFailed {
		t.Fatalf("batch readback failure: %d %s", rec.Code, rec.Body.String())
	}
	if got := body["successIds"].([]any); len(got) != 2 || int64(got[0].(float64)) != ids[0] || int64(got[1].(float64)) != ids[1] {
		t.Fatalf("committed successes lost: %v", body)
	}
	if len(body["failedItems"].([]any)) != 1 {
		t.Fatalf("write failure lost: %v", body)
	}
	var count int
	if err := db.Get(&count, "SELECT COUNT(*) FROM route_channels WHERE route_id = ?", routeID); err != nil || count != 0 {
		t.Fatalf("writes must remain committed: count=%d err=%v", count, err)
	}
	items, _, _, _ := channelsCacheEnvelope(t, requireCacheState(t, router, "/api/channels", "miss"))
	if len(items) != 0 {
		t.Fatalf("stale channels remained after committed writes: %v", items)
	}
}
