package routecondition

import (
	"bytes"
	"encoding/json"
)

type requestMessage struct {
	Role, Name, ToolCallID, Reasoning string
	Content                           json.RawMessage
	ReasoningFallback                 string `json:"reasoning"`
	ToolCalls                         []struct {
		Type     string
		Function struct {
			Name, Arguments string
		}
	} `json:"tool_calls"`
}

func describeChat(object map[string]json.RawMessage, result *Request) error {
	var messages []requestMessage
	// Only recognized identity fields need explicit snake_case mapping.
	var rows []json.RawMessage
	if json.Unmarshal(object["messages"], &rows) != nil || len(rows) == 0 {
		return errRequest
	}
	for _, row := range rows {
		var message requestMessage
		if json.Unmarshal(row, &message) != nil {
			return errRequest
		}
		fields, err := rawObject(row)
		if err != nil {
			return err
		}
		if message.ToolCallID, err = rawString(fields["tool_call_id"]); err != nil {
			return err
		}
		if message.Reasoning, err = rawString(fields["reasoning_content"]); err != nil {
			return err
		}
		if raw := fields["reasoning_content"]; len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			message.Reasoning = message.ReasoningFallback
		}
		messages = append(messages, message)
	}
	for _, message := range messages {
		result.PromptTokens += sourceTokens(message.Role) + sourceTokens(message.Name) +
			sourceTokens(message.ToolCallID) + sourceTokens(message.Reasoning)
		tokens, err := chatContent(message.Content, result)
		if err != nil {
			return err
		}
		result.PromptTokens += tokens
		for _, call := range message.ToolCalls {
			result.PromptTokens += sourceTokens(call.Type) + sourceTokens(call.Function.Name) + sourceTokens(call.Function.Arguments)
		}
	}
	tools, err := rawArray(object["tools"])
	if err != nil {
		return err
	}
	for _, raw := range tools {
		var tool struct {
			Type     string
			Function struct {
				Name, Description string
				Parameters        json.RawMessage
			}
		}
		if json.Unmarshal(raw, &tool) != nil {
			return errRequest
		}
		result.PromptTokens += sourceTokens(tool.Type) + sourceTokens(tool.Function.Name) +
			sourceTokens(tool.Function.Description) + sourceTokens(string(tool.Function.Parameters))
	}
	return nil
}

func chatContent(raw json.RawMessage, result *Request) (int64, error) {
	text, err := rawString(raw)
	if err == nil {
		return sourceTokens(text), nil
	}
	parts, err := rawArray(raw)
	if err != nil {
		return 0, err
	}
	var tokens int64
	for _, raw := range parts {
		var part struct {
			Type       string
			Text       string
			ImageURL   *struct{ URL, Detail string }  `json:"image_url"`
			VideoURL   *struct{ URL string }          `json:"video_url"`
			InputAudio *struct{ Format, Data string } `json:"input_audio"`
		}
		if json.Unmarshal(raw, &part) != nil {
			return 0, errRequest
		}
		switch part.Type {
		case "text", "image_url", "video_url", "input_audio":
		default:
			return 0, errRequest
		}
		tokens += sourceTokens(part.Type) + sourceTokens(part.Text)
		if part.ImageURL != nil {
			result.HasImage = true
			tokens += 128 + sourceTokens(part.ImageURL.Detail)
		}
		if part.VideoURL != nil {
			result.HasVideo = true
			tokens += 128
		}
		if part.InputAudio != nil {
			result.HasAudio = true
			tokens += 128 + sourceTokens(part.InputAudio.Format)
		}
	}
	return tokens, nil
}
