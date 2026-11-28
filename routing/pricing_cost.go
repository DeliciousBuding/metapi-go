package routing

import (
	"math"
	"strings"

	"github.com/deliciousbuding/metapi-go/internal/toolusage"
)

// CalculateModelUsageBreakdown computes token-based billing details.
// Returns nil for per-call quotaType (1), matching TS behavior.
func CalculateModelUsageBreakdown(
	model PricingModel,
	usage UsageForCost,
	groupRatio map[string]float64,
) *ProxyBillingDetails {
	if model.QuotaType == 1 {
		return nil
	}

	multiplier := resolveGroupMultiplier(model, groupRatio)
	normalized := normalizeUsageBreakdownInput(usage)

	cacheRatio := ResolveCacheRatio(model.ModelName, model.CacheRatio)
	cacheCreationRatio := ResolveCacheCreationRatio(model.ModelName, model.CacheCreationRatio)

	modelRatio := model.ModelRatio
	if modelRatio <= 0 || !isFiniteFloat(modelRatio) {
		modelRatio = 1
	}
	completionRatio := model.CompletionRatio
	if completionRatio <= 0 || !isFiniteFloat(completionRatio) {
		completionRatio = 1
	}

	inputPerMillion := roundCost(modelRatio * 2 * multiplier)
	outputPerMillion := roundCost(modelRatio * completionRatio * 2 * multiplier)
	cacheReadPerMillion := roundCost(modelRatio * cacheRatio * 2 * multiplier)
	cacheCreationPerMillion := roundCost(modelRatio * cacheCreationRatio * 2 * multiplier)

	inputCost := roundCost((float64(normalized.BillablePromptTokens) / 1_000_000) * inputPerMillion)
	outputCost := roundCost((float64(normalized.CompletionTokens) / 1_000_000) * outputPerMillion)
	cacheReadCost := roundCost((float64(normalized.CacheReadTokens) / 1_000_000) * cacheReadPerMillion)

	details := &ProxyBillingDetails{
		QuotaType: model.QuotaType,
		Usage:     normalized,
		Pricing: ProxyBillingPricing{
			ModelRatio:         modelRatio,
			CompletionRatio:    completionRatio,
			CacheRatio:         cacheRatio,
			CacheCreationRatio: cacheCreationRatio,
			GroupRatio:         multiplier,
		},
		Breakdown: ProxyBillingBreakdown{
			InputPerMillion:         inputPerMillion,
			OutputPerMillion:        outputPerMillion,
			CacheReadPerMillion:     cacheReadPerMillion,
			CacheCreationPerMillion: cacheCreationPerMillion,
			InputCost:               inputCost,
			OutputCost:              outputCost,
			CacheReadCost:           cacheReadCost,
		},
	}
	b := &details.Breakdown
	b.priceCacheWrites(normalized, model, modelRatio, multiplier, cacheCreationRatio)
	b.TotalCost = roundCost(inputCost + outputCost + cacheReadCost + b.CacheCreationCost)
	b.addToolSurcharges(model, usage.ToolUsage, multiplier)
	return details
}

func (b *ProxyBillingBreakdown) priceCacheWrites(u ProxyBillingUsage, model PricingModel, modelRatio, multiplier, baseRatio float64) {
	ordinary := u.CacheCreationTokens - u.CacheCreation5mTokens - u.CacheCreation1hTokens - u.CacheCreation12hTokens
	price := func(tokens int64, rate float64) float64 { return float64(tokens) / 1_000_000 * rate }
	if ordinary == u.CacheCreationTokens {
		b.CacheCreationCost = roundCost(price(ordinary, b.CacheCreationPerMillion))
		return
	}
	tierRate := func(override *float64, fallback float64) float64 {
		ratio := fallback
		if override != nil && isFiniteNonNeg(*override) {
			ratio = *override
		}
		return roundCost(modelRatio * ratio * 2 * multiplier)
	}
	five := price(u.CacheCreation5mTokens, b.CacheCreationPerMillion)
	hour := price(u.CacheCreation1hTokens, tierRate(model.CacheCreation1hRatio, baseRatio*cacheWrite1hWeight))
	long := price(u.CacheCreation12hTokens, tierRate(model.CacheCreation12hRatio, baseRatio*cacheWrite12hWeight))
	b.CacheCreation5mCost = roundCost(five)
	b.CacheCreation1hCost = roundCost(hour)
	b.CacheCreation12hCost = roundCost(long)
	// Round the aggregate once, including sub-microcurrency tier quantities.
	b.CacheCreationCost = roundCost(price(ordinary, b.CacheCreationPerMillion) + five + hour + long)
}

