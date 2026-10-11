package relaykitbridge

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

func terminalError() error {
	return fmt.Errorf("invalid or incomplete upstream terminal: %w", ErrConversion)
}

func validReason(format Format, reason string) bool {
	switch format {
	case Chat:
		return reason == "stop" || reason == "length" || reason == "tool_calls" || reason == "content_filter"
	case Messages:
		return reason == "end_turn" || reason == "max_tokens" || reason == "tool_use" || reason == "stop_sequence" || reason == "refusal"
	case Gemini:
		return reason == "STOP" || reason == "MAX_TOKENS" || reason == "SAFETY" || reason == "RECITATION"
	}
	return false
}

func validateJSONTerminal(format Format, value map[string]any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return ErrConversion
	}
	obj, err := readObject(raw)
	if err != nil {
		return err
	}
	switch format {
	case Chat:
		choices, err := array(obj["choices"])
		if err != nil || len(choices) != 1 {
			return terminalError()
		}
		choice, err := readObject(choices[0])
		if err != nil || !validReason(Chat, text(choice["finish_reason"])) {
			return terminalError()
		}
		if err := zeroIndex(choice); err != nil {
			return err
		}
		msg, err := readObject(choice["message"])
		if err != nil {
			return err
		}
		return validateToolCalls(msg["tool_calls"])
	case Messages:
		if text(obj["type"]) != "message" || !validReason(Messages, text(obj["stop_reason"])) {
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
			if text(part["type"]) == "tool_use" {
				if text(part["id"]) == "" || text(part["name"]) == "" {
					return terminalError()
				}
				if _, err := readObject(part["input"]); err != nil {
					return err
				}
			}
		}
	case Responses:
		if text(obj["status"]) != "completed" {
			return terminalError()
		}
		items, err := array(obj["output"])
		if err != nil {
			return err
		}
		for _, raw := range items {
			item, err := readObject(raw)
			if err != nil {
				return err
			}
			if text(item["type"]) == "function_call" {
				if text(item["call_id"]) == "" || text(item["name"]) == "" {
					return terminalError()
				}
				if _, err := readObject([]byte(text(item["arguments"]))); err != nil {
					return err
				}
			}
		}
	case Gemini:
		candidates, err := array(obj["candidates"])
		if err != nil || len(candidates) != 1 {
			return terminalError()
		}
		candidate, err := readObject(candidates[0])
		if err != nil || !validReason(Gemini, text(candidate["finishReason"])) {
			return terminalError()
		}
		if err := validateGeminiCandidate(candidate); err != nil {
			return err
		}
	}
	return nil
}

func validateToolCalls(raw json.RawMessage) error {
	if !present(raw) {
		return nil
	}
	calls, err := array(raw)
	if err != nil {
		return err
	}
	for _, raw := range calls {
		call, err := readObject(raw)
		if err != nil {
			return err
		}
		fn, err := readObject(call["function"])
		if err != nil {
			return err
		}
		if text(call["id"]) == "" || text(fn["name"]) == "" || text(call["type"]) != "function" {
			return terminalError()
		}
		if _, err := readObject([]byte(text(fn["arguments"]))); err != nil {
			return err
		}
	}
	return nil
}

type toolState struct {
	id, name     string
	args         []byte
	initial      json.RawMessage
	tool, closed bool
}
type terminalState struct {
	logicalEnd, done, started bool
	tools                     map[int]*toolState
	blocks                    map[int]*toolState
	responseTools             map[int]*toolState
}

func zeroIndex(obj object) error {
	if !present(obj["index"]) {
		return nil
	}
	i, err := indexOf(obj, "index")
	if err != nil || i != 0 {
		return terminalError()
	}
	return nil
}

func validateGeminiCandidate(candidate object) error {
	if err := zeroIndex(candidate); err != nil {
		return err
	}
	if !present(candidate["content"]) {
		return nil
	}
	content, err := readObject(candidate["content"])
	if err != nil {
		return err
	}
	if !present(content["parts"]) {
		return nil
	}
	parts, err := array(content["parts"])
	if err != nil {
		return err
	}
	for _, raw := range parts {
		part, err := readObject(raw)
		if err != nil {
			return err
		}
		if present(part["functionCall"]) {
			fn, err := readObject(part["functionCall"])
			if err != nil {
				return err
			}
			if text(fn["name"]) == "" {
				return terminalError()
			}
			if _, err := readObject(fn["args"]); err != nil {
				return err
			}
		}
	}
	return nil
}

func indexOf(obj object, key string) (int, error) {
	var value int
	if json.Unmarshal(obj[key], &value) != nil || value < 0 {
		return 0, terminalError()
	}
	return value, nil
}

