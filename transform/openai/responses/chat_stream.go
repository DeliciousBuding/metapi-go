package responses

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// BridgeStreamLimit bounds accumulated text, reasoning, refusal and function arguments.
// Deltas are emitted immediately; retained content is needed by the Responses
// done events and final response object. Exceeding the limit is a sticky error.
const BridgeStreamLimit = 8 << 20
const bridgeItemLimit = 128

type bridgePart struct {
	kind   string
	text   strings.Builder
	done   bool
	closed bool
}
type bridgeItem struct {
	id, callID, name, kind string
	arguments              strings.Builder
	parts                  []*bridgePart
	started, done          bool
	argumentsDone          bool
}

func (item *bridgeItem) object(status string) bridgeObject {
	if item.kind == "function_call" {
		return bridgeObject{"id": item.id, "type": item.kind, "call_id": item.callID, "name": item.name, "arguments": item.arguments.String(), "status": status}
	}
	content := []any{}
	for _, part := range item.parts {
		if part.kind == "summary_text" {
			content = append(content, bridgeObject{"type": "summary_text", "text": part.text.String()})
		} else if part.kind == "refusal" {
			content = append(content, bridgeObject{"type": "refusal", "refusal": part.text.String()})
		} else {
			content = append(content, bridgeObject{"type": "output_text", "text": part.text.String(), "annotations": []any{}})
		}
	}
	if item.kind == "reasoning" {
		return bridgeObject{"id": item.id, "type": "reasoning", "summary": content, "status": status}
	}
	return bridgeObject{"id": item.id, "type": "message", "role": "assistant", "content": content, "status": status}
}

func bridgeFrame(block []byte) (event, data string, hasData bool, err error) {
	if len(block) > BridgeStreamLimit {
		return "", "", false, fmt.Errorf("Responses/Chat bridge: SSE frame exceeds limit")
	}
	frame := strings.ReplaceAll(strings.ReplaceAll(string(block), "\r\n", "\n"), "\r", "\n")
	if !strings.HasSuffix(frame, "\n\n") {
		return "", "", false, fmt.Errorf("Responses/Chat bridge: incomplete SSE frame")
	}
	frame = strings.TrimSuffix(frame, "\n\n")
	if strings.Contains(frame, "\n\n") {
		return "", "", false, fmt.Errorf("Responses/Chat bridge: expected exactly one SSE frame")
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

func bridgeEmit(out *bytes.Buffer, event string, obj bridgeObject) {
	encoded, _ := json.Marshal(obj)
	if event != "" {
		out.WriteString("event: " + event + "\n")
	}
	out.WriteString("data: ")
	out.Write(encoded)
	out.WriteString("\n\n")
}

func bridgeIdentity(current *string, next string) error {
	if next == "" {
		return nil
	}
	if strings.TrimSpace(next) == "" || (*current != "" && *current != next) {
		return fmt.Errorf("Responses/Chat bridge: conflicting stream identity")
	}
	*current = next
	return nil
}

func bridgeIndex(v any) (int, error) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, fmt.Errorf("Responses/Chat bridge: missing stream index")
	}
	i, err := n.Int64()
	if err != nil || i < 0 || i > 1<<20 {
		return 0, fmt.Errorf("Responses/Chat bridge: invalid stream index")
	}
	return int(i), nil
}

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

func (s *ChatStream) response(status string, details any) bridgeObject {
	output := []any{}
	if status != "in_progress" {
		for _, item := range s.items {
			output = append(output, item.object(status))
		}
	}
	result := bridgeObject{"id": s.id, "object": "response", "model": s.model, "status": status, "output": output, "error": nil, "incomplete_details": details, "usage": s.usage, "store": false}
	if s.created != nil {
		result["created_at"] = s.created
	}
	return result
}

func (s *ChatStream) retain(n int) error {
	s.retained += n
	if s.retained > BridgeStreamLimit {
		return fmt.Errorf("Responses/Chat bridge: accumulated output exceeds %d bytes", BridgeStreamLimit)
	}
	return nil
}

func (s *ChatStream) itemIndex(item *bridgeItem) int {
	for i, current := range s.items {
		if current == item {
			return i
		}
	}
	return -1
}

func (s *ChatStream) addItem(item *bridgeItem, out *bytes.Buffer) error {
	if len(s.items) >= bridgeItemLimit {
		return fmt.Errorf("Responses/Chat bridge: too many output items")
	}
	s.items = append(s.items, item)
	item.started = true
	s.emit(out, "response.output_item.added", bridgeObject{"output_index": len(s.items) - 1, "item": item.object("in_progress")})
	return nil
}

func (s *ChatStream) text(value string, kind string, out *bytes.Buffer) error {
	if err := s.retain(len(value)); err != nil {
		return err
	}
	target := &s.message
	id, itemKind, partEvent, textEvent, indexKey := "msg_"+s.id, "message", "content_part", kind, "content_index"
	if kind == "summary_text" {
		target = &s.reasoning
		id, itemKind, partEvent, textEvent, indexKey = "rs_"+s.id, "reasoning", "reasoning_summary_part", "reasoning_summary_text", "summary_index"
	}
	if *target == nil {
		*target = &bridgeItem{id: id, kind: itemKind}
		if err := s.addItem(*target, out); err != nil {
			return err
		}
	}
	item := *target
	var part *bridgePart
	index := 0
	for i, current := range item.parts {
		if current.kind == kind {
			part = current
			index = i
			break
		}
	}
	if part == nil {
		part = &bridgePart{kind: kind}
		index = len(item.parts)
		item.parts = append(item.parts, part)
		p := bridgeObject{"type": kind}
		if kind == "refusal" {
			p["refusal"] = ""
		} else {
			p["text"] = ""
			if kind == "output_text" {
				p["annotations"] = []any{}
			}
		}
		s.emit(out, "response."+partEvent+".added", bridgeObject{"item_id": item.id, "output_index": s.itemIndex(item), indexKey: index, "part": p})
	}
	part.text.WriteString(value)
	if value != "" {
		s.emit(out, "response."+textEvent+".delta", bridgeObject{"item_id": item.id, "output_index": s.itemIndex(item), indexKey: index, "delta": value})
	}
	return nil
}

