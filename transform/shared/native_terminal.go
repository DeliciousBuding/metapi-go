package shared

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

// NativeTerminalProtocol identifies native wire formats with narrowly bounded
// terminal compatibility repairs.
type NativeTerminalProtocol uint8

const (
	NativeChatCompletions NativeTerminalProtocol = iota + 1
	NativeMessages
	NativeResponses
)

// Keep protocol repair narrow: never invent a terminal event, repair truncated
// arguments, or change length/refusal/error outcomes. RawMessage preserves usage,
// large integer metadata, reasoning, signatures, and provider extensions.
func NormalizeNativeTerminalJSON(raw []byte, protocol NativeTerminalProtocol) []byte {
	doc := terminalObject(raw)
	if doc == nil || terminalHasError(doc) {
		return raw
	}
	changed := false
	switch protocol {
	case NativeChatCompletions:
		if RawString(doc["object"]) != "chat.completion" {
			return raw
		}
		var choices []json.RawMessage
		if json.Unmarshal(doc["choices"], &choices) != nil {
			return raw
		}
		for i, choiceRaw := range choices {
			choice := terminalObject(choiceRaw)
			if choice == nil || terminalHasError(choice) || RawString(choice["finish_reason"]) != "stop" {
				continue
			}
			message := terminalObject(choice["message"])
			if message == nil || RawString(message["role"]) != "assistant" || terminalHasError(message) || terminalPresent(message["refusal"]) {
				continue
			}
			var calls []json.RawMessage
			if json.Unmarshal(message["tool_calls"], &calls) != nil || len(calls) == 0 {
				continue
			}
			ready := true
			for _, callRaw := range calls {
				call := terminalObject(callRaw)
				fn := terminalObject(call["function"])
				if RawString(call["type"]) != "function" || !terminalName(call["id"]) || !terminalName(fn["name"]) || terminalObject([]byte(RawString(fn["arguments"]))) == nil {
					ready = false
					break
				}
			}
			if ready {
				choice["finish_reason"] = json.RawMessage(`"tool_calls"`)
				choices[i] = terminalMarshal(choice, choiceRaw)
				changed = true
			}
		}
		if changed {
			doc["choices"] = terminalMarshal(choices, doc["choices"])
		}
	case NativeMessages:
		if RawString(doc["type"]) != "message" || RawString(doc["role"]) != "assistant" || RawString(doc["stop_reason"]) != "end_turn" || terminalPresent(doc["stop_sequence"]) {
			return raw
		}
		var parts []json.RawMessage
		if json.Unmarshal(doc["content"], &parts) != nil {
			return raw
		}
		tools := 0
		for _, partRaw := range parts {
			part := terminalObject(partRaw)
			if part == nil || terminalHasError(part) {
				return raw
			}
			if RawString(part["type"]) != "tool_use" {
				continue
			}
			if !terminalName(part["id"]) || !terminalName(part["name"]) || terminalObject(part["input"]) == nil {
				return raw
			}
			tools++
		}
		if tools > 0 {
			doc["stop_reason"] = json.RawMessage(`"tool_use"`)
			changed = true
		}
	case NativeResponses:
		// Buffered Responses bodies and the response document inside a
		// response.completed event share one shape. Repair only completed
		// responses that carry the same structurally complete reasoning items
		// the streaming path already accepts.
		if RawString(doc["object"]) != "response" {
			return raw
		}
		changed = normalizeResponsesReasoningOutput(doc)
	}
	if !changed {
		return raw
	}
	return terminalMarshal(doc, raw)
}

// normalizeResponsesReasoningOutput rewrites explicit null statuses on
// structurally complete reasoning items inside a completed response document.
// The response status is authoritative: failed or incomplete responses are
// never repaired, even when one of their items looks complete.
func normalizeResponsesReasoningOutput(response map[string]json.RawMessage) bool {
	if RawString(response["status"]) != "completed" {
		return false
	}
	var items []json.RawMessage
	if json.Unmarshal(response["output"], &items) != nil {
		return false
	}
	changed := false
	for i, rawItem := range items {
		item := terminalObject(rawItem)
		if item == nil || !repairReasoningItem(item) {
			continue
		}
		items[i] = terminalMarshal(item, rawItem)
		changed = true
	}
	if changed {
		response["output"] = terminalMarshal(items, response["output"])
	}
	return changed
}

// repairReasoningItem reports whether item is a complete reasoning item whose
// only defect is an explicit null status. Missing, failed, incomplete or
// malformed items remain authoritative.
func repairReasoningItem(item map[string]json.RawMessage) bool {
	if terminalHasError(item) ||
		RawString(item["type"]) != "reasoning" ||
		!terminalName(item["id"]) ||
		!terminalArray(item["summary"]) ||
		!terminalArray(item["content"]) {
		return false
	}
	status, present := item["status"]
	if !present || !bytes.Equal(bytes.TrimSpace(status), []byte("null")) {
		return false
	}
	item["status"] = json.RawMessage(`"completed"`)
	return true
}

func terminalName(raw json.RawMessage) bool { return strings.TrimSpace(RawString(raw)) != "" }
func terminalArray(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '[' {
		return false
	}
	var values []json.RawMessage
	return json.Unmarshal(raw, &values) == nil
}
func terminalPresent(raw json.RawMessage) bool {
	return len(raw) != 0 && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) && !bytes.Equal(bytes.TrimSpace(raw), []byte(`""`))
}
func terminalHasError(doc map[string]json.RawMessage) bool {
	return terminalPresent(doc["error"]) || terminalPresent(doc["incomplete_details"]) || RawString(doc["type"]) == "error"
}
func terminalMarshal(v any, fallback []byte) []byte {
	out, err := json.Marshal(v)
	if err != nil {
		return fallback
	}
	return out
}

// Reject duplicate keys in each object we interpret instead of turning an
// ambiguous upstream payload into a seemingly valid response by re-encoding it.
func terminalObject(raw []byte) map[string]json.RawMessage {
	d := json.NewDecoder(bytes.NewReader(raw))
	t, err := d.Token()
	if err != nil || t != json.Delim('{') {
		return nil
	}
	obj := make(map[string]json.RawMessage)
	for d.More() {
		k, err := d.Token()
		if err != nil {
			return nil
		}
		key, ok := k.(string)
		if !ok {
			return nil
		}
		if _, duplicate := obj[key]; duplicate {
			return nil
		}
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return nil
		}
		obj[key] = value
	}
	if t, err = d.Token(); err != nil || t != json.Delim('}') {
		return nil
	}
	if _, err = d.Token(); err != io.EOF {
		return nil
	}
	return obj
}
