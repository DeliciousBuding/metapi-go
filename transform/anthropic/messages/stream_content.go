package messages

type streamBlock struct {
	index  int
	closed bool
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
