package admin

import (
	"strings"
)

// effectiveModelSQL is the canonical model name for a proxy log: the upstream
// actual model when known, else the requested model. Kept in one place so the
// top-model and observed-price queries aggregate the same expression.
const effectiveModelSQL = `COALESCE(NULLIF(TRIM(pl.model_actual), ''), NULLIF(TRIM(pl.model_requested), ''), '')`

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
