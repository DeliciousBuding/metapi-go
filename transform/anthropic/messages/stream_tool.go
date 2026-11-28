package messages

import (
	"encoding/json"
	"strings"
)

type streamTool struct {
	sourceIndex        *int64
	id, name, outputID string
	arguments          strings.Builder
	block              *streamBlock
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
