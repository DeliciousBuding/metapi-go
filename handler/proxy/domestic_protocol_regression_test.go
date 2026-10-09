package proxyhandler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

const auditDomesticAnswer = `{"id":"audit-answer","object":"chat.completion","model":"provider-model","choices":[{"index":0,"message":{"role":"assistant","content":"receipt"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`
const auditDomesticThinkingTool = `{"id":"audit-answer","object":"chat.completion","model":"provider-model","choices":[{"index":0,"message":{"role":"assistant","content":null,"reasoning_content":"private planning","tool_calls":[{"id":"audit-call","type":"function","function":{"name":"echo","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`

func TestDomesticProviderThinkingWire(t *testing.T) {
	for _, provider := range []string{"deepseek", "zai", "zhipu", "xiaomi"} {
		t.Run(provider, func(t *testing.T) {
			var observed map[string]json.RawMessage
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewDecoder(r.Body).Decode(&observed)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, auditDomesticAnswer)
			}))
			defer upstream.Close()
			installAxonHubEndpointFixture(t, provider, upstream.URL, nil)
			out := httptest.NewRecorder()
			HandleChatCompletions(out, makeProxyReq("POST", "/v1/chat/completions", `{"model":"client-alias","messages":[{"role":"user","content":"hi"}],"reasoning_effort":"none"}`))
			if out.Code != 200 {
				t.Fatalf("relay failed: %d %s", out.Code, out.Body.String())
			}
			if string(observed["thinking"]) != `{"type":"disabled"}` {
				t.Errorf("source provider contract requires thinking.disabled; got %s", observed["thinking"])
			}
			if provider == "deepseek" && observed["reasoning_effort"] != nil {
				t.Errorf("DeepSeek source removes unsupported reasoning_effort none; got %s", observed["reasoning_effort"])
			}
		})
	}
}
