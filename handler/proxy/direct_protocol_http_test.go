package proxyhandler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/proxy"
)

type directWireFixture struct {
	provider, format, path, request, response, toolMarker string
	handler                                               http.HandlerFunc
}

// Fixtures describe native wires independently of the runtime converters. Every
// request traverses the imported DB graph, router and two real HTTP transports.
var directWireFixtures = []directWireFixture{
	{"openai", "openai/chat_completions", "/v1/chat/completions", `{"model":"client-alias","messages":[{"role":"user","content":"receipt-history"}],"tools":[{"type":"function","function":{"name":"echo","parameters":{"type":"object"}}}]}`, `{"id":"id-fixture","object":"chat.completion","model":"provider-model","choices":[{"index":0,"message":{"role":"assistant","content":"receipt-text","tool_calls":[{"id":"call-fixture","type":"function","function":{"name":"echo","arguments":"{\"value\":\"receipt-tool\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`, "tool_calls", HandleChatCompletions},
	{"openai_responses", "openai/responses", "/v1/responses", `{"model":"client-alias","input":[{"role":"user","content":"receipt-history"}],"tools":[{"type":"function","name":"echo","parameters":{"type":"object"}}]}`, `{"id":"id-fixture","object":"response","model":"provider-model","status":"completed","output":[{"id":"msg-fixture","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"receipt-text","annotations":[]}]},{"id":"fc-fixture","type":"function_call","status":"completed","call_id":"call-fixture","name":"echo","arguments":"{\"value\":\"receipt-tool\"}"}],"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}}`, "function_call", func(w http.ResponseWriter, r *http.Request) { HandleResponses(w, r, "") }},
	{"anthropic", "anthropic/messages", "/v1/messages", `{"model":"client-alias","max_tokens":64,"messages":[{"role":"user","content":"receipt-history"}],"tools":[{"name":"echo","input_schema":{"type":"object"}}]}`, `{"id":"id-fixture","type":"message","model":"provider-model","role":"assistant","content":[{"type":"text","text":"receipt-text"},{"type":"tool_use","id":"call-fixture","name":"echo","input":{"value":"receipt-tool"}}],"stop_reason":"tool_use","stop_sequence":null,"usage":{"input_tokens":5,"output_tokens":2}}`, "tool_use", HandleClaudeMessages},
	{"gemini", "gemini/contents", "/v1beta/models/client-alias:generateContent", `{"contents":[{"role":"user","parts":[{"text":"receipt-history"}]}],"tools":[{"functionDeclarations":[{"name":"echo","parameters":{"type":"object"}}]}]}`, `{"responseId":"id-fixture","modelVersion":"provider-model","candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"receipt-text"},{"functionCall":{"id":"call-fixture","name":"echo","args":{"value":"receipt-tool"}}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":2,"totalTokenCount":7}}`, "functionCall", HandleGeminiGenerateContent},
}

func TestDirectImportedProtocolsHTTPMatrix(t *testing.T) {
	for _, up := range directWireFixtures {
		for _, down := range directWireFixtures {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s_to_%s_stream_%t", down.provider, up.provider, stream), func(t *testing.T) {
					upstreamCalls := 0
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						upstreamCalls++
						body, _ := io.ReadAll(r.Body)
						if !strings.Contains(string(body), "receipt-history") || !strings.Contains(string(body), "echo") {
							t.Errorf("request semantics lost: %s", body)
						}
						authHeader := "Authorization"
						expected := "Bearer fixture-upstream-key"
						switch up.provider {
						case "anthropic":
							authHeader = "x-api-key"
							expected = "fixture-upstream-key"
						case "gemini":
							authHeader = "x-goog-api-key"
							expected = "fixture-upstream-key"
						}
						if r.Header.Get(authHeader) != expected {
							t.Errorf("wrong %s authentication", up.provider)
						}
						for _, header := range []string{"Authorization", "x-api-key", "x-goog-api-key"} {
							if header != authHeader && r.Header.Get(header) != "" {
								t.Errorf("foreign credential header leaked: %s", header)
							}
						}
						if up.provider == "gemini" {
							action := "generateContent"
							if stream {
								action = "streamGenerateContent"
							}
							if r.URL.Path != "/v1beta/models/provider-model:"+action {
								t.Errorf("native Gemini model/action path: %s", r.URL.String())
							}
							if stream && r.URL.Query().Get("alt") != "sse" {
								t.Errorf("missing native SSE query")
							}
						}
						if stream {
							w.Header().Set("Content-Type", "text/event-stream")
							for _, frame := range directNativeSSE(up) {
								_, _ = io.WriteString(w, frame)
								w.(http.Flusher).Flush()
							}
						} else {
							w.Header().Set("Content-Type", "application/json")
							_, _ = io.WriteString(w, up.response)
						}
					}))
					defer upstream.Close()
					var endpoints []map[string]any
					if up.provider != "gemini" {
						endpoints = []map[string]any{{"api_format": up.format, "path": "/native-wire"}}
					}
					installAxonHubEndpointFixture(t, up.provider, upstream.URL, endpoints)
					logs := make(chan proxy.ProxyLogEntry, 2)
					getUpstreamConfig().LogProxy = func(_ context.Context, entry proxy.ProxyLogEntry) error { logs <- entry; return nil }
					downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						r = r.WithContext(auth.WithProxyAuth(r.Context(), &auth.ProxyAuthContext{Token: "fixture-client", Source: "global", Policy: auth.EmptyDownstreamRoutingPolicy}))
						down.handler(w, r)
					}))
					defer downstream.Close()
					body := down.request
					path := down.path
					if stream {
						if down.provider == "gemini" {
							path = strings.Replace(path, ":generateContent", ":streamGenerateContent", 1)
						} else {
							var payload map[string]any
							_ = json.Unmarshal([]byte(body), &payload)
							payload["stream"] = true
							encoded, _ := json.Marshal(payload)
							body = string(encoded)
						}
					}
					resp, err := http.Post(downstream.URL+path, "application/json", strings.NewReader(body))
					if err != nil {
						t.Fatal(err)
					}
					output, err := io.ReadAll(resp.Body)
					_ = resp.Body.Close()
					if err != nil {
						t.Fatal(err)
					}
					if resp.StatusCode != 200 || upstreamCalls != 1 {
						t.Fatalf("status=%d upstreamCalls=%d body=%s", resp.StatusCode, upstreamCalls, output)
					}
					for _, marker := range []string{"receipt-text", "receipt-tool", down.toolMarker} {
						if !strings.Contains(string(output), marker) {
							t.Errorf("missing %s: %s", marker, output)
						}
					}
					if strings.Contains(string(output), `"type":"error"`) {
						t.Errorf("stream reported conversion failure: %s", output)
					}
					select {
					case entry := <-logs:
						if entry.PromptTokens == nil || *entry.PromptTokens != 5 || entry.CompletionTokens == nil || *entry.CompletionTokens != 2 || entry.TotalTokens == nil || *entry.TotalTokens != 7 {
							t.Errorf("wrong original usage: %+v", entry)
						}
					default:
						t.Error("missing actual proxy usage log")
					}
				})
			}
		}
	}
}

