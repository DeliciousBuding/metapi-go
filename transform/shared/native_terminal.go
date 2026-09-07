package shared

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

// NativeTerminalProtocol identifies the two native wire formats whose otherwise
// complete tool responses sometimes carry the upstream's ordinary text stop.
type NativeTerminalProtocol uint8

const (
	NativeChatCompletions NativeTerminalProtocol = iota + 1
	NativeMessages
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
		if terminalString(doc["object"]) != "chat.completion" {
			return raw
		}
		var choices []json.RawMessage
		if json.Unmarshal(doc["choices"], &choices) != nil {
			return raw
		}
		for i, choiceRaw := range choices {
			choice := terminalObject(choiceRaw)
			if choice == nil || terminalHasError(choice) || terminalString(choice["finish_reason"]) != "stop" {
				continue
			}
			message := terminalObject(choice["message"])
			if message == nil || terminalString(message["role"]) != "assistant" || terminalHasError(message) || terminalPresent(message["refusal"]) {
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
				if terminalString(call["type"]) != "function" || !terminalName(call["id"]) || !terminalName(fn["name"]) || terminalObject([]byte(terminalString(fn["arguments"]))) == nil {
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
		if terminalString(doc["type"]) != "message" || terminalString(doc["role"]) != "assistant" || terminalString(doc["stop_reason"]) != "end_turn" || terminalPresent(doc["stop_sequence"]) {
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
			if terminalString(part["type"]) != "tool_use" {
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
	}
	if !changed {
		return raw
	}
	return terminalMarshal(doc, raw)
}

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
	default:
		return raw
	}
}

func (s *NativeTerminalStream) chatData(doc map[string]json.RawMessage, raw []byte) []byte {
	if terminalString(doc["object"]) != "chat.completion.chunk" {
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
				if !ok || call == nil || (terminalPresent(call["type"]) && terminalString(call["type"]) != "function") {
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
					id := terminalString(call["id"])
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
	switch terminalString(doc["type"]) {
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
		if terminalString(part["type"]) == "tool_use" {
			b.tool = true
			b.call.id = terminalString(part["id"])
			b.call.name = terminalString(part["name"])
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
			if terminalString(delta["type"]) != "input_json_delta" || json.Unmarshal(delta["partial_json"], &text) != nil || !s.addBytes(len(text)) {
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
		if terminalString(delta["stop_reason"]) != "end_turn" || terminalPresent(delta["stop_sequence"]) || !s.started {
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
func terminalString(raw json.RawMessage) string { var s string; _ = json.Unmarshal(raw, &s); return s }
func terminalName(raw json.RawMessage) bool     { return strings.TrimSpace(terminalString(raw)) != "" }
func terminalPresent(raw json.RawMessage) bool {
	return len(raw) != 0 && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) && !bytes.Equal(bytes.TrimSpace(raw), []byte(`""`))
}
func terminalHasError(doc map[string]json.RawMessage) bool {
	return terminalPresent(doc["error"]) || terminalPresent(doc["incomplete_details"]) || terminalString(doc["type"]) == "error"
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
