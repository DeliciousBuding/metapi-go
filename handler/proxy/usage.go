package proxyhandler

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"unicode"

	"github.com/deliciousbuding/metapi-go/internal/toolusage"
	"github.com/deliciousbuding/metapi-go/proxy"
)

const (
	usageSourceUpstream = "upstream"
	usageSourceUnknown  = "unknown"
)

// ParsedUsage is a normalized token usage snapshot extracted from an upstream body/SSE.
type ParsedUsage struct {
	upstreamTaskID         string // original media identity only; never inferred from decoder output
	invalidForUserBudget   bool
	videoAccounting        *directVideoAccounting
	UpstreamReportedModel  *string
	FirstOutputLatencyMs   *int64
	PromptTokens           int64
	CompletionTokens       int64
	TotalTokens            int64
	CacheReadTokens        int64
	CacheCreationTokens    int64
	CacheCreation5mTokens  int64
	CacheCreation1hTokens  int64
	CacheCreation12hTokens int64
	ToolUsage              []toolusage.ToolUsageItem
	geminiSeparateThoughts bool
	// ReasoningTokens captures Gemini thoughtsTokenCount / OpenAI reasoning_tokens
	// when reported separately. Not persisted as its own proxy_logs column; it is
	// folded into CompletionTokens / TotalTokens when total is missing.
	ReasoningTokens int64
	// Source is "upstream" when any token field was present, otherwise "unknown".
	Source string
	Found  bool
}

// ToUsageSummary converts ParsedUsage into the lightweight failure-detection summary.
func (u ParsedUsage) ToUsageSummary() *proxy.UsageSummary {
	return &proxy.UsageSummary{
		PromptTokens:     int(u.PromptTokens),
		CompletionTokens: int(u.CompletionTokens),
		TotalTokens:      int(u.TotalTokens),
	}
}

// ParseUsageFromBody extracts token usage from a non-stream JSON response body.
// Supports OpenAI (prompt_tokens/completion_tokens/total_tokens), Anthropic
// (input_tokens/output_tokens + cache_*_input_tokens), Gemini
// (usageMetadata.*TokenCount including thoughtsTokenCount), nested
// Responses / message_start shapes, and media-ish OpenAI details
// (input_tokens_details text/image/audio; output audio/image) when present.

// Does not invent usage when upstream omitted token fields.
func ParseUsageFromBody(body []byte) ParsedUsage {
	var tracker toolusage.Tracker
	out := parseUsageWithTracker(body, &tracker)
	out.ToolUsage = tracker.Snapshot()
	return out
}

func parseUsageWithTracker(body []byte, tracker *toolusage.Tracker) ParsedUsage {
	body = trimJSONNoise(body)
	if len(body) == 0 {
		return ParsedUsage{Source: usageSourceUnknown}
	}
	var payload map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if decoder.Decode(&payload) != nil || decoder.Decode(new(any)) != io.EOF {
		return ParsedUsage{Source: usageSourceUnknown}
	}
	tracker.ObserveObject(payload)
	out := extractUsageFromValue(payload)
	out.UpstreamReportedModel = extractReportedModel(payload)
	return out
}

// Only protocol-owned model metadata is observed. Do not search tool results,
// user content or errors, and never infer an upstream identity from our request.
func extractReportedModel(obj map[string]any) *string {
	if obj["error"] != nil {
		return nil
	}
	for _, field := range []string{"response", "message"} {
		if nested, ok := obj[field].(map[string]any); ok {
			if model := reportedModelString(nested["model"]); model != nil {
				return model
			}
		}
	}
	if model := reportedModelString(obj["model"]); model != nil {
		return model
	}
	return reportedModelString(obj["modelVersion"])
}

func reportedModelString(value any) *string {
	model, ok := value.(string)
	if !ok || model == "" || len(model) > 256 || strings.IndexFunc(model, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return nil
	}
	return &model
}

func asInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case float32:
		return int64(n), true
	case int64:
		return n, true
	case int:
		return int64(n), true
	case int32:
		return int64(n), true
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			f, err2 := n.Float64()
			if err2 != nil {
				return 0, false
			}
			return int64(f), true
		}
		return i, true
	case string:
		s := strings.TrimSpace(n)
		if s == "" {
			return 0, false
		}
		var parsed int64
		for _, ch := range s {
			if ch < '0' || ch > '9' {
				return 0, false
			}
			parsed = parsed*10 + int64(ch-'0')
		}
		return parsed, true
	default:
		return 0, false
	}
}

func trimJSONNoise(body []byte) []byte {
	return []byte(strings.TrimSpace(string(body)))
}