func directNativeSSE(up directWireFixture) []string {
	frame := func(event, data string) string {
		if event != "" {
			return "event: " + event + "\ndata: " + data + "\n\n"
		}
		return "data: " + data + "\n\n"
	}
	switch up.provider {
	case "openai":
		return []string{frame("", `{"id":"id-fixture","object":"chat.completion.chunk","model":"provider-model","choices":[{"index":0,"delta":{"role":"assistant","content":"receipt-text"},"finish_reason":null}]}`), frame("", `{"id":"id-fixture","object":"chat.completion.chunk","model":"provider-model","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call-fixture","type":"function","function":{"name":"echo","arguments":"{\"value\":\"receipt-tool\"}"}}]},"finish_reason":"tool_calls"}]}`), frame("", `{"id":"id-fixture","object":"chat.completion.chunk","model":"provider-model","choices":[],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`), frame("", "[DONE]")}
	case "gemini":
		return []string{frame("", up.response)}
	case "anthropic":
		return []string{
			frame("message_start", `{"type":"message_start","message":{"id":"id-fixture","type":"message","role":"assistant","model":"provider-model","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":5,"output_tokens":0}}}`),
			frame("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`), frame("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"receipt-text"}}`), frame("content_block_stop", `{"type":"content_block_stop","index":0}`),
			frame("content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"call-fixture","name":"echo","input":{}}}`), frame("content_block_delta", `{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"value\":\"receipt-tool\"}"}}`), frame("content_block_stop", `{"type":"content_block_stop","index":1}`), frame("message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":2}}`), frame("message_stop", `{"type":"message_stop"}`)}
	case "openai_responses":
		return []string{
			frame("response.created", `{"type":"response.created","response":{"id":"id-fixture","object":"response","model":"provider-model","status":"in_progress","output":[]}}`),
			frame("response.output_item.added", `{"type":"response.output_item.added","output_index":0,"item":{"id":"msg-fixture","type":"message","role":"assistant","status":"in_progress","content":[]}}`),
			frame("response.content_part.added", `{"type":"response.content_part.added","output_index":0,"item_id":"msg-fixture","content_index":0,"part":{"type":"output_text","text":"","annotations":[]}}`),
			frame("response.output_text.delta", `{"type":"response.output_text.delta","output_index":0,"item_id":"msg-fixture","content_index":0,"delta":"receipt-text"}`),
			frame("response.output_text.done", `{"type":"response.output_text.done","output_index":0,"item_id":"msg-fixture","content_index":0,"text":"receipt-text"}`),
			frame("response.content_part.done", `{"type":"response.content_part.done","output_index":0,"item_id":"msg-fixture","content_index":0,"part":{"type":"output_text","text":"receipt-text","annotations":[]}}`),
			frame("response.output_item.done", `{"type":"response.output_item.done","output_index":0,"item":{"id":"msg-fixture","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"receipt-text","annotations":[]}]}}`),
			frame("response.output_item.added", `{"type":"response.output_item.added","output_index":1,"item":{"id":"fc-fixture","type":"function_call","status":"in_progress","call_id":"call-fixture","name":"echo","arguments":""}}`),
			frame("response.function_call_arguments.delta", `{"type":"response.function_call_arguments.delta","output_index":1,"item_id":"fc-fixture","delta":"{\"value\":\"receipt-tool\"}"}`),
			frame("response.function_call_arguments.done", `{"type":"response.function_call_arguments.done","output_index":1,"item_id":"fc-fixture","arguments":"{\"value\":\"receipt-tool\"}"}`),
			frame("response.output_item.done", `{"type":"response.output_item.done","output_index":1,"item":{"id":"fc-fixture","type":"function_call","status":"completed","call_id":"call-fixture","name":"echo","arguments":"{\"value\":\"receipt-tool\"}"}}`),
			frame("response.completed", `{"type":"response.completed","response":`+up.response+`}`),
		}
	}
	return nil
}

