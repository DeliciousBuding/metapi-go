package ollama

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"time"
)

// Stream holds bounded protocol state, not generated content. Each native line
// is immediately emitted as one Chat SSE chunk. Only done:true completes it.
type Stream struct {
	id               string
	model            string
	created          int64
	seenModel        bool
	done             bool
	toolCount        int
	indexes          map[int]bool
	promptTokens     int64
	completionTokens int64
	hasUsage         bool
}

func NewStream(model string) *Stream {
	return &Stream{id: "chatcmpl-ollama-" + rand.Text(), model: model, indexes: make(map[int]bool)}
}

// ToChatResponse converts one non-streaming native response. Partial responses
// and native error envelopes cannot be reported as completed Chat responses.
func ToChatResponse(body []byte) ([]byte, error) {
	s := NewStream("")
	out, err := s.convert(body, false)
	if err != nil {
		return nil, err
	}
	if !s.done {
		return nil, fmt.Errorf("Ollama response is missing done:true")
	}
	return json.Marshal(out)
}

// TransformEvent accepts one raw NDJSON payload, without an SSE data prefix.
func (s *Stream) TransformEvent(line []byte) ([]byte, error) {
	if len(bytes.TrimSpace(line)) == 0 {
		return nil, nil
	}
	out, err := s.convert(line, true)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	frame := append([]byte("data: "), body...)
	frame = append(frame, '\n', '\n')
	if s.done {
		frame = append(frame, []byte("data: [DONE]\n\n")...)
	}
	return frame, nil
}

func (s *Stream) Finish() ([]byte, error) {
	if !s.done {
		return nil, fmt.Errorf("Ollama stream ended before done:true")
	}
	return nil, nil
}

func (s *Stream) convert(line []byte, streaming bool) (map[string]any, error) {
	if s.done {
		return nil, fmt.Errorf("Ollama data after done:true")
	}
	in, err := decodeObject(line)
	if err != nil {
		return nil, fmt.Errorf("invalid Ollama response: %w", err)
	}
	if raw, ok := in["error"]; ok {
		message, err := stringValue(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid Ollama error envelope")
		}
		return nil, fmt.Errorf("Ollama upstream error: %s", message)
	}
	if err = known(in, "model", "created_at", "message", "done", "done_reason", "total_duration", "load_duration", "prompt_eval_count", "prompt_eval_duration", "eval_count", "eval_duration"); err != nil {
		return nil, err
	}
	var done bool
	if raw, ok := in["done"]; !ok || string(raw) == "null" || json.Unmarshal(raw, &done) != nil {
		return nil, fmt.Errorf("Ollama response requires boolean done")
	}
	if raw, ok := in["model"]; ok {
		model, err := stringValue(raw)
		if err != nil || model == "" {
			return nil, fmt.Errorf("invalid Ollama model")
		}
		// Ollama may resolve a supplied alias to a canonical model name. The
		// first native response owns the returned model identity thereafter.
		if s.seenModel && model != s.model {
			return nil, fmt.Errorf("Ollama model changed during stream")
		}
		s.model = model
		s.seenModel = true
	}
	if s.model == "" {
		return nil, fmt.Errorf("Ollama response has no model")
	}
	if raw, ok := in["created_at"]; ok {
		created, err := stringValue(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid Ollama created_at")
		}
		ts, err := time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return nil, fmt.Errorf("invalid Ollama created_at")
		}
		if s.created == 0 {
			s.created = ts.Unix()
		}
	}
	message := map[string]any{"role": "assistant", "content": ""}
	if raw, ok := in["message"]; ok {
		msg, err := decodeObject(raw)
		if err != nil {
			return nil, err
		}
		if err = known(msg, "role", "content", "thinking", "tool_calls"); err != nil {
			return nil, err
		}
		if role, ok := msg["role"]; ok && string(role) != `"assistant"` && string(role) != `""` {
			return nil, fmt.Errorf("invalid Ollama response role")
		}
		if raw, ok := msg["content"]; ok {
			content, err := stringValue(raw)
			if err != nil {
				return nil, fmt.Errorf("invalid Ollama content")
			}
			message["content"] = content
		}
		if raw, ok := msg["thinking"]; ok {
			thinking, err := stringValue(raw)
			if err != nil {
				return nil, fmt.Errorf("invalid Ollama thinking")
			}
			message["reasoning_content"] = thinking
		}
		if raw, ok := msg["tool_calls"]; ok {
			calls, err := s.convertToolCalls(raw, streaming)
			if err != nil {
				return nil, err
			}
			if len(calls) > 0 {
				message["tool_calls"] = calls
			}
		}
	} else if !done || !streaming {
		return nil, fmt.Errorf("Ollama response has no message")
	}
	var finish any
	if raw, ok := in["done_reason"]; ok && string(raw) != `""` {
		if !done {
			return nil, fmt.Errorf("Ollama finish reason before done:true")
		}
		reason, err := stringValue(raw)
		if err != nil {
			return nil, err
		}
		switch reason {
		case "stop", "length":
			finish = reason
		default:
			return nil, fmt.Errorf("unsupported Ollama done_reason %q", reason)
		}
	}
	if done {
		if finish == nil {
			finish = "stop"
		}
		if s.toolCount > 0 && finish == "stop" {
			finish = "tool_calls"
		}
	}
	choice := map[string]any{"index": 0, "finish_reason": finish}
	objectType := "chat.completion"
	if streaming {
		choice["delta"] = message
		objectType = "chat.completion.chunk"
	} else {
		choice["message"] = message
	}
	out := map[string]any{"id": s.id, "object": objectType, "created": s.created, "model": s.model, "choices": []any{choice}}
	prompt, err := nativeCount(in, "prompt_eval_count")
	if err != nil {
		return nil, err
	}
	completion, err := nativeCount(in, "eval_count")
	if err != nil {
		return nil, err
	}
	if _, ok := in["prompt_eval_count"]; ok {
		s.promptTokens = prompt
		s.hasUsage = true
	}
	if _, ok := in["eval_count"]; ok {
		s.completionTokens = completion
		s.hasUsage = true
	}
	if s.promptTokens > math.MaxInt64-s.completionTokens {
		return nil, fmt.Errorf("Ollama usage total overflows")
	}
	if s.hasUsage {
		out["usage"] = map[string]int64{"prompt_tokens": s.promptTokens, "completion_tokens": s.completionTokens, "total_tokens": s.promptTokens + s.completionTokens}
	}
	metrics := object{}
	for _, key := range []string{"total_duration", "load_duration", "prompt_eval_duration", "eval_duration"} {
		if raw, ok := in[key]; ok {
			if _, err := nativeCount(in, key); err != nil {
				return nil, err
			}
			metrics[key] = raw
		}
	}
	if len(metrics) > 0 {
		out["ollama"] = metrics
	}
	s.done = done
	return out, nil
}

