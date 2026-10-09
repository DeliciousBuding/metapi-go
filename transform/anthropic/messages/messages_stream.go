package messages

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// MessagesStream owns one Messages-to-Chat stream. TransformEvent consumes one
// complete SSE frame, including its empty-line delimiter, and returns complete
// Chat SSE frames. It is not safe for concurrent calls. An error is sticky.
type MessagesStream struct {
	id, model     string
	created       int64
	started, done bool
	err           error
	blocks        map[int64]*messagesBlock
	toolIDs       map[string]bool
	toolCount     int
	usage         rawObject
	terminal      rawObject
}

type messagesBlock struct {
	kind      string
	closed    bool
	toolIndex int
	initial   json.RawMessage
	arguments strings.Builder
	hasDelta  bool
}

// NewMessagesStream uses model only if the upstream message_start omits it.
func NewMessagesStream(model string) *MessagesStream {
	return &MessagesStream{model: model, created: time.Now().Unix(), blocks: make(map[int64]*messagesBlock), toolIDs: make(map[string]bool), usage: make(rawObject)}
}

// TransformEvent emits a finish chunk and [DONE] only for a real message_stop
// preceded by a valid stop reason and completed, valid tool argument objects.
func (s *MessagesStream) TransformEvent(frame []byte) (result []byte, err error) {
	if s.err != nil {
		return nil, s.err
	}
	defer func() {
		if err != nil {
			s.err, result = err, nil
		}
	}()
	event, data, hasData, err := parseFrame(frame)
	if err != nil {
		return nil, err
	}
	if event == "error" {
		return nil, invalid("Messages SSE", "contains an upstream error event")
	}
	if !hasData {
		return nil, nil
	}
	value, err := object([]byte(data), "Messages SSE data")
	if err != nil {
		return nil, err
	}
	if err := rejectUpstreamError(value, "Messages SSE data"); err != nil {
		return nil, err
	}
	typ, err := text(value["type"], "Messages SSE type")
	if err != nil {
		return nil, err
	}
	if event != "" && event != typ {
		return nil, invalid("Messages SSE event", "disagrees with the payload type")
	}
	if typ == "ping" {
		return nil, allowOnly(value, "Messages ping", "type")
	}
	if s.done {
		return nil, invalid("Messages SSE", "contains data after message_stop")
	}
	if typ == "message_start" {
		return s.start(value)
	}
	if !s.started {
		return nil, invalid("Messages SSE", "requires message_start before content")
	}
	switch typ {
	case "content_block_start", "content_block_delta", "content_block_stop":
		if s.terminal != nil {
			return nil, invalid("Messages SSE", "contains content after stop_reason")
		}
		return s.content(typ, value)
	case "message_delta":
		if err := allowOnly(value, "Messages message_delta", "type", "delta", "usage"); err != nil {
			return nil, err
		}
		delta, err := object(value["delta"], "Messages message_delta.delta")
		if err != nil {
			return nil, err
		}
		if err := allowOnly(delta, "Messages message_delta.delta", "stop_reason", "stop_sequence"); err != nil {
			return nil, err
		}
		if !absent(delta["stop_reason"]) {
			if err := s.closedBlocks(); err != nil {
				return nil, err
			}
			if _, err := chatStopReason(delta, s.toolCount); err != nil {
				return nil, err
			}
			if s.terminal != nil {
				return nil, invalid("Messages stop_reason", "was already received")
			}
			s.terminal = delta
		} else if !absent(delta["stop_sequence"]) {
			return nil, invalid("Messages stop_sequence", "requires a stop reason")
		}
		return nil, s.mergeUsage(value["usage"])
	case "message_stop":
		if err := allowOnly(value, "Messages message_stop", "type"); err != nil {
			return nil, err
		}
		if s.terminal == nil {
			return nil, invalid("Messages message_stop", "requires a valid stop_reason")
		}
		if err := s.closedBlocks(); err != nil {
			return nil, err
		}
		reason, err := chatStopReason(s.terminal, s.toolCount)
		if err != nil {
			return nil, err
		}
		usage, err := json.Marshal(s.usage)
		if err != nil {
			return nil, err
		}
		counts, err := messagesUsage(usage)
		if err != nil {
			return nil, err
		}
		result, err := s.chunk(wireObject{}, reason, counts)
		if err != nil {
			return nil, err
		}
		s.done = true
		return append(result, []byte("data: [DONE]\n\n")...), nil
	default:
		return nil, unsupported("Messages SSE event type")
	}
}

// Finish validates transport EOF. Truncation and upstream errors never become
// successful Chat terminators. After message_stop it is an idempotent no-op.
func (s *MessagesStream) Finish() ([]byte, error) {
	if s.err == nil && !s.done {
		s.err = invalid("Messages SSE", "ended without a validated message_stop")
	}
	return nil, s.err
}

func (s *MessagesStream) start(value rawObject) ([]byte, error) {
	if s.started {
		return nil, invalid("Messages message_start", "duplicates the message")
	}
	if err := allowOnly(value, "Messages message_start", "type", "message"); err != nil {
		return nil, err
	}
	message, err := object(value["message"], "Messages message_start.message")
	if err != nil {
		return nil, err
	}
	id, model, err := checkMessagesEnvelope(message, true)
	if err != nil {
		return nil, err
	}
	if model != "" {
		s.model = model
	}
	if strings.TrimSpace(s.model) == "" {
		return nil, invalid("Messages model", "must be nonempty")
	}
	blocks, err := array(message["content"], "Messages message_start.content")
	if err != nil || len(blocks) != 0 || !absent(message["stop_reason"]) || !absent(message["stop_sequence"]) {
		return nil, invalid("Messages message_start", "must have empty content and no stop reason")
	}
	if err := s.mergeUsage(message["usage"]); err != nil {
		return nil, err
	}
	s.id, s.started = id, true
	return s.chunk(wireObject{"role": "assistant", "content": ""}, "", nil)
}

