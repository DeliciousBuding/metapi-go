package relaykitbridge

import (
	"github.com/deliciousbuding/metapi-go/transform/shared"
)

func validateMessagesTerminal(obj object) error {
	if shared.RawString(obj["type"]) != "message" || !validReason(Messages, shared.RawString(obj["stop_reason"])) {
		return terminalError()
	}
	parts, err := array(obj["content"])
	if err != nil {
		return err
	}
	for _, raw := range parts {
		part, err := readObject(raw)
		if err != nil {
			return err
		}
		if shared.RawString(part["type"]) == "tool_use" {
			if shared.RawString(part["id"]) == "" || shared.RawString(part["name"]) == "" {
				return terminalError()
			}
			if _, err := readObject(part["input"]); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *terminalState) observeMessages(obj object) error {
	typ := shared.RawString(obj["type"])
	if typ == "ping" {
		return nil
	}
	if typ == "message_start" {
		if s.started {
			return terminalError()
		}
		s.started = true
		return nil
	}
	if !s.started {
		return terminalError()
	}
	switch typ {
	case "content_block_start":
		if s.logicalEnd {
			return terminalError()
		}
		idx, err := indexOf(obj, "index")
		if err != nil {
			return err
		}
		if s.blocks == nil {
			s.blocks = map[int]*toolState{}
		}
		if s.blocks[idx] != nil {
			return terminalError()
		}
		part, err := readObject(obj["content_block"])
		if err != nil {
			return err
		}
		block := &toolState{tool: shared.RawString(part["type"]) == "tool_use", id: shared.RawString(part["id"]), name: shared.RawString(part["name"]), initial: part["input"]}
		if block.tool {
			if _, err := readObject(block.initial); err != nil {
				return err
			}
		}
		s.blocks[idx] = block
	case "content_block_delta":
		idx, err := indexOf(obj, "index")
		if err != nil {
			return err
		}
		block := s.blocks[idx]
		if block == nil || s.logicalEnd {
			return terminalError()
		}
		delta, err := readObject(obj["delta"])
		if err != nil {
			return err
		}
		if shared.RawString(delta["type"]) == "input_json_delta" {
			if !block.tool {
				return terminalError()
			}
			initial, _ := readObject(block.initial)
			if len(initial) > 0 {
				return terminalError()
			}
			block.args = append(block.args, shared.RawString(delta["partial_json"])...)
		}
	case "content_block_stop":
		idx, err := indexOf(obj, "index")
		if err != nil {
			return err
		}
		block := s.blocks[idx]
		if block == nil {
			return terminalError()
		}
		if block.tool {
			if block.id == "" || block.name == "" {
				return terminalError()
			}
			if len(block.args) > 0 {
				if _, err := readObject(block.args); err != nil {
					return err
				}
			}
		}
		delete(s.blocks, idx)
	case "message_delta":
		delta, err := readObject(obj["delta"])
		if err != nil {
			return err
		}
		if reason := shared.RawString(delta["stop_reason"]); reason != "" {
			if s.logicalEnd || len(s.blocks) > 0 || !validReason(Messages, reason) {
				return terminalError()
			}
			s.logicalEnd = true
		}
	case "message_stop":
		if !s.logicalEnd || len(s.blocks) > 0 {
			return terminalError()
		}
		s.done = true
	default:
		return terminalError()
	}
	return nil
}
