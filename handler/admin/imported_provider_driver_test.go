package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/store"
)

const adminProviderSource = `export const meta={apiVersion:2,kind:"provider",key:"vendor-driver",name:"Vendor",version:"1.0.0",protocols:["chat"],credentialKinds:["api_key"],hooks:["request","response"],options:[{key:"mode",label:"Mode",type:"select",default:"safe",choices:["safe","fast"]}]};export function request(input){return {method:"POST",body:input.body};}export function response(input){return {body:input.body};}`

func TestProviderBindingQuickConnectUsesConfiguredCatalog(t *testing.T) {
	const source = `export const meta={apiVersion:2,kind:"provider",key:"connect-driver",name:"Connect",version:"1.0.0",protocols:["chat","responses"],credentialKinds:["api_key"],hooks:["request","response","modelsRequest","modelsResponse"],defaults:{endpoints:{responses:{path:"/native/responses",auth:"bearer"}},catalog:{path:"/inventory",auth:"x-api-key"}}};export function request(i){return {method:"POST",body:i.body};}export function response(i){return {body:i.body};}export function modelsRequest(){return {method:"GET"};}export function modelsResponse(i){return {models:JSON.parse(i.body).items};}`
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := catalogTestDB(t, dialect, ":memory:")
			if _, err := channelplugins.Save(t.Context(), db.DB, "connect-driver", source, true, 0); err != nil {
				t.Fatal(err)
			}
			var requests int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.URL.Path != "/inventory" || r.Method != "GET" || r.Header.Get("X-Api-Key") != "fixture-connect-key" || r.Header.Get("Authorization") != "" {
					t.Errorf("wrong catalog request: %s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"items":["fixture-provider-model"]}`)
			}))
			defer server.Close()
			router := catalogMux(db)
			body := map[string]any{"presetId": "new-api-connection", "baseUrl": server.URL, "apiKey": "fixture-connect-key", "protocols": []string{"chat"}, "providerBinding": map[string]any{"key": "connect-driver", "protocols": []string{"chat"}}}
			out := catalogCall(t, router, "POST", connectPath, body, 201)
			if requests != 1 || out["modelCount"] != float64(1) || out["discovery"].(map[string]any)["status"] != "discovered" {
				t.Fatalf("driver catalog was not used: %+v requests=%d", out, requests)
			}
			id := catalogID(out)
			var raw string
			if err := db.Get(&raw, db.Rebind(`SELECT request_config FROM upstream_channels WHERE id=?`), id); err != nil || !strings.Contains(raw, "connect-driver") || !strings.Contains(raw, server.URL+"/inventory") {
				t.Fatalf("quick connect lost provider binding: %s %v", raw, err)
			}
			body["providerBinding"] = map[string]any{"key": "connect-driver", "protocols": []string{"responses"}}
			catalogCall(t, router, "POST", connectPath, body, 400)
			if requests != 1 {
				t.Fatal("invalid binding reached discovery")
			}
			var channels int
			if err := db.Get(&channels, `SELECT COUNT(*) FROM upstream_channels`); err != nil || channels != 1 {
				t.Fatalf("invalid binding persisted a channel: %d %v", channels, err)
			}
		})
	}
}

