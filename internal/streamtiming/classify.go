package streamtiming

import "encoding/json"

type frame struct {
	kinds SignalKind
	bytes int64
}

// stringSize measures nonempty JSON string payloads in wire bytes. The JSON
// decoder validates syntax; timing needs neither unescaping nor a text copy.
type stringSize int64

func (s *stringSize) UnmarshalJSON(b []byte) error {
	*s = 0
	if len(b) > 2 && b[0] == '"' {
		*s = stringSize(len(b) - 2)
	}
	return nil
}

func (f *frame) add(kind SignalKind, size stringSize) {
	if size > 0 {
		f.kinds |= kind
		f.bytes += int64(size)
	}
}

func (f *frame) reasoningDelta(size stringSize, summarized bool) {
	if summarized {
		f.kinds |= SignalHiddenReasoning
		f.add(SignalReasoningSummary, size)
		return
	}
	f.add(SignalReasoningDelta, size)
}

func classify(protocol Protocol, event string, data []byte) frame {
	switch protocol {
	case ProtocolChat:
		return chat(data)
	case ProtocolResponses:
		return responses(event, data)
	case ProtocolMessages:
		return messages(event, data)
	case ProtocolGemini:
		return gemini(data)
	default:
		return frame{}
	}
}

type chatFunction struct{ Name, Arguments stringSize }

func chat(data []byte) (f frame) {
	var v struct {
		Summarized bool
		Choices    []struct {
			Delta struct {
				Content, Refusal, Reasoning stringSize
				ReasoningContent            stringSize `json:"reasoning_content"`
				Summarized                  bool
				ToolCalls                   []struct {
					ID       stringSize
					Function chatFunction
				} `json:"tool_calls"`
				FunctionCall chatFunction `json:"function_call"`
			}
		}
	}
	if json.Unmarshal(data, &v) != nil {
		return f
	}
	for _, c := range v.Choices {
		d := c.Delta
		f.add(SignalTextDelta, d.Content+d.Refusal)
		f.reasoningDelta(d.Reasoning+d.ReasoningContent, v.Summarized || d.Summarized)
		f.add(SignalToolDelta, d.FunctionCall.Name+d.FunctionCall.Arguments)
		for _, t := range d.ToolCalls {
			f.add(SignalToolDelta, t.ID+t.Function.Name+t.Function.Arguments)
		}
	}
	return f
}

type responseItem struct {
	Type       string
	Encrypted  stringSize `json:"encrypted_content"`
	Summarized bool
}

func responses(event string, data []byte) (f frame) {
	var v struct {
		Type       string
		Delta      stringSize
		Item       responseItem
		Encrypted  stringSize `json:"encrypted_content"`
		Summarized bool
		Response   struct{ Output []responseItem }
	}
	if json.Unmarshal(data, &v) != nil {
		return f
	}
	if v.Type != "" {
		event = v.Type
	}
	summarized := v.Summarized || v.Item.Summarized
	if summarized || v.Item.Encrypted > 0 || v.Encrypted > 0 {
		f.kinds |= SignalHiddenReasoning
	}
	switch event {
	case "response.output_text.delta", "response.refusal.delta":
		f.add(SignalTextDelta, v.Delta)
	case "response.reasoning_summary_text.delta":
		f.add(SignalReasoningSummary, v.Delta)
	case "response.reasoning_text.delta":
		f.reasoningDelta(v.Delta, summarized)
	case "response.function_call_arguments.delta", "response.custom_tool_call_input.delta":
		f.add(SignalToolDelta, v.Delta)
	case "response.output_item.added":
		switch v.Item.Type {
		case "reasoning":
			if f.kinds&SignalHiddenReasoning == 0 {
				f.kinds |= SignalReasoningStart
			}
		case "function_call", "custom_tool_call":
			f.add(SignalToolDelta, 1)
		}
	case "response.completed", "response.incomplete":
		// Replayed items can reveal sealed reasoning, but add no new bytes.
		for _, item := range v.Response.Output {
			if item.Encrypted > 0 || item.Summarized {
				f.kinds |= SignalHiddenReasoning
			}
		}
	}
	return f
}

func messages(event string, data []byte) (f frame) {
	var v struct {
		Type       string
		Summarized bool
		Block      struct {
			Type           string
			Text, Thinking stringSize
			Summarized     bool
		} `json:"content_block"`
		Delta struct {
			Type           string
			Text, Thinking stringSize
			PartialJSON    stringSize `json:"partial_json"`
			Summarized     bool
		}
	}
	if json.Unmarshal(data, &v) != nil {
		return f
	}
	if v.Type != "" {
		event = v.Type
	}
	summarized := v.Summarized || v.Block.Summarized || v.Delta.Summarized
	if summarized {
		f.kinds |= SignalHiddenReasoning
	}
	switch event {
	case "content_block_start":
		switch v.Block.Type {
		case "thinking":
			if !summarized {
				f.kinds |= SignalReasoningStart
			}
			f.reasoningDelta(v.Block.Thinking, summarized)
		case "redacted_thinking":
			f.kinds |= SignalHiddenReasoning
		case "tool_use", "server_tool_use":
			f.add(SignalToolDelta, 1)
		case "text":
			f.add(SignalTextDelta, v.Block.Text)
		}
	case "content_block_delta":
		switch v.Delta.Type {
		case "text_delta":
			f.add(SignalTextDelta, v.Delta.Text)
		case "thinking_delta":
			f.reasoningDelta(v.Delta.Thinking, summarized)
		case "input_json_delta":
			f.add(SignalToolDelta, v.Delta.PartialJSON)
		}
	}
	return f
}

func gemini(data []byte) (f frame) {
	var v struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text         stringSize
					Thought      bool
					FunctionCall struct {
						Name stringSize
						Args json.RawMessage
					}
				}
			}
		}
	}
	if json.Unmarshal(data, &v) != nil {
		return f
	}
	for _, c := range v.Candidates {
		for _, p := range c.Content.Parts {
			if p.Thought {
				f.add(SignalReasoningSummary, p.Text)
			} else {
				f.add(SignalTextDelta, p.Text)
			}
			if p.FunctionCall.Name > 0 {
				f.add(SignalToolDelta, p.FunctionCall.Name+stringSize(len(p.FunctionCall.Args)))
			}
		}
	}
	return f
}
