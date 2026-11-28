package proxyhandler

import (
	"github.com/deliciousbuding/metapi-go/internal/toolusage"
)

func extractUsageFromValue(obj map[string]any) ParsedUsage {
	out := ParsedUsage{Source: usageSourceUnknown}

	// Direct usage object (OpenAI / Anthropic / Responses).
	applyUsageContainer(&out, obj, "usage")
	// Gemini-style usageMetadata.
	applyUsageContainer(&out, obj, "usageMetadata")
	// Nested response.usage (OpenAI Responses API stream events).
	if resp, ok := obj["response"].(map[string]any); ok {
		applyUsageContainer(&out, resp, "usage")
		// Antigravity wraps native Gemini usage in the response envelope.
		applyUsageContainer(&out, resp, "usageMetadata")
	}
	// Nested message.usage (Anthropic message_start).
	if msg, ok := obj["message"].(map[string]any); ok {
		applyUsageContainer(&out, msg, "usage")
	}

	finalizeParsedUsage(&out)
	return out
}

func applyUsageContainer(out *ParsedUsage, obj map[string]any, field string) {
	value, present := obj[field]
	if !present || value == nil {
		return
	} // OpenAI intermediate chunks have usage:null.
	usage, ok := value.(map[string]any)
	if !ok {
		out.invalidForUserBudget = true
		return
	}
	applyUsageMap(out, usage, field == "usageMetadata")
}

func applyUsageMap(out *ParsedUsage, usage map[string]any, gemini bool) {
	if out == nil || usage == nil {
		return
	}
	// Preserve the legacy protocol/accounting parser behavior for system Keys,
	// but separately mark malformed counters for fail-closed user settlement.
	if !validUserBudgetUsageMap(usage, gemini) {
		out.invalidForUserBudget = true
	}

	// OpenAI chat.completions style
	if n, ok := asInt64(usage["prompt_tokens"]); ok {
		out.PromptTokens = n
		out.Found = true
	}
	if n, ok := asInt64(usage["completion_tokens"]); ok {
		out.CompletionTokens = n
		out.Found = true
	}
	if n, ok := asInt64(usage["total_tokens"]); ok {
		out.TotalTokens = n
		out.Found = true
	}

	// Anthropic + OpenAI Responses style (input/output).
	// Note: Anthropic input_tokens is the *non-cached* input portion; cache
	// fields are exclusive and must be added for total prompt accounting.
	// OpenAI Responses input_tokens already includes cached_tokens (subset).
	if n, ok := asInt64(usage["input_tokens"]); ok {
		out.PromptTokens = n
		out.Found = true
	}
	if n, ok := asInt64(usage["output_tokens"]); ok {
		out.CompletionTokens = n
		out.Found = true
	}

	// Anthropic exclusive cache fields (not subsets of input_tokens).
	var anthropicCacheRead, anthropicCacheCreate int64
	sawAnthropicCacheKeys := false
	if cacheRead, ok := asInt64(usage["cache_read_input_tokens"]); ok {
		out.CacheReadTokens = cacheRead
		anthropicCacheRead = cacheRead
		out.Found = true
		sawAnthropicCacheKeys = true
	}
	if cacheCreate, ok := asInt64(usage["cache_creation_input_tokens"]); ok {
		out.CacheCreationTokens = cacheCreate
		anthropicCacheCreate = cacheCreate
		out.Found = true
		sawAnthropicCacheKeys = true
	}
	cache := toolusage.ExtractCacheUsageObject(usage)
	if cache.CacheCreationTokens > 0 {
		out.CacheCreationTokens = cache.CacheCreationTokens
		out.CacheCreation5mTokens = cache.CacheCreation5mTokens
		out.CacheCreation1hTokens = cache.CacheCreation1hTokens
		out.CacheCreation12hTokens = cache.CacheCreation12hTokens
		out.Found = true
	}
	if cache.CacheReadTokens > 0 {
		out.CacheReadTokens = cache.CacheReadTokens
		out.Found = true
	}
	anthropicTiers := cache.CacheCreation5mTokens + cache.CacheCreation1hTokens
	if sawAnthropicCacheKeys || anthropicTiers > 0 {
		// input_tokens (if present) is non-cached; expand prompt only by Anthropic
		// exclusive cache fields (never re-adding OpenAI prompt_tokens_details subsets).
		exclusiveCreate := anthropicCacheCreate
		if exclusiveCreate == 0 && anthropicTiers > 0 {
			exclusiveCreate = anthropicTiers
		}
		out.PromptTokens = out.PromptTokens + anthropicCacheRead + exclusiveCreate
	}

	// OpenAI prompt_tokens_details.cached_tokens / input_tokens_details.cached_tokens
	// are subsets of prompt/input tokens — record for cache pricing, do not add again.
	// Media (): when top-level prompt/input is absent, sum text/image/audio
	// detail leaves so images/audio endpoints that only emit details still count.
	if details, ok := usage["prompt_tokens_details"].(map[string]any); ok {
		if n, ok := asInt64(details["cached_tokens"]); ok {
			out.CacheReadTokens = n
			out.Found = true
		}
		applyOpenAIMediaDetailPrompt(out, details)
	}
	if details, ok := usage["input_tokens_details"].(map[string]any); ok {
		if n, ok := asInt64(details["cached_tokens"]); ok {
			out.CacheReadTokens = n
			out.Found = true
		}
		applyOpenAIMediaDetailPrompt(out, details)
	}

	// OpenAI completion_tokens_details.reasoning_tokens is typically already inside
	// completion_tokens / total_tokens. Record only; do not double-count when total present.
	// Media: audio_tokens / image_tokens under completion/output details fill completion
	// only when top-level completion/output is missing (no invent beyond reported leaves).
	if details, ok := usage["completion_tokens_details"].(map[string]any); ok {
		if n, ok := asInt64(details["reasoning_tokens"]); ok {
			out.ReasoningTokens = n
			out.Found = true
		}
		applyOpenAIMediaDetailCompletion(out, details)
	}
	if details, ok := usage["output_tokens_details"].(map[string]any); ok {
		if n, ok := asInt64(details["reasoning_tokens"]); ok {
			out.ReasoningTokens = n
			out.Found = true
		}
		applyOpenAIMediaDetailCompletion(out, details)
	}

	// Gemini-style
	if n, ok := asInt64(usage["promptTokenCount"]); ok {
		out.PromptTokens = n
		out.Found = true
	}
	if n, ok := asInt64(usage["candidatesTokenCount"]); ok {
		out.CompletionTokens = n
		out.Found = true
	}
	if n, ok := asInt64(usage["totalTokenCount"]); ok {
		out.TotalTokens = n
		out.Found = true
	}
	// thoughtsTokenCount is usually included in totalTokenCount when total is present.
	// When total is omitted, fold thoughts into completion so stats do not under-count.
	// Do not fold OpenAI reasoning_tokens the same way: those are already inside
	// completion_tokens when reported.
	if n, ok := asInt64(usage["thoughtsTokenCount"]); ok {
		out.ReasoningTokens = n
		out.Found = true
		if _, hasTotal := asInt64(usage["totalTokenCount"]); !hasTotal && n > 0 {
			out.CompletionTokens += n
		} else {
			out.geminiSeparateThoughts = hasTotal
		}
	}
}

