package proxyhandler

import "encoding/json"

// hasGeneratedSseOutput recognizes output-bearing protocol deltas, not lifecycle,
// role-only, usage, heartbeat, or error events. It observes an event, not a token.
func hasGeneratedSseOutput(ev SseEvent) bool {
	if IsSseErrorEvent(ev) || ev.Data == "" || ev.Data == "[DONE]" {
		return false
	}
	var p map[string]any
	if json.Unmarshal([]byte(ev.Data), &p) != nil {
		return false
	}
	nonempty := func(v any) bool { s, ok := v.(string); return ok && s != "" }
	object := func(v any) map[string]any { m, _ := v.(map[string]any); return m }
	kind, _ := p["type"].(string)
	switch kind {
	case "response.output_text.delta", "response.reasoning_text.delta", "response.reasoning_summary_text.delta", "response.function_call_arguments.delta":
		return nonempty(p["delta"])
	case "content_block_delta":
		d := object(p["delta"])
		return nonempty(d["text"]) || nonempty(d["thinking"]) || nonempty(d["partial_json"])
	case "content_block_start":
		b := object(p["content_block"])
		return b["type"] == "tool_use" && nonempty(b["name"])
	}
	choices, _ := p["choices"].([]any)
	for _, item := range choices {
		choice := object(item)
		d := object(choice["delta"])
		if nonempty(choice["text"]) || nonempty(d["content"]) || nonempty(d["reasoning_content"]) || nonempty(d["reasoning"]) || hasChatPartsOutput(d["content"]) || hasChatPartsOutput(d["images"]) {
			return true
		}
		calls, _ := d["tool_calls"].([]any)
		if call := object(d["function_call"]); nonempty(call["name"]) || nonempty(call["arguments"]) {
			return true
		}
		for _, item := range calls {
			f := object(object(item)["function"])
			if nonempty(f["name"]) || nonempty(f["arguments"]) {
				return true
			}
		}
	}
	candidates, _ := p["candidates"].([]any)
	for _, item := range candidates {
		parts, _ := object(object(item)["content"])["parts"].([]any)
		for _, item := range parts {
			part := object(item)
			if nonempty(part["text"]) || nonempty(object(part["functionCall"])["name"]) || nonempty(object(part["inlineData"])["data"]) || nonempty(object(part["fileData"])["fileUri"]) {
				return true
			}
		}
	}
	return false
}

func hasChatPartsOutput(value any) bool {
	parts, _ := value.([]any)
	for _, value := range parts {
		part, _ := value.(map[string]any)
		if text, _ := part["text"].(string); part["type"] == "text" && text != "" {
			return true
		}
		if image, _ := part["image_url"].(map[string]any); part["type"] == "image_url" {
			if url, _ := image["url"].(string); url != "" {
				return true
			}
		}
	}
	return false
}
