package proxyhandler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/internal/channellimit"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
)

func dialDirectResponsesGateway(t *testing.T, token string) (context.Context, *websocket.Conn) {
	return dialDirectResponsesGatewayObserved(t, token, nil)
}

func dialDirectResponsesGatewayObserved(t *testing.T, token string, writer gatewayRequestWriter) (context.Context, *websocket.Conn) {
	t.Helper()
	handler := http.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pac := &auth.ProxyAuthContext{Token: token, Source: "global", Policy: auth.EmptyDownstreamRoutingPolicy}
		HandleResponsesWebsocket(w, r.WithContext(auth.WithProxyAuth(r.Context(), pac)))
	}))
	if writer != nil {
		handler = requestLifecycleMiddleware(handler, writer)
	}
	gateway := httptest.NewServer(handler)
	t.Cleanup(gateway.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(gateway.URL, "http")+"/v1/responses", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(websocket.StatusNormalClosure, "done") })
	return ctx, conn
}

func waitDirectChannelIdle(t *testing.T, manager *channellimit.Manager, channelID int64, limits channellimit.Config) channellimit.Snapshot {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		snapshot := manager.Snapshot(channelID, limits)
		if snapshot.Active == 0 && snapshot.Waiting == 0 {
			return snapshot
		}
		if time.Now().After(deadline) {
			t.Fatalf("direct WS turn did not release channel capacity: %+v", snapshot)
		}
		// A client receiving the terminal frame does not synchronize with the
		// sender's deferred cleanup. Observe that cleanup, not the socket flush.
		time.Sleep(time.Millisecond)
	}
}

func TestDirectResponsesWebsocketTurnsRotateAndRevoke(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			var dials, calls atomic.Int32
			received := make(chan string, 8)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				dial := dials.Add(1)
				if r.Header.Get("Authorization") != "Bearer fixture-media-key" && r.Header.Get("Authorization") != "Bearer rotated-fixture" {
					t.Error("incorrect upstream auth")
				}
				conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{})
				if err != nil {
					return
				}
				defer conn.CloseNow()
				for {
					_, body, err := conn.Read(r.Context())
					if err != nil {
						return
					}
					n := calls.Add(1)
					received <- string(body)
					if !strings.Contains(string(body), `"model":"provider-model"`) || !strings.Contains(string(body), `"type":"response.create"`) || strings.Contains(string(body), `"stream":`) {
						t.Error("invalid WS event body")
					}
					output := `[{"type":"message","content":[{"type":"output_text","text":"done"}]}]`
					if n == 1 {
						output = `[{"type":"function_call","id":"fc-fixture","call_id":"call-fixture","name":"echo","arguments":"{\"n\":9007199254740993}"}]`
					}
					event := fmt.Sprintf(`{"type":"response.completed","response":{"id":"resp-%d-%d","status":"completed","model":"provider-model","created_at":9007199254740993,"output":%s,"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}}`, dial, n, output)
					if err := conn.Write(r.Context(), websocket.MessageText, []byte(event)); err != nil {
						return
					}
				}
			}))
			t.Cleanup(upstream.Close)
			db := installDirectMediaRuntime(t, dialect, store.DirectEndpoints{ResponsesWebsocket: &store.DirectEndpoint{
				URL: "ws" + strings.TrimPrefix(upstream.URL, "http") + "/native", Auth: store.DirectAuthBearer,
			}}, store.DirectProtocolResponsesWebsocket, "")
			manager := channellimit.NewManager()
			var channelID int64
			if err := db.Get(&channelID, `SELECT id FROM upstream_channels`); err != nil {
				t.Fatal(err)
			}
			limit := store.DirectRateLimit{Config: channellimit.Config{RPM: 10, MaxConcurrent: 1}}
			if _, err := db.Exec(db.Rebind(`UPDATE upstream_channels SET rate_limit=?`), limit); err != nil {
				t.Fatal(err)
			}
			routing.InvalidateCache()
			cfg := *getUpstreamConfig()
			cfg.ChannelLimits = manager
			cfg.Router.(*routing.TokenRouter).SetChannelLimits(manager)
			logs := make(chan proxy.ProxyLogEntry, 8)
			cfg.LogProxy = func(_ context.Context, entry proxy.ProxyLogEntry) error { logs <- entry; return nil }
			SetUpstreamConfig(&cfg)
			// HTTP cannot acquire the WebSocket-only grant.
			out := httptest.NewRecorder()
			HandleResponses(out, makeProxyReq("POST", "/v1/responses", `{"model":"client-alias","input":"hello"}`), "/v1/responses")
			if out.Code < 400 || dials.Load() != 0 {
				t.Fatal("HTTP borrowed WS grant")
			}
			ctx, conn := dialDirectResponsesGateway(t, "key-one-fixture")
			writeWSMessage(t, ctx, conn, `{"type":"response.create","model":"client-alias","input":[{"role":"user","content":"hello","n":9007199254740993}]}`)
			_, firstFrame, err := conn.Read(ctx)
			if err != nil || !strings.Contains(string(firstFrame), `"created_at":9007199254740993`) {
				t.Fatalf("WS response precision lost: %s %v", firstFrame, err)
			}
			var frame map[string]any
			_ = json.Unmarshal(firstFrame, &frame)
			if frame["type"] != "response.completed" {
				t.Fatalf("direct WS failed: %+v", frame)
			}
			if request := <-received; !strings.Contains(request, "9007199254740993") {
				t.Fatal("input precision lost")
			}
			writeWSMessage(t, ctx, conn, `{"type":"response.create","model":"client-alias","previous_response_id":"resp-1-1","input":[{"type":"function_call_output","call_id":"call-fixture","output":"9007199254740993"}]}`)
			if frame := readWSFrame(t, ctx, conn); frame["type"] != "response.completed" {
				t.Fatalf("continuation failed: %+v", frame)
			}
			if request := <-received; !strings.Contains(request, `"previous_response_id":"resp-1-1"`) {
				t.Fatal("continuation identity lost")
			}
			if dials.Load() != 1 {
				t.Fatal("same authorized session not reused")
			}
			if _, err := db.Exec(`UPDATE upstream_credentials SET secret='rotated-fixture'`); err != nil {
				t.Fatal(err)
			}
			writeWSMessage(t, ctx, conn, `{"type":"response.create","model":"client-alias","input":[{"role":"user","content":"new identity"}]}`)
			if frame := readWSFrame(t, ctx, conn); frame["type"] != "response.completed" {
				t.Fatalf("rotation failed: %+v", frame)
			}
			<-received
			if dials.Load() != 2 {
				t.Fatal("credential rotation reused old socket")
			}
			otherCtx, other := dialDirectResponsesGateway(t, "key-two-fixture")
			writeWSMessage(t, otherCtx, other, `{"type":"response.create","model":"client-alias","input":[{"role":"user","content":"other key"}]}`)
			if frame := readWSFrame(t, otherCtx, other); frame["type"] != "response.completed" {
				t.Fatalf("other Key failed: %+v", frame)
			}
			<-received
			if dials.Load() != 3 {
				t.Fatal("different downstream Key reused connection")
			}
			if _, err := db.Exec(`UPDATE upstream_grants SET enabled=FALSE`); err != nil {
				t.Fatal(err)
			}
			// No cache invalidation: the direct turn must reread current storage.
			writeWSMessage(t, ctx, conn, `{"type":"response.create","model":"client-alias","input":[{"role":"user","content":"revoked"}]}`)
			if frame := readWSFrame(t, ctx, conn); frame["type"] != "error" || calls.Load() != 4 {
				t.Fatalf("revoked grant executed: %+v calls=%d", frame, calls.Load())
			}
			for range 4 {
				entry := <-logs
				if entry.TotalTokens == nil || *entry.TotalTokens != 3 {
					t.Fatalf("usage changed: %+v", entry)
				}
			}
			snap := waitDirectChannelIdle(t, manager, channelID, limit.Config)
			if snap.Active != 0 || snap.Requests != 4 || snap.Tokens != 12 {
				t.Fatalf("WS admission/accounting lost: %+v", snap)
			}
		})
	}
}

