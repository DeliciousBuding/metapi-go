package admin

import (
	"encoding/json"
	"math"
	"sort"
	"strings"
	"time"
)

// Stats marketplace / model-coverage builders extracted from stats.go.
// These aggregate model_availability + token_model_availability +
// token_routes + recent proxy_logs into the marketplace, token-candidate,
// without-token, missing-token-group, and endpoint-type model surfaces.
// Behavior-neutral extraction (same package, same exported surface).
//
// See docs/analysis/engineering-optimization-2026-07-30.md §2.1 for rationale.

// marketplaceAccount aggregates per-account marketplace detail for one model.
type marketplaceAccountAgg struct {
	ID       int64
	Site     string
	Username *string
	Latency  *float64
	Balance  float64
	Tokens   []map[string]any
	tokenIDs map[int64]struct{}
}

func (h *statsHandler) buildMarketplaceModels() []map[string]any {
	// Collect model names from availability tables + exact token_routes patterns.
	type modelKey = string
	type modelAgg struct {
		accounts map[int64]*marketplaceAccountAgg
		latSum   float64
		latCount int
	}
	byModel := map[modelKey]*modelAgg{}

	ensure := func(model string) *modelAgg {
		if agg, ok := byModel[model]; ok {
			return agg
		}
		agg := &modelAgg{accounts: map[int64]*marketplaceAccountAgg{}}
		byModel[model] = agg
		return agg
	}
	ensureAccount := func(agg *modelAgg, accountID int64, site, username string, balance float64, latency *float64) *marketplaceAccountAgg {
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

	// Account-level availability.
	accountRows := queryRows(h.db, `
		SELECT
			ma.model_name AS model_name,
			ma.latency_ms AS latency_ms,
			a.id AS account_id,
			COALESCE(a.username, '') AS username,
			COALESCE(a.balance, 0) AS balance,
			COALESCE(s.name, '') AS site_name,
			COALESCE(s.platform, '') AS platform
		FROM model_availability ma
		INNER JOIN accounts a ON a.id = ma.account_id
		INNER JOIN sites s ON s.id = a.site_id
		WHERE COALESCE(ma.available, 0) = 1
			AND COALESCE(a.status, '') <> 'disabled'
			AND COALESCE(s.status, '') <> 'disabled'
		ORDER BY ma.model_name ASC, a.id ASC
	`)
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
		// Attach enabled tokens for this account (may also appear via token availability).
		tokenRows := queryRows(h.db, `
			SELECT id, name, is_default
			FROM account_tokens
			WHERE account_id = ?
				AND enabled = 1
				AND (value_status IS NULL OR value_status <> 'expired')
			ORDER BY is_default DESC, id ASC
		`, acc.ID)
		for _, tr := range tokenRows {
			tid := coerceInt64(tr["id"])
			if tid <= 0 {
				continue
			}
			if _, exists := acc.tokenIDs[tid]; exists {
				continue
			}
			acc.tokenIDs[tid] = struct{}{}
			acc.Tokens = append(acc.Tokens, map[string]any{
				"id":        tid,
				"name":      coerceString(tr["name"]),
				"isDefault": coerceInt(tr["isDefault"]) == 1 || coerceString(tr["isDefault"]) == "true" || coerceString(tr["isDefault"]) == "1",
			})
		}
	}

	// Token-level availability.
	tokenAvailRows := queryRows(h.db, `
		SELECT
			tma.model_name AS model_name,
			tma.latency_ms AS latency_ms,
			at.id AS token_id,
			COALESCE(at.name, '') AS token_name,
			at.is_default AS is_default,
			a.id AS account_id,
			COALESCE(a.username, '') AS username,
			COALESCE(a.balance, 0) AS balance,
			COALESCE(s.name, '') AS site_name
		FROM token_model_availability tma
		INNER JOIN account_tokens at ON at.id = tma.token_id
		INNER JOIN accounts a ON a.id = at.account_id
		INNER JOIN sites s ON s.id = a.site_id
		WHERE COALESCE(tma.available, 0) = 1
			AND at.enabled = 1
			AND (at.value_status IS NULL OR at.value_status <> 'expired')
			AND COALESCE(a.status, '') <> 'disabled'
			AND COALESCE(s.status, '') <> 'disabled'
		ORDER BY tma.model_name ASC, a.id ASC, at.id ASC
	`)
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
	routeRows := queryRows(h.db, `
		SELECT model_pattern
		FROM token_routes
		WHERE enabled = 1
			AND route_mode <> 'explicit_group'
	`)
	for _, row := range routeRows {
		pattern := strings.TrimSpace(coerceString(row["modelPattern"]))
		if pattern == "" {
			continue
		}
		// Only exact patterns (no wildcards / regex markers) become marketplace models.
		if strings.ContainsAny(pattern, "*?^$[]()|\\+") {
			continue
		}
		if _, ok := byModel[pattern]; !ok {
			byModel[pattern] = &modelAgg{accounts: map[int64]*marketplaceAccountAgg{}}
		}
	}

	// Optional success-rate from recent proxy_logs (bounded 7d window).
	since := time.Now().UTC().Add(-7 * 24 * time.Hour).Format(time.RFC3339)
	successByModel := map[string]struct {
		total   int
		success int
	}{}
	logRows := queryRows(h.db, `
		SELECT
			COALESCE(NULLIF(TRIM(model_actual), ''), NULLIF(TRIM(model_requested), ''), '') AS model,
			COUNT(*) AS total,
			COALESCE(SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END), 0) AS success
		FROM proxy_logs
		WHERE created_at >= ?
			AND COALESCE(NULLIF(TRIM(model_actual), ''), NULLIF(TRIM(model_requested), ''), '') <> ''
		GROUP BY COALESCE(NULLIF(TRIM(model_actual), ''), NULLIF(TRIM(model_requested), ''), '')
	`, since)
	for _, row := range logRows {
		model := strings.TrimSpace(coerceString(row["model"]))
		if model == "" {
			continue
		}
		successByModel[model] = struct {
			total   int
			success int
		}{total: coerceInt(row["total"]), success: coerceInt(row["success"])}
	}

	names := make([]string, 0, len(byModel))
	for name := range byModel {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]map[string]any, 0, len(names))
	for _, name := range names {
		agg := byModel[name]
		accounts := make([]map[string]any, 0, len(agg.accounts))
		accountIDs := make([]int64, 0, len(agg.accounts))
		for id := range agg.accounts {
			accountIDs = append(accountIDs, id)
		}
		sort.Slice(accountIDs, func(i, j int) bool { return accountIDs[i] < accountIDs[j] })
		tokenCount := 0
		for _, id := range accountIDs {
			acc := agg.accounts[id]
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
		if stats, ok := successByModel[name]; ok && stats.total > 0 {
			successRate = math.Round(1000.0*float64(stats.success)/float64(stats.total)) / 10.0
		} else {
			successRate = nil
		}
		out = append(out, map[string]any{
			"name":                   name,
			"accountCount":           len(accounts),
			"tokenCount":             tokenCount,
			"avgLatency":             avgLatency,
			"successRate":            successRate,
			"description":            nil,
			"tags":                   []string{},
			"supportedEndpointTypes": inferEndpointTypesForModel(name, accounts),
			// Pricing intentionally empty — labeled residual; use price-compare for rates.
			"pricingSources": []any{},
			"accounts":       accounts,
		})
	}
	return out
}

