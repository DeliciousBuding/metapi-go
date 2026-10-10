package proxyhandler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/proxy"
)

// Each request traverses an imported SQLite graph, a native public handler,
// PrepareCtx, dispatcher, real HTTP upstream and the actual usage logger.
func TestNativeCompletionHTTPRequiresTerminalAndKeepsUsage(t *testing.T) {
	for _, up := range directWireFixtures[:3] {
		for _, mapped := range []bool{false, true} {
			for _, complete := range []bool{false, true} {
				name := up.provider + map[bool]string{false: "/native", true: "/public-model"}[mapped] + map[bool]string{false: "/truncated", true: "/complete"}[complete]
				t.Run(name, func(t *testing.T) {
					frames := directNativeSSE(up)
					if up.provider == "openai_responses" {
						// Include observed usage before the terminal: a truncated
						// response still consumed these tokens at the upstream.
						frames[0] = strings.Replace(frames[0], `"output":[]`, `"output":[],"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}`, 1)
					}
					if !complete {
						frames = frames[:len(frames)-1]
					}
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Content-Type", "text/event-stream")
						for _, frame := range frames {
							_, _ = io.WriteString(w, frame)
							w.(http.Flusher).Flush()
						}
					}))
					defer server.Close()
					installAxonHubEndpointFixture(t, up.provider, server.URL, nil)
					logs := make(chan proxy.ProxyLogEntry, 1)
					getUpstreamConfig().LogProxy = func(_ context.Context, entry proxy.ProxyLogEntry) error { logs <- entry; return nil }
					request := strings.TrimSuffix(up.request, "}") + `,"stream":true}`
					r := makeProxyReq(http.MethodPost, up.path, request)
					if mapped {
						r = r.WithContext(context.WithValue(r.Context(), downstreamResponseModelKey{}, "public-model"))
					}
					out := httptest.NewRecorder()
					up.handler(out, r)
					var entry proxy.ProxyLogEntry
					select {
					case entry = <-logs:
					default:
						t.Fatal("missing actual usage log")
					}
					hasError := strings.Contains(out.Body.String(), "upstream_error") || strings.Contains(out.Body.String(), "api_error")
					if complete {
						if entry.Status != "success" || hasError {
							t.Fatalf("complete native stream failed: status=%s body=%s", entry.Status, out.Body.String())
						}
					} else if entry.Status != "failed" || !hasError || entry.HTTPStatus != http.StatusBadGateway {
						t.Fatalf("missing terminal falsely succeeded: status=%s httpStatus=%d body=%s", entry.Status, entry.HTTPStatus, out.Body.String())
					}
					if entry.PromptTokens == nil || *entry.PromptTokens != 5 || entry.CompletionTokens == nil || *entry.CompletionTokens != 2 || entry.TotalTokens == nil || *entry.TotalTokens != 7 {
						t.Errorf("lost observed usage: %+v", entry)
					}
					if mapped && !strings.Contains(out.Body.String(), "public-model") {
						t.Errorf("public model mapping lost: %s", out.Body.String())
					}
					for _, marker := range []string{"receipt-text", "echo", "call-fixture", "receipt-tool"} {
						if !strings.Contains(out.Body.String(), marker) {
							t.Errorf("native tool bytes lost %s: %s", marker, out.Body.String())
						}
					}
				})
			}
		}
	}
}

func TestNativeCompletionReaderProtocolScopeAndErrors(t *testing.T) {
	for _, tc := range []struct {
		name, path, terminal string
		wantError            bool
	}{
		{"chat", "/v1/chat/completions", "data: [DONE]\n\n", false},
		{"messages", "/v1/messages", "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n", false},
		{"responses", "/v1/responses", "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n", false},
		{"foreign-chat-marker", "/v1/messages", "data: [DONE]\n\n", true},
		{"foreign-responses-marker", "/v1/chat/completions", "data: {\"type\":\"response.completed\"}\n\n", true},
		{"incomplete-response", "/v1/responses", "data: {\"type\":\"response.incomplete\",\"response\":{\"status\":\"incomplete\"}}\n\n", true},
		{"mismatched-response-status", "/v1/responses", "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"incomplete\"}}\n\n", true},
		{"gemini-no-done-marker", "/v1beta/models/native:streamGenerateContent", "data: {\"candidates\":[{\"finishReason\":\"STOP\"}]}\n\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, size := range []int{1, 4096} {
				source := &terminalChunkReader{data: []byte(tc.terminal), size: size, end: io.EOF}
				out, err := io.ReadAll(withNativeTerminalBody(source, tc.path))
				if (err != nil) != tc.wantError || string(out) != tc.terminal {
					t.Fatalf("terminal validation changed bytes or outcome: error=%v body=%s", err, out)
				}
			}
		})
	}
	for _, end := range []error{context.Canceled, context.DeadlineExceeded, io.ErrUnexpectedEOF} {
		source := &terminalChunkReader{data: []byte("data: [DONE]\n\n"), size: 2, end: end}
		_, err := io.ReadAll(withNativeTerminalBody(source, "/v1/chat/completions"))
		if !errors.Is(err, end) {
			t.Fatalf("terminal marker hid underlying interruption %v: %v", end, err)
		}
	}
}
