package proxyhandler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/sqlitetest"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestFirstByteTimeoutSharesRemainingBudgetAcrossChannels(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "json", true: "sse"}[stream], func(t *testing.T) {
			prevCfg, prevRT := config.GetSafe(), config.RuntimeSafe()
			config.Set(&config.Config{ProxyMaxChannelAttempts: 2})
			config.UpdateRuntime(func(r *config.RuntimeSettings) {
				r.ProxyFirstByteTimeoutSec = 1
				r.DisableCrossProtocolFallback = false
			})
			t.Cleanup(func() { config.Set(prevCfg); config.SetRuntime(prevRT) })

			slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				select {
				case <-r.Context().Done():
				case <-time.After(2 * time.Second):
				}
			}))
			t.Cleanup(slow.Close)
			var fastCalls atomic.Int32
			fast := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fastCalls.Add(1)
				// A fresh one-second timer for the second channel would allow
				// this response through after the shared window has expired.
				time.Sleep(700 * time.Millisecond)
				if stream {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, "data: {\"id\":\"chat\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":1,\"total_tokens\":3}}\n\ndata: [DONE]\n\n")
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"chat","object":"chat.completion","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`)
			}))
			t.Cleanup(fast.Close)
			router := &upstreamTestRouter{
				selected: routing.SelectedChannel{Channel: store.RouteChannel{ID: 1, Enabled: true}, Account: store.Account{ID: 1, Status: "active"}, Site: store.Site{ID: 1, URL: slow.URL, Status: "active"}, TokenValue: "first", ActualModel: "gpt-4o"},
				next:     &routing.SelectedChannel{Channel: store.RouteChannel{ID: 2, Enabled: true}, Account: store.Account{ID: 2, Status: "active"}, Site: store.Site{ID: 2, URL: fast.URL, Status: "active"}, TokenValue: "second", ActualModel: "gpt-4o"},
			}
			var logs []proxy.ProxyLogEntry
			var gateway store.GatewayRequest
			SetUpstreamConfig(&UpstreamConfig{Router: router, Executor: proxy.NewRuntimeExecutor(5 * time.Second), LogProxy: func(_ context.Context, entry proxy.ProxyLogEntry) error { logs = append(logs, entry); return nil }})
			t.Cleanup(func() { SetUpstreamConfig(nil) })
			body := `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`
			if stream {
				body = `{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"hi"}]}`
			}
			req := makeProxyReq(http.MethodPost, "/v1/chat/completions", body)
			state, req := newRequestLifecycle(req, "http", func(_ context.Context, entry store.GatewayRequest) error { gateway = entry; return nil })
			rec := httptest.NewRecorder()
			started := time.Now()
			HandleChatCompletions(&requestResponseWriter{ResponseWriter: rec, state: state}, req)
			state.finish(req.Context())
			elapsed := time.Since(started)
			if rec.Code != http.StatusRequestTimeout {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			if elapsed < 900*time.Millisecond || elapsed >= 1500*time.Millisecond {
				t.Fatalf("shared header window elapsed=%v, want about one second", elapsed)
			}
			if fastCalls.Load() != 0 {
				t.Fatalf("expired budget dispatched another full-window attempt: %d", fastCalls.Load())
			}
			if gateway.ReasonCode == nil || *gateway.ReasonCode != "upstream_first_byte_timeout" {
				t.Fatalf("reason=%v, want the first actionable timeout", gateway.ReasonCode)
			}
			if len(logs) != 1 || logs[0].TotalTokens != nil {
				t.Fatalf("timeout usage must stay unknown: %#v", logs)
			}
		})
	}
}

