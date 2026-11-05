package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/internal/pgtest"
	"github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

func TestImportedConditionManagementPreservesPredicates(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			dsn := ":memory:"
			if dialect == store.DialectPostgres {
				dsn = os.Getenv("PG_TEST_DSN")
				if dsn == "" {
					t.Skip("PG_TEST_DSN not set")
				}
			}
			db, err := store.Open(dialect, dsn, false)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if dialect == store.DialectPostgres {
				pgtest.Reset(t, db.DB)
			}
			if err = store.AutoMigrate(db); err != nil {
				t.Fatal(err)
			}
			raw := []byte(`{"version":"1.4","channels":[{"id":1,"name":"conditional","type":"openai","base_url":"https://upstream.invalid","credentials":{"apiKey":"synthetic-key"},"supported_models":["model"],"endpoints":[{"api_format":"openai/chat_completions"}]}],"models":[{"id":1,"model_id":"public","settings":{"associations":[{"type":"channel_model","priority":3,"channelModel":{"channelId":1,"modelId":"model"},"when":{"enabled":true,"condition":{"conditions":[{"type":"condition","field":"request_header.X-Route","operator":"eq","value":"private-routing-value"}]}}}]}}]}`)
			if _, err = backup.ImportAxonHubV14(db, raw, "conditions-management", false); err != nil {
				t.Fatal(err)
			}
			var id int64
			if err = db.Get(&id, `SELECT id FROM upstream_group_items`); err != nil {
				t.Fatal(err)
			}
			var before string
			if err = db.Get(&before, `SELECT association_conditions FROM upstream_group_items`); err != nil {
				t.Fatal(err)
			}
			mux := chi.NewRouter()
			RegisterImportedUpstreamRoutes(mux, db.DB)
			request := func(method, path, body string) *httptest.ResponseRecorder {
				r := httptest.NewRecorder()
				mux.ServeHTTP(r, httptest.NewRequest(method, path, strings.NewReader(body)))
				return r
			}
			out := request(http.MethodGet, "/api/imported-upstreams", "")
			if out.Code != 200 || strings.Contains(out.Body.String(), "private-routing-value") || strings.Contains(out.Body.String(), "request_header.X-Route") {
				t.Fatalf("condition payload escaped inventory: %d %s", out.Code, out.Body.String())
			}
			var inventory struct {
				Members []struct {
					Count int `json:"associationRuleCount"`
				}
			}
			if json.Unmarshal(out.Body.Bytes(), &inventory) != nil || len(inventory.Members) != 1 || inventory.Members[0].Count != 1 {
				t.Fatal("missing condition summary")
			}
			path := fmt.Sprintf("/api/imported-upstreams/members/%d", id)
			if out = request(http.MethodPatch, path, `{"priority":1}`); out.Code != 400 {
				t.Fatal("static priority edited conditional source precedence")
			}
			if out = request(http.MethodPatch, path, `{"priority":3,"weight":2,"protocolOrder":[]}`); out.Code != 200 {
				t.Fatalf("ordinary member edit failed: %s", out.Body.String())
			}
			var after string
			if err = db.Get(&after, `SELECT association_conditions FROM upstream_group_items`); err != nil || after != before {
				t.Fatal("member edit erased association conditions")
			}
			payload, err := backup.BuildPayload(db.DB, "all")
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			importBody, err := decodeBackupImportBodyFrom(encoded)
			if err != nil {
				t.Fatal(err)
			}
			for _, table := range store.ClearTableNames() {
				if _, err := db.Exec(fmt.Sprintf(`DELETE FROM %q`, table)); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := importBackupTables(db.DB, importBody, false); err != nil {
				t.Fatal(err)
			}
			if err = db.Get(&after, `SELECT association_conditions FROM upstream_group_items`); err != nil || after != before {
				t.Fatal("portable backup roundtrip lost association conditions")
			}
		})
	}
}
