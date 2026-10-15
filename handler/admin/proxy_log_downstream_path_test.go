package admin

import (
	"context"
	"encoding/json"
	"testing"

	proxyhandler "github.com/deliciousbuding/metapi-go/handler/proxy"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

func TestProxyLogDownstreamPathProjection(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			var db *store.DB
			var mux chi.Router
			if dialect == store.DialectPostgres {
				db, mux = setupStatsPostgresTest(t)
			} else {
				db, mux = setupStatsSQLiteTest(t)
			}
			path := "/v1/messages/count_tokens"
			for _, observed := range []*string{nil, &path} {
				if err := proxyhandler.InsertProxyLog(context.Background(), db, proxy.ProxyLogEntry{ModelRequested: "fixture-ingress", DownstreamPath: observed, Status: "success", HTTPStatus: 200}); err != nil {
					t.Fatal(err)
				}
				var id int64
				if err := db.Get(&id, "SELECT id FROM proxy_logs ORDER BY id DESC LIMIT 1"); err != nil {
					t.Fatal(err)
				}
				for _, url := range []string{"/api/stats/proxy-logs/" + itoa(id), "/api/stats/proxy-logs?view=query"} {
					out := doGet(t, mux, url)
					var body map[string]any
					if out.Code != 200 || json.Unmarshal(out.Body.Bytes(), &body) != nil {
						t.Fatalf("projection failed: %d %s", out.Code, out.Body.String())
					}
					if items, ok := body["items"].([]any); ok {
						for _, item := range items {
							row := item.(map[string]any)
							if coerceInt64(row["id"]) == id {
								body = row
								break
							}
						}
					}
					got, present := body["downstreamPath"]
					if !present || observed == nil && got != nil || observed != nil && got != *observed {
						t.Fatalf("%s lost nullable camelCase ingress: %+v", url, body)
					}
				}
			}
		})
	}
}
