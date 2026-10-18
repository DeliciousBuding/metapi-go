package pricing

import (
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/service/pricingcatalog"
	"testing"
)

func TestGroupPricesEffectiveOverrideAndCache(t *testing.T) {
	ptr := func(v float64) *float64 { return &v }
	model := platform.ModelPrice{Model: "synthetic", ModelRatio: ptr(1.5), CompletionRatio: ptr(5), CacheRatio: ptr(.1), CacheWriteRatio: ptr(1.25), Groups: []string{"all"}}
	snapshot := platform.AccountPricing{UsableGroups: map[string]string{"standard": "Standard", "special": "Override", "unknown": "Missing ratio"}, GroupRatios: map[string]*float64{"standard": ptr(1), "special": ptr(.5)}}
	baseline := &OfficialPrice{Known: true, Rates: ComparisonRates{Input: ptr(3), Output: ptr(15), CacheRead: ptr(.3), CacheWrite: ptr(3.75)}}
	groups := GroupPrices(model, snapshot, baseline)
	if len(groups) != 3 {
		t.Fatal(groups)
	}
	for _, g := range groups {
		if g.Group == "special" {
			if *g.Rates.Input != 1.5 || *g.Rates.Output != 7.5 || *g.Rates.CacheWrite != 1.875 || *g.Relative.Input != .5 || *g.SampleCost != .009 {
				t.Fatalf("override applied incorrectly: %+v", g)
			}
		}
		if g.Group == "unknown" && (g.SampleCost != nil || g.Rates.Input != nil) {
			t.Fatalf("invented group ratio: %+v", g)
		}
	}
	model.Groups = []string{"standard"}
	if got := GroupPrices(model, snapshot, baseline); len(got) != 1 || got[0].Group != "standard" {
		t.Fatal(got)
	}
}

func TestGroupPricesFreeUnknownConditionalAndPerCall(t *testing.T) {
	p := func(v float64) *float64 { return &v }
	s := platform.AccountPricing{UsableGroups: map[string]string{"free": ""}, GroupRatios: map[string]*float64{"free": p(0)}}
	m := platform.ModelPrice{Groups: []string{"all"}, ModelRatio: p(1), CompletionRatio: p(2)}
	g := GroupPrices(m, s, &OfficialPrice{Known: true, Rates: ComparisonRates{Input: p(0), Output: p(3)}})[0]
	if g.SampleCost == nil || *g.SampleCost != 0 || g.Relative.Input != nil || g.Relative.Output == nil || *g.Relative.Output != 0 {
		t.Fatalf("free or zero baseline: %+v", g)
	}
	m.BillingMode = "tiered_expr"
	m.BillingExpression = "tokens > 1000 ? 2 : 1"
	if g := GroupPrices(m, s, nil)[0]; g.SampleCost != nil || g.Rates.Input != nil {
		t.Fatal("invented linear price")
	}
	m.BillingMode = ""
	m.BillingExpression = ""
	m.QuotaType = 1
	m.PricePerRequest = p(.1)
	s.GroupRatios["free"] = p(.5)
	g = GroupPrices(m, s, nil)[0]
	if g.Rates.Input != nil || g.Rates.PerRequest == nil || *g.SampleCost != .05 {
		t.Fatal(g)
	}
	m.QuotaType = 5
	if g := GroupPrices(m, s, nil)[0]; g.SampleCost != nil {
		t.Fatal("unknown billing type priced")
	}
}

func TestOfficialPriceKeepsFreeAndMissingDistinct(t *testing.T) {
	s, err := pricingcatalog.ParseLLMMetadata([]byte(`{"test":{"id":"test","models":{"free":{"id":"free","cost":{"input":0,"output":0}},"unknown":{"id":"unknown"}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	free := OfficialPriceFor(s, "free")
	unknown := OfficialPriceFor(s, "unknown")
	if free == nil || !free.Known || free.Rates.Input == nil || *free.Rates.Input != 0 {
		t.Fatal(free)
	}
	if unknown == nil || unknown.Known || unknown.Rates.Input != nil {
		t.Fatal(unknown)
	}
}

func TestOfficialPriceFreeBaselineWinsMerge(t *testing.T) {
	free, err := pricingcatalog.ParseCatalog([]byte(`{"test":{"id":"test","models":{"free":{"id":"free","cost":{"input":0,"output":0}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	paid, err := pricingcatalog.ParseCatalog([]byte(`{"test":{"id":"test","models":{"free":{"id":"free","cost":{"input":2,"output":8}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	price := OfficialPriceFor(pricingcatalog.MergeSnapshots([]*pricingcatalog.CatalogSnapshot{free, paid}), "free")
	if price == nil || !price.Known || *price.Rates.Input != 0 || *price.Rates.Output != 0 {
		t.Fatalf("free baseline replaced by later price: %+v", price)
	}
}
