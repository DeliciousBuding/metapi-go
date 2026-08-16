package routing

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

// stubCatalogResolver implements CatalogPricingResolver for tests. A nil cost
// declines the query, mirroring the real provider's behavior.
type stubCatalogResolver struct {
	cost   *float64
	source string
	// perSite overrides cost/source per siteID when non-nil.
	perSite map[int64]stubCatalogAnswer
}

type stubCatalogAnswer struct {
	cost   *float64
	source string
}

func (s stubCatalogResolver) ResolveCatalogPricing(siteID, accountID int64, modelName string) (*float64, string) {
	if s.perSite != nil {
		if answer, ok := s.perSite[siteID]; ok {
			return answer.cost, answer.source
		}
		return nil, ""
	}
	return s.cost, s.source
}

// makeColdStartCandidate builds a candidate with no usage history and no
// configured unit cost — the cold-start shape catalog pricing exists for.
func makeColdStartCandidate(channelID, siteID int64) RouteChannelCandidate {
	return RouteChannelCandidate{
		Channel: store.RouteChannel{
			ID:           channelID,
			Weight:       1,
			SuccessCount: 0,
			FailCount:    0,
			TotalCost:    0,
		},
		Account: store.Account{ID: channelID, UnitCost: nil},
		Site:    store.Site{ID: siteID, Name: "cold-site"},
	}
}

func TestEffectiveUnitCost_CatalogRelayEstimate(t *testing.T) {
	candidate := makeColdStartCandidate(1, 100)
	resolver := stubCatalogResolver{cost: ptrFloat(0.0125), source: CatalogSourceRelayEstimate}

	signal := EffectiveUnitCost(candidate, "gpt-4o", resolver, 1.0)

	if signal.Source != CatalogSourceRelayEstimate {
		t.Errorf("source = %q, want %q (relay estimate must never be labeled as a real catalog price)", signal.Source, CatalogSourceRelayEstimate)
	}
	if signal.UnitCost != 0.0125 {
		t.Errorf("unit cost = %f, want 0.0125", signal.UnitCost)
	}
}

func TestEffectiveUnitCost_CatalogOfficialLabel(t *testing.T) {
	candidate := makeColdStartCandidate(1, 100)
	resolver := stubCatalogResolver{cost: ptrFloat(0.0125), source: CatalogSourceOfficial}

	signal := EffectiveUnitCost(candidate, "gpt-4o", resolver, 1.0)

	if signal.Source != CatalogSourceOfficial {
		t.Errorf("source = %q, want %q", signal.Source, CatalogSourceOfficial)
	}
}

func TestEffectiveUnitCost_CatalogDeclinedFallsBack(t *testing.T) {
	candidate := makeColdStartCandidate(1, 100)
	resolver := stubCatalogResolver{cost: nil}

	signal := EffectiveUnitCost(candidate, "unknown-model", resolver, 2.0)

	if signal.Source != "fallback" {
		t.Errorf("source = %q, want fallback when the catalog declines", signal.Source)
	}
	if signal.UnitCost != 2.0 {
		t.Errorf("unit cost = %f, want fallback 2.0", signal.UnitCost)
	}
}

// TestSelectLowestCostCandidate_ColdStartUsesCatalog covers the intended cold
// start: no history logs, no configured unit_cost — the catalog is the only
// cost signal, and lowest_cost must still return the cheapest non-zero price.
func TestSelectLowestCostCandidate_ColdStartUsesCatalog(t *testing.T) {
	cheap := 0.0025   // gpt-4o-mini-like reference cost
	mid := 0.0125     // gpt-4o-like reference cost
	expensive := 0.05 // opus-like reference cost

	c1 := makeColdStartCandidate(1, 101)
	c2 := makeColdStartCandidate(2, 102)
	c3 := makeColdStartCandidate(3, 103)

	resolver := stubCatalogResolver{perSite: map[int64]stubCatalogAnswer{
		101: {cost: &expensive, source: CatalogSourceRelayEstimate},
		102: {cost: &cheap, source: CatalogSourceOfficial},
		103: {cost: &mid, source: CatalogSourceRelayEstimate},
	}}

	selected := SelectLowestCostCandidate(
		[]RouteChannelCandidate{c1, c2, c3},
		func(RouteChannelCandidate) string { return "gpt-4o" },
		resolver,
		1.0,
	)

	if selected == nil {
		t.Fatal("cold-start selection returned nil; want the cheapest catalog-priced candidate")
	}
	if selected.Channel.ID != 2 {
		t.Fatalf("selected channel %d, want 2 (cheapest catalog price)", selected.Channel.ID)
	}

	signal := EffectiveUnitCost(*selected, "gpt-4o", resolver, 1.0)
	if signal.UnitCost <= 0 || signal.Source == "fallback" {
		t.Fatalf("cold-start selection must carry a non-zero non-fallback price, got source=%q cost=%f", signal.Source, signal.UnitCost)
	}
}

// TestSelectLowestCostCandidate_ColdStartMixedCatalogAvailability ensures a
// candidate the catalog cannot price (nil) still yields a non-empty selection
// using the fallback path instead of being skipped.
func TestSelectLowestCostCandidate_ColdStartMixedCatalogAvailability(t *testing.T) {
	c1 := makeColdStartCandidate(1, 101)
	c2 := makeColdStartCandidate(2, 102)

	resolver := stubCatalogResolver{perSite: map[int64]stubCatalogAnswer{
		101: {cost: ptrFloat(0.05), source: CatalogSourceOfficial},
		// site 102: no catalog entry → fallback
	}}

	selected := SelectLowestCostCandidate(
		[]RouteChannelCandidate{c1, c2},
		func(RouteChannelCandidate) string { return "gpt-4o" },
		resolver,
		0.5,
	)

	if selected == nil {
		t.Fatal("cold-start selection returned nil with partial catalog availability")
	}
	// Catalog-priced 0.05 beats fallback 0.5.
	if selected.Channel.ID != 1 {
		t.Fatalf("selected channel %d, want 1 (catalog price lower than fallback)", selected.Channel.ID)
	}
}
