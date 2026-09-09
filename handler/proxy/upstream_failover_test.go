package proxyhandler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestResolveUpstreamCandidatePaths(t *testing.T) {
	for _, tc := range []struct {
		name, path string
		disable    bool
		pref       proxy.SiteProtocolPreference
		want       []string
	}{
		{"chat native", "/v1/chat/completions", false, proxy.SiteProtocolPreference{}, []string{"/v1/chat/completions"}},
		{"Messages bridge", "/v1/messages", false, proxy.SiteProtocolPreference{}, []string{"/v1/messages", "/v1/chat/completions"}},
		{"Messages disabled", "/v1/messages", true, proxy.SiteProtocolPreference{}, []string{"/v1/messages"}},
		{"Messages native before preference", "/v1/messages", false, proxy.SiteProtocolPreference{PreferResponses: true}, []string{"/v1/messages", "/v1/chat/completions"}},
		{"unimplemented preference", "/v1/chat/completions", false, proxy.SiteProtocolPreference{PreferResponses: true}, []string{"/v1/chat/completions"}},
		{"responses native", "/v1/responses", false, proxy.SiteProtocolPreference{}, []string{"/v1/responses"}},
		{"non chat", "/v1/embeddings", false, proxy.SiteProtocolPreference{}, []string{"/v1/embeddings"}},
		{"count tokens", "/v1/messages/count_tokens", false, proxy.SiteProtocolPreference{}, []string{"/v1/messages/count_tokens"}},
		{"responses-shaped alias", "/v1/chat/completions", false, proxy.SiteProtocolPreference{ResponsesOnly: true, PreferStream: true}, []string{"/v1/responses"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveUpstreamCandidatePaths(tc.path, tc.disable, tc.pref); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("paths=%v want %v", got, tc.want)
			}
		})
	}
}

func TestDispatchDoesNotSendChatBodyToUnimplementedProtocol(t *testing.T) {
	var (
		mu    sync.Mutex
		paths []string
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/chat/completions" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"please use /v1/messages"}}`))
			return
		}
		if r.URL.Path == "/v1/messages" {
			_, _ = w.Write([]byte(`{"id":"msg_ok","content":[{"type":"text","text":"ok"}]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"nope"}`))
	}))
	t.Cleanup(upstream.Close)

	prevRt := config.RuntimeSafe()
	config.UpdateRuntime(func(r *config.RuntimeSettings) {
		r.DisableCrossProtocolFallback = false
		r.ProxyFirstByteTimeoutSec = 0
	})
	t.Cleanup(func() { config.SetRuntime(prevRt) })

	router := &upstreamTestRouter{selected: routing.SelectedChannel{
		Channel:     store.RouteChannel{ID: 42, Enabled: true},
		Account:     store.Account{ID: 7, Status: "active"},
		Site:        store.Site{ID: 3, URL: upstream.URL, Status: "active"},
		TokenValue:  "upstream-token",
		ActualModel: "claude-3",
	}}
	SetUpstreamConfig(&UpstreamConfig{Router: router})
	t.Cleanup(func() { SetUpstreamConfig(nil) })

	req := makeProxyReq("POST", "/v1/chat/completions", `{"model":"claude-3","messages":[{"role":"user","content":"hi"}]}`)
	rec := httptest.NewRecorder()
	HandleChatCompletions(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want original 400; body=%s paths=%v", rec.Code, rec.Body.String(), paths)
	}
	mu.Lock()
	gotPaths := append([]string(nil), paths...)
	mu.Unlock()
	if len(gotPaths) != 1 || gotPaths[0] != "/v1/chat/completions" {
		t.Fatalf("paths = %v, want only native Chat", gotPaths)
	}
	if len(router.failures) != 1 {
		t.Fatalf("failures = %#v, want the actual native failure", router.failures)
	}
	if len(router.successes) != 0 {
		t.Fatalf("unimplemented fallback reported success: %#v", router.successes)
	}
}

