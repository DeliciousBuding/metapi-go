package proxyhandler

import (
	"context"
	"encoding/json"
	"time"

	"github.com/deliciousbuding/metapi-go/internal/streamtiming"
	"github.com/deliciousbuding/metapi-go/proxy"
)

type streamTimingKey struct{}

// The observer samples the arrival clock. Conversion, downstream flush and EOF
// processing can run later, but cannot extend a decode interval.
type attemptStreamTiming struct {
	observer         *streamtiming.Observer
	protocol         streamtiming.Protocol
	arrival, endedAt time.Time
	interrupted      bool
}

func startAttemptStreamTiming(ctx context.Context, path string, started time.Time) context.Context {
	endpoint, ok := proxy.EndpointFromPath(path)
	if !ok {
		return ctx
	}
	if wire := directProviderWireFromContext(ctx); wire != nil {
		switch wire.Profile {
		case "codex":
			endpoint = proxy.EndpointResponses
		case "bedrock", "cline", "ollama", "codex-image", "antigravity":
			// The SSE relay sees a projected or enveloped provider protocol.
			// Keep the header observation without attributing synthetic content.
			endpoint = ""
		}
	}
	t := &attemptStreamTiming{protocol: streamtiming.Protocol(endpoint), arrival: started}
	t.observer = streamtiming.NewObserver(func() time.Time { return t.arrival })
	return context.WithValue(ctx, streamTimingKey{}, t)
}

func streamTimingFromContext(ctx context.Context) *attemptStreamTiming {
	t, _ := ctx.Value(streamTimingKey{}).(*attemptStreamTiming)
	return t
}

func (t *attemptStreamTiming) observe(event SseEvent) {
	data := []byte(event.Data)
	if t.protocol == "" || (event.Data != "[DONE]" && !json.Valid(data)) {
		return
	}
	t.endedAt = t.arrival
	t.observer.ObserveFrame(t.protocol, event.Event, data)
}

func attemptTimingSnapshot(ctx context.Context, isStream bool, usage ParsedUsage, failed bool) any {
	t := streamTimingFromContext(ctx)
	if !isStream || t == nil {
		return nil
	}
	output := usage.CompletionTokens
	if usage.geminiSeparateThoughts {
		output += usage.ReasoningTokens
	}
	return t.observer.Finalize(streamtiming.FinalizeInput{OutputTokens: output, ReasoningTokens: usage.ReasoningTokens, HasUsage: usage.Found, Interrupted: failed || t.interrupted, EndedAt: t.endedAt})
}
