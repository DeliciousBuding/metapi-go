package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/deliciousbuding/metapi-go/app"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/service/pricing"
)

type pricingAccount struct {
	AccountID   int64  `db:"account_id" json:"accountId"`
	SiteName    string `db:"site_name" json:"siteName"`
	Username    string `db:"username" json:"username"`
	Platform    string `db:"platform" json:"platform"`
	FetchedAt   string `db:"fetched_at" json:"fetchedAt"`
	AttemptedAt string `db:"attempted_at" json:"attemptedAt"`
	LastError   string `db:"last_error" json:"lastError"`
	Payload     string `db:"payload" json:"-"`
	Supported   bool   `json:"supported"`
}

type currentPrice struct {
	AccountID int64                  `json:"accountId"`
	SiteName  string                 `json:"siteName"`
	Username  string                 `json:"username"`
	Model     string                 `json:"model"`
	FetchedAt string                 `json:"fetchedAt"`
	LastError string                 `json:"lastError"`
	Pricing   platform.ModelPrice    `json:"pricing"`
	Groups    []pricing.GroupPrice   `json:"groups"`
	Official  *pricing.OfficialPrice `json:"official"`
}

type currentPricePage struct {
	Page        int `json:"page"`
	PageSize    int `json:"pageSize"`
	TotalModels int `json:"totalModels"`
}

func (h *statsHandler) currentPriceComparison(r *http.Request, query string, exact bool) ([]pricingAccount, []currentPrice, currentPricePage, error) {
	page := currentPricePage{Page: config.ClampInt(getQueryInt(r, "pricePage", 1), 1, 1_000_000), PageSize: config.ClampInt(getQueryInt(r, "pricePageSize", 10), 1, 50)}
	accounts := make([]pricingAccount, 0)
	err := h.db.SelectContext(r.Context(), &accounts, `SELECT a.id AS account_id,s.name AS site_name,COALESCE(a.username,'') AS username,s.platform,
	COALESCE(p.payload,'') AS payload,COALESCE(p.fetched_at,'') AS fetched_at,COALESCE(p.attempted_at,'') AS attempted_at,COALESCE(p.last_error,'') AS last_error
	FROM accounts a JOIN sites s ON s.id=a.site_id LEFT JOIN account_pricing p ON p.account_id=a.id
	WHERE COALESCE(a.status,'') <> 'disabled' AND COALESCE(s.status,'') <> 'disabled' ORDER BY s.name,a.id`)
	if err != nil {
		return nil, nil, page, err
	}
	items := make([]currentPrice, 0)
	snapshots := make(map[int64]platform.AccountPricing, len(accounts))
	names := map[string]string{}
	for i := range accounts {
		account := &accounts[i]
		_, account.Supported = platform.GetAdapter(account.Platform).(platform.PricingProvider)
		if account.Payload == "" {
			continue
		}
		var snapshot platform.AccountPricing
		if err := json.Unmarshal([]byte(account.Payload), &snapshot); err != nil {
			return nil, nil, page, fmt.Errorf("invalid saved account pricing: %w", err)
		}
		snapshots[account.AccountID] = snapshot
		for _, model := range snapshot.Models {
			if pricing.MatchesModel(model.Model, query, exact) {
				names[strings.ToLower(model.Model)] = model.Model
			}
		}
	}
	models := make([]string, 0, len(names))
	for key := range names {
		models = append(models, key)
	}
	sort.Strings(models)
	page.TotalModels = len(models)
	pages := max(1, (len(models)+page.PageSize-1)/page.PageSize)
	page.Page = min(page.Page, pages)
	start := min((page.Page-1)*page.PageSize, len(models))
	selected := map[string]bool{}
	for _, name := range models[start:min(start+page.PageSize, len(models))] {
		selected[name] = true
	}
	for _, account := range accounts {
		snapshot := snapshots[account.AccountID]
		for _, m := range snapshot.Models {
			if !selected[strings.ToLower(m.Model)] {
				continue
			}
			var official *pricing.OfficialPrice
			if manager := app.CatalogManager(); manager != nil {
				official = pricing.OfficialPriceFor(manager.Snapshot(), m.Model)
			}
			items = append(items, currentPrice{AccountID: account.AccountID, SiteName: account.SiteName, Username: account.Username, Model: m.Model, FetchedAt: account.FetchedAt, LastError: account.LastError, Pricing: m, Official: official, Groups: pricing.GroupPrices(m, snapshot, official)})
		}
	}
	return accounts, items, page, nil
}
