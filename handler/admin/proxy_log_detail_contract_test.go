package admin

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

func TestProxyLogDetailBooleanAndDownstreamIdentity(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			var db *store.DB
			var mux chi.Router
			if dialect == store.DialectPostgres {
				db, mux = setupStatsPostgresTest(t)
			} else {
				db, mux = setupStatsSQLiteTest(t)
			}
			keyID, err := execInsertID(db.DB, `INSERT INTO downstream_api_keys (name,key,enabled) VALUES ('fixture-key-name','fixture-secret-do-not-return',?)`, true)
			if err != nil {
				t.Fatal(err)
			}
			for _, stream := range []bool{false, true} {
				logID, err := execInsertID(db.DB, `INSERT INTO proxy_logs (is_stream,downstream_api_key_id,status,model_requested) VALUES (?,?,'success','gpt-6')`, stream, keyID)
				if err != nil {
					t.Fatal(err)
				}
				out := doGet(t, mux, "/api/stats/proxy-logs/"+itoa(logID))
				var body map[string]any
				if out.Code != 200 || json.Unmarshal(out.Body.Bytes(), &body) != nil {
					t.Fatalf("detail failed: %s", out.Body.String())
				}
				if got, ok := body["isStream"].(bool); !ok || got != stream {
					t.Fatalf("isStream must be a boolean %v, got %#v", stream, body["isStream"])
				}
				if body["downstreamKeyName"] != "fixture-key-name" || body["downstreamKeyId"] != float64(keyID) || strings.Contains(out.Body.String(), "fixture-secret-do-not-return") {
					t.Fatalf("incorrect/unsafe key identity: %s", out.Body.String())
				}
			}
		})
	}
}