// PriceCatalogCacheWrites applies an absolute write rate (including free use)
// to the normalized token split using the same TTL weights as ratio pricing.
func (b *ProxyBillingBreakdown) PriceCatalogCacheWrites(u ProxyBillingUsage, rate float64) {
	ordinary := u.CacheCreationTokens - u.CacheCreation5mTokens - u.CacheCreation1hTokens - u.CacheCreation12hTokens
	b.CacheCreation5mCost = float64(u.CacheCreation5mTokens) * rate / 1e6
	b.CacheCreation1hCost = float64(u.CacheCreation1hTokens) * rate * cacheWrite1hWeight / 1e6
	b.CacheCreation12hCost = float64(u.CacheCreation12hTokens) * rate * cacheWrite12hWeight / 1e6
	b.CacheCreationCost = float64(ordinary)*rate/1e6 + b.CacheCreation5mCost + b.CacheCreation1hCost + b.CacheCreation12hCost
}

func (b *ProxyBillingBreakdown) addToolSurcharges(model PricingModel, items []ToolUsageChargeInput, multiplier float64) {
	if len(items) == 0 {
		return
	}
	for _, item := range items {
		if item.Quantity < 0 || item.Quantity > toolusage.MaxCount {
			continue
		}
		line := ProxyToolSurcharge{ToolUsageChargeInput: item}
		if price, ok := model.ToolUnitPrices[item.Tool+"."+item.Unit]; ok && isFiniteNonNeg(price) {
			line.UnitPrice = &price
			line.Cost = roundCost(float64(item.Quantity) * price * multiplier)
			b.TotalCost += line.Cost
		}
		b.ToolSurcharges = append(b.ToolSurcharges, line)
	}
	b.TotalCost = roundCost(b.TotalCost)
}

// CalculateModelUsageFullPrice bills the no-cache-detail tier: every input
// token at the input rate and every output token at the output rate, with no
// cache discount. Upstreams that report token usage without cache detail must
// not be discounted — octopus semantics (internal/relay/metrics.go): missing
// cache detail means nonCachedTokens = promptTokens, so the whole input side
// is billed at full input price. This tier sits between FallbackTokenCost
// (token-divisor estimate) and CalculateModelUsageBreakdown (cache-aware
// three-way split). Unit prices reuse the existing conversion
// (inputPerMillion = modelRatio × 2 × multiplier) via CacheAwarePerMillionRates
// instead of hardcoding new formulas. Returns nil for per-call quotaType (1),
// matching CalculateModelUsageBreakdown.
func CalculateModelUsageFullPrice(
	model PricingModel,
	usage UsageForCost,
	groupRatio map[string]float64,
) *ProxyBillingDetails {
	if model.QuotaType == 1 {
		return nil
	}

	multiplier := resolveGroupMultiplier(model, groupRatio)
	normalized := normalizeUsageBreakdownInput(usage)

	inputPerMillion, outputPerMillion, _, _ := CacheAwarePerMillionRates(model, multiplier)

	// No cache subtraction: bill the entire input side at the input rate.
	// When upstream reported only a total (no prompt/completion split), the
	// total is billed as input — the conservative choice, matching the
	// effectivePrompt selection in normalizeUsageBreakdownInput.
	inputTokens := normalized.PromptTokens
	if normalized.PromptTokens == 0 && normalized.CompletionTokens == 0 {
		inputTokens = normalized.TotalTokens
	}
	inputCost := roundCost((float64(inputTokens) / 1_000_000) * inputPerMillion)
	outputCost := roundCost((float64(normalized.CompletionTokens) / 1_000_000) * outputPerMillion)
	totalCost := roundCost(inputCost + outputCost)

	// Reflect that the full input side was billed without cache discounts.
	normalized.BillablePromptTokens = inputTokens

	modelRatio := model.ModelRatio
	if modelRatio <= 0 || !isFiniteFloat(modelRatio) {
		modelRatio = 1
	}
	completionRatio := model.CompletionRatio
	if completionRatio <= 0 || !isFiniteFloat(completionRatio) {
		completionRatio = 1
	}

	details := &ProxyBillingDetails{
		QuotaType: model.QuotaType,
		Usage:     normalized,
		Pricing: ProxyBillingPricing{
			ModelRatio:         modelRatio,
			CompletionRatio:    completionRatio,
			CacheRatio:         0,
			CacheCreationRatio: 0,
			GroupRatio:         multiplier,
		},
		Breakdown: ProxyBillingBreakdown{
			InputPerMillion:         inputPerMillion,
			OutputPerMillion:        outputPerMillion,
			CacheReadPerMillion:     0,
			CacheCreationPerMillion: 0,
			InputCost:               inputCost,
			OutputCost:              outputCost,
			CacheReadCost:           0,
			CacheCreationCost:       0,
			TotalCost:               totalCost,
		},
	}
	details.Breakdown.addToolSurcharges(model, usage.ToolUsage, multiplier)
	return details
}

