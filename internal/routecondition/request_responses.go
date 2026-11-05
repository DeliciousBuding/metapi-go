package routecondition

import (
	"bytes"
	"encoding/json"
	"strings"
)

type responseItem struct {
	Type, Role, Name, CallID string
	Text                     *string
	Content, Output          json.RawMessage
	Arguments                json.RawMessage
	ImageURL, Detail         *string `json:"-"`
	EncryptedContent         string  `json:"encrypted_content"`
	Summary                  []struct{ Text string }
}

func decodeResponseItems(raw json.RawMessage) ([]responseItem, error) {
	rows, err := rawArray(raw)
	if err != nil {
		return nil, err
	}
	items := make([]responseItem, 0, len(rows))
	for _, row := range rows {
		var item responseItem
		if json.Unmarshal(row, &item) != nil {
			return nil, errRequest
		}
		fields, err := rawObject(row)
		if err != nil {
			return nil, err
		}
		if item.CallID, err = rawString(fields["call_id"]); err != nil {
			return nil, err
		}
		if raw, ok := fields["image_url"]; ok {
			var value string
			if json.Unmarshal(raw, &value) != nil {
				return nil, errRequest
			}
			item.ImageURL = &value
		}
		if raw, ok := fields["detail"]; ok {
			var value string
			if json.Unmarshal(raw, &value) != nil {
				return nil, errRequest
			}
			item.Detail = &value
		}
		items = append(items, item)
	}
	return items, nil
}

func responseCallTokens(item responseItem) (int64, error) {
	if item.Type == "custom_tool_call" {
		// Source stores custom-call data separately from Function; its estimator
		// only counts the tool-call type for this variant.
		return sourceTokens("responses_custom_tool"), nil
	}
	args, err := rawString(item.Arguments)
	if err != nil {
		args, err = compactJSON(item.Arguments, false)
	}
	return sourceTokens("function") + sourceTokens(item.Name) + sourceTokens(args), err
}

func describeResponses(object map[string]json.RawMessage, result *Request) error {
	instructions, err := rawString(object["instructions"])
	if err != nil {
		return err
	}
	if instructions != "" {
		result.PromptTokens += sourceTokens("system") + sourceTokens(instructions)
	}
	if text, err := rawString(object["input"]); err == nil {
		if _, present := object["input"]; present {
			result.PromptTokens += sourceTokens("user") + sourceTokens(text)
		}
	} else {
		items, err := decodeResponseItems(object["input"])
		if err != nil {
			return err
		}
		for i := 0; i < len(items); {
			item := items[i]
			if item.Type == "reasoning" {
				result.PromptTokens += sourceTokens("assistant")
				var reasoning strings.Builder
				for i < len(items) && items[i].Type == "reasoning" {
					for _, summary := range items[i].Summary {
						reasoning.WriteString(summary.Text)
					}
					i++
				}
				result.PromptTokens += sourceTokens(reasoning.String())
				var content int64
				for i < len(items) {
					next := items[i]
					if next.Type == "function_call" || next.Type == "custom_tool_call" {
						n, err := responseCallTokens(next)
						if err != nil {
							return err
						}
						result.PromptTokens += n
					} else if (next.Type == "message" || next.Type == "input_text" || next.Type == "") && next.Role == "assistant" {
						n, err := responseMessageContent(next, result)
						if err != nil {
							return err
						}
						content = n // Source keeps the last assistant content.
					} else {
						break
					}
					i++
				}
				result.PromptTokens += content
				continue
			}
			if item.Type == "function_call" || item.Type == "custom_tool_call" {
				result.PromptTokens += sourceTokens("assistant")
				for i < len(items) && (items[i].Type == "function_call" || items[i].Type == "custom_tool_call") {
					n, err := responseCallTokens(items[i])
					if err != nil {
						return err
					}
					result.PromptTokens += n
					i++
				}
				continue
			}
			switch item.Type {
			case "message", "input_text", "":
				n, err := responseMessageContent(item, result)
				if err != nil {
					return err
				}
				result.PromptTokens += sourceTokens(item.Role) + n
			case "input_image":
				if item.ImageURL != nil {
					role := item.Role
					if role == "" {
						role = "user"
					}
					result.PromptTokens += sourceTokens(role) + sourceTokens("image_url") + 128
					if item.Detail != nil {
						result.PromptTokens += sourceTokens(*item.Detail)
					}
					result.HasImage = true
				}
			case "function_call_output", "custom_tool_call_output":
				if len(item.Output) == 0 || bytes.Equal(bytes.TrimSpace(item.Output), []byte("null")) {
					return errRequest
				}
				n, err := responseContent(item.Output, false, result)
				if err != nil {
					return err
				}
				result.PromptTokens += sourceTokens("tool") + sourceTokens(item.CallID) + sourceTokens(item.Name) + n
			case "compaction", "compaction_summary":
				result.PromptTokens += sourceTokens("assistant") + sourceTokens(item.Type) + min(sourceTokens(item.EncryptedContent), 256)
			default:
				return errRequest
			}
			i++
		}
	}
	return describeResponseTools(object["tools"], result)
}

