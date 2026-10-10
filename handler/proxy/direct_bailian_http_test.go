package proxyhandler

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/transform/anthropic/messages"
)

func TestDirectBailianHTTPFixture(t *testing.T) {
	for _, encoding := range []string{"identity", "gzip"} {
		t.Run(encoding, func(t *testing.T) {
			release := make(chan struct{})
			var once sync.Once
			releaseUpstream := func() { once.Do(func() { close(release) }) }
			defer releaseUpstream()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
					return
				}
				if r.URL.Path != "/compatible-mode/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer fixture-only" || r.Header.Get("Content-Type") != "application/json" {
					t.Error("HTTP endpoint or headers changed")
				}
				if !bytes.Contains(body, []byte(`"enable_thinking":false`)) || bytes.Contains(body, []byte(`"reasoning_effort"`)) || bytes.Count(body, []byte(`"role":"assistant"`)) != 1 || !bytes.Contains(body, []byte(`9007199254740993`)) {
					t.Errorf("upstream received an unnormalized request: %s", body)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				var writer io.Writer = w
				flush := func() { w.(http.Flusher).Flush() }
				if encoding == "gzip" {
					w.Header().Set("Content-Encoding", "gzip")
					compressed := gzip.NewWriter(w)
					defer compressed.Close()
					writer = compressed
					flush = func() { _ = compressed.Flush(); w.(http.Flusher).Flush() }
				}
				_, _ = writer.Write(bailianTestChunk(`[{"index":0,"delta":{"role":"assistant","content":"first token"}}]`))
				flush()
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
				for _, frame := range [][]byte{
					bailianTestTool(0, `{"n":9007199254740993}`, "call-http"),
					bailianTestChunk(`[{"index":0,"delta":{"content":"deferred text","reasoning_content":"thinking","reasoning_signature":"signature"}}]`),
					bailianTestTool(0, "{}", ""),
					bailianTestChunk(`[{"index":0,"delta":{},"finish_reason":"tool_calls"}]`),
					bailianTestFrame(`{"choices":[],"usage":{"prompt_tokens":11,"completion_tokens":23,"prompt_tokens_details":{"cached_tokens":3},"completion_tokens_details":{"reasoning_tokens":7}}}`),
					bailianTestFrame(`[DONE]`),
				} {
					_, _ = writer.Write(frame)
					flush()
				}
			}))
			defer server.Close()
			body, err := prepareDirectBailianRequest([]byte(`{"model":"qwen-test","stream":true,"reasoning_effort":"none","seed":9007199254740993,"messages":[{"role":"assistant","tool_calls":[{"id":"a","function":{"arguments":"{}"}}]},{"role":"assistant","tool_calls":[{"id":"b","function":{"arguments":"{}"}}]}]}`))
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			req, _ := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/compatible-mode/v1/chat/completions", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer fixture-only")
			req.Header.Set("Accept-Encoding", encoding)
			resp, err := server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			decoded := proxy.WrapUpstreamStreamBody(resp.Header, resp.Body)
			if !decoded.Readable {
				t.Fatal("fixture encoding is unreadable")
			}
			source := resp.Body
			if decoded.Reader != nil {
				source = decoded.Reader
			}
			reader := newProtocolBridgeBody(source, newDirectBailianStream(), 1<<20)
			defer reader.Close()
			first := make([]byte, 4096)
			n, err := reader.Read(first)
			if err != nil || !bytes.Contains(first[:n], []byte("first token")) {
				t.Fatalf("first token was buffered until upstream finish: %s, %v", first[:n], err)
			}
			releaseUpstream()
			rest, err := io.ReadAll(reader)
			if err != nil {
				t.Fatal(err)
			}
			got := append(first[:n], rest...)
			for _, expected := range []string{`"arguments":""`, "deferred text", "9007199254740993", "thinking", "signature", `"finish_reason":"tool_calls"`, "[DONE]"} {
				if !bytes.Contains(got, []byte(expected)) {
					t.Fatalf("HTTP normalization lost %s: %s", expected, got)
				}
			}
			if bytes.Count(got, []byte("deferred text")) != 1 || bytes.Count(got, []byte(`"usage"`)) != 1 {
				t.Fatalf("text or usage duplicated: %s", got)
			}
			if bytes.Index(got, []byte("deferred text")) > bytes.Index(got, []byte(`"finish_reason":"tool_calls"`)) {
				t.Fatal("text was emitted after finish_reason")
			}
			usage := reader.original.Result().Usage
			if !usage.Found || usage.PromptTokens != 11 || usage.CompletionTokens != 23 || usage.CacheReadTokens != 3 {
				t.Fatalf("original upstream usage was lost: %+v", usage)
			}
		})
	}
}

func TestDirectBailianHTTPContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	reader := newProtocolBridgeBody(resp.Body, newDirectBailianStream(), 1024)
	defer reader.Close()
	result := make(chan error, 1)
	go func() { _, err := io.ReadAll(reader); result <- err }()
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("context cancellation became %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancel did not unblock HTTP stream read")
	}
}

