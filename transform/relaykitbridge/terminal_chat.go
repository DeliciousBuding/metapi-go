package relaykitbridge

import (
	"encoding/json"

	"github.com/deliciousbuding/metapi-go/transform/shared"
)

func validateChatTerminal(obj object) error {
	choices, err := array(obj["choices"])
	if err != nil || len(choices) != 1 {
		return terminalError()
	}
	choice, err := readObject(choices[0])
	if err != nil || !validReason(Chat, shared.RawString(choice["finish_reason"])) {
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
}

func (s *terminalState) observeChat(obj object) error {
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
			if id := shared.RawString(call["id"]); id != "" {
				if state.id != "" && state.id != id {
					return terminalError()
				}
				state.id = id
			}
			if typ := shared.RawString(call["type"]); typ != "" && typ != "function" {
				return terminalError()
			}
			fn, err := readObject(call["function"])
			if err != nil {
				return err
			}
			if name := shared.RawString(fn["name"]); name != "" {
				if state.name != "" && state.name != name {
					return terminalError()
				}
				state.name = name
			}
			state.args = append(state.args, shared.RawString(fn["arguments"])...)
		}
	}
	if reason := shared.RawString(choice["finish_reason"]); reason != "" {
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
		if shared.RawString(call["id"]) == "" || shared.RawString(fn["name"]) == "" || shared.RawString(call["type"]) != "function" {
			return terminalError()
		}
		if _, err := readObject([]byte(shared.RawString(fn["arguments"]))); err != nil {
			return err
		}
	}
	return nil
}
