package admin

import (
	"net/http"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/app"
	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service/pricing"
)

// Downstream pricing is a product surface for managed keys, not an operator
// console. Inputs are clamped harder than the admin view so one request cannot
// fan out into an unbounded aggregation: at most 30 days, 12 top models and
// 100 price rows per response.
const (
	downstreamPricingMaxDays        = 30
	downstreamPricingMaxTopModels   = 12
	downstreamPricingMaxLimit       = 100
	downstreamPricingMaxPricePage   = 20
	downstreamPricingDefaultLimit   = 50
	downstreamPricingDefaultPage    = 10
	downstreamPricingTopModelsFetch = 4 // over-fetch factor before policy filtering
)

// downstreamModelPriceCompare handles the downstream-key-visible price catalog:
//
//	GET /v1/pricing?model=...&days=...&limit=...
//	GET /v1/models/price-compare (alias)
//
// Unlike the admin view it applies the key's routing policy to sites, models
// and credential refs, and strips account identities and internal cost fields.
func (h *statsHandler) downstreamModelPriceCompare(w http.ResponseWriter, r *http.Request) {
	pac := auth.GetProxyAuth(r.Context())
	if pac == nil {
		writeError(w, http.StatusUnauthorized, "downstream authentication required")
		return
	}
	// This legacy catalog aggregates platform-side observations, not personal
	// tariffs. Do not expose it to ordinary users before scoped pricing exists.
	if pac.OwnerUserID != nil {
		writeError(w, http.StatusForbidden, "User pricing catalog is not available")
		return
	}

	modelQ := strings.TrimSpace(r.URL.Query().Get("model"))
	// Downstream requests are always exact (case-insensitive) model matches.
	// The admin `exactModel` switch and its substring LIKE remain admin-only:
	// a leading-wildcard LIKE over proxy_logs/model_availability is the query
	// amplification this surface must not expose to managed keys.
	limit := config.ClampInt(getQueryInt(r, "limit", downstreamPricingDefaultLimit), 1, downstreamPricingMaxLimit)
	days := config.ClampInt(getQueryInt(r, "days", downstreamPricingMaxDays), 1, downstreamPricingMaxDays)
	topModels := config.ClampInt(getQueryInt(r, "topModels", downstreamPricingMaxTopModels), 1, downstreamPricingMaxTopModels)

	scope := newDownstreamPricingScope(pac.Policy)
	if !scope.accountPricingVisible {
		writeDownstreamPricingResponse(w, modelQ, days, limit, 0, nil, nil, nil, nil, currentPricePage{Page: 1, PageSize: downstreamPricingDefaultPage})
		return
	}
	if modelQ != "" && !scope.modelVisible(modelQ) {
		writeDownstreamPricingResponse(w, modelQ, days, limit, 0, nil, nil, nil, nil, currentPricePage{Page: 1, PageSize: downstreamPricingDefaultPage})
		return
	}

	since := time.Now().UTC().AddDate(0, 0, -(days - 1)).Format(time.RFC3339)

	var models []string
	if modelQ != "" {
		models = []string{modelQ}
	} else {
		var err error
		models, err = h.downstreamTopModels(scope, topModels, since)
		if err != nil {
			writeErrorCode(w, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load top models")
			return
		}
	}

	accounts, current, currentPage, err := h.downstreamCurrentPriceComparison(r, scope, modelQ)
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load current pricing")
		return
	}

	// One aggregation per dimension for the whole request. The admin view runs
	// these queries once per model; the downstream bound (<=12 models) does not
	// make that N+1 acceptable on a key-triggerable endpoint.
	observed, err := h.downstreamObservedSignals(scope, models, since)
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load observed prices")
		return
	}
	availability, err := h.downstreamAvailability(scope, models)
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load model availability")
		return
	}
	items := scope.compareObservedPrices(accounts, models, observed, availability, limit)

	baselines := downstreamOfficialPrices(modelQ, items, current, currentPage)
	// Only advertise accounts for sites that actually contributed a row to this
	// response. A key restricted to a model gets no site topology beyond the
	// sites that price that model.
	accounts = filterDownstreamPricingAccounts(accounts, items, current)
	writeDownstreamPricingResponse(w, modelQ, days, limit, len(models), items, accounts, current, baselines, currentPage)
}

func writeDownstreamPricingResponse(w http.ResponseWriter, modelQ string, days, limit, modelsConsidered int, items []downstreamPriceCompareRow, accounts []downstreamPricingAccount, current []downstreamCurrentPrice, official map[string]*pricing.OfficialPrice, page currentPricePage) {
	if items == nil {
		items = []downstreamPriceCompareRow{}
	}
	if accounts == nil {
		accounts = []downstreamPricingAccount{}
	}
	if current == nil {
		current = []downstreamCurrentPrice{}
	}
	if official == nil {
		official = map[string]*pricing.OfficialPrice{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"model": modelQ,
		"days":  days,
		"limit": limit,
		"sampleUsage": map[string]any{
			"promptTokens":     routing.DefaultPriceCompareSampleUsage.PromptTokens,
			"completionTokens": routing.DefaultPriceCompareSampleUsage.CompletionTokens,
			"totalTokens":      routing.DefaultPriceCompareSampleUsage.TotalTokens,
		},
		"items":           items,
		"pricingAccounts": accounts,
		"currentPrices":   current,
		"officialPrices":  official,
		"currentPage":     page,
		"meta": map[string]any{
			"count":            len(items),
			"modelsConsidered": modelsConsidered,
			"sources": []string{
				routing.PriceSourceBilling,
				routing.PriceSourceObserved,
				routing.PriceSourceFallback,
			},
			"notes": "Rates/costs from billing_details or observed proxy_logs, or labeled platform fallback. Rows are scoped to this key's sites, models and credentials; account identities and configured unit cost are not exposed.",
		},
	})
}

// downstreamOfficialPrices keys catalog baselines to the models actually
// returned, so an official price never reveals a model the key cannot see.
func downstreamOfficialPrices(modelQ string, items []downstreamPriceCompareRow, current []downstreamCurrentPrice, page currentPricePage) map[string]*pricing.OfficialPrice {
	baselines := map[string]*pricing.OfficialPrice{}
	manager := app.CatalogManager()
	if manager == nil {
		return baselines
	}
	for _, row := range items {
		baselines[row.Model] = pricing.OfficialPriceFor(manager.Snapshot(), row.Model)
	}
	for _, row := range current {
		baselines[row.Model] = row.Official
	}
	if modelQ != "" && page.TotalModels == 0 {
		baselines[modelQ] = pricing.OfficialPriceFor(manager.Snapshot(), modelQ)
	}
	return baselines
}

// filterDownstreamPricingAccounts keeps site-level account entries that
// contributed to this response. Account rows carry no identity, so an entry
// with no matching price row is only site topology.
func filterDownstreamPricingAccounts(accounts []downstreamPricingAccount, items []downstreamPriceCompareRow, current []downstreamCurrentPrice) []downstreamPricingAccount {
	sites := make(map[int64]struct{}, len(items)+len(current))
	for _, row := range items {
		sites[row.SiteID] = struct{}{}
	}
	for _, row := range current {
		sites[row.SiteID] = struct{}{}
	}
	out := make([]downstreamPricingAccount, 0, len(accounts))
	for _, account := range accounts {
		if _, ok := sites[account.SiteID]; !ok {
			continue
		}
		out = append(out, account)
	}
	return out
}
