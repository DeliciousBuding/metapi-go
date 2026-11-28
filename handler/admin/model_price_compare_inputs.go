package admin

import (
	"fmt"
	"math"
	"strings"

	"github.com/deliciousbuding/metapi-go/routing"
)

type observedPriceSignal struct {
	AvgCost        float64
	Samples        int
	BillingDetails string
	ResolvedModel  string
}

func (h *statsHandler) loadPriceCompareInputs(modelName string, since string, exactModel bool) ([]routing.PriceCompareInput, error) {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return nil, nil
	}
	modelOperator := "LIKE"
	modelArgument := "%" + strings.ToLower(modelName) + "%"
	if exactModel {
		modelOperator = "="
		modelArgument = strings.ToLower(modelName)
	}

	obsByAccount, err := h.observedPricesByAccount(since, modelOperator, modelArgument)
	if err != nil {
		return nil, err
	}
	availModelByAccount, err := h.availablePriceModelsByAccount(modelOperator, modelArgument)
	if err != nil {
		return nil, err
	}

	// Active accounts on non-disabled sites.
	accountRows, err := queryRowsErr(h.db, `
		SELECT
			s.id AS site_id,
			s.name AS site_name,
			s.platform AS platform,
			a.id AS account_id,
			COALESCE(a.username, '') AS username,
			a.unit_cost AS unit_cost,
			COALESCE(a.status, '') AS account_status,
			COALESCE(s.status, '') AS site_status
		FROM accounts a
		INNER JOIN sites s ON s.id = a.site_id
		WHERE COALESCE(a.status, '') <> 'disabled'
			AND COALESCE(s.status, '') <> 'disabled'
		ORDER BY s.name ASC, a.id ASC
	`)
	if err != nil {
		return nil, err
	}

	out := make([]routing.PriceCompareInput, 0)
	for _, row := range accountRows {
		accID := coerceInt64(row["accountId"])
		if accID <= 0 {
			continue
		}
		obs, hasObs := obsByAccount[accID]
		availModel, hasAvail := availModelByAccount[accID]
		unitCost := coerceFloat(row["unitCost"])
		hasUnit := unitCost > 0 && !math.IsNaN(unitCost)

		// Include account if it has availability, observed traffic, or configured unit cost.
		if !hasObs && !hasAvail && !hasUnit {
			continue
		}

		resolvedModel := modelName
		if hasAvail && availModel != "" {
			resolvedModel = availModel
		} else if hasObs && obs.ResolvedModel != "" {
			resolvedModel = obs.ResolvedModel
		}

		in := routing.PriceCompareInput{
			SiteID:    coerceInt64(row["siteId"]),
			SiteName:  coerceString(row["siteName"]),
			Platform:  coerceString(row["platform"]),
			Model:     resolvedModel,
			AccountID: accID,
			Username:  coerceString(row["username"]),
		}
		if hasUnit {
			v := unitCost
			in.ConfiguredUnitCost = &v
		}
		if hasObs {
			in.ObservedSamples = obs.Samples
			if obs.AvgCost > 0 {
				v := obs.AvgCost
				in.ObservedAvgCost = &v
			}
			if strings.TrimSpace(obs.BillingDetails) != "" {
				applyBillingDetailsToPriceInput(&in, obs.BillingDetails)
			}
		}
		out = append(out, in)
	}

	// Empty-state: when no signals exist for this model, still return active
	// accounts as labeled fallback rows so operators see missing-price sites.
	if len(out) == 0 {
		for _, row := range accountRows {
			// Prefer active accounts for empty/missing UI.
			if coerceString(row["accountStatus"]) != "active" {
				continue
			}
			out = append(out, routing.PriceCompareInput{
				SiteID:    coerceInt64(row["siteId"]),
				SiteName:  coerceString(row["siteName"]),
				Platform:  coerceString(row["platform"]),
				Model:     modelName,
				AccountID: coerceInt64(row["accountId"]),
				Username:  coerceString(row["username"]),
			})
			if len(out) >= 50 {
				break
			}
		}
	}
	return out, nil
}

func (h *statsHandler) observedPricesByAccount(since, modelOperator, modelArgument string) (map[int64]observedPriceSignal, error) {
	// Observed success costs + latest-ish billing_details for matching models.
	observedQuery := fmt.Sprintf(`
		SELECT
			pl.account_id AS account_id,
			COALESCE(NULLIF(TRIM(pl.model_actual), ''), NULLIF(TRIM(pl.model_requested), ''), '') AS model_name,
			AVG(COALESCE(pl.estimated_cost, 0)) AS avg_cost,
			COUNT(*) AS samples,
			MAX(COALESCE(pl.billing_details, '')) AS billing_details
		FROM proxy_logs pl
		WHERE pl.created_at >= ?
			AND pl.status = 'success'
			AND COALESCE(pl.estimated_cost, 0) > 0
			AND LOWER(COALESCE(NULLIF(TRIM(pl.model_actual), ''), NULLIF(TRIM(pl.model_requested), ''), '')) %s ?
		GROUP BY pl.account_id, COALESCE(NULLIF(TRIM(pl.model_actual), ''), NULLIF(TRIM(pl.model_requested), ''), '')
	`, modelOperator)
	obsRows, err := queryRowsErr(h.db, observedQuery, since, modelArgument)
	if err != nil {
		return nil, err
	}

	obsByAccount := map[int64]observedPriceSignal{}
	for _, row := range obsRows {
		accID := coerceInt64(row["accountId"])
		if accID <= 0 {
			continue
		}
		sig := observedPriceSignal{
			AvgCost:        coerceFloat(row["avgCost"]),
			Samples:        coerceInt(row["samples"]),
			BillingDetails: coerceString(row["billingDetails"]),
			ResolvedModel:  strings.TrimSpace(coerceString(row["modelName"])),
		}
		// Keep the highest-sample signal per account for this model filter.
		if prev, ok := obsByAccount[accID]; ok && prev.Samples >= sig.Samples {
			continue
		}
		obsByAccount[accID] = sig
	}
	return obsByAccount, nil
}

func (h *statsHandler) availablePriceModelsByAccount(modelOperator, modelArgument string) (map[int64]string, error) {
	// Accounts that advertise the model via availability.
	availabilityQuery := fmt.Sprintf(`
		SELECT account_id, model_name
		FROM model_availability
		WHERE LOWER(model_name) %s ?
	`, modelOperator)
	availRows, err := queryRowsErr(h.db, availabilityQuery, modelArgument)
	if err != nil {
		return nil, err
	}
	availModelByAccount := map[int64]string{}
	for _, row := range availRows {
		accID := coerceInt64(row["accountId"])
		if accID <= 0 {
			continue
		}
		if _, ok := availModelByAccount[accID]; ok {
			continue
		}
		availModelByAccount[accID] = strings.TrimSpace(coerceString(row["modelName"]))
	}
	return availModelByAccount, nil
}
