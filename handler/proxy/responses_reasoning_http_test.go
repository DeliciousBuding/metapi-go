package proxyhandler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/go-chi/chi/v5"
)

// Independent upstream wires, not constructed by the Responses transformers.
const responsesReasoningHTTPReply = `{"id":"reasoning-fixture","object":"chat.completion","model":"provider-model","choices":[{"index":0,"message":{"role":"assistant","reasoning_content":"Inspect fixture.\nRead it.","content":"Reading now.","tool_calls":[{"id":"fixture-call","type":"function","function":{"name":"read","arguments":"{\"offset\":9007199254740993}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":13,"completion_tokens":11,"total_tokens":24,"completion_tokens_details":{"reasoning_tokens":8}}}`
const responsesReasoningHTTPAnswer = `{"id":"reasoning-answer","object":"chat.completion","model":"provider-model","choices":[{"index":0,"message":{"role":"assistant","reasoning_content":"Fixture verified.","content":"receipt"},"finish_reason":"stop"}],"usage":{"prompt_tokens":4,"completion_tokens":2,"total_tokens":6}}`

func responsesReasoningHTTPFrames() []string {
	return []string{
		`data: {"id":"reasoning-fixture","object":"chat.completion.chunk","model":"provider-model","choices":[{"index":0,"delta":{"role":"assistant","content":"","reasoning_content":"Inspect fixture.\n"},"finish_reason":null}]}` + "\n\n",
		`data: {"id":"reasoning-fixture","object":"chat.completion.chunk","model":"provider-model","choices":[{"index":0,"delta":{"reasoning_content":"Read it.","content":"Reading now.","tool_calls":[{"index":0,"id":"fixture-call","type":"function","function":{"name":"read","arguments":"{\"offset\":"}}]},"finish_reason":null}]}` + "\n\n",
		`data: {"id":"reasoning-fixture","object":"chat.completion.chunk","model":"provider-model","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"9007199254740993}"}}]},"finish_reason":"tool_calls"}]}` + "\n\n",
		`data: {"id":"reasoning-fixture","object":"chat.completion.chunk","model":"provider-model","choices":[],"usage":{"prompt_tokens":13,"completion_tokens":11,"total_tokens":24,"completion_tokens_details":{"reasoning_tokens":8}}}` + "\n\n",
		"data: [DONE]\n\n",
	}
}

func responsesReasoningHTTPGateway(t *testing.T) *httptest.Server {
	t.Helper()
	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := auth.WithProxyAuth(r.Context(), &auth.ProxyAuthContext{Token: "fixture-client", Source: "global", Policy: auth.EmptyDownstreamRoutingPolicy})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	router.Route("/v1", RegisterProxyRoutes)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	server.Client().Timeout = 10 * time.Second
	return server
}

