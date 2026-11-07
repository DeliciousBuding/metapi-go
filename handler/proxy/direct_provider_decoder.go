package proxyhandler

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"github.com/deliciousbuding/metapi-go/internal/channelplugin"
	"github.com/deliciousbuding/metapi-go/internal/requestoverride"
	"github.com/tidwall/gjson"
)

// Original provider observations are not derived from decoder output. Pricing,
// admission and settlement consume this snapshot even if a later hook fails.
func observeDirectDriverBody(wire *directProviderWire, body []byte, event bool) (ParsedUsage, bool, error) {
	usage := ParseUsageFromBody(body)
	if wire == nil || wire.Driver == nil {
		return usage, false, nil
	}
	observation, err := wire.Driver.Observe(body, event)
	if err != nil {
		if requestoverride.RejectDuplicateJSONKeys(body) != nil {
			usage = ParsedUsage{Source: usageSourceUnknown, invalidForUserBudget: true}
		} else if !usage.Found {
			usage.invalidForUserBudget = true
		}
		return usage, false, err
	}
	mapped := ParseUsageFromBody(observation.Usage)
	if observation.Failed {
		usage.UpstreamReportedModel = nil
	}
	if !observation.Failed && gjson.GetBytes(body, "error").Type == gjson.Null && len(observation.Model) > 0 {
		var model any
		if json.Unmarshal(observation.Model, &model) == nil {
			mapped.UpstreamReportedModel = reportedModelString(model)
		}
	}
	// A declared alias may add missing facts, not replace a contradictory
	// already-present native counter or reported model.
	for _, pair := range [][2]int64{{usage.PromptTokens, mapped.PromptTokens}, {usage.CompletionTokens, mapped.CompletionTokens}, {usage.CacheReadTokens, mapped.CacheReadTokens}, {usage.CacheCreationTokens, mapped.CacheCreationTokens}} {
		if pair[0] > 0 && pair[1] > 0 && pair[0] != pair[1] {
			usage.invalidForUserBudget = true
			return usage, observation.Failed, channelplugin.ErrResult
		}
	}
	if usage.UpstreamReportedModel != nil && mapped.UpstreamReportedModel != nil && *usage.UpstreamReportedModel != *mapped.UpstreamReportedModel {
		return usage, observation.Failed, channelplugin.ErrResult
	}
	usage = mergeUsagePreferLater(usage, mapped)
	if usage.invalidForUserBudget {
		return usage, observation.Failed, channelplugin.ErrResult
	}
	return usage, observation.Failed, nil
}

func decodeDirectDriverPayload(ctx context.Context, wire *directProviderWire, body []byte, event bool, statusCode int) ([]byte, ParsedUsage, error) {
	usage, failed, err := observeDirectDriverBody(wire, body, event)
	if err != nil || failed {
		return nil, usage, channelplugin.ErrResult
	}
	if wire == nil || wire.Driver == nil {
		return body, usage, nil
	}
	hook := "decodeResponse"
	if event {
		hook = "decodeEvent"
	}
	if !wire.Driver.HasHook(hook) {
		return body, usage, nil
	}
	raw, err := wire.Driver.Call(ctx, hook, channelplugin.DriverInput{Body: string(body), Model: wire.PluginModel, Stream: event, StatusCode: statusCode})
	if err != nil {
		return nil, usage, err
	}
	var decoded struct {
		Body string `json:"body"`
	}
	if requestoverride.Decode(raw, &decoded) != nil || requestoverride.RejectDuplicateJSONKeys([]byte(decoded.Body)) != nil || len(decoded.Body) == 0 || bytes.TrimSpace([]byte(decoded.Body))[0] != '{' || !responseControlAliasesMatch([]byte(`{}`), []byte(decoded.Body)) || !nativeDriverPayload(wire.PluginProtocol, []byte(decoded.Body), event) {
		return nil, usage, channelplugin.ErrResult
	}
	// Vendor fields named type/status/id are not automatically native controls.
	// Existing native envelopes retain the same stronger immutable-field rule.
	if nativeDriverPayload(wire.PluginProtocol, body, event) && !channelPluginControlsMatch(wire.PluginProtocol, body, []byte(decoded.Body), true) {
		return nil, usage, channelplugin.ErrResult
	}
	nativeUsage := ParseUsageFromBody([]byte(decoded.Body))
	if nativeUsage.invalidForUserBudget || nativeUsage.Found && (!usage.Found || !sameDriverObservedUsage(usage, nativeUsage)) {
		return nil, usage, channelplugin.ErrResult
	}
	if usage.UpstreamReportedModel != nil && nativeUsage.UpstreamReportedModel != nil && *usage.UpstreamReportedModel != *nativeUsage.UpstreamReportedModel {
		return nil, usage, channelplugin.ErrResult
	}
	return []byte(decoded.Body), usage, nil
}

