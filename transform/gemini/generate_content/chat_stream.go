package generate_content

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// GeminiStream converts native Gemini SSE events into Chat chunks. It retains
// only identity, terminal state and tool indexes; text is emitted immediately.
type GeminiStream struct {
	id, model             string
	finished, done, tools bool
	toolIndex             int
	err                   error
}

func NewGeminiStream(model string) *GeminiStream {
	return &GeminiStream{id: bridgeID("chatcmpl_"), model: model}
}
func (s *GeminiStream) TransformEvent(frame []byte) (out []byte, err error) {
	if s.err != nil {
		return nil, s.err
	}
	defer func() {
		if err != nil {
			s.err = err
			out = nil
		}
	}()
	data, err := bridgeFrame(frame)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	if s.done {
		return nil, fmt.Errorf("Gemini stream data after terminal")
	}
	if string(data) == "[DONE]" {
		return s.Finish()
	}
	in, err := bridgeObject(data)
	if err != nil {
		return nil, err
	}
	delta, finish, err := geminiDelta(in, true)
	if err != nil {
		return nil, err
	}
	if s.finished && (len(delta) > 0 || finish != "") {
		return nil, fmt.Errorf("Gemini content after finishReason")
	}
	if id, _ := in["responseId"].(string); id != "" {
		if s.toolIndex > 0 && s.id != id {
			return nil, fmt.Errorf("Gemini response identity changed")
		}
		s.id = id
	}
	if model, _ := in["modelVersion"].(string); model != "" {
		s.model = model
	}
	if calls, ok := delta["tool_calls"].([]any); ok {
		s.tools = true
		for _, raw := range calls {
			call := raw.(map[string]any)
			call["index"] = s.toolIndex
			s.toolIndex++
		}
	}
	if finish == "stop" && s.tools {
		finish = "tool_calls"
	}
	chunk := map[string]any{"id": s.id, "object": "chat.completion.chunk", "model": s.model, "choices": []any{}}
	if len(delta) > 0 || finish != "" {
		delta["role"] = "assistant"
		var terminal any
		if finish != "" {
			terminal = finish
			s.finished = true
		}
		chunk["choices"] = []any{map[string]any{"index": 0, "delta": delta, "finish_reason": terminal}}
	}
	if meta, ok := in["usageMetadata"].(map[string]any); ok {
		chunk["usage"] = geminiUsage(meta)
	}
	return bridgeSSE(chunk), nil
}
func (s *GeminiStream) Finish() ([]byte, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.done {
		return nil, nil
	}
	if !s.finished {
		s.err = fmt.Errorf("Gemini stream ended before finishReason")
		return nil, s.err
	}
	s.done = true
	return []byte("data: [DONE]\n\n"), nil
}

// ChatStream converts Chat SSE into native Gemini chunks. Tool arguments are
// bounded and emitted once complete because Gemini has no JSON argument delta.
type ChatStream struct {
	id, model, finish string
	done              bool
	err               error
	tools             map[int]*geminiStreamTool
	order             []int
	argumentBytes     int
	usage             map[string]any
}
type geminiStreamTool struct {
	id, name, signature string
	arguments           strings.Builder
}

const maxBridgeToolBytes = 1 << 20

