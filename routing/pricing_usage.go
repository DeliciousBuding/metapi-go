package routing

import (
	"math"

	"github.com/deliciousbuding/metapi-go/internal/toolusage"
)

// PricingModel is a normalized upstream pricing row used for cost estimation.
// Pointer ratios distinguish "missing" from an explicit zero.
type PricingModel struct {
	ModelName             string
	QuotaType             int // 0 = token-based, 1 = per-call
	ModelRatio            float64
	CompletionRatio       float64
	CacheRatio            *float64
	CacheCreationRatio    *float64
	CacheCreation1hRatio  *float64
	CacheCreation12hRatio *float64
	// ToolUnitPrices is currency per single unit, keyed by "tool.unit"
	// (e.g. "grounding.queries"). Group pricing applies once at settlement.
	ToolUnitPrices map[string]float64
	ModelPrice     *ModelPrice
	EnableGroups   []string
}

// ModelPrice is either a flat per-call price or input/output pair.
type ModelPrice struct {
	// Flat is set for scalar model_price values.
	Flat *float64
	// Input/Output are set for object model_price values (one-hub times).
	Input  float64
	Output float64
	IsPair bool
}

// UsageForCost is token usage for a single request cost estimate.
type UsageForCost struct {
	PromptTokens           int64
	CompletionTokens       int64
	TotalTokens            int64
	CacheReadTokens        int64
	CacheCreationTokens    int64
	CacheCreation5mTokens  int64
	CacheCreation1hTokens  int64
	CacheCreation12hTokens int64
	ToolUsage              []ToolUsageChargeInput
	// PromptTokensIncludeCache:
	// true/nil → prompt tokens already include cache tokens (subtract for billable input)
	// false → prompt tokens are exclusive of cache tokens
	PromptTokensIncludeCache *bool
}

// ToolUsageChargeInput accepts Tracker.Snapshot() without a conversion.
type ToolUsageChargeInput = toolusage.ToolUsageItem

type ProxyToolSurcharge struct {
	ToolUsageChargeInput
	// Nil denotes an unpriced observation; a pointer to zero denotes free use.
	UnitPrice *float64 `json:"unitPrice,omitempty"`
	Cost      float64  `json:"cost"`
}

// ProxyBillingDetails mirrors the TS ProxyBillingDetails shape (camelCase JSON).
type ProxyBillingDetails struct {
	QuotaType int                   `json:"quotaType"`
	Usage     ProxyBillingUsage     `json:"usage"`
	Pricing   ProxyBillingPricing   `json:"pricing"`
	Breakdown ProxyBillingBreakdown `json:"breakdown"`
}

// ProxyBillingUsage is the usage section of billing details.
type ProxyBillingUsage struct {
	PromptTokens             int64 `json:"promptTokens"`
	CompletionTokens         int64 `json:"completionTokens"`
	TotalTokens              int64 `json:"totalTokens"`
	CacheReadTokens          int64 `json:"cacheReadTokens"`
	CacheCreationTokens      int64 `json:"cacheCreationTokens"`
	CacheCreation5mTokens    int64 `json:"cacheCreation5mTokens,omitempty"`
	CacheCreation1hTokens    int64 `json:"cacheCreation1hTokens,omitempty"`
	CacheCreation12hTokens   int64 `json:"cacheCreation12hTokens,omitempty"`
	BillablePromptTokens     int64 `json:"billablePromptTokens"`
	PromptTokensIncludeCache *bool `json:"promptTokensIncludeCache"`
}

// ProxyBillingPricing is the ratio section of billing details.
type ProxyBillingPricing struct {
	ModelRatio         float64 `json:"modelRatio"`
	CompletionRatio    float64 `json:"completionRatio"`
	CacheRatio         float64 `json:"cacheRatio"`
	CacheCreationRatio float64 `json:"cacheCreationRatio"`
	GroupRatio         float64 `json:"groupRatio"`
}

// ProxyBillingBreakdown is the money breakdown of billing details.
type ProxyBillingBreakdown struct {
	InputPerMillion         float64              `json:"inputPerMillion"`
	OutputPerMillion        float64              `json:"outputPerMillion"`
	CacheReadPerMillion     float64              `json:"cacheReadPerMillion"`
	CacheCreationPerMillion float64              `json:"cacheCreationPerMillion"`
	InputCost               float64              `json:"inputCost"`
	OutputCost              float64              `json:"outputCost"`
	CacheReadCost           float64              `json:"cacheReadCost"`
	CacheCreationCost       float64              `json:"cacheCreationCost"`
	CacheCreation5mCost     float64              `json:"cacheCreation5mCost,omitempty"`
	CacheCreation1hCost     float64              `json:"cacheCreation1hCost,omitempty"`
	CacheCreation12hCost    float64              `json:"cacheCreation12hCost,omitempty"`
	ToolSurcharges          []ProxyToolSurcharge `json:"toolSurcharges,omitempty"`
	TotalCost               float64              `json:"totalCost"`
}

func normalizeUsageBreakdownInput(usage UsageForCost) ProxyBillingUsage {
	promptTokens := toPositiveInt64(usage.PromptTokens)
	completionTokens := toPositiveInt64(usage.CompletionTokens)
	totalTokensRaw := toPositiveInt64(usage.TotalTokens)
	totalTokens := totalTokensRaw
	// Saturate instead of wrapping when prompt+completion overflows int64: a
	// wrapped (negative) sum would silently skip the total repair and leave
	// TotalTokens smaller than its own components.
	if completionTokens > math.MaxInt64-promptTokens {
		totalTokens = math.MaxInt64
	} else if promptTokens+completionTokens > totalTokens {
		totalTokens = promptTokens + completionTokens
	}
	cacheReadTokens := toPositiveInt64(usage.CacheReadTokens)
	cacheCreationTokens := toPositiveInt64(usage.CacheCreationTokens)
	// Each TTL is a subset of the write total; ordinary writes are the rest.
	subset := func(n, remaining int64) int64 {
		if n < 0 || n > toolusage.MaxCount {
			return 0
		}
		return min(n, remaining)
	}
	long := subset(usage.CacheCreation12hTokens, cacheCreationTokens)
	hour := subset(usage.CacheCreation1hTokens, cacheCreationTokens-long)
	five := subset(usage.CacheCreation5mTokens, cacheCreationTokens-long-hour)
	promptTokensIncludeCache := usage.PromptTokensIncludeCache

	hasSplit := promptTokens > 0 || completionTokens > 0
	effectivePromptTokens := promptTokens
	if !hasSplit {
		effectivePromptTokens = totalTokens
	}

	var billablePromptTokens int64
	if promptTokensIncludeCache != nil && !*promptTokensIncludeCache {
		billablePromptTokens = effectivePromptTokens
	} else {
		billable := effectivePromptTokens - cacheReadTokens - cacheCreationTokens
		if billable < 0 {
			billable = 0
		}
		billablePromptTokens = billable
	}

	return ProxyBillingUsage{
		PromptTokens:             promptTokens,
		CompletionTokens:         completionTokens,
		TotalTokens:              totalTokens,
		CacheReadTokens:          cacheReadTokens,
		CacheCreationTokens:      cacheCreationTokens,
		CacheCreation5mTokens:    five,
		CacheCreation1hTokens:    hour,
		CacheCreation12hTokens:   long,
		BillablePromptTokens:     billablePromptTokens,
		PromptTokensIncludeCache: promptTokensIncludeCache,
	}
}

func toPositiveInt64(v int64) int64 {
	if v < 0 {
		return 0
	}
	return v
}
