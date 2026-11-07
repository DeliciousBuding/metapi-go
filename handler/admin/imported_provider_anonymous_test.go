package admin

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/store"
)

const anonymousCatalogProvider = `export const meta={apiVersion:2,kind:"provider",key:"anonymous",name:"Anonymous",version:"1.0.0",protocols:["messages"],credentialKinds:["none"],hooks:["request","response","modelsRequest","modelsResponse"],defaults:{catalog:{path:"/inventory",auth:"none"}}};export function request(i){return {method:"POST",body:i.body};}export function response(i){return {body:i.body};}export function modelsRequest(){return {method:"GET"};}export function modelsResponse(i){return {models:JSON.parse(i.body).items};}`

func TestProviderAnonymousCatalogConnectRefreshAndRejection(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := catalogTestDB(t, dialect, ":memory:")
			if _, err := channelplugins.Save(t.Context(), db.DB, "anonymous", anonymousCatalogProvider, true, 0); err != nil {
				t.Fatalf("anonymous provider cannot be installed: %v", err)
			}
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != "GET" || r.URL.Path != "/inventory" {
					t.Errorf("wrong catalog operation: %s %s", r.Method, r.URL.Path)
				}
				for _, key := range []string{"Authorization", "X-Api-Key", "X-Goog-Api-Key", "Cookie", "Proxy-Authorization"} {
					if r.Header.Get(key) != "" {
						t.Errorf("anonymous catalog sent %s", key)
					}
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"items":["local-model"]}`)
			}))
			defer server.Close()
			router := catalogMux(db)
			body := map[string]any{"presetId": "ollama-anthropic", "baseUrl": server.URL, "protocols": []string{"messages"}, "providerBinding": map[string]any{"key": "anonymous", "protocols": []string{"messages"}}}
			out := catalogCall(t, router, "POST", connectPath, body, 201)
			id := catalogID(out)
			if calls.Load() != 1 || out["modelCount"] != float64(1) {
				t.Fatalf("anonymous connect did not discover models: %+v calls=%d", out, calls.Load())
			}
			var kind, secret string
			if err := db.QueryRow(db.Rebind(`SELECT kind,secret FROM upstream_credentials WHERE channel_id=?`), id).Scan(&kind, &secret); err != nil || kind != store.DirectCredentialNone || secret != "" {
				t.Fatalf("anonymous identity changed: kind=%s err=%v", kind, err)
			}
			path := fmt.Sprintf("%s/%d/models/refresh-preview", catalogPrefix, id)
			catalogCall(t, router, "POST", path, map[string]any{}, 200)
			if calls.Load() != 2 {
				t.Fatal("stored anonymous credential was not resolved for refresh")
			}
			// An explicitly anonymous catalog cannot turn an API key (or a
			// failed resolver) into successful credential-free discovery.
			if _, err := db.Exec(db.Rebind(`UPDATE upstream_credentials SET enabled=? WHERE channel_id=?`), false, id); err != nil {
				t.Fatal(err)
			}
			catalogCall(t, router, "POST", path, map[string]any{}, 400)
			if calls.Load() != 2 {
				t.Fatal("disabled anonymous credential reached the catalog")
			}
			body["apiKey"] = "fixture-not-anonymous"
			catalogCall(t, router, "POST", connectPath, body, 400)
			delete(body, "apiKey")
			body["presetId"] = "new-api-connection"
			catalogCall(t, router, "POST", connectPath, body, 400)
			if calls.Load() != 2 {
				t.Fatal("manifest broadened anonymous credential policy")
			}
			if _, err := channelplugins.Save(t.Context(), db.DB, "anonymous", strings.Replace(anonymousCatalogProvider, `auth:"none"`, `auth:"bearer"`, 1), true, 1); err != nil {
				t.Fatal(err)
			}
			body["presetId"] = "ollama-anthropic"
			body["providerBinding"] = map[string]any{"key": "anonymous", "protocols": []string{"messages"}, "catalog": map[string]any{"url": server.URL + "/inventory", "auth": "bearer"}}
			catalogCall(t, router, "POST", connectPath, body, 400)
			if calls.Load() != 2 {
				t.Fatal("empty identity authorized an authenticated catalog")
			}
		})
	}
}
