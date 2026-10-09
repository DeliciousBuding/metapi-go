package admin

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	backupsvc "github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

func TestDownstreamAccessPolicyAdminAndBackupRoundtrip(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			var db *store.DB
			var router chi.Router
			if dialect == store.DialectPostgres {
				db, router = setupDownstreamKeysPostgresTest(t)
			} else {
				db, router = setupDownstreamKeysTest(t)
			}
			policy := map[string]any{"allowedUpstreamChannelIds": []int{}, "modelIds": []string{"native-model"}, "modelMappings": []any{map[string]any{"from": "alias", "to": "native-model"}}, "quota": map[string]any{"requests": 0, "period": map[string]any{"type": "calendar_duration", "calendarDuration": map[string]any{"unit": "day"}}, "timezone": "Asia/Hong_Kong"}}
			out := doPostJSON(t, router, "/api/downstream-keys", map[string]any{"name": "native policy", "key": "sk-native-policy", "supportedModels": []string{"*"}, "accessPolicy": policy})
			if out.Code != http.StatusOK {
				t.Fatalf("create: %d %s", out.Code, out.Body.String())
			}
			var id int64
			var persisted string
			if err := db.QueryRowx(`SELECT id,access_policy FROM downstream_api_keys WHERE key='sk-native-policy'`).Scan(&id, &persisted); err != nil {
				t.Fatal(err)
			}
			parsed, err := store.ParseDownstreamAccessPolicy(persisted)
			if err != nil || parsed.AllowedUpstreamChannelIDs == nil || len(*parsed.AllowedUpstreamChannelIDs) != 0 {
				t.Fatal("strict empty channel list lost in persistence")
			}
			out = doPutJSON(t, router, "/api/downstream-keys/"+strconv.FormatInt(id, 10), map[string]any{"name": "renamed"})
			if out.Code != 200 {
				t.Fatalf("rename: %s", out.Body.String())
			}
			var after string
			_ = db.Get(&after, db.Rebind(`SELECT access_policy FROM downstream_api_keys WHERE id=?`), id)
			if after != persisted {
				t.Fatal("partial update dropped access policy")
			}
			if _, err := db.Exec(`INSERT INTO downstream_quota_usage(key_id,event_key,occurred_at,total_tokens,cost) VALUES (?,'fixture',123,7,0.2)`, id); err != nil {
				t.Fatal(err)
			}
			payload, err := backupsvc.BuildPayload(db.DB, "all")
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
			// Replay the real restore implementation against a clean destination.
			if _, err := db.Exec(`DELETE FROM downstream_api_keys WHERE id=?`, id); err != nil {
				t.Fatal(err)
			}
			if _, err := importBackupTables(db.DB, tables, true); err != nil {
				t.Fatal(err)
			}
			_ = db.Get(&after, db.Rebind(`SELECT access_policy FROM downstream_api_keys WHERE id=?`), id)
			if after != persisted {
				t.Fatal("restore changed access policy")
			}
			var count int
			_ = db.Get(&count, `SELECT COUNT(*) FROM downstream_quota_usage`)
			if count != 1 {
				t.Fatal("restore lost quota usage")
			}
			preview, err := previewBackupImportTables(db.DB, tables)
			if err != nil {
				t.Fatal(err)
			}
			if preview["downstream_quota_usage"].Duplicates != 1 {
				t.Fatal("repeat preview did not detect persisted quota event")
			}
			out = doPutJSON(t, router, "/api/downstream-keys/"+strconv.FormatInt(id, 10), map[string]any{"accessPolicy": map[string]any{"quota": map[string]any{"requests": 5, "period": map[string]any{"type": "invalid"}}}})
			if out.Code != 400 {
				t.Fatal("invalid quota accepted")
			}
			out = doPutJSON(t, router, "/api/downstream-keys/"+strconv.FormatInt(id, 10), map[string]any{"accessPolicy": nil})
			if out.Code != 200 {
				t.Fatalf("clear: %s", out.Body.String())
			}
			var nullPolicy *string
			if err := db.QueryRowx(`SELECT access_policy FROM downstream_api_keys WHERE id=?`, id).Scan(&nullPolicy); err != nil || nullPolicy != nil {
				t.Fatal("explicit null did not clear policy")
			}
		})
	}
}
