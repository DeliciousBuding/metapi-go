package proxyhandler

import (
	"context"
	"strings"
	"sync/atomic"

	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service/pricingcatalog"
)

// BillingCostResult is the cost estimate attached to a successful proxy log.
type BillingCostResult struct {
	EstimatedCost  float64
	BillingDetails map[string]any
}

// Direct grants use normalized OpenAI billing attribution, as proxy logs did
// before task accounting. Keep the ledger, logs and health-cost feed identical.
func selectedBillingPlatform(selected *routing.SelectedChannel) string {
	if selected == nil {
		return ""
	}
	if selected.Direct != nil {
		return "openai"
	}
	return selected.Site.Platform
}

var billingCatalog atomic.Pointer[func() *pricingcatalog.CatalogSnapshot]

// SetBillingCatalogProvider installs the current application catalog reader.
// Passing nil disables catalog pricing without starting or retaining a sync loop.
func SetBillingCatalogProvider(fn func() *pricingcatalog.CatalogSnapshot) {
	if fn == nil {
		billingCatalog.Store(nil)
		return
	}
	billingCatalog.Store(&fn)
}

func currentBillingCatalog() *pricingcatalog.CatalogSnapshot {
	if fn := billingCatalog.Load(); fn != nil {
		return (*fn)()
	}
	return nil
}

func selectedBillingClass(ctx context.Context, selected *routing.SelectedChannel) pricingcatalog.SiteClass {
	if selected == nil {
		return pricingcatalog.SiteClassRelay
	}
	site := pricingcatalog.SiteSnapshot{Platform: selected.Site.Platform, URL: selected.Site.URL}
	if direct := selected.Direct; direct != nil {
		site = pricingcatalog.SiteSnapshot{Platform: direct.Provider}
		if wire := directProviderWireFromContext(ctx); wire != nil && wire.DriverURL != "" {
			site.URL = wire.DriverURL
		} else if wire != nil && wire.Endpoint != nil {
			site.URL = wire.Endpoint.URL
		} else if !direct.Endpoints.IsConfigured() {
			site.URL = direct.BaseURL
		}
	}
	site.Platform = platform.NormalizePlatformAlias(site.Platform)
	return pricingcatalog.ClassifySite(site)
}

// EstimateBillingCostFromUsage builds estimated_cost + billing_details for proxy_logs.
func EstimateBillingCostFromUsage(modelName, platform string, usage ParsedUsage) BillingCostResult {
	return EstimateBillingCostFromUsageWithCatalog(modelName, platform, usage, currentBillingCatalog(), pricingcatalog.SiteClassRelay)
}

