package responses

import (
	"bytes"
	"fmt"
)

func (s *ResponsesStream) getItem(obj bridgeObject) (int, *bridgeItem, error) {
	index, err := bridgeIndex(obj["output_index"])
	if err != nil {
		return 0, nil, err
	}
	item := s.items[index]
	if item == nil {
		return 0, nil, fmt.Errorf("Responses/Chat bridge: event references unknown output item")
	}
	if item.done {
		return 0, nil, fmt.Errorf("Responses/Chat bridge: event after output item done")
	}
	if id := obj["item_id"]; id != nil && id != item.id {
		return 0, nil, fmt.Errorf("Responses/Chat bridge: conflicting item ID")
	}
	return index, item, nil
}

func (s *ResponsesStream) getPart(obj bridgeObject) (*bridgeItem, *bridgePart, error) {
	_, item, err := s.getItem(obj)
	if err != nil {
		return nil, nil, err
	}
	indexKey := "content_index"
	if item.kind == "reasoning" {
		indexKey = "summary_index"
	}
	index, err := bridgeIndex(obj[indexKey])
	if err != nil {
		return nil, nil, err
	}
	if item.kind != "message" && item.kind != "reasoning" || index >= len(item.parts) {
		return nil, nil, fmt.Errorf("Responses/Chat bridge: unknown content part")
	}
	return item, item.parts[index], nil
}

func (s *ResponsesStream) addItem(obj bridgeObject, out *bytes.Buffer) error {
	index, err := bridgeIndex(obj["output_index"])
	if err != nil {
		return err
	}
	if len(s.items) >= bridgeItemLimit || s.items[index] != nil {
		return fmt.Errorf("Responses/Chat bridge: too many or duplicate output items")
	}
	raw := bridgeMap(obj["item"])
	if raw == nil {
		return fmt.Errorf("Responses/Chat bridge: invalid output item")
	}
	id, err := bridgeID(raw["id"], "item.id")
	if err != nil {
		return err
	}
	for _, item := range s.items {
		if item.id == id {
			return fmt.Errorf("Responses/Chat bridge: duplicate output item ID")
		}
	}
	item := &bridgeItem{id: id, kind: bridgeString(raw["type"]), started: true}
	switch item.kind {
	case "reasoning":
		if err = bridgeFields(raw, "id", "type", "status", "summary"); err != nil {
			return err
		}
		if raw["summary"] != nil {
			parts, ok := raw["summary"].([]any)
			if !ok || len(parts) != 0 {
				return fmt.Errorf("Responses/Chat bridge: reasoning start must have empty summary")
			}
		}
	case "message":
		if err = bridgeFields(raw, "id", "type", "role", "status", "content"); err != nil {
			return err
		}
		if raw["role"] != "assistant" {
			return fmt.Errorf("Responses/Chat bridge: unsupported output role")
		}
		if raw["content"] != nil {
			parts, ok := raw["content"].([]any)
			if !ok || len(parts) != 0 {
				return fmt.Errorf("Responses/Chat bridge: message start must have empty content")
			}
		}
	case "function_call":
		if err = bridgeFields(raw, "id", "type", "status", "call_id", "name", "arguments", "reasoning_content"); err != nil {
			return err
		}
		item.callID, err = bridgeID(raw["call_id"], "call_id")
		if err != nil {
			return err
		}
		item.name, err = bridgeID(raw["name"], "function.name")
		if err != nil {
			return err
		}
		for _, other := range s.items {
			if other.kind == "function_call" && other.callID == item.callID {
				return fmt.Errorf("Responses/Chat bridge: duplicate tool call ID")
			}
		}
		args, err := bridgeText(raw["arguments"], "function arguments")
		if err != nil {
			return err
		}
		if err = s.retain(len(args)); err != nil {
			return err
		}
		item.arguments.WriteString(args)
		if err := s.callReasoning(raw, item, out); err != nil {
			return err
		}
		s.toolIndices[index] = len(s.toolIndices)
		s.chunk(out, bridgeObject{"tool_calls": []any{bridgeObject{"index": s.toolIndices[index], "id": item.callID, "type": "function", "function": bridgeObject{"name": item.name, "arguments": args}}}}, nil)
	default:
		return fmt.Errorf("Responses/Chat bridge: unsupported output item %q", item.kind)
	}
	if err = s.retain(len(item.id) + len(item.callID) + len(item.name)); err != nil {
		return err
	}
	s.items[index] = item
	s.order = append(s.order, index)
	return nil
}

// Compatible providers can attach the same plain reasoning to multiple call
// snapshots. Emit it once, and reject a snapshot that revises emitted text.
func (s *ResponsesStream) callReasoning(raw bridgeObject, item *bridgeItem, out *bytes.Buffer) error {
	if item.kind != "function_call" || raw["reasoning_content"] == nil {
		return nil
	}
	text, err := bridgeReasoningParts(raw["reasoning_content"], "reasoning_text")
	if err != nil {
		return err
	}
	if item.callReasoning != "" && text != item.callReasoning {
		return fmt.Errorf("Responses/Chat bridge: tool reasoning changed")
	}
	if text != item.callReasoning {
		if err := s.retain(len(text)); err != nil {
			return err
		}
		item.callReasoning = text
		s.chunk(out, bridgeObject{"reasoning_content": text}, nil)
	}
	return nil
}

func bridgePartMatches(raw any, part *bridgePart) bool {
	obj := bridgeMap(raw)
	if obj == nil || obj["type"] != part.kind {
		return false
	}
	if part.kind == "refusal" {
		return bridgeFields(obj, "type", "refusal") == nil && obj["refusal"] == part.text.String()
	}
	if part.kind == "summary_text" {
		text, err := bridgeOptionalString(obj, "text")
		return err == nil && bridgeFields(obj, "type", "text") == nil && text == part.text.String()
	}
	text, err := bridgeContent([]any{obj}, true)
	return err == nil && text == part.text.String()
}

func bridgeItemMatches(raw any, item *bridgeItem) bool {
	obj := bridgeMap(raw)
	if obj == nil || obj["id"] != item.id || obj["type"] != item.kind {
		return false
	}
	if item.kind == "function_call" {
		if bridgeFields(obj, "id", "type", "call_id", "name", "arguments", "status", "reasoning_content") != nil {
			return false
		}
		return obj["call_id"] == item.callID && obj["name"] == item.name && obj["arguments"] == item.arguments.String()
	}
	contentKey := "content"
	if item.kind == "reasoning" {
		contentKey = "summary"
		if bridgeFields(obj, "id", "type", "summary", "status") != nil {
			return false
		}
	} else if bridgeFields(obj, "id", "type", "role", "content", "status") != nil || obj["role"] != "assistant" {
		return false
	}
	parts, ok := obj[contentKey].([]any)
	if !ok || len(parts) != len(item.parts) {
		return false
	}
	for i, part := range item.parts {
		if !bridgePartMatches(parts[i], part) {
			return false
		}
	}
	return true
}