func (s *terminalState) observe(format Format, obj object) error {
	if s.done {
		if format == Gemini && present(obj["usageMetadata"]) {
			candidates, err := array(obj["candidates"])
			if !present(obj["candidates"]) || err == nil && len(candidates) == 0 {
				return nil
			}
		}
		return terminalError()
	}
	switch format {
	case Chat:
		choices, err := array(obj["choices"])
		if err != nil {
			return err
		}
		if len(choices) == 0 {
			if !present(obj["usage"]) {
				return terminalError()
			}
			return nil
		}
		if len(choices) != 1 || s.logicalEnd {
			return terminalError()
		}
		choice, err := readObject(choices[0])
		if err != nil {
			return err
		}
		if err := zeroIndex(choice); err != nil {
			return err
		}
		delta, err := readObject(choice["delta"])
		if err != nil {
			return err
		}
		if present(delta["tool_calls"]) {
			calls, err := array(delta["tool_calls"])
			if err != nil {
				return err
			}
			if s.tools == nil {
				s.tools = map[int]*toolState{}
			}
			for _, raw := range calls {
				call, err := readObject(raw)
				if err != nil {
					return err
				}
				idx, err := indexOf(call, "index")
				if err != nil {
					return err
				}
				state := s.tools[idx]
				if state == nil {
					state = &toolState{}
					s.tools[idx] = state
				}
				if id := text(call["id"]); id != "" {
					if state.id != "" && state.id != id {
						return terminalError()
					}
					state.id = id
				}
				if typ := text(call["type"]); typ != "" && typ != "function" {
					return terminalError()
				}
				fn, err := readObject(call["function"])
				if err != nil {
					return err
				}
				if name := text(fn["name"]); name != "" {
					if state.name != "" && state.name != name {
						return terminalError()
					}
					state.name = name
				}
				state.args = append(state.args, text(fn["arguments"])...)
			}
		}
		if reason := text(choice["finish_reason"]); reason != "" {
			if !validReason(Chat, reason) {
				return terminalError()
			}
			for _, tool := range s.tools {
				if tool.id == "" || tool.name == "" {
					return terminalError()
				}
				if _, err := readObject(tool.args); err != nil {
					return err
				}
			}
			s.logicalEnd = true
		}
	case Messages:
		typ := text(obj["type"])
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
			block := &toolState{tool: text(part["type"]) == "tool_use", id: text(part["id"]), name: text(part["name"]), initial: part["input"]}
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
			if text(delta["type"]) == "input_json_delta" {
				if !block.tool {
					return terminalError()
				}
				initial, _ := readObject(block.initial)
				if len(initial) > 0 {
					return terminalError()
				}
				block.args = append(block.args, text(delta["partial_json"])...)
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
			if reason := text(delta["stop_reason"]); reason != "" {
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
	case Responses:
		switch text(obj["type"]) {
		case "response.output_item.added", "response.output_item.done":
			item, err := readObject(obj["item"])
			if err != nil {
				return err
			}
			if text(item["type"]) != "function_call" {
				break
			}
			idx, err := indexOf(obj, "output_index")
			if err != nil {
				return err
			}
			if s.responseTools == nil {
				s.responseTools = map[int]*toolState{}
			}
			tool := s.responseTools[idx]
			if tool == nil {
				tool = &toolState{id: text(item["call_id"]), name: text(item["name"])}
				s.responseTools[idx] = tool
			}
			if tool.closed || tool.id == "" || tool.name == "" || tool.id != text(item["call_id"]) || tool.name != text(item["name"]) {
				return terminalError()
			}
			args := []byte(text(item["arguments"]))
			if text(obj["type"]) == "response.output_item.done" {
				if _, err := readObject(args); err != nil {
					return err
				}
				if len(tool.args) > 0 && !sameJSON(tool.args, args) {
					return terminalError()
				}
				tool.closed = true
			}
			tool.args = args
		case "response.function_call_arguments.delta", "response.function_call_arguments.done":
			idx, err := indexOf(obj, "output_index")
			if err != nil {
				return err
			}
			tool := s.responseTools[idx]
			if tool == nil || tool.closed {
				return terminalError()
			}
			if text(obj["type"]) == "response.function_call_arguments.delta" {
				tool.args = append(tool.args, text(obj["delta"])...)
			} else {
				args := []byte(text(obj["arguments"]))
				if _, err := readObject(args); err != nil {
					return err
				}
				if len(tool.args) > 0 && !sameJSON(tool.args, args) {
					return terminalError()
				}
				tool.args = args
			}
		case "response.completed":
			for _, tool := range s.responseTools {
				if !tool.closed {
					return terminalError()
				}
			}
			var response map[string]any
			if (numberCodec{}).Unmarshal(obj["response"], &response) != nil {
				return terminalError()
			}
			if err := validateJSONTerminal(Responses, response); err != nil {
				return err
			}
			s.logicalEnd = true
			s.done = true
		case "response.failed", "response.incomplete", "error":
			return terminalError()
		default:
			if !strings.HasPrefix(text(obj["type"]), "response.") {
				return terminalError()
			}
		}
	case Gemini:
		if !present(obj["candidates"]) && present(obj["usageMetadata"]) {
			return nil
		}
		candidates, err := array(obj["candidates"])
		if err != nil {
			return err
		}
		if len(candidates) == 0 && present(obj["usageMetadata"]) {
			return nil
		}
		if len(candidates) != 1 {
			return terminalError()
		}
		candidate, err := readObject(candidates[0])
		if err != nil {
			return err
		}
		if err := validateGeminiCandidate(candidate); err != nil {
			return err
		}
		if reason := text(candidate["finishReason"]); reason != "" {
			if !validReason(Gemini, reason) {
				return terminalError()
			}
			s.logicalEnd = true
			s.done = true
		}
	}
	return nil
}

func sameJSON(a, b []byte) bool {
	var aa, bb any
	if (numberCodec{}).Unmarshal(a, &aa) != nil || (numberCodec{}).Unmarshal(b, &bb) != nil {
		return false
	}
	return reflect.DeepEqual(aa, bb)
}
