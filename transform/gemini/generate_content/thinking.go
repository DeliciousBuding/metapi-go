package generate_content

import (
	"strings"

	"github.com/deliciousbuding/metapi-go/transform/shared"
)

// DummyThoughtSignature is the base64 sentinel for "skip_thought_signature_validator".
// Gemini accepts any non-empty base64 string when a real thought signature is unavailable.
const DummyThoughtSignature = "c2tpcF90aG91Z2h0X3NpZ25hdHVyZV92YWxpZGF0b3I="

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
		return shared.CloneObject(derived)
	}
	// Current takes precedence for thinking fields
	merged := shared.CloneObject(cm)
	for k, v := range derived {
		if merged[k] == nil {
			merged[k] = v
		}
	}
	return merged
}