func TestProviderDefaultsPersistWithoutOverwritingCustomEndpoints(t *testing.T) {
	const source = `export const meta={apiVersion:2,kind:"provider",key:"defaults",name:"Defaults",version:"1.0.0",protocols:["chat"],credentialKinds:["api_key"],hooks:["request","response","modelsRequest","modelsResponse"],defaults:{endpoints:{chat:{path:"/native/generate",auth:"bearer"}},catalog:{path:"/inventory",auth:"bearer",cursorParameter:"cursor"}}};export function request(i){return {method:"POST",body:i.body};}export function response(i){return {body:i.body};}export function modelsRequest(){return {method:"GET"};}export function modelsResponse(i){return {models:JSON.parse(i.body).items};}`
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := catalogTestDB(t, dialect, ":memory:")
			if _, err := channelplugins.Save(t.Context(), db.DB, "defaults", source, true, 0); err != nil {
				t.Fatalf("valid provider defaults cannot be installed: %v", err)
			}
			router := catalogMux(db)
			input := map[string]any{"name": "Defaults", "provider": "custom", "baseUrl": "https://fixture.invalid/tenant", "enabled": true, "endpointConfig": map[string]any{}, "requestTransform": map[string]any{"provider": map[string]any{"key": "defaults", "protocols": []string{"chat"}}}}
			id := catalogID(catalogCall(t, router, "POST", catalogPrefix, input, 201))
			var endpoints, requestConfig string
			if err := db.QueryRow(db.Rebind(`SELECT endpoint_config,request_config FROM upstream_channels WHERE id=?`), id).Scan(&endpoints, &requestConfig); err != nil || !strings.Contains(endpoints, "https://fixture.invalid/tenant/native/generate") || !strings.Contains(requestConfig, "https://fixture.invalid/tenant/inventory") {
				t.Fatalf("defaults did not become saved configuration: %s %s %v", endpoints, requestConfig, err)
			}
			if _, err := channelplugins.Save(t.Context(), db.DB, "defaults", strings.ReplaceAll(source, "/native/generate", "/changed"), true, 1); err != nil {
				t.Fatal(err)
			}
			var unchanged string
			if err := db.Get(&unchanged, db.Rebind(`SELECT endpoint_config FROM upstream_channels WHERE id=?`), id); err != nil || unchanged != endpoints {
				t.Fatalf("upgrade changed saved target: %s %v", unchanged, err)
			}
			input["name"] = "Custom"
			input["endpointConfig"] = map[string]any{"chat": map[string]any{"url": "https://fixture.invalid/custom", "auth": "x-api-key"}}
			input["requestTransform"] = map[string]any{"provider": map[string]any{"key": "defaults", "protocols": []string{"chat"}, "catalog": map[string]any{"url": "https://fixture.invalid/custom-models", "auth": "x-api-key"}}}
			id = catalogID(catalogCall(t, router, "POST", catalogPrefix, input, 201))
			if err := db.QueryRow(db.Rebind(`SELECT endpoint_config,request_config FROM upstream_channels WHERE id=?`), id).Scan(&endpoints, &requestConfig); err != nil || !strings.Contains(endpoints, "https://fixture.invalid/custom") || !strings.Contains(requestConfig, "https://fixture.invalid/custom-models") {
				t.Fatalf("custom defaults overwritten: %s %s %v", endpoints, requestConfig, err)
			}
		})
	}
}

