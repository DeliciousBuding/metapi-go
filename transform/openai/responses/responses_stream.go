package responses

import (
	"bytes"
	"fmt"
	"strings"
)

// ResponsesStream converts Responses SSE to Chat SSE, retaining bounded
// evidence to verify every done event and final output against emitted deltas.
// Errors are sticky. Only a real completed/incomplete event emits a terminal.
type ResponsesStream struct {
	id, model     string
	created       any
	started, done bool
	err           error
	retained      int
	items         map[int]*bridgeItem
	order         []int
	toolIndices   map[int]int
}

func NewResponsesStream(model string) *ResponsesStream {
	return &ResponsesStream{model: model, items: map[int]*bridgeItem{}, toolIndices: map[int]int{}}
}

func (s *ResponsesStream) chunk(out *bytes.Buffer, delta bridgeObject, reason any) {
	obj := bridgeObject{"id": s.id, "object": "chat.completion.chunk", "model": s.model, "choices": []any{bridgeObject{"index": 0, "delta": delta, "finish_reason": reason}}}
	if s.created != nil {
		obj["created"] = s.created
	}
	bridgeEmit(out, "", obj)
}

func (s *ResponsesStream) retain(n int) error {
	s.retained += n
	if s.retained > BridgeStreamLimit {
		return fmt.Errorf("Responses/Chat bridge: accumulated output exceeds %d bytes", BridgeStreamLimit)
	}
	return nil
}

func (s *ResponsesStream) identity(obj bridgeObject) error {
	if err := bridgeRejectError(obj); err != nil {
		return err
	}
	id, err := bridgeOptionalString(obj, "id")
	if err != nil {
		return err
	}
	if err := bridgeIdentity(&s.id, id); err != nil {
		return err
	}
	model, err := bridgeOptionalString(obj, "model")
	if err != nil {
		return err
	}
	if !s.started && model != "" {
		s.model = model
	} else if err := bridgeIdentity(&s.model, model); err != nil {
		return err
	}
	if s.created == nil && obj["created_at"] != nil {
		s.created, err = bridgeCreatedAt(obj["created_at"])
		if err != nil {
			return err
		}
	}
	if s.id == "" || s.model == "" {
		return fmt.Errorf("Responses/Chat bridge: stream lacks response identity")
	}
	return nil
}