func TestDirectBailianChainedClientStreams(t *testing.T) {
	var input []byte
	for _, frame := range [][]byte{
		bailianTestChunk(`[{"index":0,"delta":{"role":"assistant"}}]`),
		bailianTestTool(0, `{"filter":{}}`, "call-bridge"),
		bailianTestChunk(`[{"index":0,"delta":{"content":"tool explanation"}}]`),
		bailianTestTool(0, "{}", ""),
		bailianTestChunk(`[{"index":0,"delta":{},"finish_reason":"tool_calls"}]`),
		bailianTestFrame(`{"id":"bailian-id","object":"chat.completion.chunk","created":9007199254740993,"model":"qwen-test","choices":[],"usage":{"prompt_tokens":11,"completion_tokens":23}}`),
		bailianTestFrame(`[DONE]`),
	} {
		input = append(input, frame...)
	}
	for _, client := range []struct{ path, terminal string }{
		{"/v1/messages", "event: message_stop"},
		{"/v1/responses", "event: response.completed"},
		{"/v1beta/models/qwen-test:streamGenerateContent", `"finishReason":"STOP"`},
	} {
		t.Run(client.path, func(t *testing.T) {
			bridge := directResponseStream(client.path, "/v1/chat/completions", "qwen-test", messages.Options{})
			if bridge == nil {
				t.Fatal("client bridge missing")
			}
			stream := &chainedProtocolStream{first: newDirectBailianStream(), second: bridge}
			reader := newProtocolBridgeBody(io.NopCloser(bytes.NewReader(input)), stream, 1<<20)
			defer reader.Close()
			got, err := io.ReadAll(reader)
			if err != nil || !bytes.Contains(got, []byte(client.terminal)) || !bytes.Contains(got, []byte("tool explanation")) || !bytes.Contains(got, []byte("namespace__lookup")) {
				t.Fatalf("client stream conversion lost tool, text or terminal: %s, %v", got, err)
			}
			usage := reader.original.Result().Usage
			if !usage.Found || usage.PromptTokens != 11 || usage.CompletionTokens != 23 {
				t.Fatalf("client conversion changed actual upstream usage: %+v", usage)
			}
		})
	}
}

func TestDirectBailianReaderRetainsFailureAndByteLimit(t *testing.T) {
	input := append(bailianTestTool(0, "{}", "call"), bailianTestChunk(`[{"index":0,"delta":{"content":"pending"}}]`)...)
	input = append(input, bailianTestFrame(`{"choices":[],"usage":{"prompt_tokens":11,"completion_tokens":7}}`)...)
	failure := errors.New("fixture transport failed")
	for _, test := range []struct {
		name   string
		source io.ReadCloser
		limit  int64
		want   error
	}{
		{"transport", io.NopCloser(io.MultiReader(bytes.NewReader(input), bailianFailReader{failure})), 1 << 20, failure},
		{"truncated", io.NopCloser(bytes.NewReader(input)), 1 << 20, io.ErrUnexpectedEOF},
		{"input byte limit", io.NopCloser(bytes.NewReader(input)), int64(len(input) - 1), errMessagesChatStreamLimit},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader := newProtocolBridgeBody(test.source, newDirectBailianStream(), test.limit)
			defer reader.Close()
			got, err := io.ReadAll(reader)
			if !errors.Is(err, test.want) || bytes.Contains(got, []byte("[DONE]")) || bytes.Contains(got, []byte(`"content":"pending"`)) {
				t.Fatalf("failed stream fabricated output or lost error: %s, %v", got, err)
			}
			if test.name != "input byte limit" {
				usage := reader.original.Result().Usage
				if !usage.Found || usage.CompletionTokens != 7 || usage.PromptTokens != 11 {
					t.Fatalf("partial usage lost on failure: %+v", usage)
				}
			}
		})
	}
}

func TestDirectBailianReaderUsesExistingIdleGuard(t *testing.T) {
	source, writer := io.Pipe()
	defer writer.Close()
	idle := &streamIdleBody{ReadCloser: source}
	idle.guard = newStreamIdleGuard(30*time.Millisecond, idle.closeUnderlying)
	reader := newProtocolBridgeBody(idle, newDirectBailianStream(), 1<<20)
	defer reader.Close()
	result := make(chan error, 1)
	go func() { _, err := io.ReadAll(reader); result <- err }()
	select {
	case err := <-result:
		if err == nil || !idle.guard.fired.Load() {
			t.Fatalf("idle deadline became a successful finish: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("idle deadline did not unblock read")
	}
}

func TestDirectBailianReaderDoesNotReadEagerly(t *testing.T) {
	source := &bailianCountReader{ReadCloser: io.NopCloser(strings.NewReader(""))}
	reader := newProtocolBridgeBody(source, newDirectBailianStream(), 1024)
	defer reader.Close()
	if source.reads != 0 {
		t.Fatal("bridge read upstream before SSE headers")
	}
	if _, err := io.ReadAll(reader); !errors.Is(err, io.ErrUnexpectedEOF) || source.reads == 0 {
		t.Fatalf("empty stream did not retain read/EOF semantics: %v", err)
	}
}

type bailianFailReader struct{ err error }

func (r bailianFailReader) Read([]byte) (int, error) { return 0, r.err }

type bailianCountReader struct {
	io.ReadCloser
	reads int
}

func (r *bailianCountReader) Read(p []byte) (int, error) {
	r.reads++
	return r.ReadCloser.Read(p)
}
