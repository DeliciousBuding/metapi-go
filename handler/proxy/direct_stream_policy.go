package proxyhandler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/internal/upstreamadapter"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/deliciousbuding/metapi-go/transform/streamaggregate"
)

func directStreamAggregateProtocol(path string) string {
	if strings.HasSuffix(strings.TrimRight(path, "/"), "/count_tokens") {
		return ""
	}
	switch proxy.DirectProtocolForPath(path) {
	case store.DirectProtocolChat, store.DirectProtocolOllama:
		return "chat"
	case store.DirectProtocolCompletions:
		return "completions"
	case store.DirectProtocolResponses:
		return "responses"
	case store.DirectProtocolMessages:
		return "messages"
	case store.DirectProtocolGemini:
		return "gemini"
	}
	return ""
}

func requireDirectStreamBody(body []byte, path string) ([]byte, error) {
	var object map[string]json.RawMessage
	if json.Unmarshal(body, &object) != nil || object == nil {
		return nil, fmt.Errorf("invalid text request for required streaming")
	}
	if proxy.DirectProtocolForPath(path) == store.DirectProtocolGemini {
		delete(object, "stream")
	} else {
		object["stream"] = json.RawMessage("true")
	}
	return json.Marshal(object)
}

type directPolicyCollectorStream struct {
	ctx       context.Context
	collector *streamaggregate.Collector
}

func (s *directPolicyCollectorStream) TransformEvent(frame []byte) ([]byte, error) {
	event := parseSseBlock(string(frame))
	if event == nil {
		return nil, nil
	}
	raw := []byte(event.Data)
	if event.Data != "" && event.Data != "[DONE]" {
		if wire := directProviderWireFromContext(s.ctx); wire != nil {
			if adapter, ok := upstreamadapter.Lookup(wire.Profile); ok {
				var err error
				raw, err = adapter.NormalizeResponse(raw, true)
				if err != nil {
					return nil, err
				}
			}
		}
	}
	return nil, s.collector.Add(event.Event, raw)
}
func (s *directPolicyCollectorStream) Finish() ([]byte, error) { return s.collector.Result() }

func collectDirectPolicyResponse(ctx context.Context, resp *http.Response, started time.Time, path string) ([]byte, ParsedUsage, error) {
	collector, err := streamaggregate.New(directStreamAggregateProtocol(path), streamResponseByteLimit())
	if err != nil {
		_ = resp.Body.Close()
		return nil, ParsedUsage{Source: usageSourceUnknown}, err
	}
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
	var stream protocolEventStream = &directPolicyCollectorStream{ctx: ctx, collector: collector}
	if wire := directProviderWireFromContext(ctx); wire != nil {
		var filter protocolEventStream
		switch wire.Profile {
		case "nanogpt":
			filter = newDirectNanoGPTStream(wire.NanoGPTTools)
		case "openrouter", "cerebras":
			filter = newDirectRouterChatStream()
		case "bailian":
			filter = newDirectBailianStream()
		}
		if filter != nil {
			stream = &chainedProtocolStream{first: filter, second: stream}
		}
	}
	reader := newProtocolBridgeBody(idle, stream, streamResponseByteLimit())
	defer reader.Close()
	var firstOutput *int64
	reader.original.onFirstOutput = func() { firstOutput = int64Ptr(time.Since(started).Milliseconds()) }
	body, err := proxy.ReadBufferedResponseBody(reader)
	usage := reader.original.Result().Usage
	if err == nil {
		// Some providers wrap usage inside their native envelope. The
		// normalized terminal is authoritative once aggregation succeeds.
		if terminalUsage := ParseUsageFromBody(body); terminalUsage.Found {
			terminalUsage.invalidForUserBudget = terminalUsage.invalidForUserBudget || usage.invalidForUserBudget
			usage = terminalUsage
		}
	}
	usage.FirstOutputLatencyMs = firstOutput
	if ctx.Err() != nil {
		return nil, usage, ctx.Err()
	}
	if idle.guard.fired.Load() {
		return nil, usage, fmt.Errorf("upstream stream idle timeout")
	}
	return body, usage, err
}