// applyOpenAIMediaDetailPrompt folds text/image/audio detail leaves into PromptTokens
// only when PromptTokens is still zero (top-level input/prompt missing). Details are
// subsets when top-level is present — never double-add.
func applyOpenAIMediaDetailPrompt(out *ParsedUsage, details map[string]any) {
	if out == nil || details == nil {
		return
	}
	var sum int64
	var any bool
	for _, key := range []string{"text_tokens", "image_tokens", "audio_tokens"} {
		if n, ok := asInt64(details[key]); ok {
			sum += n
			any = true
		}
	}
	if !any {
		return
	}
	out.Found = true
	if out.PromptTokens == 0 && sum > 0 {
		out.PromptTokens = sum
	}
}

// applyOpenAIMediaDetailCompletion folds image/audio/text detail leaves into
// CompletionTokens only when CompletionTokens is still zero.
func applyOpenAIMediaDetailCompletion(out *ParsedUsage, details map[string]any) {
	if out == nil || details == nil {
		return
	}
	var sum int64
	var any bool
	for _, key := range []string{"image_tokens", "audio_tokens", "text_tokens"} {
		if n, ok := asInt64(details[key]); ok {
			sum += n
			any = true
		}
	}
	if !any {
		return
	}
	out.Found = true
	if out.CompletionTokens == 0 && sum > 0 {
		out.CompletionTokens = sum
	}
}

func finalizeParsedUsage(out *ParsedUsage) {
	if out == nil || !out.Found {
		if out != nil && out.Source == "" {
			out.Source = usageSourceUnknown
		}
		return
	}

	if out.TotalTokens == 0 && (out.PromptTokens > 0 || out.CompletionTokens > 0) {
		out.TotalTokens = out.PromptTokens + out.CompletionTokens
	}
	// If total is present but still less than prompt+completion (partial upstream),
	// prefer the explicit split sum rather than inventing beyond reported fields.
	// Common after Anthropic cache fields expand prompt beyond a stale total.
	if out.TotalTokens > 0 {
		sum := out.PromptTokens + out.CompletionTokens
		if sum > out.TotalTokens {
			out.TotalTokens = sum
		}
	}

	out.Source = usageSourceUpstream
}
