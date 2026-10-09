package messages

import (
	"encoding/json"
	"errors"
	"strings"
)

const ClaudeCodeSystemMessage = "You are Claude Code, Anthropic's official CLI for Claude."

// PrepareClaudeCodeRequest preserves native Messages fields while applying the
// source channel's CLI system/tool contract. Identity and HTTP headers remain
// outside the protocol transformer.
func PrepareClaudeCodeRequest(raw []byte, userID string, prefixTools bool) ([]byte, error) {
	var body map[string]json.RawMessage
	if json.Unmarshal(raw, &body) != nil || body == nil {
		return nil, errors.New("invalid Claude Code request")
	}
	var system []json.RawMessage
	if raw, ok := body["system"]; ok {
		var text string
		if json.Unmarshal(raw, &text) == nil {
			part, _ := json.Marshal(map[string]any{"type": "text", "text": text})
			system = []json.RawMessage{part}
		} else if json.Unmarshal(raw, &system) != nil {
			return nil, errors.New("invalid Claude Code system instructions")
		}
	}
	var first struct {
		Text string `json:"text"`
	}
	if len(system) > 0 {
		_ = json.Unmarshal(system[0], &first)
	}
	if first.Text != ClaudeCodeSystemMessage {
		part, _ := json.Marshal(map[string]any{"type": "text", "text": ClaudeCodeSystemMessage, "cache_control": map[string]any{"type": "ephemeral"}})
		system = append([]json.RawMessage{part}, system...)
	}
	body["system"], _ = json.Marshal(system)
	var metadata map[string]json.RawMessage
	if raw, ok := body["metadata"]; ok && string(raw) != "null" {
		if json.Unmarshal(raw, &metadata) != nil {
			return nil, errors.New("invalid Claude Code metadata")
		}
	}
	if metadata == nil {
		metadata = map[string]json.RawMessage{}
	}
	metadata["user_id"], _ = json.Marshal(userID)
	body["metadata"], _ = json.Marshal(metadata)
	var choice map[string]json.RawMessage
	if json.Unmarshal(body["tool_choice"], &choice) == nil && choice != nil {
		var typ string
		_ = json.Unmarshal(choice["type"], &typ)
		if typ == "tool" || typ == "any" {
			delete(body, "thinking")
		}
		if typ == "tool" && prefixTools {
			prefixClaudeToolName(choice)
			body["tool_choice"], _ = json.Marshal(choice)
		}
	}
	if prefixTools {
		var tools []map[string]json.RawMessage
		if raw, ok := body["tools"]; ok {
			if json.Unmarshal(raw, &tools) != nil {
				return nil, errors.New("invalid Claude Code tools")
			}
			for _, tool := range tools {
				prefixClaudeToolName(tool)
			}
			body["tools"], _ = json.Marshal(tools)
		}
	}
	return json.Marshal(body)
}

func prefixClaudeToolName(tool map[string]json.RawMessage) {
	var name string
	if json.Unmarshal(tool["name"], &name) == nil && name != "" && !strings.HasPrefix(name, "proxy_") {
		tool["name"], _ = json.Marshal("proxy_" + name)
	}
}

// RestoreClaudeCodeTools handles a native JSON response or one framed SSE data
// payload. It only runs when this attempt actually applied the tool prefix.
func RestoreClaudeCodeTools(raw []byte, prefixed bool) []byte {
	if !prefixed {
		return raw
	}
	var body map[string]json.RawMessage
	if json.Unmarshal(raw, &body) != nil || body == nil {
		return raw
	}
	changed := false
	strip := func(raw json.RawMessage) json.RawMessage {
		var item map[string]json.RawMessage
		if json.Unmarshal(raw, &item) != nil {
			return raw
		}
		var typ, name string
		_ = json.Unmarshal(item["type"], &typ)
		_ = json.Unmarshal(item["name"], &name)
		if typ != "tool_use" || !strings.HasPrefix(name, "proxy_") {
			return raw
		}
		item["name"], _ = json.Marshal(strings.TrimPrefix(name, "proxy_"))
		result, _ := json.Marshal(item)
		changed = true
		return result
	}
	if block, ok := body["content_block"]; ok {
		body["content_block"] = strip(block)
	}
	var blocks []json.RawMessage
	if json.Unmarshal(body["content"], &blocks) == nil && blocks != nil {
		for i, block := range blocks {
			blocks[i] = strip(block)
		}
		body["content"], _ = json.Marshal(blocks)
	}
	if !changed {
		return raw
	}
	result, err := json.Marshal(body)
	if err != nil {
		return raw
	}
	return result
}
