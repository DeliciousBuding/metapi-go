package admin

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"

	"github.com/deliciousbuding/metapi-go/app"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/service/pricing"
)

type downstreamPricingAccount struct {
	AccountID   int64  `db:"account_id" json:"-"`
	SiteID      int64  `db:"site_id" json:"siteId"`
	SiteName    string `db:"site_name" json:"siteName"`
	Platform    string `db:"platform" json:"platform"`
	Payload     string `db:"payload" json:"-"`
	FetchedAt   string `db:"fetched_at" json:"fetchedAt"`
	AttemptedAt string `db:"attempted_at" json:"attemptedAt"`
	Supported   bool   `db:"-" json:"supported"`
}

type downstreamCurrentPrice struct {
	SiteID    int64                  `json:"siteId"`
	SiteName  string                 `json:"siteName"`
	Model     string                 `json:"model"`
	FetchedAt string                 `json:"fetchedAt"`
	Pricing   platform.ModelPrice    `json:"pricing"`
	Groups    []pricing.GroupPrice   `json:"groups"`
	Official  *pricing.OfficialPrice `json:"official"`
}

// downstreamCurrentPriceComparison is the policy-scoped counterpart of
// currentPriceComparison. It returns site-level accounts (no account identity),
// site/model-deduplicated current prices, and the page over policy-visible
// models.
func (h *statsHandler) downstreamCurrentPriceComparison(r *http.Request, scope *downstreamPricingScope, modelQ string) ([]downstreamPricingAccount, []downstreamCurrentPrice, currentPricePage, error) {
	page := currentPricePage{
		Page:     config.ClampInt(getQueryInt(r, "pricePage", 1), 1, 1_000_000),
		PageSize: config.ClampInt(getQueryInt(r, "pricePageSize", downstreamPricingDefaultPage), 1, downstreamPricingMaxPricePage),
	}

	filters, filterArgs := scope.accountFilters("a.site_id", "a.id")
	query := `
		SELECT a.id AS account_id, a.site_id AS site_id, s.name AS site_name, s.platform AS platform,
			COALESCE(p.payload, '') AS payload,
			COALESCE(p.fetched_at, '') AS fetched_at,
			COALESCE(p.attempted_at, '') AS attempted_at
		FROM accounts a
		JOIN sites s ON s.id = a.site_id
		LEFT JOIN account_pricing p ON p.account_id = a.id
		WHERE COALESCE(a.status, '') <> 'disabled'
			AND COALESCE(s.status, '') <> 'disabled'` + filters + `
		ORDER BY s.name, a.id`

	accounts := make([]downstreamPricingAccount, 0)
	if err := h.db.SelectContext(r.Context(), &accounts, query, filterArgs...); err != nil {
		return nil, nil, page, err
	}

	snapshots := make(map[int64]platform.AccountPricing, len(accounts))
	names := map[string]string{}
	visibleAccounts := make([]downstreamPricingAccount, 0, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		if !scope.accountVisible(account.SiteID, account.AccountID) {
			continue
		}
		_, account.Supported = platform.GetAdapter(account.Platform).(platform.PricingProvider)
		visibleAccounts = append(visibleAccounts, *account)
		if account.Payload == "" {
			continue
		}
		var snapshot platform.AccountPricing
		if err := json.Unmarshal([]byte(account.Payload), &snapshot); err != nil {
			return nil, nil, page, fmt.Errorf("invalid saved account pricing: %w", err)
		}
		snapshots[account.AccountID] = snapshot
		for _, model := range snapshot.Models {
			if !scope.modelVisible(model.Model) || (modelQ != "" && !pricing.MatchesModel(model.Model, modelQ, true)) {
				continue
			}
			names[normalizeModelKey(model.Model)] = model.Model
		}
	}

	// pricingAccounts is site-level: account IDs are internal identifiers.
	accountsOut := make([]downstreamPricingAccount, 0, len(visibleAccounts))
	seenSites := map[int64]struct{}{}
	for _, account := range visibleAccounts {
		if _, ok := seenSites[account.SiteID]; ok {
			continue
		}
		seenSites[account.SiteID] = struct{}{}
		accountsOut = append(accountsOut, account)
	}

	modelKeys := make([]string, 0, len(names))
	for key := range names {
		modelKeys = append(modelKeys, key)
	}
	sort.Strings(modelKeys)
	page.TotalModels = len(modelKeys)
	pages := max(1, (len(modelKeys)+page.PageSize-1)/page.PageSize)
	page.Page = min(page.Page, pages)
	start := min((page.Page-1)*page.PageSize, len(modelKeys))
	selected := map[string]struct{}{}
	for _, key := range modelKeys[start:min(start+page.PageSize, len(modelKeys))] {
		selected[key] = struct{}{}
	}

	items := make([]downstreamCurrentPrice, 0)
	seenRows := map[string]int{}
	for _, account := range visibleAccounts {
		snapshot, ok := snapshots[account.AccountID]
		if !ok {
			continue
		}
		for _, model := range snapshot.Models {
			key := normalizeModelKey(model.Model)
			if _, ok := selected[key]; !ok {
				continue
			}
			rowKey := siteModelKey(account.SiteID, key)
			var official *pricing.OfficialPrice
			if manager := app.CatalogManager(); manager != nil {
				official = pricing.OfficialPriceFor(manager.Snapshot(), model.Model)
			}
			candidate := downstreamCurrentPrice{
				SiteID:    account.SiteID,
				SiteName:  account.SiteName,
				Model:     model.Model,
				FetchedAt: account.FetchedAt,
				Pricing:   model,
				Official:  official,
				Groups:    pricing.GroupPrices(model, snapshot, official),
			}
			if idx, ok := seenRows[rowKey]; ok {
				// Multiple accounts on one site quote the same model. Without
				// account identity the cheapest quote is the only meaningful
				// site-level answer.
				if downstreamCurrentPriceCost(candidate) < downstreamCurrentPriceCost(items[idx]) {
					items[idx] = candidate
				}
				continue
			}
			seenRows[rowKey] = len(items)
			items = append(items, candidate)
		}
	}
	return accountsOut, items, page, nil
}

func downstreamCurrentPriceCost(item downstreamCurrentPrice) float64 {
	best := math.Inf(1)
	for _, group := range item.Groups {
		if group.SampleCost != nil && *group.SampleCost < best {
			best = *group.SampleCost
		}
	}
	return best
}
