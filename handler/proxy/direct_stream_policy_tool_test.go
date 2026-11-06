package proxyhandler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestDirectRequiredStreamToolRoundTripAndBrokenArguments(t *testing.T) {
	for _, broken := range []bool{false, true} {
		t.Run(fmt.Sprintf("broken=%t", broken), func(t *testing.T) {
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var request struct {
					Stream   bool
					Messages []map[string]json.RawMessage
				}
				if json.NewDecoder(r.Body).Decode(&request) != nil || !request.Stream {
					t.Error("invalid required stream request")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				if calls == 1 {
					_, _ = fmt.Fprint(w, streamPolicyFrame("", `{"id":"c","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call","type":"function","function":{"name":"echo","arguments":"{\"n\":"}}]},"finish_reason":null}]}`))
					tail := "9007199254740993}"
					if broken {
						tail = "9007199254740993"
					}
					_, _ = fmt.Fprint(w, streamPolicyFrame("", fmt.Sprintf(`{"id":"c","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":%q}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`, tail))+streamPolicyFrame("", "[DONE]"))
					return
				}
				if len(request.Messages) != 3 || string(request.Messages[2]["tool_call_id"]) != `"call"` || !strings.Contains(string(request.Messages[1]["tool_calls"]), "9007199254740993") {
					t.Error("tool continuation identity/arguments were not preserved")
				}
				_, _ = fmt.Fprint(w, streamPolicyFrame("", `{"id":"c2","choices":[{"index":0,"delta":{"role":"assistant","content":"tool-roundtrip-complete"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`)+streamPolicyFrame("", "[DONE]"))
			}))
			t.Cleanup(upstream.Close)
			db := installDirectMediaRuntime(t, store.DialectSQLite, store.DirectEndpoints{Chat: &store.DirectEndpoint{URL: upstream.URL, Auth: store.DirectAuthBearer}}, store.DirectProtocolChat, "")
			if _, err := db.Exec(`UPDATE upstream_channels SET stream_policy='require'`); err != nil {
				t.Fatal(err)
			}
			routing.InvalidateCache()
			out := httptest.NewRecorder()
			HandleChatCompletions(out, makeProxyReq("POST", "/v1/chat/completions", `{"model":"client-alias","messages":[{"role":"user","content":"hello"}]}`))
			if broken {
				if out.Code < 400 || strings.Contains(out.Body.String(), `"tool_calls"`) || calls != 1 {
					t.Fatalf("broken tool succeeded: %d %s", out.Code, out.Body.String())
				}
				return
			}
			var response struct {
				Choices []struct{ Message map[string]json.RawMessage }
			}
			if out.Code != 200 || json.Unmarshal(out.Body.Bytes(), &response) != nil || len(response.Choices) != 1 {
				t.Fatalf("tool response invalid: %d %s", out.Code, out.Body.String())
			}
			body, _ := json.Marshal(map[string]any{"model": "client-alias", "messages": []any{
				map[string]any{"role": "user", "content": "hello"},
				response.Choices[0].Message,
				map[string]any{"role": "tool", "tool_call_id": "call", "content": `{"n":9007199254740993}`},
			}})
			out = httptest.NewRecorder()
			HandleChatCompletions(out, makeProxyReq("POST", "/v1/chat/completions", string(body)))
			if out.Code != 200 || calls != 2 || !strings.Contains(out.Body.String(), "tool-roundtrip-complete") {
				t.Fatalf("tool continuation failed: %d %s", out.Code, out.Body.String())
			}
		})
	}
}

func TestDirectRequiredStreamKeepsProviderStreamFilters(t *testing.T) {
	for _, profile := range []string{"bailian", "openrouter", "cerebras", "nanogpt"} {
		t.Run(profile, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				switch profile {
				case "bailian":
					for _, frame := range [][]byte{
						bailianTestTool(0, `{"n":9007199254740993}`, "call"),
						bailianTestTool(0, "{}", ""),
						bailianTestChunk(`[{"index":0,"delta":{"role":"assistant"},"finish_reason":"tool_calls"}]`),
						bailianTestFrame(`{"choices":[],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`),
						bailianTestFrame("[DONE]"),
					} {
						_, _ = w.Write(frame)
					}
				case "nanogpt":
					_, _ = w.Write(nanoGPTHTTPFrames("before\n<Read>{\"n\":9007199254740993}</Read>\nafter"))
				default:
					_, _ = fmt.Fprint(w, streamPolicyFrame("", `{"id":"c","model":"provider-model","choices":[{"index":0,"delta":{"role":"assistant","content":"done","tool_calls":[{"index":0,"id":"call","type":"function","function":{"name":"Read","arguments":"{\"n\":9007199254740993}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)+streamPolicyFrame("", "[DONE]"))
				}
			}))
			t.Cleanup(upstream.Close)
			db := installDirectMediaRuntime(t, store.DialectSQLite, store.DirectEndpoints{Chat: &store.DirectEndpoint{URL: upstream.URL, Auth: store.DirectAuthBearer, Profile: profile}}, store.DirectProtocolChat, "")
			if _, err := db.Exec(`UPDATE upstream_channels SET stream_policy='require'`); err != nil {
				t.Fatal(err)
			}
			routing.InvalidateCache()
			out := httptest.NewRecorder()
			HandleChatCompletions(out, makeProxyReq("POST", "/v1/chat/completions", `{"model":"client-alias","messages":[{"role":"user","content":"hello"}],"tools":[{"type":"function","function":{"name":"Read","parameters":{"type":"object"}}},{"type":"function","function":{"name":"ExistingCase","parameters":{"type":"object"}}}]}`))
			if out.Code != 200 || !json.Valid(out.Body.Bytes()) || !strings.Contains(out.Body.String(), "9007199254740993") {
				t.Fatalf("provider stream filter lost: %d %s", out.Code, out.Body.String())
			}
			if profile == "nanogpt" && strings.Contains(out.Body.String(), "<Read>") {
				t.Fatal("NanoGPT XML was not normalized")
			}
		})
	}
}
