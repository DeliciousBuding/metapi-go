package proxyhandler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/deliciousbuding/metapi-go/transform/anthropic/messages"
)

type directReplayRouter struct {
	proxy.TokenRouterInterface
	normal    int
	preferred []int64
}

func (r *directReplayRouter) SelectChannel(ctx context.Context, model string, policy routing.DownstreamRoutingPolicy) (*routing.SelectedChannel, error) {
	r.normal++
	return r.TokenRouterInterface.SelectChannel(ctx, model, policy)
}

func (r *directReplayRouter) SelectPreferredChannel(ctx context.Context, model string, id int64, policy routing.DownstreamRoutingPolicy, excluded []int64) (*routing.SelectedChannel, error) {
	r.preferred = append(r.preferred, id)
	return r.TokenRouterInterface.SelectPreferredChannel(ctx, model, id, policy, excluded)
}

func directReplayFixture(t *testing.T, baseURL string) *store.DB {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"version": "1.4", "channels": []any{map[string]any{
			"id": 1, "type": "moonshot", "name": "replay fixture", "base_url": baseURL,
			"credentials": map[string]any{"apiKey": "fixture-upstream-key"}, "supported_models": []string{"provider-model"},
		}},
		"models": []any{map[string]any{"id": 1, "model_id": "client-alias", "status": "enabled",
			"settings": map[string]any{"associations": []any{map[string]any{"type": "channel_model", "channelModel": map[string]any{"channelId": 1, "modelId": "provider-model"}}}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return installAxonHubRawFixture(t, raw)
}

func directReplayToolResponse(t *testing.T, output string, stream bool) []byte {
	t.Helper()
	if strings.Contains(output, "private planning") || strings.Contains(output, "reasoning_content") {
		t.Fatal("hidden upstream reasoning leaked to the Messages client")
	}
	if !stream {
		return []byte(output)
	}
	var blocks []any
	for _, line := range strings.Split(output, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event map[string]any
		if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event) != nil {
			continue
		}
		if event["type"] == "content_block_start" {
			block, _ := event["content_block"].(map[string]any)
			if block["type"] == "tool_use" {
				blocks = append(blocks, block)
			}
		}
	}
	if len(blocks) != 1 {
		t.Fatalf("stream must contain one tool block: %s", output)
	}
	// This fixture's function takes an empty object; its SSE JSON delta is {}.
	reply, err := json.Marshal(map[string]any{"content": blocks})
	if err != nil {
		t.Fatal(err)
	}
	return reply
}

func TestDirectMessagesReasoningReplayHTTP(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, mutation := range []string{"none", "downstream key", "conversation", "channel policy", "credential rotation", "missing record", "malformed tool id", "foreign tool id"} {
			t.Run(map[bool]string{false: "json", true: "sse"}[stream]+"/"+mutation, func(t *testing.T) {
				var calls atomic.Int32
				observed := make(chan map[string]any, 4)
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					observed <- body
					turn := calls.Add(1)
					w.Header().Set("Content-Type", "application/json")
					if turn == 1 {
						if stream {
							w.Header().Set("Content-Type", "text/event-stream")
							_, _ = io.WriteString(w, "data: "+`{"id":"audit-answer","object":"chat.completion.chunk","model":"provider-model","choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"private planning","tool_calls":[{"index":0,"id":"audit-call","type":"function","function":{"name":"echo","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`+"\n\ndata: [DONE]\n\n")
						} else {
							_, _ = io.WriteString(w, auditDomesticThinkingTool)
						}
					} else if stream {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, "data: "+`{"id":"audit-answer","object":"chat.completion.chunk","model":"provider-model","choices":[{"index":0,"delta":{"role":"assistant","content":"receipt"},"finish_reason":"stop"}]}`+"\n\ndata: [DONE]\n\n")
					} else {
						_, _ = io.WriteString(w, auditDomesticAnswer)
					}
				}))
				defer upstream.Close()
				db := directReplayFixture(t, upstream.URL)
				cfg := *getUpstreamConfig()
				router := &directReplayRouter{TokenRouterInterface: cfg.Router}
				cfg.Router = router
				SetUpstreamConfig(&cfg)
				var ctx Ctx
				if err := json.Unmarshal([]byte(`{"model":"client-alias","max_tokens":64,"thinking":{"type":"adaptive"},"metadata":{"user_id":"replay-conversation"},"messages":[{"role":"user","content":"call echo"}],"tools":[{"name":"echo","input_schema":{"type":"object"}}]}`), &ctx.Body); err != nil {
					t.Fatal(err)
				}
				ctx.Body["stream"] = stream
				messagesReplayRefresh(t, &ctx)
				first := httptest.NewRecorder()
				HandleClaudeMessages(first, makeProxyReq(http.MethodPost, "/v1/messages", string(ctx.RawBody)))
				if first.Code != 200 || strings.Contains(first.Body.String(), `"type":"error"`) {
					t.Fatalf("first turn failed: %d %s", first.Code, first.Body.String())
				}
				ids := messagesReplayAppendResponse(t, &ctx, directReplayToolResponse(t, first.Body.String(), stream))
				if len(ids) != 1 || !strings.HasPrefix(ids[0], messagesBridgeToolIDPrefix) {
					t.Fatalf("missing bridge-owned tool ID: %v", ids)
				}
				<-observed
				router.normal = 0
				if mutation == "conversation" {
					ctx.Body["metadata"].(map[string]any)["user_id"] = "different-conversation"
				}
				if mutation == "credential rotation" {
					if _, err := db.Exec(`UPDATE upstream_credentials SET secret = ?`, "rotated-fixture-key"); err != nil {
						t.Fatal(err)
					}
				}
				messagesReplayRefresh(t, &ctx)
				if replacement := map[string]string{"missing record": messagesBridgeToolIDPrefix + strings.Repeat("0", 32), "malformed tool id": messagesBridgeToolIDPrefix + "bad", "foreign tool id": "call_foreign"}[mutation]; replacement != "" {
					ctx.RawBody = []byte(strings.ReplaceAll(string(ctx.RawBody), ids[0], replacement))
				}
				req := makeProxyReq(http.MethodPost, "/v1/messages", string(ctx.RawBody))
				pac := auth.GetProxyAuth(req.Context())
				if mutation == "downstream key" {
					pac.Token = "different-downstream-key"
				}
				if mutation == "channel policy" {
					deny := []int64{}
					pac.Policy.AccessPolicy = &store.DownstreamAccessPolicy{AllowedUpstreamChannelIDs: &deny}
				}
				second := httptest.NewRecorder()
				HandleClaudeMessages(second, req)
				if mutation != "none" {
					if second.Code < 400 || calls.Load() != 1 {
						t.Fatalf("old replay escaped %s: status=%d calls=%d body=%s", mutation, second.Code, calls.Load(), second.Body.String())
					}
					if strings.Contains(second.Body.String(), "private planning") {
						t.Fatal("replay error leaked reasoning")
					}
					return
				}
				if second.Code != 200 || calls.Load() != 2 || !strings.Contains(second.Body.String(), "receipt") {
					t.Fatalf("second turn failed: status=%d calls=%d body=%s", second.Code, calls.Load(), second.Body.String())
				}
				if router.normal != 0 || len(router.preferred) != 1 || router.preferred[0] >= 0 {
					t.Fatalf("continuation did not select original negative Direct item: normal=%d preferred=%v", router.normal, router.preferred)
				}
				next := <-observed
				found := false
				for _, value := range next["messages"].([]any) {
					msg := value.(map[string]any)
					if msg["role"] == "assistant" {
						found = true
						if msg["reasoning_content"] != "private planning" {
							t.Fatalf("reasoning history lost: %v", msg)
						}
					}
				}
				if !found {
					t.Fatal("assistant tool history missing")
				}
			})
		}
	}
}