func responseMessageContent(item responseItem, result *Request) (int64, error) {
	if len(item.Content) != 0 {
		return responseContent(item.Content, true, result)
	}
	if item.Text != nil {
		return sourceTokens(*item.Text), nil
	}
	return 0, nil
}

func responseContent(raw json.RawMessage, outputMessage bool, result *Request) (int64, error) {
	if text, err := rawString(raw); err == nil {
		return sourceTokens(text), nil
	}
	items, err := decodeResponseItems(raw)
	if err != nil {
		return 0, err
	}
	output := false
	if outputMessage {
		for _, item := range items {
			output = output || item.Type == "output_text"
		}
	}
	var tokens int64
	parts := 0
	var onlyText *string
	for _, item := range items {
		switch item.Type {
		case "text", "input_text", "output_text":
			if item.Text == nil && !output {
				continue
			}
			text := ""
			if item.Text != nil {
				text = *item.Text
			}
			parts++
			onlyText = &text
			tokens += sourceTokens("text") + sourceTokens(text)
		case "input_image":
			if output {
				continue
			}
			if item.ImageURL != nil {
				parts++
				onlyText = nil
				result.HasImage = true
				tokens += sourceTokens("image_url") + 128
				if item.Detail != nil {
					tokens += sourceTokens(*item.Detail)
				}
			}
		case "compaction", "compaction_summary":
			if output {
				continue
			}
			parts++
			onlyText = nil
			tokens += sourceTokens(item.Type) + min(sourceTokens(item.EncryptedContent), 256)
		default:
			return 0, errRequest
		}
	}
	if parts == 1 && onlyText != nil {
		return sourceTokens(*onlyText), nil
	}
	return tokens, nil
}

func describeResponseTools(raw json.RawMessage, result *Request) error {
	tools, err := rawArray(raw)
	if err != nil {
		return err
	}
	for _, raw := range tools {
		object, err := rawObject(raw)
		if err != nil {
			return err
		}
		kind, err := rawString(object["type"])
		if err != nil {
			return err
		}
		switch kind {
		case "function":
			if err := addFunctionTool(object, "parameters", "", true, result); err != nil {
				return err
			}
		case "namespace":
			prefix, err := rawString(object["name"])
			if err != nil {
				return err
			}
			children, err := rawArray(object["tools"])
			if err != nil {
				return err
			}
			for _, child := range children {
				value, err := rawObject(child)
				if err != nil {
					return err
				}
				childKind, err := rawString(value["type"])
				if err != nil || childKind != "function" {
					return errRequest
				}
				if err := addFunctionTool(value, "parameters", prefix+"__", true, result); err != nil {
					return err
				}
			}
		case "web_search":
			result.PromptTokens += sourceTokens("web_search")
			if filters := object["filters"]; len(filters) != 0 {
				obj, err := rawObject(filters)
				if err != nil {
					return err
				}
				if err := addStringList(obj["allowed_domains"], result); err != nil {
					return err
				}
			}
			if err := addLocation(object["user_location"], true, result); err != nil {
				return err
			}
		case "custom":
			result.PromptTokens += sourceTokens("responses_custom_tool")
			for _, field := range []string{"name", "description"} {
				value, err := rawString(object[field])
				if err != nil {
					return err
				}
				result.PromptTokens += sourceTokens(value)
			}
			if len(object["format"]) > 0 {
				format, err := rawObject(object["format"])
				if err != nil {
					return err
				}
				for _, field := range []string{"type", "syntax", "definition"} {
					value, err := rawString(format[field])
					if err != nil {
						return err
					}
					result.PromptTokens += sourceTokens(value)
				}
			}
		case "image_generation":
			result.PromptTokens += sourceTokens("image_generation")
		default:
			return errRequest
		}
	}
	return nil
}
