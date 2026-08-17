package proxyhandler

import (
	"math"
	"testing"
)

func TestEstimateBillingCostFromUsage_FallbackPositive(t *testing.T) {
	got := EstimateBillingCostFromUsage("gpt-4o", "openai", ParsedUsage{
		PromptTokens: 1000, CompletionTokens: 500, TotalTokens: 1500, Found: true, Source: "upstream",
	})
	if got.EstimatedCost <= 0 {
		t.Fatalf("expected positive cost, got %v", got.EstimatedCost)
	}
	if got.BillingDetails == nil {
		t.Fatal("nil details")
	}
}

func TestEstimateBillingCostFromUsage_ClaudeCacheFields(t *testing.T) {
	got := EstimateBillingCostFromUsage("claude-sonnet-4", "anthropic", ParsedUsage{
		PromptTokens: 100, CompletionTokens: 50, CacheReadTokens: 20, CacheCreationTokens: 10,
		Found: true, Source: "upstream",
	})
	if got.BillingDetails == nil {
		t.Fatal("nil details")
	}

	breakdown, ok := got.BillingDetails["breakdown"].(map[string]any)
	if !ok {
		t.Fatalf("breakdown missing: %#v", got.BillingDetails)
	}

	pricing, ok := got.BillingDetails["pricing"].(map[string]any)
	if !ok {
		t.Fatalf("pricing missing: %#v", got.BillingDetails)
	}

	// EstimateBillingCostFromUsage builds PricingModel without CacheRatio pointers.
	// CalculateModelUsageBreakdown must surface Claude Anthropic defaults via
	// ResolveCacheRatio / ResolveCacheCreationRatio (0.1 / 1.25).
	if got := asFloat(t, pricing["cache_ratio"]); got != 0.1 {
		t.Fatalf("claude pricing.cache_ratio=%v want 0.1", got)
	}
	if got := asFloat(t, pricing["cache_creation_ratio"]); got != 1.25 {
		t.Fatalf("claude pricing.cache_creation_ratio=%v want 1.25", got)
	}

	// Cache token costs must be present and positive when cache tokens > 0.
	cacheReadCost := asFloat(t, breakdown["cache_read_cost"])
	if cacheReadCost <= 0 {
		t.Fatalf("cache_read_cost should be > 0 when cache_read_tokens > 0, got %v", cacheReadCost)
	}
	cacheCreationCost := asFloat(t, breakdown["cache_creation_cost"])
	if cacheCreationCost <= 0 {
		t.Fatalf("cache_creation_cost should be > 0 when cache_creation_tokens > 0, got %v", cacheCreationCost)
	}
}

func TestEstimateBillingCostFromUsage_NonClaudeMissingCacheRatiosStayOne(t *testing.T) {
	got := EstimateBillingCostFromUsage("gpt-4o", "openai", ParsedUsage{
		PromptTokens: 100, CompletionTokens: 50, CacheReadTokens: 20, CacheCreationTokens: 10,
		Found: true, Source: "upstream",
	})
	if got.BillingDetails == nil {
		t.Fatal("nil details")
	}

	pricing, ok := got.BillingDetails["pricing"].(map[string]any)
	if !ok {
		t.Fatalf("pricing missing: %#v", got.BillingDetails)
	}

	// Non-Claude models keep historical Metapi fallback of 1.0 when ratios are missing.
	if got := asFloat(t, pricing["cache_ratio"]); got != 1.0 {
		t.Fatalf("non-claude pricing.cache_ratio=%v want 1.0", got)
	}
	if got := asFloat(t, pricing["cache_creation_ratio"]); got != 1.0 {
		t.Fatalf("non-claude pricing.cache_creation_ratio=%v want 1.0", got)
	}

	breakdown, ok := got.BillingDetails["breakdown"].(map[string]any)
	if !ok {
		t.Fatalf("breakdown missing: %#v", got.BillingDetails)
	}
	if asFloat(t, breakdown["cache_read_cost"]) <= 0 {
		t.Fatalf("cache_read_cost should be present and > 0 when tokens > 0: %#v", breakdown)
	}
	if asFloat(t, breakdown["cache_creation_cost"]) <= 0 {
		t.Fatalf("cache_creation_cost should be present and > 0 when tokens > 0: %#v", breakdown)
	}
}

