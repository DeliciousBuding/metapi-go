package proxy

import (
	"encoding/json"
	"strings"

	"github.com/deliciousbuding/metapi-go/config"
)

// FailureResult is a content-based failure detection result.
type FailureResult struct {
	Status int
	Reason string
}

// UsageSummary is a lightweight usage summary for failure detection.
type UsageSummary struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// FailureCode names why the single content judge declared a failure. Buffered
// and streaming paths surface exactly this set, so one code means the same
// thing wherever it is logged, stored or alerted on.
type FailureCode string

const (
	// FailureCodeNone means the content judgement found no failure.
	FailureCodeNone FailureCode = ""
	// FailureCodeErrorKeyword means the upstream content matched an operator
	// configured PROXY_ERROR_KEYWORDS entry.
	FailureCodeErrorKeyword FailureCode = "upstream_error_keyword"
	// FailureCodeEmptyContent means PROXY_EMPTY_CONTENT_FAIL is enabled and the
	// upstream produced neither completion output nor completion tokens.
	FailureCodeEmptyContent FailureCode = "upstream_empty_content"
)

// UpstreamContentFacts is the protocol-agnostic description of what an upstream
// actually returned. Each call path fills it from data it already has: the
// buffered path from the raw response body, the streaming path from the bounded
// incremental SSE analyzer (which never retains the raw body).
type UpstreamContentFacts struct {
	// StatusCode is the HTTP status the upstream answered with. The judgement
	// itself stays status-agnostic (callers only judge 2xx answers); it is
	// carried so a verdict can be traced back to its attempt.
	StatusCode int
	// Streaming marks the SSE path, which has no raw body to inspect.
	Streaming bool
	// RawText is the content to keyword-scan. Buffered: the full response body.
	// Streaming: the concatenated SSE error-event payloads, the only content
	// signal the bounded analyzer keeps.
	RawText string
	// HasOutput reports whether the upstream produced completion content.
	// Streaming callers set it from the analyzer data-event flag; the buffered
	// path leaves it false and the judge derives it from RawText.
	HasOutput bool
	// Usage is the parsed usage summary (nil when the upstream sent none).
	Usage *UsageSummary
}

// UpstreamVerdict is the single content judgement result.
type UpstreamVerdict struct {
	Failed bool
	Code   FailureCode
	Status int
	Reason string
}

// JudgeUpstreamContent is the ONE owner of content-level failure judgement.
// Both the buffered and the streaming dispatch path call it, so the two can no
// longer drift apart in judgement strength — the historical split (buffered
// judged, stream only logged) is what let a failed upstream answer be recorded
// as a success.
//
// Pure: no I/O, no *http.Response, no protocol plumbing. Same facts in, same
// verdict out.
//
// Detection:
// 1. Keyword matching: if config.ProxyErrorKeywords is non-empty, case-insensitive
// 2. Empty content check: if ProxyEmptyContentFailEnabled and no completion tokens + no output
func JudgeUpstreamContent(facts UpstreamContentFacts) UpstreamVerdict {
	pass := UpstreamVerdict{Code: FailureCodeNone}
	rt := config.RuntimeSafe()
	if rt == nil {
		// No published runtime snapshot: nothing is configured, so nothing can
		// be judged. Never invent a failure (and never panic on a request path).
		return pass
	}
	rawText := strings.TrimSpace(facts.RawText)

	// 1. Keyword matching
	if len(rt.ProxyErrorKeywords) > 0 {
		normalizedText := strings.ToLower(rawText)
		for _, kw := range rt.ProxyErrorKeywords {
			kw = strings.TrimSpace(strings.ToLower(kw))
			if kw == "" {
				continue
			}
			if strings.Contains(normalizedText, kw) {
				return UpstreamVerdict{
					Failed: true,
					Code:   FailureCodeErrorKeyword,
					Status: 502,
					Reason: "Upstream response matched failure keyword: " + kw,
				}
			}
		}
	}

	// 2. Empty content check
	if rt.ProxyEmptyContentFailEnabled {
		compTokens := 0
		if facts.Usage != nil {
			compTokens = facts.Usage.CompletionTokens
		}
		hasOutput := facts.HasOutput
		if !facts.Streaming {
			hasOutput = detectHasUpstreamOutput(rawText)
		}
		if !hasOutput && compTokens <= 0 {
			return UpstreamVerdict{
				Failed: true,
				Code:   FailureCodeEmptyContent,
				Status: 502,
				Reason: "Upstream returned empty content",
			}
		}
	}

	return pass
}

// DetectProxyFailure detects proxy failures from response content.
// This is PURELY content-based — it does NOT look at HTTP status codes.
//
// Kept as the buffered-path shaped wrapper over JudgeUpstreamContent so there
// is still exactly one implementation of the judgement; callers that already
// have parsed content should prefer JudgeUpstreamContent directly.
func DetectProxyFailure(rawText string, usage *UsageSummary) *FailureResult {
	verdict := JudgeUpstreamContent(UpstreamContentFacts{RawText: rawText, Usage: usage})
	if !verdict.Failed {
		return nil
	}
	return &FailureResult{Status: verdict.Status, Reason: verdict.Reason}
}

