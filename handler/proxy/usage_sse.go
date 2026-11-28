package proxyhandler

import (
	"github.com/deliciousbuding/metapi-go/internal/toolusage"
)

// ParseUsageFromSSEEvents extracts the best-effort final usage from SSE data events.
// Later events win when they carry usage so stream-end payloads (message_delta,
// response.completed, final chat.completion.chunk) override partial early values.
func ParseUsageFromSSEEvents(events []SseEvent) ParsedUsage {
	var tracker toolusage.Tracker
	var best ParsedUsage
	best.Source = usageSourceUnknown
	for _, ev := range events {
		if ev.Data == "" || ev.Data == "[DONE]" {
			continue
		}
		if !looksLikeJSONObject(ev.Data) {
			continue
		}
		got := parseUsageWithTracker([]byte(ev.Data), &tracker)
		best = mergeUsagePreferLater(best, got)
	}
	best.ToolUsage = tracker.Snapshot()
	return best
}

func mergeUsagePreferLater(prev, next ParsedUsage) ParsedUsage {
	out := prev
	out.invalidForUserBudget = prev.invalidForUserBudget || next.invalidForUserBudget
	if next.UpstreamReportedModel != nil {
		out.UpstreamReportedModel = next.UpstreamReportedModel
	}
	if !next.Found {
		return out
	}
	out.Found = true
	out.Source = usageSourceUpstream
	// Prefer non-zero fields from the later event; retain earlier values when
	// the later payload is partial (e.g. Anthropic message_delta output only).
	if next.PromptTokens > 0 {
		out.PromptTokens = next.PromptTokens
	} else if !prev.Found {
		out.PromptTokens = next.PromptTokens
	}
	if next.CompletionTokens > 0 {
		out.CompletionTokens = next.CompletionTokens
	} else if !prev.Found {
		out.CompletionTokens = next.CompletionTokens
	}
	if next.CacheReadTokens > 0 {
		out.CacheReadTokens = next.CacheReadTokens
	} else if !prev.Found {
		out.CacheReadTokens = next.CacheReadTokens
	}
	if next.CacheCreationTokens > 0 {
		out.CacheCreationTokens = next.CacheCreationTokens
		// Some terminal usage objects repeat the aggregate without its earlier TTL split.
		if next.CacheCreation5mTokens+next.CacheCreation1hTokens+next.CacheCreation12hTokens > 0 || next.CacheCreationTokens < prev.CacheCreationTokens {
			out.CacheCreation5mTokens = next.CacheCreation5mTokens
			out.CacheCreation1hTokens = next.CacheCreation1hTokens
			out.CacheCreation12hTokens = next.CacheCreation12hTokens
		}
	} else if !prev.Found {
		out.CacheCreationTokens = next.CacheCreationTokens
	}
	if next.ReasoningTokens > 0 {
		out.ReasoningTokens = next.ReasoningTokens
		out.geminiSeparateThoughts = next.geminiSeparateThoughts
	} else if !prev.Found {
		out.ReasoningTokens = next.ReasoningTokens
	}
	// Recompute total from merged prompt+completion unless the later event
	// provides an explicit total that covers both sides (total >= sum).
	sum := out.PromptTokens + out.CompletionTokens
	if next.TotalTokens > 0 && next.TotalTokens >= sum {
		out.TotalTokens = next.TotalTokens
	} else if sum > 0 {
		out.TotalTokens = sum
	} else if next.TotalTokens > 0 {
		out.TotalTokens = next.TotalTokens
	} else if !prev.Found {
		out.TotalTokens = next.TotalTokens
	}
	return out
}
