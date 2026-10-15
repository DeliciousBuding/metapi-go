package proxyhandler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/deliciousbuding/metapi-go/internal/pgtest"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestProxyLogDownstreamPath(t *testing.T) {
	for _, tc := range []struct{ path, want string }{
		{"/v1/chat/completions?debug=private", "/v1/chat/completions"},
		{"/responses", "/v1/responses"},
		{"/v1/responses/compact", "/v1/responses/compact"},
		{"/v1/messages/count_tokens", "/v1/messages/count_tokens"},
		{"/v1beta/models/private-model:generateContent", "/v1beta/models/{model}:generateContent"},
		{"/v1beta/models/private-model:streamGenerateContent?alt=sse", "/v1beta/models/{model}:streamGenerateContent"},
		{"/v1internal::generateContent", "/v1internal::generateContent"},
		{"/v1/embeddings", "/v1/embeddings"},
		{"/v1/images/generations", "/v1/images/generations"},
		{"/v1/videos/private-task/content", ""},
		{"/v1/files/private-file", ""},
		{"/unknown", ""},
		{"", ""},
	} {
		t.Run(tc.path, func(t *testing.T) {
			got := proxyLogDownstreamPath(&Ctx{DownstreamPath: tc.path})
			if tc.want == "" {
				if got != nil {
					t.Fatalf("unknown/resource path recorded: %q", *got)
				}
			} else if got == nil || *got != tc.want {
				t.Fatalf("got %v, want %s", got, tc.want)
			}
		})
	}
	if proxyLogDownstreamPath(nil) != nil {
		t.Fatal("missing request context must stay unknown")
	}
}

func TestDownstreamPathPersistence(t *testing.T) {
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
			if err := store.AutoMigrate(db); err != nil {
				t.Fatal(err)
			}
			path := "/v1/messages"
			entries := []proxy.ProxyLogEntry{
				{DownstreamPath: &path, ModelRequested: "fixture-ingress", Status: "success", HTTPStatus: 200},
				{ModelRequested: "fixture-ingress", Status: "failed", HTTPStatus: 502},
			}
			if err := InsertProxyLog(context.Background(), db, entries[0]); err != nil {
				t.Fatal(err)
			}
			if err := batchInsertProxyLogs(context.Background(), db, entries); err != nil {
				t.Fatal(err)
			}
			var rows []store.ProxyLog
			if err := db.Select(&rows, "SELECT * FROM proxy_logs ORDER BY id"); err != nil {
				t.Fatal(err)
			}
			if len(rows) != 3 || rows[0].DownstreamPath == nil || *rows[0].DownstreamPath != path || rows[1].DownstreamPath == nil || *rows[1].DownstreamPath != path || rows[2].DownstreamPath != nil {
				t.Fatalf("single/batch ingress metadata changed: %+v", rows)
			}
		})
	}
}

func TestDownstreamPathFailureHTTP(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"fixture rate limit"}}`))
	}))
	defer upstream.Close()
	db := installAxonHubEndpointFixture(t, "openai", upstream.URL, nil)
	getUpstreamConfig().LogProxy = func(ctx context.Context, entry proxy.ProxyLogEntry) error {
		return InsertProxyLog(ctx, db, entry)
	}
	out := httptest.NewRecorder()
	HandleChatCompletions(out, makeProxyReq(http.MethodPost, "/v1/chat/completions", `{"model":"client-alias","messages":[{"role":"user","content":"hello"}]}`))
	var row store.ProxyLog
	if err := db.Get(&row, "SELECT * FROM proxy_logs ORDER BY id DESC LIMIT 1"); err != nil {
		t.Fatal(err)
	}
	if out.Code == http.StatusOK || row.Status == nil || *row.Status != "failed" || row.DownstreamPath == nil || *row.DownstreamPath != "/v1/chat/completions" {
		t.Fatalf("failed dispatch lost ingress: status=%d log=%+v", out.Code, row)
	}
}
