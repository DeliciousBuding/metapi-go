package admin

import (
	"net/http"
	"sort"
	"strings"
)

// Marketplace server-side query contract (search / filters / sort / facets).
//
// The query layer runs after the fleet-wide aggregation and before hydration:
// it operates on one lightweight index row per model, so filtering, sorting,
// totals, and facet counts cover the whole marketplace instead of the page the
// browser happened to download. Only the surviving page (or the single detail
// row) is hydrated into the full response shape afterwards.
//
// Semantics, kept identical to the column filterFns the client ran when it
// could only filter the current page:
//   - q: case-insensitive substring over the model name, catalog description,
//     catalog tags, and supply connection/site/account names.
//   - capability / endpointType / sourceKind: OR within one dimension, AND
//     across dimensions (a row must match q AND every active dimension).
//   - sort: one whitelisted column + direction; unknown or unsupported values
//     fall back to name.asc, ties break by ascending name.
//
// Facets are disjunctive per dimension: the count list for one dimension is
// computed with that dimension's own filter removed (q and the other
// dimensions still apply), so selecting a value never hides the alternatives
// the user could switch to. facetScope states this explicitly.

const (
	marketplaceSortDefault  = "name.asc"
	marketplaceFacetScope   = "filteredExcludingSelf"
	marketplaceQueryMaxDims = 32
)

// marketplaceQuery is the normalized query context echoed back in meta.query.
type marketplaceQuery struct {
	Q             string
	Capabilities  []string
	EndpointTypes []string
	SourceKinds   []string
	Sort          string
	sortColumn    string
	sortDesc      bool
}

// marketplaceIndexRow carries every field the query layer needs. Accounts,
// supply sources, and pricing stay in the aggregation until hydration.
type marketplaceIndexRow struct {
	name           string
	supplyCount    int
	accountCount   int
	tokenCount     int
	avgLatency     *float64 // rounded ms
	successRate    *float64 // one decimal percent
	priceInput     *float64 // lowest per-million input across pricing sources
	deprecated     bool
	description    string
	tags           []string
	endpointTypes  []string
	sourceKinds    map[string]struct{}
	searchHaystack string
}

var marketplaceSortableColumns = map[string]bool{
	"name":         true,
	"supplyCount":  true,
	"accountCount": true,
	"tokenCount":   true,
	"avgLatency":   true,
	"successRate":  true,
	"priceInput":   true,
}

var marketplaceSourceKindSet = map[string]bool{
	"account": true,
	"oauth":   true,
	"direct":  true,
}

// parseMarketplaceQuery normalizes the raw URL query into the whitelist-bound
// contract. Values are matched case-insensitively; the echo in meta.query
// reports the normalized (lowercased) forms the filter actually used.
func parseMarketplaceQuery(r *http.Request) marketplaceQuery {
	values := r.URL.Query()
	q := marketplaceQuery{
		Q:             strings.TrimSpace(values.Get("q")),
		Capabilities:  parseMarketplaceListParam(values.Get("capability")),
		EndpointTypes: parseMarketplaceListParam(values.Get("endpointType")),
		SourceKinds:   parseMarketplaceListParam(values.Get("sourceKind")),
	}
	q.Sort, q.sortColumn, q.sortDesc = parseMarketplaceSort(values.Get("sort"))
	return q
}

func parseMarketplaceListParam(raw string) []string {
	out := []string{}
	seen := map[string]struct{}{}
	for _, part := range strings.Split(raw, ",") {
		value := strings.ToLower(strings.TrimSpace(part))
		if value == "" {
			continue
		}
		if _, dup := seen[value]; dup {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
		if len(out) >= marketplaceQueryMaxDims {
			break
		}
	}
	return out
}

// parseMarketplaceSort accepts the table's `column:direction` wire form (and
// the dotted alias) and clamps it to the whitelist. Anything else degrades to
// the default instead of failing the whole request.
func parseMarketplaceSort(raw string) (echo string, column string, desc bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return marketplaceSortDefault, "name", false
	}
	// Only the first term is honoured; multi-column sorts are not part of the
	// contract yet and silently picking a later term would surprise.
	first, _, _ := strings.Cut(raw, ",")
	name, dir, found := strings.Cut(strings.TrimSpace(first), ":")
	if !found {
		name, dir, found = strings.Cut(strings.TrimSpace(first), ".")
	}
	name = strings.TrimSpace(name)
	desc = strings.EqualFold(strings.TrimSpace(dir), "desc")
	if !found || !marketplaceSortableColumns[name] {
		return marketplaceSortDefault, "name", false
	}
	if desc {
		return name + ".desc", name, true
	}
	return name + ".asc", name, false
}

