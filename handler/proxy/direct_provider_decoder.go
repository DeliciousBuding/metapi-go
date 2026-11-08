package proxyhandler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/internal/channelplugin"
	"github.com/deliciousbuding/metapi-go/internal/requestoverride"
	"github.com/tidwall/gjson"
)

// Original provider observations are not derived from decoder output. Pricing,
// admission and settlement consume this snapshot even if a later hook fails.
func observeDirectDriverBody(wire *directProviderWire, body []byte, event bool) (ParsedUsage, bool, error) {
	if wire != nil && wire.DriverImageQuery != nil && wire.DriverImageUsage != nil {
		return *wire.DriverImageUsage, nativeDriverError(body), nil
	}
	usage := ParseUsageFromBody(body)
	if wire != nil && wire.DriverOperation == "deleteTask" && len(body) == 0 {
		// No JSON or usage fact exists. Only the response consumer may accept
		// an empty body, and only for the supplier's native 204 status.
		return usage, false, nil
	}
	if wire != nil && wire.PluginProtocol == "ollama" && !event && (gjson.GetBytes(body, "message").IsObject() || gjson.GetBytes(body, "done").Exists()) {
		// Reuse the native codec's integer/control checks on original bytes,
		// not executable projection output. The response owner still performs
		// the actual downstream conversion after provider decoding.
		converted, err := normalizeDirectOllamaJSON(body, true)
		if err != nil {
			return usage, false, channelplugin.ErrResult
		}
		usage = mergeUsagePreferLater(usage, ParseUsageFromBody(converted))
	}
	if wire != nil && wire.DriverOperation != "" && (wire.DriverOperation == "deleteTask" || gjson.GetBytes(body, "object").String() == "video") {
		id := gjson.GetBytes(body, "id")
		if id.Type == gjson.String && validObservedVideoTaskID(id.String()) {
			usage.upstreamTaskID = id.String()
		} else if wire.DriverOperation == "deleteTask" && id.Exists() {
			// Deletions need no object:"video" marker. A present native ID
			// cannot become missing or be replaced by a vendor alias/decoder.
			usage.invalidForUserBudget = true
			return usage, false, channelplugin.ErrResult
		}
	}
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
	if wire.DriverOperation != "" && nativeDriverError(body) {
		observation.Failed = true
	}
	taskID := usage.upstreamTaskID
	if (wire.DriverOperation != "" || wire.DriverImageQuery != nil) && len(observation.TaskID) > 0 {
		var selected string
		if json.Unmarshal(observation.TaskID, &selected) != nil || !validObservedVideoTaskID(selected) || taskID != "" && taskID != selected {
			usage.invalidForUserBudget = true
			return usage, observation.Failed, channelplugin.ErrResult
		}
		taskID = selected
	}
	if wire.DriverOperation == "deleteTask" && taskID == "" {
		// Nonempty deletion results must establish identity on original bytes.
		// A declared vendor selector may supply it; executable output may not.
		return usage, observation.Failed, channelplugin.ErrResult
	}
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
	usage.upstreamTaskID = taskID
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
	if wire != nil && wire.DriverOperation != "" && wire.DriverOperation != "remix" && wire.DriverTaskID != "" && usage.upstreamTaskID != "" && usage.upstreamTaskID != wire.DriverTaskID {
		return nil, usage, channelplugin.ErrResult
	}
	if wire == nil || wire.Driver == nil {
		return body, usage, nil
	}
	if wire.DriverOperation == "deleteTask" && statusCode == http.StatusNoContent && len(body) == 0 {
		return body, usage, nil
	}
	if wire.DriverOperation == "deleteTask" {
		if deleted := gjson.GetBytes(body, "deleted"); deleted.Exists() && deleted.Type != gjson.True {
			return nil, usage, channelplugin.ErrResult
		}
	}
	if wire.DriverImageQuery != nil && wire.DriverImageUsage != nil {
		return body, usage, nil
	}
	hook := "decodeResponse"
	if wire.DriverOperation != "" {
		hook = "taskResult"
	}
	if event {
		hook = "decodeEvent"
	}
	if !wire.Driver.HasHook(hook) {
		return body, usage, nil
	}
	raw, err := wire.Driver.Call(ctx, hook, channelplugin.DriverInput{Body: string(body), Model: wire.PluginModel, Stream: event, StatusCode: statusCode, TaskID: wire.DriverTaskID, Operation: wire.DriverOperation})
	if err != nil {
		return nil, usage, err
	}
	var decoded struct {
		Body string `json:"body"`
	}
	if requestoverride.Decode(raw, &decoded) != nil || requestoverride.RejectDuplicateJSONKeys([]byte(decoded.Body)) != nil || len(decoded.Body) == 0 || bytes.TrimSpace([]byte(decoded.Body))[0] != '{' || !responseControlAliasesMatch([]byte(`{}`), []byte(decoded.Body)) {
		return nil, usage, channelplugin.ErrResult
	}
	validPayload := nativeDriverPayload(wire.PluginProtocol, []byte(decoded.Body), event)
	if wire.DriverOperation == "deleteTask" {
		validPayload = validDriverTaskDeletion([]byte(decoded.Body))
	}
	if !validPayload {
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
	decodedID := gjson.GetBytes([]byte(decoded.Body), "id").String()
	if wire.DriverOperation == "remix" && (!validObservedVideoTaskID(decodedID) || decodedID == wire.DriverTaskID) {
		return nil, usage, channelplugin.ErrResult
	}
	if wire.DriverOperation != "" && wire.DriverOperation != "remix" && wire.DriverTaskID != "" && decodedID != wire.DriverTaskID {
		return nil, usage, channelplugin.ErrResult
	}
	if wire.DriverOperation != "" && usage.upstreamTaskID != "" && gjson.GetBytes([]byte(decoded.Body), "id").String() != usage.upstreamTaskID {
		return nil, usage, channelplugin.ErrResult
	}
	return []byte(decoded.Body), usage, nil
}

func validDriverTaskDeletion(body []byte) bool {
	var object map[string]json.RawMessage
	if json.Unmarshal(body, &object) != nil || nativeDriverError(body) || gjson.GetBytes(body, "deleted").Type != gjson.True {
		return false
	}
	for key := range object {
		if strings.EqualFold(key, "deleted") && key != "deleted" {
			return false
		}
	}
	id := gjson.GetBytes(body, "id")
	return id.Type == gjson.String && validObservedVideoTaskID(id.String())
}

func validObservedVideoTaskID(id string) bool { return len(id) <= 4096 && validVideoUpstreamID(id) }

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
	if nativeDriverError(body) {
		return true
	}
	kind := gjson.GetBytes(body, "type").String()
	switch protocol {
	case "chat", "completions":
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
	case "ollama":
		return gjson.GetBytes(body, "message").IsObject() || gjson.GetBytes(body, "response").Type == gjson.String || gjson.GetBytes(body, "choices").IsArray()
	case "responsesCompact":
		return gjson.GetBytes(body, "object").String() == "response.compaction" && gjson.GetBytes(body, "output").IsArray()
	case "embeddings", "jinaEmbeddings":
		return gjson.GetBytes(body, "data").IsArray()
	case "geminiEmbeddings":
		return gjson.GetBytes(body, "embedding.values").IsArray() || gjson.GetBytes(body, "embeddings").IsArray()
	case "rerank", "moderations":
		return gjson.GetBytes(body, "results").IsArray()
	case "imageGeneration", "imageEdit", "imageVariation", "modelscopeImageGeneration":
		return gjson.GetBytes(body, "data").IsArray()
	case "audioTranscription", "audioTranslation":
		return gjson.GetBytes(body, "text").Type == gjson.String
	case "decisions":
		valid, _ := decisionsResponseShape(body)
		return valid
	case "systemOne", "alphaSearch":
		return json.Valid(body) && bytes.HasPrefix(bytes.TrimSpace(body), []byte("{"))
	case "video", "seedanceVideo", "zenmuxVideo":
		status := gjson.GetBytes(body, "status").String()
		return gjson.GetBytes(body, "object").String() == "video" && validVideoUpstreamID(gjson.GetBytes(body, "id").String()) && (status == "queued" || status == "in_progress" || status == "completed" || status == "failed")
	default:
		return false
	}
}

func nativeDriverError(body []byte) bool {
	value := gjson.GetBytes(body, "error")
	return value.Exists() && value.Type != gjson.Null && value.Raw != "false" && value.Raw != "0" && value.Raw != `""`
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
