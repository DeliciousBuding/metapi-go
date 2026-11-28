package responses

import (
	"bytes"
	"fmt"
	"strings"
)

// ChatStream converts complete Chat SSE frames into Responses SSE frames. It is
// request-local and not concurrency safe. Success requires finish_reason and
// [DONE]; Finish never manufactures success from a transport EOF.
type ChatStream struct {
	id, model, reason  string
	created            any
	started, done      bool
	textSeen           bool
	err                error
	sequence, retained int
	usage              bridgeObject
	items              []*bridgeItem
	tools              map[int]*bridgeItem
	message            *bridgeItem
	reasoning          *bridgeItem
}

func NewChatStream(model string) *ChatStream {
	return &ChatStream{model: model, tools: map[int]*bridgeItem{}}
}

func (s *ChatStream) emit(out *bytes.Buffer, event string, obj bridgeObject) {
	obj["type"] = event
	obj["sequence_number"] = s.sequence
	s.sequence++
	bridgeEmit(out, event, obj)
}

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
	event, data, has, err := bridgeFrame(block)
	if err != nil {
		return nil, err
	}
	if event == "error" {
		return nil, fmt.Errorf("Responses/Chat bridge: upstream SSE error")
	}
	if !has {
		return nil, nil
	}
	if s.done {
		return nil, fmt.Errorf("Responses/Chat bridge: data after terminal")
	}
	if strings.TrimSpace(data) == "[DONE]" {
		return s.complete()
	}
	obj, err := bridgeDecode([]byte(data))
	if err != nil {
		return nil, err
	}
	if err = bridgeRejectError(obj); err != nil {
		return nil, err
	}
	if event == "ping" {
		return nil, nil
	}
	if bridgeString(obj["object"]) != "chat.completion.chunk" {
		return nil, fmt.Errorf("Responses/Chat bridge: expected chat.completion.chunk")
	}
	id, err := bridgeOptionalString(obj, "id")
	if err != nil {
		return nil, err
	}
	if err = bridgeIdentity(&s.id, id); err != nil {
		return nil, err
	}
	model, err := bridgeOptionalString(obj, "model")
	if err != nil {
		return nil, err
	}
	if !s.started && model != "" {
		s.model = model
	} else if err = bridgeIdentity(&s.model, model); err != nil {
		return nil, err
	}
	if s.created == nil {
		s.created = obj["created"]
	}
	if usage := obj["usage"]; usage != nil {
		s.usage, err = bridgeUsage(usage, true)
		if err != nil {
			return nil, err
		}
	}
	choices, ok := obj["choices"].([]any)
	if !ok {
		return nil, fmt.Errorf("Responses/Chat bridge: missing choices")
	}
	if len(choices) == 0 {
		if obj["usage"] == nil {
			return nil, fmt.Errorf("Responses/Chat bridge: empty choices without usage")
		}
		return nil, nil
	}
	choice, err := bridgeChoice(obj)
	if err != nil {
		return nil, err
	}
	delta := bridgeMap(choice["delta"])
	if delta == nil {
		return nil, fmt.Errorf("Responses/Chat bridge: invalid delta")
	}
	if err = bridgeFields(delta, "role", "content", "tool_calls", "refusal", "reasoning_content"); err != nil {
		return nil, err
	}
	if role := delta["role"]; role != nil && role != "assistant" {
		return nil, fmt.Errorf("Responses/Chat bridge: unexpected role")
	}
	reason := bridgeString(choice["finish_reason"])
	if s.reason != "" {
		return nil, fmt.Errorf("Responses/Chat bridge: choice after finish_reason")
	}
	var out bytes.Buffer
	if !s.started {
		if s.id == "" || s.model == "" {
			return nil, fmt.Errorf("Responses/Chat bridge: stream lacks identity")
		}
		s.started = true
		s.emit(&out, "response.created", bridgeObject{"response": s.response("in_progress", nil)})
		s.emit(&out, "response.in_progress", bridgeObject{"response": s.response("in_progress", nil)})
	}
	for _, pair := range [][2]string{{"reasoning_content", "summary_text"}, {"content", "output_text"}, {"refusal", "refusal"}} {
		if delta[pair[0]] != nil {
			if pair[0] == "content" {
				s.textSeen = true
			}
			value, err := bridgeText(delta[pair[0]], pair[0])
			if err != nil {
				return nil, err
			}
			if value == "" && (pair[0] == "reasoning_content" || pair[0] == "content" && delta["role"] == "assistant") {
				continue
			}
			if err = s.text(value, pair[1], &out); err != nil {
				return nil, err
			}
		}
	}
	if raw := delta["tool_calls"]; raw != nil {
		calls, ok := raw.([]any)
		if !ok {
			return nil, fmt.Errorf("Responses/Chat bridge: invalid tool_calls")
		}
		for _, call := range calls {
			if err = s.tool(call, &out); err != nil {
				return nil, err
			}
		}
	}
	if reason != "" {
		if _, _, err = bridgeStatus(reason, len(s.tools)); err != nil {
			return nil, err
		}
		s.reason = reason
	} else if choice["finish_reason"] != nil && choice["finish_reason"] != "" {
		return nil, fmt.Errorf("Responses/Chat bridge: invalid finish_reason")
	}
	return out.Bytes(), nil
}

func (s *ChatStream) Finish() ([]byte, error) {
	if s.err != nil {
		return nil, s.err
	}
	if !s.done {
		s.err = fmt.Errorf("Responses/Chat bridge: EOF before finish_reason and [DONE]")
		return nil, s.err
	}
	return nil, nil
}
