package proxyhandler

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/transform/openai/responses"
)

type directProviderStream struct {
	ctx   context.Context
	next  protocolEventStream
	codex *responses.CodexResponseCollector
}

func (s *directProviderStream) TransformEvent(frame []byte) ([]byte, error) {
	if event := parseSseBlock(string(frame)); event != nil && event.Data != "" {
		raw := []byte(event.Data)
		if s.codex != nil {
			if event.Event == "error" {
				return nil, fmt.Errorf("Codex upstream error")
			}
			if err := s.codex.AddData(raw); err != nil {
				return nil, err
			}
		}
		restored := restoreDirectProviderResponse(s.ctx, raw)
		if wire := directProviderWireFromContext(s.ctx); wire != nil && s.next != nil && event.Data != "[DONE]" {
			var err error
			restored, err = projectDirectProviderResponse(wire, restored, true)
			if err != nil {
				return nil, err
			}
		}
		if !bytes.Equal(raw, restored) {
			frame = replaceSSEBlockData(frame, restored)
		}
	}
	if s.next != nil {
		return s.next.TransformEvent(frame)
	}
	return frame, nil
}

func projectDirectProviderResponse(wire *directProviderWire, body []byte, stream bool) ([]byte, error) {
	switch wire.Profile {
	case "cline":
		return projectDirectClineReasoning(body, stream)
	case "openrouter", "cerebras":
		return projectDirectRouterChatResponse(body, stream)
	default:
		return body, nil
	}
}
func (s *directProviderStream) Finish() ([]byte, error) {
	if s.codex != nil {
		if _, err := s.codex.Result(); err != nil {
			return nil, err
		}
	}
	if s.next != nil {
		return s.next.Finish()
	}
	return nil, nil
}

type directCodexCollectorStream struct {
	collector responses.CodexResponseCollector
}

func (s *directCodexCollectorStream) TransformEvent(frame []byte) ([]byte, error) {
	if event := parseSseBlock(string(frame)); event != nil && event.Data != "" {
		if event.Event == "error" {
			return nil, fmt.Errorf("Codex upstream error")
		}
		return nil, s.collector.AddData([]byte(event.Data))
	}
	return nil, nil
}
func (s *directCodexCollectorStream) Finish() ([]byte, error) { return s.collector.Result() }

// Reuse the bounded frame reader and idle guard used by streaming clients. The
// collector emits only the verified terminal JSON, never the source SSE text.
func collectDirectCodexResponse(resp *http.Response, started time.Time) ([]byte, ParsedUsage, error) {
	prepared := proxy.WrapUpstreamStreamBody(resp.Header, resp.Body)
	if prepared.Reader != nil {
		resp.Body = prepared.Reader
	}
	if !prepared.Readable {
		_ = resp.Body.Close()
		return nil, ParsedUsage{Source: usageSourceUnknown}, fmt.Errorf("cannot decode upstream stream")
	}
	idle := &streamIdleBody{ReadCloser: resp.Body}
	idle.guard = newStreamIdleGuard(streamIdleTimeout(), idle.closeUnderlying)
	collector := &directCodexCollectorStream{collector: responses.CodexResponseCollector{Limit: streamResponseByteLimit()}}
	reader := newProtocolBridgeBody(idle, collector, streamResponseByteLimit())
	defer reader.Close()
	var firstOutput *int64
	reader.original.onFirstOutput = func() { firstOutput = int64Ptr(time.Since(started).Milliseconds()) }
	body, err := proxy.ReadBufferedResponseBody(reader)
	usage := reader.original.Result().Usage
	usage.FirstOutputLatencyMs = firstOutput
	if idle.guard.fired.Load() {
		return nil, usage, fmt.Errorf("upstream stream idle timeout")
	}
	return body, usage, err
}
