package admin

import (
	"fmt"
	"strings"

	"github.com/deliciousbuding/metapi-go/routing"
)

type downstreamPriceCompareRow struct {
	SiteID              int64   `json:"siteId"`
	SiteName            string  `json:"siteName"`
	Platform            string  `json:"platform"`
	Model               string  `json:"model"`
	InputPerMillion     float64 `json:"inputPerMillion"`
	OutputPerMillion    float64 `json:"outputPerMillion"`
	Source              string  `json:"source"`
	RatesSource         string  `json:"ratesSource"`
	EstimatedCostSample float64 `json:"estimatedCostSample"`
	ObservedSamples     int     `json:"observedSamples,omitempty"`
	MissingPrice        bool    `json:"missingPrice"`
	Recommended         bool    `json:"recommended"`
}

func (scope *downstreamPricingScope) compareObservedPrices(
	accounts []downstreamPricingAccount, models []string,
	observed map[int64]map[string]observedPriceSignal, availability map[int64]map[string]string,
	limit int,
) []downstreamPriceCompareRow {
	rows := make([]routing.PriceCompareRow, 0)
	for _, account := range accounts {
		if !scope.accountVisible(account.SiteID, account.AccountID) {
			continue
		}
		accountObserved := observed[account.AccountID]
		accountAvailability := availability[account.AccountID]
		for _, modelName := range models {
			key := normalizeModelKey(modelName)
			signal, hasObserved := accountObserved[key]
			availableName, hasAvailability := accountAvailability[key]
			if !hasObserved && !hasAvailability {
				continue
			}
			resolvedModel := strings.TrimSpace(modelName)
			if hasAvailability && strings.TrimSpace(availableName) != "" {
				resolvedModel = strings.TrimSpace(availableName)
			} else if hasObserved && signal.ResolvedModel != "" {
				resolvedModel = signal.ResolvedModel
			}
			in := routing.PriceCompareInput{
				SiteID:    account.SiteID,
				SiteName:  account.SiteName,
				Platform:  account.Platform,
				Model:     resolvedModel,
				AccountID: account.AccountID,
				// Username and ConfiguredUnitCost are admin-only: the former is
				// an upstream account identity, the latter the operator's cost
				// basis, not a price offered to the downstream consumer.
			}
			if hasObserved {
				in.ObservedSamples = signal.Samples
				if signal.AvgCost > 0 {
					v := signal.AvgCost
					in.ObservedAvgCost = &v
				}
				if strings.TrimSpace(signal.BillingDetails) != "" {
					applyBillingDetailsToPriceInput(&in, signal.BillingDetails)
				}
			}
			rows = append(rows, routing.BuildPriceCompareRow(in, routing.DefaultPriceCompareSampleUsage))
		}
	}

	// Account identity is not part of the downstream contract, so collapse
	// multiple accounts on one site to the best price for that site/model.
	rows = aggregateDownstreamPriceRows(rows)
	sorted := routing.AggregatePriceCompareRows(rows)
	if len(sorted) > limit {
		sorted = sorted[:limit]
	}
	items := make([]downstreamPriceCompareRow, 0, len(sorted))
	for _, row := range sorted {
		items = append(items, downstreamPriceCompareRow{
			SiteID:              row.SiteID,
			SiteName:            row.SiteName,
			Platform:            row.Platform,
			Model:               row.Model,
			InputPerMillion:     row.InputPerMillion,
			OutputPerMillion:    row.OutputPerMillion,
			Source:              row.Source,
			RatesSource:         row.RatesSource,
			EstimatedCostSample: row.EstimatedCostSample,
			ObservedSamples:     row.ObservedSamples,
			MissingPrice:        row.MissingPrice,
			Recommended:         row.Recommended,
		})
	}
	return items
}

// aggregateDownstreamPriceRows collapses rows to one per site/model. Without
// account identity a consumer cannot tell two rows apart anyway, and the
// cheaper row is the one that matters for planning.
func aggregateDownstreamPriceRows(rows []routing.PriceCompareRow) []routing.PriceCompareRow {
	out := make([]routing.PriceCompareRow, 0, len(rows))
	index := map[string]int{}
	for _, row := range rows {
		key := siteModelKey(row.SiteID, normalizeModelKey(row.Model))
		idx, ok := index[key]
		if !ok {
			index[key] = len(out)
			out = append(out, row)
			continue
		}
		if downstreamPriceRowBetter(row, out[idx]) {
			out[idx] = row
		}
	}
	return out
}

func downstreamPriceRowBetter(candidate, current routing.PriceCompareRow) bool {
	if candidate.MissingPrice != current.MissingPrice {
		return !candidate.MissingPrice
	}
	return candidate.EstimatedCostSample < current.EstimatedCostSample
}

func normalizeModelKey(model string) string {
	return strings.ToLower(strings.TrimSpace(model))
}

func siteModelKey(siteID int64, modelKey string) string {
	return fmt.Sprintf("%d\x00%s", siteID, modelKey)
}