func inferEndpointTypesForModel(modelName string, accounts []map[string]any) []string {
	// Prefer model-name heuristics; fall back to empty when unknown.
	lower := strings.ToLower(modelName)
	switch {
	case strings.Contains(lower, "claude") || strings.HasPrefix(lower, "anthropic"):
		return []string{"anthropic"}
	case strings.Contains(lower, "gemini"):
		return []string{"gemini"}
	case strings.Contains(lower, "gpt") || strings.Contains(lower, "o1") || strings.Contains(lower, "o3") || strings.Contains(lower, "o4"):
		return []string{"openai"}
	default:
		return []string{}
	}
}

func (h *statsHandler) buildTokenCandidateModels(allowed map[string]struct{}) map[string][]map[string]any {
	rows := queryRows(h.db, `
		SELECT
			tma.model_name AS model_name,
			a.id AS account_id,
			at.id AS token_id,
			COALESCE(at.name, '') AS token_name,
			at.is_default AS is_default,
			COALESCE(a.username, '') AS username,
			s.id AS site_id,
			COALESCE(s.name, '') AS site_name
		FROM token_model_availability tma
		INNER JOIN account_tokens at ON at.id = tma.token_id
		INNER JOIN accounts a ON a.id = at.account_id
		INNER JOIN sites s ON s.id = a.site_id
		WHERE COALESCE(tma.available, 0) = 1
			AND at.enabled = 1
			AND (at.value_status IS NULL OR at.value_status <> 'expired')
			AND COALESCE(a.status, '') <> 'disabled'
			AND COALESCE(s.status, '') <> 'disabled'
		ORDER BY tma.model_name ASC, a.id ASC, at.id ASC
	`)

	out := map[string][]map[string]any{}
	seen := map[string]map[int64]struct{}{} // model -> tokenIDs
	for _, row := range rows {
		model := strings.TrimSpace(coerceString(row["modelName"]))
		if model == "" || !modelAllowed(model, allowed) {
			continue
		}
		tokenID := coerceInt64(row["tokenId"])
		if tokenID <= 0 {
			continue
		}
		if _, ok := seen[model]; !ok {
			seen[model] = map[int64]struct{}{}
		}
		if _, dup := seen[model][tokenID]; dup {
			continue
		}
		seen[model][tokenID] = struct{}{}
		var username any
		if u := strings.TrimSpace(coerceString(row["username"])); u != "" {
			username = u
		} else {
			username = nil
		}
		out[model] = append(out[model], map[string]any{
			"accountId": coerceInt64(row["accountId"]),
			"tokenId":   tokenID,
			"tokenName": coerceString(row["tokenName"]),
			"isDefault": coerceInt(row["isDefault"]) == 1 || coerceString(row["isDefault"]) == "true" || coerceString(row["isDefault"]) == "1",
			"username":  username,
			"siteId":    coerceInt64(row["siteId"]),
			"siteName":  coerceString(row["siteName"]),
		})
	}
	return out
}

