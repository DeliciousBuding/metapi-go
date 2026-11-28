package admin

import (
	"encoding/json"
	"strings"
)

func (h *statsHandler) buildTokenCandidateModels(allowed map[string]struct{}) (map[string][]map[string]any, error) {
	rows, err := queryRowsErr(h.db.DB, `
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
	return out, nil
}

func (h *statsHandler) buildModelsWithoutToken(allowed map[string]struct{}) (map[string][]map[string]any, error) {
	// Accounts with available model_availability but no token_model_availability
	// coverage for that model AND no enabled account_tokens at all (or none that
	// list the model). Operators use this for zero-channel route hints.
	rows, err := queryRowsErr(h.db.DB, `
		SELECT
			ma.model_name AS model_name,
			a.id AS account_id,
			COALESCE(a.username, '') AS username,
			s.id AS site_id,
			COALESCE(s.name, '') AS site_name
		FROM model_availability ma
		INNER JOIN accounts a ON a.id = ma.account_id
		INNER JOIN sites s ON s.id = a.site_id
		WHERE COALESCE(ma.available, false) = true
			AND COALESCE(a.status, '') <> 'disabled'
			AND COALESCE(s.status, '') <> 'disabled'
			AND NOT EXISTS (
				SELECT 1
				FROM account_tokens at
				INNER JOIN token_model_availability tma ON tma.token_id = at.id
				WHERE at.account_id = a.id
					AND at.enabled = true
					AND (at.value_status IS NULL OR at.value_status <> 'expired')
					AND COALESCE(tma.available, false) = true
					AND tma.model_name = ma.model_name
			)
			AND NOT EXISTS (
				-- API-key style accounts that store a single key without managed tokens
				-- are still "without token" for route channel binding when no account_tokens rows exist.
				SELECT 1 FROM account_tokens at
				WHERE at.account_id = a.id
					AND at.enabled = true
					AND (at.value_status IS NULL OR at.value_status <> 'expired')
			)
		ORDER BY ma.model_name ASC, a.id ASC
	`)
	if err != nil {
		return nil, err
	}

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
	return out, nil
}

func (h *statsHandler) buildModelsMissingTokenGroups(allowed map[string]struct{}) (map[string][]map[string]any, error) {
	// When an account has model availability and managed tokens, but none of the
	// enabled tokens have a resolvable token_group label, group coverage is uncertain
	// / missing. We do not invent required groups from a remote pricing catalog.
	rows, err := queryRowsErr(h.db.DB, `
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
		WHERE COALESCE(ma.available, false) = true
			AND COALESCE(a.status, '') <> 'disabled'
			AND COALESCE(s.status, '') <> 'disabled'
			AND at.enabled = true
			AND (at.value_status IS NULL OR at.value_status <> 'expired')
		ORDER BY ma.model_name ASC, a.id ASC, at.id ASC
	`)
	if err != nil {
		return nil, err
	}

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
	return out, nil
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

func (h *statsHandler) buildEndpointTypesByModel(allowed map[string]struct{}) (map[string][]string, error) {
	// Union of endpoint types inferred from model names present in availability.
	models := map[string]struct{}{}
	availModels, err := queryRowsErr(h.db.DB, `
		SELECT DISTINCT model_name AS model_name FROM model_availability WHERE COALESCE(available, false) = true
		UNION
		SELECT DISTINCT model_name AS model_name FROM token_model_availability WHERE COALESCE(available, false) = true
	`)
	if err != nil {
		return nil, err
	}
	for _, row := range availModels {
		model := strings.TrimSpace(coerceString(row["modelName"]))
		if model == "" || !modelAllowed(model, allowed) {
			continue
		}
		models[model] = struct{}{}
	}
	out := map[string][]string{}
	catalogSnapshot := appCatalogSnapshot()
	for model := range models {
		types := inferEndpointTypesForModel(catalogSnapshot, model, nil)
		if len(types) == 0 {
			// default OpenAI-compatible when unknown
			types = []string{"openai"}
		}
		out[model] = types
	}
	return out, nil
}

func (h *statsHandler) loadGlobalAllowedModels() map[string]struct{} {
	// Optional whitelist from settings table. Empty / missing → allow all.
	var raw string
	err := h.db.Get(&raw, `SELECT value FROM settings WHERE key = ?`, "global_allowed_models")
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