func responsesReasoningHTTPPost(t *testing.T, gateway *httptest.Server, path string, body []byte) (int, []byte) {
	t.Helper()
	response, err := gateway.Client().Post(gateway.URL+path, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	result, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, result
}

func responsesReasoningHTTPObject(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var value map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil || value == nil {
		t.Fatalf("invalid HTTP JSON: %s (%v)", raw, err)
	}
	return value
}

func responsesReasoningHTTPJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func responsesReasoningHTTPFinal(t *testing.T, raw []byte, stream bool) map[string]any {
	t.Helper()
	if !stream {
		return responsesReasoningHTTPObject(t, raw)
	}
	events, pending := ParseSseStream(string(raw))
	if pending != "" {
		t.Fatalf("incomplete downstream frame: %q", pending)
	}
	counts := map[string]int{}
	var final map[string]any
	var reasoning strings.Builder
	for _, event := range events {
		if IsSseErrorEvent(event) {
			t.Fatalf("Responses stream returned error: %s", event.Data)
		}
		value := responsesReasoningHTTPObject(t, []byte(event.Data))
		kind, _ := value["type"].(string)
		counts[kind]++
		if strings.HasPrefix(kind, "response.reasoning_summary_") {
			if value["output_index"] != json.Number("0") || value["summary_index"] != json.Number("0") || value["item_id"] != "rs_reasoning-fixture" {
				t.Fatalf("wrong reasoning SSE indices: %s", event.Data)
			}
		}
		if kind == "response.reasoning_summary_text.delta" {
			reasoning.WriteString(value["delta"].(string))
		}
		if kind == "response.reasoning_summary_text.done" && value["text"] != "Inspect fixture.\nRead it." {
			t.Fatal("reasoning done changed the original upstream text")
		}
		if kind == "response.completed" {
			final = value["response"].(map[string]any)
		}
	}
	for event, want := range map[string]int{"response.reasoning_summary_part.added": 1, "response.reasoning_summary_text.delta": 2, "response.reasoning_summary_text.done": 1, "response.reasoning_summary_part.done": 1, "response.completed": 1} {
		if counts[event] != want {
			t.Fatalf("%s count=%d want=%d", event, counts[event], want)
		}
	}
	if reasoning.String() != "Inspect fixture.\nRead it." || final == nil {
		t.Fatal("reasoning deltas or actual completed response missing")
	}
	return final
}

func TestDirectResponsesReasoningHTTPToolContinuation(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, explicit := range []bool{false, true} {
			t.Run(fmt.Sprintf("stream_%t_explicit_%t", stream, explicit), func(t *testing.T) {
				observed := make(chan []byte, 4)
				var calls atomic.Int64
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					n := calls.Add(1)
					body, _ := io.ReadAll(r.Body)
					observed <- body
					if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer fixture-upstream-key" {
						t.Error("imported Direct Chat endpoint/credential was not used")
					}
					if n == 1 && stream {
						w.Header().Set("Content-Type", "text/event-stream")
						for _, frame := range responsesReasoningHTTPFrames() {
							_, _ = io.WriteString(w, frame)
							w.(http.Flusher).Flush()
						}
						return
					}
					w.Header().Set("Content-Type", "application/json")
					if n == 1 {
						_, _ = io.WriteString(w, responsesReasoningHTTPReply)
					} else {
						_, _ = io.WriteString(w, responsesReasoningHTTPAnswer)
					}
				}))
				t.Cleanup(upstream.Close)
				installAxonHubEndpointFixture(t, "moonshot", upstream.URL, nil)
				logs := make(chan proxy.ProxyLogEntry, 4)
				getUpstreamConfig().LogProxy = func(_ context.Context, entry proxy.ProxyLogEntry) error { logs <- entry; return nil }
				gateway := responsesReasoningHTTPGateway(t)
				request := responsesReasoningHTTPObject(t, []byte(`{"model":"client-alias","input":"Read fixture","tools":[{"type":"function","name":"read","parameters":{"type":"object"}}]}`))
				request["stream"] = stream
				if explicit {
					request["reasoning"] = map[string]any{"effort": "high", "summary": "detailed"}
				}
				status, body := responsesReasoningHTTPPost(t, gateway, "/v1/responses", responsesReasoningHTTPJSON(t, request))
				if status != http.StatusOK || calls.Load() != 1 {
					t.Fatalf("first HTTP turn failed: status=%d calls=%d body=%s", status, calls.Load(), body)
				}
				firstRequest := responsesReasoningHTTPObject(t, <-observed)
				if firstRequest["model"] != "provider-model" || firstRequest["messages"] == nil || firstRequest["input"] != nil {
					t.Fatalf("Responses did not become a native Chat request: %v", firstRequest)
				}
				if explicit && (firstRequest["reasoning_effort"] != "high" || firstRequest["reasoning_summary"] != "detailed") || !explicit && firstRequest["reasoning_effort"] != nil {
					t.Fatal("explicit/default reasoning settings changed")
				}
				final := responsesReasoningHTTPFinal(t, body, stream)
				output, ok := final["output"].([]any)
				if !ok || len(output) != 3 || final["status"] != "completed" || final["model"] != "provider-model" {
					t.Fatalf("final reasoning/message/tool output or upstream model lost: %v", final)
				}
				reasoning := output[0].(map[string]any)
				if reasoning["type"] != "reasoning" || reasoning["summary"].([]any)[0].(map[string]any)["text"] != "Inspect fixture.\nRead it." {
					t.Fatal("upstream reasoning was lost or changed")
				}
				usage := final["usage"].(map[string]any)
				if usage["input_tokens"] != json.Number("13") || usage["output_tokens"] != json.Number("11") || usage["output_tokens_details"].(map[string]any)["reasoning_tokens"] != json.Number("8") {
					t.Fatalf("reasoning usage lost: %v", usage)
				}
				select {
				case log := <-logs:
					if log.Status != "success" || log.PromptTokens == nil || *log.PromptTokens != 13 || log.CompletionTokens == nil || *log.CompletionTokens != 11 || log.TotalTokens == nil || *log.TotalTokens != 24 {
						t.Fatalf("actual upstream usage log changed: %+v", log)
					}
				default:
					t.Fatal("successful HTTP response has no usage log")
				}
				history := append([]any{map[string]any{"role": "user", "content": "Read fixture"}}, output...)
				history = append(history, map[string]any{"type": "function_call_output", "call_id": "fixture-call", "output": "fixture contents"})
				request["input"], request["stream"] = history, false
				status, body = responsesReasoningHTTPPost(t, gateway, "/v1/responses", responsesReasoningHTTPJSON(t, request))
				if status != http.StatusOK || calls.Load() != 2 || !bytes.Contains(body, []byte("receipt")) {
					t.Fatalf("tool continuation failed: status=%d calls=%d body=%s", status, calls.Load(), body)
				}
				secondRequest := responsesReasoningHTTPObject(t, <-observed)
				messages := secondRequest["messages"].([]any)
				if len(messages) != 3 {
					t.Fatalf("tool turn was split: %v", messages)
				}
				assistant, result := messages[1].(map[string]any), messages[2].(map[string]any)
				if assistant["role"] != "assistant" || assistant["reasoning_content"] != "Inspect fixture.\nRead it." || assistant["content"] != "Reading now." || result["role"] != "tool" || result["tool_call_id"] != "fixture-call" || result["content"] != "fixture contents" {
					t.Fatalf("tool history semantics changed: %v", messages)
				}
				call := assistant["tool_calls"].([]any)[0].(map[string]any)
				if call["id"] != "fixture-call" || call["function"].(map[string]any)["arguments"] != `{"offset":9007199254740993}` {
					t.Fatal("tool identity/argument precision lost")
				}
			})
		}
	}
}

