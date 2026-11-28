package messages

import (
	"bytes"
	"encoding/json"
	"strings"
)

type eventFrames struct {
	bytes.Buffer
	err error
}

func (out *eventFrames) emit(event string, payload wireObject) {
	if out.err != nil {
		return
	}
	payload["type"] = event
	encoded, err := json.Marshal(payload)
	if err != nil {
		out.err = err
		return
	}
	out.WriteString("event: " + event + "\ndata: ")
	out.Write(encoded)
	out.WriteString("\n\n")
}

func parseFrame(block []byte) (event, data string, hasData bool, err error) {
	frame := strings.ReplaceAll(string(block), "\r\n", "\n")
	frame = strings.ReplaceAll(frame, "\r", "\n")
	if !strings.HasSuffix(frame, "\n\n") {
		return "", "", false, invalid("Chat SSE block", "is missing its empty-line delimiter")
	}
	frame = strings.TrimSuffix(frame, "\n\n")
	if strings.Contains(frame, "\n\n") {
		return "", "", false, invalid("Chat SSE block", "contains more than one frame")
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
