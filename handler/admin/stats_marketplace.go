package admin

import (
	"sort"
	"strings"
	"time"
)

// marketplaceAccount aggregates per-account marketplace detail for one model.
type marketplaceAccountAgg struct {
	ID       int64
	SiteID   int64
	Kind     string
	Site     string
	Username *string
	Latency  *float64
	Balance  float64
	Tokens   []map[string]any
	tokenIDs map[int64]struct{}
}

// marketplaceModelAgg accumulates one model's per-account availability rows
// before hydration.
type marketplaceModelAgg struct {
	accounts map[int64]*marketplaceAccountAgg
	latSum   float64
	latCount int
}

// marketplaceSuccessStat is the 7d proxy_logs outcome tally for one model.
type marketplaceSuccessStat struct {
	total   int
	success int
}

// marketplaceAggregation is the fleet-wide batch: a fixed set of queries
// whose cost does not depend on how many models a page eventually renders.
// Filtering, sorting, totals, and facets run on the lightweight index
// projection of this aggregation; full hydration happens afterwards, only
// for the rows a response actually returns.
type marketplaceAggregation struct {
	byModel        map[string]*marketplaceModelAgg
	directSources  map[string][]marketplaceModelSource
	successByModel map[string]marketplaceSuccessStat
	names          []string
}

// buildMarketplaceModels hydrates every marketplace row. Legacy callers that
// omit the pagination/query contract keep this full-list behavior.
func (h *statsHandler) buildMarketplaceModels() ([]map[string]any, error) {
	aggregation, err := h.buildMarketplaceAggregation()
	if err != nil {
		return nil, err
	}
	catalogSnapshot := appCatalogSnapshot()
	out := make([]map[string]any, 0, len(aggregation.names))
	for _, name := range aggregation.names {
		out = append(out, hydrateMarketplaceRow(aggregation, name, catalogSnapshot))
	}
	return out, nil
}

