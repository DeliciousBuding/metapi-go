package proxyhandler

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
)

const wireToolResponse = `{"id":"chat-1","object":"chat.completion","model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":null,"reasoning_content":"reason","tool_calls":[{"id":"call-1","type":"function","function":{"name":"echo","arguments":"{}"}}]},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":3,"total_tokens":13}}`
const wireMessageTool = `{"id":"msg-1","type":"message","role":"assistant","model":"test-model","content":[{"type":"tool_use","id":"call-1","name":"echo","input":{}}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":3}}`

func TestNativeToolTerminalReachesClientThroughDispatch(t *testing.T) {
	for _, tc := range []struct {
		name, path, request, response, want string
		handler                             http.HandlerFunc
	}{
		{"chat", "/v1/chat/completions", `{"model":"test-model","messages":[{"role":"user","content":"echo"}]}`, wireToolResponse, `"finish_reason":"tool_calls"`, HandleChatCompletions},
		{"messages", "/v1/messages", `{"model":"test-model","max_tokens":64,"messages":[{"role":"user","content":"echo"}]}`, wireMessageTool, `"stop_reason":"tool_use"`, HandleClaudeMessages},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("ETag", "original-body")
				_, _ = io.WriteString(w, tc.response)
			}))
			defer upstream.Close()
			SetUpstreamConfig(&UpstreamConfig{Router: &upstreamTestRouter{selected: routing.SelectedChannel{Channel: store.RouteChannel{ID: 42, Enabled: true}, Account: store.Account{ID: 7, Status: "active"}, Site: store.Site{ID: 3, URL: upstream.URL, Platform: "new-api", Status: "active"}, TokenValue: "fixture-token", ActualModel: "test-model"}}})
			defer SetUpstreamConfig(nil)
			rec := httptest.NewRecorder()
			tc.handler(rec, makeProxyReq("POST", tc.path, tc.request))
			if rec.Code != 200 || !strings.Contains(rec.Body.String(), tc.want) {
				t.Fatalf("HTTP %d: %s", rec.Code, rec.Body.String())
			}
			if rec.Header().Get("ETag") != "" {
				t.Fatal("forwarded a validator for different bytes")
			}
		})
	}
}
func TestNativeTerminalBufferedSkipsOtherRepresentations(t *testing.T) {
	for _, tc := range []struct{ name, path, ctype, disposition string }{
		{"download", "/v1/files/one/content", "application/json", ""},
		{"responses", "/v1/responses", "application/json", ""},
		{"attachment", "/v1/chat/completions", "application/json", "attachment; filename=data.json"},
		{"binary", "/v1/chat/completions", "application/octet-stream", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := &http.Response{Header: make(http.Header)}
			resp.Header.Set("Content-Type", tc.ctype)
			resp.Header.Set("Content-Disposition", tc.disposition)
			resp.Header.Set("ETag", "same")
			out := normalizeNativeTerminalResponse(resp, []byte(wireToolResponse), tc.path)
			if string(out) != wireToolResponse || resp.Header.Get("ETag") != "same" {
				t.Fatal("modified unrelated representation")
			}
		})
	}
}

type terminalChunkReader struct {
	data   []byte
	size   int
	end    error
	closed bool
}

