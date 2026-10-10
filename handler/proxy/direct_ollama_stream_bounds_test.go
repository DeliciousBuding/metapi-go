package proxyhandler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/transform/anthropic/messages"
	"github.com/deliciousbuding/metapi-go/transform/ollama"
)

func TestDirectOllamaStreamBoundOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		options ollama.StreamOptions
		want    streamOutcome
	}{
		{"idle", ollama.StreamOptions{IdleTimeout: 20 * time.Millisecond}, streamEndedIdleTimeout},
		{"line", ollama.StreamOptions{MaxLineBytes: 16}, streamEndedTruncated},
		{"total", ollama.StreamOptions{MaxBytes: 16}, streamEndedTruncated},
	} {
		t.Run(tc.name, func(t *testing.T) {
			closed := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(closed)
				w.Header().Set("Content-Type", "application/x-ndjson")
				_, _ = io.WriteString(w, `{"model":"fixture","done":false,"message":{"content":"first"}}`+"\n")
				w.(http.Flusher).Flush()
				<-r.Context().Done()
			}))
			defer server.Close()
			resp, err := server.Client().Get(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body = ollama.NewNDJSONReader(context.Background(), resp.Body, "fixture", tc.options)
			defer resp.Body.Close()
			resp.Header.Set("Content-Type", "text/event-stream")
			out := httptest.NewRecorder()
			req := makeProxyReq("POST", "/v1/chat/completions", `{"model":"fixture","stream":true}`)
			_, outcome, _ := handleStreamUpstreamForEndpoint(out, req, resp, 0, "/api/chat", "fixture", messages.Options{}, nil, true)
			if outcome != tc.want || !strings.Contains(out.Body.String(), "error") {
				t.Fatalf("native bound outcome=%v want=%v body=%s", outcome, tc.want, out.Body.String())
			}
			select {
			case <-closed:
			case <-time.After(time.Second):
				t.Fatal("bound did not cancel HTTP transport")
			}
		})
	}
}
