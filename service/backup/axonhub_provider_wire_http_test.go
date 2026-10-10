package backup_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func providerWireResponse(w http.ResponseWriter, protocol, model string, stream, wrapped bool) {
	chat := fmt.Sprintf(`{"id":"fixture-id","object":"chat.completion","model":%q,"choices":[{"index":0,"message":{"role":"assistant","content":"provider-receipt","reasoning":"provider-reasoning"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`, model)
	responses := fmt.Sprintf(`{"id":"fixture-id","object":"response","status":"completed","model":%q,"output":[{"id":"fixture-message","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"provider-receipt"}]}],"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}`, model)
	messages := fmt.Sprintf(`{"id":"fixture-id","type":"message","role":"assistant","model":%q,"content":[{"type":"text","text":"provider-receipt"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":2}}`, model)
	if !stream {
		body := chat
		if protocol == "responses" {
			body = responses
		} else if protocol == "messages" {
			body = messages
		}
		if wrapped {
			body = `{"success":true,"data":` + body + `}`
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	frame := func(event, body string) {
		if wrapped && body != "[DONE]" {
			body = `{"success":true,"data":` + body + `}`
		}
		if event != "" {
			_, _ = fmt.Fprintf(w, "event: %s\n", event)
		}
		_, _ = fmt.Fprintf(w, "data: %s\n\n", body)
	}
	switch protocol {
	case "responses":
		frame("response.created", fmt.Sprintf(`{"type":"response.created","response":{"id":"fixture-id","object":"response","status":"in_progress","model":%q,"output":[]}}`, model))
		frame("response.output_item.added", `{"type":"response.output_item.added","output_index":0,"item":{"id":"fixture-message","type":"message","role":"assistant","status":"in_progress","content":[]}}`)
		frame("response.content_part.added", `{"type":"response.content_part.added","output_index":0,"item_id":"fixture-message","content_index":0,"part":{"type":"output_text","text":"","annotations":[]}}`)
		frame("response.output_text.delta", `{"type":"response.output_text.delta","output_index":0,"item_id":"fixture-message","content_index":0,"delta":"provider-receipt"}`)
		frame("response.output_text.done", `{"type":"response.output_text.done","output_index":0,"item_id":"fixture-message","content_index":0,"text":"provider-receipt"}`)
		frame("response.content_part.done", `{"type":"response.content_part.done","output_index":0,"item_id":"fixture-message","content_index":0,"part":{"type":"output_text","text":"provider-receipt","annotations":[]}}`)
		frame("response.output_item.done", `{"type":"response.output_item.done","output_index":0,"item":{"id":"fixture-message","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"provider-receipt","annotations":[]}]}}`)
		frame("response.completed", `{"type":"response.completed","response":`+responses+`}`)
	case "messages":
		frame("message_start", fmt.Sprintf(`{"type":"message_start","message":{"id":"fixture-id","type":"message","role":"assistant","model":%q,"content":[],"usage":{"input_tokens":3,"output_tokens":0}}}`, model))
		frame("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
		frame("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"provider-receipt"}}`)
		frame("content_block_stop", `{"type":"content_block_stop","index":0}`)
		frame("message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}`)
		frame("message_stop", `{"type":"message_stop"}`)
	default:
		frame("", fmt.Sprintf(`{"id":"fixture-id","object":"chat.completion.chunk","model":%q,"choices":[{"index":0,"delta":{"role":"assistant","content":"provider-receipt","reasoning":"provider-reasoning"},"finish_reason":null}]}`, model))
		frame("", fmt.Sprintf(`{"id":"fixture-id","object":"chat.completion.chunk","model":%q,"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`, model))
		frame("", "[DONE]")
	}
}

func TestAxonHubProviderWireImportHTTPBailianAndCline(t *testing.T) {
	for _, provider := range []string{"bailian", "cline"} {
		for _, custom := range []bool{false, true} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/custom=%t/stream=%t", provider, custom, stream), func(t *testing.T) {
					var calls atomic.Int32
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						body, _ := io.ReadAll(r.Body)
						var request map[string]json.RawMessage
						if err := json.Unmarshal(body, &request); err != nil {
							t.Error(err)
						}
						wantPath := "/api/v1/chat/completions"
						if custom {
							wantPath = "/api/custom-chat"
						}
						if r.URL.Path != wantPath || string(request["model"]) != `"provider-model"` || r.Header.Get("Authorization") != "Bearer fixture-media-key" || r.Header.Get("x-api-key") != "" {
							t.Errorf("provider path/auth/model changed: %s %s", r.URL.Path, body)
						}
						if provider == "bailian" {
							if !custom && (string(request["enable_thinking"]) != "false" || len(request["reasoning_effort"]) != 0 || strings.Count(string(request["messages"]), `"role":"assistant"`) != 1) {
								t.Errorf("Bailian profile was not applied: %s", body)
							}
							if custom && (len(request["enable_thinking"]) != 0 || string(request["reasoning_effort"]) != `"none"` || strings.Count(string(request["messages"]), `"role":"assistant"`) != 2) {
								t.Errorf("Bailian custom acquired default profile: %s", body)
							}
						} else if r.Header.Get("X-Client-Type") != "cline-cli" || !strings.Contains(string(body), `"content":[{"type":"text","text":""}]`) {
							t.Errorf("Cline request normalization missing: %s", body)
						}
						providerWireResponse(w, "chat", "provider-model", stream, provider == "cline")
					}))
					defer upstream.Close()
					format, customPath := "", ""
					if custom {
						format, customPath = "openai/chat_completions", "/custom-chat"
					}
					router := installAxonHubMediaHTTP(t, provider, upstream.URL+"/api", format, customPath, "chat")
					body := fmt.Sprintf(`{"model":"client-alias","stream":%t,"reasoning_effort":"none","messages":[{"role":"user","content":""},{"role":"assistant","tool_calls":[{"id":"a","type":"function","function":{"name":"echo","arguments":"{}"}}]},{"role":"assistant","tool_calls":[{"id":"b","type":"function","function":{"name":"echo","arguments":"{}"}}]}]}`, stream)
					out := requestAxonHubNativeHTTP(router, http.MethodPost, "/v1/chat/completions", body)
					if out.Code != 200 || calls.Load() != 1 || !strings.Contains(out.Body.String(), "provider-receipt") {
						t.Fatalf("relay status=%d calls=%d body=%s", out.Code, calls.Load(), out.Body.String())
					}
					if provider == "cline" && (!strings.Contains(out.Body.String(), "reasoning_content") || strings.Contains(out.Body.String(), `"success"`)) {
						t.Fatalf("Cline response normalization missing: %s", out.Body.String())
					}
				})
			}
		}
	}
}

