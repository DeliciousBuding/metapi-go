package backup_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func domesticWireChatResponse(w http.ResponseWriter, provider string, stream bool) {
	message := map[string]any{"role": "assistant", "content": "domestic-receipt"}
	switch provider {
	case "openrouter", "cerebras":
		message["reasoning"] = "fallback-reasoning"
		message["reasoning_details"] = []any{map[string]any{"type": "reasoning.text", "text": "detailed-reasoning", "index": json.Number("9007199254740993")}}
	case "nanogpt":
		message["reasoning"] = "nano-reasoning"
	default:
		message["reasoning_content"] = "plain-reasoning"
	}
	if !stream {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "fixture-id", "object": "chat.completion", "model": "provider-model", "receipt": json.Number("9007199254740993"), "choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": "stop"}}, "usage": map[string]int{"prompt_tokens": 3, "completion_tokens": 2, "total_tokens": 5}})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	for _, frame := range []map[string]any{
		{"id": "fixture-id", "object": "chat.completion.chunk", "model": "provider-model", "receipt": json.Number("9007199254740993"), "choices": []any{map[string]any{"index": 0, "delta": message, "finish_reason": nil}}},
		{"id": "fixture-id", "object": "chat.completion.chunk", "model": "provider-model", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}}, "usage": map[string]int{"prompt_tokens": 3, "completion_tokens": 2, "total_tokens": 5}},
	} {
		raw, _ := json.Marshal(frame)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", raw)
	}
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
}

func TestAxonHubDomesticWireImportHTTPProfiles(t *testing.T) {
	for _, provider := range []string{"moonshot", "longcat", "openrouter", "cerebras", "nanogpt"} {
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
						path := "/api/v1/chat/completions"
						if !custom && (provider == "openrouter" || provider == "cerebras") {
							path = "/api/chat/completions"
						}
						if custom {
							path = "/api/fixed-chat"
						}
						if r.URL.Path != path || string(request["model"]) != `"provider-model"` || string(request["precision"]) != "9007199254740993" || r.Header.Get("Authorization") != "Bearer fixture-media-key" || r.Header.Get("x-api-key") != "" || r.Header.Get("x-goog-api-key") != "" {
							t.Errorf("provider wire changed: %s %s", r.URL.Path, body)
						}
						var messages []map[string]json.RawMessage
						if err := json.Unmarshal(request["messages"], &messages); err != nil || len(messages) != 2 {
							t.Errorf("messages malformed: %s", request["messages"])
							return
						}
						switch provider {
						case "moonshot":
							want := "json_schema"
							if !custom {
								want = "json_object"
							}
							var format struct {
								Type string `json:"type"`
							}
							_ = json.Unmarshal(request["response_format"], &format)
							if format.Type != want {
								t.Errorf("Moonshot profile scope changed: %s", request["response_format"])
							}
						case "longcat":
							if (!custom && !bytes.HasPrefix(messages[0]["content"], []byte("["))) || (custom && string(messages[0]["content"]) != `"hello"`) {
								t.Errorf("LongCat content profile scope changed: %s", request["messages"])
							}
						case "openrouter", "cerebras", "nanogpt":
							key := "reasoning_content"
							if !custom {
								key = "reasoning"
							}
							if string(messages[1][key]) != `"retained-reasoning"` {
								t.Errorf("reasoning key not selected: %s", request["messages"])
							}
						}
						if provider == "cerebras" && ((!custom && len(request["store"]) != 0) || (custom && string(request["store"]) != "true")) {
							t.Errorf("Cerebras store scope changed: %s", body)
						}
						domesticWireChatResponse(w, provider, stream)
					}))
					defer upstream.Close()
					format, path := "", ""
					if custom {
						format, path = "openai/chat_completions", "/fixed-chat"
					}
					router := installAxonHubMediaHTTP(t, provider, upstream.URL+"/api", format, path, "chat")
					body := fmt.Sprintf(`{"model":"client-alias","stream":%t,"store":true,"precision":9007199254740993,"response_format":{"type":"json_schema","json_schema":{"name":"fixture","schema":{"type":"object"}}},"messages":[{"role":"user","content":"hello"},{"role":"assistant","content":"history","reasoning_content":"retained-reasoning"}]}`, stream)
					out := requestAxonHubNativeHTTP(router, http.MethodPost, "/v1/chat/completions", body)
					response := out.Body.String()
					if out.Code != 200 || calls.Load() != 1 || !strings.Contains(response, "domestic-receipt") || !strings.Contains(response, "9007199254740993") || strings.Contains(response, `"error"`) {
						t.Fatalf("relay status=%d calls=%d body=%s", out.Code, calls.Load(), response)
					}
					if provider == "openrouter" || provider == "cerebras" || provider == "nanogpt" {
						if strings.Contains(response, `"reasoning_content"`) == custom {
							t.Fatalf("response reasoning profile scope changed: %s", response)
						}
					}
				})
			}
		}
	}
}

