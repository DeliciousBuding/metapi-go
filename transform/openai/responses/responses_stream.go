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
	if s.created == nil {
		s.created = obj["created_at"]
	}
	if s.id == "" || s.model == "" {
		return fmt.Errorf("Responses/Chat bridge: stream lacks response identity")
	}
	return nil
}

func (s *ResponsesStream) getItem(obj bridgeObject) (int, *bridgeItem, error) {
	index, err := bridgeIndex(obj["output_index"])
	if err != nil {
		return 0, nil, err
	}
	item := s.items[index]
	if item == nil {
		return 0, nil, fmt.Errorf("Responses/Chat bridge: event references unknown output item")
	}
	if item.done {
		return 0, nil, fmt.Errorf("Responses/Chat bridge: event after output item done")
	}
	if id := obj["item_id"]; id != nil && id != item.id {
		return 0, nil, fmt.Errorf("Responses/Chat bridge: conflicting item ID")
	}
	return index, item, nil
}

func (s *ResponsesStream) getPart(obj bridgeObject) (*bridgeItem, *bridgePart, error) {
	_, item, err := s.getItem(obj)
	if err != nil {
		return nil, nil, err
	}
	index, err := bridgeIndex(obj["content_index"])
	if err != nil {
		return nil, nil, err
	}
	if item.kind != "message" || index >= len(item.parts) {
		return nil, nil, fmt.Errorf("Responses/Chat bridge: unknown content part")
	}
	return item, item.parts[index], nil
}

func (s *ResponsesStream) addItem(obj bridgeObject, out *bytes.Buffer) error {
	index, err := bridgeIndex(obj["output_index"])
	if err != nil {
		return err
	}
	if len(s.items) >= bridgeItemLimit || s.items[index] != nil {
		return fmt.Errorf("Responses/Chat bridge: too many or duplicate output items")
	}
	raw := bridgeMap(obj["item"])
	if raw == nil {
		return fmt.Errorf("Responses/Chat bridge: invalid output item")
	}
	id, err := bridgeID(raw["id"], "item.id")
	if err != nil {
		return err
	}
	for _, item := range s.items {
		if item.id == id {
			return fmt.Errorf("Responses/Chat bridge: duplicate output item ID")
		}
	}
	item := &bridgeItem{id: id, kind: bridgeString(raw["type"]), started: true}
	switch item.kind {
	case "message":
		if err = bridgeFields(raw, "id", "type", "role", "status", "content"); err != nil {
			return err
		}
		if raw["role"] != "assistant" {
			return fmt.Errorf("Responses/Chat bridge: unsupported output role")
		}
		if raw["content"] != nil {
			parts, ok := raw["content"].([]any)
			if !ok || len(parts) != 0 {
				return fmt.Errorf("Responses/Chat bridge: message start must have empty content")
			}
		}
	case "function_call":
		if err = bridgeFields(raw, "id", "type", "status", "call_id", "name", "arguments"); err != nil {
			return err
		}
		item.callID, err = bridgeID(raw["call_id"], "call_id")
		if err != nil {
			return err
		}
		item.name, err = bridgeID(raw["name"], "function.name")
		if err != nil {
			return err
		}
		for _, other := range s.items {
			if other.kind == "function_call" && other.callID == item.callID {
				return fmt.Errorf("Responses/Chat bridge: duplicate tool call ID")
			}
		}
		args, err := bridgeText(raw["arguments"], "function arguments")
		if err != nil {
			return err
		}
		if err = s.retain(len(args)); err != nil {
			return err
		}
		item.arguments.WriteString(args)
		s.toolIndices[index] = len(s.toolIndices)
		s.chunk(out, bridgeObject{"tool_calls": []any{bridgeObject{"index": s.toolIndices[index], "id": item.callID, "type": "function", "function": bridgeObject{"name": item.name, "arguments": args}}}}, nil)
	default:
		return fmt.Errorf("Responses/Chat bridge: unsupported output item %q", item.kind)
	}
	if err = s.retain(len(item.id) + len(item.callID) + len(item.name)); err != nil {
		return err
	}
	s.items[index] = item
	s.order = append(s.order, index)
	return nil
}

