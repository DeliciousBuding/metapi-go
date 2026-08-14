package generate_content

import (
	"strings"

	"github.com/deliciousbuding/metapi-go/transform/shared"
)

// --- 4xx field filtering ---

// AllowedGeminiKeys are the 9 keys forwarded to the Gemini API.
var AllowedGeminiKeys = map[string]bool{
	"contents": true, "systemInstruction": true, "cachedContent": true,
	"safetySettings": true, "generationConfig": true, "tools": true,
	"toolConfig": true, "labels": true, "model": true,
}

// DummyThoughtSignature is the base64 sentinel for "skip_thought_signature_validator".
// Gemini accepts any non-empty base64 string when a real thought signature is unavailable.
const DummyThoughtSignature = "c2tpcF90aG91Z2h0X3NpZ25hdHVyZV92YWxpZGF0b3I="

// NormalizeRequest filters and normalizes an incoming Gemini request body.
func NormalizeRequest(body map[string]any, modelName string) map[string]any {
	next := map[string]any{}
	if body == nil {
		return next
	}

	if body["contents"] != nil {
		next["contents"] = cloneGeminiContents(body["contents"])
	}
	if body["systemInstruction"] != nil {
		next["systemInstruction"] = cloneJSONValue(body["systemInstruction"])
	}
	if body["cachedContent"] != nil {
		next["cachedContent"] = cloneJSONValue(body["cachedContent"])
	}
	if body["safetySettings"] != nil {
		next["safetySettings"] = cloneJSONValue(body["safetySettings"])
	}
	if body["generationConfig"] != nil {
		next["generationConfig"] = cloneGeminiGenerationConfig(body["generationConfig"])
	}
	if body["tools"] != nil {
		next["tools"] = cloneGeminiTools(body["tools"])
	}
	if body["toolConfig"] != nil {
		next["toolConfig"] = cloneJSONValue(body["toolConfig"])
	}

	// Derive thinking config from reasoning params
	derivedTC := ResolveGeminiThinkingConfigFromRequest(modelName, body)
	if derivedTC != nil {
		gc, _ := next["generationConfig"].(map[string]any)
		if gc == nil {
			gc = map[string]any{}
		}
		tc := mergeThinkingConfig(gc["thinkingConfig"], derivedTC)
		if tc != nil {
			gc["thinkingConfig"] = tc
		}
		next["generationConfig"] = gc
	}

	// Passthrough allowed keys
	for k, v := range body {
		if !AllowedGeminiKeys[k] {
			continue
		}
		if next[k] != nil {
			continue
		}
		next[k] = cloneJSONValue(v)
	}

	// Official Gemini multi-turn tool history requires thoughtSignature on
	// functionCall parts for Gemini 3.x (and thinking-enabled models).
	injectThoughtSignaturesIntoContents(next, modelName)

	return next
}

// --- OpenAI -> Gemini body conversion ---

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
				args := shared.ParseJSONLike(shared.AsTrimmedString(fn["arguments"]))
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

// --- Thought signature helpers ---

// isDummyThoughtSafeModel reports whether dummy thought signatures are safe for this model.
// Only official Gemini model IDs accept the sentinel; third-party "Gemini-compatible" models may not.
func isDummyThoughtSafeModel(modelName string) bool {
	normalized := strings.ToLower(shared.AsTrimmedString(modelName))
	return strings.HasPrefix(normalized, "gemini-") || strings.HasPrefix(normalized, "models/gemini-")
}

// requiresFunctionCallThoughtSignature reports models that reject tool history without thoughtSignature
// even when the client did not enable thinking/reasoning explicitly (Gemini 3.x).
func requiresFunctionCallThoughtSignature(modelName string) bool {
	normalized := strings.ToLower(shared.AsTrimmedString(modelName))
	normalized = strings.TrimPrefix(normalized, "models/")
	// gemini-3, gemini-3.5-flash, gemini-3-pro-preview, etc.
	return strings.HasPrefix(normalized, "gemini-3")
}

// requestHasThinkingEnabled detects thinking either via derived reasoning params or explicit thinkingConfig.
func requestHasThinkingEnabled(modelName string, body map[string]any) bool {
	if ResolveGeminiThinkingConfigFromRequest(modelName, body) != nil {
		return true
	}
	if body == nil {
		return false
	}
	if gc, ok := body["generationConfig"].(map[string]any); ok && gc != nil {
		if tc := gc["thinkingConfig"]; tc != nil {
			return true
		}
	}
	return false
}

func extractThoughtSignature(part map[string]any) string {
	if part == nil {
		return ""
	}
	if sig := shared.AsTrimmedString(part["thoughtSignature"]); sig != "" {
		return sig
	}
	if sig := shared.AsTrimmedString(part["thought_signature"]); sig != "" {
		return sig
	}
	return ""
}