func TestDispatchDisableCrossProtocolFallbackStopsAfterPrimary(t *testing.T) {
	var (
		mu    sync.Mutex
		paths []string
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"please use /v1/chat/completions"}}`))
	}))
	t.Cleanup(upstream.Close)

	prevRt := config.RuntimeSafe()
	config.UpdateRuntime(func(r *config.RuntimeSettings) {
		r.DisableCrossProtocolFallback = true
	})
	t.Cleanup(func() { config.SetRuntime(prevRt) })

	router := &upstreamTestRouter{selected: routing.SelectedChannel{
		Channel:     store.RouteChannel{ID: 42, Enabled: true},
		Account:     store.Account{ID: 7, Status: "active"},
		Site:        store.Site{ID: 3, URL: upstream.URL, Status: "active"},
		TokenValue:  "upstream-token",
		ActualModel: "claude-3",
	}}
	SetUpstreamConfig(&UpstreamConfig{Router: router})
	t.Cleanup(func() { SetUpstreamConfig(nil) })

	req := makeProxyReq("POST", "/v1/messages", `{"model":"claude-3","max_tokens":64,"messages":[{"role":"user","content":"hi"}]}`)
	rec := httptest.NewRecorder()
	HandleClaudeMessages(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	mu.Lock()
	gotPaths := append([]string(nil), paths...)
	mu.Unlock()
	if len(gotPaths) != 1 || gotPaths[0] != "/v1/messages" {
		t.Fatalf("paths = %v, want only primary Messages path", gotPaths)
	}
}

func TestDispatchChatTimeoutDoesNotReplayOtherProtocols(t *testing.T) {
	var (
		mu    sync.Mutex
		paths []string
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		if r.URL.Path == "/v1/chat/completions" {
			// Exceed 1s first-byte timeout.
			time.Sleep(1200 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"late"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl_fast","choices":[{"message":{"content":"ok"}}]}`))
	}))
	t.Cleanup(upstream.Close)

	prevRt := config.RuntimeSafe()
	config.UpdateRuntime(func(r *config.RuntimeSettings) {
		r.DisableCrossProtocolFallback = false
		// PROXY_FIRST_BYTE_TIMEOUT_SEC is seconds; convert to ms internally.
		r.ProxyFirstByteTimeoutSec = 1
	})
	t.Cleanup(func() { config.SetRuntime(prevRt) })

	router := &upstreamTestRouter{selected: routing.SelectedChannel{
		Channel:     store.RouteChannel{ID: 42, Enabled: true},
		Account:     store.Account{ID: 7, Status: "active"},
		Site:        store.Site{ID: 3, URL: upstream.URL, Status: "active"},
		TokenValue:  "upstream-token",
		ActualModel: "gpt-4o",
	}}
	SetUpstreamConfig(&UpstreamConfig{
		Router:   router,
		Executor: proxy.NewRuntimeExecutor(5 * time.Second),
	})
	t.Cleanup(func() { SetUpstreamConfig(nil) })

	req := makeProxyReq("POST", "/v1/chat/completions", `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`)
	rec := httptest.NewRecorder()
	HandleChatCompletions(rec, req)

	if rec.Code != http.StatusRequestTimeout {
		t.Fatalf("status = %d, want native timeout; body=%s", rec.Code, rec.Body.String())
	}
	mu.Lock()
	gotPaths := append([]string(nil), paths...)
	mu.Unlock()
	if len(gotPaths) != 1 || gotPaths[0] != "/v1/chat/completions" {
		t.Fatalf("paths = %v, want only native Chat timeout", gotPaths)
	}
	if len(router.failures) != 1 || len(router.successes) != 0 {
		t.Fatalf("failures = %#v, successes = %#v, want one native timeout", router.failures, router.successes)
	}
	if body := rec.Body.String(); !strings.Contains(body, "first-byte timeout") {
		t.Fatalf("body = %q, want the real timeout", body)
	}
}

func TestDispatchResponsesOnlySiteRoutesToResponsesAndForcesStream(t *testing.T) {
	var (
		mu       sync.Mutex
		paths    []string
		streamed bool
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if r.URL.Path != "/v1/responses" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not responses"}`))
			return
		}
		if v, ok := body["stream"].(bool); ok && v {
			streamed = true
		}
		// SSE-ish response for forced stream path.
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"resp_ok\",\"type\":\"response.completed\"}\n\n"))
	}))
	t.Cleanup(upstream.Close)

	customHeaders := `{"x-metapi-responses-only":"true"}`
	router := &upstreamTestRouter{selected: routing.SelectedChannel{
		Channel:     store.RouteChannel{ID: 42, Enabled: true},
		Account:     store.Account{ID: 7, Status: "active"},
		Site:        store.Site{ID: 3, URL: upstream.URL, Status: "active", Platform: "openai", CustomHeaders: &customHeaders},
		TokenValue:  "upstream-token",
		ActualModel: "gpt-4o",
	}}
	SetUpstreamConfig(&UpstreamConfig{Router: router})
	t.Cleanup(func() { SetUpstreamConfig(nil) })

	// Chat path + responses-shaped body (input, no messages) should rewrite to /v1/responses.
	req := makeProxyReq("POST", "/v1/chat/completions", `{"model":"gpt-4o","input":"hello"}`)
	rec := httptest.NewRecorder()
	HandleChatCompletions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s paths=%v", rec.Code, rec.Body.String(), paths)
	}
	mu.Lock()
	gotPaths := append([]string(nil), paths...)
	mu.Unlock()
	if len(gotPaths) != 1 || gotPaths[0] != "/v1/responses" {
		t.Fatalf("paths = %v, want only /v1/responses", gotPaths)
	}
	if !streamed {
		t.Fatal("expected stream=true forced for responses-only site")
	}
}

func TestDispatchResponsesOnlySiteRejectsChatShapedBody(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("upstream should not be called; got %s", r.URL.Path)
	}))
	t.Cleanup(upstream.Close)

	customHeaders := `{"x-metapi-responses-only":"1"}`
	router := &upstreamTestRouter{selected: routing.SelectedChannel{
		Channel:     store.RouteChannel{ID: 42, Enabled: true},
		Account:     store.Account{ID: 7, Status: "active"},
		Site:        store.Site{ID: 3, URL: upstream.URL, Status: "active", Platform: "openai", CustomHeaders: &customHeaders},
		TokenValue:  "upstream-token",
		ActualModel: "gpt-4o",
	}}
	SetUpstreamConfig(&UpstreamConfig{Router: router})
	t.Cleanup(func() { SetUpstreamConfig(nil) })

	req := makeProxyReq("POST", "/v1/chat/completions", `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`)
	rec := httptest.NewRecorder()
	HandleChatCompletions(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, "responses-only") {
		t.Fatalf("body = %q, want clear responses-only error", body)
	}
}
