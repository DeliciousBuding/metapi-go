package proxyhandler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/deliciousbuding/metapi-go/internal/channellimit"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestDirectResponsesWSCancelReleasesCapacity(t *testing.T) {
	sent := make(chan struct{})
	closed := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		if _, _, err := conn.Read(r.Context()); err != nil {
			return
		}
		close(sent)
		_, _, _ = conn.Read(r.Context())
		close(closed)
	}))
	t.Cleanup(upstream.Close)
	db := installDirectMediaRuntime(t, store.DialectSQLite, store.DirectEndpoints{ResponsesWebsocket: &store.DirectEndpoint{URL: "ws" + strings.TrimPrefix(upstream.URL, "http"), Auth: store.DirectAuthBearer}}, store.DirectProtocolResponsesWebsocket, "")
	manager := channellimit.NewManager()
	var id int64
	_ = db.Get(&id, `SELECT id FROM upstream_channels`)
	limits := store.DirectRateLimit{Config: channellimit.Config{MaxConcurrent: 1, RPM: 5}}
	if _, err := db.Exec(`UPDATE upstream_channels SET rate_limit=?`, limits); err != nil {
		t.Fatal(err)
	}
	routing.InvalidateCache()
	cfg := *getUpstreamConfig()
	cfg.ChannelLimits = manager
	cfg.Router.(*routing.TokenRouter).SetChannelLimits(manager)
	logs := make(chan proxy.ProxyLogEntry, 1)
	cfg.LogProxy = func(_ context.Context, e proxy.ProxyLogEntry) error { logs <- e; return nil }
	SetUpstreamConfig(&cfg)
	ctx, conn := dialDirectResponsesGateway(t, "cancel-fixture")
	writeWSMessage(t, ctx, conn, `{"type":"response.create","model":"client-alias","input":[{"role":"user","content":"wait"}]}`)
	select {
	case <-sent:
	case <-ctx.Done():
		t.Fatal("turn never sent")
	}
	conn.CloseNow()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("downstream close did not cancel upstream")
	}
	select {
	case entry := <-logs:
		if entry.Status == "success" {
			t.Fatal("cancelled attempt logged success")
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled attempt not accounted")
	}
	waitDirectChannelIdle(t, manager, id, limits.Config)
}

func TestDirectResponsesWSUsesConfiguredProxy(t *testing.T) {
	var proxies atomic.Int32
	forward := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxies.Add(1)
		if r.URL.Host != "websocket-fixture.invalid" || r.Header.Get("Authorization") != "Bearer fixture-media-key" || r.Header.Get("X-Upstream") != "selected" || r.Header.Get("X-Downstream-Private") != "" {
			t.Errorf("proxy handshake identity changed: %s %v", r.URL, r.Header)
		}
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		if _, _, err := conn.Read(r.Context()); err != nil {
			return
		}
		_ = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"response.completed","response":{"id":"resp-proxy","status":"completed","output":[]}}`))
	}))
	t.Cleanup(forward.Close)
	db := installDirectMediaRuntime(t, store.DialectSQLite, store.DirectEndpoints{ResponsesWebsocket: &store.DirectEndpoint{URL: "ws://websocket-fixture.invalid/socket", Auth: store.DirectAuthBearer}}, store.DirectProtocolResponsesWebsocket, "")
	if _, err := db.Exec(`UPDATE upstream_channels SET channel_proxy=?,custom_header=?`, forward.URL, `[{"header_key":"X-Upstream","header_value":"selected"}]`); err != nil {
		t.Fatal(err)
	}
	routing.InvalidateCache()
	ctx, conn := dialDirectResponsesGateway(t, "proxy-key")
	writeWSMessage(t, ctx, conn, `{"type":"response.create","model":"client-alias","input":[{"role":"user","content":"hello"}]}`)
	if frame := readWSFrame(t, ctx, conn); frame["type"] != "response.completed" || proxies.Load() != 1 {
		t.Fatalf("explicit proxy not used: %+v calls=%d", frame, proxies.Load())
	}
}

func TestCodexRuntimeExpiresSocketAndBoundsMalformedFrames(t *testing.T) {
	for _, malformed := range []bool{false, true} {
		t.Run(fmt.Sprint(malformed), func(t *testing.T) {
			var dials atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				dials.Add(1)
				conn, err := websocket.Accept(w, r, nil)
				if err != nil {
					return
				}
				defer conn.CloseNow()
				for {
					if _, _, err := conn.Read(r.Context()); err != nil {
						return
					}
					payload := `{"type":"response.completed","response":{"id":"resp-ttl","status":"completed","output":[]}}`
					if malformed {
						payload = `{broken`
					}
					if err := conn.Write(r.Context(), websocket.MessageText, []byte(payload)); err != nil {
						return
					}
				}
			}))
			t.Cleanup(upstream.Close)
			key := "runtime-ttl-fixture-" + fmt.Sprint(malformed)
			t.Cleanup(func() { CloseCodexWebsocketSession(key); clearCodexSessionResponseID(key) })
			input := CodexWebsocketSendInput{SessionID: key, RequestURL: upstream.URL, Body: map[string]any{"model": "fixture"}, DisablePreviousRecovery: true}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			_, err := SendCodexWebsocketRequest(ctx, input)
			if malformed {
				if err == nil {
					t.Fatal("malformed socket response succeeded")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			session := getOrCreateCodexWSSocketSession(key)
			session.mu.Lock()
			session.lastUsed = time.Now().Add(-codexSessionTTL() - time.Second)
			session.mu.Unlock()
			result, err := SendCodexWebsocketRequest(ctx, input)
			if err != nil || result.ReusedSession || dials.Load() != 2 {
				t.Fatalf("expired socket reused: %v dials=%d", err, dials.Load())
			}
		})
	}
}

func TestDirectResponsesWSTerminalOutcomeAndUsage(t *testing.T) {
	for _, kind := range []string{"response.incomplete", "response.failed"} {
		t.Run(kind, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := websocket.Accept(w, r, nil)
				if err != nil {
					return
				}
				defer conn.CloseNow()
				if _, _, err := conn.Read(r.Context()); err != nil {
					return
				}
				response := `{"id":"resp-terminal","status":"incomplete","output":[],"incomplete_details":{"reason":"max_output_tokens"},"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`
				if kind == "response.failed" {
					response = `{"id":"resp-terminal","status":"failed","output":[],"error":{"code":"upstream_failed","message":"fixture"},"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`
				}
				_ = conn.Write(r.Context(), websocket.MessageText, []byte(fmt.Sprintf(`{"type":%q,"response":%s}`, kind, response)))
			}))
			t.Cleanup(upstream.Close)
			installDirectMediaRuntime(t, store.DialectSQLite, store.DirectEndpoints{ResponsesWebsocket: &store.DirectEndpoint{URL: "ws" + strings.TrimPrefix(upstream.URL, "http"), Auth: store.DirectAuthBearer}}, store.DirectProtocolResponsesWebsocket, "")
			cfg := *getUpstreamConfig()
			logs := make(chan proxy.ProxyLogEntry, 1)
			cfg.LogProxy = func(_ context.Context, e proxy.ProxyLogEntry) error { logs <- e; return nil }
			SetUpstreamConfig(&cfg)
			requests := make(chan store.GatewayRequest, 4)
			ctx, conn := dialDirectResponsesGatewayObserved(t, "terminal-key", func(_ context.Context, e store.GatewayRequest) error { requests <- e; return nil })
			writeWSMessage(t, ctx, conn, `{"type":"response.create","model":"client-alias","input":[{"role":"user","content":"hello"}]}`)
			frame := readWSFrame(t, ctx, conn)
			if frame["type"] != kind {
				t.Fatalf("native terminal lost: %+v", frame)
			}
			entry := <-logs
			want := "success"
			if kind == "response.failed" {
				want = "failed"
			}
			if entry.Status != want || entry.TotalTokens == nil || *entry.TotalTokens != 3 {
				t.Fatalf("terminal outcome/usage wrong: %+v", entry)
			}
			select {
			case request := <-requests:
				if kind == "response.incomplete" && request.TerminalStatus != "completed" {
					t.Fatalf("native incomplete terminal misclassified: %+v", request)
				}
				if kind == "response.failed" && request.TerminalStatus == "completed" {
					t.Fatalf("failed terminal recorded completed: %+v", request)
				}
			case <-ctx.Done():
				t.Fatal("turn request record missing")
			}
		})
	}
}

