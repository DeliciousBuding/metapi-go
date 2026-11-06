package admin

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestImportedStreamPolicyManagementAndPortableBackup(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := catalogTestDB(t, dialect, ":memory:")
			router := catalogMux(db)
			response := catalogCall(t, router, "POST", catalogPrefix, map[string]any{
				"name": "Streaming channel", "provider": "qiniu", "dialect": "generic", "enabled": true,
				"baseUrl": "https://fixture.invalid", "streamPolicy": "require",
				"endpointConfig": store.DirectEndpoints{Chat: &store.DirectEndpoint{URL: "https://fixture.invalid/chat", Auth: store.DirectAuthBearer}},
			}, 201)
			id := int64(response["id"].(float64))
			path := fmt.Sprintf("%s/%d", catalogPrefix, id)
			for _, method := range []string{"GET", "PATCH"} {
				if method == "PATCH" {
					catalogCall(t, router, method, path, map[string]any{"name": "Renamed", "paramOverride": `{"temperature":0.2}`}, 200)
				}
				out := catalogRequest(router, "GET", path, nil)
				if out.Code != 200 || !strings.Contains(out.Body.String(), `"streamPolicy":"require"`) {
					t.Fatalf("ordinary save lost policy: %s", out.Body.String())
				}
			}
			catalogCall(t, router, "PATCH", path, map[string]any{"streamPolicy": "future"}, 400)
			catalogCall(t, router, "PATCH", path, map[string]any{"streamPolicy": "forbid"}, 200)
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
			if _, err := db.Exec(db.Rebind(`DELETE FROM upstream_channels WHERE id=?`), id); err != nil {
				t.Fatal(err)
			}
			if _, err := importBackupTables(db.DB, tables, true); err != nil {
				t.Fatal(err)
			}
			var policy string
			if err := db.Get(&policy, db.Rebind(`SELECT stream_policy FROM upstream_channels WHERE id=?`), id); err != nil || policy != "forbid" {
				t.Fatalf("portable policy=%q err=%v", policy, err)
			}
		})
	}
}
