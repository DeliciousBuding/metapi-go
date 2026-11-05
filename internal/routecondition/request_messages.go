package routecondition

import (
	"bytes"
	"encoding/json"
	"strings"
)

type anthropicBlock struct {
	Type           string
	Text, Thinking *string
	Signature      string
	Data           string
	Name           string
	Input, Content json.RawMessage
	ToolUseID      string                                       `json:"tool_use_id"`
	Source         *struct{ Type, URL, Data, MediaType string } `json:"-"`
}

func anthropicBlocks(raw json.RawMessage) ([]anthropicBlock, error) {
	rows, err := rawArray(raw)
	if err != nil {
		return nil, err
	}
	blocks := make([]anthropicBlock, 0, len(rows))
	for _, row := range rows {
		var block anthropicBlock
		if json.Unmarshal(row, &block) != nil {
			return nil, errRequest
		}
		object, err := rawObject(row)
		if err != nil {
			return nil, err
		}
		if raw := object["source"]; len(raw) != 0 && !bytes.Equal(raw, []byte("null")) {
			var source struct {
				Type, URL, Data string
				MediaType       string `json:"media_type"`
			}
			if json.Unmarshal(raw, &source) != nil {
				return nil, errRequest
			}
			block.Source = &struct{ Type, URL, Data, MediaType string }{source.Type, source.URL, source.Data, source.MediaType}
		}
		blocks = append(blocks, block)
	}
	return blocks, nil
}

func anthropicContent(blocks []anthropicBlock, collapse bool, result *Request) (int64, bool, error) {
	var tokens int64
	parts := 0
	onlyText := ""
	onlyTextPart := false
	for _, block := range blocks {
		switch block.Type {
		case "text":
			text := ""
			if block.Text != nil {
				text = *block.Text
			}
			parts++
			onlyText = text
			onlyTextPart = true
			tokens += sourceTokens("text") + sourceTokens(text)
		case "image":
			if block.Source == nil || (block.Source.Type == "base64" && block.Source.Data == "") ||
				(block.Source.Type != "base64" && block.Source.URL == "") {
				continue
			}
			parts++
			onlyTextPart = false
			tokens += sourceTokens("image_url") + 128
			result.HasImage = true
		default:
			return 0, false, errRequest
		}
	}
	if collapse && parts == 1 && onlyTextPart {
		tokens = sourceTokens(onlyText)
	}
	return tokens, parts > 0, nil
}

func describeMessages(object map[string]json.RawMessage, result *Request) error {
	if system := object["system"]; len(system) > 0 && !bytes.Equal(bytes.TrimSpace(system), []byte("null")) {
		if text, err := rawString(system); err == nil {
			result.PromptTokens += sourceTokens("system") + sourceTokens(text)
		} else {
			blocks, err := anthropicBlocks(system)
			if err != nil {
				return err
			}
			for _, block := range blocks {
				if block.Type != "text" {
					return errRequest
				}
				result.PromptTokens += sourceTokens("system")
				if block.Text != nil {
					result.PromptTokens += sourceTokens(*block.Text)
				}
			}
		}
	}
	rows, err := rawArray(object["messages"])
	if err != nil || len(rows) == 0 {
		return errRequest
	}
	for _, row := range rows {
		var message struct {
			Role    string
			Content json.RawMessage
		}
		if json.Unmarshal(row, &message) != nil {
			return errRequest
		}
		if text, err := rawString(message.Content); err == nil {
			if len(message.Content) != 0 && !bytes.Equal(bytes.TrimSpace(message.Content), []byte("null")) {
				result.PromptTokens += sourceTokens(message.Role) + sourceTokens(text)
			}
			continue
		}
		blocks, err := anthropicBlocks(message.Content)
		if err != nil {
			return err
		}
		var visible []anthropicBlock
		var ownTokens int64
		var reasoning strings.Builder
		hasContent := false
		for _, block := range blocks {
			switch {
			case block.Type == "text" || block.Type == "image":
				visible = append(visible, block)
			case block.Type == "thinking":
				if block.Thinking != nil {
					reasoning.WriteString(*block.Thinking)
				}
				hasContent = hasContent || (block.Thinking != nil && *block.Thinking != "") || block.Signature != ""
			case block.Type == "redacted_thinking":
				hasContent = hasContent || block.Data != ""
			case block.Type == "tool_use" || strings.HasSuffix(block.Type, "_tool_use"):
				ownTokens += sourceTokens("function") + sourceTokens(block.Name) + sourceTokens(string(block.Input))
				hasContent = true
			case block.Type == "tool_result":
				if len(block.Content) == 0 || bytes.Equal(bytes.TrimSpace(block.Content), []byte("null")) {
					continue
				}
				n, err := rawString(block.Content)
				var contentTokens int64
				if err == nil {
					contentTokens = sourceTokens(n)
				} else {
					children, err := anthropicBlocks(block.Content)
					if err != nil {
						return err
					}
					contentTokens, _, err = anthropicContent(children, false, result)
					if err != nil {
						return err
					}
				}
				result.PromptTokens += sourceTokens("tool") + sourceTokens(block.ToolUseID) + contentTokens
			case strings.HasSuffix(block.Type, "_tool_result"):
				// Source retains these in InlineToolResults, outside its estimator.
				hasContent = true
			default:
				return errRequest
			}
		}
		n, visibleContent, err := anthropicContent(visible, true, result)
		if err != nil {
			return err
		}
		if hasContent || visibleContent {
			result.PromptTokens += sourceTokens(message.Role) + ownTokens + n + sourceTokens(reasoning.String())
		}
	}
	tools, err := rawArray(object["tools"])
	if err != nil {
		return err
	}
	for _, raw := range tools {
		tool, err := rawObject(raw)
		if err != nil {
			return err
		}
		kind, err := rawString(tool["type"])
		if err != nil {
			return err
		}
		switch kind {
		case "", "custom":
			if err := addFunctionTool(tool, "input_schema", "", false, result); err != nil {
				return err
			}
		case "web_search", "web_search_20250305":
			result.PromptTokens += sourceTokens("web_search")
			if raw := tool["max_uses"]; len(raw) != 0 && !bytes.Equal(raw, []byte("null")) {
				result.PromptTokens += 2
			}
			if raw := tool["strict"]; len(raw) != 0 && !bytes.Equal(raw, []byte("null")) {
				result.PromptTokens++
			}
			if err := addStringList(tool["allowed_domains"], result); err != nil {
				return err
			}
			if err := addStringList(tool["blocked_domains"], result); err != nil {
				return err
			}
			if err := addLocation(tool["user_location"], false, result); err != nil {
				return err
			}
		default:
			return errRequest
		}
	}
	return nil
}
