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
	"github.com/deliciousbuding/metapi-go/internal/channellimit"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/store"
)

func streamPolicyFrame(event, data string) string {
	if event == "" {
		return "data: " + data + "\n\n"
	}
	return "event: " + event + "\ndata: " + data + "\n\n"
}

func TestDirectRequiredStreamNativeAggregation(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, tc := range []struct {
			name, path, body, response, expected string
			bit                                  int
			endpoint                             func(string) store.DirectEndpoints
		}{
			{"chat", "/v1/chat/completions", `{"model":"client-alias","messages":[{"role":"user","content":"hello"}]}`,
				streamPolicyFrame("", `{"id":"c","object":"chat.completion.chunk","model":"provider-model","vendor":9007199254740993,"choices":[{"index":0,"delta":{"role":"assistant","content":"hel","reasoning_content":"think","tool_calls":[{"index":0,"id":"t","type":"function","function":{"name":"echo","arguments":"{\"n\":"}}]},"finish_reason":null}]}`) +
					streamPolicyFrame("", `{"id":"c","choices":[{"index":0,"delta":{"content":"lo","tool_calls":[{"index":0,"function":{"arguments":"9007199254740993}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`) + streamPolicyFrame("", "[DONE]"),
				`"arguments":"{\"n\":9007199254740993}"`, store.DirectProtocolChat,
				func(url string) store.DirectEndpoints {
					return store.DirectEndpoints{Chat: &store.DirectEndpoint{URL: url, Auth: store.DirectAuthBearer}}
				}},
			{"responses", "/v1/responses", `{"model":"client-alias","input":"hello"}`,
				streamPolicyFrame("response.function_call_arguments.delta", `{"type":"response.function_call_arguments.delta","output_index":0,"delta":"{\"n\":"}`) +
					streamPolicyFrame("response.function_call_arguments.delta", `{"type":"response.function_call_arguments.delta","output_index":0,"delta":"9007199254740993}"}`) +
					streamPolicyFrame("response.output_item.done", `{"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","call_id":"t","name":"echo","arguments":"{\"n\":9007199254740993}"}}`) +
					streamPolicyFrame("response.completed", `{"type":"response.completed","response":{"id":"r","status":"completed","model":"provider-model","vendor":9007199254740993,"usage":{"input_tokens":2,"output_tokens":3}}}`),
				`"arguments":"{\"n\":9007199254740993}"`, store.DirectProtocolResponses,
				func(url string) store.DirectEndpoints {
					return store.DirectEndpoints{Responses: &store.DirectEndpoint{URL: url, Auth: store.DirectAuthBearer}}
				}},
			{"messages", "/v1/messages", `{"model":"client-alias","max_tokens":30,"messages":[{"role":"user","content":"hello"}]}`,
				streamPolicyFrame("message_start", `{"type":"message_start","message":{"id":"m","type":"message","role":"assistant","model":"provider-model","content":[],"usage":{"input_tokens":2,"output_tokens":0},"vendor":9007199254740993}}`) +
					streamPolicyFrame("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}`) +
					streamPolicyFrame("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"reason"}}`) +
					streamPolicyFrame("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig"}}`) +
					streamPolicyFrame("content_block_stop", `{"type":"content_block_stop","index":0}`) +
					streamPolicyFrame("content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"t","name":"echo","input":{}}}`) +
					streamPolicyFrame("content_block_delta", `{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"n\":"}}`) +
					streamPolicyFrame("content_block_delta", `{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"9007199254740993}"}}`) +
					streamPolicyFrame("content_block_stop", `{"type":"content_block_stop","index":1}`) +
					streamPolicyFrame("message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":3}}`) +
					streamPolicyFrame("message_stop", `{"type":"message_stop"}`),
				`"signature":"sig"`, store.DirectProtocolMessages,
				func(url string) store.DirectEndpoints {
					return store.DirectEndpoints{Messages: &store.DirectEndpoint{URL: url, Auth: store.DirectAuthAPIKey}}
				}},
			{"gemini", "/v1beta/models/client-alias:generateContent", `{"contents":[{"role":"user","parts":[{"text":"hello"}]}]}`,
				streamPolicyFrame("", `{"candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"hel","thought":true}]}}]}`) +
					streamPolicyFrame("", `{"candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"lo","thought":true,"thoughtSignature":"sig"},{"functionCall":{"name":"echo","args":{"n":9007199254740993}}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":3,"totalTokenCount":5},"modelVersion":"provider-model","vendor":9007199254740993}`),
				`"thoughtSignature":"sig"`, store.DirectProtocolGemini,
				func(url string) store.DirectEndpoints {
					return store.DirectEndpoints{Gemini: &store.DirectEndpoint{URL: url + "/models", Auth: store.DirectAuthGoogle, ModelPath: true}}
				}},
			{"completions", "/v1/completions", `{"model":"client-alias","prompt":"hello"}`,
				streamPolicyFrame("", `{"id":"c","object":"text_completion","model":"provider-model","vendor":9007199254740993,"choices":[{"index":0,"text":"hel","finish_reason":null}]}`) +
					streamPolicyFrame("", `{"id":"c","choices":[{"index":0,"text":"lo","finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`) + streamPolicyFrame("", "[DONE]"),
				`"text":"hello"`, store.DirectProtocolCompletions,
				func(url string) store.DirectEndpoints {
					return store.DirectEndpoints{Completions: &store.DirectEndpoint{URL: url, Auth: store.DirectAuthBearer}}
				}},
		} {
			t.Run(dialect+"/"+tc.name, func(t *testing.T) {
				var calls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					var body map[string]json.RawMessage
					if json.NewDecoder(r.Body).Decode(&body) != nil {
						t.Error("invalid upstream body")
					}
					if tc.bit == store.DirectProtocolGemini {
						if len(body["stream"]) != 0 || !strings.HasSuffix(r.URL.Path, ":streamGenerateContent") || r.URL.Query().Get("alt") != "sse" {
							t.Error("Gemini stream must be selected in the URL")
						}
					} else if string(body["stream"]) != "true" {
						t.Errorf("upstream did not stream: %s", body["stream"])
					}
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = fmt.Fprint(w, tc.response)
				}))
				t.Cleanup(upstream.Close)
				db := installDirectMediaRuntime(t, dialect, tc.endpoint(upstream.URL), tc.bit, "")
				if _, err := db.Exec(`UPDATE upstream_channels SET stream_policy='require'`); err != nil {
					t.Fatal(err)
				}
				// Association predicates must still see the non-streaming caller.
				if _, err := db.Exec(`UPDATE upstream_group_items SET association_conditions='[{"priority":1,"protocolOrder":[` + fmt.Sprint(tc.bit) + `],"condition":{"conditions":[{"field":"stream","operator":"eq","value":false}]}}]'`); err != nil {
					t.Fatal(err)
				}
				routing.InvalidateCache()
				var logs []proxy.ProxyLogEntry
				getUpstreamConfig().LogProxy = func(_ context.Context, entry proxy.ProxyLogEntry) error { logs = append(logs, entry); return nil }
				out := httptest.NewRecorder()
				directMediaRouter().ServeHTTP(out, makeProxyReq(http.MethodPost, tc.path, tc.body))
				if out.Code != 200 || calls.Load() != 1 || !json.Valid(out.Body.Bytes()) || !strings.Contains(out.Body.String(), tc.expected) || !strings.Contains(out.Body.String(), "9007199254740993") || strings.Contains(out.Body.String(), "data:") {
					t.Fatalf("aggregate status=%d calls=%d body=%s", out.Code, calls.Load(), out.Body.String())
				}
				if strings.Contains(out.Header().Get("Content-Type"), "event-stream") {
					t.Fatal("nonstream caller received SSE")
				}
				if len(logs) != 1 || logs[0].Status != "success" || logs[0].TotalTokens == nil || *logs[0].TotalTokens != 5 {
					t.Fatalf("aggregate usage was not settled exactly once: logs=%+v", logs)
				}
			})
		}
	}
}

