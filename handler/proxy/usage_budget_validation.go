package proxyhandler

import (
	"encoding/json"
	"math"
	"strings"
)

func validUserBudgetUsageMap(usage map[string]any, gemini bool) bool {
	for key, value := range usage {
		if gemini {
			switch key {
			case "promptTokensDetails", "candidatesTokensDetails", "cacheTokensDetails", "toolUsePromptTokensDetails":
				if !validGeminiTokenDetails(value) {
					return false
				}
				continue
			}
		}
		isCounter := strings.Contains(strings.ToLower(key), "token")
		isDetails := strings.HasSuffix(key, "_tokens_details")
		if nested, ok := value.(map[string]any); ok {
			if isCounter && !isDetails {
				return false
			}
			if !validUserBudgetUsageMap(nested, false) {
				return false
			}
			continue
		}
		if isDetails {
			return false
		}
		if isCounter {
			number, ok := value.(json.Number)
			n, err := number.Float64()
			// JSON token counters must be exact nonnegative safe integers. The
			// upper bound also prevents overflow when exclusive cache counters
			// are added by the compatibility parser.
			if !ok || err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > 1<<50 || math.Trunc(n) != n {
				return false
			}
		}
	}
	return true
}

// Gemini ModalityTokenCount entries describe subsets of the top-level counts.
func validGeminiTokenDetails(value any) bool {
	entries, ok := value.([]any)
	if !ok {
		return false
	}
	for _, entry := range entries {
		obj, ok := entry.(map[string]any)
		if !ok || len(obj) != 2 {
			return false
		}
		switch obj["modality"] {
		case "TEXT", "IMAGE", "AUDIO", "VIDEO", "DOCUMENT", "MODALITY_UNSPECIFIED":
		default:
			return false
		}
		number, ok := obj["tokenCount"].(json.Number)
		if !ok {
			return false
		}
		n, err := number.Int64()
		if err != nil || n < 0 || n > 1<<50 {
			return false
		}
	}
	return true
}