func (s *MessagesStream) content(kind string, value rawObject) ([]byte, error) {
	fields := []string{"type", "index"}
	if kind == "content_block_start" {
		fields = append(fields, "content_block")
	} else if kind == "content_block_delta" {
		fields = append(fields, "delta")
	}
	if err := allowOnly(value, "Messages "+kind, fields...); err != nil {
		return nil, err
	}
	index, err := nonnegativeInt(value["index"], "Messages block.index")
	if err != nil {
		return nil, err
	}
	block := s.blocks[index]
	if kind == "content_block_start" {
		if block != nil || index != int64(len(s.blocks)) {
			return nil, invalid("Messages block.index", "must be the next contiguous block index")
		}
		if err := s.closedBlocks(); err != nil {
			return nil, err
		}
		raw, err := object(value["content_block"], "Messages content_block")
		if err != nil {
			return nil, err
		}
		typ, err := checkMessagesBlock(raw, "Messages content_block")
		if err != nil {
			return nil, err
		}
		block = &messagesBlock{kind: typ}
		s.blocks[index] = block
		if typ == "text" {
			if s.toolCount != 0 {
				return nil, invalid("Messages content_block", "cannot reorder text after tool calls through Chat")
			}
			value, err := text(raw["text"], "Messages content_block.text")
			if err != nil {
				return nil, err
			}
			return s.chunk(wireObject{"content": value}, "", nil)
		}
		id, name, err := messagesToolIdentity(raw, "Messages content_block")
		if err != nil {
			return nil, err
		}
		if s.toolIDs[id] {
			return nil, invalid("Messages content_block.id", "duplicates a tool call ID")
		}
		if _, err := object(raw["input"], "Messages content_block.input"); err != nil {
			return nil, err
		}
		block.initial, block.toolIndex = raw["input"], s.toolCount
		s.toolIDs[id], s.toolCount = true, s.toolCount+1
		return s.chunk(wireObject{"tool_calls": []wireObject{{"index": block.toolIndex, "id": id, "type": "function", "function": wireObject{"name": name, "arguments": ""}}}}, "", nil)
	}
	if block == nil || block.closed {
		return nil, invalid("Messages block.index", "must reference an open block")
	}
	if kind == "content_block_stop" {
		block.closed = true
		if block.kind == "tool_use" {
			arguments := block.arguments.String()
			if !block.hasDelta {
				arguments = string(block.initial)
			}
			if _, err := object([]byte(arguments), "Messages tool arguments"); err != nil {
				return nil, err
			}
			if !block.hasDelta {
				return s.toolArguments(block, arguments)
			}
		}
		return nil, nil
	}
	delta, err := object(value["delta"], "Messages content delta")
	if err != nil {
		return nil, err
	}
	typ, err := text(delta["type"], "Messages content delta.type")
	if err != nil {
		return nil, err
	}
	if block.kind == "text" && typ == "text_delta" {
		if err := allowOnly(delta, "Messages text delta", "type", "text"); err != nil {
			return nil, err
		}
		if s.toolCount != 0 {
			return nil, invalid("Messages text delta", "cannot reorder text after tool calls through Chat")
		}
		text, err := text(delta["text"], "Messages text delta.text")
		if err != nil {
			return nil, err
		}
		return s.chunk(wireObject{"content": text}, "", nil)
	}
	if block.kind != "tool_use" || typ != "input_json_delta" {
		return nil, unsupported("Messages content delta.type")
	}
	if err := allowOnly(delta, "Messages input delta", "type", "partial_json"); err != nil {
		return nil, err
	}
	initial, _ := object(block.initial, "Messages tool input")
	if len(initial) != 0 {
		return nil, invalid("Messages tool input", "cannot combine initial arguments with JSON deltas")
	}
	part, err := text(delta["partial_json"], "Messages input delta.partial_json")
	if err != nil {
		return nil, err
	}
	block.hasDelta = true
	block.arguments.WriteString(part)
	return s.toolArguments(block, part)
}

func (s *MessagesStream) toolArguments(block *messagesBlock, arguments string) ([]byte, error) {
	return s.chunk(wireObject{"tool_calls": []wireObject{{"index": block.toolIndex, "function": wireObject{"arguments": arguments}}}}, "", nil)
}

func (s *MessagesStream) closedBlocks() error {
	for index, block := range s.blocks {
		if !block.closed {
			return invalid(fmt.Sprintf("Messages block[%d]", index), "did not receive content_block_stop")
		}
	}
	return nil
}

func (s *MessagesStream) mergeUsage(raw json.RawMessage) error {
	if absent(raw) {
		return nil
	}
	if _, err := messagesUsage(raw); err != nil {
		return err
	}
	value, err := object(raw, "Messages usage")
	if err != nil {
		return err
	}
	for key, raw := range value {
		if !absent(raw) {
			s.usage[key] = raw
		}
	}
	return nil
}

func (s *MessagesStream) chunk(delta wireObject, reason string, usage wireObject) ([]byte, error) {
	choice := wireObject{"index": 0, "delta": delta, "finish_reason": nil}
	if reason != "" {
		choice["finish_reason"] = reason
		if !absent(s.terminal["stop_sequence"]) {
			choice["stop_sequence"] = s.terminal["stop_sequence"]
		}
	}
	out := wireObject{"id": s.id, "model": s.model, "object": "chat.completion.chunk", "created": s.created, "choices": []wireObject{choice}}
	if len(usage) != 0 {
		out["usage"] = usage
	}
	data, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	return append(append([]byte("data: "), data...), []byte("\n\n")...), nil
}
