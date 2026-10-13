package admin

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

// HTTP-level contract for the catalog refresh endpoints (route-decision E
// batch): preview is read-only, apply requires a fresh basis token, and the
// applied result matches the previewed plan exactly.

func TestCatalogRefreshEndpointsRoundTrip(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := catalogTestDB(t, dialect, filepath.Join(t.TempDir(), "refresh.db"))
			mux := catalogMux(db)

			live := []string{"m1", "m2"}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer fixture-refresh-key" {
					w.WriteHeader(401)
					return
				}
				parts := make([]string, 0, len(live))
				for _, m := range live {
					parts = append(parts, `{"id":"`+m+`"}`)
				}
				fmt.Fprint(w, `{"data":[`+strings.Join(parts, ",")+`]}`)
			}))
			t.Cleanup(server.Close)

			created := catalogCall(t, mux, "POST", catalogPrefix, catalogChannelInput("refresh-fixture", server.URL), 201)
			channelID := catalogID(created)
			catalogCall(t, mux, "POST", fmt.Sprintf(catalogPrefix+"/%d/credentials", channelID), map[string]any{"name": "Default", "kind": "api_key", "apiKey": "fixture-refresh-key", "enabled": true}, 201)
			// Operator-owned row; refresh must never touch it.
			catalogCreateModel(t, mux, channelID, "manual-keep")

			preview := catalogCall(t, mux, "POST", fmt.Sprintf(catalogPrefix+"/%d/models/refresh-preview", channelID), nil, 200)
			if preview["basisToken"] == "" {
				t.Fatal("preview must return a basisToken")
			}
			added := preview["added"].([]any)
			if len(added) != 2 {
				t.Fatalf("added: %#v", added)
			}
			if preview["legacyNativeCount"].(float64) != 1 {
				t.Fatalf("legacyNativeCount: %#v", preview["legacyNativeCount"])
			}
			// Preview is read-only: the catalog still has only the manual row.
			models := catalogCall(t, mux, "GET", fmt.Sprintf(catalogPrefix+"/%d/models", channelID), nil, 200)
			if len(models["items"].([]any)) != 1 {
				t.Fatalf("preview must not write: %#v", models)
			}

			applied := catalogCall(t, mux, "POST", fmt.Sprintf(catalogPrefix+"/%d/models/refresh-apply", channelID), map[string]any{"basisToken": preview["basisToken"]}, 200)
			if applied["addedCount"].(float64) != 2 || applied["staleCount"].(float64) != 0 {
				t.Fatalf("applied: %#v", applied)
			}

			// Upstream moves: m1 disappears, m4 appears.
			live = []string{"m2", "m4"}
			second := catalogCall(t, mux, "POST", fmt.Sprintf(catalogPrefix+"/%d/models/refresh-preview", channelID), nil, 200)
			stale := second["stale"].([]any)
			if len(stale) != 1 || stale[0].(map[string]any)["name"] != "m1" {
				t.Fatalf("stale: %#v", stale)
			}
			if len(second["added"].([]any)) != 1 {
				t.Fatalf("added after move: %#v", second["added"])
			}
			catalogCall(t, mux, "POST", fmt.Sprintf(catalogPrefix+"/%d/models/refresh-apply", channelID), map[string]any{"basisToken": second["basisToken"]}, 200)

			// m1 disabled but kept; manual-keep still enabled.
			models = catalogCall(t, mux, "GET", fmt.Sprintf(catalogPrefix+"/%d/models", channelID), nil, 200)
			byName := map[string]map[string]any{}
			for _, item := range models["items"].([]any) {
				m := item.(map[string]any)
				byName[m["name"].(string)] = m
			}
			if byName["m1"]["enabled"].(bool) {
				t.Fatal("m1 must be disabled (stale), not deleted")
			}
			if !byName["manual-keep"]["enabled"].(bool) {
				t.Fatal("manual-keep must stay enabled")
			}
			if !byName["m4"]["enabled"].(bool) {
				t.Fatal("m4 must exist and be enabled")
			}
		})
	}
}

func TestCatalogRefreshEndpointsValidation(t *testing.T) {
	db := catalogTestDB(t, store.DialectSQLite, filepath.Join(t.TempDir(), "refresh-validation.db"))
	mux := catalogMux(db)

	// Unknown channel: 404 on both endpoints.
	catalogCall(t, mux, "POST", catalogPrefix+"/9999/models/refresh-preview", nil, 404)
	catalogCall(t, mux, "POST", catalogPrefix+"/9999/models/refresh-apply", map[string]any{"basisToken": "x"}, 404)

	// Missing basisToken: 400.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[{"id":"m1"}]}`)
	}))
	t.Cleanup(server.Close)
	created := catalogCall(t, mux, "POST", catalogPrefix, catalogChannelInput("refresh-validation", server.URL), 201)
	channelID := catalogID(created)
	catalogCall(t, mux, "POST", fmt.Sprintf(catalogPrefix+"/%d/models/refresh-apply", channelID), map[string]any{}, 400)

	// No credential while the endpoint requires bearer auth: 400, not a guess.
	catalogCall(t, mux, "POST", fmt.Sprintf(catalogPrefix+"/%d/models/refresh-preview", channelID), nil, 400)
}
