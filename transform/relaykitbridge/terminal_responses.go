package relaykitbridge

import (
	"reflect"
	"strings"

	"github.com/deliciousbuding/metapi-go/transform/shared"
)

func validateResponsesTerminal(obj object) error {
	if shared.RawString(obj["status"]) != "completed" {
		return terminalError()
	}
	items, err := array(obj["output"])
	if err != nil {
		return err
	}
	for _, raw := range items {
		item, err := readObject(raw)
		if err != nil {
			return err
		}
		if shared.RawString(item["type"]) == "function_call" {
			if shared.RawString(item["call_id"]) == "" || shared.RawString(item["name"]) == "" {
				return terminalError()
			}
			if _, err := readObject([]byte(shared.RawString(item["arguments"]))); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *terminalState) observeResponses(obj object) error {
	switch shared.RawString(obj["type"]) {
	case "response.output_item.added", "response.output_item.done":
		item, err := readObject(obj["item"])
		if err != nil {
			return err
		}
		if shared.RawString(item["type"]) != "function_call" {
			break
		}
		idx, err := indexOf(obj, "output_index")
		if err != nil {
			return err
		}
		if s.responseTools == nil {
			s.responseTools = map[int]*toolState{}
		}
		tool := s.responseTools[idx]
		if tool == nil {
			tool = &toolState{id: shared.RawString(item["call_id"]), name: shared.RawString(item["name"])}
			s.responseTools[idx] = tool
		}
		if tool.closed || tool.id == "" || tool.name == "" || tool.id != shared.RawString(item["call_id"]) || tool.name != shared.RawString(item["name"]) {
			return terminalError()
		}
		args := []byte(shared.RawString(item["arguments"]))
		if shared.RawString(obj["type"]) == "response.output_item.done" {
			if _, err := readObject(args); err != nil {
				return err
			}
			if len(tool.args) > 0 && !sameJSON(tool.args, args) {
				return terminalError()
			}
			tool.closed = true
		}
		tool.args = args
	case "response.function_call_arguments.delta", "response.function_call_arguments.done":
		idx, err := indexOf(obj, "output_index")
		if err != nil {
			return err
		}
		tool := s.responseTools[idx]
		if tool == nil || tool.closed {
			return terminalError()
		}
		if shared.RawString(obj["type"]) == "response.function_call_arguments.delta" {
			tool.args = append(tool.args, shared.RawString(obj["delta"])...)
		} else {
			args := []byte(shared.RawString(obj["arguments"]))
			if _, err := readObject(args); err != nil {
				return err
			}
			if len(tool.args) > 0 && !sameJSON(tool.args, args) {
				return terminalError()
			}
			tool.args = args
		}
	case "response.completed":
		for _, tool := range s.responseTools {
			if !tool.closed {
				return terminalError()
			}
		}
		var response map[string]any
		if (numberCodec{}).Unmarshal(obj["response"], &response) != nil {
			return terminalError()
		}
		if err := validateJSONTerminal(Responses, response); err != nil {
			return err
		}
		s.logicalEnd = true
		s.done = true
	case "response.failed", "response.incomplete", "error":
		return terminalError()
	default:
		if !strings.HasPrefix(shared.RawString(obj["type"]), "response.") {
			return terminalError()
		}
	}
	return nil
}

func sameJSON(a, b []byte) bool {
	var aa, bb any
	if (numberCodec{}).Unmarshal(a, &aa) != nil || (numberCodec{}).Unmarshal(b, &bb) != nil {
		return false
	}
	return reflect.DeepEqual(aa, bb)
}
