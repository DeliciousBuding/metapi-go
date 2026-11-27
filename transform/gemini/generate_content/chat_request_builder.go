package generate_content

import (
	"strings"

	"github.com/deliciousbuding/metapi-go/transform/shared"
)

// BuildGeminiGenerateContentRequestFromOpenAi converts an OpenAI body to Gemini format.
func BuildGeminiGenerateContentRequestFromOpenAi(openaiBody map[string]any, modelName string) map[string]any {
	if modelName == "" {
		modelName = shared.AsTrimmedString(openaiBody["model"])
	}

	var contents []map[string]any
	var systemInstruction map[string]any

	msgs, _ := openaiBody["messages"].([]any)

	// Map tool_call_id -> function name for functionResponse.name when tool messages omit name.
	toolNameByID := map[string]string{}
	// Map tool_call_id -> thought_signature from provider_specific_fields (or top-level aliases).
	thoughtSignatureByID := map[string]string{}
	for _, item := range msgs {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if strings.ToLower(shared.AsTrimmedString(m["role"])) != "assistant" {
			continue
		}
		tcs, ok := m["tool_calls"].([]any)
		if !ok {
			continue
		}
		for _, tc := range tcs {
			tcm, ok := tc.(map[string]any)
			if !ok {
				continue
			}
			id := shared.AsTrimmedString(tcm["id"])
			fn, _ := tcm["function"].(map[string]any)
			if fn == nil {
				fn = map[string]any{}
			}
			name := shared.AsTrimmedString(fn["name"])
			if id != "" && name != "" {
				toolNameByID[id] = name
			}
			if id == "" {
				continue
			}
			if sig := extractThoughtSignatureFromToolCall(tcm); sig != "" {
				thoughtSignatureByID[id] = sig
			}
		}
	}

	hasThinkingEnabled := requestHasThinkingEnabled(modelName, openaiBody)
	allowsDummy := isDummyThoughtSafeModel(modelName)
	requiresSig := requiresFunctionCallThoughtSignature(modelName)
	shouldDisableThinkingConfig := false

	for _, item := range msgs {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		role := shared.AsTrimmedString(m["role"])

		if role == "system" || role == "developer" {
			text := extractOpenAIText(m["content"])
			if text != "" {
				systemInstruction = map[string]any{
					"parts": []map[string]any{{"text": text}},
				}
			}
			continue
		}

		geminiRole := "user"
		if role == "assistant" || role == "model" {
			geminiRole = "model"
		}

		var textParts []map[string]any
		var fcParts []map[string]any

		// Reasoning content
		if rc := shared.AsTrimmedString(m["reasoning_content"]); rc != "" {
			textParts = append(textParts, map[string]any{"text": rc, "thought": true})
		}

		// Content blocks
		content := m["content"]
		if s, ok := content.(string); ok {
			if s != "" {
				textParts = append(textParts, map[string]any{"text": s})
			}
		} else if arr, ok := content.([]any); ok {
			for _, block := range arr {
				bm, ok := block.(map[string]any)
				if !ok {
					continue
				}
				bt := shared.AsTrimmedString(bm["type"])
				if bt == "text" {
					if t := shared.AsTrimmedString(bm["text"]); t != "" {
						textParts = append(textParts, map[string]any{"text": t})
					}
				} else if bt == "image_url" {
					if iu, ok := bm["image_url"].(map[string]any); ok {
						url := shared.AsTrimmedString(iu["url"])
						if inline := parseDataURLToGeminiInline(url); inline != nil {
							textParts = append(textParts, inline)
						} else if url != "" {
							textParts = append(textParts, map[string]any{
								"fileData": map[string]any{"fileUri": url, "mimeType": inferMimeFromURL(url)},
							})
						}
					}
				}
			}
		}

		// Tool calls -> functionCall parts, with thoughtSignature when required/available.
		if tcs, ok := m["tool_calls"].([]any); ok {
			for _, tc := range tcs {
				tcm, ok := tc.(map[string]any)
				if !ok {
					continue
				}
				fn, _ := tcm["function"].(map[string]any)
				if fn == nil {
					fn = map[string]any{}
				}
				name := shared.AsTrimmedString(fn["name"])
				if name == "" {
					continue
				}
				args := parseToolArguments(shared.AsTrimmedString(fn["arguments"]))
				fcPart := map[string]any{
					"functionCall": map[string]any{
						"name": name,
						"args": args,
					},
				}
				id := shared.AsTrimmedString(tcm["id"])
				if id != "" {
					if fc, ok := fcPart["functionCall"].(map[string]any); ok {
						fc["id"] = id
					}
				}
				sig := thoughtSignatureByID[id]
				if sig == "" {
					sig = extractThoughtSignatureFromToolCall(tcm)
				}
				if sig != "" {
					fcPart["thoughtSignature"] = sig
				} else if (hasThinkingEnabled || requiresSig) && allowsDummy {
					fcPart["thoughtSignature"] = DummyThoughtSignature
				} else if hasThinkingEnabled {
					// Non-Gemini thinking targets cannot safely receive dummy signatures.
					shouldDisableThinkingConfig = true
				}
				fcParts = append(fcParts, fcPart)
			}
		}

		// Tool results
		if role == "tool" {
			toolCallID := shared.AsTrimmedString(m["tool_call_id"])
			name := shared.AsTrimmedString(m["name"])
			if name == "" {
				name = toolNameByID[toolCallID]
			}
			if name == "" {
				name = "unknown"
			}
			response := m["content"]
			contents = append(contents, map[string]any{
				"role": "user",
				"parts": []map[string]any{{
					"functionResponse": map[string]any{
						"name":     name,
						"id":       toolCallID,
						"response": response,
					},
				}},
			})
			continue
		}

		// Official Gemini expects signed functionCall parts separated from sibling text parts.
		hasSigned := false
		for _, p := range fcParts {
			if shared.AsTrimmedString(p["thoughtSignature"]) != "" {
				hasSigned = true
				break
			}
		}
		if hasSigned && len(textParts) > 0 && len(fcParts) > 0 {
			contents = append(contents,
				map[string]any{"role": geminiRole, "parts": textParts},
				map[string]any{"role": geminiRole, "parts": fcParts},
			)
			continue
		}

		allParts := append(append([]map[string]any{}, textParts...), fcParts...)
		if len(allParts) > 0 {
			contents = append(contents, map[string]any{
				"role":  geminiRole,
				"parts": allParts,
			})
		}
	}

	body := map[string]any{
		"model":    modelName,
		"contents": contents,
	}

	if systemInstruction != nil {
		body["systemInstruction"] = systemInstruction
	}

	// Generation config
	gc := map[string]any{}
	if t, ok := openaiBody["temperature"].(float64); ok {
		gc["temperature"] = t
	}
	if tp, ok := openaiBody["top_p"].(float64); ok {
		gc["topP"] = tp
	}
	if mt, ok := openaiBody["max_tokens"].(float64); ok {
		gc["maxOutputTokens"] = int(mt)
	}
	thinkingConfig := ResolveGeminiThinkingConfigFromRequest(modelName, openaiBody)
	if thinkingConfig != nil && !shouldDisableThinkingConfig {
		gc["thinkingConfig"] = thinkingConfig
	}
	if len(gc) > 0 {
		body["generationConfig"] = gc
	}

	// Tools
	if tools := openaiBody["tools"]; tools != nil {
		body["tools"] = convertOpenAiToolsToGemini(tools)
	}

	// Tool choice
	if tc := openaiBody["tool_choice"]; tc != nil {
		body["toolConfig"] = convertOpenAiToolChoiceToGemini(tc)
	}

	return body
}

