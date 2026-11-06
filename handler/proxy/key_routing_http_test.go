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

	"github.com/coder/websocket"
	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/service/oauth"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestImportedKeyRoutingHTTPAffinityAndPolicyIsolation(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			var calls [2]atomic.Int32
			var failFirst atomic.Bool
			servers := make([]*httptest.Server, 2)
			for i := range servers {
				servers[i] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls[i].Add(1)
					if i == 0 && failFirst.Load() {
						w.WriteHeader(503)
						_, _ = fmt.Fprint(w, `{"error":{"message":"temporarily unavailable"}}`)
						return
					}
					var body struct {
						Stream bool `json:"stream"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if body.Stream {
						w.Header().Set("Content-Type", "text/event-stream")
						if r.URL.Path == "/v1/responses" {
							_, _ = fmt.Fprintf(w, "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"response-fixture\",\"status\":\"completed\",\"model\":\"provider\",\"output\":[{\"id\":\"message-fixture\",\"type\":\"message\",\"role\":\"assistant\",\"status\":\"completed\",\"content\":[{\"type\":\"output_text\",\"text\":\"channel-%d\",\"annotations\":[]}]}],\"usage\":{\"input_tokens\":1,\"output_tokens\":1,\"total_tokens\":2}}}\n\n", i)
							return
						}
						_, _ = fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"channel-%d\"}}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}}\n\ndata: [DONE]\n\n", i)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"content":"channel-%d"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`, i)
				}))
				defer servers[i].Close()
			}
			db := installDirectMediaRuntime(t, dialect, store.DirectEndpoints{
				Chat: &store.DirectEndpoint{URL: servers[0].URL + "/v1/chat/completions", Auth: store.DirectAuthBearer},
			}, store.DirectProtocolChat, "")
			channels := []any{}
			for i := range servers {
				channels = append(channels, map[string]any{
					"id": i + 1, "type": "qiniu", "name": fmt.Sprintf("route-%d", i), "base_url": servers[i].URL,
					"credentials": map[string]any{"apiKey": fmt.Sprintf("synthetic-upstream-%d", i)}, "supported_models": []string{"provider"},
				})
			}
			payload := map[string]any{
				"version": "1.4", "channels": channels,
				"models": []any{map[string]any{"id": 1, "model_id": "routing-model", "type": "chat", "settings": map[string]any{
					"loadBalancerStrategy": "round-robin", "traceStickyMode": "disabled",
					"associations": []any{map[string]any{"type": "model", "modelId": map[string]any{"modelId": "provider"}}},
				}}},
				"projects": []any{map[string]any{"id": 1, "name": "fixture", "status": "active"}},
				"api_keys": []any{
					map[string]any{"id": 1, "key": "synthetic-sticky-key", "project_id": 1, "name": "sticky", "scopes": []string{"write_requests"}, "profiles": map[string]any{"activeProfile": "active", "profiles": []any{map[string]any{"name": "active", "loadBalanceStrategy": "round-robin", "traceStickyMode": "prefer_previous_channel"}}}},
					map[string]any{"id": 2, "key": "synthetic-plain-key", "project_id": 1, "name": "plain", "scopes": []string{"write_requests"}, "profiles": map[string]any{"activeProfile": "active", "profiles": []any{map[string]any{"name": "active", "loadBalanceStrategy": "default", "traceStickyMode": "disabled"}}}},
				},
			}
			raw, _ := json.Marshal(payload)
			if _, err := backup.ImportAxonHubV14(db, raw, "key-routing", false); err != nil {
				t.Fatal(err)
			}
			videoExec(t, db, `UPDATE upstream_group_items SET last_selected_at='2001-01-01T00:00:00Z' WHERE origin_key='key-routing'`)
			videoExec(t, db, `UPDATE upstream_group_items SET last_selected_at='2000-01-01T00:00:00Z' WHERE grant_id IN (SELECT g.id FROM upstream_grants g JOIN upstream_models m ON m.id=g.model_id JOIN upstream_channels c ON c.id=m.channel_id WHERE c.origin_key='key-routing' AND c.source_id=1)`)
			previousDB, previousCfg := store.GetDB(), config.GetSafe()
			store.OverrideDB(db)
			cfg := &config.Config{ProxyMaxChannelAttempts: 2}
			config.Set(cfg)
			t.Cleanup(func() { store.OverrideDB(previousDB); config.Set(previousCfg) })
			router := routing.NewTokenRouter(service.NewProxyRoutingStore(db), cfg, nil, nil)
			SetUpstreamConfig(&UpstreamConfig{
				Router: router, LogProxy: func(context.Context, proxy.ProxyLogEntry) error { return nil },
				ResolveDirectCredential: func(ctx context.Context, id int64, proxyURL *string, force bool) (*oauth.DirectCredentialResult, error) {
					return oauth.ResolveDirectCredential(ctx, db.DB, id, proxyURL, force)
				},
			})
			handler := requestLifecycleMiddleware(auth.ProxyAuth()(http.HandlerFunc(HandleChatCompletions)), func(context.Context, store.GatewayRequest) error { return nil })
			call := func(key, trace string) string {
				req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"routing-model","messages":[{"role":"user","content":"hello"}]}`))
				req.Header.Set("Authorization", "Bearer "+key)
				req.Header.Set("AH-Trace-Id", trace)
				out := httptest.NewRecorder()
				handler.ServeHTTP(out, req)
				if out.Code != 200 {
					t.Fatalf("key routing status=%d body=%s", out.Code, out.Body.String())
				}
				return out.Body.String()
			}
			first := call("synthetic-sticky-key", "same-trace")
			if !strings.Contains(first, "channel-0") || !strings.Contains(call("synthetic-sticky-key", "same-trace"), "channel-0") {
				t.Fatal("successful channel was not preferred within one Key")
			}
			if !strings.Contains(call("synthetic-plain-key", "same-trace"), "channel-1") {
				t.Fatal("another Key inherited trace affinity or round-robin never advanced")
			}
			// A current Key restriction makes the previous channel ineligible.
			var channelIDs []int64
			if err := db.Select(&channelIDs, `SELECT id FROM upstream_channels WHERE origin_key='key-routing' ORDER BY source_id`); err != nil || len(channelIDs) != 2 {
				t.Fatalf("channel IDs=%v err=%v", channelIDs, err)
			}
			var policy store.DownstreamAccessPolicy
			var policyRaw string
			if err := db.Get(&policyRaw, db.Rebind(`SELECT access_policy FROM downstream_api_keys WHERE key=?`), "synthetic-sticky-key"); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(policyRaw), &policy); err != nil {
				t.Fatal(err)
			}
			allowed := channelIDs[1:]
			policy.AllowedUpstreamChannelIDs = &allowed
			next, _ := json.Marshal(policy)
			videoExec(t, db, `UPDATE downstream_api_keys SET access_policy=? WHERE key=?`, string(next), "synthetic-sticky-key")
			if !strings.Contains(call("synthetic-sticky-key", "same-trace"), "channel-1") {
				t.Fatal("sticky preference bypassed current Key channel permission")
			}
			policy.AllowedUpstreamChannelIDs = &channelIDs
			next, _ = json.Marshal(policy)
			videoExec(t, db, `UPDATE downstream_api_keys SET access_policy=? WHERE key=?`, string(next), "synthetic-sticky-key")
			// New trace selects the least-recently-selected first channel. Its
			// failure falls back, and only the completed second channel sticks.
			failFirst.Store(true)
			before := calls[1].Load()
			if !strings.Contains(call("synthetic-sticky-key", "new-trace"), "channel-1") || calls[1].Load() != before+1 {
				t.Fatal("failed preferred attempt did not use authorized fallback")
			}
			failFirst.Store(false)
			if !strings.Contains(call("synthetic-sticky-key", "new-trace"), "channel-1") {
				t.Fatal("failed first channel replaced the successful preference")
			}
			// The downstream Responses WS bridge carries the same Key-scoped
			// identity even though AH-Trace-Id is not an upstream forwarding header.
			for i, channelID := range channelIDs {
				endpoints := store.DirectEndpoints{
					Chat:      &store.DirectEndpoint{URL: servers[i].URL + "/v1/chat/completions", Auth: store.DirectAuthBearer},
					Responses: &store.DirectEndpoint{URL: servers[i].URL + "/v1/responses", Auth: store.DirectAuthBearer},
				}
				videoExec(t, db, `UPDATE upstream_channels SET endpoint_config=? WHERE id=?`, endpoints, channelID)
			}
			videoExec(t, db, `UPDATE upstream_grants SET protocols=6 WHERE origin_key='key-routing'`)
			videoExec(t, db, `UPDATE upstream_group_items SET protocol_order='[2,4]' WHERE origin_key='key-routing'`)
			routing.InvalidateCache()
			wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				pac := &auth.ProxyAuthContext{Source: "global", Token: "synthetic-ws-key", Policy: auth.DownstreamRoutingPolicy{
					AccessPolicy: &store.DownstreamAccessPolicy{TraceStickyMode: "prefer_previous_channel", RoutingStrategy: "round_robin"},
				}}
				HandleResponsesWebsocket(w, r.WithContext(auth.WithProxyAuth(r.Context(), pac)))
			}))
			defer wsServer.Close()
			wsCtx, cancel := context.WithCancel(t.Context())
			defer cancel()
			conn, _, err := websocket.Dial(wsCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http")+"/v1/responses", &websocket.DialOptions{
				HTTPHeader: http.Header{"AH-Trace-Id": []string{"shared-ws-trace"}},
			})
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close(websocket.StatusNormalClosure, "done")
			beforeWS := [2]int32{calls[0].Load(), calls[1].Load()}
			for range 2 {
				if err := conn.Write(wsCtx, websocket.MessageText, []byte(`{"type":"response.create","model":"routing-model","input":[{"role":"user","content":"hello"}]}`)); err != nil {
					t.Fatal(err)
				}
				for {
					frame := readWSFrame(t, wsCtx, conn)
					if frame["type"] == "error" || frame["type"] == "response.failed" {
						t.Fatalf("WS terminal=%v", frame)
					}
					if frame["type"] == "response.completed" {
						break
					}
				}
			}
			deltas := [2]int32{calls[0].Load() - beforeWS[0], calls[1].Load() - beforeWS[1]}
			if deltas != [2]int32{2, 0} && deltas != [2]int32{0, 2} {
				t.Fatalf("WS bridge lost same-Key trace affinity: calls=%v", deltas)
			}
		})
	}
}
