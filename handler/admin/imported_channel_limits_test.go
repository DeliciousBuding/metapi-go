package admin

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestImportedChannelRateLimitsAdminAndPortableBackup(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := catalogTestDB(t, dialect, ":memory:")
			router := catalogMux(db)
			limits := map[string]any{"rpm": 3, "tpm": 200, "maxConcurrent": 2, "queueSize": 4, "queueTimeoutMs": 20}
			response := catalogCall(t, router, "POST", catalogPrefix, map[string]any{
				"name": "Limited channel", "provider": "qiniu", "dialect": "generic", "enabled": true,
				"baseUrl": "https://fixture.invalid", "rateLimit": limits,
				"endpointConfig": store.DirectEndpoints{Chat: &store.DirectEndpoint{URL: "https://fixture.invalid/chat", Auth: store.DirectAuthBearer}},
			}, 201)
			id := int64(response["id"].(float64))
			path := fmt.Sprintf("%s/%d", catalogPrefix, id)
			for _, readPath := range []string{path, catalogPrefix} {
				out := catalogRequest(router, "GET", readPath, nil)
				if out.Code != 200 || !strings.Contains(out.Body.String(), `"rateLimit":{"rpm":3`) {
					t.Fatalf("rate policy readback=%d %s", out.Code, out.Body.String())
				}
			}
			catalogCall(t, router, "PATCH", path, map[string]any{"name": "Renamed"}, 200)
			var actual store.DirectRateLimit
			if err := db.Get(&actual, db.Rebind(`SELECT rate_limit FROM upstream_channels WHERE id=?`), id); err != nil || actual.RPM != 3 || actual.MaxConcurrent != 2 {
				t.Fatalf("ordinary save lost limits: %+v %v", actual, err)
			}
			for _, invalid := range []string{`{"rateLimit":{"rpm":-1}}`, `{"rateLimit":null}`, `{"rateLimit":{"unknown":0}}`, `{"rateLimit":{"rpm":1,"rpm":2}}`} {
				catalogCall(t, router, "PATCH", path, invalid, 400)
			}
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
			catalogCall(t, router, "PATCH", path, map[string]any{"rateLimit": map[string]any{}}, 200)
			if err := db.Get(&actual, db.Rebind(`SELECT rate_limit FROM upstream_channels WHERE id=?`), id); err != nil || actual.Enabled() {
				t.Fatalf("empty replacement retained limits: %+v %v", actual, err)
			}
			if _, err := db.Exec(db.Rebind(`DELETE FROM upstream_channels WHERE id=?`), id); err != nil {
				t.Fatal(err)
			}
			if _, err := importBackupTables(db.DB, tables, true); err != nil {
				t.Fatal(err)
			}
			if err := db.Get(&actual, db.Rebind(`SELECT rate_limit FROM upstream_channels WHERE id=?`), id); err != nil || actual.RPM != 3 || actual.TPM != 200 || actual.MaxConcurrent != 2 || actual.QueueSize != 4 || actual.QueueTimeoutMs != 20 {
				t.Fatalf("portable backup lost limits: %+v %v", actual, err)
			}
		})
	}
}