func extractOpenAIText(content any) string {
	if s, ok := content.(string); ok {
		return s
	}
	if arr, ok := content.([]any); ok {
		var texts []string
		for _, item := range arr {
			if m, ok := item.(map[string]any); ok {
				if m["type"] == "text" {
					if t, ok := m["text"].(string); ok {
						texts = append(texts, t)
					}
				}
			}
		}
		return strings.Join(texts, "\n")
	}
	return ""
}

func parseDataURLToGeminiInline(url string) map[string]any {
	const prefix = "data:"
	if !strings.HasPrefix(url, prefix) {
		return nil
	}
	rest := url[len(prefix):]
	comma := strings.Index(rest, ",")
	if comma < 0 {
		return nil
	}
	mimeType := rest[:comma]
	mimeType = strings.TrimSuffix(mimeType, ";base64")
	data := rest[comma+1:]
	return map[string]any{
		"inlineData": map[string]any{
			"mimeType": mimeType,
			"data":     data,
		},
	}
}

func inferMimeFromURL(url string) string {
	lower := strings.ToLower(url)
	if strings.Contains(lower, ".png") {
		return "image/png"
	}
	if strings.Contains(lower, ".jpg") || strings.Contains(lower, ".jpeg") {
		return "image/jpeg"
	}
	if strings.Contains(lower, ".gif") {
		return "image/gif"
	}
	if strings.Contains(lower, ".webp") {
		return "image/webp"
	}
	return "application/octet-stream"
}