func (s *ChatStream) tool(raw any, out *bytes.Buffer) error {
	call := bridgeMap(raw)
	if call == nil {
		return fmt.Errorf("Responses/Chat bridge: invalid tool delta")
	}
	if err := bridgeFields(call, "index", "id", "type", "function"); err != nil {
		return err
	}
	index, err := bridgeIndex(call["index"])
	if err != nil {
		return err
	}
	if kind := bridgeString(call["type"]); kind != "" && kind != "function" {
		return fmt.Errorf("Responses/Chat bridge: unsupported tool delta")
	}
	fn := bridgeMap(call["function"])
	if call["function"] != nil && fn == nil {
		return fmt.Errorf("Responses/Chat bridge: invalid function delta")
	}
	if err := bridgeFields(fn, "name", "arguments"); err != nil {
		return err
	}
	item := s.tools[index]
	if item == nil {
		if len(s.tools) >= bridgeItemLimit {
			return fmt.Errorf("Responses/Chat bridge: too many tools")
		}
		item = &bridgeItem{kind: "function_call"}
		s.tools[index] = item
	}
	for _, pair := range []struct {
		value   any
		current *string
	}{{call["id"], &item.callID}, {fn["name"], &item.name}} {
		if pair.value != nil {
			value, err := bridgeText(pair.value, "tool identity")
			if err != nil {
				return err
			}
			if err = s.retain(len(value)); err != nil {
				return err
			}
			if err = bridgeIdentity(pair.current, value); err != nil {
				return err
			}
		}
	}
	if item.callID != "" {
		for i, other := range s.tools {
			if i != index && other.callID == item.callID {
				return fmt.Errorf("Responses/Chat bridge: duplicate tool ID")
			}
		}
	}
	args := ""
	if fn["arguments"] != nil {
		args, err = bridgeText(fn["arguments"], "tool arguments delta")
		if err != nil {
			return err
		}
	}
	if err = s.retain(len(args)); err != nil {
		return err
	}
	item.arguments.WriteString(args)
	if !item.started && item.callID != "" && item.name != "" {
		item.id = "fc_" + item.callID
		// Start exposes an empty argument string; accumulated early fragments
		// follow as the first delta, so consumers never receive them twice.
		if len(s.items) >= bridgeItemLimit {
			return fmt.Errorf("Responses/Chat bridge: too many output items")
		}
		s.items = append(s.items, item)
		item.started = true
		obj := item.object("in_progress")
		obj["arguments"] = ""
		s.emit(out, "response.output_item.added", bridgeObject{"output_index": len(s.items) - 1, "item": obj})
		args = item.arguments.String()
	}
	if item.started && args != "" {
		s.emit(out, "response.function_call_arguments.delta", bridgeObject{"item_id": item.id, "output_index": s.itemIndex(item), "delta": args})
	}
	return nil
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

func (s *ChatStream) complete() ([]byte, error) {
	if !s.started || s.reason == "" {
		return nil, fmt.Errorf("Responses/Chat bridge: [DONE] without content and finish_reason")
	}
	var out bytes.Buffer
	// A role prelude's empty content must not precede a later reasoning item.
	// Still preserve an explicitly empty completion when nothing else arrived.
	if len(s.items) == 0 && s.textSeen {
		if err := s.text("", "output_text", &out); err != nil {
			return nil, err
		}
	}
	if len(s.items) == 0 {
		return nil, fmt.Errorf("Responses/Chat bridge: [DONE] without content and finish_reason")
	}
	for _, item := range s.tools {
		if !item.started {
			return nil, fmt.Errorf("Responses/Chat bridge: incomplete tool identity")
		}
		if _, err := bridgeArguments(item.arguments.String()); err != nil {
			return nil, err
		}
	}
	status, details, err := bridgeStatus(s.reason, len(s.tools))
	if err != nil {
		return nil, err
	}
	for index, item := range s.items {
		if item.kind == "function_call" {
			s.emit(&out, "response.function_call_arguments.done", bridgeObject{"item_id": item.id, "output_index": index, "arguments": item.arguments.String()})
		} else {
			contentKey, indexKey, partEvent := "content", "content_index", "content_part"
			if item.kind == "reasoning" {
				contentKey, indexKey, partEvent = "summary", "summary_index", "reasoning_summary_part"
			}
			content := item.object(status)[contentKey].([]any)
			for i, part := range item.parts {
				obj := bridgeObject{"item_id": item.id, "output_index": index, indexKey: i}
				if part.kind == "refusal" {
					obj["refusal"] = part.text.String()
				} else {
					obj["text"] = part.text.String()
				}
				textEvent := part.kind
				if item.kind == "reasoning" {
					textEvent = "reasoning_summary_text"
				}
				s.emit(&out, "response."+textEvent+".done", obj)
				s.emit(&out, "response."+partEvent+".done", bridgeObject{"item_id": item.id, "output_index": index, indexKey: i, "part": content[i]})
			}
		}
		s.emit(&out, "response.output_item.done", bridgeObject{"output_index": index, "item": item.object(status)})
	}
	s.emit(&out, "response."+status, bridgeObject{"response": s.response(status, details)})
	s.done = true
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
