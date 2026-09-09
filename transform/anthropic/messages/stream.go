package messages

import (
	"bytes"
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

type streamBlock struct {
	index  int
	closed bool
}

type streamTool struct {
	sourceIndex        *int64
	id, name, outputID string
	arguments          strings.Builder
	block              *streamBlock
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

func (s *ChatStream) transformTool(raw json.RawMessage, path string, out *eventFrames) error {
	value, err := object(raw, path)
	if err != nil {
		return err
	}
	if err := allowOnly(value, path, "index", "id", "type", "function"); err != nil {
		return err
	}
	id, err := optionalText(value["id"], path+".id")
	if err != nil {
		return err
	}
	typ, err := optionalText(value["type"], path+".type")
	if err != nil {
		return err
	}
	if typ != "" && typ != "function" {
		return unsupported(path + ".type")
	}
	var index *int64
	if raw, ok := value["index"]; ok {
		n, err := nonnegativeInt(raw, path+".index")
		if err != nil {
			return err
		}
		index = &n
	}
	if index == nil && id == "" {
		return invalid(path, "needs an index or nonempty ID; array position is not an identity")
	}
	var name, arguments string
	if !absent(value["function"]) {
		function, err := object(value["function"], path+".function")
		if err != nil {
			return err
		}
		if err := allowOnly(function, path+".function", "name", "arguments"); err != nil {
			return err
		}
		name, err = optionalText(function["name"], path+".function.name")
		if err != nil {
			return err
		}
		arguments, err = optionalText(function["arguments"], path+".function.arguments")
		if err != nil {
			return err
		}
	}
	var tool *streamTool
	if index != nil {
		tool = s.byIndex[*index]
	}
	if byID := s.byID[id]; id != "" && byID != nil {
		if tool != nil && tool != byID {
			return invalid(path, "has conflicting tool index and ID")
		}
		tool = byID
	}
	if tool == nil {
		tool = &streamTool{}
		s.tools = append(s.tools, tool)
	}
	if index != nil {
		if tool.sourceIndex != nil && *tool.sourceIndex != *index {
			return invalid(path+".index", "changes a known tool index")
		}
		tool.sourceIndex = index
		if s.byIndex == nil {
			s.byIndex = make(map[int64]*streamTool)
		}
		s.byIndex[*index] = tool
	}
	if err := updateIdentity(&tool.id, id, path+".id"); err != nil {
		return err
	}
	if err := updateIdentity(&tool.name, name, path+".function.name"); err != nil {
		return err
	}
	if tool.id != "" {
		if s.byID == nil {
			s.byID = make(map[string]*streamTool)
		}
		s.byID[tool.id] = tool
	}
	tool.arguments.WriteString(arguments)
	if tool.block == nil && tool.id != "" && tool.name != "" {
		outputID, err := s.options.toolUseID(tool.id)
		if err != nil {
			return err
		}
		if s.outputIDs[outputID] {
			return invalid("tool_use.id", "allocator returned duplicate identities")
		}
		if s.outputIDs == nil {
			s.outputIDs = make(map[string]bool)
		}
		s.outputIDs[outputID] = true
		tool.outputID = outputID
		s.closeText(out)
		// Anthropic start indices must be contiguous, even when a Chat tool's
		// identity arrives late or the upstream tool indices are sparse.
		tool.block = s.newBlock()
		out.emit("content_block_start", wireObject{"index": tool.block.index, "content_block": wireObject{"type": "tool_use", "id": tool.outputID, "name": tool.name, "input": wireObject{}}})
		arguments = tool.arguments.String()
	}
	if tool.block != nil && arguments != "" {
		s.closeText(out)
		out.emit("content_block_delta", wireObject{"index": tool.block.index, "delta": wireObject{"type": "input_json_delta", "partial_json": arguments}})
	}
	return nil
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

func (s *ChatStream) newBlock() *streamBlock {
	block := &streamBlock{index: len(s.blocks)}
	s.blocks = append(s.blocks, block)
	return block
}

func (s *ChatStream) emitText(out *eventFrames, text string) {
	if s.textBlock == nil {
		s.textBlock = s.newBlock()
		out.emit("content_block_start", wireObject{"index": s.textBlock.index, "content_block": wireObject{"type": "text", "text": ""}})
	}
	out.emit("content_block_delta", wireObject{"index": s.textBlock.index, "delta": wireObject{"type": "text_delta", "text": text}})
}

func (s *ChatStream) closeText(out *eventFrames) {
	if s.textBlock != nil {
		out.emit("content_block_stop", wireObject{"index": s.textBlock.index})
		s.textBlock.closed = true
		s.textBlock = nil
	}
}

func (s *ChatStream) finishMessage() ([]byte, error) {
	if !s.started || s.finishReason == "" {
		return nil, invalid("Chat SSE [DONE]", "arrived without a message and finish_reason")
	}
	if !s.textSeen && len(s.tools) == 0 {
		return nil, invalid("Chat SSE", "contains no supported assistant content")
	}
	for _, tool := range s.tools {
		if tool.id == "" || tool.outputID == "" || tool.name == "" || tool.block == nil {
			return nil, invalid("Chat tool call", "ended with an incomplete identity")
		}
		if _, err := object([]byte(tool.arguments.String()), "Chat tool arguments"); err != nil {
			return nil, err
		}
	}
	reason, err := nativeStopReason(s.finishReason, len(s.tools))
	if err != nil {
		return nil, err
	}
	usage, err := s.usage.native()
	if err != nil {
		return nil, err
	}
	var out eventFrames
	if s.textSeen && len(s.blocks) == 0 {
		s.emitText(&out, "")
	}
	for _, block := range s.blocks {
		if !block.closed {
			out.emit("content_block_stop", wireObject{"index": block.index})
			block.closed = true
		}
	}
	out.emit("message_delta", wireObject{"delta": wireObject{"stop_reason": reason, "stop_sequence": nil}, "usage": usage})
	out.emit("message_stop", wireObject{})
	if out.err != nil {
		return nil, out.err
	}
	ids := make([]string, 0, len(s.tools))
	byBlock := make(map[int]string, len(s.tools))
	for _, tool := range s.tools {
		byBlock[tool.block.index] = tool.outputID
	}
	for _, block := range s.blocks {
		if id, ok := byBlock[block.index]; ok {
			ids = append(ids, id)
		}
	}
	if err := s.options.remember(ids, s.reasoning.String()); err != nil {
		return nil, err
	}
	s.done = true
	return out.Bytes(), nil
}

type eventFrames struct {
	bytes.Buffer
	err error
}

func (out *eventFrames) emit(event string, payload wireObject) {
	if out.err != nil {
		return
	}
	payload["type"] = event
	encoded, err := json.Marshal(payload)
	if err != nil {
		out.err = err
		return
	}
	out.WriteString("event: " + event + "\ndata: ")
	out.Write(encoded)
	out.WriteString("\n\n")
}

func parseFrame(block []byte) (event, data string, hasData bool, err error) {
	frame := strings.ReplaceAll(string(block), "\r\n", "\n")
	frame = strings.ReplaceAll(frame, "\r", "\n")
	if !strings.HasSuffix(frame, "\n\n") {
		return "", "", false, invalid("Chat SSE block", "is missing its empty-line delimiter")
	}
	frame = strings.TrimSuffix(frame, "\n\n")
	if strings.Contains(frame, "\n\n") {
		return "", "", false, invalid("Chat SSE block", "contains more than one frame")
	}
	var lines []string
	for _, line := range strings.Split(frame, "\n") {
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "event":
			event = value
		case "data":
			hasData = true
			lines = append(lines, value)
		}
	}
	return event, strings.Join(lines, "\n"), hasData, nil
}