// Inspect protocol-owned containers only. Tool arguments/user objects retain
// their own keys; unchanged legacy aliases are not rewritten by wire hooks.
func responseControlAliasesMatch(before, after []byte) bool {
	fields := []string{"model", "type", "status", "error", "usage", "id", "object", "response", "message", "choices", "candidates", "output", "usageMetadata", "modelVersion", "stop_reason", "finish_reason", "incomplete_details", "item", "part", "output_index", "content_index", "item_id", "call_id", "sequence_number", "previous_response_id", "prompt_tokens", "completion_tokens", "total_tokens", "input_tokens", "output_tokens", "cache_read_input_tokens", "cache_creation_input_tokens", "prompt_tokens_details", "input_tokens_details", "completion_tokens_details", "output_tokens_details", "cached_tokens", "reasoning_tokens", "promptTokenCount", "candidatesTokenCount", "totalTokenCount", "thoughtsTokenCount"}
	for _, path := range []string{"", "response", "message", "usage", "usageMetadata", "response.usage", "message.usage", "response.usageMetadata", "usage.prompt_tokens_details", "usage.input_tokens_details", "usage.completion_tokens_details", "usage.output_tokens_details"} {
		previous, next := before, after
		if path != "" {
			previous = []byte(gjson.GetBytes(before, path).Raw)
			next = []byte(gjson.GetBytes(after, path).Raw)
		}
		var oldFields, newFields map[string]json.RawMessage
		_ = json.Unmarshal(previous, &oldFields)
		if json.Unmarshal(next, &newFields) != nil {
			continue
		}
		for key, value := range newFields {
			for _, field := range fields {
				if key != field && strings.EqualFold(key, field) && !bytes.Equal(value, oldFields[key]) {
					return false
				}
			}
		}
	}
	return true
}

func nativeDriverPayload(protocol string, body []byte, event bool) bool {
	if value := gjson.GetBytes(body, "error"); value.Exists() && value.Type != gjson.Null && value.Raw != "false" && value.Raw != "0" && value.Raw != `""` {
		return true
	}
	kind := gjson.GetBytes(body, "type").String()
	switch protocol {
	case "chat":
		return gjson.GetBytes(body, "choices").IsArray()
	case "responses", "responsesWebsocket":
		if event {
			return kind == "error" || strings.HasPrefix(kind, "response.") || kind == "rate_limits.updated"
		}
		return gjson.GetBytes(body, "output").IsArray()
	case "messages":
		if event {
			return kind == "ping" || kind == "error" || strings.HasPrefix(kind, "message_") || strings.HasPrefix(kind, "content_block_")
		}
		return kind == "message" && gjson.GetBytes(body, "content").IsArray()
	case "gemini":
		return gjson.GetBytes(body, "candidates").IsArray() || gjson.GetBytes(body, "usageMetadata").IsObject() || gjson.GetBytes(body, "promptFeedback").IsObject()
	case "responsesCompact":
		return gjson.GetBytes(body, "object").String() == "response.compaction" && gjson.GetBytes(body, "output").IsArray()
	default:
		return false
	}
}

func decodedDriverSSEEvent(wire *directProviderWire, body []byte) (string, error) {
	kind := "message"
	if wire.PluginProtocol == "responses" || wire.PluginProtocol == "responsesWebsocket" || wire.PluginProtocol == "messages" {
		kind = gjson.GetBytes(body, "type").String()
	}
	if kind == "" || len(kind) > 128 || strings.ContainsAny(kind, "\r\n\x00") {
		return "", channelplugin.ErrResult
	}
	return kind, nil
}

func sameDriverObservedUsage(a, b ParsedUsage) bool {
	return a.PromptTokens == b.PromptTokens && a.CompletionTokens == b.CompletionTokens && a.TotalTokens == b.TotalTokens && a.CacheReadTokens == b.CacheReadTokens && a.CacheCreationTokens == b.CacheCreationTokens && a.ReasoningTokens == b.ReasoningTokens
}

// Framing readers call onEvent before conversion. Their existing analyzer owns
// cumulative/partial merging and keeps observations after decoder failure.
func observeDirectDriverStream(analyzer *incrementalSseAnalyzer, wire *directProviderWire) {
	if analyzer == nil || wire == nil || wire.Driver == nil {
		return
	}
	analyzer.onEvent = func(event SseEvent) {
		if event.Data == "" || event.Data == "[DONE]" {
			return
		}
		usage, _, err := observeDirectDriverBody(wire, []byte(event.Data), true)
		analyzer.usage = mergeUsagePreferLater(analyzer.usage, usage)
		if err != nil {
			analyzer.usage.invalidForUserBudget = true
		}
	}
}
