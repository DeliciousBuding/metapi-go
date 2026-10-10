package proxyhandler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// The existing protocol bridge owns SSE framing, input/output byte budgets,
// decompression, cancellation and idle deadlines. This filter only retains
// bounded tool arguments and delayed content, independently for each choice.
type directBailianStream struct {
	choices    map[int]*directBailianChoice
	stateBytes int
	stateLimit int
	callCount  int
	done       bool
	err        error
}

type directBailianChoice struct {
	calls    map[int]*directBailianCall
	pending  [][]byte
	sawTools bool
	finished bool
}

type directBailianCall struct {
	id   string
	args []byte
}

const directBailianObjectLimit = 128

func newDirectBailianStream() *directBailianStream {
	return &directBailianStream{choices: make(map[int]*directBailianChoice), stateLimit: 1 << 20}
}

func (s *directBailianStream) TransformEvent(frame []byte) ([]byte, error) {
	if s.err != nil {
		return nil, s.err
	}
	out, err := s.transform(frame)
	if err != nil {
		s.err = err
	}
	return out, err
}

func (s *directBailianStream) transform(frame []byte) ([]byte, error) {
	boundary, separator := nextSseBoundary(string(frame))
	if boundary < 0 || boundary+separator != len(frame) {
		return nil, fmt.Errorf("Bailian Chat stream requires one complete SSE frame")
	}
	if boundary > maxIncrementalSsePendingBytes {
		return nil, fmt.Errorf("Bailian Chat SSE frame exceeds byte limit")
	}
	event := parseSseBlock(string(frame))
	if event == nil || (event.Data == "" && event.Event != "error") {
		return frame, nil
	}
	if s.done {
		return nil, fmt.Errorf("Bailian Chat stream contains data after [DONE]")
	}
	if event.Event == "error" {
		// Keep the provider's actual error for native clients and accounting;
		// reject any subsequent terminal instead of converting failure to success.
		s.err = fmt.Errorf("Bailian Chat upstream error event")
		return frame, nil
	}
	if strings.TrimSpace(event.Data) == "[DONE]" {
		if len(s.choices) == 0 {
			return nil, fmt.Errorf("Bailian Chat [DONE] arrived without a choice")
		}
		for _, choice := range s.choices {
			if !choice.finished || len(choice.pending) > 0 {
				return nil, fmt.Errorf("Bailian Chat [DONE] arrived before finish_reason")
			}
		}
		s.done = true
		return frame, nil
	}
	doc, err := directBailianObject([]byte(event.Data))
	if err != nil {
		return nil, fmt.Errorf("Bailian Chat stream: %w", err)
	}
	if directBailianPresent(doc["error"]) {
		s.err = fmt.Errorf("Bailian Chat upstream error payload")
		return frame, nil
	}
	var choices []json.RawMessage
	if json.Unmarshal(doc["choices"], &choices) != nil || choices == nil {
		return nil, fmt.Errorf("Bailian Chat stream requires choices")
	}
	changed := false
	var flush [][]byte
	var terminals []json.RawMessage
	seen := make(map[int]bool)
	for i, raw := range choices {
		choice, err := directBailianObject(raw)
		if err != nil {
			return nil, fmt.Errorf("Bailian Chat stream choice: %w", err)
		}
		index, err := directBailianIndex(choice["index"])
		if err != nil || seen[index] {
			return nil, fmt.Errorf("Bailian Chat stream requires unique nonnegative choice indices")
		}
		seen[index] = true
		state := s.choices[index]
		if state == nil {
			if len(s.choices) >= directBailianObjectLimit {
				return nil, fmt.Errorf("Bailian Chat stream exceeds choice limit")
			}
			state = &directBailianChoice{calls: make(map[int]*directBailianCall)}
			s.choices[index] = state
		}
		if state.finished {
			return nil, fmt.Errorf("Bailian Chat choice received data after finish_reason")
		}
		finished := directBailianPresent(choice["finish_reason"])
		if finished && directBailianString(choice["finish_reason"]) == "" {
			return nil, fmt.Errorf("Bailian Chat finish_reason must be a nonempty string")
		}
		delta := map[string]json.RawMessage{}
		if directBailianPresent(choice["delta"]) {
			delta, err = directBailianObject(choice["delta"])
			if err != nil {
				return nil, fmt.Errorf("Bailian Chat stream delta: %w", err)
			}
		}
		deltaChanged, err := s.filterCalls(state, delta)
		if err != nil {
			return nil, err
		}
		if state.sawTools {
			text, err := directBailianHasText(delta["content"])
			if err != nil {
				return nil, err
			}
			if text {
				// Keep the complete content value, including multipart metadata,
				// instead of flattening text/image arrays into an invented string.
				deferred := map[string]json.RawMessage{"index": choice["index"], "delta": directBailianJSON(map[string]json.RawMessage{"content": delta["content"]})}
				if logprobs, ok := choice["logprobs"]; ok {
					deferred["logprobs"] = logprobs
					delete(choice, "logprobs")
				}
				pending := directBailianSyntheticFrame(doc, []json.RawMessage{directBailianJSON(deferred)})
				if err := s.retain(len(pending)); err != nil {
					return nil, err
				}
				state.pending = append(state.pending, pending)
				delete(delta, "content")
				deltaChanged = true
			}
		}
		if deltaChanged {
			choice["delta"] = directBailianJSON(delta)
			changed = true
		}
		if finished {
			state.finished = true
			if len(state.pending) > 0 {
				// A finish chunk may also contain the last tool-argument fragment.
				// Emit that delta first, then delayed content, then the actual finish.
				terminal := map[string]json.RawMessage{"index": choice["index"], "delta": json.RawMessage(`{}`), "finish_reason": choice["finish_reason"]}
				terminals = append(terminals, directBailianJSON(terminal))
				choice["finish_reason"] = json.RawMessage(`null`)
				flush = append(flush, state.pending...)
				for _, pending := range state.pending {
					s.stateBytes -= len(pending)
				}
				state.pending = nil
				changed = true
			}
			for _, call := range state.calls {
				s.stateBytes -= len(call.args) + len(call.id)
			}
			state.calls = nil
		}
		if changed {
			choices[i] = directBailianJSON(choice)
		}
	}
	if !changed {
		return frame, nil
	}
	doc["choices"] = directBailianJSON(choices)
	out := replaceSSEBlockData(frame, directBailianJSON(doc))
	for _, pending := range flush {
		out = append(out, pending...)
	}
	if len(terminals) > 0 {
		out = append(out, directBailianSyntheticFrame(doc, terminals)...)
	}
	return out, nil
}

