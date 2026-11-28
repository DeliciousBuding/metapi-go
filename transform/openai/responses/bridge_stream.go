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
	callReasoning          string
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
