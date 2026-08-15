package admin

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"

	"github.com/deliciousbuding/metapi-go/routing"
)

// ---- Model Rate Overview ----

// Read-only aggregation of every multiplier/rate surface.
// Batch update of accounts.unit_cost + route_channels.weight — pure
// config writes; unit_cost stays a display/planning field (estimated_cost is
// ratio-based, never account-priced; N9b-b explicitly closed).
//

// RegisterModelRatesRoutes mounts the rate overview endpoint.
func RegisterModelRatesRoutes(r chi.Router, db *sqlx.DB) {
	h := &modelRatesHandler{db: db}
	r.Get("/api/models/rates", h.rates)
	r.Put("/api/models/rates", h.updateRates)
}

type modelRatesHandler struct {
	db *sqlx.DB
}

// PUT /api/models/rates — batch update body:
// {"accounts": [{"id": 1, "unitCost": 0.003}], "channels": [{"id": 5, "weight": 20}]}
// unitCost/weight must be >= 0. Missing arrays are no-ops.
func (h *modelRatesHandler) updateRates(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Accounts []struct {
			ID       int64    `json:"id"`
			UnitCost *float64 `json:"unitCost"`
		} `json:"accounts"`
		Channels []struct {
			ID     int64    `json:"id"`
			Weight *float64 `json:"weight"`
		} `json:"channels"`
	}
	if err := decodeJSONRequest(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid JSON body"})
		return
	}
	if len(body.Accounts) == 0 && len(body.Channels) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "nothing to update"})
		return
	}

	updatedAccounts := 0
	for _, acc := range body.Accounts {
		if acc.ID <= 0 || acc.UnitCost == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "account entries need id and unitCost"})
			return
		}
		if *acc.UnitCost < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "unitCost must be >= 0"})
			return
		}
		res, err := h.db.Exec(rebindAdminQuery(h.db, "UPDATE accounts SET unit_cost = ? WHERE id = ?"), *acc.UnitCost, acc.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to update account unit cost"})
			return
		}
		if n, _ := res.RowsAffected(); n > 0 {
			updatedAccounts++
		}
	}

	updatedChannels := 0
	for _, ch := range body.Channels {
		if ch.ID <= 0 || ch.Weight == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "channel entries need id and weight"})
			return
		}
		if *ch.Weight < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "weight must be >= 0"})
			return
		}
		res, err := h.db.Exec(rebindAdminQuery(h.db, "UPDATE route_channels SET weight = ? WHERE id = ?"), *ch.Weight, ch.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to update channel weight"})
			return
		}
		if n, _ := res.RowsAffected(); n > 0 {
			updatedChannels++
		}
	}

	// Weights feed routing cache — invalidate so changes apply immediately.
	routing.InvalidateCache()

	writeJSON(w, http.StatusOK, map[string]any{
		"success":         true,
		"updatedAccounts": updatedAccounts,
		"updatedChannels": updatedChannels,
	})
}