func TestProviderBindingPatchPreservesPrivateOperations(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := catalogTestDB(t, dialect, ":memory:")
			if _, err := channelplugins.Save(t.Context(), db.DB, "vendor-driver", adminProviderSource, true, 0); err != nil {
				t.Fatal(err)
			}
			router := catalogMux(db)
			input := catalogChannelInput("Provider binding", "https://fixture.invalid")
			input["requestTransform"] = map[string]any{"headers": []map[string]string{{"op": "set", "path": "X-Private", "value": "synthetic-private-operation"}}}
			id := catalogID(catalogCall(t, router, "POST", catalogPrefix, input, 201))
			path := fmt.Sprintf("%s/%d", catalogPrefix, id)
			catalogCall(t, router, "PATCH", path, `{"providerBinding":{"key":"vendor-driver","protocols":["chat"],"options":{"mode":"fast"}}}`, 200)
			response := catalogRequest(router, "GET", path+"/request-config", nil)
			if response.Code != 200 || !strings.Contains(response.Body.String(), `"providerBinding"`) || !strings.Contains(response.Body.String(), `"vendor-driver"`) || strings.Contains(response.Body.String(), "synthetic-private-operation") {
				t.Fatalf("unsafe provider binding readback: %d %s", response.Code, response.Body.String())
			}
			var raw string
			if err := db.Get(&raw, db.Rebind(`SELECT request_config FROM upstream_channels WHERE id=?`), id); err != nil || !strings.Contains(raw, "synthetic-private-operation") || !strings.Contains(raw, "vendor-driver") {
				t.Fatalf("provider patch discarded operations: %s %v", raw, err)
			}
			var grants int
			if err := db.Get(&grants, `SELECT COUNT(*) FROM upstream_grants`); err != nil || grants != 0 {
				t.Fatalf("binding created grants: %d %v", grants, err)
			}
			for _, patch := range []string{
				`{"providerBinding":{"key":"vendor-driver","protocols":["responses"]}}`,
				`{"providerBinding":{"key":"vendor-driver","protocols":["imageGeneration"]}}`,
				`{"providerBinding":{"key":"vendor-driver","protocols":["chat"],"options":{"mode":"invalid"}}}`,
				`{"providerBinding":{"key":"vendor-driver","protocols":["chat"]},"requestTransform":{}}`,
				`{"endpointConfig":{"responses":{"url":"https://fixture.invalid/responses","auth":"bearer"}}}`,
				`{"providerBinding":{"key":"vendor-driver","protocols":["chat"],"options":{"mode":null}}}`,
				`{"name":null}`,
				`{"pluginBindings":null}`,
				`{"requestTransform":null}`,
			} {
				catalogCall(t, router, "PATCH", path, patch, 400)
				var unchanged string
				if err := db.Get(&unchanged, db.Rebind(`SELECT request_config FROM upstream_channels WHERE id=?`), id); err != nil || unchanged != raw {
					t.Fatalf("rejected provider patch changed config: %s %v", unchanged, err)
				}
			}
			catalogCall(t, router, "PATCH", path, `{"name":"Renamed provider"}`, 200)
			var unchanged string
			if err := db.Get(&unchanged, db.Rebind(`SELECT request_config FROM upstream_channels WHERE id=?`), id); err != nil || unchanged != raw {
				t.Fatalf("ordinary save discarded provider: %s %v", unchanged, err)
			}
			payload, err := backup.BuildPayload(db.DB, "all")
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			tables, err := decodeBackupImportBodyFrom(encoded)
			if err != nil {
				t.Fatal(err)
			}
			if _, found := tables["channel_plugins"]; found {
				t.Fatal("portable backup would install executable code")
			}
			catalogCall(t, router, "PATCH", path, `{"providerBinding":null,"pluginBindings":[]}`, 200)
			if err := db.Get(&unchanged, db.Rebind(`SELECT request_config FROM upstream_channels WHERE id=?`), id); err != nil || strings.Contains(unchanged, "vendor-driver") || !strings.Contains(unchanged, "synthetic-private-operation") {
				t.Fatalf("clearing provider lost unrelated operations: %s %v", unchanged, err)
			}
			if _, err := db.Exec(db.Rebind(`DELETE FROM upstream_channels WHERE id=?`), id); err != nil {
				t.Fatal(err)
			}
			if _, err := importBackupTables(db.DB, tables, true); err != nil {
				t.Fatal(err)
			}
			if err := db.Get(&unchanged, db.Rebind(`SELECT request_config FROM upstream_channels WHERE id=?`), id); err != nil || !strings.Contains(unchanged, "vendor-driver") || !strings.Contains(unchanged, "synthetic-private-operation") {
				t.Fatalf("portable backup lost provider binding: %s %v", unchanged, err)
			}
			created := catalogChannelInput("Created provider", "https://fixture.invalid")
			created["requestTransform"] = map[string]any{"provider": map[string]any{"key": "vendor-driver", "protocols": []string{"chat"}}}
			createdID := catalogID(catalogCall(t, router, "POST", catalogPrefix, created, 201))
			if err := db.Get(&unchanged, db.Rebind(`SELECT request_config FROM upstream_channels WHERE id=?`), createdID); err != nil || !strings.Contains(unchanged, "vendor-driver") {
				t.Fatalf("create producer lost provider binding: %s %v", unchanged, err)
			}
		})
	}
}