// filterMarketplaceIndex applies q AND every active dimension. Within one
// dimension any selected value matches (OR), mirroring the retired
// client-side filterFns.
func filterMarketplaceIndex(rows []marketplaceIndexRow, q marketplaceQuery) []marketplaceIndexRow {
	matches := func(row marketplaceIndexRow) bool {
		if q.Q != "" && !strings.Contains(row.searchHaystack, strings.ToLower(q.Q)) {
			return false
		}
		if len(q.Capabilities) > 0 && !rowMatchesAnyValue(row.tags, q.Capabilities) {
			return false
		}
		if len(q.EndpointTypes) > 0 && !rowMatchesAnyValue(row.endpointTypes, q.EndpointTypes) {
			return false
		}
		if len(q.SourceKinds) > 0 {
			matched := false
			for _, kind := range q.SourceKinds {
				if !marketplaceSourceKindSet[kind] {
					// Unknown kinds are not silently dropped: they match
					// nothing, so a typo reads as an empty result instead of
					// as an unfiltered list.
					continue
				}
				if _, ok := row.sourceKinds[kind]; ok {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		}
		return true
	}
	out := make([]marketplaceIndexRow, 0, len(rows))
	for _, row := range rows {
		if matches(row) {
			out = append(out, row)
		}
	}
	return out
}

func rowMatchesAnyValue(haystack []string, needles []string) bool {
	for _, value := range haystack {
		for _, needle := range needles {
			if strings.EqualFold(value, needle) {
				return true
			}
		}
	}
	return false
}

// sortMarketplaceIndex orders filtered rows by the whitelisted column. Missing
// measurements (latency / success rate / price) always sort last in both
// directions — "no data" must never read as "best" or "worst" — so the
// null placement is decided before the direction is applied.
func sortMarketplaceIndex(rows []marketplaceIndexRow, q marketplaceQuery) {
	isOptional := q.sortColumn == "avgLatency" || q.sortColumn == "successRate" || q.sortColumn == "priceInput"
	optional := func(row marketplaceIndexRow) *float64 {
		switch q.sortColumn {
		case "avgLatency":
			return row.avgLatency
		case "successRate":
			return row.successRate
		case "priceInput":
			return row.priceInput
		default:
			return nil
		}
	}
	less := func(i, j int) bool {
		a, b := rows[i], rows[j]
		cmp := 0
		if isOptional {
			av, bv := optional(a), optional(b)
			switch {
			case av == nil && bv == nil:
				// Both unmeasured: fall through to the name tie-break.
			case av == nil:
				return false // a sorts after b in both directions
			case bv == nil:
				return true
			default:
				cmp = compareFloat(*av, *bv)
			}
		} else {
			switch q.sortColumn {
			case "supplyCount":
				cmp = compareInt(a.supplyCount, b.supplyCount)
			case "accountCount":
				cmp = compareInt(a.accountCount, b.accountCount)
			case "tokenCount":
				cmp = compareInt(a.tokenCount, b.tokenCount)
			default: // "name"
				cmp = strings.Compare(a.name, b.name)
			}
		}
		if cmp == 0 {
			// Stable, deterministic order: name is the row identity.
			cmp = strings.Compare(a.name, b.name)
		} else if q.sortDesc {
			cmp = -cmp
		}
		return cmp < 0
	}
	sort.SliceStable(rows, less)
}

func compareFloat(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func compareInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// marketplaceFacetValue is one facet option plus the number of rows in the
// disjunctive context that carry it.
type marketplaceFacetValue struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

// marketplaceFacets carries one option list per filterable dimension.
type marketplaceFacets struct {
	EndpointTypes []marketplaceFacetValue `json:"endpointTypes"`
	Capabilities  []marketplaceFacetValue `json:"capabilities"`
	SourceKinds   []marketplaceFacetValue `json:"sourceKinds"`
}

// buildMarketplaceFacets computes each dimension over the rows matching q and
// the *other* dimensions (disjunctive faceting): selecting "openai" must not
// collapse the endpoint-type list down to just "openai", or the filter would
// hide every alternative the user could switch to. Slice fields are never nil
// so the JSON shape is stable for clients.
func buildMarketplaceFacets(rows []marketplaceIndexRow, q marketplaceQuery) marketplaceFacets {
	without := func(drop string) marketplaceQuery {
		next := q
		switch drop {
		case "endpointType":
			next.EndpointTypes = nil
		case "capability":
			next.Capabilities = nil
		case "sourceKind":
			next.SourceKinds = nil
		}
		next.Sort, next.sortColumn, next.sortDesc = marketplaceSortDefault, "name", false
		return next
	}
	return marketplaceFacets{
		EndpointTypes: countMarketplaceFacet(filterMarketplaceIndex(rows, without("endpointType")),
			func(row marketplaceIndexRow) []string { return row.endpointTypes }),
		Capabilities: countMarketplaceFacet(filterMarketplaceIndex(rows, without("capability")),
			func(row marketplaceIndexRow) []string { return row.tags }),
		SourceKinds: countMarketplaceFacet(filterMarketplaceIndex(rows, without("sourceKind")),
			func(row marketplaceIndexRow) []string {
				kinds := make([]string, 0, len(row.sourceKinds))
				for kind := range row.sourceKinds {
					kinds = append(kinds, kind)
				}
				return kinds
			}),
	}
}

// countMarketplaceFacet tallies each distinct value once per row, sorted by
// count desc then value asc so the most common options surface first.
func countMarketplaceFacet(rows []marketplaceIndexRow, values func(marketplaceIndexRow) []string) []marketplaceFacetValue {
	counts := map[string]int{}
	for _, row := range rows {
		seen := map[string]struct{}{}
		for _, value := range values(row) {
			key := strings.ToLower(strings.TrimSpace(value))
			if key == "" {
				continue
			}
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			counts[key]++
		}
	}
	out := make([]marketplaceFacetValue, 0, len(counts))
	for value, count := range counts {
		out = append(out, marketplaceFacetValue{Value: value, Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Value < out[j].Value
	})
	return out
}

// metaQueryEcho reports the normalized query context the server actually
// applied, so debugging a shared link never requires guessing how a raw
// parameter was parsed.
func (q marketplaceQuery) metaQueryEcho() map[string]any {
	return map[string]any{
		"q":             q.Q,
		"capabilities":  q.Capabilities,
		"endpointTypes": q.EndpointTypes,
		"sourceKinds":   q.SourceKinds,
		"sort":          q.Sort,
	}
}
