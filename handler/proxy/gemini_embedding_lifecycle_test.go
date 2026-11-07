package proxyhandler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestGeminiEmbeddingIngressHasCanonicalRequestAndAttempt(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, prefix := range []string{"/v1beta/models/", "/gemini/v1alpha/models/"} {
			for _, action := range []string{"embedContent", "batchEmbedContents"} {
				t.Run(dialect+prefix+action, func(t *testing.T) {
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Content-Type", "application/json")
						_, _ = w.Write([]byte(`{"embedding":{"values":[0.1]},"embeddings":[{"values":[0.1]}],"usage":{"total_tokens":9}}`))
					}))
					defer upstream.Close()
					installDirectMediaRuntime(t, dialect, store.DirectEndpoints{GeminiEmbeddings: &store.DirectEndpoint{URL: upstream.URL + "/embed", Auth: store.DirectAuthBearer}}, store.DirectProtocolGeminiEmbeddings, "")
					var records []store.GatewayRequest
					var logs []proxy.ProxyLogEntry
					getUpstreamConfig().LogProxy = func(_ context.Context, entry proxy.ProxyLogEntry) error { logs = append(logs, entry); return nil }
					wrapped := requestLifecycleMiddleware(directMediaRouter(), func(_ context.Context, entry store.GatewayRequest) error {
						records = append(records, entry)
						return nil
					})
					body := `{"content":{"parts":[{"text":"fixture"}]}}`
					if action == "batchEmbedContents" {
						body = `{"requests":[{"content":{"parts":[{"text":"fixture"}]}}]}`
					}
					request := makeProxyReq("POST", prefix+"client-alias:"+action, body)
					request.Header.Set("X-Request-Id", "embedding-ingress-fixture")
					response := httptest.NewRecorder()
					wrapped.ServeHTTP(response, request)
					canonical := "/v1beta/models/{model}:" + action
					if response.Code != 200 || len(records) != 1 || len(logs) != 1 {
						t.Fatalf("embedding observation absent: %d records=%d attempts=%d", response.Code, len(records), len(logs))
					}
					if records[0].DownstreamPath == nil || *records[0].DownstreamPath != canonical || records[0].AttemptCount != 1 || records[0].TerminalStatus != "completed" || records[0].ExternalRequestID == nil || *records[0].ExternalRequestID != "embedding-ingress-fixture" || logs[0].GatewayRequestID != records[0].ID || logs[0].DownstreamPath == nil || *logs[0].DownstreamPath != canonical {
						t.Fatalf("embedding identity/path lost: %+v %+v", records, logs)
					}
				})
			}
		}
	}
}

func TestGeminiEmbeddingCanonicalPathDoesNotExposeModelOrObserveUnknownRoutes(t *testing.T) {
	for _, path := range []string{"/v1beta/models/private-model:embedContent?key=fixture-private", "/gemini/v1/models/private-model:batchEmbedContents"} {
		canonical := canonicalRequestPath(path)
		if canonical == nil || strings.Contains(*canonical, "private") || strings.Contains(*canonical, "key=") {
			t.Fatalf("embedding path not canonicalized: %v", canonical)
		}
	}
	for _, path := range []string{"/api/models/private-model:embedContent", "/v1beta/models/private-model:unknown", "/gemini/v1/models/private-model:unknown"} {
		if canonicalRequestPath(path) != nil {
			t.Fatalf("unknown surface entered proxy observations: %s", path)
		}
	}
}