// GET /api/models/rates
func (h *modelRatesHandler) rates(w http.ResponseWriter, r *http.Request) {
	// Accounts with their unit cost (nullable) and channel footprint.
	accRows, err := queryRowsErr(h.db, `
		SELECT a.id AS account_id, COALESCE(a.username, '') AS username,
			s.id AS site_id, COALESCE(s.name, '') AS site_name,
			a.unit_cost AS unit_cost,
			COUNT(rc.id) AS channel_count,
			COALESCE(SUM(CASE WHEN rc.enabled = TRUE THEN rc.weight ELSE 0 END), 0) AS total_weight
		FROM accounts a
		LEFT JOIN sites s ON s.id = a.site_id
		LEFT JOIN route_channels rc ON rc.account_id = a.id
		GROUP BY a.id, a.username, s.id, s.name, a.unit_cost
		ORDER BY COALESCE(a.unit_cost, 0) DESC, a.id ASC
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load rate overview")
		return
	}
	accounts := make([]map[string]any, 0, len(accRows))
	for _, row := range accRows {
		accounts = append(accounts, map[string]any{
			"accountId":    coerceInt64(row["accountId"]),
			"username":     coerceString(row["username"]),
			"siteId":       row["siteId"],
			"siteName":     coerceString(row["siteName"]),
			"unitCost":     row["unitCost"],
			"channelCount": coerceInt(row["channelCount"]),
			"totalWeight":  coerceFloat(row["totalWeight"]),
		})
	}

	// Channels with their weight (the effective routing multiplier).
	chRows, err := queryRowsErr(h.db, `
		SELECT rc.id AS channel_id, rc.route_id, rc.account_id, rc.weight, rc.enabled,
			COALESCE(rc.source_model, '') AS model_name,
			COALESCE(tr.model_pattern, '') AS route_pattern,
			COALESCE(a.username, '') AS username
		FROM route_channels rc
		LEFT JOIN token_routes tr ON tr.id = rc.route_id
		LEFT JOIN accounts a ON a.id = rc.account_id
		ORDER BY rc.weight DESC, rc.id ASC
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load rate overview")
		return
	}
	channels := make([]map[string]any, 0, len(chRows))
	for _, row := range chRows {
		channels = append(channels, map[string]any{
			"channelId":    coerceInt64(row["channelId"]),
			"routeId":      row["routeId"],
			"routePattern": coerceString(row["routePattern"]),
			"accountId":    row["accountId"],
			"username":     coerceString(row["username"]),
			"modelName":    coerceString(row["modelName"]),
			"weight":       coerceFloat(row["weight"]),
			"enabled":      coerceBool(row["enabled"]),
		})
	}

	// Sites with global weight.
	siteRows, err := queryRowsErr(h.db, `
		SELECT id AS site_id, COALESCE(name, '') AS site_name, global_weight
		FROM sites ORDER BY global_weight DESC, id ASC
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load rate overview")
		return
	}
	sites := make([]map[string]any, 0, len(siteRows))
	for _, row := range siteRows {
		sites = append(sites, map[string]any{
			"siteId":       coerceInt64(row["siteId"]),
			"siteName":     coerceString(row["siteName"]),
			"globalWeight": coerceFloat(row["globalWeight"]),
		})
	}

	// Downstream keys with weight (the per-key multiplier).
	keyRows, err := queryRowsErr(h.db, `
		SELECT id AS key_id, COALESCE(name, '') AS name, key_weight
		FROM downstream_api_keys
		ORDER BY COALESCE(key_weight, 1.0) DESC, id ASC
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load rate overview")
		return
	}
	keys := make([]map[string]any, 0, len(keyRows))
	for _, row := range keyRows {
		keys = append(keys, map[string]any{
			"keyId":     coerceInt64(row["keyId"]),
			"name":      coerceString(row["name"]),
			"keyWeight": row["keyWeight"],
		})
	}

	// Observed model costs (30d) — the effective price evidence.
	modelRows, err := queryRowsErr(h.db, `
		SELECT model,
			COALESCE(SUM(total_calls), 0) AS calls,
			COALESCE(SUM(total_spend), 0) AS spend,
			COALESCE(SUM(total_tokens), 0) AS tokens
		FROM model_day_usage
		WHERE local_day >= ?
		GROUP BY model
		ORDER BY spend DESC, model ASC
	`, time.Now().UTC().AddDate(0, 0, -29).Format("2006-01-02"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load rate overview")
		return
	}
	models := make([]map[string]any, 0, len(modelRows))
	for _, row := range modelRows {
		models = append(models, map[string]any{
			"model":  coerceString(row["model"]),
			"calls":  coerceInt(row["calls"]),
			"spend":  roundMicro(coerceFloat(row["spend"])),
			"tokens": coerceInt64(row["tokens"]),
		})
	}

	// Summary: total channel weight, unit-cost coverage.
	summary := map[string]any{
		"accountsWithUnitCost": 0,
		"accountsTotal":        len(accounts),
		"channelsTotal":        len(channels),
		"channelsEnabled":      0,
	}
	for _, acc := range accounts {
		if acc["unitCost"] != nil {
			summary["accountsWithUnitCost"] = summary["accountsWithUnitCost"].(int) + 1
		}
	}
	for _, ch := range channels {
		if ch["enabled"] == true {
			summary["channelsEnabled"] = summary["channelsEnabled"].(int) + 1
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"generatedAt": time.Now().UTC().Format(time.RFC3339),
		"summary":     summary,
		"accounts":    accounts,
		"channels":    channels,
		"sites":       sites,
		"keys":        keys,
		"models":      models,
	})
}