func TestDirectMessagesReplayScopeIsolation(t *testing.T) {
	for _, field := range []string{"route", "group", "item", "channel", "credential", "grant", "model", "token", "endpoint", "header", "parameters", "protocol", "provider"} {
		t.Run(field, func(t *testing.T) {
			cache := newMessagesReplayCache()
			r, ctx, selected := messagesReplayFixture(t, "read")
			selected.Channel.ID = -3
			selected.Direct = &store.DirectUpstreamCandidate{RouteID: 1, GroupID: 2, ItemID: 3, ChannelID: 4, CredentialID: 5, GrantID: 6, ModelID: 7, Protocols: 1}
			if err := messagesReplayFor(cache, r, ctx, selected).Options().SaveReasoning(messagesReplayFixtureIDs("a"), "hidden"); err != nil {
				t.Fatal(err)
			}
			messagesReplayAppend(t, ctx, "a")
			messagesReplayExpectChannel(t, cache, ctx, -3)
			d := selected.Direct
			switch field {
			case "route":
				d.RouteID++
			case "group":
				d.GroupID++
			case "item":
				d.ItemID++
				selected.Channel.ID = -d.ItemID
			case "channel":
				d.ChannelID++
			case "credential":
				d.CredentialID++
			case "grant":
				d.GrantID++
			case "model":
				d.ModelID++
			case "token":
				selected.TokenValue = "rotated-key"
			case "endpoint":
				d.BaseURL = "https://other.invalid"
			case "header":
				d.CustomHeader = `{"x-provider-account":"other"}`
			case "parameters":
				d.ParamOverride = `{"model":"other"}`
			case "protocol":
				d.Protocols = 2
			case "provider":
				d.Provider = "other"
			}
			next := messagesReplayFor(cache, r, ctx, selected)
			body, err := messages.ToChatRequest(ctx.RawBody, next.Options())
			if !errors.Is(err, messages.ErrReasoningReplay) || len(body) != 0 || next.UsedReplay() {
				t.Fatalf("changed %s reused prior replay: %v", field, err)
			}
		})
	}
}