func TestDirectProtocolOverridesReachOnlySelectedEndpoint(t *testing.T) {
	for _, tc := range []struct {
		name    string
		formats []string
		want    string
	}{
		{"forced Responses", []string{"openai/responses"}, "/responses-only"},
		{"client match before first item", []string{"openai/responses", "openai/chat_completions"}, "/chat-only"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := []string{}
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls = append(calls, r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				fixture := directWireFixtures[0]
				if r.URL.Path == "/responses-only" {
					fixture = directWireFixtures[1]
				}
				_, _ = io.WriteString(w, fixture.response)
			}))
			defer up.Close()
			installAxonHubEndpointFixture(t, "openai", up.URL, []map[string]any{{"api_format": "openai/chat_completions", "path": "/chat-only"}, {"api_format": "openai/responses", "path": "/responses-only"}}, map[string]any{"modelProtocols": []any{map[string]any{"model": "client-alias", "apiFormats": tc.formats}}})
			out := httptest.NewRecorder()
			HandleChatCompletions(out, makeProxyReq("POST", "/v1/chat/completions", directWireFixtures[0].request))
			if out.Code != 200 || len(calls) != 1 || calls[0] != tc.want {
				t.Fatalf("status=%d calls=%v body=%s", out.Code, calls, out.Body.String())
			}
		})
	}
}

func TestDirectConvertedStreamsKeepTruncationAndActualUsage(t *testing.T) {
	for _, up := range directWireFixtures {
		t.Run(up.provider, func(t *testing.T) {
			frames := directNativeSSE(up)
			if up.provider == "gemini" {
				frames[0] = strings.Replace(frames[0], `,"finishReason":"STOP"`, "", 1)
			} else {
				frames = frames[:len(frames)-1]
			}
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				for _, frame := range frames {
					_, _ = io.WriteString(w, frame)
				}
			}))
			defer upstream.Close()
			installAxonHubEndpointFixture(t, up.provider, upstream.URL, nil)
			logs := make(chan proxy.ProxyLogEntry, 2)
			getUpstreamConfig().LogProxy = func(_ context.Context, entry proxy.ProxyLogEntry) error { logs <- entry; return nil }
			down := directWireFixtures[0]
			if up.provider == "openai" {
				down = directWireFixtures[1]
			}
			var request map[string]any
			_ = json.Unmarshal([]byte(down.request), &request)
			request["stream"] = true
			raw, _ := json.Marshal(request)
			out := httptest.NewRecorder()
			down.handler(out, makeProxyReq("POST", down.path, string(raw)))
			if !strings.Contains(out.Body.String(), "upstream_error") {
				t.Fatalf("truncated stream did not report failure: %s", out.Body.String())
			}
			if strings.Contains(out.Body.String(), `"type":"response.completed"`) || strings.Contains(out.Body.String(), `"finish_reason":"tool_calls"`) {
				t.Fatalf("truncated stream received successful terminal: %s", out.Body.String())
			}
			select {
			case entry := <-logs:
				if entry.Status != "failed" {
					t.Fatalf("truncated log status=%s", entry.Status)
				}
				if up.provider != "openai_responses" && (entry.TotalTokens == nil || *entry.TotalTokens != 7) {
					t.Fatalf("lost actual usage: %+v", entry)
				}
			default:
				t.Fatal("missing failure log")
			}
		})
	}
}

func TestDirectGeminiNativePreservesSignedOpaqueFields(t *testing.T) {
	raw := `{"contents":[{"role":"model","parts":[{"text":"signed thought","thought":true,"thoughtSignature":"real-signature"},{"functionCall":{"name":"echo","args":{"id":9007199254740993}},"thoughtSignature":"tool-signature"}]}],"labels":{"fixture":"retained"},"cachedContent":"cached/native"}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if string(body) != raw {
			t.Errorf("native Gemini body changed: %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, directWireFixtures[3].response)
	}))
	defer upstream.Close()
	installAxonHubEndpointFixture(t, "gemini", upstream.URL, nil)
	out := httptest.NewRecorder()
	HandleGeminiGenerateContent(out, makeProxyReq("POST", "/v1beta/models/client-alias:generateContent", raw))
	if out.Code != 200 {
		t.Fatalf("native request failed: %d %s", out.Code, out.Body.String())
	}
}
