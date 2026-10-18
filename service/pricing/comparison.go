package pricing

import (
	"math"
	"sort"
	"strings"

	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/service/pricingcatalog"
)

// ComparisonRates is USD per million tokens, except PerRequest (USD/call).
type ComparisonRates struct {
	Input      *float64 `json:"input"`
	Output     *float64 `json:"output"`
	CacheRead  *float64 `json:"cacheRead"`
	CacheWrite *float64 `json:"cacheWrite"`
	PerRequest *float64 `json:"perRequest"`
}

type GroupPrice struct {
	Group       string          `json:"group"`
	Description string          `json:"description"`
	Multiplier  *float64        `json:"multiplier"`
	Rates       ComparisonRates `json:"rates"`
	Relative    ComparisonRates `json:"relative"`
	SampleCost  *float64        `json:"sampleCost"`
}

type OfficialPrice struct {
	Model     string          `json:"model"`
	Provider  string          `json:"provider"`
	Aliased   bool            `json:"aliased"`
	Source    string          `json:"source"`
	FetchedAt string          `json:"fetchedAt"`
	UpdatedAt string          `json:"updatedAt"`
	Known     bool            `json:"known"`
	Rates     ComparisonRates `json:"rates"`
}

func OfficialPriceFor(snapshot *pricingcatalog.CatalogSnapshot, model string) *OfficialPrice {
	entry, ok := snapshot.Lookup(model)
	if !ok {
		return nil
	}
	out := &OfficialPrice{Model: entry.ModelID, Provider: entry.Provider, Aliased: entry.Aliased, Source: entry.PriceSource, UpdatedAt: entry.LastUpdated, Known: entry.ListPriceKnown}
	if out.Source == "" {
		out.Source = snapshot.Source
	}
	if !entry.PriceFetchedAt.IsZero() {
		out.FetchedAt = entry.PriceFetchedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	if entry.ListPriceKnown {
		out.Rates = ComparisonRates{Input: amount(&entry.InputPerMillion), Output: amount(&entry.OutputPerMillion), CacheRead: amount(entry.CacheReadPerMillion), CacheWrite: amount(entry.CacheWritePerMillion)}
	}
	return out
}

// GroupPrices applies the response's effective multiplier once. Missing factors
// stay unknown; expression/tiered billing retains its rule without a flat quote.
func GroupPrices(model platform.ModelPrice, snapshot platform.AccountPricing, official *OfficialPrice) []GroupPrice {
	names := make([]string, 0, len(snapshot.UsableGroups))
	for name := range snapshot.UsableGroups {
		for _, enabled := range model.Groups {
			if enabled == "all" || enabled == name {
				names = append(names, name)
				break
			}
		}
	}
	sort.Strings(names)
	out := make([]GroupPrice, 0, len(names))
	for _, name := range names {
		g := GroupPrice{Group: name, Description: snapshot.UsableGroups[name], Multiplier: amount(snapshot.GroupRatios[name])}
		linear := model.BillingExpression == "" && (model.BillingMode == "" || model.BillingMode == "flat" || model.BillingMode == "ratio")
		if linear && model.QuotaType == 0 {
			g.Rates.Input = product(model.ModelRatio, g.Multiplier, 2)
			g.Rates.Output = product(g.Rates.Input, model.CompletionRatio, 1)
			g.Rates.CacheRead = product(g.Rates.Input, model.CacheRatio, 1)
			g.Rates.CacheWrite = product(g.Rates.Input, model.CacheWriteRatio, 1)
			if g.Rates.Input != nil && g.Rates.Output != nil {
				cost := (*g.Rates.Input + *g.Rates.Output) / 1000
				g.SampleCost = amount(&cost)
			}
		} else if linear && model.QuotaType == 1 {
			g.Rates.PerRequest = product(model.PricePerRequest, g.Multiplier, 1)
			g.SampleCost = g.Rates.PerRequest
		}
		if official != nil && official.Known {
			g.Relative = ComparisonRates{Input: relative(g.Rates.Input, official.Rates.Input), Output: relative(g.Rates.Output, official.Rates.Output), CacheRead: relative(g.Rates.CacheRead, official.Rates.CacheRead), CacheWrite: relative(g.Rates.CacheWrite, official.Rates.CacheWrite)}
		}
		out = append(out, g)
	}
	return out
}

func amount(v *float64) *float64 {
	if v == nil || math.IsNaN(*v) || math.IsInf(*v, 0) || *v < 0 {
		return nil
	}
	n := *v
	return &n
}
func product(a, b *float64, scale float64) *float64 {
	a = amount(a)
	b = amount(b)
	if a == nil || b == nil {
		return nil
	}
	v := *a * *b * scale
	return amount(&v)
}
func relative(a, b *float64) *float64 {
	if a == nil || b == nil || *b <= 0 {
		return nil
	}
	v := *a / *b
	return amount(&v)
}

func MatchesModel(name, query string, exact bool) bool {
	name = strings.ToLower(name)
	query = strings.ToLower(strings.TrimSpace(query))
	return query == "" || (exact && name == query) || (!exact && strings.Contains(name, query))
}