func TestAxonHubDomesticWireImportHTTPClientFormats(t *testing.T) {
	for _, provider := range []string{"moonshot", "longcat", "openrouter", "cerebras", "nanogpt"} {
		for _, client := range []struct{ name, path, body string }{
			{"responses", "/v1/responses", `{"model":"client-alias","input":"hello"}`},
			{"messages", "/v1/messages", `{"model":"client-alias","max_tokens":20,"messages":[{"role":"user","content":"hello"}]}`},
			{"gemini", "/v1beta/models/client-alias:generateContent", `{"contents":[{"role":"user","parts":[{"text":"hello"}]}]}`},
		} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/stream=%t", provider, client.name, stream), func(t *testing.T) {
					var calls atomic.Int32
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						var request map[string]json.RawMessage
						if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
							t.Error(err)
						}
						path := "/api/v1/chat/completions"
						if provider == "openrouter" || provider == "cerebras" {
							path = "/api/chat/completions"
						}
						if r.URL.Path != path || string(request["model"]) != `"provider-model"` || !bytes.Contains(request["messages"], []byte("hello")) || r.Header.Get("Authorization") != "Bearer fixture-media-key" || r.Header.Get("x-api-key") != "" || r.Header.Get("x-goog-api-key") != "" {
							t.Errorf("cross-protocol wire changed: %s %s", r.URL.Path, request)
						}
						domesticWireChatResponse(w, provider, stream)
					}))
					defer upstream.Close()
					router := installAxonHubMediaHTTP(t, provider, upstream.URL+"/api", "", "", "chat")
					path, body := client.path, client.body
					if stream {
						if client.name == "gemini" {
							path = strings.Replace(path, ":generateContent", ":streamGenerateContent", 1)
						} else {
							body = strings.TrimSuffix(body, "}") + `,"stream":true}`
						}
					}
					out := requestAxonHubNativeHTTP(router, http.MethodPost, path, body)
					if out.Code != 200 || calls.Load() != 1 || !strings.Contains(out.Body.String(), "domestic-receipt") || strings.Contains(out.Body.String(), `"error":{`) || strings.Contains(out.Body.String(), `"type":"error"`) {
						t.Fatalf("cross-protocol status=%d calls=%d body=%s", out.Code, calls.Load(), out.Body.String())
					}
				})
			}
		}
	}
}

func TestAxonHubDomesticWireImportHTTPNanoXML(t *testing.T) {
	for _, custom := range []bool{false, true} {
		t.Run(fmt.Sprintf("custom=%t", custom), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"id": "fixture-id", "model": "provider-model", "choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": "domestic-receipt\n" + `<Write file_path="fixture.txt">hello</Write>`, "reasoning": "nano-reasoning"}, "finish_reason": "stop"}}, "usage": map[string]int{"prompt_tokens": 3, "completion_tokens": 2, "total_tokens": 5}})
			}))
			defer upstream.Close()
			format, path := "", ""
			if custom {
				format, path = "openai/chat_completions", "/fixed-chat"
			}
			router := installAxonHubMediaHTTP(t, "nanogpt", upstream.URL, format, path, "chat")
			out := requestAxonHubNativeHTTP(router, http.MethodPost, "/v1/chat/completions", `{"model":"client-alias","messages":[{"role":"user","content":"hello"}],"tools":[{"type":"function","function":{"name":"Write","parameters":{"type":"object"}}}]}`)
			if out.Code != 200 || !strings.Contains(out.Body.String(), "domestic-receipt") || strings.Contains(out.Body.String(), `"tool_calls"`) == custom {
				t.Fatalf("Nano XML scope changed: status=%d body=%s", out.Code, out.Body.String())
			}
		})
	}
}

func TestAxonHubDomesticWireImportHTTPMediaIsolation(t *testing.T) {
	for _, provider := range []string{"openrouter", "nanogpt"} {
		t.Run(provider+"/audio", func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path != "/api/v1/audio/speech" || r.Header.Get("Authorization") != "Bearer fixture-media-key" {
					t.Errorf("audio inherited Chat URL/auth: %s", r.URL.Path)
				}
				w.Header().Set("Content-Type", "audio/mpeg")
				_, _ = w.Write([]byte{0, 1, 255, 2})
			}))
			defer upstream.Close()
			router := installAxonHubMediaHTTP(t, provider, upstream.URL+"/api", "", "", "chat")
			out := requestAxonHubNativeHTTP(router, http.MethodPost, "/v1/audio/speech", `{"model":"client-alias","input":"hello","voice":"alloy"}`)
			if out.Code != 200 || calls.Load() != 1 || !bytes.Equal(out.Body.Bytes(), []byte{0, 1, 255, 2}) {
				t.Fatalf("audio changed status=%d calls=%d body=%s", out.Code, calls.Load(), out.Body.String())
			}
		})
	}
	for _, provider := range []string{"openrouter", "nanogpt"} {
		t.Run(provider+"/custom-image", func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var request map[string]json.RawMessage
				_ = json.NewDecoder(r.Body).Decode(&request)
				if r.URL.Path != "/api/custom-image" || string(request["model"]) != `"provider-model"` || string(request["moderation"]) != `"low"` || string(request["response_format"]) != `"b64_json"` || string(request["user"]) != `"fixture-user"` {
					t.Errorf("custom image acquired wrapper: %s %s", r.URL.Path, request)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"created":9007199254740993,"data":[{"b64_json":"ZmFrZQ=="}]}`)
			}))
			defer upstream.Close()
			router := installAxonHubMediaHTTP(t, provider, upstream.URL+"/api", "openai/image_generation", "/custom-image", "image_generation")
			out := requestAxonHubNativeHTTP(router, http.MethodPost, "/v1/images/generations", `{"model":"client-alias","prompt":"hello","moderation":"low","response_format":"b64_json","user":"fixture-user"}`)
			if out.Code != 200 || calls.Load() != 1 || !strings.Contains(out.Body.String(), "9007199254740993") {
				t.Fatalf("image changed status=%d calls=%d body=%s", out.Code, calls.Load(), out.Body.String())
			}
		})
	}
}
