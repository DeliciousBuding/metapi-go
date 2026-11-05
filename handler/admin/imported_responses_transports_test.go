package admin

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestImportedResponsesTransportsAdminAndPortableBackup(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := catalogTestDB(t, dialect, ":memory:")
			router := catalogMux(db)
			endpoints := store.DirectEndpoints{
				ResponsesCompact:   &store.DirectEndpoint{URL: "https://fixture.invalid/compact", Auth: store.DirectAuthBearer},
				ResponsesWebsocket: &store.DirectEndpoint{URL: "wss://fixture.invalid/socket", Auth: store.DirectAuthBearer},
			}
			result := catalogCall(t, router, "POST", catalogPrefix, map[string]any{"name": "Responses transports", "provider": "qiniu", "dialect": "generic", "enabled": true, "baseUrl": "https://fixture.invalid", "endpointConfig": endpoints}, 201)
			id := int64(result["id"].(float64))
			path := fmt.Sprintf("%s/%d", catalogPrefix, id)
			catalogCall(t, router, "PATCH", path, map[string]any{"name": "Renamed"}, 200)
			out := catalogRequest(router, "GET", path, nil)
			if out.Code != 200 || !strings.Contains(out.Body.String(), `"responsesCompact"`) || !strings.Contains(out.Body.String(), `"responsesWebsocket"`) {
				t.Fatalf("transport readback lost: %d %s", out.Code, out.Body.String())
			}
			catalogCall(t, router, "PATCH", path, map[string]any{"endpointConfig": map[string]any{"responsesWebsocket": map[string]any{"url": "wss://169.254.169.254/latest", "auth": "bearer"}}}, 400)
			payload, err := backup.BuildPayload(db.DB, "all")
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(payload)
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
			var loaded store.DirectEndpoints
			if err := db.Get(&loaded, db.Rebind(`SELECT endpoint_config FROM upstream_channels WHERE id=?`), id); err != nil {
				t.Fatal(err)
			}
			if loaded.ProtocolMask() != store.DirectProtocolResponsesCompact|store.DirectProtocolResponsesWebsocket || *loaded.ResponsesCompact != *endpoints.ResponsesCompact || *loaded.ResponsesWebsocket != *endpoints.ResponsesWebsocket {
				t.Fatal("portable restore changed explicit transport permissions")
			}
		})
	}
}