func TestDirectResponsesReasoningHTTPOpaqueRequestsFailClosed(t *testing.T) {
	for _, field := range []string{"encrypted_content", "signature"} {
		t.Run(field, func(t *testing.T) {
			var calls atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
			t.Cleanup(upstream.Close)
			installAxonHubEndpointFixture(t, "moonshot", upstream.URL, nil)
			gateway := responsesReasoningHTTPGateway(t)
			request := responsesReasoningHTTPObject(t, []byte(`{"model":"client-alias","input":[{"type":"reasoning","summary":[{"type":"summary_text","text":"plain fixture"}]},{"role":"user","content":"continue"}]}`))
			request["input"].([]any)[0].(map[string]any)[field] = "opaque-fixture"
			status, body := responsesReasoningHTTPPost(t, gateway, "/v1/responses", responsesReasoningHTTPJSON(t, request))
			if status != http.StatusBadRequest || calls.Load() != 0 || !bytes.Contains(body, []byte("invalid_request_error")) || bytes.Contains(body, []byte("opaque-fixture")) {
				t.Fatalf("opaque input forwarded, accepted or leaked: status=%d calls=%d body=%s", status, calls.Load(), body)
			}
		})
	}
}

func TestDirectResponsesReasoningHTTPSignedOutputFailsClosed(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if stream {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, `data: {"id":"signed","model":"provider-model","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"plain fixture","reasoning_signature":"opaque-fixture"},"finish_reason":"stop"}]}`+"\n\ndata: [DONE]\n\n")
				} else {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, strings.Replace(responsesReasoningHTTPReply, `"content":"Reading now."`, `"content":"Reading now.","reasoning_signature":"opaque-fixture"`, 1))
				}
			}))
			t.Cleanup(upstream.Close)
			installAxonHubEndpointFixture(t, "moonshot", upstream.URL, nil)
			gateway := responsesReasoningHTTPGateway(t)
			status, body := responsesReasoningHTTPPost(t, gateway, "/v1/responses", []byte(fmt.Sprintf(`{"model":"client-alias","input":"hi","stream":%t}`, stream)))
			if !stream && status != http.StatusBadGateway || stream && status != http.StatusOK && status != http.StatusBadGateway {
				t.Fatalf("wrong conversion failure status: %d %s", status, body)
			}
			if !bytes.Contains(body, []byte("upstream_error")) || bytes.Contains(body, []byte("response.completed")) || bytes.Contains(body, []byte("opaque-fixture")) {
				t.Fatalf("opaque upstream output succeeded, disappeared, or leaked: %s", body)
			}
		})
	}
}

func TestDirectResponsesReasoningHTTPGenericChatRemainsNative(t *testing.T) {
	observed := make(chan []byte, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		observed <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, strings.Replace(responsesReasoningHTTPReply, `"content":"Reading now."`, `"content":"Reading now.","reasoning_signature":"native-opaque"`, 1))
	}))
	t.Cleanup(upstream.Close)
	installAxonHubEndpointFixture(t, "moonshot", upstream.URL, nil)
	gateway := responsesReasoningHTTPGateway(t)
	request := []byte(`{"model":"client-alias","reasoning_effort":"high","reasoning_summary":"detailed","thinking":{"type":"enabled"},"vendor_option":{"shape":"retained"},"messages":[{"role":"assistant","content":"before","reasoning_content":"plain fixture","reasoning_signature":"native-history"},{"role":"user","content":"continue"}]}`)
	status, body := responsesReasoningHTTPPost(t, gateway, "/v1/chat/completions", request)
	if status != http.StatusOK {
		t.Fatalf("native Chat unexpectedly used strict Responses bridge: %d %s", status, body)
	}
	want := responsesReasoningHTTPObject(t, request)
	want["model"] = "provider-model"
	if got := responsesReasoningHTTPObject(t, <-observed); !reflect.DeepEqual(got, want) {
		t.Fatalf("generic Chat fields changed beyond model alias: got=%v want=%v", got, want)
	}
	expected := responsesReasoningHTTPObject(t, []byte(strings.Replace(responsesReasoningHTTPReply, `"content":"Reading now."`, `"content":"Reading now.","reasoning_signature":"native-opaque"`, 1)))
	if got := responsesReasoningHTTPObject(t, body); !reflect.DeepEqual(got, expected) {
		t.Fatalf("native Chat output changed: got=%v want=%v", got, expected)
	}
}