// buildMarketplaceAggregation collects model names from the availability
// tables, exact token_routes patterns, and direct upstream grants, then
// attaches per-account detail and the bounded 7d outcome window.
func (h *statsHandler) buildMarketplaceAggregation() (*marketplaceAggregation, error) {
	byModel := map[string]*marketplaceModelAgg{}

	ensure := func(model string) *marketplaceModelAgg {
		if agg, ok := byModel[model]; ok {
			return agg
		}
		agg := &marketplaceModelAgg{accounts: map[int64]*marketplaceAccountAgg{}}
		byModel[model] = agg
		return agg
	}
	ensureAccount := func(agg *marketplaceModelAgg, accountID int64, site, username string, balance float64, latency *float64) *marketplaceAccountAgg {
		acc, ok := agg.accounts[accountID]
		if !ok {
			var userPtr *string
			if strings.TrimSpace(username) != "" {
				u := username
				userPtr = &u
			}
			acc = &marketplaceAccountAgg{
				ID:       accountID,
				Site:     site,
				Username: userPtr,
				Balance:  balance,
				Tokens:   []map[string]any{},
				tokenIDs: map[int64]struct{}{},
			}
			agg.accounts[accountID] = acc
		}
		if latency != nil {
			if acc.Latency == nil || *latency < *acc.Latency {
				v := *latency
				acc.Latency = &v
			}
			agg.latSum += *latency
			agg.latCount++
		}
		return acc
	}

	// Enabled, non-expired tokens for every account in ONE query (the
	// N+1 fix): the previous shape re-ran the per-account token query inside
	// the accountRows loop below — once per model×account availability row,
	// i.e. hundreds of identical-round-trip queries per marketplace render.
	// account_tokens is fleet-bounded (a handful of tokens per account), so a
	// single scan grouped in Go is safe on both dialects. Rows arrive ordered
	// (account_id, is_default DESC, id ASC), so each account's slice keeps
	// exactly the ordering the legacy per-account query produced.
	enabledTokensByAccount := make(map[int64][]map[string]any)
	enabledTokenRows, err := queryRowsErr(h.db, `
		SELECT id, account_id, name, is_default
		FROM account_tokens
		WHERE enabled = true
			AND (value_status IS NULL OR value_status <> 'expired')
		ORDER BY account_id ASC, is_default DESC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	for _, tr := range enabledTokenRows {
		tid := coerceInt64(tr["id"])
		if tid <= 0 {
			continue
		}
		accountID := coerceInt64(tr["accountId"])
		enabledTokensByAccount[accountID] = append(enabledTokensByAccount[accountID], map[string]any{
			"id":        tid,
			"name":      coerceString(tr["name"]),
			"isDefault": coerceInt(tr["isDefault"]) == 1 || coerceString(tr["isDefault"]) == "true" || coerceString(tr["isDefault"]) == "1",
		})
	}

	// Account-level availability.
	accountRows, err := queryRowsErr(h.db, `
		SELECT
			ma.model_name AS model_name,
			ma.latency_ms AS latency_ms,
			a.id AS account_id,
			COALESCE(a.username, '') AS username,
			COALESCE(a.balance, 0) AS balance,
			s.id AS site_id, COALESCE(a.oauth_provider, '') AS oauth_provider,
			COALESCE(s.name, '') AS site_name,
			COALESCE(s.platform, '') AS platform
		FROM model_availability ma
		INNER JOIN accounts a ON a.id = ma.account_id
		INNER JOIN sites s ON s.id = a.site_id
		WHERE COALESCE(ma.available, false) = true
			AND COALESCE(a.status, '') <> 'disabled'
			AND COALESCE(s.status, '') <> 'disabled'
		ORDER BY ma.model_name ASC, a.id ASC
	`)
	if err != nil {
		return nil, err
	}
	for _, row := range accountRows {
		model := strings.TrimSpace(coerceString(row["modelName"]))
		if model == "" {
			continue
		}
		agg := ensure(model)
		var latency *float64
		if row["latencyMs"] != nil && coerceString(row["latencyMs"]) != "" {
			v := coerceFloat(row["latencyMs"])
			latency = &v
		}
		acc := ensureAccount(agg,
			coerceInt64(row["accountId"]),
			coerceString(row["siteName"]),
			coerceString(row["username"]),
			coerceFloat(row["balance"]),
			latency,
		)
		acc.SiteID = coerceInt64(row["siteId"])
		acc.Kind = "account"
		if strings.TrimSpace(coerceString(row["oauthProvider"])) != "" {
			acc.Kind = "oauth"
		}
		// Attach enabled tokens for this account (may also appear via token
		// availability) from the single pre-loaded batch above.
		for _, token := range enabledTokensByAccount[acc.ID] {
			tid := coerceInt64(token["id"])
			if _, exists := acc.tokenIDs[tid]; exists {
				continue
			}
			acc.tokenIDs[tid] = struct{}{}
			acc.Tokens = append(acc.Tokens, token)
		}
	}

	// Token-level availability.
	tokenAvailRows, err := queryRowsErr(h.db, `
		SELECT
			tma.model_name AS model_name,
			tma.latency_ms AS latency_ms,
			at.id AS token_id,
			COALESCE(at.name, '') AS token_name,
			at.is_default AS is_default,
			a.id AS account_id,
			COALESCE(a.username, '') AS username,
			COALESCE(a.balance, 0) AS balance,
			s.id AS site_id, COALESCE(a.oauth_provider, '') AS oauth_provider,
			COALESCE(s.name, '') AS site_name
		FROM token_model_availability tma
		INNER JOIN account_tokens at ON at.id = tma.token_id
		INNER JOIN accounts a ON a.id = at.account_id
		INNER JOIN sites s ON s.id = a.site_id
		WHERE COALESCE(tma.available, false) = true
			AND at.enabled = true
			AND (at.value_status IS NULL OR at.value_status <> 'expired')
			AND COALESCE(a.status, '') <> 'disabled'
			AND COALESCE(s.status, '') <> 'disabled'
		ORDER BY tma.model_name ASC, a.id ASC, at.id ASC
	`)
	if err != nil {
		return nil, err
	}
	for _, row := range tokenAvailRows {
		model := strings.TrimSpace(coerceString(row["modelName"]))
		if model == "" {
			continue
		}
		agg := ensure(model)
		var latency *float64
		if row["latencyMs"] != nil && coerceString(row["latencyMs"]) != "" {
			v := coerceFloat(row["latencyMs"])
			latency = &v
		}
		acc := ensureAccount(agg,
			coerceInt64(row["accountId"]),
			coerceString(row["siteName"]),
			coerceString(row["username"]),
			coerceFloat(row["balance"]),
			latency,
		)
		acc.SiteID = coerceInt64(row["siteId"])
		acc.Kind = "account"
		if strings.TrimSpace(coerceString(row["oauthProvider"])) != "" {
			acc.Kind = "oauth"
		}
		tid := coerceInt64(row["tokenId"])
		if tid <= 0 {
			continue
		}
		if _, exists := acc.tokenIDs[tid]; exists {
			continue
		}
		acc.tokenIDs[tid] = struct{}{}
		acc.Tokens = append(acc.Tokens, map[string]any{
			"id":        tid,
			"name":      coerceString(row["tokenName"]),
			"isDefault": coerceInt(row["isDefault"]) == 1 || coerceString(row["isDefault"]) == "true" || coerceString(row["isDefault"]) == "1",
		})
	}

	// Exact-model token_routes contribute model names even when availability is empty
	// so operators still see configured routes in the marketplace list.
	routeRows, err := queryRowsErr(h.db, `
		SELECT model_pattern
		FROM token_routes
		WHERE enabled = true
			AND route_mode <> 'explicit_group'
	`)
	if err != nil {
		return nil, err
	}
	for _, row := range routeRows {
		pattern := strings.TrimSpace(coerceString(row["modelPattern"]))
		if pattern == "" {
			continue
		}
		// Only exact patterns (no wildcards / regex markers) become marketplace models.
		if !marketplaceExactModel(pattern) {
			continue
		}
		ensure(pattern)
	}
	directSources, err := h.marketplaceDirectSources()
	if err != nil {
		return nil, err
	}
	for model := range directSources {
		ensure(model)
	}

	// Optional success-rate from recent proxy_logs (bounded 7d window).
	since := time.Now().UTC().Add(-7 * 24 * time.Hour).Format(time.RFC3339)
	successByModel := map[string]marketplaceSuccessStat{}
	logRows, err := queryRowsErr(h.db, `
		SELECT
			COALESCE(NULLIF(TRIM(model_actual), ''), NULLIF(TRIM(model_requested), ''), '') AS model,
			COUNT(*) AS total,
			COALESCE(SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END), 0) AS success
		FROM proxy_logs
		WHERE created_at >= ?
			AND COALESCE(NULLIF(TRIM(model_actual), ''), NULLIF(TRIM(model_requested), ''), '') <> ''
		GROUP BY COALESCE(NULLIF(TRIM(model_actual), ''), NULLIF(TRIM(model_requested), ''), '')
	`, since)
	if err != nil {
		return nil, err
	}
	for _, row := range logRows {
		model := strings.TrimSpace(coerceString(row["model"]))
		if model == "" {
			continue
		}
		successByModel[model] = marketplaceSuccessStat{total: coerceInt(row["total"]), success: coerceInt(row["success"])}
	}

	names := make([]string, 0, len(byModel))
	for name := range byModel {
		names = append(names, name)
	}
	sort.Strings(names)

	return &marketplaceAggregation{
		byModel:        byModel,
		directSources:  directSources,
		successByModel: successByModel,
		names:          names,
	}, nil
}
