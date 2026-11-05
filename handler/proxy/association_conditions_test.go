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

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestAssociationDescriptorBuiltOnceFromImmutableRequest(t *testing.T) {
	raw := []byte(`{"messages":[{"role":"user","content":"hello"}]}`)
	headers := http.Header{"X-Route": []string{"before"}}
	resolve := lazyAssociationRequest("/v1/chat/completions", raw, headers, false, time.Now())
	headers.Set("X-Route", "after")
	first := resolve()
	if first == nil || first.Headers["X-Route"] != "before" || resolve() != first {
		t.Fatal("request descriptor was not one immutable request snapshot")
	}
	if associationRequest("/v1/embeddings", []byte(`{"input":"hello"}`), nil, false, time.Now()) == nil {
		t.Fatal("supported non-generation source format lost request context")
	}
}

func TestImportedAssociationConditionsChoosePerRequest(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			var counts [2]atomic.Int32
			upstreams := make([]*httptest.Server, 2)
			for index := range upstreams {
				i := index
				upstreams[i] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					counts[i].Add(1)
					var payload map[string]any
					if json.NewDecoder(r.Body).Decode(&payload) != nil || payload["model"] != "provider-model" {
						t.Error("selection lost actual model")
					}
					metadata, _ := payload["metadata"].(map[string]any)
					if metadata["channel"] != float64(i+1) || r.Header.Get("X-Route") != fmt.Sprintf("rewritten-%d", i+1) {
						t.Error("selected channel lost its request operations")
					}
					response := fmt.Sprintf(`{"id":"fixture","object":"chat.completion","model":"provider-model","choices":[{"index":0,"message":{"role":"assistant","content":"channel-%d"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`, i+1)
					if payload["stream"] == true {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = fmt.Fprintf(w, "data: {\"id\":\"fixture\",\"object\":\"chat.completion.chunk\",\"model\":\"provider-model\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"channel-%d\"},\"finish_reason\":null}]}\n\ndata: {\"id\":\"fixture\",\"object\":\"chat.completion.chunk\",\"model\":\"provider-model\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":1,\"total_tokens\":3}}\n\ndata: [DONE]\n\n", i+1)
					} else {
						w.Header().Set("Content-Type", "application/json")
						_, _ = fmt.Fprint(w, response)
					}
				}))
				t.Cleanup(upstreams[i].Close)
			}
			db := installDirectMediaRuntime(t, dialect, store.DirectEndpoints{Chat: &store.DirectEndpoint{URL: upstreams[0].URL + "/v1/chat/completions", Auth: store.DirectAuthBearer}}, store.DirectProtocolChat, "")
			leaf := func(field, operator string, value any) map[string]any {
				return map[string]any{"type": "condition", "field": field, "operator": operator, "value": value}
			}
			association := func(channel, priority int, conditions ...any) map[string]any {
				return map[string]any{"type": "channel_model", "priority": priority, "channelModel": map[string]any{"channelId": channel, "modelId": "provider-model"}, "when": map[string]any{"enabled": true, "condition": map[string]any{"type": "group", "logic": "AND", "conditions": conditions}}}
			}
			payload := map[string]any{"version": "1.4", "channels": []any{
				map[string]any{"id": 1, "type": "qiniu", "name": "Conditional primary", "base_url": upstreams[0].URL, "credentials": map[string]any{"apiKey": "synthetic-first"}, "supported_models": []string{"provider-model"}, "endpoints": []any{map[string]any{"api_format": "openai/chat_completions"}}},
				map[string]any{"id": 2, "type": "qiniu", "name": "Conditional second", "base_url": upstreams[1].URL, "credentials": map[string]any{"apiKey": "synthetic-second"}, "supported_models": []string{"provider-model"}, "endpoints": []any{map[string]any{"api_format": "openai/chat_completions"}}},
			}, "models": []any{map[string]any{"id": 1, "model_id": "client-alias", "settings": map[string]any{"associations": []any{
				association(1, 1, leaf("request_header.X-Route", "eq", "primary"), leaf("stream", "eq", true)),
				association(1, 9, leaf("request_header.X-Route", "eq", "primary"), leaf("stream", "eq", false)),
				association(2, 3, leaf("request_header.X-Route", "eq", "second")),
			}}}}}
			// Conditions use the original request; ordered operations apply only
			// after selecting a channel. Neither configuration may erase the other
			// during import, reimport or candidate loading.
			for index, channel := range payload["channels"].([]any) {
				channel.(map[string]any)["settings"] = map[string]any{
					"bodyOverrideOperations": []any{map[string]any{
						"op": "set", "path": "metadata.channel", "value": fmt.Sprint(index + 1),
					}},
					"headerOverrideOperations": []any{map[string]any{
						"op": "set", "path": "X-Route", "value": fmt.Sprintf("rewritten-%d", index+1),
					}},
				}
			}
			raw, _ := json.Marshal(payload)
			for i := 0; i < 2; i++ {
				if _, err := backup.ImportAxonHubV14(db, raw, "media-fixture", false); err != nil {
					t.Fatal(err)
				}
			}
			policy := routing.EmptyDownstreamRoutingPolicy
			policy.RequiredUpstreamProtocol = store.DirectProtocolChat
			decision, err := getUpstreamConfig().Router.(interface {
				ExplainSelection(context.Context, string, []int64, routing.DownstreamRoutingPolicy) (routing.RouteDecisionExplanation, error)
			}).ExplainSelection(context.Background(), "client-alias", nil, policy)
			if err != nil {
				t.Fatal(err)
			}
			if len(decision.Candidates) != 2 {
				t.Fatalf("duplicate or missing shared-grant members: %d", len(decision.Candidates))
			}
			for _, candidate := range decision.Candidates {
				if candidate.Eligible || candidate.Eligibility != "notEvaluated" || !candidate.AssociationContextRequired {
					t.Fatalf("preview pretended to evaluate request conditions: %+v", candidate)
				}
			}
			catalog := getAvailableModels(context.Background(), auth.EmptyDownstreamRoutingPolicy)
			found := false
			for _, name := range catalog {
				found = found || name == "client-alias"
			}
			if !found {
				t.Fatal("conditional model vanished from catalog before a request existed")
			}
			for _, tc := range []struct {
				header string
				stream bool
				want   int
			}{
				{"primary", false, 1}, {"second", false, 2}, {"primary", true, 1}, {"none", false, 0}, {"second", true, 2}, {"primary", false, 1},
			} {
				body := fmt.Sprintf(`{"model":"client-alias","stream":%t,"messages":[{"role":"user","content":"hello"}],"max_tokens":8}`, tc.stream)
				req := makeProxyReq(http.MethodPost, "/v1/chat/completions", body)
				req.Header.Set("x-route", tc.header)
				before := counts[0].Load() + counts[1].Load()
				out := httptest.NewRecorder()
				HandleChatCompletions(out, req)
				if tc.want == 0 {
					if out.Code < 400 || counts[0].Load()+counts[1].Load() != before {
						t.Fatalf("condition refusal bypassed: %d %s", out.Code, out.Body.String())
					}
				} else if out.Code != 200 || !strings.Contains(out.Body.String(), fmt.Sprintf("channel-%d", tc.want)) || counts[0].Load()+counts[1].Load() != before+1 {
					t.Fatalf("wrong per-request route: %d %s", out.Code, out.Body.String())
				}
			}
			// A matching predicate is still bounded by the Key's hard scope.
			before := counts[0].Load() + counts[1].Load()
			req := makeProxyReq(http.MethodPost, "/v1/chat/completions", `{"model":"client-alias","messages":[{"role":"user","content":"hello"}]}`)
			req.Header.Set("X-Route", "primary")
			req = req.WithContext(auth.WithProxyAuth(req.Context(), &auth.ProxyAuthContext{Token: "synthetic-restricted", Source: "managed", Policy: auth.DownstreamRoutingPolicy{AllowedSiteIDs: []int64{999}}}))
			out := httptest.NewRecorder()
			HandleChatCompletions(out, req)
			if out.Code < 400 || counts[0].Load()+counts[1].Load() != before {
				t.Fatal("condition bypassed hard Key scope")
			}
			emptyRoutes := []int64{}
			req = makeProxyReq(http.MethodPost, "/v1/chat/completions", `{"model":"client-alias","messages":[{"role":"user","content":"hello"}]}`)
			req.Header.Set("X-Route", "primary")
			req = req.WithContext(auth.WithProxyAuth(req.Context(), &auth.ProxyAuthContext{
				Token: "synthetic-user", Source: "managed", Policy: auth.DownstreamRoutingPolicy{UserRouteIDs: &emptyRoutes},
			}))
			out = httptest.NewRecorder()
			HandleChatCompletions(out, req)
			if out.Code < 400 || counts[0].Load()+counts[1].Load() != before {
				t.Fatal("condition bypassed the user's empty hard group grant")
			}
		})
	}
}
