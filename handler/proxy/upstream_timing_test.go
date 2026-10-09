package proxyhandler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestProxyLogTimingIncludesBodyAfterResponseHeaders(t *testing.T) {
	for _, stream := range []bool{false, true} {
		name := "json"
		if stream {
			name = "sse"
		}
		t.Run(name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if stream {
					w.Header().Set("Content-Type", "text/event-stream")
				} else {
					w.Header().Set("Content-Type", "application/json")
				}
				w.WriteHeader(http.StatusOK)
				w.(http.Flusher).Flush()
				if stream {
					_, _ = w.Write([]byte(": heartbeat\n\ndata: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n"))
					w.(http.Flusher).Flush()
				}
				// Model a body arriving after the headers, not a client-side wait.
				select {
				case <-time.After(100 * time.Millisecond):
				case <-r.Context().Done():
					return
				}
				if stream {
					_, _ = w.Write([]byte("data: {\"model\":\"fixture-upstream-model\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"))
				} else {
					_, _ = w.Write([]byte(`{"model":"fixture-upstream-model","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
				}
			}))
			defer upstream.Close()
			var logs []proxy.ProxyLogEntry
			SetUpstreamConfig(&UpstreamConfig{
				Router: &upstreamTestRouter{selected: routing.SelectedChannel{
					Channel:    store.RouteChannel{ID: 42, Enabled: true},
					Account:    store.Account{ID: 7, Status: "active"},
					Site:       store.Site{ID: 3, URL: upstream.URL, Status: "active"},
					TokenValue: "fixture-token", ActualModel: "gpt-5-mini",
				}},
				LogProxy: func(_ context.Context, entry proxy.ProxyLogEntry) error { logs = append(logs, entry); return nil },
			})
			t.Cleanup(func() { SetUpstreamConfig(nil) })
			body := `{"model":"gpt-5-mini","stream":false,"messages":[{"role":"user","content":"hi"}]}`
			if stream {
				body = `{"model":"gpt-5-mini","stream":true,"messages":[{"role":"user","content":"hi"}]}`
			}
			rec := httptest.NewRecorder()
			HandleChatCompletions(rec, makeProxyReq("POST", "/v1/chat/completions", body))
			if rec.Code != http.StatusOK || len(logs) != 1 || logs[0].Status != "success" {
				t.Fatalf("status=%d logs=%+v", rec.Code, logs)
			}
			entry := logs[0]
			if entry.ModelActual == nil || *entry.ModelActual != "gpt-5-mini" || entry.UpstreamReportedModel == nil || *entry.UpstreamReportedModel != "fixture-upstream-model" {
				t.Fatalf("selected and upstream-reported models conflated: %+v", entry)
			}
			if entry.PromptTokens != nil || entry.CompletionTokens != nil || entry.TotalTokens != nil {
				t.Fatal("missing upstream usage was logged as observed zero")
			}
			if entry.FirstByteLatencyMs == nil {
				t.Fatal("response headers were not recorded")
			}
			if entry.LatencyMs-*entry.FirstByteLatencyMs < 80 {
				t.Fatalf("body time missing: first=%d total=%d", *entry.FirstByteLatencyMs, entry.LatencyMs)
			}
			if stream {
				if entry.FirstOutputLatencyMs == nil || *entry.FirstOutputLatencyMs-*entry.FirstByteLatencyMs < 80 || *entry.FirstOutputLatencyMs > entry.LatencyMs {
					t.Fatalf("generated output timing is not independent: %+v", entry)
				}
			} else if entry.FirstOutputLatencyMs != nil {
				t.Fatal("buffered JSON invented a first output measurement")
			}
		})
	}
}
