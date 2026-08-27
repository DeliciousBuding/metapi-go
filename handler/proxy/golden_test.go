package proxyhandler

// golden_test.go pins response-body usage extraction and incremental SSE
// stream parsing with checked-in snapshots. In metapi-go the response/stream
// half of protocol conversion lives here (SSE relay is byte pass-through;
// usage/finish/error state is parsed for billing and health), so these
// fixtures cover the request/response/stream golden categories that the
// transform/ packages do not own.
// Regenerate snapshots with GOLDEN_UPDATE=1 go test ./handler/proxy.

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/internal/golden"
)

type usageGolden struct {
	Found               bool   `json:"found"`
	PromptTokens        int64  `json:"promptTokens"`
	CompletionTokens    int64  `json:"completionTokens"`
	TotalTokens         int64  `json:"totalTokens"`
	CacheReadTokens     int64  `json:"cacheReadTokens"`
	CacheCreationTokens int64  `json:"cacheCreationTokens"`
	ReasoningTokens     int64  `json:"reasoningTokens"`
	Source              string `json:"source"`
}

func usageGoldenFrom(u ParsedUsage) usageGolden {
	return usageGolden{
		Found:               u.Found,
		PromptTokens:        u.PromptTokens,
		CompletionTokens:    u.CompletionTokens,
		TotalTokens:         u.TotalTokens,
		CacheReadTokens:     u.CacheReadTokens,
		CacheCreationTokens: u.CacheCreationTokens,
		ReasoningTokens:     u.ReasoningTokens,
		Source:              u.Source,
	}
}

func TestGoldenParseUsageFromBody(t *testing.T) {
	cases := []string{
		"usage_openai_chat",
		"usage_openai_responses",
		"usage_gemini_native",
		"usage_gemini_thoughts_no_total",
		"usage_anthropic_messages",
		"usage_missing",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			raw := golden.ReadRawInput(t, "response", name, ".json")

			got := ParseUsageFromBody(raw)
			golden.Check(t, "response", name, usageGoldenFrom(got))
		})
	}
}

type streamGolden struct {
	EventCount            int         `json:"eventCount"`
	HasDataEvent          bool        `json:"hasDataEvent"`
	HasErrorEvent         bool        `json:"hasErrorEvent"`
	HasDoneMarker         bool        `json:"hasDoneMarker"`
	DroppedOversizedEvent bool        `json:"droppedOversizedEvent"`
	PendingBytes          int         `json:"pendingBytes"`
	Usage                 usageGolden `json:"usage"`
}

func analyzeStream(t *testing.T, raw []byte, chunkSize int) streamGolden {
	t.Helper()
	analyzer := newIncrementalSseAnalyzer()
	if chunkSize <= 0 || chunkSize >= len(raw) {
		analyzer.Push(raw)
	} else {
		for i := 0; i < len(raw); i += chunkSize {
			end := i + chunkSize
			if end > len(raw) {
				end = len(raw)
			}
			analyzer.Push(raw[i:end])
		}
	}
	res := analyzer.Result()
	return streamGolden{
		EventCount:            res.EventCount,
		HasDataEvent:          res.HasDataEvent,
		HasErrorEvent:         res.HasErrorEvent,
		HasDoneMarker:         res.HasDoneMarker,
		DroppedOversizedEvent: res.DroppedOversizedEvent,
		PendingBytes:          res.PendingBytes,
		Usage:                 usageGoldenFrom(res.Usage),
	}
}

func TestGoldenSseStreams(t *testing.T) {
	cases := []struct {
		name      string
		chunkSize int
	}{
		{"sse_openai_chat", 0},
		// Same fixture pushed in awkward 7-byte chunks: parsed state must be
		// identical, pinning chunk-boundary independence of the SSE analyzer.
		{"sse_openai_chat_split", 7},
		{"sse_gemini", 0},
		{"sse_responses", 0},
		{"sse_anthropic", 0},
		{"sse_error", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw := golden.ReadRawInput(t, "stream", c.name, ".sse")

			golden.Check(t, "stream", c.name, analyzeStream(t, raw, c.chunkSize))
		})
	}
}
