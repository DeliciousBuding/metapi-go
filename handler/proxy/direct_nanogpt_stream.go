package proxyhandler

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type directNanoGPTStream struct {
	tools      *directNanoGPTTools
	choices    map[int]*nanoGPTStreamChoice
	responseID string
	terminal   []byte
	err        error
}
type nanoGPTStreamChoice struct {
	text      *nanoGPTText
	xmlCalls  []nanoGPTXMLCall
	nativeIDs map[int]string
	maxNative int
	finished  bool
}

func newDirectNanoGPTStream(tools *directNanoGPTTools) protocolEventStream {
	return &directNanoGPTStream{tools: tools, choices: make(map[int]*nanoGPTStreamChoice)}
}

func (s *directNanoGPTStream) TransformEvent(frame []byte) ([]byte, error) {
	if s.err != nil {
		return nil, s.err
	}
	result, err := s.transform(frame)
	if err != nil {
		s.err = err
	}
	return result, err
}

func (s *directNanoGPTStream) transform(frame []byte) ([]byte, error) {
	end, separator := nextSseBoundary(string(frame))
	if end < 0 || end+separator != len(frame) {
		return nil, fmt.Errorf("NanoGPT requires a complete SSE frame")
	}
	event := parseSseBlock(string(frame))
	if event == nil || event.Data == "" && event.Event != "error" {
		return frame, nil
	}
	if s.terminal != nil {
		return nil, fmt.Errorf("NanoGPT data followed the terminal marker")
	}
	if IsSseErrorEvent(*event) {
		s.err = fmt.Errorf("NanoGPT upstream error event")
		return frame, nil
	}
	if strings.TrimSpace(event.Data) == "[DONE]" {
		if len(s.choices) == 0 {
			return nil, fmt.Errorf("NanoGPT terminal arrived without choices")
		}
		for _, choice := range s.choices {
			if !choice.finished {
				return nil, fmt.Errorf("NanoGPT terminal arrived before finish_reason")
			}
		}
		// Release the actual marker only after clean EOF. Neither a disconnect
		// nor a late provider error may be hidden by a fabricated success marker.
		s.terminal = append([]byte(nil), frame...)
		return nil, nil
	}
	object, err := nanoGPTObject([]byte(event.Data))
	if err != nil {
		return nil, err
	}
	var choices []map[string]json.RawMessage
	if json.Unmarshal(object["choices"], &choices) != nil || choices == nil {
		return nil, fmt.Errorf("NanoGPT stream requires Chat choices")
	}
	if len(choices) == 0 && !nanoGPTPresent(object["usage"]) {
		return nil, fmt.Errorf("NanoGPT empty choices require usage")
	}
	if err := nanoGPTChatObject(object, true); err != nil {
		return nil, err
	}
	if id := nanoGPTString(object["id"]); id != "" {
		if s.responseID != "" && s.responseID != id {
			return nil, fmt.Errorf("NanoGPT response identity changed midstream")
		}
		s.responseID = id
	}
	seen := make(map[int]bool)
	for _, choice := range choices {
		index, err := nanoGPTIndex(choice["index"])
		if err != nil || seen[index] {
			return nil, fmt.Errorf("NanoGPT choice indices must be unique nonnegative integers")
		}
		seen[index] = true
		state := s.choices[index]
		if state == nil {
			if len(s.choices) >= directNanoGPTMaxItems {
				return nil, fmt.Errorf("NanoGPT exceeds choice limit")
			}
			state = &nanoGPTStreamChoice{text: newNanoGPTText(s.tools), nativeIDs: make(map[int]string), maxNative: -1}
			s.choices[index] = state
		}
		if state.finished {
			return nil, fmt.Errorf("NanoGPT choice continued after finish_reason")
		}
		delta := make(map[string]json.RawMessage)
		if nanoGPTPresent(choice["delta"]) {
			delta, err = nanoGPTObject(choice["delta"])
			if err != nil {
				return nil, err
			}
		}
		if err := nanoGPTMoveReasoning(delta, "reasoning", "reasoning_content"); err != nil {
			return nil, err
		}
		var native []json.RawMessage
		if nanoGPTPresent(delta["tool_calls"]) {
			if json.Unmarshal(delta["tool_calls"], &native) != nil {
				return nil, fmt.Errorf("invalid NanoGPT native tool deltas")
			}
			for _, raw := range native {
				call, err := nanoGPTObject(raw)
				if err != nil {
					return nil, err
				}
				toolIndex, err := nanoGPTIndex(call["index"])
				if err != nil {
					return nil, err
				}
				if _, exists := state.nativeIDs[toolIndex]; !exists && len(state.nativeIDs) >= directNanoGPTMaxItems {
					return nil, fmt.Errorf("NanoGPT exceeds native tool limit")
				}
				id := nanoGPTString(call["id"])
				if previous := state.nativeIDs[toolIndex]; previous != "" && id != "" && previous != id {
					return nil, fmt.Errorf("NanoGPT native tool identity changed")
				}
				if id != "" {
					state.nativeIDs[toolIndex] = id
				} else if _, exists := state.nativeIDs[toolIndex]; !exists {
					state.nativeIDs[toolIndex] = ""
				}
				state.maxNative = max(state.maxNative, toolIndex)
			}
		}
		var finish string
		if nanoGPTPresent(choice["finish_reason"]) && json.Unmarshal(choice["finish_reason"], &finish) != nil {
			return nil, fmt.Errorf("invalid NanoGPT finish_reason")
		}
		var text string
		hasText := nanoGPTPresent(delta["content"]) && json.Unmarshal(delta["content"], &text) == nil
		if nanoGPTPresent(delta["content"]) && !hasText && len(state.text.buffer) > 0 {
			return nil, fmt.Errorf("NanoGPT mixed non-text content with pending XML")
		}
		remaining, calls, err := state.text.feed(text, finish != "")
		if err != nil {
			return nil, err
		}
		state.xmlCalls = append(state.xmlCalls, calls...)
		if len(state.xmlCalls)+len(state.nativeIDs) > directNanoGPTMaxItems {
			return nil, fmt.Errorf("NanoGPT exceeds tool limit")
		}
		if hasText || remaining != "" {
			delta["content"], _ = json.Marshal(remaining)
		}
		if finish != "" {
			if len(state.xmlCalls) > 0 {
				if finish != "stop" && finish != "tool_calls" {
					return nil, fmt.Errorf("NanoGPT XML tools require a successful choice terminal")
				}
				if state.maxNative > int(^uint(0)>>1)-len(state.xmlCalls) {
					return nil, fmt.Errorf("NanoGPT tool index overflow")
				}
				ids := make(map[string]bool)
				for _, id := range state.nativeIDs {
					if id == "" || ids[id] {
						return nil, fmt.Errorf("invalid NanoGPT native tool identity")
					}
					ids[id] = true
				}
				// Native indices stay untouched. XML calls wait for this choice's
				// finish so later native calls cannot collide with generated indices.
				for ordinal, call := range state.xmlCalls {
					id := nanoGPTCallID(s.tools, s.responseID, index, ordinal, call)
					if ids[id] {
						return nil, fmt.Errorf("NanoGPT XML/native tool identity collision")
					}
					ids[id] = true
					encoded, _ := json.Marshal(map[string]any{"index": state.maxNative + 1 + ordinal, "id": id, "type": "function", "function": map[string]string{"name": call.name, "arguments": call.arguments}})
					native = append(native, encoded)
				}
				delta["tool_calls"], _ = json.Marshal(native)
				choice["finish_reason"] = json.RawMessage(`"tool_calls"`)
				state.xmlCalls = nil
			}
			state.finished = true
		}
		choice["delta"], err = json.Marshal(delta)
		if err != nil {
			return nil, err
		}
	}
	held := 0
	for _, state := range s.choices {
		held += len(state.text.buffer)
		for _, call := range state.xmlCalls {
			held += len(call.name) + len(call.arguments) + 128
		}
		for _, id := range state.nativeIDs {
			held += len(id) + 32
		}
	}
	if held > 1<<20 {
		return nil, fmt.Errorf("NanoGPT stream exceeds buffered state limit")
	}
	object["choices"], err = json.Marshal(choices)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(object)
	if err != nil {
		return nil, err
	}
	return replaceSSEBlockData(frame, encoded), nil
}

func (s *directNanoGPTStream) Finish() ([]byte, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.terminal == nil {
		return nil, fmt.Errorf("NanoGPT stream ended without its terminal marker: %w", io.ErrUnexpectedEOF)
	}
	terminal := s.terminal
	s.terminal = nil
	return terminal, nil
}