func TestDirectResponsesWSNeverReplaysAfterSend(t *testing.T) {
	for _, accepted := range []bool{false, true} {
		t.Run(fmt.Sprint(accepted), func(t *testing.T) {
			var calls, fallbacks atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/http" {
					fallbacks.Add(1)
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = fmt.Fprint(w, "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"fallback\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n")
					return
				}
				if !accepted {
					http.Error(w, "not upgraded", 426)
					return
				}
				conn, err := websocket.Accept(w, r, nil)
				if err != nil {
					return
				}
				defer conn.CloseNow()
				if _, _, err = conn.Read(r.Context()); err == nil {
					calls.Add(1)
				}
			}))
			t.Cleanup(upstream.Close)
			installDirectMediaRuntime(t, store.DialectSQLite, store.DirectEndpoints{
				Responses:          &store.DirectEndpoint{URL: upstream.URL + "/http", Auth: store.DirectAuthBearer},
				ResponsesWebsocket: &store.DirectEndpoint{URL: "ws" + strings.TrimPrefix(upstream.URL, "http") + "/ws", Auth: store.DirectAuthBearer},
			}, store.DirectProtocolResponses|store.DirectProtocolResponsesWebsocket, "")
			ctx, conn := dialDirectResponsesGateway(t, "fallback-key")
			writeWSMessage(t, ctx, conn, `{"type":"response.create","model":"client-alias","input":[{"role":"user","content":"hello"}]}`)
			frame := readWSFrame(t, ctx, conn)
			if accepted {
				if frame["type"] != "error" || calls.Load() != 1 || fallbacks.Load() != 0 {
					t.Fatalf("accepted event replayed: %+v sends=%d fallback=%d", frame, calls.Load(), fallbacks.Load())
				}
			} else if frame["type"] != "response.completed" || fallbacks.Load() != 1 {
				t.Fatalf("zero event upgrade fallback lost: %+v %d", frame, fallbacks.Load())
			}
		})
	}
}

func TestResponsesWSParsePreservesLargeIntegers(t *testing.T) {
	msg, err := ParseResponsesWSMessage([]byte(`{"type":"response.create","model":"m","input":[{"role":"user","content":"hi","n":9007199254740993}]}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(cloneMap(msg.Raw))
	if !strings.Contains(string(raw), "9007199254740993") {
		t.Fatalf("large integer rounded: %s", raw)
	}
}
