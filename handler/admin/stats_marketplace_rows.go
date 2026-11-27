package admin

import (
	"math"
	"sort"
	"strings"

	"github.com/deliciousbuding/metapi-go/service/pricingcatalog"
)

// buildMarketplaceIndex projects the aggregation into the lightweight rows
// the query layer (search / filters / sort / facets) runs on. Everything the
// query contract can reference is resolved here — supply kinds, catalog tags,
// endpoint types, and the lowest catalog input rate — so filtering never
// touches per-account detail.
func buildMarketplaceIndex(aggregation *marketplaceAggregation, catalogSnapshot *pricingcatalog.CatalogSnapshot) []marketplaceIndexRow {
	rows := make([]marketplaceIndexRow, 0, len(aggregation.names))
	for _, name := range aggregation.names {
		modelAgg := aggregation.byModel[name]
		directSources := aggregation.directSources[name]
		row := marketplaceIndexRow{
			name:         name,
			supplyCount:  len(directSources) + len(modelAgg.accounts),
			accountCount: len(modelAgg.accounts),
			sourceKinds:  map[string]struct{}{},
		}
		haystack := []string{name}
		seenHaystack := map[string]struct{}{strings.ToLower(name): {}}
		addHaystack := func(values ...string) {
			for _, value := range values {
				value = strings.TrimSpace(value)
				key := strings.ToLower(value)
				if key == "" {
					continue
				}
				if _, dup := seenHaystack[key]; dup {
					continue
				}
				seenHaystack[key] = struct{}{}
				haystack = append(haystack, value)
			}
		}
		for _, acc := range modelAgg.accounts {
			row.tokenCount += len(acc.Tokens)
			if acc.Kind != "" {
				row.sourceKinds[acc.Kind] = struct{}{}
			}
			username := ""
			if acc.Username != nil {
				username = *acc.Username
			}
			addHaystack(acc.Site, username)
		}
		if len(directSources) > 0 {
			row.sourceKinds["direct"] = struct{}{}
		}
		for _, source := range directSources {
			addHaystack(source.ConnectionName, source.Name, source.UpstreamModel)
		}
		if modelAgg.latCount > 0 {
			v := math.Round(modelAgg.latSum / float64(modelAgg.latCount))
			row.avgLatency = &v
		}
		if stats, ok := aggregation.successByModel[name]; ok && stats.total > 0 {
			v := math.Round(1000.0*float64(stats.success)/float64(stats.total)) / 10.0
			row.successRate = &v
		}
		entry, catalogHit := catalogLookup(catalogSnapshot, name)
		row.deprecated = catalogHit && entry.Status == "deprecated"
		if description, ok := catalogDescription(entry, catalogHit).(string); ok {
			row.description = description
			addHaystack(description)
		}
		row.tags = catalogTags(entry, catalogHit)
		addHaystack(row.tags...)
		row.endpointTypes = inferEndpointTypesForModel(catalogSnapshot, name, nil)
		row.priceInput = lowestCatalogInputRate(catalogPricingSources(entry, catalogHit))
		row.searchHaystack = strings.ToLower(strings.Join(haystack, "\x00"))
		rows = append(rows, row)
	}
	return rows
}

// hydrateMarketplaceRow expands one aggregated model into the full response
// shape. This is the only step whose cost scales with the number of rendered
// rows: the paginated query path calls it for the selected page only.
func hydrateMarketplaceRow(aggregation *marketplaceAggregation, name string, catalogSnapshot *pricingcatalog.CatalogSnapshot) map[string]any {
	agg := aggregation.byModel[name]
	sources := append([]marketplaceModelSource{}, aggregation.directSources[name]...)
	accounts := make([]map[string]any, 0, len(agg.accounts))
	accountIDs := make([]int64, 0, len(agg.accounts))
	for id := range agg.accounts {
		accountIDs = append(accountIDs, id)
	}
	sort.Slice(accountIDs, func(i, j int) bool { return accountIDs[i] < accountIDs[j] })
	tokenCount := 0
	for _, id := range accountIDs {
		acc := agg.accounts[id]
		username := ""
		if acc.Username != nil {
			username = *acc.Username
		}
		sources = append(sources, marketplaceModelSource{
			Kind: acc.Kind, ID: acc.ID, Name: username, SiteID: acc.SiteID,
			ConnectionName: acc.Site, UpstreamModel: name, Evidence: "availability", Enabled: true,
			Bindings: []marketplaceRouteBinding{},
		})
		tokenCount += len(acc.Tokens)
		var latency any
		if acc.Latency != nil {
			latency = int64(math.Round(*acc.Latency))
		} else {
			latency = nil
		}
		accounts = append(accounts, map[string]any{
			"id":       acc.ID,
			"site":     acc.Site,
			"username": acc.Username,
			"latency":  latency,
			"balance":  roundMicro(acc.Balance),
			"tokens":   acc.Tokens,
		})
	}
	var avgLatency any
	if agg.latCount > 0 {
		avgLatency = int64(math.Round(agg.latSum / float64(agg.latCount)))
	} else {
		avgLatency = nil
	}
	var successRate any
	if stats, ok := aggregation.successByModel[name]; ok && stats.total > 0 {
		successRate = math.Round(1000.0*float64(stats.success)/float64(stats.total)) / 10.0
	} else {
		successRate = nil
	}
	entry, catalogHit := catalogLookup(catalogSnapshot, name)
	return map[string]any{
		"name":          name,
		"supplyCount":   len(sources),
		"supplySources": sources,
		"accountCount":  len(accounts),
		"tokenCount":    tokenCount,
		"avgLatency":    avgLatency,
		"successRate":   successRate,
		"description":   catalogDescription(entry, catalogHit),
		"tags":          catalogTags(entry, catalogHit),
		"deprecated":    catalogHit && entry.Status == "deprecated",
		"releaseDate":   catalogReleaseDate(entry, catalogHit),
		"contextWindow": entry.ContextLimit,
		"catalogStatus": catalogStatusOf(catalogHit),
		// supportedEndpointTypes: catalog-driven (provider dialect + modality
		// surfaces) when the model is cataloged; name-prefix heuristic only as a
		// catalog-miss fallback.
		"supportedEndpointTypes": inferEndpointTypesForModel(catalogSnapshot, name, accounts),
		// Official catalog list price as a synthetic pricing source; the
		// frontend labels it "catalog estimate" because marketplace sites
		// are third-party relays (siteName "catalog" is the sentinel).
		"pricingSources": catalogPricingSources(entry, catalogHit),
		"accounts":       accounts,
	}
}
