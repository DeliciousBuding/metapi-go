package admin

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"

	"github.com/deliciousbuding/metapi-go/routing"
)

func applyBillingDetailsToPriceInput(in *routing.PriceCompareInput, raw string) {
	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return
	}
	breakdown := firstMap(parsed, "breakdown", "Breakdown")
	pricing := firstMap(parsed, "pricing", "Pricing")

	if breakdown != nil {
		if v, ok := floatFromAny(firstAny(breakdown, "inputPerMillion", "input_per_million", "InputPerMillion")); ok {
			in.InputPerMillion = &v
		}
		if v, ok := floatFromAny(firstAny(breakdown, "outputPerMillion", "output_per_million", "OutputPerMillion")); ok {
			in.OutputPerMillion = &v
		}
	}
	if pricing != nil {
		if v, ok := floatFromAny(firstAny(pricing, "modelRatio", "model_ratio", "ModelRatio")); ok {
			in.ModelRatio = &v
		}
		if v, ok := floatFromAny(firstAny(pricing, "completionRatio", "completion_ratio", "CompletionRatio")); ok {
			in.CompletionRatio = &v
		}
		if v, ok := floatFromAny(firstAny(pricing, "groupRatio", "group_ratio", "GroupRatio")); ok {
			in.GroupRatio = &v
		}
	}
}

func firstMap(m map[string]any, keys ...string) map[string]any {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if mm, ok2 := v.(map[string]any); ok2 {
				return mm
			}
		}
	}
	return nil
}

func firstAny(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v
		}
	}
	return nil
}

func floatFromAny(v any) (float64, bool) {
	if v == nil {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return 0, false
		}
		return n, true
	case float32:
		f := float64(n)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, false
		}
		return f, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, false
		}
		return f, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}
