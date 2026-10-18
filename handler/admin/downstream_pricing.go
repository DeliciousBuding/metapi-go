package admin

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/app"
	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	proxyhandler "github.com/deliciousbuding/metapi-go/handler/proxy"
	"github.com/deliciousbuding/metapi-go/platform"
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

// effectiveModelSQL is the canonical model name for a proxy log: the upstream
// actual model when known, else the requested model. Kept in one place so the
// top-model and observed-price queries aggregate the same expression.
const effectiveModelSQL = `COALESCE(NULLIF(TRIM(pl.model_actual), ''), NULLIF(TRIM(pl.model_requested), ''), '')`

// downstreamPricingScope is the visibility filter derived from the managed
// key's routing policy. It deliberately mirrors routing's allow/exclude
// semantics (see routing.ChannelSelector.resolveDownstreamExclusionBlocker)
// instead of inventing a second policy language for pricing.
type downstreamPricingScope struct {
	policy routing.DownstreamRoutingPolicy

	allowedSites  map[int64]struct{}
	excludedSites map[int64]struct{}

	// accountID -> siteID. Credential refs that cannot be mapped to an account
	// (direct grants) contribute nothing: their pricing rows cannot be
	// attributed reliably, so they are never returned.
	allowedAccountSites  map[int64]int64
	excludedAccountSites map[int64]int64
	credentialRestricted bool

	// accountPricingVisible mirrors the routing check for account-backed
	// candidates (non-direct candidates carry upstream channel ID 0). A policy
	// restricted to specific imported direct channels has no account-backed
	// pricing rows to expose.
	accountPricingVisible bool
}

func newDownstreamPricingScope(policy auth.DownstreamRoutingPolicy) *downstreamPricingScope {
	routePolicy := proxyhandler.RoutingPolicyFromAuth(policy)
	scope := &downstreamPricingScope{
		policy:                routePolicy,
		allowedSites:          map[int64]struct{}{},
		excludedSites:         map[int64]struct{}{},
		allowedAccountSites:   map[int64]int64{},
		excludedAccountSites:  map[int64]int64{},
		credentialRestricted:  len(routePolicy.AllowedCredentialRefs) > 0,
		accountPricingVisible: routePolicy.AccessPolicy.AllowsUpstreamChannel(0),
	}
	for _, id := range routePolicy.AllowedSiteIDs {
		if id > 0 {
			scope.allowedSites[id] = struct{}{}
		}
	}
	for _, id := range routePolicy.ExcludedSiteIDs {
		if id > 0 {
			scope.excludedSites[id] = struct{}{}
		}
	}
	for _, ref := range routePolicy.AllowedCredentialRefs {
		if siteID, accountID, ok := credentialRefAccount(ref); ok {
			scope.allowedAccountSites[accountID] = siteID
		}
	}
	for _, ref := range routePolicy.ExcludedCredentialRefs {
		if siteID, accountID, ok := credentialRefAccount(ref); ok {
			scope.excludedAccountSites[accountID] = siteID
		}
	}
	return scope
}

// credentialRefAccount maps a credential reference onto the account whose
// aggregate pricing row it selects. Only account-level refs are mappable;
// direct grants are a different upstream dimension and map to nothing.
func credentialRefAccount(ref routing.CredentialRef) (siteID, accountID int64, ok bool) {
	switch ref.Kind {
	case string(auth.CredentialRefAccountToken), string(auth.CredentialRefDefaultApiKey), "":
		if ref.SiteID <= 0 || ref.AccountID <= 0 {
			return 0, 0, false
		}
		return ref.SiteID, ref.AccountID, true
	default:
		return 0, 0, false
	}
}

func (s *downstreamPricingScope) siteVisible(siteID int64) bool {
	if siteID <= 0 {
		return false
	}
	if len(s.allowedSites) > 0 {
		if _, ok := s.allowedSites[siteID]; !ok {
			return false
		}
	}
	_, excluded := s.excludedSites[siteID]
	return !excluded
}

func (s *downstreamPricingScope) accountVisible(siteID, accountID int64) bool {
	if !s.siteVisible(siteID) {
		return false
	}
	if refSite, ok := s.excludedAccountSites[accountID]; ok && refSite == siteID {
		return false
	}
	if s.credentialRestricted {
		refSite, ok := s.allowedAccountSites[accountID]
		if !ok || refSite != siteID {
			return false
		}
	}
	return true
}

// modelVisible reuses the routing policy's model visibility (AccessPolicy
// model list + supported-model patterns/routes). The pricing surface is
// upstream-model oriented, so a rename target that is not itself allowed by
// the policy is withheld.
func (s *downstreamPricingScope) modelVisible(model string) bool {
	return routing.IsModelAllowedByDownstreamPolicy(model, s.policy)
}