func nativeCount(obj object, key string) (int64, error) {
	var value int64
	if raw, ok := obj[key]; ok {
		if string(raw) == "null" || json.Unmarshal(raw, &value) != nil || value < 0 {
			return 0, fmt.Errorf("invalid Ollama %s", key)
		}
	}
	return value, nil
}

func (s *Stream) convertToolCalls(raw json.RawMessage, streaming bool) ([]map[string]any, error) {
	var calls []object
	if string(raw) == "null" || json.Unmarshal(raw, &calls) != nil {
		return nil, fmt.Errorf("invalid Ollama tool_calls")
	}
	if len(calls) > 1024-s.toolCount {
		return nil, fmt.Errorf("too many Ollama tool calls")
	}
	out := make([]map[string]any, 0, len(calls))
	for _, call := range calls {
		if err := known(call, "function"); err != nil {
			return nil, err
		}
		fn, err := decodeObject(call["function"])
		if err != nil {
			return nil, err
		}
		if err = known(fn, "name", "arguments", "index", "description"); err != nil {
			return nil, err
		}
		name, err := stringValue(fn["name"])
		if err != nil || name == "" {
			return nil, fmt.Errorf("Ollama tool name is required")
		}
		args, ok := fn["arguments"]
		if !ok {
			return nil, fmt.Errorf("Ollama tool arguments are required")
		}
		if _, err := decodeObject(args); err != nil {
			return nil, fmt.Errorf("Ollama tool arguments must be an object")
		}
		index := s.toolCount
		if raw, ok := fn["index"]; ok {
			if string(raw) == "null" || json.Unmarshal(raw, &index) != nil || index < 0 || index >= 1024 {
				return nil, fmt.Errorf("invalid Ollama tool index")
			}
		}
		if s.indexes[index] {
			return nil, fmt.Errorf("duplicate complete Ollama tool call index")
		}
		s.indexes[index] = true
		var compact bytes.Buffer
		if err := json.Compact(&compact, args); err != nil {
			return nil, err
		}
		function := map[string]any{"name": name, "arguments": compact.String()}
		if raw, ok := fn["description"]; ok {
			description, err := stringValue(raw)
			if err != nil {
				return nil, err
			}
			function["description"] = description
		}
		value := map[string]any{"id": fmt.Sprintf("call_%s_%d", s.id, index), "type": "function", "function": function}
		if streaming {
			value["index"] = index
		}
		out = append(out, value)
		s.toolCount++
	}
	return out, nil
}
