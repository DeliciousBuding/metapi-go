package generate_content

import (
	"strings"

	"github.com/deliciousbuding/metapi-go/transform/shared"
)

// AllowedGeminiKeys are the 9 keys forwarded to the Gemini API.
var AllowedGeminiKeys = map[string]bool{
	"contents": true, "systemInstruction": true, "cachedContent": true,
	"safetySettings": true, "generationConfig": true, "tools": true,
	"toolConfig": true, "labels": true, "model": true,
}

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

func cloneJSONValue(v any) any {
	if v == nil {
		return nil
	}
	if m, ok := v.(map[string]any); ok {
		return shared.CloneObject(m)
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
		next := shared.CloneObject(m)
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
		"stopSequences", "responseModalities", "responseMimeType", "responseSchema", "responseJsonSchema",
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
			next = shared.CloneObject(m)
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