func (s *directBailianStream) Finish() ([]byte, error) {
	if s.err != nil {
		return nil, s.err
	}
	if !s.done {
		s.err = fmt.Errorf("Bailian Chat stream ended without finish_reason and [DONE]: %w", io.ErrUnexpectedEOF)
		return nil, s.err
	}
	return nil, nil
}

func (s *directBailianStream) filterCalls(state *directBailianChoice, delta map[string]json.RawMessage) (bool, error) {
	if !directBailianPresent(delta["tool_calls"]) {
		return false, nil
	}
	var calls []json.RawMessage
	if json.Unmarshal(delta["tool_calls"], &calls) != nil {
		return false, fmt.Errorf("Bailian Chat tool_calls must be an array")
	}
	changed := false
	for i, raw := range calls {
		call, err := directBailianObject(raw)
		if err != nil {
			return false, fmt.Errorf("Bailian Chat tool call: %w", err)
		}
		index, err := directBailianIndex(call["index"])
		if err != nil {
			return false, fmt.Errorf("Bailian Chat tool call requires a nonnegative index")
		}
		state.sawTools = true
		tracked := state.calls[index]
		if tracked == nil {
			if s.callCount >= directBailianObjectLimit {
				return false, fmt.Errorf("Bailian Chat stream exceeds tool call limit")
			}
			tracked = &directBailianCall{}
			state.calls[index] = tracked
			s.callCount++
		}
		if directBailianPresent(call["id"]) {
			id := directBailianString(call["id"])
			if id == "" || (tracked.id != "" && tracked.id != id) {
				return false, fmt.Errorf("Bailian Chat tool call has an invalid or changed ID")
			}
			if tracked.id == "" {
				if err := s.retain(len(id)); err != nil {
					return false, err
				}
				tracked.id = id
			}
		}
		if !directBailianPresent(call["function"]) {
			continue
		}
		function, err := directBailianObject(call["function"])
		if err != nil {
			return false, fmt.Errorf("Bailian Chat tool function: %w", err)
		}
		if !directBailianPresent(function["arguments"]) {
			continue
		}
		var arguments string
		if err := json.Unmarshal(function["arguments"], &arguments); err != nil {
			return false, fmt.Errorf("Bailian Chat tool arguments must be a string")
		}
		// A nested empty object can be a legitimate next fragment (e.g.
		// '{"filter":' + '{}' + '}'). Only drop '{}' after a complete object.
		prior := bytes.TrimSpace(tracked.args)
		if strings.TrimSpace(arguments) == "{}" && len(prior) > 0 && prior[0] == '{' && json.Valid(prior) {
			function["arguments"] = json.RawMessage(`""`)
			call["function"] = directBailianJSON(function)
			calls[i] = directBailianJSON(call)
			changed = true
			continue
		}
		if err := s.retain(len(arguments)); err != nil {
			return false, err
		}
		tracked.args = append(tracked.args, arguments...)
	}
	if changed {
		delta["tool_calls"] = directBailianJSON(calls)
	}
	return changed, nil
}

