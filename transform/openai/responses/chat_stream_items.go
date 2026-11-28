package responses

import (
	"bytes"
	"fmt"
)

func (s *ChatStream) retain(n int) error {
	s.retained += n
	if s.retained > BridgeStreamLimit {
		return fmt.Errorf("Responses/Chat bridge: accumulated output exceeds %d bytes", BridgeStreamLimit)
	}
	return nil
}

func (s *ChatStream) itemIndex(item *bridgeItem) int {
	for i, current := range s.items {
		if current == item {
			return i
		}
	}
	return -1
}

func (s *ChatStream) addItem(item *bridgeItem, out *bytes.Buffer) error {
	if len(s.items) >= bridgeItemLimit {
		return fmt.Errorf("Responses/Chat bridge: too many output items")
	}
	s.items = append(s.items, item)
	item.started = true
	s.emit(out, "response.output_item.added", bridgeObject{"output_index": len(s.items) - 1, "item": item.object("in_progress")})
	return nil
}

func (s *ChatStream) text(value string, kind string, out *bytes.Buffer) error {
	if err := s.retain(len(value)); err != nil {
		return err
	}
	target := &s.message
	id, itemKind, partEvent, textEvent, indexKey := "msg_"+s.id, "message", "content_part", kind, "content_index"
	if kind == "summary_text" {
		target = &s.reasoning
		id, itemKind, partEvent, textEvent, indexKey = "rs_"+s.id, "reasoning", "reasoning_summary_part", "reasoning_summary_text", "summary_index"
	}
	if *target == nil {
		*target = &bridgeItem{id: id, kind: itemKind}
		if err := s.addItem(*target, out); err != nil {
			return err
		}
	}
	item := *target
	var part *bridgePart
	index := 0
	for i, current := range item.parts {
		if current.kind == kind {
			part = current
			index = i
			break
		}
	}
	if part == nil {
		part = &bridgePart{kind: kind}
		index = len(item.parts)
		item.parts = append(item.parts, part)
		p := bridgeObject{"type": kind}
		if kind == "refusal" {
			p["refusal"] = ""
		} else {
			p["text"] = ""
			if kind == "output_text" {
				p["annotations"] = []any{}
			}
		}
		s.emit(out, "response."+partEvent+".added", bridgeObject{"item_id": item.id, "output_index": s.itemIndex(item), indexKey: index, "part": p})
	}
	part.text.WriteString(value)
	if value != "" {
		s.emit(out, "response."+textEvent+".delta", bridgeObject{"item_id": item.id, "output_index": s.itemIndex(item), indexKey: index, "delta": value})
	}
	return nil
}

func (s *ChatStream) tool(raw any, out *bytes.Buffer) error {
	call := bridgeMap(raw)
	if call == nil {
		return fmt.Errorf("Responses/Chat bridge: invalid tool delta")
	}
	if err := bridgeFields(call, "index", "id", "type", "function"); err != nil {
		return err
	}
	index, err := bridgeIndex(call["index"])
	if err != nil {
		return err
	}
	if kind := bridgeString(call["type"]); kind != "" && kind != "function" {
		return fmt.Errorf("Responses/Chat bridge: unsupported tool delta")
	}
	fn := bridgeMap(call["function"])
	if call["function"] != nil && fn == nil {
		return fmt.Errorf("Responses/Chat bridge: invalid function delta")
	}
	if err := bridgeFields(fn, "name", "arguments"); err != nil {
		return err
	}
	item := s.tools[index]
	if item == nil {
		if len(s.tools) >= bridgeItemLimit {
			return fmt.Errorf("Responses/Chat bridge: too many tools")
		}
		item = &bridgeItem{kind: "function_call"}
		s.tools[index] = item
	}
	for _, pair := range []struct {
		value   any
		current *string
	}{{call["id"], &item.callID}, {fn["name"], &item.name}} {
		if pair.value != nil {
			value, err := bridgeText(pair.value, "tool identity")
			if err != nil {
				return err
			}
			if err = s.retain(len(value)); err != nil {
				return err
			}
			if err = bridgeIdentity(pair.current, value); err != nil {
				return err
			}
		}
	}
	if item.callID != "" {
		for i, other := range s.tools {
			if i != index && other.callID == item.callID {
				return fmt.Errorf("Responses/Chat bridge: duplicate tool ID")
			}
		}
	}
	args := ""
	if fn["arguments"] != nil {
		args, err = bridgeText(fn["arguments"], "tool arguments delta")
		if err != nil {
			return err
		}
	}
	if err = s.retain(len(args)); err != nil {
		return err
	}
	item.arguments.WriteString(args)
	if !item.started && item.callID != "" && item.name != "" {
		item.id = "fc_" + item.callID
		// Start exposes an empty argument string; accumulated early fragments
		// follow as the first delta, so consumers never receive them twice.
		if len(s.items) >= bridgeItemLimit {
			return fmt.Errorf("Responses/Chat bridge: too many output items")
		}
		s.items = append(s.items, item)
		item.started = true
		obj := item.object("in_progress")
		obj["arguments"] = ""
		s.emit(out, "response.output_item.added", bridgeObject{"output_index": len(s.items) - 1, "item": obj})
		args = item.arguments.String()
	}
	if item.started && args != "" {
		s.emit(out, "response.function_call_arguments.delta", bridgeObject{"item_id": item.id, "output_index": s.itemIndex(item), "delta": args})
	}
	return nil
}
