package admin

import (
	"fmt"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/internal/requestoverride"
	"github.com/deliciousbuding/metapi-go/store"
)

const adminPluginSource = `export const meta={apiVersion:1,key:"fixture",name:"Fixture",version:"1.0.0",protocols:["chat"],hooks:["request"]};export function request(){return {};}`

func TestChannelPluginManagementAPIAndPrivateBindingPatch(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := catalogTestDB(t, dialect, ":memory:")
			mux := catalogMux(db)
			catalogCall(t, mux, "PUT", "/api/channel-plugins/fixture", map[string]any{"source": adminPluginSource, "enabled": true, "expectedRevision": 0}, 200)
			list := catalogRequest(mux, "GET", "/api/channel-plugins", nil)
			if list.Code != 200 || strings.Contains(list.Body.String(), "export const") || list.Header().Get("Cache-Control") != noStoreCacheControl {
				t.Fatalf("unsafe list: %d %s", list.Code, list.Body.String())
			}
			catalogCall(t, mux, "PUT", "/api/channel-plugins/fixture", map[string]any{"source": adminPluginSource, "enabled": true, "expectedRevision": 0}, 409)
			catalogCall(t, mux, "DELETE", "/api/channel-plugins/fixture", map[string]any{"expectedRevision": 1}, 409)
			channel := catalogCall(t, mux, "POST", catalogPrefix, catalogChannelInput("Fixture", "https://example.invalid"), 201)
			id := catalogID(channel)
			secret := "synthetic-private-prompt"
			config := store.DirectRequestConfig{Config: requestoverride.Config{Body: []requestoverride.Operation{{Op: "set", Path: "system", Value: secret}}}}
			if _, err := db.Exec(db.Rebind(`UPDATE upstream_channels SET request_config=? WHERE id=?`), config, id); err != nil {
				t.Fatal(err)
			}
			path := fmt.Sprintf("%s/%d", catalogPrefix, id)
			catalogCall(t, mux, "PATCH", path, map[string]any{"pluginBindings": []any{map[string]any{"key": "fixture", "protocols": []string{"chat"}}}}, 200)
			var saved store.DirectRequestConfig
			if err := db.Get(&saved, db.Rebind(`SELECT request_config FROM upstream_channels WHERE id=?`), id); err != nil || len(saved.Body) != 1 || saved.Body[0].Value != secret || len(saved.Plugins) != 1 {
				t.Fatalf("private transform lost: %+v %v", saved, err)
			}
			read := catalogRequest(mux, "GET", path+"/request-config", nil)
			if read.Code != 200 || strings.Contains(read.Body.String(), secret) || !strings.Contains(read.Body.String(), "pluginBindings") {
				t.Fatalf("unsafe config read: %s", read.Body.String())
			}
			catalogCall(t, mux, "PATCH", path, `{"pluginBindings":[],"requestTransform":{}}`, 400)
			catalogCall(t, mux, "PATCH", path, `{"pluginBindings":[{"key":"missing","protocols":["chat"]}]}`, 400)
			catalogCall(t, mux, "PATCH", "/api/channel-plugins/fixture", map[string]any{"enabled": false, "expectedRevision": 1}, 200)
			catalogCall(t, mux, "PATCH", "/api/channel-plugins/fixture", map[string]any{"enabled": true, "expectedRevision": 1}, 409)
			catalogCall(t, mux, "DELETE", "/api/channel-plugins/fixture", map[string]any{"expectedRevision": 2}, 200)
			catalogCall(t, mux, "GET", "/api/channel-plugins/fixture", nil, 404)
		})
	}
}

func TestChannelPluginAPIErrorsDoNotEchoSource(t *testing.T) {
	db := catalogTestDB(t, store.DialectSQLite, ":memory:")
	mux := catalogMux(db)
	for _, source := range []string{`export const syntax=; // synthetic-private-code`, `throw new Error("synthetic-private-code");` + adminPluginSource} {
		out := catalogRequest(mux, "PUT", "/api/channel-plugins/fixture", map[string]any{"source": source, "enabled": true, "expectedRevision": 0})
		if out.Code != 400 || strings.Contains(out.Body.String(), "synthetic-private-code") {
			t.Fatalf("unsafe failure: %d %s", out.Code, out.Body.String())
		}
	}
}
