package admin

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/internal/requestoverride"
	"github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestImportedRequestOperationsAdminAndPortableBackup(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := catalogTestDB(t, dialect, ":memory:")
			router := catalogMux(db)
			config := store.DirectRequestConfig{Config: requestoverride.Config{
				Body:    []requestoverride.Operation{{Op: "set", Path: "temperature", Value: "0.2"}},
				Headers: []requestoverride.Operation{{Op: "set", Path: "X-Private", Value: "synthetic-private-value"}},
			}}
			response := catalogCall(t, router, "POST", catalogPrefix, map[string]any{
				"name": "Operation channel", "provider": "qiniu", "dialect": "generic", "enabled": true,
				"baseUrl":          "https://fixture.invalid",
				"endpointConfig":   store.DirectEndpoints{Chat: &store.DirectEndpoint{URL: "https://fixture.invalid/chat", Auth: store.DirectAuthBearer}},
				"requestTransform": config,
			}, 201)
			id := int64(response["id"].(float64))
			path := fmt.Sprintf("%s/%d", catalogPrefix, id)
			out := catalogRequest(router, "GET", path+"/request-config", nil)
			if out.Code != 200 || !strings.Contains(out.Body.String(), `"requestTransformSummary"`) || strings.Contains(out.Body.String(), "synthetic-private-value") {
				t.Fatalf("unsafe request readback: %d %s", out.Code, out.Body.String())
			}
			var readback map[string]json.RawMessage
			_ = json.Unmarshal(out.Body.Bytes(), &readback)
			if _, ok := readback["requestTransform"]; ok {
				t.Fatal("summary used writable contract name")
			}
			catalogCall(t, router, "PATCH", path, map[string]any{"name": "Renamed", "paramOverride": `{"top_p":0.8}`}, 200)
			var actual store.DirectRequestConfig
			if err := db.Get(&actual, db.Rebind(`SELECT request_config FROM upstream_channels WHERE id=?`), id); err != nil {
				t.Fatal(err)
			}
			if len(actual.Headers) != 1 || actual.Headers[0].Value != "synthetic-private-value" {
				t.Fatal("ordinary advanced save discarded imported operations")
			}
			catalogCall(t, router, "PATCH", path, map[string]any{"requestTransformSummary": map[string]any{}}, 400)
			catalogCall(t, router, "PATCH", path, `{"requestTransform":{"body":[{"op":"set","path":"model","value":"other"}]}}`, 400)
			payload, err := backup.BuildPayload(db.DB, "all")
			if err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			tables, err := decodeBackupImportBodyFrom(raw)
			if err != nil {
				t.Fatal(err)
			}
			catalogCall(t, router, "PATCH", path, map[string]any{"requestTransform": map[string]any{}}, 200)
			if _, err := db.Exec(db.Rebind(`DELETE FROM upstream_channels WHERE id=?`), id); err != nil {
				t.Fatal(err)
			}
			if _, err := importBackupTables(db.DB, tables, true); err != nil {
				t.Fatal(err)
			}
			if err := db.Get(&actual, db.Rebind(`SELECT request_config FROM upstream_channels WHERE id=?`), id); err != nil {
				t.Fatal(err)
			}
			if len(actual.Body) != 1 || len(actual.Headers) != 1 || actual.Headers[0].Value != "synthetic-private-value" {
				t.Fatal("portable backup lost request operations")
			}
		})
	}
}
