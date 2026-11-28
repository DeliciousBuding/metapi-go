package admin

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/app"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/service/pricing"
	"github.com/go-chi/chi/v5"
)

// modelPriceCompare handles:
//
//	GET /api/models/price-compare?model=...&limit=...
//	GET /api/stats/model-prices?model=...&limit=...
//
// Aggregates effective model prices across enabled sites/accounts using
// billing_details rates, observed proxy_logs costs, configured unit_cost, and
// labeled platform fallbacks. Does not invent unlabeled prices.
func (h *statsHandler) modelPriceCompare(w http.ResponseWriter, r *http.Request) {
	modelQ := strings.TrimSpace(r.URL.Query().Get("model"))
	exactModel := modelQ != "" && r.URL.Query().Get("exactModel") == "true"
	limit, _ := parseLimitOffset(r, 50, 200)
	days := config.ClampInt(getQueryInt(r, "days", 30), 1, 365)
	topModels := config.ClampInt(getQueryInt(r, "topModels", 12), 1, 50)

	since := time.Now().UTC().AddDate(0, 0, -(days - 1)).Format(time.RFC3339)

	var models []string
	if modelQ != "" {
		models = []string{modelQ}
	} else {
		var err error
		models, err = h.topModelsForPriceCompare(topModels, since)
		if err != nil {
			writeErrorCode(w, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load top models")
			return
		}
	}

	candidates := make([]routing.PriceCompareRow, 0)
	for _, modelName := range models {
		inputs, err := h.loadPriceCompareInputs(modelName, since, exactModel)
		if err != nil {
			writeErrorCode(w, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load price compare inputs")
			return
		}
		for _, in := range inputs {
			row := routing.BuildPriceCompareRow(in, routing.DefaultPriceCompareSampleUsage)
			candidates = append(candidates, row)
		}
	}

	if modelQ != "" {
		q := strings.ToLower(modelQ)
		filtered := make([]routing.PriceCompareRow, 0, len(candidates))
		for _, row := range candidates {
			rowModel := strings.ToLower(strings.TrimSpace(row.Model))
			if (exactModel && rowModel == q) || (!exactModel && strings.Contains(rowModel, q)) {
				filtered = append(filtered, row)
			}
		}
		candidates = filtered
	}

	sorted := routing.AggregatePriceCompareRows(candidates)
	if len(sorted) > limit {
		sorted = sorted[:limit]
	}
	if sorted == nil {
		sorted = []routing.PriceCompareRow{}
	}
	accounts, current, currentPage, err := h.currentPriceComparison(r, modelQ, exactModel)
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load current pricing")
		return
	}
	baselines := map[string]*pricing.OfficialPrice{}
	manager := app.CatalogManager()
	if manager != nil {
		if currentPage.TotalModels == 0 {
			for _, row := range sorted {
				baselines[row.Model] = pricing.OfficialPriceFor(manager.Snapshot(), row.Model)
			}
		}
		for _, row := range current {
			baselines[row.Model] = row.Official
		}
		if modelQ != "" && currentPage.TotalModels == 0 {
			baselines[modelQ] = pricing.OfficialPriceFor(manager.Snapshot(), modelQ)
		}
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
		"items":           sorted,
		"pricingAccounts": accounts,
		"currentPrices":   current,
		"officialPrices":  baselines,
		"currentPage":     currentPage,
		"meta": map[string]any{
			"count":            len(sorted),
			"modelsConsidered": len(models),
			"sources": []string{
				routing.PriceSourceBilling,
				routing.PriceSourceObserved,
				routing.PriceSourceConfigured,
				routing.PriceSourceFallback,
			},
			"notes": "Rates/costs from billing_details, observed proxy_logs, account unit_cost, or labeled fallback. missingPrice=true means no catalog/observed/configured signal.",
		},
	})
}

func (h *statsHandler) refreshPriceCompareAccount(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "accountId"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid accountId")
		return
	}
	if err := service.RefreshAccountPricing(r.Context(), h.db.DB, id); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (h *statsHandler) topModelsForPriceCompare(limit int, since string) ([]string, error) {
	rows, err := queryRowsErr(h.db, `
		SELECT COALESCE(NULLIF(TRIM(pl.model_actual), ''), NULLIF(TRIM(pl.model_requested), ''), '') AS model,
			COUNT(*) AS calls
		FROM proxy_logs pl
		WHERE pl.created_at >= ?
			AND pl.status = 'success'
			AND COALESCE(NULLIF(TRIM(pl.model_actual), ''), NULLIF(TRIM(pl.model_requested), ''), '') <> ''
		GROUP BY COALESCE(NULLIF(TRIM(pl.model_actual), ''), NULLIF(TRIM(pl.model_requested), ''), '')
		ORDER BY calls DESC
		LIMIT ?
	`, since, limit)
	if err != nil {
		return nil, err
	}

	out := make([]string, 0, len(rows))
	seen := map[string]struct{}{}
	for _, row := range rows {
		m := strings.TrimSpace(coerceString(row["model"]))
		if m == "" {
			continue
		}
		key := strings.ToLower(m)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, m)
	}
	if len(out) > 0 {
		return out, nil
	}

	avail, err := queryRowsErr(h.db, `
		SELECT model_name AS model, COUNT(*) AS cnt
		FROM model_availability
		GROUP BY model_name
		ORDER BY cnt DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	for _, row := range avail {
		m := strings.TrimSpace(coerceString(row["model"]))
		if m == "" {
			continue
		}
		key := strings.ToLower(m)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, m)
	}
	return out, nil
}
