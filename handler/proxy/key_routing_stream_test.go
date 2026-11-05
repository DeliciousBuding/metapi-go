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

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/service/oauth"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestKeyAffinityRespectsStreamPreferenceAndChannelFailure(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			var calls [2]atomic.Int32
			var failRequired atomic.Bool
			servers := make([]*httptest.Server, 2)
			for index := range servers {
				servers[index] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls[index].Add(1)
					var body struct {
						Stream bool `json:"stream"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if index == 0 && !body.Stream {
						t.Error("required channel received nonstream")
					}
					if index == 0 && failRequired.Load() {
						w.WriteHeader(503)
						_, _ = fmt.Fprint(w, `{"error":{"message":"temporary failure"}}`)
						return
					}
					if body.Stream {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"channel-%d\"}}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}}\n\ndata: [DONE]\n\n", index)
					} else {
						w.Header().Set("Content-Type", "application/json")
						_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"content":"channel-%d"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`, index)
					}
				}))
				defer servers[index].Close()
			}
			db := installDirectMediaRuntime(t, dialect, store.DirectEndpoints{Chat: &store.DirectEndpoint{URL: servers[0].URL, Auth: store.DirectAuthBearer}}, store.DirectProtocolChat, "")
			payload := map[string]any{"version": "1.4", "channels": []any{
				map[string]any{"id": 1, "type": "qiniu", "name": "required", "base_url": servers[0].URL, "credentials": map[string]any{"apiKeys": []string{"fixture-first", "fixture-second"}}, "supported_models": []string{"provider"}, "policies": map[string]any{"stream": "require"}},
				map[string]any{"id": 2, "type": "qiniu", "name": "native", "base_url": servers[1].URL, "credentials": map[string]any{"apiKey": "fixture-native"}, "supported_models": []string{"provider"}},
			}, "models": []any{map[string]any{"id": 1, "model_id": "stream-affinity", "settings": map[string]any{"traceStickyMode": "prefer_previous_channel", "loadBalancerStrategy": "round-robin", "associations": []any{map[string]any{"type": "model", "modelId": map[string]any{"modelId": "provider"}}}}}}}
			raw, _ := json.Marshal(payload)
			if _, err := backup.ImportAxonHubV14(db, raw, "stream-affinity", false); err != nil {
				t.Fatal(err)
			}
			videoExec(t, db, `UPDATE upstream_channels SET enabled=FALSE WHERE origin_key='stream-affinity' AND source_id=2`)
			priorDB, priorCfg := store.GetDB(), config.GetSafe()
			store.OverrideDB(db)
			cfg := &config.Config{ProxyMaxChannelAttempts: 3}
			config.Set(cfg)
			t.Cleanup(func() { store.OverrideDB(priorDB); config.Set(priorCfg) })
			router := routing.NewTokenRouter(service.NewProxyRoutingStore(db), cfg, nil, nil)
			SetUpstreamConfig(&UpstreamConfig{Router: router, LogProxy: func(context.Context, proxy.ProxyLogEntry) error { return nil }, ResolveDirectCredential: func(ctx context.Context, id int64, proxyURL *string, force bool) (*oauth.DirectCredentialResult, error) {
				return oauth.ResolveDirectCredential(ctx, db.DB, id, proxyURL, force)
			}})
			call := func(stream bool) *httptest.ResponseRecorder {
				req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(fmt.Sprintf(`{"model":"stream-affinity","stream":%t,"messages":[{"role":"user","content":"hello"}]}`, stream)))
				req.Header.Set("AH-Trace-Id", "fixture-stable-trace")
				req = req.WithContext(auth.WithProxyAuth(req.Context(), &auth.ProxyAuthContext{Source: "global", Token: "fixture-downstream"}))
				out := httptest.NewRecorder()
				HandleChatCompletions(out, req)
				return out
			}
			if out := call(false); out.Code != 200 || !strings.Contains(out.Body.String(), "channel-0") {
				t.Fatalf("initial required aggregate: %d %s", out.Code, out.Body.String())
			}
			videoExec(t, db, `UPDATE upstream_channels SET enabled=TRUE WHERE origin_key='stream-affinity' AND source_id=2`)
			routing.InvalidateCache()
			if out := call(false); out.Code != 200 || !strings.Contains(out.Body.String(), "channel-1") || calls[0].Load() != 1 {
				t.Fatalf("sticky required channel bypassed new native: %d %s calls=%d", out.Code, out.Body.String(), calls[0].Load())
			}
			videoExec(t, db, `UPDATE upstream_channels SET stream_policy='forbid' WHERE origin_key='stream-affinity' AND source_id=2`)
			routing.InvalidateCache()
			if out := call(true); out.Code != 200 || !strings.Contains(out.Body.String(), "channel-0") || calls[1].Load() != 1 {
				t.Fatalf("sticky forbid channel received stream: %d %s", out.Code, out.Body.String())
			}
			before := calls[0].Load()
			failRequired.Store(true)
			out := call(true)
			if out.Code < 500 || calls[0].Load() != before+1 || calls[1].Load() != 1 {
				t.Fatalf("failed sticky channel escaped via sibling credential/forbid: status=%d required=%d native=%d body=%s", out.Code, calls[0].Load()-before, calls[1].Load(), out.Body.String())
			}
		})
	}
}
