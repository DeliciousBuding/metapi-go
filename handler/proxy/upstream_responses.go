package proxyhandler

import (
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/deliciousbuding/metapi-go/handler/shared"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/transform/openai/responses"
)

func responsesOnlyClientError(downstreamPath string, bodyBytes []byte, pref proxy.SiteProtocolPreference) string {
	if !pref.ResponsesOnly {
		return ""
	}
	ep, ok := proxy.EndpointFromPath(downstreamPath)
	if !ok || ep == proxy.EndpointResponses {
		return ""
	}
	// If body already looks responses-shaped (has input, no messages), allow path rewrite.
	if bodyLooksResponsesShaped(bodyBytes) {
		return ""
	}
	return proxy.ResponsesOnlyChatUnsupportedMessage(downstreamPath)
}

func bodyLooksResponsesShaped(bodyBytes []byte) bool {
	if len(bodyBytes) == 0 {
		return false
	}
	var body map[string]any
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		return false
	}
	if _, hasMessages := body["messages"]; hasMessages {
		return false
	}
	if _, hasInput := body["input"]; hasInput {
		return true
	}
	return false
}

// upstreamStreamRewriteGates computes the per-candidate gates for the
// single-pass stream rewrite: stream forcing (responses-only / stream-
// preferring sites, codex/sub2api platforms) and include_usage injection
// (OpenAI-compatible chat/completions paths on accepting platforms).
func upstreamStreamRewriteGates(sitePlatform, upstreamPath string, pref proxy.SiteProtocolPreference) (forceStream, injectUsage bool) {
	pathLower := strings.ToLower(upstreamPath)
	isCompact := strings.Contains(pathLower, "/responses/compact")
	forceStream = responses.ShouldForceResponsesUpstreamStream(sitePlatform, isCompact) ||
		proxy.ShouldForceUpstreamStream(pref, upstreamPath, isCompact)
	injectUsage = acceptsOpenAIStreamIncludeUsagePath(upstreamPath) && !rejectsOpenAIStreamOptions(sitePlatform)
	return forceStream, injectUsage
}

// applyUpstreamStreamPreference forces stream=true when site/platform requires it.
// The rewrite is a single allocation-free scan + splice; irregular bodies fall
// back to the legacy full-decode path for exact semantics.
func applyUpstreamStreamPreference(bodyBytes []byte, sitePlatform, upstreamPath string, pref proxy.SiteProtocolPreference) ([]byte, bool) {
	pathLower := strings.ToLower(upstreamPath)
	isCompact := strings.Contains(pathLower, "/responses/compact")
	// Platform helper (codex/sub2api) OR site preference.
	force := responses.ShouldForceResponsesUpstreamStream(sitePlatform, isCompact) ||
		proxy.ShouldForceUpstreamStream(pref, upstreamPath, isCompact)
	if !force {
		return bodyBytes, false
	}
	out, forced, _, ok := rewriteUpstreamStreamFlags(bodyBytes, scanBodyStreamHints(bodyBytes), true, false, false)
	if ok {
		return out, forced
	}
	return legacyApplyStreamPreference(bodyBytes)
}

// legacyApplyStreamPreference is the original map[string]any implementation,
// retained as the fallback for bodies the allocation-free scanner declines.
func legacyApplyStreamPreference(bodyBytes []byte) ([]byte, bool) {
	if len(bodyBytes) == 0 {
		return []byte(`{"stream":true}`), true
	}
	var body map[string]any
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		return bodyBytes, false
	}
	// Already streaming?
	if v, ok := body["stream"]; ok {
		if b, ok := v.(bool); ok && b {
			return bodyBytes, false
		}
		if s, ok := v.(string); ok && (s == "true" || s == "1") {
			return bodyBytes, false
		}
	}
	next := make(map[string]any, len(body)+1)
	for k, v := range body {
		next[k] = v
	}
	next["stream"] = true
	out, err := json.Marshal(next)
	if err != nil {
		return bodyBytes, false
	}
	return out, true
}

// applyUpstreamStreamIncludeUsage forces stream_options.include_usage=true on OpenAI-compatible
// chat/completions and legacy /v1/completions stream bodies so upstream SSE emits a final usage chunk (known limitation).
// Platform-safe: skips non-chat endpoints and platforms known to reject stream_options (codex/sub2api).
// Does not invent tokens; only asks the provider to include usage when streaming.
// The rewrite is a single allocation-free scan + splice; irregular bodies fall
// back to the legacy full-decode path for exact semantics.