func (s *directBailianStream) retain(size int) error {
	if size > s.stateLimit-s.stateBytes {
		return fmt.Errorf("Bailian Chat stream exceeds buffered state byte limit")
	}
	s.stateBytes += size
	return nil
}

func directBailianHasText(raw []byte) (bool, error) {
	if !directBailianPresent(raw) {
		return false, nil
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text != "", nil
	}
	var parts []json.RawMessage
	if json.Unmarshal(raw, &parts) != nil {
		return false, fmt.Errorf("Bailian Chat content must be text or an array")
	}
	for _, part := range parts {
		doc, err := directBailianObject(part)
		if err != nil {
			return false, fmt.Errorf("Bailian Chat content part: %w", err)
		}
		if directBailianString(doc["type"]) == "text" && directBailianString(doc["text"]) != "" {
			return true, nil
		}
	}
	return false, nil
}

func directBailianIndex(raw []byte) (int, error) {
	var index int
	if !directBailianPresent(raw) || json.Unmarshal(raw, &index) != nil || index < 0 {
		return 0, fmt.Errorf("invalid index")
	}
	return index, nil
}

func directBailianJSON(value any) json.RawMessage {
	// Only validated RawMessages and JSON-serializable internal values reach here.
	encoded, _ := json.Marshal(value)
	return encoded
}

func directBailianSyntheticFrame(source map[string]json.RawMessage, choices []json.RawMessage) []byte {
	// Keep identity fields on synthesized deltas; unknown envelope fields and
	// actual usage are preserved exactly once on the original upstream event.
	doc := make(map[string]json.RawMessage, 7)
	for _, key := range []string{"id", "object", "created", "model", "system_fingerprint", "service_tier"} {
		if value, ok := source[key]; ok {
			doc[key] = value
		}
	}
	doc["choices"] = directBailianJSON(choices)
	out := append([]byte("data: "), directBailianJSON(doc)...)
	return append(out, '\n', '\n')
}
