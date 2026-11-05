package proxyhandler

import (
	"context"
	"encoding/json"
	"io"
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

func TestDirectRequestOperationsRespectRealUserBudgetAdmission(t *testing.T) {
	for _, tc := range []struct {
		name       string
		operations []any
		status     int
	}{
		{"headers only", nil, 200},
		{"shrinking body", []any{map[string]any{"op": "delete", "path": "metadata"}}, 200},
		{"prompt expansion", []any{map[string]any{"op": "array_append", "path": "messages", "value": `{"role":"user","content":"extra prompt material"}`}}, 403},
		{"opaque continuation", []any{map[string]any{"op": "set", "path": "previous_response_id", "value": "foreign-response"}}, 403},
		{"hosted tool", []any{map[string]any{"op": "set", "path": "tools", "value": `[{"type":"web_search"}]`}}, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, users, user, keys := newBudgetProxyUser(t)
			ctx := context.Background()
			videoExec(t, db, `UPDATE token_routes SET created_at='2026-11-05T00:00:00Z',updated_at='2026-11-05T00:00:00Z'`)
			if _, err := users.SetUserBudget(ctx, user.ID, 1, false); err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var body map[string]any
				if json.NewDecoder(r.Body).Decode(&body) != nil || body["max_tokens"] != float64(16) || r.Header.Get("X-Imported") != "yes" {
					t.Errorf("bounded user request/header changed: %#v", body)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"fixture","object":"chat.completion","model":"provider","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`)
			}))
			defer upstream.Close()
			raw, _ := json.Marshal(map[string]any{
				"version": "1.4", "channels": []any{map[string]any{
					"id": 1, "type": "qiniu", "name": "operation-fixture", "base_url": upstream.URL,
					"credentials":      map[string]any{"apiKey": "synthetic-upstream"},
					"supported_models": []string{"provider"},
					"settings": map[string]any{
						"bodyOverrideOperations":   tc.operations,
						"headerOverrideOperations": []any{map[string]any{"op": "set", "path": "X-Imported", "value": "yes"}},
					},
				}},
				"models": []any{map[string]any{
					"id": 1, "model_id": "operation-model", "type": "chat",
					"settings": map[string]any{"associations": []any{map[string]any{"type": "channel_model", "channelModel": map[string]any{"channelId": 1, "modelId": "provider"}}}},
				}},
			})
			if _, err := backup.ImportAxonHubV14(db, raw, "user-operations", false); err != nil {
				t.Fatal(err)
			}
			var routeID int64
			if err := db.Get(&routeID, `SELECT id FROM token_routes WHERE model_pattern='operation-model'`); err != nil {
				t.Fatal(err)
			}
			if err := users.GrantUserGroupRoute(ctx, user.GroupID, routeID); err != nil {
				t.Fatal(err)
			}
			loaded, err := service.NewProxyRoutingStore(db).LoadRouteChannels(ctx, []int64{routeID})
			if err != nil || len(loaded) != 1 {
				t.Fatalf("fixture candidates: %v count=%d", err, len(loaded))
			}
			priorConfig, priorUpstream := config.GetSafe(), getUpstreamConfig()
			config.Set(&config.Config{ProxyMaxChannelAttempts: 1})
			t.Cleanup(func() { config.Set(priorConfig); SetUpstreamConfig(priorUpstream); routing.SetGlobalCache(nil) })
			SetUpstreamConfig(&UpstreamConfig{
				Router:   routing.NewTokenRouter(service.NewProxyRoutingStore(db), &config.Config{}, nil, nil),
				LogProxy: func(context.Context, proxy.ProxyLogEntry) error { return nil },
				ResolveDirectCredential: func(ctx context.Context, id int64, proxyURL *string, force bool) (*oauth.DirectCredentialResult, error) {
					return oauth.ResolveDirectCredential(ctx, db.DB, id, proxyURL, force)
				},
			})
			handler := requestLifecycleMiddleware(auth.ProxyAuth()(http.HandlerFunc(HandleChatCompletions)), func(context.Context, store.GatewayRequest) error { return nil })
			body := `{"model":"operation-model","messages":[{"role":"user","content":"hello"}],"metadata":{"extra":"delete me"},"max_tokens":16}`
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+keys[0])
			out := httptest.NewRecorder()
			handler.ServeHTTP(out, req)
			if out.Code != tc.status {
				t.Fatalf("status=%d want=%d body=%s", out.Code, tc.status, out.Body.String())
			}
			wantCalls := int32(0)
			if tc.status == 200 {
				wantCalls = 1
			}
			if calls.Load() != wantCalls {
				t.Fatalf("calls=%d want=%d", calls.Load(), wantCalls)
			}
			budget, err := users.GetUserBudget(ctx, user.ID)
			if err != nil || budget.Reserved != 0 {
				t.Fatalf("budget not settled: %+v %v", budget, err)
			}
			if tc.status != 200 && budget.Used != 0 {
				t.Fatalf("refused zero-dispatch request billed %+v", budget)
			}
		})
	}
}