// accountFilters returns " AND ..." clauses for an accounts/sites join. Column
// aliases are supplied by the caller so the same policy is pushed into every
// aggregation query instead of being applied only after loading all rows.
func (s *downstreamPricingScope) accountFilters(siteCol, accountCol string) (string, []any) {
	clauses := make([]string, 0, 4)
	args := make([]any, 0, 4)

	if len(s.allowedSites) > 0 {
		ids := sortedInt64Keys(s.allowedSites)
		clauses = append(clauses, siteCol+" IN ("+sqlPlaceholders(len(ids))+")")
		for _, id := range ids {
			args = append(args, id)
		}
	}
	if len(s.excludedSites) > 0 {
		ids := sortedInt64Keys(s.excludedSites)
		clauses = append(clauses, siteCol+" NOT IN ("+sqlPlaceholders(len(ids))+")")
		for _, id := range ids {
			args = append(args, id)
		}
	}
	if s.credentialRestricted {
		if len(s.allowedAccountSites) == 0 {
			// An allow-list made only of direct grants authorizes no account
			// pricing row.
			clauses = append(clauses, "1 = 0")
		} else {
			ids := sortedInt64MapKeys(s.allowedAccountSites)
			clauses = append(clauses, accountCol+" IN ("+sqlPlaceholders(len(ids))+")")
			for _, id := range ids {
				args = append(args, id)
			}
		}
	}
	if len(s.excludedAccountSites) > 0 {
		ids := sortedInt64MapKeys(s.excludedAccountSites)
		clauses = append(clauses, accountCol+" NOT IN ("+sqlPlaceholders(len(ids))+")")
		for _, id := range ids {
			args = append(args, id)
		}
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " AND " + strings.Join(clauses, " AND "), args
}

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

	baselines := downstreamOfficialPrices(modelQ, items, current, currentPage)
	// Only advertise accounts for sites that actually contributed a row to this
	// response. A key restricted to a model gets no site topology beyond the
	// sites that price that model.
	accounts = filterDownstreamPricingAccounts(accounts, items, current)
	writeDownstreamPricingResponse(w, modelQ, days, limit, len(models), items, accounts, current, baselines, currentPage)
}

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

// downstreamTopModels returns a bounded list of the busiest policy-visible
// models. Availability is the fallback when there is no successful traffic yet,
// mirroring the admin view's cold-start behaviour.
func (h *statsHandler) downstreamTopModels(scope *downstreamPricingScope, limit int, since string) ([]string, error) {
	fetchLimit := limit * downstreamPricingTopModelsFetch
	filters, filterArgs := scope.accountFilters("a.site_id", "a.id")
	query := `
		SELECT ` + effectiveModelSQL + ` AS model, COUNT(*) AS calls
		FROM proxy_logs pl
		JOIN accounts a ON a.id = pl.account_id
		JOIN sites s ON s.id = a.site_id
		WHERE pl.created_at >= ?
			AND pl.status = 'success'
			AND ` + effectiveModelSQL + ` <> ''
			AND COALESCE(a.status, '') <> 'disabled'
			AND COALESCE(s.status, '') <> 'disabled'` + filters + `
		GROUP BY ` + effectiveModelSQL + `
		ORDER BY calls DESC
		LIMIT ?`
	args := append([]any{since}, filterArgs...)
	args = append(args, fetchLimit)
	rows, err := queryRowsErr(h.db, query, args...)
	if err != nil {
		return nil, err
	}

	out := make([]string, 0, limit)
	seen := map[string]struct{}{}
	appendModel := func(raw string) {
		model := strings.TrimSpace(raw)
		if model == "" || len(out) >= limit || !scope.modelVisible(model) {
			return
		}
		key := normalizeModelKey(model)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, model)
	}
	for _, row := range rows {
		appendModel(coerceString(row["model"]))
	}
	if len(out) > 0 {
		return out, nil
	}

	filters, filterArgs = scope.accountFilters("a.site_id", "a.id")
	availQuery := `
		SELECT ma.model_name AS model, COUNT(*) AS cnt
		FROM model_availability ma
		JOIN accounts a ON a.id = ma.account_id
		JOIN sites s ON s.id = a.site_id
		WHERE COALESCE(a.status, '') <> 'disabled'
			AND COALESCE(s.status, '') <> 'disabled'` + filters + `
		GROUP BY ma.model_name
		ORDER BY cnt DESC
		LIMIT ?`
	args = append(filterArgs, fetchLimit)
	availRows, err := queryRowsErr(h.db, availQuery, args...)
	if err != nil {
		return nil, err
	}
	for _, row := range availRows {
		appendModel(coerceString(row["model"]))
	}
	return out, nil
}