func TestDirectResponsesWSRPMRefusalIsNotAnUpstreamAttempt(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		for {
			if _, _, err := conn.Read(r.Context()); err != nil {
				return
			}
			calls.Add(1)
			if err := conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"response.completed","response":{"id":"rpm-fixture","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`)); err != nil {
				return
			}
		}
	}))
	t.Cleanup(upstream.Close)
	db := installDirectMediaRuntime(t, store.DialectSQLite, store.DirectEndpoints{ResponsesWebsocket: &store.DirectEndpoint{URL: "ws" + strings.TrimPrefix(upstream.URL, "http"), Auth: store.DirectAuthBearer}}, store.DirectProtocolResponsesWebsocket, "")
	manager := channellimit.NewManager()
	limit := store.DirectRateLimit{Config: channellimit.Config{RPM: 1, MaxConcurrent: 1}}
	if _, err := db.Exec(`UPDATE upstream_channels SET rate_limit=?`, limit); err != nil {
		t.Fatal(err)
	}
	routing.InvalidateCache()
	cfg := *getUpstreamConfig()
	cfg.ChannelLimits = manager
	cfg.Router.(*routing.TokenRouter).SetChannelLimits(manager)
	var logged atomic.Int32
	cfg.LogProxy = func(context.Context, proxy.ProxyLogEntry) error { logged.Add(1); return nil }
	SetUpstreamConfig(&cfg)
	ctx, conn := dialDirectResponsesGateway(t, "rpm-key")
	writeWSMessage(t, ctx, conn, `{"type":"response.create","model":"client-alias","input":[{"role":"user","content":"first"}]}`)
	if frame := readWSFrame(t, ctx, conn); frame["type"] != "response.completed" {
		t.Fatal(frame)
	}
	writeWSMessage(t, ctx, conn, `{"type":"response.create","model":"client-alias","input":[{"role":"user","content":"second"}]}`)
	frame := readWSFrame(t, ctx, conn)
	if frame["type"] != "error" || frame["status"] != float64(503) || calls.Load() != 1 || logged.Load() != 1 {
		t.Fatalf("local refusal dispatched or billed: %+v calls=%d logs=%d", frame, calls.Load(), logged.Load())
	}
	var failures int
	if err := db.Get(&failures, `SELECT fail_count FROM upstream_grants`); err != nil || failures != 0 {
		t.Fatalf("local refusal started cooldown: %d %v", failures, err)
	}
}
