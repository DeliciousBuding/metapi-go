package relaykitbridge

import (
	"encoding/json"
	"fmt"
)

func terminalError() error {
	return fmt.Errorf("invalid or incomplete upstream terminal: %w", ErrConversion)
}

func validReason(format Format, reason string) bool {
	switch format {
	case Chat:
		return reason == "stop" || reason == "length" || reason == "tool_calls" || reason == "content_filter"
	case Messages:
		return reason == "end_turn" || reason == "max_tokens" || reason == "tool_use" || reason == "stop_sequence" || reason == "refusal"
	case Gemini:
		return reason == "STOP" || reason == "MAX_TOKENS" || reason == "SAFETY" || reason == "RECITATION"
	}
	return false
}

func validateJSONTerminal(format Format, value map[string]any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return ErrConversion
	}
	obj, err := readObject(raw)
	if err != nil {
		return err
	}
	switch format {
	case Chat:
		return validateChatTerminal(obj)
	case Messages:
		return validateMessagesTerminal(obj)
	case Responses:
		return validateResponsesTerminal(obj)
	case Gemini:
		return validateGeminiTerminal(obj)
	}
	return nil
}

type toolState struct {
	id, name     string
	args         []byte
	initial      json.RawMessage
	tool, closed bool
}
type terminalState struct {
	logicalEnd, done, started bool
	tools                     map[int]*toolState
	blocks                    map[int]*toolState
	responseTools             map[int]*toolState
}

func zeroIndex(obj object) error {
	if !present(obj["index"]) {
		return nil
	}
	i, err := indexOf(obj, "index")
	if err != nil || i != 0 {
		return terminalError()
	}
	return nil
}

func indexOf(obj object, key string) (int, error) {
	var value int
	if json.Unmarshal(obj[key], &value) != nil || value < 0 {
		return 0, terminalError()
	}
	return value, nil
}

func (s *terminalState) observe(format Format, obj object) error {
	if s.done {
		if format == Gemini && present(obj["usageMetadata"]) {
			candidates, err := array(obj["candidates"])
			if !present(obj["candidates"]) || err == nil && len(candidates) == 0 {
				return nil
			}
		}
		return terminalError()
	}
	switch format {
	case Chat:
		return s.observeChat(obj)
	case Messages:
		return s.observeMessages(obj)
	case Responses:
		return s.observeResponses(obj)
	case Gemini:
		return s.observeGemini(obj)
	}
	return nil
}
