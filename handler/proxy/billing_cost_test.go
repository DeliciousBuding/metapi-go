package proxyhandler

import (
	"math"
	"testing"

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service/pricingcatalog"
	"github.com/deliciousbuding/metapi-go/store"
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
	usage := got.BillingDetails["usage"].(map[string]any)
	if usage["billable_prompt_tokens"] != int64(1000) {
		t.Fatalf("billable input not exposed: %v", usage)
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

func TestBillingCatalogRates(t *testing.T) {
	catalog, err := pricingcatalog.ParseCatalog([]byte(`{"vendor":{"models":{"m":{"id":"m","cost":{"input":5,"output":12,"cache_read":0,"cache_write":8}},"free":{"id":"free","cost":{"input":0,"output":0}},"out":{"id":"out","cost":{"input":0,"output":3}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	ratios, err := pricingcatalog.ParseNewAPIRatios([]byte(`{"success":true,"data":{"model_ratio":{"m":2},"completion_ratio":{"m":3},"cache_ratio":{"m":0}}}`))
	if err != nil {
		t.Fatal(err)
	}
	catalog = pricingcatalog.MergeSnapshots([]*pricingcatalog.CatalogSnapshot{catalog, ratios})
	for _, tc := range []struct {
		model  string
		class  pricingcatalog.SiteClass
		usage  ParsedUsage
		want   float64
		source string
	}{
		{"m", pricingcatalog.SiteClassOfficial, ParsedUsage{Found: true, PromptTokens: 1000, CompletionTokens: 100, CacheReadTokens: 200, CacheCreationTokens: 50}, .00535, "catalog"},
		{"m", pricingcatalog.SiteClassOfficial, ParseUsageFromBody([]byte(`{"usage":{"input_tokens":100,"output_tokens":10,"input_tokens_details":{"cached_creation_tokens":50,"cache_write_tokens_details":{"ttl_12h_tokens":20}}}}`)), .00093, "catalog"},
		{"free", pricingcatalog.SiteClassOfficial, ParsedUsage{Found: true, PromptTokens: 100, CacheCreationTokens: 100, CacheCreation12hTokens: 100}, 0, "catalog"},
		{"m", pricingcatalog.SiteClassRelay, ParsedUsage{Found: true, PromptTokens: 1000, CompletionTokens: 100}, .0052, "catalog_estimate"},
		{"m", pricingcatalog.SiteClassRelay, ParsedUsage{Found: true, PromptTokens: 1000, CacheReadTokens: 1000}, 0, "catalog_estimate"},
		{"free", pricingcatalog.SiteClassOfficial, ParsedUsage{Found: true, PromptTokens: 1000}, 0, "catalog"},
		{"out", pricingcatalog.SiteClassRelay, ParsedUsage{Found: true, PromptTokens: 1000, CompletionTokens: 100}, .0003, "catalog_estimate"},
		{"unknown", pricingcatalog.SiteClassRelay, ParsedUsage{Found: true, TotalTokens: 1000}, .002, "full_price_fallback"},
	} {
		got := EstimateBillingCostFromUsageWithCatalog(tc.model, "openai", tc.usage, catalog, tc.class)
		p := got.BillingDetails["pricing"].(map[string]any)
		b := got.BillingDetails["breakdown"].(map[string]any)
		if math.Abs(got.EstimatedCost-tc.want) > 1e-12 || b["total_cost"] != got.EstimatedCost || p["model"] != tc.model || p["source"] != tc.source {
			t.Fatalf("%s %s = %+v want %v", tc.model, tc.class, got, tc.want)
		}
	}
}

func TestSelectedBillingClass(t *testing.T) {
	site := routing.SelectedChannel{Site: store.Site{Platform: "openai", URL: "https://api.openai.com"}}
	if selectedBillingClass(t.Context(), &site) != pricingcatalog.SiteClassOfficial {
		t.Fatal("official site classified as relay")
	}
	endpoint := &store.DirectEndpoint{URL: "https://relay.example/v1"}
	direct := routing.SelectedChannel{Direct: &store.DirectUpstreamCandidate{BaseURL: "https://api.openai.com", Provider: "openai", Endpoints: store.DirectEndpoints{Chat: endpoint}}}
	for _, tc := range []struct {
		wire *directProviderWire
		want pricingcatalog.SiteClass
	}{
		{nil, pricingcatalog.SiteClassRelay},
		{&directProviderWire{Endpoint: endpoint}, pricingcatalog.SiteClassRelay},
		{&directProviderWire{Endpoint: &store.DirectEndpoint{URL: "https://api.openai.com/v1"}}, pricingcatalog.SiteClassOfficial},
		{&directProviderWire{Endpoint: &store.DirectEndpoint{URL: "https://api.openai.com/v1"}, DriverURL: "https://relay.example/v1"}, pricingcatalog.SiteClassRelay},
	} {
		ctx := t.Context()
		if tc.wire != nil {
			ctx = withDirectProviderWire(ctx, tc.wire)
		}
		if got := selectedBillingClass(ctx, &direct); got != tc.want {
			t.Fatalf("direct class=%s want %s", got, tc.want)
		}
	}
}

func TestBillingZeroBreakdownOverridesFallback(t *testing.T) {
	// One Claude cache-read token at the historical $0.20/M default rounds to
	// zero at the ledger's precision. The token-divisor fallback is still positive.
	got := EstimateBillingCostFromUsage("claude-synthetic", "anthropic", ParsedUsage{Found: true, PromptTokens: 1, CacheReadTokens: 1})
	b := got.BillingDetails["breakdown"].(map[string]any)
	if b["fallback_total_cost"].(float64) <= 0 || b["total_cost"] != 0.0 || got.EstimatedCost != 0 {
		t.Fatalf("zero-cost detailed bill=%+v", got)
	}
}

func TestBillingCatalogToolSurchargeAndTieredWrites(t *testing.T) {
	catalog, err := pricingcatalog.ParseCatalog([]byte(`{"vendor":{"models":{"m":{"id":"m","cost":{"input":5,"output":12,"cache_write":8}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	usage := ParseUsageFromBody([]byte(`{"usage":{"input_tokens":100,"output_tokens":10,"cache_creation_input_tokens":50,"cache_creation":{"ephemeral_5m_input_tokens":10,"ephemeral_1h_input_tokens":20},"server_tool_use":{"web_search_requests":2}}}`))
	// models.dev reports token rates. Supply a synthetic tool rate through the
	// same pricing model the estimator uses, leaving missing rates unpriced.
	for _, priced := range []bool{false, true} {
		var got BillingCostResult
		wantTool := 0.0
		if priced {
			pm := routing.PricingModel{ModelName: "m", ModelRatio: 1, CompletionRatio: 1, ToolUnitPrices: map[string]float64{"web_search.calls": 0.01}}
			got = estimateBillingCostWithPricingModel(pm, "anthropic", usage, catalog, pricingcatalog.SiteClassOfficial)
			wantTool = 0.02
		} else {
			got = EstimateBillingCostFromUsageWithCatalog("m", "anthropic", usage, catalog, pricingcatalog.SiteClassOfficial)
		}
		b := got.BillingDetails["breakdown"].(map[string]any)
		lines := b["toolSurcharges"].([]routing.ProxyToolSurcharge)
		if got.BillingDetails["pricing"].(map[string]any)["source"] != "catalog" || len(lines) != 1 || lines[0].Quantity != 2 || lines[0].Cost != wantTool || (lines[0].UnitPrice != nil) != priced {
			t.Fatalf("priced=%t details=%+v", priced, got)
		}
		// 20 ordinary + 10 five-minute + 20 one-hour writes at 1.6x.
		for k, want := range map[string]float64{"input_cost": .0005, "output_cost": .00012, "cacheCreation5mCost": .00008, "cacheCreation1hCost": .000256, "cache_creation_cost": .000496, "total_cost": .001116 + wantTool} {
			if math.Abs(asFloat(t, b[k])-want) > 1e-12 {
				t.Errorf("priced=%t %s=%v want %v", priced, k, b[k], want)
			}
		}
		if got.EstimatedCost != b["total_cost"] {
			t.Fatalf("estimate and breakdown disagree: %+v", got)
		}
	}
}
