package shared

import (
	"bytes"
	"encoding/json"
	"strings"
)

// NativeTerminalStream owns only bounded evidence for actual tool calls. It
// accepts complete SSE data payloads; framing, timeouts, and byte limits remain
// with the relay. Zero values are valid when Protocol is set before use.
type NativeTerminalStream struct {
	Protocol NativeTerminalProtocol
	choices  map[int]*terminalChoice
	blocks   map[int]*terminalBlock
	bytes    int
	objects  int
	invalid  bool
	started  bool
	ended    bool
}
type terminalChoice struct {
	calls   map[int]*terminalCall
	ended   bool
	invalid bool
}
type terminalCall struct{ id, name, args string }
type terminalBlock struct {
	tool    bool
	closed  bool
	call    terminalCall
	initial json.RawMessage
}

const terminalStateLimit = 1 << 20
const terminalObjectLimit = 128

func (s *NativeTerminalStream) AddData(raw []byte) []byte {
	if s.invalid || s.ended {
		return raw
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("[DONE]")) {
		s.ended = true
		return raw
	}
	doc := terminalObject(raw)
	if doc == nil || terminalHasError(doc) {
		s.invalid = true
		return raw
	}
	switch s.Protocol {
	case NativeChatCompletions:
		return s.chatData(doc, raw)
	case NativeMessages:
		return s.messageData(doc, raw)
	case NativeResponses:
		return s.responsesData(doc, raw)
	default:
		return raw
	}
}

func (s *NativeTerminalStream) responsesData(doc map[string]json.RawMessage, raw []byte) []byte {
	kind := RawString(doc["type"])
	switch kind {
	case "response.completed":
		s.ended = true
		response := terminalObject(doc["response"])
		if response == nil || terminalHasError(response) || !normalizeResponsesReasoningOutput(response) {
			return raw
		}
		doc["response"] = terminalMarshal(response, doc["response"])
		return terminalMarshal(doc, raw)
	case "response.incomplete", "response.failed":
		s.ended = true
		return raw
	}
	if kind != "response.output_item.done" {
		return raw
	}
	if _, ok := terminalIndex(doc["output_index"]); !ok {
		return raw
	}
	item := terminalObject(doc["item"])
	if item == nil || !repairReasoningItem(item) {
		return raw
	}
	doc["item"] = terminalMarshal(item, doc["item"])
	return terminalMarshal(doc, raw)
}

func (s *NativeTerminalStream) chatData(doc map[string]json.RawMessage, raw []byte) []byte {
	if RawString(doc["object"]) != "chat.completion.chunk" {
		return raw
	}
	var choices []json.RawMessage
	if json.Unmarshal(doc["choices"], &choices) != nil {
		return raw
	}
	changed := false
	if s.choices == nil {
		s.choices = make(map[int]*terminalChoice)
	}
	for i, choiceRaw := range choices {
		choice := terminalObject(choiceRaw)
		index, ok := terminalIndex(choice["index"])
		if !ok || choice == nil || terminalHasError(choice) {
			s.invalid = true
			return raw
		}
		c := s.choices[index]
		if c == nil {
			if !s.addObject() {
				return raw
			}
			c = &terminalChoice{calls: make(map[int]*terminalCall)}
			s.choices[index] = c
		}
		if c.ended {
			continue
		}
		delta := terminalObject(choice["delta"])
		if delta == nil || terminalHasError(delta) || terminalPresent(delta["refusal"]) {
			c.invalid = true
			continue
		}
		if terminalPresent(delta["tool_calls"]) {
			var calls []json.RawMessage
			if json.Unmarshal(delta["tool_calls"], &calls) != nil {
				c.invalid = true
				continue
			}
			for _, callRaw := range calls {
				call := terminalObject(callRaw)
				idx, ok := terminalIndex(call["index"])
				if !ok || call == nil || (terminalPresent(call["type"]) && RawString(call["type"]) != "function") {
					c.invalid = true
					break
				}
				t := c.calls[idx]
				if t == nil {
					if !s.addObject() {
						return raw
					}
					t = &terminalCall{}
					c.calls[idx] = t
				}
				if terminalPresent(call["id"]) {
					id := RawString(call["id"])
					if id == "" || (t.id != "" && t.id != id) {
						c.invalid = true
						break
					}
					if t.id == "" {
						if !s.addBytes(len(id)) {
							return raw
						}
						t.id = id
					}
				}
				if terminalPresent(call["function"]) {
					fn := terminalObject(call["function"])
					if fn == nil {
						c.invalid = true
						break
					}
					for key, dst := range map[string]*string{"name": &t.name, "arguments": &t.args} {
						if _, exists := fn[key]; !exists {
							continue
						}
						var value string
						if json.Unmarshal(fn[key], &value) != nil {
							c.invalid = true
							break
						}
						if !s.addBytes(len(value)) {
							return raw
						}
						*dst += value
					}
				}
			}
		}
		if c.invalid {
			continue
		}
		var reason string
		if json.Unmarshal(choice["finish_reason"], &reason) != nil {
			continue
		}
		if bytes.Equal(bytes.TrimSpace(choice["finish_reason"]), []byte(`""`)) {
			choice["finish_reason"] = json.RawMessage(`null`)
			changed = true
		} else if reason != "" {
			c.ended = true
			if reason == "stop" && len(c.calls) > 0 {
				ready := true
				for _, call := range c.calls {
					if !call.ready() {
						ready = false
						break
					}
				}
				if ready {
					choice["finish_reason"] = json.RawMessage(`"tool_calls"`)
					changed = true
				}
			}
		}
		choices[i] = terminalMarshal(choice, choiceRaw)
	}
	if !changed || s.invalid {
		return raw
	}
	doc["choices"] = terminalMarshal(choices, doc["choices"])
	return terminalMarshal(doc, raw)
}