// detectHasUpstreamOutput checks if raw text contains actual upstream output.
// Parses JSON or SSE event streams and looks for non-empty content.
func detectHasUpstreamOutput(rawText string) bool {
	trimmed := strings.TrimSpace(rawText)
	if trimmed == "" {
		return false
	}

	// Try JSON parse
	var parsed any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
		return hasCompletionContentFromPayload(parsed)
	}

	// Try SSE event stream parsing
	sseEvents := pullSseDataEvents(trimmed)
	if len(sseEvents) > 0 {
		for _, event := range sseEvents {
			payload := strings.TrimSpace(event)
			if payload == "" || payload == "[DONE]" {
				continue
			}
			var parsedEvent any
			if err := json.Unmarshal([]byte(payload), &parsedEvent); err == nil {
				if hasCompletionContentFromPayload(parsedEvent) {
					return true
				}
			} else {
				// Non-JSON payload still counts as output
				return true
			}
		}
		// SSE payloads exist but none contain output
		return false
	}

	// Looks like SSE but contains no non-DONE payloads
	if strings.Contains(rawText, "data:") {
		return false
	}

	// Not JSON and not SSE: assume plain-text output
	return true
}

// pullSseDataEvents extracts "data:" lines from SSE text.
func pullSseDataEvents(rawText string) []string {
	var events []string
	lines := strings.Split(rawText, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data:") {
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			events = append(events, payload)
		}
	}
	return events
}

// hasCompletionContentFromPayload checks if a parsed JSON payload has upstream output.
// This mirrors the TS hasCompletionContentFromPayload function.
func hasCompletionContentFromPayload(payload any) bool {
	if payload == nil {
		return false
	}

	obj, ok := payload.(map[string]any)
	if !ok {
		return false
	}

	// Check choices
	if choices, ok := obj["choices"].([]any); ok {
		for _, choice := range choices {
			if hasCompletionContentFromChoice(choice) {
				return true
			}
		}
	}

	// Direct output_text
	if s, ok := obj["output_text"].(string); ok && strings.TrimSpace(s) != "" {
		return true
	}
	if s, ok := obj["outputText"].(string); ok && strings.TrimSpace(s) != "" {
		return true
	}

	// Check output array
	if output, ok := obj["output"].([]any); ok {
		for _, item := range output {
			itemMap, ok := item.(map[string]any)
			if !ok {
				continue
			}
			itemType := strings.ToLower(stringValue(itemMap["type"]))
			if strings.Contains(itemType, "function_call") || strings.Contains(itemType, "tool_call") {
				return true
			}
			if s, ok := itemMap["text"].(string); ok && strings.TrimSpace(s) != "" {
				return true
			}
			if s, ok := itemMap["output_text"].(string); ok && strings.TrimSpace(s) != "" {
				return true
			}
			if content, ok := itemMap["content"].([]any); ok {
				for _, part := range content {
					if partHasContent(part) {
						return true
					}
				}
			}
			if hasToolCallLikeMap(itemMap) {
				return true
			}
		}
	}

	// Check content parts
	if content, ok := obj["content"].([]any); ok {
		for _, part := range content {
			if partHasContent(part) {
				return true
			}
		}
	}

	// Direct text/delta
	if s, ok := obj["delta"].(string); ok && strings.TrimSpace(s) != "" {
		return true
	}
	if s, ok := obj["text"].(string); ok && strings.TrimSpace(s) != "" {
		return true
	}
	if hasToolCallLikeMap(obj) {
		return true
	}

	return false
}

func hasCompletionContentFromChoice(choice any) bool {
	cm, ok := choice.(map[string]any)
	if !ok {
		return false
	}
	if s, ok := cm["text"].(string); ok && strings.TrimSpace(s) != "" {
		return true
	}
	if s, ok := cm["completion"].(string); ok && strings.TrimSpace(s) != "" {
		return true
	}
	if s, ok := cm["output_text"].(string); ok && strings.TrimSpace(s) != "" {
		return true
	}

	message, _ := cm["message"].(map[string]any)
	if message != nil {
		if s, ok := message["content"].(string); ok && strings.TrimSpace(s) != "" {
			return true
		}
		if contentArr, ok := message["content"].([]any); ok {
			for _, part := range contentArr {
				if partHasContent(part) {
					return true
				}
			}
		}
		if s, ok := message["refusal"].(string); ok && strings.TrimSpace(s) != "" {
			return true
		}
		if hasToolCallLikeMap(message) {
			return true
		}
	}

	// Direct tool calls on choice
	if hasToolCallLikeMap(cm) {
		return true
	}

	// Delta
	delta, _ := cm["delta"].(map[string]any)
	if delta != nil {
		if s, ok := delta["content"].(string); ok && strings.TrimSpace(s) != "" {
			return true
		}
		if s, ok := delta["refusal"].(string); ok && strings.TrimSpace(s) != "" {
			return true
		}
		if hasToolCallLikeMap(delta) {
			return true
		}
	}

	return false
}

func partHasContent(part any) bool {
	pm, ok := part.(map[string]any)
	if !ok {
		return false
	}
	if s, ok := pm["text"].(string); ok && strings.TrimSpace(s) != "" {
		return true
	}
	if s, ok := pm["output_text"].(string); ok && strings.TrimSpace(s) != "" {
		return true
	}
	if s, ok := pm["content"].(string); ok && strings.TrimSpace(s) != "" {
		return true
	}
	partType := strings.ToLower(stringValue(pm["type"]))
	if strings.Contains(partType, "function_call") || strings.Contains(partType, "tool_call") {
		return true
	}
	return false
}

func hasToolCallLikeMap(m map[string]any) bool {
	for _, key := range []string{"tool_calls", "toolCalls", "function_call", "functionCall"} {
		if v, ok := m[key]; ok {
			return hasToolCallLike(v)
		}
	}
	return false
}

func hasToolCallLike(v any) bool {
	if v == nil {
		return false
	}
	if arr, ok := v.([]any); ok {
		return len(arr) > 0
	}
	if m, ok := v.(map[string]any); ok {
		return len(m) > 0
	}
	return false
}

func stringValue(v any) string {
	s, _ := v.(string)
	return s
}
