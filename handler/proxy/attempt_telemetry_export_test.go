package proxyhandler

import (
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/store"
)

func RunTelemetryFixture(t *testing.T, dialect string, endpoints store.DirectEndpoints, ep *store.DirectEndpoint, bit int, path, body string, frames []string) (*store.DB, *httptest.ResponseRecorder) {
	t.Helper()
	var chunks []string
	for _, f := range frames {
		chunks = append(chunks, "data: "+f+"\n\n")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		defer gz.Close()
		for i, frame := range chunks {
			if endpoints.Responses != nil && i > 0 && i <= 2 {
				time.Sleep(140 * time.Millisecond)
			}
			_, _ = io.WriteString(gz, frame)
			_ = gz.Flush()
			w.(http.Flusher).Flush()
		}
	}))
	t.Cleanup(srv.Close)
	ep.URL = srv.URL
	db := installDirectMediaRuntime(t, dialect, endpoints, bit, "")
	cfg := *getUpstreamConfig()
	cfg.LogProxy = func(ctx context.Context, e proxy.ProxyLogEntry) error { return InsertProxyLog(ctx, db, e) }
	SetUpstreamConfig(&cfg)
	handler := requestLifecycleMiddleware(directMediaRouter(), func(ctx context.Context, e store.GatewayRequest) error { return store.InsertGatewayRequest(ctx, db, e) })
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, makeProxyReq(http.MethodPost, path, body))
	return db, rec
}