func TestFirstFailureSurvivesLaterExhaustion(t *testing.T) {
	prevCfg, prevRT := config.GetSafe(), config.RuntimeSafe()
	config.Set(&config.Config{ProxyMaxChannelAttempts: 2})
	config.UpdateRuntime(func(r *config.RuntimeSettings) {
		r.ProxyFirstByteTimeoutSec = 1
		r.DisableCrossProtocolFallback = false
	})
	t.Cleanup(func() { config.Set(prevCfg); config.SetRuntime(prevRT) })

	unauthorized := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"invalid token","type":"authentication_error"}}`)
	}))
	t.Cleanup(unauthorized.Close)
	stalled := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	t.Cleanup(stalled.Close)
	router := &upstreamTestRouter{
		selected: routing.SelectedChannel{Channel: store.RouteChannel{ID: 1, Enabled: true}, Account: store.Account{ID: 1, Status: "active"}, Site: store.Site{ID: 1, URL: unauthorized.URL, Status: "active"}, TokenValue: "first", ActualModel: "gpt-4o"},
		next:     &routing.SelectedChannel{Channel: store.RouteChannel{ID: 2, Enabled: true}, Account: store.Account{ID: 2, Status: "active"}, Site: store.Site{ID: 2, URL: stalled.URL, Status: "active"}, TokenValue: "second", ActualModel: "gpt-4o"},
	}
	var gateway store.GatewayRequest
	SetUpstreamConfig(&UpstreamConfig{Router: router, Executor: proxy.NewRuntimeExecutor(5 * time.Second), LogProxy: EnqueueProxyLog})
	t.Cleanup(func() { SetUpstreamConfig(nil) })
	db := sqlitetest.Open(t)
	previousDB := store.GetDB()
	store.OverrideDB(db)
	t.Cleanup(func() { store.OverrideDB(previousDB) })
	req := makeProxyReq(http.MethodPost, "/v1/chat/completions", `{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	state, req := newRequestLifecycle(req, "http", defaultGatewayRequestWriter)
	rec := httptest.NewRecorder()
	HandleChatCompletions(&requestResponseWriter{ResponseWriter: rec, state: state}, req)
	state.finish(req.Context())
	if rec.Code != http.StatusRequestTimeout {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if err := db.Get(&gateway, `SELECT * FROM gateway_requests WHERE id=?`, state.entry.ID); err != nil {
		t.Fatal(err)
	}
	if gateway.TerminalStatus != "upstream_failed" || gateway.ReasonCode == nil || *gateway.ReasonCode != "upstream_auth_rejected" || gateway.AttemptCount != 2 {
		t.Fatalf("first failure was lost: %+v", gateway)
	}
	var attempts int
	if err := db.Get(&attempts, `SELECT COUNT(*) FROM proxy_logs WHERE gateway_request_id=?`, gateway.ID); err != nil || attempts != 2 {
		t.Fatalf("attempt rows=%d error=%v", attempts, err)
	}
}

func TestFastFailureAllowsHealthyFallbackAndPreservesFirstCause(t *testing.T) {
	prevCfg, prevRT := config.GetSafe(), config.RuntimeSafe()
	config.Set(&config.Config{ProxyMaxChannelAttempts: 2})
	config.UpdateRuntime(func(r *config.RuntimeSettings) {
		r.ProxyFirstByteTimeoutSec = 1
		r.DisableCrossProtocolFallback = false
	})
	t.Cleanup(func() { config.Set(prevCfg); config.SetRuntime(prevRT) })

	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"invalid token","type":"authentication_error"}}`)
	}))
	t.Cleanup(slow.Close)
	fast := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"id\":\"chat\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"content\":\"fallback\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":2,\"total_tokens\":6}}\n\n")
		w.(http.Flusher).Flush()
		// The header budget must not turn into an SSE body lifetime limit.
		time.Sleep(1100 * time.Millisecond)
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(fast.Close)
	router := &upstreamTestRouter{
		selected: routing.SelectedChannel{Channel: store.RouteChannel{ID: 1, Enabled: true}, Account: store.Account{ID: 1, Status: "active"}, Site: store.Site{ID: 1, URL: slow.URL, Status: "active"}, TokenValue: "first", ActualModel: "gpt-4o"},
		next:     &routing.SelectedChannel{Channel: store.RouteChannel{ID: 2, Enabled: true}, Account: store.Account{ID: 2, Status: "active"}, Site: store.Site{ID: 2, URL: fast.URL, Status: "active"}, TokenValue: "second", ActualModel: "gpt-4o"},
	}
	var logs []proxy.ProxyLogEntry
	var gateway store.GatewayRequest
	SetUpstreamConfig(&UpstreamConfig{Router: router, Executor: proxy.NewRuntimeExecutor(5 * time.Second), LogProxy: func(_ context.Context, entry proxy.ProxyLogEntry) error { logs = append(logs, entry); return nil }})
	t.Cleanup(func() { SetUpstreamConfig(nil) })
	req := makeProxyReq(http.MethodPost, "/v1/chat/completions", `{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	state, req := newRequestLifecycle(req, "http", func(_ context.Context, entry store.GatewayRequest) error { gateway = entry; return nil })
	rec := httptest.NewRecorder()
	HandleChatCompletions(&requestResponseWriter{ResponseWriter: rec, state: state}, req)
	state.finish(req.Context())
	if rec.Code != http.StatusOK || gateway.TerminalStatus != "completed" || gateway.AttemptCount != 2 || gateway.ReasonCode == nil || *gateway.ReasonCode != "upstream_auth_rejected" {
		t.Fatalf("fallback did not preserve its recovered first cause: status=%d selections=%d request=%+v body=%s logs=%+v", rec.Code, len(router.policies), gateway, rec.Body.String(), logs)
	}
	if len(logs) != 2 || logs[0].TotalTokens != nil || logs[1].TotalTokens == nil || *logs[1].TotalTokens != 6 {
		t.Fatalf("fallback usage mismatch: %#v", logs)
	}
}