func (r *terminalChunkReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, r.end
	}
	n := min(len(p), r.size, len(r.data))
	copy(p, r.data[:n])
	r.data = r.data[n:]
	return n, nil
}
func (r *terminalChunkReader) Close() error { r.closed = true; return nil }
func TestNativeTerminalSSEFramingAcrossEveryByte(t *testing.T) {
	input := ": comment\r\nevent: message\r\nid: one\r\nretry: 250\r\ndata: {\"id\":\"one\",\"object\":\"chat.completion.chunk\",\"choices\":[\r\ndata: {\"index\":0,\"delta\":{\"content\":\"你好\"},\"finish_reason\":\"\"}]}\r\n\r\ndata: [DONE]\r\n\r\n"
	for _, size := range []int{1, 2, 7, 4096} {
		source := &terminalChunkReader{data: []byte(input), size: size, end: io.EOF}
		reader := withNativeTerminalBody(source, "/v1/chat/completions")
		out, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(out, []byte(`"finish_reason":null`)) || !bytes.Contains(out, []byte(`你好`)) {
			t.Fatalf("bad normalized data: %s", out)
		}
		for _, part := range []string{": comment\r\n", "event: message\r\n", "id: one\r\n", "retry: 250\r\n", "\r\n\r\ndata: [DONE]\r\n\r\n"} {
			if !bytes.Contains(out, []byte(part)) {
				t.Fatalf("lost frame metadata %q: %s", part, out)
			}
		}
		if err := reader.Close(); err != nil || !source.closed {
			t.Fatal("underlying reader was not closed")
		}
	}
}
func TestNativeTerminalReaderPreservesPartialErrorAndOversize(t *testing.T) {
	upstreamErr := errors.New("interrupted")
	for _, tc := range []struct {
		name, data, path string
		end              error
	}{
		{"partial", "data: {\"object\":\"chat.completion.chunk\"", "/v1/chat/completions", io.EOF},
		{"read_error", "data: {\"object\":\"chat.completion.chunk\"", "/v1/chat/completions", upstreamErr},
		{"unrecognized", "data: {\"choices\":[{\"finish_reason\":\"\"}]}\n\n", "/v1/chat/completions", io.EOF},
		{"other_api", "data: {\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"\"}]}\n\n", "/v1/responses", io.EOF},
		{"oversized", "data: " + strings.Repeat("x", maxIncrementalSsePendingBytes+10) + "\n\n", "/v1/chat/completions", io.EOF},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := &terminalChunkReader{data: []byte(tc.data), size: 4096, end: tc.end}
			out, err := io.ReadAll(withNativeTerminalBody(source, tc.path))
			if !bytes.Equal(out, []byte(tc.data)) {
				t.Fatal("changed partial/unsupported stream or appended a false terminal")
			}
			if tc.end == io.EOF {
				if err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, tc.end) {
				t.Fatalf("lost source error: %v", err)
			}
		})
	}
}
func TestNativeTerminalStreamDispatchKeepsToolEndAndUsage(t *testing.T) {
	stream := `data: {"id":"x","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call-1","type":"function","function":{"name":"echo","arguments":"{}"}}]},"finish_reason":""}]}` + "\n\n" + `data: {"id":"x","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":4,"completion_tokens":8,"total_tokens":12}}` + "\n\ndata: [DONE]\n\n"
	resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(stream))}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	usage, end, verdict := handleStreamUpstream(rec, req, resp, 1)
	if end != streamEndedNormally || (verdict != nil && verdict.Failed) {
		t.Fatalf("relay failed: %v %v", end, verdict)
	}
	if usage.TotalTokens != 12 || !strings.Contains(rec.Body.String(), `"finish_reason":"tool_calls"`) || !strings.Contains(rec.Body.String(), `"finish_reason":null`) {
		t.Fatalf("wire/usage mismatch: %+v %s", usage, rec.Body.String())
	}
}

func TestNativeTerminalStreamDispatchPreservesAttachmentsAndPostDoneData(t *testing.T) {
	start := `data: {"id":"x","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call-1","type":"function","function":{"name":"echo","arguments":"{}"}}]},"finish_reason":null}]}` + "\n\n"
	terminal := `data: {"id":"x","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}` + "\n\n"
	for _, tc := range []struct{ name, disposition, stream string }{
		{"attachment", "attachment; filename=completion.sse", start + terminal + "data: [DONE]\n\n"},
		{"attachment_case", "Attachment; filename=completion.sse", start + terminal + "data: [DONE]\n\n"},
		{"after_done", "", start + "data: [DONE]\n\n" + terminal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(tc.stream))}
			resp.Header.Set("Content-Disposition", tc.disposition)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
			handleStreamUpstream(rec, req, resp, 1)
			if rec.Body.String() != tc.stream {
				t.Fatalf("changed protected stream bytes: %s", rec.Body.String())
			}
		})
	}
}
