// Package pricing provides pure, database-free functions for normalizing
// upstream pricing signals into a canonical product model and converting
// pricing multipliers (ratios) into USD per million tokens. It is shared by
// the admin price-comparison surface so every consumer applies the same math
// and never invents an unlabeled price.
package pricing

import (
	"math"
	"strings"
)

// BaseUSDPerMillionInput is the base USD per 1M input tokens at model ratio
// 1.0. It matches routing.CacheAwarePerMillionRates
// (input = modelRatio x 2 x groupMultiplier) so the canonical layer and the
// routing layer agree on the same effective rate.
const BaseUSDPerMillionInput = 2.0

// ProductCanonicalModel is a normalized cross-site product model with
// effective USD-per-million input/output rates. It is the reusable shape
// consumed by price comparison rather than a wire type.
type ProductCanonicalModel struct {
	ModelName        string  `json:"modelName"`
	InputPerMillion  float64 `json:"inputPerMillion"`
	OutputPerMillion float64 `json:"outputPerMillion"`
}

// NormalizeInput carries the raw pricing signals for one model. Pointer
// fields distinguish "missing" from an explicit zero.
type NormalizeInput struct {
	ModelName        string
	InputPerMillion  *float64 // explicit $/M input (already normalized)
	OutputPerMillion *float64 // explicit $/M output
	ModelRatio       *float64
	CompletionRatio  *float64
	GroupMultiplier  *float64
}

// MultiplierToUSDPerMillion converts a pricing multiplier into USD per 1M
// input tokens. Non-positive, NaN, or Inf multipliers yield 0 (no invented
// price).
func MultiplierToUSDPerMillion(multiplier float64) float64 {
	if !(multiplier > 0) || math.IsNaN(multiplier) || math.IsInf(multiplier, 0) {
		return 0
	}
	return round6(multiplier * BaseUSDPerMillionInput)
}

// NormalizeToProductCanonical resolves raw pricing signals into canonical
// USD-per-million rates. Explicit per-million values win when both sides are
// present; otherwise ratios are converted via MultiplierToUSDPerMillion.
func NormalizeToProductCanonical(in NormalizeInput) ProductCanonicalModel {
	out := ProductCanonicalModel{ModelName: strings.TrimSpace(in.ModelName)}

	inPerM := positiveFloat(in.InputPerMillion)
	outPerM := positiveFloat(in.OutputPerMillion)
	if inPerM != nil && outPerM != nil {
		out.InputPerMillion = round6(*inPerM)
		out.OutputPerMillion = round6(*outPerM)
		return out
	}

	modelRatio := positiveOr(in.ModelRatio, 1)
	completionRatio := positiveOr(in.CompletionRatio, 1)
	groupMultiplier := positiveOr(in.GroupMultiplier, 1)

	out.InputPerMillion = MultiplierToUSDPerMillion(modelRatio * groupMultiplier)
	out.OutputPerMillion = MultiplierToUSDPerMillion(modelRatio * completionRatio * groupMultiplier)
	return out
}

// positiveFloat returns v when it is a finite positive number, else nil.
func positiveFloat(v *float64) *float64 {
	if v == nil || !(*v > 0) || math.IsNaN(*v) || math.IsInf(*v, 0) {
		return nil
	}
	return v
}

// positiveOr returns a finite positive pointer value or the fallback.
func positiveOr(v *float64, fallback float64) float64 {
	if p := positiveFloat(v); p != nil {
		return *p
	}
	return fallback
}

// round6 rounds a non-negative value to six decimal places (matching the
// routing layer's roundCost precision) and clamps negatives/non-finite to 0.
func round6(v float64) float64 {
	if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return math.Round(v*1_000_000) / 1_000_000
}