func (h *statsHandler) buildModelsWithoutToken(allowed map[string]struct{}) map[string][]map[string]any {
	// Accounts with available model_availability but no token_model_availability
	// coverage for that model AND no enabled account_tokens at all (or none that
	// list the model). Operators use this for zero-channel route hints.
	rows := queryRows(h.db, `
		SELECT
			ma.model_name AS model_name,
			a.id AS account_id,
			COALESCE(a.username, '') AS username,
			s.id AS site_id,
			COALESCE(s.name, '') AS site_name
		FROM model_availability ma
		INNER JOIN accounts a ON a.id = ma.account_id
		INNER JOIN sites s ON s.id = a.site_id
		WHERE COALESCE(ma.available, 0) = 1
			AND COALESCE(a.status, '') <> 'disabled'
			AND COALESCE(s.status, '') <> 'disabled'
			AND NOT EXISTS (
				SELECT 1
				FROM account_tokens at
				INNER JOIN token_model_availability tma ON tma.token_id = at.id
				WHERE at.account_id = a.id
					AND at.enabled = 1
					AND (at.value_status IS NULL OR at.value_status <> 'expired')
					AND COALESCE(tma.available, 0) = 1
					AND tma.model_name = ma.model_name
			)
			AND NOT EXISTS (
				-- API-key style accounts that store a single key without managed tokens
				-- are still "without token" for route channel binding when no account_tokens rows exist.
				SELECT 1 FROM account_tokens at
				WHERE at.account_id = a.id
					AND at.enabled = 1
					AND (at.value_status IS NULL OR at.value_status <> 'expired')
			)
		ORDER BY ma.model_name ASC, a.id ASC
	`)

	out := map[string][]map[string]any{}
	seen := map[string]map[int64]struct{}{}
	for _, row := range rows {
		model := strings.TrimSpace(coerceString(row["modelName"]))
		if model == "" || !modelAllowed(model, allowed) {
			continue
		}
		accountID := coerceInt64(row["accountId"])
		if accountID <= 0 {
			continue
		}
		if _, ok := seen[model]; !ok {
			seen[model] = map[int64]struct{}{}
		}
		if _, dup := seen[model][accountID]; dup {
			continue
		}
		seen[model][accountID] = struct{}{}
		var username any
		if u := strings.TrimSpace(coerceString(row["username"])); u != "" {
			username = u
		} else {
			username = nil
		}
		out[model] = append(out[model], map[string]any{
			"accountId": accountID,
			"username":  username,
			"siteId":    coerceInt64(row["siteId"]),
			"siteName":  coerceString(row["siteName"]),
		})
	}
	return out
}