func TestEstimateBillingCostFromUsage_NoCacheDetailFullPrice(t *testing.T) {
	// Tokens present but no cache detail → full-price tier: every input token
	// at the input rate, every output token at the output rate, no cache keys.
	got := EstimateBillingCostFromUsage("gpt-4o", "openai", ParsedUsage{
		PromptTokens: 1000, CompletionTokens: 500, TotalTokens: 1500,
		Found: true, Source: "upstream",
	})
	if got.BillingDetails == nil {
		t.Fatal("nil details")
	}

	pricing, ok := got.BillingDetails["pricing"].(map[string]any)
	if !ok {
		t.Fatalf("pricing missing: %#v", got.BillingDetails)
	}
	if src, _ := pricing["source"].(string); src != "full_price_fallback" {
		t.Fatalf("pricing.source=%v want full_price_fallback", src)
	}
	// Full-price tier must not advertise cache discounts.
	if _, present := pricing["cache_ratio"]; present {
		t.Fatalf("full-price tier must not emit cache_ratio: %#v", pricing)
	}
	if _, present := pricing["cache_creation_ratio"]; present {
		t.Fatalf("full-price tier must not emit cache_creation_ratio: %#v", pricing)
	}

	breakdown, ok := got.BillingDetails["breakdown"].(map[string]any)
	if !ok {
		t.Fatalf("breakdown missing: %#v", got.BillingDetails)
	}
	// input = 1000 × 2/1e6, output = 500 × 2/1e6 (default ratio 1).
	if got := asFloat(t, breakdown["input_cost"]); math.Abs(got-0.002) > 1e-9 {
		t.Fatalf("input_cost=%v want 0.002", got)
	}
	if got := asFloat(t, breakdown["output_cost"]); math.Abs(got-0.001) > 1e-9 {
		t.Fatalf("output_cost=%v want 0.001", got)
	}
	if got := asFloat(t, breakdown["cache_read_cost"]); got != 0 {
		t.Fatalf("cache_read_cost=%v want 0", got)
	}
	if got := asFloat(t, breakdown["cache_creation_cost"]); got != 0 {
		t.Fatalf("cache_creation_cost=%v want 0", got)
	}
	if math.Abs(got.EstimatedCost-0.003) > 1e-9 {
		t.Fatalf("EstimatedCost=%v want 0.003", got.EstimatedCost)
	}
}

func TestEstimateBillingCostFromUsage_NoCacheDetailClaudeStillFullPrice(t *testing.T) {
	// Claude without cache detail must also bill the whole input at the input
	// rate — no Anthropic cache-ratio defaults leak into a no-cache request.
	got := EstimateBillingCostFromUsage("claude-sonnet-4", "anthropic", ParsedUsage{
		PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150,
		Found: true, Source: "upstream",
	})
	if got.BillingDetails == nil {
		t.Fatal("nil details")
	}

	pricing, ok := got.BillingDetails["pricing"].(map[string]any)
	if !ok {
		t.Fatalf("pricing missing: %#v", got.BillingDetails)
	}
	if src, _ := pricing["source"].(string); src != "full_price_fallback" {
		t.Fatalf("pricing.source=%v want full_price_fallback", src)
	}
	if _, present := pricing["cache_ratio"]; present {
		t.Fatalf("full-price tier must not emit cache_ratio: %#v", pricing)
	}

	breakdown, ok := got.BillingDetails["breakdown"].(map[string]any)
	if !ok {
		t.Fatalf("breakdown missing: %#v", got.BillingDetails)
	}
	// input = 100 × 2/1e6, output = 50 × 2/1e6.
	if got := asFloat(t, breakdown["input_cost"]); math.Abs(got-0.0002) > 1e-9 {
		t.Fatalf("input_cost=%v want 0.0002", got)
	}
	if got := asFloat(t, breakdown["output_cost"]); math.Abs(got-0.0001) > 1e-9 {
		t.Fatalf("output_cost=%v want 0.0001", got)
	}
	if math.Abs(got.EstimatedCost-0.0003) > 1e-9 {
		t.Fatalf("EstimatedCost=%v want 0.0003", got.EstimatedCost)
	}
}

func TestEstimateBillingCostFromUsage_NotFoundStaysDivisionFallback(t *testing.T) {
	// No token fields at all → the original fallback_token_cost tier stays.
	got := EstimateBillingCostFromUsage("gpt-4o", "openai", ParsedUsage{
		TotalTokens: 1500, Source: "unknown",
	})
	if got.BillingDetails == nil {
		t.Fatal("nil details")
	}
	pricing, ok := got.BillingDetails["pricing"].(map[string]any)
	if !ok {
		t.Fatalf("pricing missing: %#v", got.BillingDetails)
	}
	if src, _ := pricing["source"].(string); src != "fallback_token_cost" {
		t.Fatalf("pricing.source=%v want fallback_token_cost", src)
	}
	breakdown, ok := got.BillingDetails["breakdown"].(map[string]any)
	if !ok {
		t.Fatalf("breakdown missing: %#v", got.BillingDetails)
	}
	if _, present := breakdown["input_cost"]; present {
		t.Fatalf("fallback tier must not emit input_cost: %#v", breakdown)
	}
	if math.Abs(got.EstimatedCost-0.003) > 1e-9 {
		t.Fatalf("EstimatedCost=%v want 0.003 (1500/500000)", got.EstimatedCost)
	}
}

func asFloat(t *testing.T, v any) float64 {
	t.Helper()
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		t.Fatalf("expected numeric value, got %T (%#v)", v, v)
		return 0
	}
}