func extractThoughtSignatureFromToolCall(toolCall map[string]any) string {
	if toolCall == nil {
		return ""
	}
	if sig := shared.AsTrimmedString(toolCall["thoughtSignature"]); sig != "" {
		return sig
	}
	if sig := shared.AsTrimmedString(toolCall["thought_signature"]); sig != "" {
		return sig
	}
	if psf, ok := toolCall["provider_specific_fields"].(map[string]any); ok && psf != nil {
		if sig := shared.AsTrimmedString(psf["thought_signature"]); sig != "" {
			return sig
		}
		if sig := shared.AsTrimmedString(psf["thoughtSignature"]); sig != "" {
			return sig
		}
	}
	return ""
}

// injectThoughtSignaturesIntoContents ensures native Gemini contents with functionCall parts
// carry thoughtSignature when official Gemini would reject unsigned tool history.
// Existing real signatures are preserved; dummy is only injected when missing.
func injectThoughtSignaturesIntoContents(body map[string]any, modelName string) {
	if body == nil {
		return
	}
	allowsDummy := isDummyThoughtSafeModel(modelName)
	requiresSig := requiresFunctionCallThoughtSignature(modelName)
	hasThinking := requestHasThinkingEnabled(modelName, body)
	if !allowsDummy || (!requiresSig && !hasThinking) {
		return
	}

	// contents may be []any (from clone) or []map[string]any in tests.
	switch contents := body["contents"].(type) {
	case []any:
		for _, item := range contents {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			injectThoughtSignaturesIntoParts(m)
		}
	case []map[string]any:
		for _, m := range contents {
			injectThoughtSignaturesIntoParts(m)
		}
	}
}

func injectThoughtSignaturesIntoParts(content map[string]any) {
	if content == nil {
		return
	}
	switch parts := content["parts"].(type) {
	case []any:
		for _, p := range parts {
			pm, ok := p.(map[string]any)
			if !ok {
				continue
			}
			if _, hasFC := pm["functionCall"]; !hasFC {
				continue
			}
			if extractThoughtSignature(pm) != "" {
				continue
			}
			pm["thoughtSignature"] = DummyThoughtSignature
		}
	case []map[string]any:
		for _, pm := range parts {
			if pm == nil {
				continue
			}
			if _, hasFC := pm["functionCall"]; !hasFC {
				continue
			}
			if extractThoughtSignature(pm) != "" {
				continue
			}
			pm["thoughtSignature"] = DummyThoughtSignature
		}
	}
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

func convertOpenAiToolsToGemini(tools any) any {
	arr, ok := tools.([]any)
	if !ok {
		return tools
	}
	var fds []map[string]any
	var otherTools []map[string]any
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		t := strings.ToLower(shared.AsTrimmedString(m["type"]))
		if t == "function" {
			if fn, ok := m["function"].(map[string]any); ok {
				name := shared.AsTrimmedString(fn["name"])
				if name == "" {
					continue
				}
				fd := map[string]any{"name": name}
				if d := shared.AsTrimmedString(fn["description"]); d != "" {
					fd["description"] = d
				}
				if params, ok := fn["parameters"].(map[string]any); ok {
					fd["parametersJsonSchema"] = params
				}
				fds = append(fds, fd)
			}
		} else if t == "web_search" || t == "google_search" {
			otherTools = append(otherTools, map[string]any{"googleSearch": map[string]any{}})
		} else if t == "code_interpreter" {
			otherTools = append(otherTools, map[string]any{"codeExecution": map[string]any{}})
		}
	}
	var result []map[string]any
	if len(fds) > 0 {
		result = append(result, map[string]any{"functionDeclarations": fds})
	}
	result = append(result, otherTools...)
	if len(result) == 0 {
		return tools
	}
	return result
}