func TestAxonHubProviderWireImportHTTPOpenCode(t *testing.T) {
	for _, tc := range []struct{ name, provider, model, format, logical, path, protocol string }{
		{"default Chat", "opencode_go", "other-model", "", "", "/go/chat/completions", "chat"},
		{"case sensitive Chat", "opencode_go", "GPT-fixture", "", "", "/go/chat/completions", "chat"},
		{"DeepSeek", "opencode_go", "deepseek-fixture", "", "", "/go/chat/completions", "chat"},
		{"GPT", "opencode_go", "gpt-fixture", "", "", "/go/responses", "responses"},
		{"Grok", "opencode_go", "grok-fixture", "", "", "/go/responses", "responses"},
		{"MiniMax", "opencode_go", "minimax-fixture", "", "", "/go/messages", "messages"},
		{"Qwen", "opencode_go", "qwen3-fixture", "", "", "/go/messages", "messages"},
		{"custom Chat stays fixed", "opencode_go", "gpt-fixture", "openai/chat_completions", "openai/chat_completions", "/go/custom", "chat"},
		{"custom Responses stays fixed", "opencode_go", "qwen3-fixture", "openai/responses", "openai/responses", "/go/custom", "responses"},
		{"custom Messages stays fixed", "opencode_go", "gpt-fixture", "anthropic/messages", "anthropic/messages", "/go/custom", "messages"},
		{"custom Responses does not replace internal", "opencode_go", "gpt-fixture", "openai/responses", "openai/chat_completions", "/go/responses", "responses"},
		{"legacy primary Messages", "opencode_go_anthropic", "gpt-fixture", "", "", "/go/messages", "messages"},
		{"legacy custom Chat", "opencode_go_anthropic", "gpt-fixture", "openai/chat_completions", "openai/chat_completions", "/go/custom", "chat"},
	} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, stream), func(t *testing.T) {
				var calls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					body, _ := io.ReadAll(r.Body)
					var request map[string]json.RawMessage
					if err := json.Unmarshal(body, &request); err != nil {
						t.Error(err)
					}
					var model string
					_ = json.Unmarshal(request["model"], &model)
					if r.URL.Path != tc.path || model != tc.model || r.Header.Get("X-Opencode-Session") != "fixture-session" {
						t.Errorf("OpenCode destination/model/session changed: %s %s", r.URL.Path, body)
					}
					for _, header := range []string{"Authorization", "x-api-key", "x-goog-api-key"} {
						want := ""
						if header == "Authorization" && tc.protocol != "messages" {
							want = "Bearer fixture-media-key"
						} else if header == "x-api-key" && tc.protocol == "messages" {
							want = "fixture-media-key"
						}
						if r.Header.Get(header) != want {
							t.Errorf("OpenCode selected incorrect %s", header)
						}
					}
					if tc.protocol == "responses" && len(request["input"]) == 0 {
						t.Error("OpenCode Responses body was not converted")
					}
					if tc.protocol == "messages" && len(request["max_tokens"]) == 0 {
						t.Error("OpenCode Messages body was not converted")
					}
					providerWireResponse(w, tc.protocol, tc.model, stream, false)
				}))
				defer upstream.Close()
				custom := ""
				if tc.format != "" {
					custom = "/custom"
				}
				router := installAxonHubHTTPFixture(t, tc.provider, upstream.URL+"/go#", tc.format, custom, "chat", func(ch map[string]any) {
					ch["supported_models"] = []string{tc.model}
					settings := map[string]any{"modelMappings": []any{map[string]any{"from": "provider-model", "to": tc.model}}}
					if tc.logical != "" {
						settings["modelProtocols"] = []any{map[string]any{"model": "client-alias", "apiFormats": []string{tc.logical}}}
					}
					ch["settings"] = settings
				})
				withSession := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					r.Header.Set("X-Session-Id", " fixture-session ")
					router.ServeHTTP(w, r)
				})
				out := requestAxonHubNativeHTTP(withSession, http.MethodPost, "/v1/chat/completions", fmt.Sprintf(`{"model":"client-alias","stream":%t,"messages":[{"role":"user","content":"hello"}]}`, stream))
				if out.Code != 200 || calls.Load() != 1 || !strings.Contains(out.Body.String(), "provider-receipt") {
					t.Fatalf("OpenCode status=%d calls=%d body=%s", out.Code, calls.Load(), out.Body.String())
				}
			})
		}
	}
}