func (s *NativeTerminalStream) messageData(doc map[string]json.RawMessage, raw []byte) []byte {
	switch RawString(doc["type"]) {
	case "message_start":
		if s.started {
			s.invalid = true
			return raw
		}
		s.started = true
		s.blocks = make(map[int]*terminalBlock)
	case "content_block_start":
		idx, ok := terminalIndex(doc["index"])
		part := terminalObject(doc["content_block"])
		if !s.started || !ok || part == nil || s.blocks[idx] != nil || !s.addObject() {
			s.invalid = true
			return raw
		}
		b := &terminalBlock{}
		if RawString(part["type"]) == "tool_use" {
			b.tool = true
			b.call.id = RawString(part["id"])
			b.call.name = RawString(part["name"])
			b.initial = part["input"]
			if b.call.id == "" || b.call.name == "" || terminalObject(b.initial) == nil || !s.addBytes(len(b.call.id)+len(b.call.name)+len(b.initial)) {
				s.invalid = true
				return raw
			}
		}
		s.blocks[idx] = b
	case "content_block_delta":
		idx, ok := terminalIndex(doc["index"])
		delta := terminalObject(doc["delta"])
		b := s.blocks[idx]
		if !ok || b == nil || b.closed || delta == nil {
			s.invalid = true
			return raw
		}
		if b.tool {
			var text string
			if RawString(delta["type"]) != "input_json_delta" || json.Unmarshal(delta["partial_json"], &text) != nil || !s.addBytes(len(text)) {
				s.invalid = true
				return raw
			}
			b.call.args += text
		}
	case "content_block_stop":
		idx, ok := terminalIndex(doc["index"])
		b := s.blocks[idx]
		if !ok || b == nil || b.closed {
			s.invalid = true
			return raw
		}
		b.closed = true
	case "message_delta":
		delta := terminalObject(doc["delta"])
		if delta == nil || !terminalPresent(delta["stop_reason"]) {
			return raw
		}
		s.ended = true
		if RawString(delta["stop_reason"]) != "end_turn" || terminalPresent(delta["stop_sequence"]) || !s.started {
			return raw
		}
		tools := 0
		for _, b := range s.blocks {
			if !b.closed {
				return raw
			}
			if !b.tool {
				continue
			}
			if b.call.args == "" {
				b.call.args = string(b.initial)
			} else if len(terminalObject(b.initial)) != 0 {
				return raw
			}
			if !b.call.ready() {
				return raw
			}
			tools++
		}
		if tools > 0 {
			delta["stop_reason"] = json.RawMessage(`"tool_use"`)
			doc["delta"] = terminalMarshal(delta, doc["delta"])
			return terminalMarshal(doc, raw)
		}
	case "error", "message_stop":
		s.ended = true
	}
	return raw
}

func (c *terminalCall) ready() bool {
	return strings.TrimSpace(c.id) != "" && strings.TrimSpace(c.name) != "" && terminalObject([]byte(c.args)) != nil
}
func (s *NativeTerminalStream) addObject() bool {
	s.objects++
	if s.objects > terminalObjectLimit {
		s.invalid = true
	}
	return !s.invalid
}
func (s *NativeTerminalStream) addBytes(n int) bool {
	s.bytes += n
	if s.bytes > terminalStateLimit {
		s.invalid = true
		s.choices = nil
		s.blocks = nil
	}
	return !s.invalid
}
func terminalIndex(raw json.RawMessage) (int, bool) {
	var n int
	err := json.Unmarshal(raw, &n)
	return n, err == nil && len(raw) > 0 && string(raw) != "null" && n >= 0 && n < terminalObjectLimit
}
