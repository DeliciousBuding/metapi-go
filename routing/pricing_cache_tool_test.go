package routing

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/internal/toolusage"
)

func TestTieredCacheWrites(t *testing.T) {
	model := PricingModel{ModelRatio: 1, ModelName: "claude"}
	usage := UsageForCost{PromptTokens: 2_000_000, CacheCreationTokens: 1_000_000, CacheCreation5mTokens: 250_000, CacheCreation1hTokens: 250_000}
	d := CalculateModelUsageBreakdown(model, usage, nil)
	if d.Usage.BillablePromptTokens != 1_000_000 || d.Breakdown.CacheCreation5mCost != 0.625 || d.Breakdown.CacheCreation1hCost != 1 || d.Breakdown.CacheCreationCost != 2.875 || d.Breakdown.TotalCost != 4.875 {
		t.Fatalf("Claude: %+v", d)
	}
	usage = UsageForCost{PromptTokens: 1_000_000, CacheCreationTokens: 1_000_000, CacheCreation12hTokens: 400_000}
	model = PricingModel{ModelRatio: 1}
	d = CalculateModelUsageBreakdown(model, usage, nil)
	if d.Breakdown.CacheCreation12hCost != 1.6 || d.Breakdown.TotalCost != 2.8 {
		t.Fatalf("12h + ordinary: %+v", d)
	}
	zero := 0.0
	model.CacheCreation12hRatio = &zero
	usage.CacheCreation12hTokens = 2_000_000
	d = CalculateModelUsageBreakdown(model, usage, nil)
	if d.Usage.CacheCreation12hTokens != 1_000_000 || d.Breakdown.TotalCost != 0 {
		t.Fatalf("free 12h: %+v", d)
	}
	usage.CacheCreation12hTokens = 1<<50 + 1
	usage.CacheCreation1hTokens = -1
	d = CalculateModelUsageBreakdown(model, usage, nil)
	if d.Usage.CacheCreation12hTokens != 0 || d.Breakdown.TotalCost != 2 {
		t.Fatalf("invalid tiers: %+v", d)
	}
}

func TestToolSurchargesMissingAndFree(t *testing.T) {
	model := PricingModel{ToolUnitPrices: map[string]float64{
		"web_search.calls":           0.01,
		"grounding.grounded_prompts": 0,
		"file_search.calls":          math.Inf(1),
	}}
	usage := UsageForCost{ToolUsage: []ToolUsageChargeInput{
		{Tool: "web_search", Unit: "calls", Quantity: 2},
		{Tool: "grounding", Unit: "queries", Quantity: 3},
		{Tool: "grounding", Unit: "grounded_prompts", Quantity: 1},
		{Tool: "file_search", Unit: "calls", Quantity: 1},
		{Tool: "web_search", Unit: "calls", Quantity: 1<<50 + 1},
	}}
	groups := map[string]float64{"default": 2}
	for _, d := range []*ProxyBillingDetails{CalculateModelUsageBreakdown(model, usage, groups), CalculateModelUsageFullPrice(model, usage, groups)} {
		charges := d.Breakdown.ToolSurcharges
		if len(charges) != 4 || d.Breakdown.TotalCost != 0.04 || charges[0].Cost != 0.04 || charges[1].UnitPrice != nil || charges[2].UnitPrice == nil || *charges[2].UnitPrice != 0 || charges[3].UnitPrice != nil {
			t.Fatalf("charges: %+v", d.Breakdown)
		}
	}
	model.QuotaType = 1
	flat := 0.1
	model.ModelPrice = &ModelPrice{Flat: &flat}
	if got := CalculateModelUsageCost(model, usage, groups); got != 0.24 {
		t.Fatalf("per-call = %v", got)
	}
}

func TestGeminiGroundedPromptBill(t *testing.T) {
	frame := []byte(`{"usageMetadata":{"promptTokenCount":100,"cachedContentTokenCount":40},"candidates":[{"groundingMetadata":{"webSearchQueries":["private fixture","private fixture"]}}]}`)
	var tracker toolusage.Tracker
	tracker.Observe(frame)
	readRatio := 0.5
	model := PricingModel{CacheRatio: &readRatio, ToolUnitPrices: map[string]float64{"grounding.grounded_prompts": 0.035}}
	usage := UsageForCost{PromptTokens: 100, CacheReadTokens: toolusage.ExtractCacheUsage(frame).CacheReadTokens, ToolUsage: tracker.Snapshot()}
	d := CalculateModelUsageBreakdown(model, usage, nil)
	if d.Usage.BillablePromptTokens != 60 || d.Usage.TotalTokens != 100 || d.Breakdown.TotalCost != 0.035160 {
		t.Fatalf("Gemini: %+v", d)
	}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "private fixture") {
		t.Fatalf("query in billing details: %s", b)
	}
}

func TestLegacyCostJSONOmitsTiersAndTools(t *testing.T) {
	d := CalculateModelUsageBreakdown(PricingModel{}, UsageForCost{PromptTokens: 2, CacheCreationTokens: 1}, nil)
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"5m", "1h", "12h", "toolSurcharges"} {
		if strings.Contains(string(b), field) {
			t.Fatalf("legacy details include %s: %s", field, b)
		}
	}
	if d.Breakdown.TotalCost != 0.000004 {
		t.Fatalf("legacy total: %v", d.Breakdown.TotalCost)
	}
}