func NewChatStream(model string) *ChatStream {
	return &ChatStream{model: model, tools: map[int]*geminiStreamTool{}}
}
func (s *ChatStream) TransformEvent(frame []byte) (out []byte, err error) {
	if s.err != nil {
		return nil, s.err
	}
	defer func() {
		if err != nil {
			s.err = err
			out = nil
		}
	}()
	data, err := bridgeFrame(frame)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	if s.done {
		return nil, fmt.Errorf("Chat data after [DONE]")
	}
	if string(data) == "[DONE]" {
		return s.finishResponse()
	}
	in, err := bridgeObject(data)
	if err != nil {
		return nil, err
	}
	if in["error"] != nil {
		return nil, fmt.Errorf("upstream Chat error")
	}
	if id, _ := in["id"].(string); id != "" {
		if s.id != "" && s.id != id {
			return nil, fmt.Errorf("Chat stream id changed")
		}
		s.id = id
	}
	if model, _ := in["model"].(string); model != "" {
		s.model = model
	}
	if usage, ok := in["usage"].(map[string]any); ok {
		s.usage = chatUsage(usage)
	}
	choices, ok := in["choices"].([]any)
	if !ok {
		return nil, fmt.Errorf("Chat stream choices must be an array")
	}
	if len(choices) == 0 {
		if in["usage"] == nil {
			return nil, fmt.Errorf("empty Chat choices without usage")
		}
		return nil, nil
	}
	if len(choices) != 1 {
		return nil, fmt.Errorf("Chat bridge requires one choice")
	}
	choice, ok := choices[0].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid Chat choice")
	}
	delta, ok := choice["delta"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid Chat delta")
	}
	if s.finish != "" {
		return nil, fmt.Errorf("Chat content after finish_reason")
	}
	if err := bridgeKeys(delta, "role", "content", "reasoning_content", "tool_calls", "refusal"); err != nil {
		return nil, err
	}
	if raw, ok := delta["tool_calls"]; ok {
		calls, ok := raw.([]any)
		if !ok {
			return nil, fmt.Errorf("invalid Chat tool delta")
		}
		for _, raw := range calls {
			call, ok := raw.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid Chat tool delta")
			}
			if err := bridgeKeys(call, "index", "id", "type", "function", "provider_specific_fields"); err != nil {
				return nil, err
			}
			_, numeric := call["index"].(json.Number)
			index := bridgeNumber(call["index"])
			if !numeric || index < 0 || index != float64(int(index)) {
				return nil, fmt.Errorf("Chat tool delta requires nonnegative index")
			}
			if len(s.tools) >= 128 && s.tools[int(index)] == nil {
				return nil, fmt.Errorf("Chat tool count exceeds bridge limit")
			}
			tool := s.tools[int(index)]
			if tool == nil {
				tool = &geminiStreamTool{}
				s.tools[int(index)] = tool
				s.order = append(s.order, int(index))
			}
			if id, _ := call["id"].(string); id != "" {
				if tool.id != "" && tool.id != id {
					return nil, fmt.Errorf("Chat tool id changed")
				}
				tool.id = id
			}
			if fn, ok := call["function"].(map[string]any); ok {
				if err := bridgeKeys(fn, "name", "arguments"); err != nil {
					return nil, err
				}
				if name, _ := fn["name"].(string); name != "" {
					if tool.name != "" && tool.name != name {
						return nil, fmt.Errorf("Chat tool name changed")
					}
					tool.name = name
				}
				if args, ok := fn["arguments"].(string); ok {
					s.argumentBytes += len(args)
					if s.argumentBytes > maxBridgeToolBytes {
						return nil, fmt.Errorf("Chat tool arguments exceed bridge limit")
					}
					tool.arguments.WriteString(args)
				}
			}
			if fields, ok := call["provider_specific_fields"].(map[string]any); ok {
				if err := bridgeKeys(fields, "thought_signature"); err != nil {
					return nil, err
				}
				if sig, _ := fields["thought_signature"].(string); sig != "" {
					if tool.signature != "" && tool.signature != sig {
						return nil, fmt.Errorf("Chat tool signature changed")
					}
					tool.signature = sig
				}
			}
		}
		delete(delta, "tool_calls")
	}
	parts, err := chatParts(delta)
	if err != nil {
		return nil, err
	}
	if reason, _ := choice["finish_reason"].(string); reason != "" {
		if _, err := geminiFinish(reason); err != nil {
			return nil, err
		}
		s.finish = reason
	}
	if len(parts) == 0 {
		return nil, nil
	}
	return bridgeSSE(s.envelope(parts, "", nil)), nil
}
func (s *ChatStream) Finish() ([]byte, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.done {
		return nil, nil
	}
	s.err = fmt.Errorf("Chat stream ended before [DONE]")
	return nil, s.err
}
func (s *ChatStream) finishResponse() ([]byte, error) {
	if s.finish == "" {
		return nil, fmt.Errorf("Chat stream ended before finish_reason")
	}
	if s.finish == "tool_calls" && len(s.order) == 0 {
		return nil, fmt.Errorf("Chat tool_calls terminal without tools")
	}
	var parts []any
	ids := map[string]bool{}
	for _, index := range s.order {
		tool := s.tools[index]
		if tool.id == "" || tool.name == "" || ids[tool.id] {
			return nil, fmt.Errorf("incomplete or duplicate Chat tool identity")
		}
		ids[tool.id] = true
		args, err := bridgeObject([]byte(tool.arguments.String()))
		if err != nil {
			return nil, fmt.Errorf("incomplete Chat tool arguments")
		}
		part := map[string]any{"functionCall": map[string]any{"id": tool.id, "name": tool.name, "args": args}}
		if tool.signature != "" {
			part["thoughtSignature"] = tool.signature
		}
		parts = append(parts, part)
	}
	reason, err := geminiFinish(s.finish)
	if err != nil {
		return nil, err
	}
	s.done = true
	return bridgeSSE(s.envelope(parts, reason, s.usage)), nil
}
func (s *ChatStream) envelope(parts []any, finish string, usage map[string]any) map[string]any {
	if parts == nil {
		parts = []any{}
	}
	candidate := map[string]any{"index": 0, "content": map[string]any{"role": "model", "parts": parts}}
	if finish != "" {
		candidate["finishReason"] = finish
	}
	out := map[string]any{"candidates": []any{candidate}, "modelVersion": s.model}
	if s.id != "" {
		out["responseId"] = s.id
	}
	if usage != nil {
		out["usageMetadata"] = usage
	}
	return out
}

func bridgeFrame(frame []byte) ([]byte, error) {
	var data []byte
	for _, line := range bytes.Split(frame, []byte("\n")) {
		line = bytes.TrimSuffix(line, []byte("\r"))
		if bytes.HasPrefix(line, []byte("event:")) && strings.TrimSpace(string(line[6:])) == "error" {
			return nil, fmt.Errorf("upstream SSE error")
		}
		if bytes.HasPrefix(line, []byte("data:")) {
			value := line[5:]
			if len(value) > 0 && value[0] == ' ' {
				value = value[1:]
			}
			if len(data) > 0 {
				data = append(data, '\n')
			}
			data = append(data, value...)
		}
	}
	return data, nil
}
func bridgeSSE(value any) []byte {
	encoded, _ := json.Marshal(value)
	return append(append([]byte("data: "), encoded...), []byte("\n\n")...)
}