// downstreamObservedSignals aggregates successful cost-bearing traffic once per
// account/model for the whole requested model set.
func (h *statsHandler) downstreamObservedSignals(scope *downstreamPricingScope, models []string, since string) (map[int64]map[string]observedPriceSignal, error) {
	if len(models) == 0 {
		return map[int64]map[string]observedPriceSignal{}, nil
	}
	filters, filterArgs := scope.accountFilters("a.site_id", "a.id")
	placeholders := sqlPlaceholders(len(models))
	query := `
		SELECT
			pl.account_id AS account_id,
			` + effectiveModelSQL + ` AS model_name,
			AVG(COALESCE(pl.estimated_cost, 0)) AS avg_cost,
			COUNT(*) AS samples,
			MAX(COALESCE(pl.billing_details, '')) AS billing_details
		FROM proxy_logs pl
		JOIN accounts a ON a.id = pl.account_id
		JOIN sites s ON s.id = a.site_id
		WHERE pl.created_at >= ?
			AND pl.status = 'success'
			AND COALESCE(pl.estimated_cost, 0) > 0
			AND LOWER(` + effectiveModelSQL + `) IN (` + placeholders + `)
			AND COALESCE(a.status, '') <> 'disabled'
			AND COALESCE(s.status, '') <> 'disabled'` + filters + `
		GROUP BY pl.account_id, ` + effectiveModelSQL
	args := make([]any, 0, 1+len(models)+len(filterArgs))
	args = append(args, since)
	for _, model := range models {
		args = append(args, normalizeModelKey(model))
	}
	args = append(args, filterArgs...)
	rows, err := queryRowsErr(h.db, query, args...)
	if err != nil {
		return nil, err
	}

	out := map[int64]map[string]observedPriceSignal{}
	for _, row := range rows {
		accountID := coerceInt64(row["accountId"])
		if accountID <= 0 {
			continue
		}
		modelName := strings.TrimSpace(coerceString(row["modelName"]))
		key := normalizeModelKey(modelName)
		if key == "" {
			continue
		}
		signal := observedPriceSignal{
			AvgCost:        coerceFloat(row["avgCost"]),
			Samples:        coerceInt(row["samples"]),
			BillingDetails: coerceString(row["billingDetails"]),
			ResolvedModel:  modelName,
		}
		byModel := out[accountID]
		if byModel == nil {
			byModel = map[string]observedPriceSignal{}
			out[accountID] = byModel
		}
		// Different casings of the same model can group separately. Keep the
		// highest-sample signal per account/model, as the admin view does.
		if prev, ok := byModel[key]; ok && prev.Samples >= signal.Samples {
			continue
		}
		byModel[key] = signal
	}
	return out, nil
}

// downstreamAvailability loads one row per account/model for the requested
// model set, scoped to sites/accounts the key may see.
func (h *statsHandler) downstreamAvailability(scope *downstreamPricingScope, models []string) (map[int64]map[string]string, error) {
	if len(models) == 0 {
		return map[int64]map[string]string{}, nil
	}
	filters, filterArgs := scope.accountFilters("a.site_id", "a.id")
	query := `
		SELECT ma.account_id AS account_id, ma.model_name AS model_name
		FROM model_availability ma
		JOIN accounts a ON a.id = ma.account_id
		JOIN sites s ON s.id = a.site_id
		WHERE LOWER(TRIM(ma.model_name)) IN (` + sqlPlaceholders(len(models)) + `)
			AND COALESCE(a.status, '') <> 'disabled'
			AND COALESCE(s.status, '') <> 'disabled'` + filters
	args := make([]any, 0, len(models)+len(filterArgs))
	for _, model := range models {
		args = append(args, normalizeModelKey(model))
	}
	args = append(args, filterArgs...)
	rows, err := queryRowsErr(h.db, query, args...)
	if err != nil {
		return nil, err
	}

	out := map[int64]map[string]string{}
	for _, row := range rows {
		accountID := coerceInt64(row["accountId"])
		if accountID <= 0 {
			continue
		}
		modelName := strings.TrimSpace(coerceString(row["modelName"]))
		key := normalizeModelKey(modelName)
		if key == "" {
			continue
		}
		byModel := out[accountID]
		if byModel == nil {
			byModel = map[string]string{}
			out[accountID] = byModel
		}
		if _, ok := byModel[key]; !ok {
			byModel[key] = modelName
		}
	}
	return out, nil
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
	if err := h.db.SelectContext(r.Context(), &accounts, rebindAdminQuery(h.db, query), filterArgs...); err != nil {
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

func sortedInt64Keys(set map[int64]struct{}) []int64 {
	out := make([]int64, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func sortedInt64MapKeys(set map[int64]int64) []int64 {
	out := make([]int64, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func sqlPlaceholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}
