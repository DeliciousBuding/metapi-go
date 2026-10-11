package proxy

import (
	"encoding/json"
	"strings"

	"github.com/deliciousbuding/metapi-go/config"
)

// UsageSummary is a lightweight usage summary for failure detection.
type UsageSummary struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// FailureCode names why the single content judge declared a failure. A code
// keeps the same meaning wherever it is logged, stored or alerted on; explicit
// protocol-error evidence is currently supplied by the streaming path.
type FailureCode string

const (
	// FailureCodeNone means the content judgement found no failure.
	FailureCodeNone FailureCode = ""
	// FailureCodeErrorEvent means the upstream explicitly reported a protocol
	// error, independent of optional keyword and empty-content heuristics.
	FailureCodeErrorEvent FailureCode = "upstream_error_event"
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
	// Streaming callers set it from output-bearing events; the buffered
	// path leaves it false and the judge derives it from RawText.
	HasOutput bool
	// HasErrorEvent reports a parsed protocol error, not output mentioning an
	// error. Streaming callers set it from the SSE analyzer.
	HasErrorEvent bool
	// Usage is the parsed usage summary (nil when the upstream sent none).
	Usage *UsageSummary
	// Unreadable reports that the bytes behind these facts could not be read at
	// all — an upstream Content-Encoding we do not decode (br/zstd/a stacked
	// list), or a codec we do implement that failed on this body. It is the
	// caller's honest admission that it has NO content evidence, and it short-
	// circuits the whole judgement: a body we cannot parse must never be turned
	// into a failure, because that records a 502 against a channel whose answer
	// may have been perfectly good (see proxy/upstream_encoding.go).
	Unreadable bool
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
// 0. Unreadable body: no judgement at all (see UpstreamContentFacts.Unreadable)
// 1. Keyword matching: if config.ProxyErrorKeywords is non-empty, case-insensitive
// 2. Explicit protocol error events: always fail, even with output or usage
// 3. Empty content check: if ProxyEmptyContentFailEnabled and no completion tokens + no output
func JudgeUpstreamContent(facts UpstreamContentFacts) UpstreamVerdict {
	pass := UpstreamVerdict{Code: FailureCodeNone}
	if facts.Unreadable {
		// No readable content evidence: neither the keyword scan nor the
		// empty-content rule may run. Both would be guessing at bytes nobody
		// parsed, and the empty-content rule in particular would report a
		// healthy-but-encoded answer as a 502 — a false failure that poisons
		// channel health. The caller surfaces the gap through
		// UpstreamEncodingSkippedMessage instead.
		return pass
	}
	rt := config.RuntimeSafe()
	rawText := strings.TrimSpace(facts.RawText)

	// 1. Keyword matching
	if rt != nil && len(rt.ProxyErrorKeywords) > 0 {
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

	// 2. Explicit errors are evidence, not an opt-in content heuristic.
	if facts.HasErrorEvent {
		return UpstreamVerdict{
			Failed: true,
			Code:   FailureCodeErrorEvent,
			Status: 502,
			Reason: "Upstream returned an error event",
		}
	}

	// 3. Empty content check
	if rt != nil && rt.ProxyEmptyContentFailEnabled {
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

	if hasMediaOutput(obj) {
		return true
	}
	if candidates, ok := obj["candidates"].([]any); ok {
		for _, candidate := range candidates {
			item, _ := candidate.(map[string]any)
			content, _ := item["content"].(map[string]any)
			parts, _ := content["parts"].([]any)
			for _, part := range parts {
				if partHasContent(part) {
					return true
				}
			}
		}
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

// Media APIs return structured results rather than completion text. Only
// recognized result shapes count; an ID or an arbitrary non-empty array does
// not turn an empty/error response into successful output.
func hasMediaOutput(obj map[string]any) bool {
	if data, ok := obj["data"].([]any); ok {
		for _, value := range data {
			item, _ := value.(map[string]any)
			for _, key := range []string{"b64_json", "url", "embedding"} {
				if s, ok := item[key].(string); ok && strings.TrimSpace(s) != "" {
					return true
				}
			}
			if hasNumericVector(item["embedding"]) {
				return true
			}
		}
	}
	if embedding, ok := obj["embedding"].(map[string]any); ok && hasNumericVector(embedding["values"]) {
		return true
	}
	if embeddings, ok := obj["embeddings"].([]any); ok {
		for _, value := range embeddings {
			if embedding, ok := value.(map[string]any); ok && hasNumericVector(embedding["values"]) {
				return true
			}
		}
	}
	if results, ok := obj["results"].([]any); ok {
		for _, value := range results {
			item, _ := value.(map[string]any)
			if _, ok := item["relevance_score"].(float64); ok {
				if index, ok := item["index"].(float64); ok && index >= 0 {
					return true
				}
			}
			if _, ok := item["flagged"].(bool); ok {
				if categories, ok := item["categories"].(map[string]any); ok && len(categories) > 0 {
					return true
				}
			}
		}
	}
	if obj["object"] == "video" && strings.TrimSpace(stringValue(obj["id"])) != "" {
		switch obj["status"] {
		case "queued", "in_progress", "completed", "failed":
			// A failed asynchronous task is still a populated native resource.
			// Its status/error must reach the client, not become an empty-body 502.
			return true
		}
	}
	// Native video deletion acknowledges a completed operation without content.
	if obj["object"] == "video.deleted" && obj["deleted"] == true && strings.TrimSpace(stringValue(obj["id"])) != "" {
		return true
	}
	return false
}

func hasNumericVector(value any) bool {
	values, ok := value.([]any)
	if !ok || len(values) == 0 {
		return false
	}
	for _, item := range values {
		if _, ok := item.(float64); !ok {
			return false
		}
	}
	return true
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
		if partsHaveContent(message["images"]) {
			return true
		}
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
		if partsHaveContent(delta["content"]) || partsHaveContent(delta["images"]) {
			return true
		}
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
	if partType == "image_url" {
		image, _ := pm["image_url"].(map[string]any)
		if strings.TrimSpace(stringValue(image["url"])) != "" {
			return true
		}
	}
	for key, field := range map[string]string{"inlineData": "data", "fileData": "fileUri", "functionCall": "name"} {
		data, _ := pm[key].(map[string]any)
		if strings.TrimSpace(stringValue(data[field])) != "" {
			return true
		}
	}
	if strings.Contains(partType, "function_call") || strings.Contains(partType, "tool_call") {
		return true
	}
	return false
}

func partsHaveContent(value any) bool {
	parts, _ := value.([]any)
	for _, part := range parts {
		if partHasContent(part) {
			return true
		}
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