func TestDirectStreamPolicySelectsNativeBeforeRequired(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			var calls [2]atomic.Int32
			var upstreams [2]*httptest.Server
			for i := range upstreams {
				index := i
				upstreams[i] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls[index].Add(1)
					var body map[string]any
					_ = json.NewDecoder(r.Body).Decode(&body)
					if index == 0 {
						if body["stream"] != true {
							t.Error("required channel received nonstream body")
						}
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = fmt.Fprint(w, streamPolicyFrame("", `{"id":"c","model":"provider-model","choices":[{"index":0,"delta":{"role":"assistant","content":"required"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)+streamPolicyFrame("", "[DONE]"))
					} else {
						if body["stream"] == true {
							t.Error("forbid channel received stream")
						}
						w.Header().Set("Content-Type", "application/json")
						_, _ = fmt.Fprint(w, `{"id":"c","object":"chat.completion","model":"provider-model","choices":[{"index":0,"message":{"role":"assistant","content":"native"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
					}
				}))
				t.Cleanup(upstreams[i].Close)
			}
			db := installDirectMediaRuntime(t, dialect, store.DirectEndpoints{Chat: &store.DirectEndpoint{URL: upstreams[0].URL, Auth: store.DirectAuthBearer}}, store.DirectProtocolChat, "")
			payload := fmt.Sprintf(`{"version":"1.4","channels":[{"id":1,"type":"qiniu","name":"Required","base_url":%q,"credentials":{"apiKey":"synthetic-one"},"supported_models":["provider-model"],"policies":{"stream":"require"}},{"id":2,"type":"qiniu","name":"Native","base_url":%q,"credentials":{"apiKey":"synthetic-two"},"supported_models":["provider-model"],"policies":{"stream":"forbid"}}],"models":[{"id":1,"model_id":"client-alias","settings":{"associations":[{"type":"channel_model","priority":1,"channelModel":{"channelId":1,"modelId":"provider-model"}},{"type":"channel_model","priority":9,"channelModel":{"channelId":2,"modelId":"provider-model"}}]}}]}`, upstreams[0].URL, upstreams[1].URL)
			if _, err := backup.ImportAxonHubV14(db, []byte(payload), "media-fixture", false); err != nil {
				t.Fatal(err)
			}
			for _, stream := range []bool{false, true, false} {
				out := httptest.NewRecorder()
				HandleChatCompletions(out, makeProxyReq("POST", "/v1/chat/completions", fmt.Sprintf(`{"model":"client-alias","stream":%t,"messages":[{"role":"user","content":"hello"}]}`, stream)))
				if out.Code != 200 {
					t.Fatalf("status=%d body=%s", out.Code, out.Body.String())
				}
			}
			if calls[0].Load() != 1 || calls[1].Load() != 2 {
				t.Fatalf("wrong mixed pool selection: required=%d native=%d", calls[0].Load(), calls[1].Load())
			}
			// Native preference is a candidate policy; temporary RPM pressure
			// does not erase it. Admission rejects that candidate first, then
			// normal retry selection may use an independently authorized require.
			manager := channellimit.NewManager()
			limits := channellimit.Config{RPM: 1}
			getUpstreamConfig().ChannelLimits = manager
			router := getUpstreamConfig().Router.(*routing.TokenRouter)
			router.SetChannelLimits(manager)
			var nativeID int64
			if err := db.Get(&nativeID, `SELECT id FROM upstream_channels WHERE name='Native'`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(db.Rebind(`UPDATE upstream_channels SET rate_limit=? WHERE id=?`), store.DirectRateLimit{Config: limits}, nativeID); err != nil {
				t.Fatal(err)
			}
			if err := manager.TryRequest(t.Context(), nativeID, limits); err != nil {
				t.Fatal(err)
			}
			routing.InvalidateCache()
			nonstream := false
			policy := routing.DownstreamRoutingPolicy{RequestStream: &nonstream, AllowStreamAggregation: true, RequiredUpstreamProtocol: store.DirectProtocolChat}
			selected, err := router.SelectChannel(t.Context(), "client-alias", policy)
			if err != nil || selected == nil || selected.Direct.ChannelID != nativeID {
				t.Fatalf("load ranking bypassed native stream preference: %+v %v", selected, err)
			}
			pressure := httptest.NewRecorder()
			HandleChatCompletions(pressure, makeProxyReq("POST", "/v1/chat/completions", `{"model":"client-alias","messages":[{"role":"user","content":"hello"}]}`))
			if pressure.Code != 200 || calls[0].Load() != 2 || calls[1].Load() != 2 {
				t.Fatalf("RPM refusal did not reach authorized streaming fallback: %d %s", pressure.Code, pressure.Body.String())
			}
			if _, err := db.Exec(`UPDATE upstream_channels SET enabled=FALSE WHERE name='Native'`); err != nil {
				t.Fatal(err)
			}
			routing.InvalidateCache()
			out := httptest.NewRecorder()
			HandleChatCompletions(out, makeProxyReq("POST", "/v1/chat/completions", `{"model":"client-alias","messages":[{"role":"user","content":"hello"}]}`))
			if out.Code != 200 || calls[0].Load() != 3 || !strings.Contains(out.Body.String(), "required") {
				t.Fatalf("authorized fallback unavailable: %d %s", out.Code, out.Body.String())
			}
			before := calls[0].Load()
			req := makeProxyReq("POST", "/v1/chat/completions", `{"model":"client-alias","messages":[{"role":"user","content":"hello"}]}`)
			empty := []int64{}
			req = req.WithContext(auth.WithProxyAuth(req.Context(), &auth.ProxyAuthContext{Source: "global", Policy: auth.DownstreamRoutingPolicy{UserRouteIDs: &empty}}))
			HandleChatCompletions(httptest.NewRecorder(), req)
			if calls[0].Load() != before {
				t.Fatal("required fallback bypassed user route grant")
			}
		})
	}
}

func TestDirectRequiredStreamProtocolMatrix(t *testing.T) {
	for _, up := range directWireFixtures {
		for _, down := range directWireFixtures {
			t.Run(down.provider+"_from_"+up.provider, func(t *testing.T) {
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var body map[string]json.RawMessage
					_ = json.NewDecoder(r.Body).Decode(&body)
					if up.provider == "gemini" {
						if !strings.HasSuffix(r.URL.Path, ":streamGenerateContent") {
							t.Error("upstream Gemini mode")
						}
					} else if string(body["stream"]) != "true" {
						t.Error("upstream mode was not forced")
					}
					w.Header().Set("Content-Type", "text/event-stream")
					for _, frame := range directNativeSSE(up) {
						_, _ = fmt.Fprint(w, frame)
					}
				}))
				t.Cleanup(upstream.Close)
				var endpoints []map[string]any
				if up.provider != "gemini" {
					endpoints = []map[string]any{{"api_format": up.format, "path": "/native"}}
				}
				db := installAxonHubEndpointFixture(t, up.provider, upstream.URL, endpoints)
				if _, err := db.Exec(`UPDATE upstream_channels SET stream_policy='require'`); err != nil {
					t.Fatal(err)
				}
				routing.InvalidateCache()
				out := httptest.NewRecorder()
				down.handler(out, makeProxyReq("POST", down.path, down.request))
				if out.Code != 200 || !json.Valid(out.Body.Bytes()) || !strings.Contains(out.Body.String(), down.toolMarker) || !strings.Contains(out.Body.String(), "receipt-tool") || !strings.Contains(out.Body.String(), "receipt-text") {
					t.Fatalf("cross protocol aggregate: %d %s", out.Code, out.Body.String())
				}
			})
		}
	}
}