func bridgePartMatches(raw any, part *bridgePart) bool {
	obj := bridgeMap(raw)
	if obj == nil || obj["type"] != part.kind {
		return false
	}
	if part.kind == "refusal" {
		return bridgeFields(obj, "type", "refusal") == nil && obj["refusal"] == part.text.String()
	}
	text, err := bridgeContent([]any{obj}, true)
	return err == nil && text == part.text.String()
}

func bridgeItemMatches(raw any, item *bridgeItem) bool {
	obj := bridgeMap(raw)
	if obj == nil || obj["id"] != item.id || obj["type"] != item.kind {
		return false
	}
	if item.kind == "function_call" {
		if bridgeFields(obj, "id", "type", "call_id", "name", "arguments", "status") != nil {
			return false
		}
		return obj["call_id"] == item.callID && obj["name"] == item.name && obj["arguments"] == item.arguments.String()
	}
	if bridgeFields(obj, "id", "type", "role", "content", "status") != nil || obj["role"] != "assistant" {
		return false
	}
	parts, ok := obj["content"].([]any)
	if !ok || len(parts) != len(item.parts) {
		return false
	}
	for i, part := range item.parts {
		if !bridgePartMatches(parts[i], part) {
			return false
		}
	}
	return true
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
	switch typ {
	case "response.output_item.added":
		err = s.addItem(obj, &out)
	case "response.content_part.added":
		_, item, e := s.getItem(obj)
		if e != nil {
			return nil, e
		}
		index, e := bridgeIndex(obj["content_index"])
		if e != nil {
			return nil, e
		}
		if item.kind != "message" || index != len(item.parts) || index >= bridgeItemLimit {
			return nil, fmt.Errorf("Responses/Chat bridge: invalid content part index")
		}
		raw := bridgeMap(obj["part"])
		kind := bridgeString(raw["type"])
		if kind != "output_text" && kind != "refusal" {
			return nil, fmt.Errorf("Responses/Chat bridge: unsupported content part")
		}
		part := &bridgePart{kind: kind}
		if !bridgePartMatches(raw, part) {
			return nil, fmt.Errorf("Responses/Chat bridge: content part must start empty")
		}
		item.parts = append(item.parts, part)
	case "response.output_text.delta", "response.refusal.delta":
		_, part, e := s.getPart(obj)
		if e != nil {
			return nil, e
		}
		if part.done || typ != "response."+part.kind+".delta" {
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
		}
		s.chunk(&out, bridgeObject{key: value}, nil)
	case "response.output_text.done", "response.refusal.done":
		_, part, e := s.getPart(obj)
		if e != nil {
			return nil, e
		}
		if part.done || typ != "response."+part.kind+".done" {
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
	case "response.content_part.done":
		_, part, e := s.getPart(obj)
		if e != nil {
			return nil, e
		}
		if !part.done || part.closed || !bridgePartMatches(obj["part"], part) {
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
		if (item.kind == "function_call" && !item.argumentsDone) || (item.kind == "message" && len(item.parts) == 0) {
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

func (s *ResponsesStream) complete(obj bridgeObject, event string) ([]byte, error) {
	response := bridgeMap(obj["response"])
	if response == nil {
		return nil, fmt.Errorf("Responses/Chat bridge: missing final response")
	}
	if err := s.identity(response); err != nil {
		return nil, err
	}
	if event != "response."+bridgeString(response["status"]) {
		return nil, fmt.Errorf("Responses/Chat bridge: terminal event/status mismatch")
	}
	if len(s.items) == 0 {
		return nil, fmt.Errorf("Responses/Chat bridge: terminal without output")
	}
	for _, index := range s.order {
		item := s.items[index]
		if !item.done {
			return nil, fmt.Errorf("Responses/Chat bridge: terminal before output item done")
		}
	}
	_, tools, err := bridgeResponsesOutput(response["output"])
	if err != nil {
		return nil, err
	}
	finalItems, ok := response["output"].([]any)
	if !ok || len(finalItems) != len(s.order) {
		return nil, fmt.Errorf("Responses/Chat bridge: terminal item mismatch")
	}
	for i, index := range s.order {
		if !bridgeItemMatches(finalItems[i], s.items[index]) {
			return nil, fmt.Errorf("Responses/Chat bridge: terminal item identity mismatch")
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
	var out bytes.Buffer
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
