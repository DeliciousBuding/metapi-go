package proxyhandler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestChatFamilyJSONRepairsUpstreamTextMediaType(t *testing.T) {
	cases := []struct {
		name, path, request, response string
		handle                        http.HandlerFunc
	}{
		{"chat tool call", "/v1/chat/completions", `{"model":"test-model","messages":[{"role":"user","content":"echo"}]}`, `{"id":"chat-1","object":"chat.completion","model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call-1","type":"function","function":{"name":"echo","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`, HandleChatCompletions},
		{"messages", "/v1/messages", `{"model":"test-model","max_tokens":64,"messages":[{"role":"user","content":"hello"}]}`, `{"id":"msg-1","type":"message","role":"assistant","model":"test-model","content":[{"type":"text","text":"hello"}],"stop_reason":"end_turn"}`, HandleClaudeMessages},
		{"responses", "/v1/responses", `{"model":"test-model","input":"hello"}`, `{"id":"resp-1","object":"response","model":"test-model","status":"completed","output":[{"id":"msg-1","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"hello"}]}]}`, func(w http.ResponseWriter, r *http.Request) { HandleResponses(w, r, "/v1/responses") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				_, _ = w.Write([]byte(tc.response))
			}))
			defer upstream.Close()
			SetUpstreamConfig(&UpstreamConfig{Router: &upstreamTestRouter{selected: routing.SelectedChannel{
				Channel: store.RouteChannel{ID: 42, Enabled: true}, Account: store.Account{ID: 7, Status: "active"},
				Site: store.Site{ID: 3, URL: upstream.URL, Platform: "new-api", Status: "active"}, TokenValue: "fixture-upstream-token", ActualModel: "test-model",
			}}})
			defer SetUpstreamConfig(nil)
			rec := httptest.NewRecorder()
			tc.handle(rec, makeProxyReq("POST", tc.path, tc.request))
			if rec.Code != http.StatusOK {
				t.Fatalf("HTTP %d: %s", rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Content-Type"); got != "application/json" {
				t.Fatalf("Content-Type=%q, want application/json", got)
			}
			if rec.Body.String() != tc.response {
				t.Fatalf("relay changed body bytes: %s", rec.Body.String())
			}
		})
	}
}

func TestJSONMediaTypeNormalizationPreservesOtherRepresentations(t *testing.T) {
	cases := []struct{ name, path, contentType, encoding, disposition, body, want string }{
		{"missing", "/v1/messages", "", "", "", `{"ok":true}`, "application/json"},
		{"already JSON", "/v1/chat/completions", "application/json; charset=utf-8", "", "", `{"ok":true}`, "application/json; charset=utf-8"},
		{"vendor JSON", "/v1/responses", "application/problem+json", "", "", `{"error":true}`, "application/problem+json"},
		{"non JSON", "/v1/chat/completions", "text/plain", "", "", "not json", "text/plain"},
		{"download path", "/v1/files/file-1/content", "text/plain", "", "", `{"data":1}`, "text/plain"},
		{"attachment", "/v1/responses", "text/plain", "", `attachment; filename="data.json"`, `{"data":1}`, "text/plain"},
		{"opaque encoding", "/v1/chat/completions", "text/plain", "br", "", `{"data":1}`, "text/plain"},
		{"binary type", "/v1/messages", "application/octet-stream", "", "", `{"data":1}`, "application/octet-stream"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := http.Header{}
			h.Set("Content-Type", tc.contentType)
			h.Set("Content-Encoding", tc.encoding)
			h.Set("Content-Disposition", tc.disposition)
			normalizeUpstreamJSONContentType(h, []byte(tc.body), tc.path)
			if got := h.Get("Content-Type"); got != tc.want {
				t.Fatalf("Content-Type=%q, want %q", got, tc.want)
			}
		})
	}
}
