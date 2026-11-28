package responses

import (
	"bytes"
	"fmt"
)

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