// The bool is true when the outbound stream body is expected to carry usage via include_usage
// (we injected it, or the client already set include_usage=true on an accepting path). Callers
// use that flag to warn once if the stream ends without extracted usage (known limitation).
func applyUpstreamStreamIncludeUsage(bodyBytes []byte, sitePlatform, upstreamPath string, isStream bool) ([]byte, bool) {
	if !isStream || len(bodyBytes) == 0 {
		return bodyBytes, false
	}
	if !acceptsOpenAIStreamIncludeUsagePath(upstreamPath) {
		return bodyBytes, false
	}
	if rejectsOpenAIStreamOptions(sitePlatform) {
		return bodyBytes, false
	}
	out, _, expect, ok := rewriteUpstreamStreamFlags(bodyBytes, scanBodyStreamHints(bodyBytes), false, isStream, true)
	if ok {
		return out, expect
	}
	return legacyApplyStreamIncludeUsage(bodyBytes)
}

// legacyApplyStreamIncludeUsage is the original map[string]any implementation,
// retained as the fallback for bodies the allocation-free scanner declines.
func legacyApplyStreamIncludeUsage(bodyBytes []byte) ([]byte, bool) {
	var body map[string]any
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		return bodyBytes, false
	}
	opts, _ := body["stream_options"].(map[string]any)
	if opts == nil {
		opts = map[string]any{}
	} else {
		// Copy so we do not mutate nested maps shared with original body reference.
		nextOpts := make(map[string]any, len(opts)+1)
		for k, v := range opts {
			nextOpts[k] = v
		}
		opts = nextOpts
	}
	if jsonTruthyBool(opts["include_usage"]) {
		// Already requested; leave other stream_options keys intact without rewrite.
		// Still expect a final usage chunk from upstream.
		return bodyBytes, true
	}
	opts["include_usage"] = true
	next := make(map[string]any, len(body)+1)
	for k, v := range body {
		next[k] = v
	}
	next["stream_options"] = opts
	out, err := json.Marshal(next)
	if err != nil {
		return bodyBytes, false
	}
	return out, true
}

// shouldWarnMissingStreamUsage reports whether a completed/partial stream that requested
// include_usage still lacks usable token counts. Never invents tokens — only detects absence.
func shouldWarnMissingStreamUsage(expectIncludeUsage bool, usage ParsedUsage) bool {
	if !expectIncludeUsage {
		return false
	}
	if !usage.Found {
		return true
	}
	// Found but all zero: provider emitted a usage object without counts (still known limitation).
	return usage.PromptTokens == 0 && usage.CompletionTokens == 0 && usage.TotalTokens == 0 &&
		usage.CacheReadTokens == 0 && usage.CacheCreationTokens == 0 && usage.ReasoningTokens == 0
}

// warnMissingStreamUsageAfterIncludeUsage logs once per call site (one success/partial end path).
// model/path identify the request; tokens are never invented here.
func warnMissingStreamUsageAfterIncludeUsage(model, path string, usage ParsedUsage) {
	if !shouldWarnMissingStreamUsage(true, usage) {
		return
	}
	shared.RecordStreamMissingUsage()
	slog.Warn("stream ended without usage after include_usage",
		"model", model,
		"path", path,
		"usage_found", usage.Found,
		"prompt_tokens", usage.PromptTokens,
		"completion_tokens", usage.CompletionTokens,
		"total_tokens", usage.TotalTokens,
	)
}

// acceptsOpenAIStreamIncludeUsagePath reports OpenAI-compatible paths that honor
// stream_options.include_usage (chat + legacy completions). Messages/Responses excluded.
func acceptsOpenAIStreamIncludeUsagePath(upstreamPath string) bool {
	ep, ok := proxy.EndpointFromPath(upstreamPath)
	if ok && ep == proxy.EndpointChat {
		return true
	}
	path := strings.TrimSpace(upstreamPath)
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	path = strings.TrimRight(path, "/")
	// Legacy OpenAI completions (not chat/completions — EndpointChat already matched).
	if path == "/v1/completions" || path == "/completions" || strings.HasSuffix(path, "/v1/completions") {
		return true
	}
	return false
}

// rejectsOpenAIStreamOptions reports platforms that historically 400 on stream_options
// (original metapi Codex OAuth) or always strip it in Responses sanitize.
func rejectsOpenAIStreamOptions(sitePlatform string) bool {
	switch strings.ToLower(strings.TrimSpace(sitePlatform)) {
	case "codex", "chatgpt-codex", "chatgpt codex", "sub2api":
		return true
	default:
		return false
	}
}

// jsonTruthyBool accepts JSON bool or common string encodings of true.
func jsonTruthyBool(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		s := strings.TrimSpace(strings.ToLower(t))
		return s == "true" || s == "1" || s == "yes"
	default:
		return false
	}
}

// dispatchEndpointAttempt is the single-path entry used by multipart.