func convertOpenAiToolChoiceToGemini(tc any) any {
	var mode string
	if s, ok := tc.(string); ok {
		n := strings.ToUpper(strings.TrimSpace(s))
		switch n {
		case "NONE":
			mode = "NONE"
		case "REQUIRED", "ANY":
			mode = "ANY"
		case "AUTO":
			mode = "AUTO"
		default:
			mode = "AUTO"
		}
	} else if m, ok := tc.(map[string]any); ok {
		t := strings.ToLower(shared.AsTrimmedString(m["type"]))
		switch t {
		case "none":
			mode = "NONE"
		case "required", "any":
			mode = "ANY"
		case "function", "tool":
			mode = "ANY"
		default:
			mode = "AUTO"
		}
	} else {
		mode = "AUTO"
	}
	return map[string]any{
		"functionCallingConfig": map[string]any{"mode": mode},
	}
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

// --- Thinking config conversion ---

// ReasoningToThinkingConfig converts OpenAI reasoning settings to Gemini thinkingConfig.
func ReasoningToThinkingConfig(effort string, budgetTokens int) map[string]any {
	if effort != "" {
		switch strings.ToLower(effort) {
		case "none":
			return nil
		case "low":
			return map[string]any{"thinkingBudget": 0}
		case "medium":
			return map[string]any{"thinkingBudget": 8192}
		case "high":
			return map[string]any{"thinkingBudget": 32768}
		case "max":
			return map[string]any{"thinkingBudget": 65536}
		}
	}
	if budgetTokens > 0 {
		return map[string]any{"thinkingBudget": budgetTokens}
	}
	return nil
}

// ResolveGeminiThinkingConfigFromRequest extracts thinking config from request params.
func ResolveGeminiThinkingConfigFromRequest(modelName string, body map[string]any) map[string]any {
	// Check if model supports thinking levels
	gc, _ := body["generationConfig"].(map[string]any)
	if gc != nil {
		if tc := gc["thinkingConfig"]; tc != nil {
			return nil // User already provided explicit config
		}
	}

	// Check for reasoning params
	effort := shared.AsTrimmedString(body["reasoning_effort"])
	budget := 0
	if n, ok := body["reasoning_budget"].(float64); ok {
		budget = int(n)
	}

	return ReasoningToThinkingConfig(effort, budget)
}

func mergeThinkingConfig(current any, derived map[string]any) map[string]any {
	cm, _ := current.(map[string]any)
	if cm == nil {
		return cloneMapSimple(derived)
	}
	// Current takes precedence for thinking fields
	merged := cloneMapSimple(cm)
	for k, v := range derived {
		if merged[k] == nil {
			merged[k] = v
		}
	}
	return merged
}

// --- Cloning helpers ---

func cloneJSONValue(v any) any {
	if v == nil {
		return nil
	}
	if m, ok := v.(map[string]any); ok {
		return cloneMapSimple(m)
	}
	if arr, ok := v.([]any); ok {
		out := make([]any, len(arr))
		for i, item := range arr {
			out[i] = cloneJSONValue(item)
		}
		return out
	}
	return v
}

func cloneGeminiContents(v any) any {
	arr, ok := v.([]any)
	if !ok {
		return v
	}
	var out []any
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		next := cloneMapSimple(m)
		if parts, ok := m["parts"].([]any); ok {
			var clonedParts []any
			for _, p := range parts {
				clonedParts = append(clonedParts, cloneJSONValue(p))
			}
			next["parts"] = clonedParts
		}
		out = append(out, next)
	}
	return out
}

func cloneGeminiGenerationConfig(v any) any {
	m, ok := v.(map[string]any)
	if !ok {
		return v
	}
	allowedKeys := []string{
		"stopSequences", "responseModalities", "responseMimeType", "responseSchema",
		"candidateCount", "maxOutputTokens", "temperature", "topP", "topK",
		"presencePenalty", "frequencyPenalty", "seed", "responseLogprobs", "logprobs",
		"thinkingConfig", "imageConfig",
	}
	next := map[string]any{}
	for _, k := range allowedKeys {
		if v, ok := m[k]; ok {
			if k == "thinkingConfig" {
				next[k] = cloneThinkingConfig(v)
				if next[k] == nil {
					next[k] = cloneJSONValue(v)
				}
			} else {
				next[k] = cloneJSONValue(v)
			}
		}
	}
	if len(next) == 0 {
		return nil
	}
	return next
}

func cloneGeminiTools(v any) any {
	arr, ok := v.([]any)
	if !ok {
		return v
	}
	var out []any
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		next := map[string]any{}
		if m["functionDeclarations"] != nil {
			next["functionDeclarations"] = cloneJSONValue(m["functionDeclarations"])
		}
		if m["googleSearch"] != nil {
			next["googleSearch"] = cloneJSONValue(m["googleSearch"])
		}
		if m["urlContext"] != nil {
			next["urlContext"] = cloneJSONValue(m["urlContext"])
		}
		if m["codeExecution"] != nil {
			next["codeExecution"] = cloneJSONValue(m["codeExecution"])
		}
		if len(next) == 0 {
			next = cloneMapSimple(m)
		}
		out = append(out, next)
	}
	return out
}

func cloneThinkingConfig(v any) any {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	next := map[string]any{}
	for k, val := range m {
		if k == "thinkingLevel" {
			n := strings.ToLower(shared.AsTrimmedString(val))
			if n == "minimal" {
				next[k] = "low"
			} else if n != "" {
				next[k] = n
			}
		} else if k == "thinkingBudget" {
			if n, ok := val.(float64); ok {
				next[k] = max(0, int(n))
			}
		} else {
			next[k] = cloneJSONValue(val)
		}
	}
	if len(next) == 0 {
		return nil
	}
	return next
}

func cloneMapSimple(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