// EstimateBillingCostFromUsageWithCatalog prices the canonical billing model in
// one immutable snapshot. The endpoint class controls provenance and ratio priority.
func EstimateBillingCostFromUsageWithCatalog(billingModel, platform string, usage ParsedUsage, snapshot *pricingcatalog.CatalogSnapshot, class pricingcatalog.SiteClass) BillingCostResult {
	billingModel = strings.TrimSpace(billingModel)
	total := usage.TotalTokens
	if total <= 0 {
		total = usage.PromptTokens + usage.CompletionTokens
	}
	cost := routing.FallbackTokenCost(total, platform)
	details := map[string]any{
		"quota_type": 0,
		"usage": map[string]any{
			"prompt_tokens":         usage.PromptTokens,
			"completion_tokens":     usage.CompletionTokens,
			"total_tokens":          total,
			"cache_read_tokens":     usage.CacheReadTokens,
			"cache_creation_tokens": usage.CacheCreationTokens,
		},
		"pricing": map[string]any{
			"source":   "fallback_token_cost",
			"platform": strings.TrimSpace(platform),
			"model":    billingModel,
		},
		"breakdown": map[string]any{
			"total_cost": cost,
		},
		"usage_source": usage.Source,
	}
	if !usage.Found && len(usage.ToolUsage) == 0 {
		return BillingCostResult{cost, details}
	}
	u := routing.UsageForCost{
		PromptTokens: usage.PromptTokens, CompletionTokens: usage.CompletionTokens, TotalTokens: total,
		CacheReadTokens: usage.CacheReadTokens, CacheCreationTokens: usage.CacheCreationTokens,
		CacheCreation5mTokens:  usage.CacheCreation5mTokens,
		CacheCreation1hTokens:  usage.CacheCreation1hTokens,
		CacheCreation12hTokens: usage.CacheCreation12hTokens,
		ToolUsage:              usage.ToolUsage,
	}
	pm := routing.PricingModel{ModelName: billingModel, ModelRatio: 1, CompletionRatio: 1}
	entry, found := snapshot.Lookup(billingModel)
	list := found && entry.ListPriceKnown
	ratios := found && entry.HasRatioData() && (!list || class != pricingcatalog.SiteClassOfficial)
	if ratios {
		pm.ModelRatio = *entry.ModelRatio
		if entry.CompletionRatio != nil {
			pm.CompletionRatio = *entry.CompletionRatio
		}
		pm.CacheRatio = entry.CacheRatio
	}
	pricing := details["pricing"].(map[string]any)
	hasCache := usage.CacheReadTokens > 0 || usage.CacheCreationTokens > 0
	var detail *routing.ProxyBillingDetails
	if hasCache {
		detail = routing.CalculateModelUsageBreakdown(pm, u, nil)
		pricing["source"] = "model_ratio_defaults"
	} else {
		detail = routing.CalculateModelUsageFullPrice(pm, u, nil)
		pricing["source"] = "full_price_fallback"
	}
	if list || ratios {
		pricing["source"] = pricingcatalog.SourceRelayEstimate
		if class == pricingcatalog.SiteClassOfficial && list {
			pricing["source"] = pricingcatalog.SourceOfficial
		}
		pricing["catalog_model"] = entry.ModelID
	}
	b := detail.Breakdown
	if list && !ratios {
		// The ratio calculator normalizes zero model/completion ratios to one.
		// Absolute catalog rates (including zero) price the normalized token split directly.
		input, output := entry.InputPerMillion, entry.OutputPerMillion
		read, write := input*detail.Pricing.CacheRatio, input*detail.Pricing.CacheCreationRatio
		if entry.CacheReadPerMillion != nil {
			read = *entry.CacheReadPerMillion
		}
		if entry.CacheWritePerMillion != nil {
			write = *entry.CacheWritePerMillion
		}
		pricing["input_per_million"], pricing["output_per_million"] = input, output
		if hasCache {
			pricing["cache_read_per_million"], pricing["cache_creation_per_million"] = read, write
		}
		b.InputCost = float64(detail.Usage.BillablePromptTokens) * input / 1e6
		b.OutputCost = float64(detail.Usage.CompletionTokens) * output / 1e6
		b.CacheReadCost = float64(detail.Usage.CacheReadTokens) * read / 1e6
		b.PriceCatalogCacheWrites(detail.Usage, write)
		var toolCost float64
		for _, s := range b.ToolSurcharges {
			toolCost += s.Cost
		}
		b.TotalCost = b.InputCost + b.OutputCost + b.CacheReadCost + b.CacheCreationCost + toolCost
	} else {
		pricing["model_ratio"], pricing["completion_ratio"] = detail.Pricing.ModelRatio, detail.Pricing.CompletionRatio
		if hasCache {
			pricing["cache_ratio"], pricing["cache_creation_ratio"] = detail.Pricing.CacheRatio, detail.Pricing.CacheCreationRatio
		}
	}
	billingUsage := details["usage"].(map[string]any)
	billingUsage["billable_prompt_tokens"] = detail.Usage.BillablePromptTokens
	billingUsage["prompt_tokens_include_cache"] = detail.Usage.PromptTokensIncludeCache
	breakdown := map[string]any{
		"input_cost": b.InputCost, "output_cost": b.OutputCost,
		"cache_read_cost": b.CacheReadCost, "cache_creation_cost": b.CacheCreationCost,
		"total_cost": b.TotalCost, "fallback_total_cost": cost,
	}
	for _, tier := range []struct {
		label  string
		tokens int64
		cost   float64
	}{
		{"5m", detail.Usage.CacheCreation5mTokens, b.CacheCreation5mCost},
		{"1h", detail.Usage.CacheCreation1hTokens, b.CacheCreation1hCost},
		{"12h", detail.Usage.CacheCreation12hTokens, b.CacheCreation12hCost},
	} {
		if tier.tokens > 0 {
			billingUsage["cacheCreation"+tier.label+"Tokens"] = tier.tokens
			breakdown["cacheCreation"+tier.label+"Cost"] = tier.cost
		}
	}
	if len(b.ToolSurcharges) > 0 {
		breakdown["toolSurcharges"] = b.ToolSurcharges
	}
	details["breakdown"] = breakdown
	return BillingCostResult{EstimatedCost: b.TotalCost, BillingDetails: details}
}