func TestAxonHubProviderWireImportHTTPBailianToolFragments(t *testing.T) {
	for _, custom := range []bool{false, true} {
		t.Run(fmt.Sprintf("custom=%t", custom), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				for _, frame := range []string{
					`{"id":"fixture-id","object":"chat.completion.chunk","model":"provider-model","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call-fixture","type":"function","function":{"name":"echo","arguments":"{\"ok\":true}"}}]}}]}`,
					`{"id":"fixture-id","object":"chat.completion.chunk","model":"provider-model","choices":[{"index":0,"delta":{"content":"deferred-receipt"}}]}`,
					`{"id":"fixture-id","object":"chat.completion.chunk","model":"provider-model","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{}"}}]}}]}`,
					`{"id":"fixture-id","object":"chat.completion.chunk","model":"provider-model","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
					`{"id":"fixture-id","object":"chat.completion.chunk","model":"provider-model","choices":[],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`,
					`[DONE]`,
				} {
					_, _ = fmt.Fprintf(w, "data: %s\n\n", frame)
				}
			}))
			defer upstream.Close()
			format, path := "", ""
			if custom {
				format, path = "openai/chat_completions", "/custom"
			}
			router := installAxonHubMediaHTTP(t, "bailian", upstream.URL, format, path, "chat")
			out := requestAxonHubNativeHTTP(router, http.MethodPost, "/v1/chat/completions", `{"model":"client-alias","stream":true,"messages":[{"role":"user","content":"hello"}]}`)
			body := out.Body.String()
			if out.Code != 200 || strings.Count(body, "deferred-receipt") != 1 || !strings.Contains(body, "[DONE]") || strings.Contains(body, `"arguments":"{}"`) != custom {
				t.Fatalf("Bailian stream profile scope changed: status=%d body=%s", out.Code, body)
			}
		})
	}
}

func TestAxonHubProviderWireImportHTTPClineFailureEnvelope(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if stream {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, "data: {\"success\":false,\"error\":{\"message\":\"fixture-rejected\"}}\n\ndata: [DONE]\n\n")
				} else {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"success":false,"error":{"message":"fixture-rejected"}}`)
				}
			}))
			defer upstream.Close()
			router := installAxonHubMediaHTTP(t, "cline", upstream.URL, "", "", "chat")
			out := requestAxonHubNativeHTTP(router, http.MethodPost, "/v1/chat/completions", fmt.Sprintf(`{"model":"client-alias","stream":%t,"messages":[{"role":"user","content":"hello"}]}`, stream))
			if (!stream && out.Code < 400) || calls.Load() != 1 || !strings.Contains(out.Body.String(), `"error"`) || strings.Contains(out.Body.String(), `"finish_reason":"stop"`) {
				t.Fatalf("Cline failure was reported as success: status=%d calls=%d body=%s", out.Code, calls.Load(), out.Body.String())
			}
		})
	}
}

func TestAxonHubProviderWireImportHTTPRawURL(t *testing.T) {
	for _, provider := range []string{"bailian", "cline", "opencode_go"} {
		t.Run(provider, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.EscapedPath() != "/raw/" {
					t.Errorf("raw endpoint path changed: %s", r.URL.EscapedPath())
				}
				providerWireResponse(w, "chat", "provider-model", false, provider == "cline")
			}))
			defer upstream.Close()
			format, path := "", ""
			if provider == "opencode_go" {
				format, path = "openai/chat_completions", "/ignored"
			}
			router := installAxonHubMediaHTTP(t, provider, upstream.URL+"/raw/##", format, path, "chat")
			out := requestAxonHubNativeHTTP(router, http.MethodPost, "/v1/chat/completions", `{"model":"client-alias","messages":[{"role":"user","content":"hello"}]}`)
			if out.Code != 200 || calls.Load() != 1 || !strings.Contains(out.Body.String(), "provider-receipt") {
				t.Fatalf("raw URL status=%d calls=%d body=%s", out.Code, calls.Load(), out.Body.String())
			}
		})
	}
}