func (s *ResponsesStream) TransformEvent(block []byte) (result []byte, err error) {
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
	if strings.TrimSpace(data) == "[DONE]" {
		if !s.done {
			return nil, fmt.Errorf("Responses/Chat bridge: [DONE] before response terminal")
		}
		return nil, nil
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
	if s.done {
		return nil, fmt.Errorf("Responses/Chat bridge: data after response terminal")
	}
	typ := bridgeString(obj["type"])
	if typ == "" || (event != "" && event != typ) {
		return nil, fmt.Errorf("Responses/Chat bridge: mismatched SSE event type")
	}
	var out bytes.Buffer
	if typ == "response.created" || typ == "response.in_progress" {
		response := bridgeMap(obj["response"])
		if response == nil {
			return nil, fmt.Errorf("Responses/Chat bridge: missing response")
		}
		if err = s.identity(response); err != nil {
			return nil, err
		}
		if response["status"] != "in_progress" {
			return nil, fmt.Errorf("Responses/Chat bridge: invalid initial response status")
		}
		if raw := response["output"]; raw != nil {
			output, ok := raw.([]any)
			if !ok || len(output) != 0 {
				return nil, fmt.Errorf("Responses/Chat bridge: initial response must have empty output")
			}
		}
		if !s.started {
			s.started = true
			s.chunk(&out, bridgeObject{"role": "assistant"}, nil)
		}
		return out.Bytes(), nil
	}
	if typ == "response.failed" || typ == "response.error" || typ == "error" {
		return nil, fmt.Errorf("Responses/Chat bridge: unsuccessful upstream response")
	}
	if !s.started {
		return nil, fmt.Errorf("Responses/Chat bridge: content before response.created")
	}
	if strings.HasPrefix(typ, "response.reasoning_summary_") {
		field := "part"
		if typ == "response.reasoning_summary_text.delta" {
			field = "delta"
		} else if typ == "response.reasoning_summary_text.done" {
			field = "text"
		}
		if err := bridgeFields(obj, "type", "sequence_number", "item_id", "output_index", "summary_index", "obfuscation", field); err != nil {
			return nil, err
		}
	}
	switch typ {
	case "response.output_item.added":
		err = s.addItem(obj, &out)
	case "response.content_part.added", "response.reasoning_summary_part.added":
		_, item, e := s.getItem(obj)
		if e != nil {
			return nil, e
		}
		itemKind, indexKey := "message", "content_index"
		if typ == "response.reasoning_summary_part.added" {
			itemKind, indexKey = "reasoning", "summary_index"
		}
		index, e := bridgeIndex(obj[indexKey])
		if e != nil {
			return nil, e
		}
		if item.kind != itemKind || index != len(item.parts) || index >= bridgeItemLimit {
			return nil, fmt.Errorf("Responses/Chat bridge: invalid content part index")
		}
		if itemKind == "reasoning" && index > 0 && !item.parts[index-1].closed {
			return nil, fmt.Errorf("Responses/Chat bridge: overlapping reasoning summary parts cannot preserve text order")
		}
		raw := bridgeMap(obj["part"])
		kind := bridgeString(raw["type"])
		if itemKind == "reasoning" && kind != "summary_text" || itemKind == "message" && kind != "output_text" && kind != "refusal" {
			return nil, fmt.Errorf("Responses/Chat bridge: unsupported content part")
		}
		part := &bridgePart{kind: kind}
		if !bridgePartMatches(raw, part) {
			return nil, fmt.Errorf("Responses/Chat bridge: content part must start empty")
		}
		item.parts = append(item.parts, part)
	case "response.output_text.delta", "response.refusal.delta", "response.reasoning_summary_text.delta":
		_, part, e := s.getPart(obj)
		if e != nil {
			return nil, e
		}
		if part.done || typ != "response."+bridgePartEvent(part.kind)+".delta" {
			return nil, fmt.Errorf("Responses/Chat bridge: invalid text delta lifecycle")
		}
		value, e := bridgeText(obj["delta"], "text delta")
		if e != nil {
			return nil, e
		}
		if err = s.retain(len(value)); err != nil {
			return nil, err
		}
		part.text.WriteString(value)
		key := "content"
		if part.kind == "refusal" {
			key = "refusal"
		} else if part.kind == "summary_text" {
			key = "reasoning_content"
		}
		s.chunk(&out, bridgeObject{key: value}, nil)
	case "response.output_text.done", "response.refusal.done", "response.reasoning_summary_text.done":
		_, part, e := s.getPart(obj)
		if e != nil {
			return nil, e
		}
		if part.done || typ != "response."+bridgePartEvent(part.kind)+".done" {
			return nil, fmt.Errorf("Responses/Chat bridge: invalid text done lifecycle")
		}
		key := "text"
		if part.kind == "refusal" {
			key = "refusal"
		}
		if obj[key] != part.text.String() {
			return nil, fmt.Errorf("Responses/Chat bridge: text done contradicts emitted deltas")
		}
		part.done = true
	case "response.content_part.done", "response.reasoning_summary_part.done":
		item, part, e := s.getPart(obj)
		if e != nil {
			return nil, e
		}
		if (item.kind == "reasoning") != (typ == "response.reasoning_summary_part.done") || !part.done || part.closed || !bridgePartMatches(obj["part"], part) {
			return nil, fmt.Errorf("Responses/Chat bridge: content part done contradicts text")
		}
		part.closed = true
	case "response.function_call_arguments.delta":
		index, item, e := s.getItem(obj)
		if e != nil {
			return nil, e
		}
		if item.kind != "function_call" || item.argumentsDone {
			return nil, fmt.Errorf("Responses/Chat bridge: invalid tool delta lifecycle")
		}
		value, e := bridgeText(obj["delta"], "argument delta")
		if e != nil {
			return nil, e
		}
		if err = s.retain(len(value)); err != nil {
			return nil, err
		}
		item.arguments.WriteString(value)
		s.chunk(&out, bridgeObject{"tool_calls": []any{bridgeObject{"index": s.toolIndices[index], "function": bridgeObject{"arguments": value}}}}, nil)
	case "response.function_call_arguments.done":
		_, item, e := s.getItem(obj)
		if e != nil {
			return nil, e
		}
		if item.kind != "function_call" || item.argumentsDone || obj["arguments"] != item.arguments.String() {
			return nil, fmt.Errorf("Responses/Chat bridge: tool done contradicts emitted deltas")
		}
		if _, err = bridgeArguments(item.arguments.String()); err != nil {
			return nil, err
		}
		item.argumentsDone = true
	case "response.output_item.done":
		_, item, e := s.getItem(obj)
		if e != nil {
			return nil, e
		}
		if (item.kind == "function_call" && !item.argumentsDone) || (item.kind != "function_call" && len(item.parts) == 0) {
			return nil, fmt.Errorf("Responses/Chat bridge: output item ended without content")
		}
		for _, part := range item.parts {
			if !part.closed {
				return nil, fmt.Errorf("Responses/Chat bridge: output item ended before content done")
			}
		}
		if !bridgeItemMatches(obj["item"], item) {
			return nil, fmt.Errorf("Responses/Chat bridge: output item done contradicts content")
		}
		if err := s.callReasoning(bridgeMap(obj["item"]), item, &out); err != nil {
			return nil, err
		}
		item.done = true
	case "response.completed", "response.incomplete":
		return s.complete(obj, typ)
	default:
		return nil, fmt.Errorf("Responses/Chat bridge: unsupported SSE event %q", typ)
	}
	if err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func bridgePartEvent(kind string) string {
	if kind == "summary_text" {
		return "reasoning_summary_text"
	}
	return kind
}

func (s *ResponsesStream) complete(obj bridgeObject, event string) ([]byte, error) {
	response := bridgeMap(obj["response"])
	if response == nil {
		return nil, fmt.Errorf("Responses/Chat bridge: missing final response")
	}
	if err := s.identity(response); err != nil {
		return nil, err
	}
	if event == "response.incomplete" && response["status"] != "incomplete" {
		return nil, fmt.Errorf("Responses/Chat bridge: terminal event/status mismatch")
	}
	for _, index := range s.order {
		item := s.items[index]
		if !item.done {
			return nil, fmt.Errorf("Responses/Chat bridge: terminal before output item done")
		}
	}
	_, tools, err := bridgeResponsesOutput(response)
	if err != nil {
		return nil, err
	}
	finalItems, ok := response["output"].([]any)
	if !ok || len(finalItems) != len(s.order) {
		return nil, fmt.Errorf("Responses/Chat bridge: terminal item mismatch")
	}
	var out bytes.Buffer
	for i, index := range s.order {
		if !bridgeItemMatches(finalItems[i], s.items[index]) {
			return nil, fmt.Errorf("Responses/Chat bridge: terminal item identity mismatch")
		}
		if err := s.callReasoning(bridgeMap(finalItems[i]), s.items[index], &out); err != nil {
			return nil, err
		}
	}
	reason, err := bridgeFinishReason(response, tools)
	if err != nil {
		return nil, err
	}
	usage, err := bridgeUsage(response["usage"], false)
	if err != nil {
		return nil, err
	}
	s.chunk(&out, bridgeObject{}, reason)
	if usage != nil {
		chunk := bridgeObject{"id": s.id, "object": "chat.completion.chunk", "model": s.model, "choices": []any{}, "usage": usage}
		if s.created != nil {
			chunk["created"] = s.created
		}
		bridgeEmit(&out, "", chunk)
	}
	out.WriteString("data: [DONE]\n\n")
	s.done = true
	return out.Bytes(), nil
}

func (s *ResponsesStream) Finish() ([]byte, error) {
	if s.err != nil {
		return nil, s.err
	}
	if !s.done {
		s.err = fmt.Errorf("Responses/Chat bridge: EOF before response terminal")
		return nil, s.err
	}
	return nil, nil
}
