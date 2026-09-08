package proxyhandler

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

type shortBridgeReads struct {
	io.ReadCloser
	size int
}

func (r shortBridgeReads) Read(p []byte) (int, error) {
	return r.ReadCloser.Read(p[:min(len(p), r.size)])
}

func TestMessagesChatBridgeFramesAndOriginalUsage(t *testing.T) {
	stream := "data: " + `{"id":"chat-1","object":"chat.completion.chunk","model":"model","choices":[{"index":0,"delta":{"role":"assistant","content":"receipt"},"finish_reason":null}]}` + "\r\n\r\n" +
		"data: " + `{"id":"chat-1","object":"chat.completion.chunk","model":"model","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}` + "\n\n" +
		"data: " + `{"id":"chat-1","object":"chat.completion.chunk","model":"model","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":3,"total_tokens":13}}` + "\n\ndata: [DONE]\n\n"
	for _, size := range []int{1, 7, 4096} {
		b := newMessagesChatBody(shortBridgeReads{io.NopCloser(strings.NewReader(stream)), size}, "model", 1<<20)
		out, err := io.ReadAll(b)
		if err != nil {
			t.Fatalf("chunk size %d: %v", size, err)
		}
		if !strings.Contains(string(out), "event: message_stop") || !strings.Contains(string(out), "receipt") || strings.Contains(string(out), "chat.completion") {
			t.Fatalf("invalid Messages wire for chunk size %d: %s", size, out)
		}
		usage := b.original.Result().Usage
		if !usage.Found || usage.Source != usageSourceUpstream || usage.TotalTokens != 13 {
			t.Fatalf("chunk size %d: original usage=%+v", size, usage)
		}
	}
}

func TestMessagesChatBridgeRawByteLimitCountsDiscardedFrames(t *testing.T) {
	b := newMessagesChatBody(io.NopCloser(strings.NewReader(strings.Repeat(": ping\n\n", 100))), "model", 32)
	if _, err := io.ReadAll(b); err != errMessagesChatStreamLimit {
		t.Fatalf("raw input limit was bypassed: %v", err)
	}
	if b.readBytes != 32 {
		t.Fatalf("read bytes=%d want 32", b.readBytes)
	}
}

func TestMessagesChatBridgeDoesNotCompleteMalformedOrTruncatedInput(t *testing.T) {
	for _, wire := range []string{
		"data: not-json\n\ndata: [DONE]\n\n",
		"data: " + `{"id":"chat-1","object":"chat.completion.chunk","model":"model","choices":[{"index":0,"delta":{"content":"unfinished"},"finish_reason":null}]}` + "\n\n",
		strings.Repeat("x", maxIncrementalSsePendingBytes+1),
	} {
		b := newMessagesChatBody(io.NopCloser(strings.NewReader(wire)), "model", 2<<20)
		out, err := io.ReadAll(b)
		if err == nil {
			t.Fatal("malformed/truncated stream accepted")
		}
		if strings.Contains(string(out), "event: message_stop") {
			t.Fatalf("malformed stream invented a successful terminal: %s", out)
		}
	}
}

func TestMessagesProxyStreamErrorIsNativeAndTerminal(t *testing.T) {
	rec := httptest.NewRecorder()
	writeSSEStreamError(rec, rec, "/v1/messages", "upstream stream interrupted", "upstream_error")
	body := rec.Body.String()
	if !strings.HasPrefix(body, "event: error\ndata: ") || !strings.Contains(body, `"type":"api_error"`) || strings.Contains(body, "[DONE]") || strings.Contains(body, "message_stop") {
		t.Fatalf("invalid Messages failure wire: %s", body)
	}
}
