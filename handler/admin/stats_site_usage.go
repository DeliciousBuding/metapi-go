package admin

import (
	"net/http"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
)

// ---- Site Distribution ----
// GET /api/stats/site-distribution?days=&refresh=
func (h *statsHandler) siteDistribution(w http.ResponseWriter, r *http.Request) {
	days := config.ClampInt(getQueryInt(r, "days", 7), 1, 365)
	fromDay := time.Now().UTC().AddDate(0, 0, -(days - 1)).Format("2006-01-02")

	// Prefer projected site_day_usage spend; include live account balances.
	// total_balance keeps SUM(balance) result ORIGINAL (NULL when no account
	// balance is known) instead of COALESCE-ing to 0, so the response can
	// distinguish "balance unknown" (accounts exist, all NULL) from "no
	// accounts at all" (handled in the loop below).
	rows, err := queryRowsErr(h.db.DB, `
		SELECT
			s.id AS site_id,
			s.name AS site_name,
			s.platform AS platform,
			bal.total_balance AS total_balance,
			COALESCE(usage.total_spend, 0) AS total_spend,
			COALESCE(bal.account_count, 0) AS account_count
		FROM sites s
		LEFT JOIN (
			SELECT site_id, SUM(balance) AS total_balance, COUNT(*) AS account_count
			FROM accounts
			GROUP BY site_id
		) bal ON bal.site_id = s.id
		LEFT JOIN (
			SELECT site_id, COALESCE(SUM(COALESCE(total_summary_spend, 0)), 0) AS total_spend
			FROM site_day_usage
			WHERE local_day >= ?
			GROUP BY site_id
		) usage ON usage.site_id = s.id
		ORDER BY total_spend DESC, s.name ASC
	`, fromDay)
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load site distribution")
		return
	}

	distribution := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		// NULL total_balance with accounts present means the site's balance
		// is unknown, not zero — surface null so clients do not misreport
		// $0.00. A site without any account is a genuine zero.
		var totalBalance any
		if row["totalBalance"] == nil && coerceInt(row["accountCount"]) > 0 {
			totalBalance = nil
		} else {
			totalBalance = coerceFloat(row["totalBalance"])
		}
		distribution = append(distribution, map[string]any{
			"siteId":       row["siteId"],
			"siteName":     row["siteName"],
			"platform":     row["platform"],
			"totalBalance": totalBalance,
			"totalSpend":   coerceFloat(row["totalSpend"]),
			"accountCount": coerceInt(row["accountCount"]),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"distribution": distribution})
}

// ---- Site Trend ----
// GET /api/stats/site-trend?days=&refresh=
func (h *statsHandler) siteTrend(w http.ResponseWriter, r *http.Request) {
	days := config.ClampInt(getQueryInt(r, "days", 7), 1, 365)
	fromDay := time.Now().UTC().AddDate(0, 0, -(days - 1)).Format("2006-01-02")

	rows, err := queryRowsErr(h.db.DB, `
		SELECT
			u.local_day AS local_day,
			s.name AS site_name,
			COALESCE(SUM(u.total_summary_spend), 0) AS spend,
			COALESCE(SUM(u.total_calls), 0) AS calls
		FROM site_day_usage u
		INNER JOIN sites s ON s.id = u.site_id
		WHERE u.local_day >= ?
		GROUP BY u.local_day, s.name
		ORDER BY u.local_day ASC, s.name ASC
	`, fromDay)
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load site trend")
		return
	}

	// Shape: [{ date, sites: { [siteName]: { spend, calls } } }]
	byDate := make(map[string]map[string]map[string]any)
	order := make([]string, 0)
	for _, row := range rows {
		day := coerceString(row["localDay"])
		siteName := coerceString(row["siteName"])
		if day == "" || siteName == "" {
			continue
		}
		if _, ok := byDate[day]; !ok {
			byDate[day] = make(map[string]map[string]any)
			order = append(order, day)
		}
		byDate[day][siteName] = map[string]any{
			"spend": coerceFloat(row["spend"]),
			"calls": coerceInt(row["calls"]),
		}
	}

	trend := make([]map[string]any, 0, len(order))
	for _, day := range order {
		trend = append(trend, map[string]any{
			"date":  day,
			"sites": byDate[day],
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"trend": trend})
}

// GET /api/stats/model-by-site?days=&siteId=
func (h *statsHandler) modelBySite(w http.ResponseWriter, r *http.Request) {
	days := config.ClampInt(getQueryInt(r, "days", 7), 1, 365)
	siteID := getQueryInt(r, "siteId", 0)
	fromDay := time.Now().UTC().AddDate(0, 0, -(days - 1)).Format("2006-01-02")

	query := `SELECT model,
		COALESCE(SUM(total_calls), 0) AS calls,
		COALESCE(SUM(total_spend), 0) AS spend,
		COALESCE(SUM(total_tokens), 0) AS tokens
		FROM model_day_usage
		WHERE local_day >= ?`
	args := []any{fromDay}
	if siteID > 0 {
		query += " AND site_id = ?"
		args = append(args, siteID)
	}
	query += " GROUP BY model ORDER BY calls DESC"

	rows, err := queryRowsErr(h.db.DB, query, args...)
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load model-by-site")
		return
	}
	models := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		models = append(models, map[string]any{
			"model":  coerceString(row["model"]),
			"calls":  coerceInt(row["calls"]),
			"spend":  coerceFloat(row["spend"]),
			"tokens": coerceInt64(row["tokens"]),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": models})
}
