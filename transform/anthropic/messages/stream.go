package messages

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ChatStream is a single request's Chat-to-Messages SSE state. It is not safe for
// concurrent calls. Transport framing, decompression and idle guards are owned
// by the caller. An error is sticky: neither later frames nor Finish can repair it.
type ChatStream struct {
	id, model, finishReason string
	started, done, textSeen bool
	err                     error
	usage                   chatUsage
	options                 Options
	reasoning               strings.Builder
	blocks                  []*streamBlock
	textBlock               *streamBlock
	tools                   []*streamTool
	byIndex                 map[int64]*streamTool
	byID                    map[string]*streamTool
	outputIDs               map[string]bool
}

// NewChatStream uses model only when the first Chat chunk omits its model. The
// upstream ID and later nonempty identity fields must remain consistent.
func NewChatStream(model string, options ...Options) *ChatStream {
	replay, err := requestOptionsValue(options)
	return &ChatStream{model: model, options: replay, err: err, byIndex: make(map[int64]*streamTool), byID: make(map[string]*streamTool)}
}

// TransformEvent accepts exactly one complete SSE block (including its empty
// line delimiter) and returns zero or more complete native Messages SSE frames.
// [DONE] is consumed, never forwarded or synthesized. A successful terminal is
// emitted only after a real finish_reason and validation of all tool arguments.
func (s *ChatStream) TransformEvent(block []byte) (result []byte, err error) {
	if s.err != nil {
		return nil, s.err
	}
	defer func() {
		if err != nil {
			s.err = err
			result = nil
		}
	}()
	event, data, hasData, err := parseFrame(block)
	if err != nil {
		return nil, err
	}
	if event == "error" {
		return nil, invalid("Chat SSE", "contains an upstream error event")
	}
	if !hasData {
		return nil, nil
	}
	if event == "ping" {
		if payload, parseErr := object([]byte(data), "Chat ping"); parseErr == nil {
			if err := rejectUpstreamError(payload, "Chat ping"); err != nil {
				return nil, err
			}
		}
		return nil, nil
	}
	if s.done {
		return nil, invalid("Chat SSE", "contains data after [DONE]")
	}
	if strings.TrimSpace(data) == "[DONE]" {
		return s.finishMessage()
	}
	envelope, err := object([]byte(data), "Chat SSE data")
	if err != nil {
		return nil, err
	}
	if err := rejectUpstreamError(envelope, "Chat SSE data"); err != nil {
		return nil, err
	}
	if err := checkChatObject(envelope, "chat.completion.chunk"); err != nil {
		return nil, err
	}
	id, err := optionalText(envelope["id"], "Chat chunk.id")
	if err != nil {
		return nil, err
	}
	if err := updateIdentity(&s.id, id, "Chat chunk.id"); err != nil {
		return nil, err
	}
	model, err := optionalText(envelope["model"], "Chat chunk.model")
	if err != nil {
		return nil, err
	}
	if !s.started && model != "" {
		s.model = model
	} else if err := updateIdentity(&s.model, model, "Chat chunk.model"); err != nil {
		return nil, err
	}
	usage, err := readUsage(envelope["usage"])
	if err != nil {
		return nil, err
	}
	s.usage.merge(usage)
	choices, err := array(envelope["choices"], "Chat choices")
	if err != nil {
		return nil, err
	}
	if len(choices) == 0 {
		if absent(envelope["usage"]) {
			return nil, invalid("Chat choices", "is empty without usage")
		}
		return nil, nil
	}
	choice, err := singleChoice(envelope["choices"])
	if err != nil {
		return nil, err
	}
	delta, err := object(choice["delta"], "Chat delta")
	if err != nil {
		return nil, err
	}
	if err := checkChatContent(delta, true); err != nil {
		return nil, err
	}
	reason, err := optionalText(choice["finish_reason"], "Chat finish_reason")
	if err != nil {
		return nil, err
	}
	switch reason {
	case "", "stop", "length", "tool_calls":
	default:
		return nil, unsupported("Chat finish_reason")
	}
	value, err := optionalText(delta["content"], "Chat delta.content")
	if err != nil {
		return nil, err
	}
	var calls []json.RawMessage
	if !absent(delta["tool_calls"]) {
		calls, err = array(delta["tool_calls"], "Chat delta.tool_calls")
		if err != nil {
			return nil, err
		}
	}
	reasoning, err := optionalText(delta["reasoning_content"], "Chat reasoning_content")
	if err != nil {
		return nil, err
	}
	if s.finishReason != "" {
		if (reason != "" && reason != s.finishReason) || value != "" || len(calls) != 0 || reasoning != "" {
			return nil, invalid("Chat SSE", "contains updates after finish_reason")
		}
	}
	var out eventFrames
	if !s.started {
		if strings.TrimSpace(s.id) == "" || strings.TrimSpace(s.model) == "" {
			return nil, invalid("Chat chunk", "must identify the message and model before content")
		}
		counts, err := s.usage.native()
		if err != nil {
			return nil, err
		}
		out.emit("message_start", wireObject{"message": nativeMessage(s.id, s.model, counts)})
		s.started = true
	}
	if !absent(delta["content"]) {
		s.textSeen = true
		if value != "" {
			s.emitText(&out, value)
		}
	}
	for i, raw := range calls {
		if err := s.transformTool(raw, fmt.Sprintf("Chat delta.tool_calls[%d]", i), &out); err != nil {
			return nil, err
		}
	}
	s.reasoning.WriteString(reasoning)
	if reason != "" {
		s.finishReason = reason
	}
	return out.Bytes(), out.err
}

// Finish checks transport EOF. Missing [DONE], missing finish_reason, upstream
// errors and incomplete tool calls are never reclassified as successful output.
// After a successfully transformed [DONE] frame it is an idempotent no-op.
func (s *ChatStream) Finish() ([]byte, error) {
	if s.err != nil {
		return nil, s.err
	}
	if !s.done {
		s.err = invalid("Chat SSE", "ended without a validated finish_reason and [DONE]")
		return nil, s.err
	}
	return nil, nil
}

// Chat identity fields are full values, not argument fragments. Empty strings
// are ordinary no-update continuations; conflicting nonempty identities cannot
// be repaired after a native content_block_start has been sent.
func updateIdentity(current *string, next, path string) error {
	if next == "" {
		return nil
	}
	if strings.TrimSpace(next) == "" {
		return invalid(path, "must not be blank")
	}
	if *current != "" && *current != next {
		return invalid(path, "changes a known identity")
	}
	*current = next
	return nil
}
