package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

func TestKeyRoutingAdminAndPortableBackup(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := catalogTestDB(t, dialect, ":memory:")
			router := chi.NewRouter()
			RegisterDownstreamKeysRoutes(router, db.DB)
			RegisterTokenRoutesWithDeps(router, db.DB, TokenRoutesDeps{})
			body := `{"name":"routing-key","key":"sk-fixture","supportedModels":["*"],"accessPolicy":{"routingStrategy":"least_busy","traceStickyMode":"prefer_previous_channel"}}`
			out := catalogRequest(router, http.MethodPost, "/api/downstream-keys", body)
			if out.Code != 201 && out.Code != 200 {
				t.Fatalf("create key: %d %s", out.Code, out.Body.String())
			}
			var id int64
			if err := db.Get(&id, `SELECT id FROM downstream_api_keys WHERE name='routing-key'`); err != nil {
				t.Fatal(err)
			}
			catalogCall(t, router, http.MethodPut, fmt.Sprintf("/api/downstream-keys/%d", id), map[string]any{"name": "renamed"}, 200)
			var raw string
			if err := db.Get(&raw, db.Rebind(`SELECT access_policy FROM downstream_api_keys WHERE id=?`), id); err != nil {
				t.Fatal(err)
			}
			policy, err := store.ParseDownstreamAccessPolicy(raw)
			if err != nil || policy.RoutingStrategy != "least_busy" || policy.TraceStickyMode != "prefer_previous_channel" {
				t.Fatalf("ordinary edit lost policy %+v %v", policy, err)
			}
			route := catalogRequest(router, http.MethodPost, "/api/routes", `{"modelPattern":"routing-fixture","traceStickyMode":"prefer_previous_channel"}`)
			if route.Code != 200 && route.Code != 201 {
				t.Fatalf("route create: %d %s", route.Code, route.Body.String())
			}
			var routeID int64
			if err := db.Get(&routeID, `SELECT id FROM token_routes WHERE model_pattern='routing-fixture'`); err != nil {
				t.Fatal(err)
			}
			catalogCall(t, router, http.MethodPut, fmt.Sprintf("/api/routes/%d", routeID), map[string]any{"traceStickyMode": "unknown"}, 400)
			payload, err := backup.BuildPayload(db.DB, "all")
			if err != nil {
				t.Fatal(err)
			}
			encoded, _ := json.Marshal(payload)
			tables, err := decodeBackupImportBodyFrom(encoded)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`DELETE FROM downstream_api_keys`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`DELETE FROM token_routes`); err != nil {
				t.Fatal(err)
			}
			if _, err := importBackupTables(db.DB, tables, true); err != nil {
				t.Fatal(err)
			}
			var sticky string
			if err := db.Get(&sticky, db.Rebind(`SELECT trace_sticky_mode FROM token_routes WHERE id=?`), routeID); err != nil || sticky != "prefer_previous_channel" {
				t.Fatalf("portable route policy=%s %v", sticky, err)
			}
			if err := db.Get(&raw, db.Rebind(`SELECT access_policy FROM downstream_api_keys WHERE id=?`), id); err != nil || !strings.Contains(raw, "least_busy") {
				t.Fatalf("portable key policy=%s %v", raw, err)
			}
			list := httptest.NewRecorder()
			router.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/downstream-keys", nil))
			if list.Code != 200 || !strings.Contains(list.Body.String(), "least_busy") || strings.Contains(list.Body.String(), "sk-fixture") {
				t.Fatalf("unsafe policy readback: %d %s", list.Code, list.Body.String())
			}
		})
	}
}