// CalculateModelUsageCost returns the estimated cost for a model + usage.
func CalculateModelUsageCost(
	model PricingModel,
	usage UsageForCost,
	groupRatio map[string]float64,
) float64 {
	multiplier := resolveGroupMultiplier(model, groupRatio)
	if model.QuotaType == 1 {
		b := ProxyBillingBreakdown{TotalCost: roundCost(calculatePerCallCost(model.ModelPrice, multiplier))}
		b.addToolSurcharges(model, usage.ToolUsage, multiplier)
		return b.TotalCost
	}
	detail := CalculateModelUsageBreakdown(model, usage, groupRatio)
	if detail == nil {
		return 0
	}
	return detail.Breakdown.TotalCost
}

// FallbackTokenCost is the last-resort estimate when no pricing catalog is
// available. Mirrors TS fallbackTokenCost (veloera uses 1e6 divisor, others 5e5).
func FallbackTokenCost(totalTokens int64, platform string) float64 {
	divisor := 500_000.0
	if strings.EqualFold(strings.TrimSpace(platform), "veloera") {
		divisor = 1_000_000.0
	}
	tokens := float64(toPositiveInt64(totalTokens))
	return roundCost(tokens / divisor)
}

func resolveGroupMultiplier(model PricingModel, groupRatio map[string]float64) float64 {
	if groupRatio == nil {
		groupRatio = map[string]float64{}
	}
	groups := model.EnableGroups
	if len(groups) == 0 {
		groups = []string{"default"}
	}

	if containsString(groups, "default") {
		if r, ok := groupRatio["default"]; ok && r > 0 && isFiniteFloat(r) {
			return r
		}
	}
	for _, g := range groups {
		if r, ok := groupRatio[g]; ok && r > 0 && isFiniteFloat(r) {
			return r
		}
	}
	for _, r := range groupRatio {
		if r > 0 && isFiniteFloat(r) {
			return r
		}
	}
	return 1
}

func calculatePerCallCost(price *ModelPrice, multiplier float64) float64 {
	if price == nil {
		return 0
	}
	if price.Flat != nil {
		return *price.Flat * multiplier
	}
	if price.IsPair {
		// done-hub/one-hub times pricing follows input ratio only.
		return price.Input * multiplier * oneHubPerCallRatio
	}
	return 0
}

func roundCost(value float64) float64 {
	if value < 0 || !isFiniteFloat(value) {
		return 0
	}
	return math.Round(value*1_000_000) / 1_000_000
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
