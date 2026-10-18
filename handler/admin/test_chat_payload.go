package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/deliciousbuding/metapi-go/transform/relaykitbridge"
)

func (o *chatTestOptions) validate() error {
	switch o.TargetFormat {
	case "", "openai", "claude", "responses", "gemini":
	default:
		return fmt.Errorf("unsupported targetFormat")
	}
	if o.Temperature != nil && (*o.Temperature < 0 || *o.Temperature > 2) {
		return fmt.Errorf("temperature must be between 0 and 2")
	}
	if o.TopP != nil && (*o.TopP < 0 || *o.TopP > 1) {
		return fmt.Errorf("top_p must be between 0 and 1")
	}
	if o.MaxTokens != nil && (*o.MaxTokens < 1 || *o.MaxTokens > 128000) {
		return fmt.Errorf("max_tokens must be between 1 and 128000")
	}
	for _, message := range o.Messages {
		switch message.Role {
		case "system", "user", "assistant":
		default:
			return fmt.Errorf("messages must use system, user or assistant roles")
		}
		var content string
		if err := json.Unmarshal(message.Content, &content); err != nil {
			var parts []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}
			if err := json.Unmarshal(message.Content, &parts); err != nil || len(parts) == 0 {
				return fmt.Errorf("messages must contain text content")
			}
			for _, part := range parts {
				if part.Type != "text" {
					return fmt.Errorf("messages must contain text content")
				}
			}
		}
	}
	return nil
}

// Use the same protocol converter as the proxy, with the existing probe
// transport retaining the forced channel, credentials, deadline and cancellation.
func (o *chatTestOptions) request(ctx context.Context, model, prompt string) (string, []byte, error) {
	messages := o.Messages
	if len(messages) == 0 {
		content, _ := json.Marshal(prompt)
		messages = []testMessage{{Role: "user", Content: content}}
	}
	maxTokens := 1024
	if o.MaxTokens != nil {
		maxTokens = *o.MaxTokens
	}
	payload := map[string]any{"model": model, "messages": messages, "max_tokens": maxTokens, "stream": false}
	if o.Temperature != nil {
		payload["temperature"] = *o.Temperature
	}
	if o.TopP != nil {
		payload["top_p"] = *o.TopP
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", nil, err
	}
	format, path := relaykitbridge.Chat, "/v1/chat/completions"
	switch o.TargetFormat {
	case "claude":
		format, path = relaykitbridge.Messages, "/v1/messages"
	case "responses":
		format, path = relaykitbridge.Responses, "/v1/responses"
	case "gemini":
		format, path = relaykitbridge.Gemini, "/v1beta/models/"+url.PathEscape(model)+":generateContent"
	}
	_, body, err := relaykitbridge.ConvertRequest(ctx, relaykitbridge.Chat, format, model, raw)
	return path, body, err
}