func (h *statsHandler) buildModelsMissingTokenGroups(allowed map[string]struct{}) map[string][]map[string]any {
	// When an account has model availability and managed tokens, but none of the
	// enabled tokens have a resolvable token_group label, group coverage is uncertain
	// / missing. We do not invent required groups from a remote pricing catalog.
	rows := queryRows(h.db, `
		SELECT
			ma.model_name AS model_name,
			a.id AS account_id,
			COALESCE(a.username, '') AS username,
			s.id AS site_id,
			COALESCE(s.name, '') AS site_name,
			COALESCE(at.token_group, '') AS token_group,
			COALESCE(at.name, '') AS token_name
		FROM model_availability ma
		INNER JOIN accounts a ON a.id = ma.account_id
		INNER JOIN sites s ON s.id = a.site_id
		INNER JOIN account_tokens at ON at.account_id = a.id
		WHERE COALESCE(ma.available, 0) = 1
			AND COALESCE(a.status, '') <> 'disabled'
			AND COALESCE(s.status, '') <> 'disabled'
			AND at.enabled = 1
			AND (at.value_status IS NULL OR at.value_status <> 'expired')
		ORDER BY ma.model_name ASC, a.id ASC, at.id ASC
	`)

	type accGroups struct {
		accountID int64
		username  string
		siteID    int64
		siteName  string
		available []string
		uncertain bool
	}
	// model -> accountID -> groups
	byModel := map[string]map[int64]*accGroups{}
	for _, row := range rows {
		model := strings.TrimSpace(coerceString(row["modelName"]))
		if model == "" || !modelAllowed(model, allowed) {
			continue
		}
		accountID := coerceInt64(row["accountId"])
		if accountID <= 0 {
			continue
		}
		if _, ok := byModel[model]; !ok {
			byModel[model] = map[int64]*accGroups{}
		}
		ag, ok := byModel[model][accountID]
		if !ok {
			ag = &accGroups{
				accountID: accountID,
				username:  coerceString(row["username"]),
				siteID:    coerceInt64(row["siteId"]),
				siteName:  coerceString(row["siteName"]),
			}
			byModel[model][accountID] = ag
		}
		group := resolveTokenGroupLabel(coerceString(row["tokenGroup"]), coerceString(row["tokenName"]))
		if group == "" {
			ag.uncertain = true
			continue
		}
		// de-dupe
		found := false
		for _, g := range ag.available {
			if strings.EqualFold(g, group) {
				found = true
				break
			}
		}
		if !found {
			ag.available = append(ag.available, group)
		}
	}

	out := map[string][]map[string]any{}
	for model, accounts := range byModel {
		for _, ag := range accounts {
			// Only surface accounts where no token has a resolvable group label.
			if len(ag.available) > 0 {
				continue
			}
			var username any
			if strings.TrimSpace(ag.username) != "" {
				username = ag.username
			} else {
				username = nil
			}
			item := map[string]any{
				"accountId":              ag.accountID,
				"username":               username,
				"siteId":                 ag.siteID,
				"siteName":               ag.siteName,
				"missingGroups":          []string{},
				"requiredGroups":         []string{},
				"availableGroups":        []string{},
				"groupCoverageUncertain": true,
			}
			out[model] = append(out[model], item)
		}
	}
	return out
}

func resolveTokenGroupLabel(tokenGroup, tokenName string) string {
	group := strings.TrimSpace(tokenGroup)
	if group != "" {
		return group
	}
	name := strings.TrimSpace(tokenName)
	if name == "" {
		return ""
	}
	lower := strings.ToLower(name)
	if lower == "default" || name == "默认" {
		return ""
	}
	// Names like token-1 / token-N are not group labels.
	if strings.HasPrefix(lower, "token-") {
		return ""
	}
	return name
}

func (h *statsHandler) buildEndpointTypesByModel(allowed map[string]struct{}) map[string][]string {
	// Union of endpoint types inferred from model names present in availability.
	models := map[string]struct{}{}
	for _, row := range queryRows(h.db, `
		SELECT DISTINCT model_name AS model_name FROM model_availability WHERE COALESCE(available, 0) = 1
		UNION
		SELECT DISTINCT model_name AS model_name FROM token_model_availability WHERE COALESCE(available, 0) = 1
	`) {
		model := strings.TrimSpace(coerceString(row["modelName"]))
		if model == "" || !modelAllowed(model, allowed) {
			continue
		}
		models[model] = struct{}{}
	}
	out := map[string][]string{}
	for model := range models {
		types := inferEndpointTypesForModel(model, nil)
		if len(types) == 0 {
			// default OpenAI-compatible when unknown
			types = []string{"openai"}
		}
		out[model] = types
	}
	return out
}

func (h *statsHandler) loadGlobalAllowedModels() map[string]struct{} {
	// Optional whitelist from settings table. Empty / missing → allow all.
	var raw string
	err := h.db.Get(&raw, rebindAdminQuery(h.db, `SELECT value FROM settings WHERE key = ?`), "global_allowed_models")
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	var list []string
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		// also accept CSV
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				list = append(list, part)
			}
		}
	}
	if len(list) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(list))
	for _, m := range list {
		m = strings.TrimSpace(m)
		if m != "" {
			out[strings.ToLower(m)] = struct{}{}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

