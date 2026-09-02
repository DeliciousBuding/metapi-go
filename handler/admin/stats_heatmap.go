package admin

import (
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/jmoiron/sqlx"
)

func (h *statsHandler) usageHeatmap(w http.ResponseWriter, r *http.Request) {
	days := config.ClampInt(getQueryInt(r, "days", 7), 1, 31)
	dimension := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("dimension")))
	if dimension != "model" {
		dimension = "site"
	}

	since := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour).Truncate(time.Hour).Format(time.RFC3339)
	source := "proxy_logs"
	var cells []map[string]any

	if dimension == "site" {
		// Prefer projected hour aggregates (cheap, already limited by projector).
		rows, err := queryRowsErr(h.db, `
			SELECT
				u.bucket_start_utc AS bucket,
				CAST(u.site_id AS TEXT) AS key,
				COALESCE(s.name, '') AS label,
				COALESCE(u.total_calls, 0) AS calls,
				COALESCE(u.total_tokens, 0) AS tokens,
				COALESCE(u.total_summary_spend, 0) AS spend
			FROM site_hour_usage u
			LEFT JOIN sites s ON s.id = u.site_id
			WHERE u.bucket_start_utc >= ?
			ORDER BY u.bucket_start_utc ASC, u.total_calls DESC
			LIMIT ?
		`, since, usageHeatmapCellLimit)
		if err != nil {
			writeErrorCode(w, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load usage heatmap")
			return
		}
		if len(rows) > 0 {
			source = "site_hour_usage"
			cells = makeUsageHeatmapCells(rows)
		} else {
			// Fallback: bounded live aggregate from proxy_logs when projection is empty.
			hourExpr := hourBucketSQLExpr(h.db, "pl.created_at")
			rows, err = queryRowsErr(h.db, `
				SELECT
					`+hourExpr+` AS bucket,
					CAST(s.id AS TEXT) AS key,
					COALESCE(s.name, '') AS label,
					COUNT(*) AS calls,
					COALESCE(SUM(`+service.EffectiveProxyTokensSQL+`), 0) AS tokens,
					COALESCE(SUM(COALESCE(pl.estimated_cost, 0)), 0) AS spend
				FROM proxy_logs pl
				INNER JOIN accounts a ON a.id = pl.account_id
				INNER JOIN sites s ON s.id = a.site_id
				WHERE pl.created_at >= ?
				GROUP BY `+hourExpr+`, s.id, s.name
				ORDER BY bucket ASC, calls DESC
				LIMIT ?
			`, since, usageHeatmapCellLimit)
			if err != nil {
				writeErrorCode(w, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load usage heatmap")
				return
			}
			cells = makeUsageHeatmapCells(rows)
		}
	} else {
		// Model density: no model_hour_usage table; aggregate proxy_logs with LIMIT.
		hourExpr := hourBucketSQLExpr(h.db, "pl.created_at")
		modelExpr := `COALESCE(NULLIF(pl.model_actual, ''), NULLIF(pl.model_requested, ''), 'unknown')`
		rows, err := queryRowsErr(h.db, `
			SELECT
				`+hourExpr+` AS bucket,
				`+modelExpr+` AS key,
				`+modelExpr+` AS label,
				COUNT(*) AS calls,
				COALESCE(SUM(`+service.EffectiveProxyTokensSQL+`), 0) AS tokens,
				COALESCE(SUM(COALESCE(pl.estimated_cost, 0)), 0) AS spend
			FROM proxy_logs pl
			WHERE pl.created_at >= ?
			GROUP BY `+hourExpr+`, `+modelExpr+`
			ORDER BY bucket ASC, calls DESC
			LIMIT ?
		`, since, usageHeatmapCellLimit)
		if err != nil {
			writeErrorCode(w, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load usage heatmap")
			return
		}
		cells = makeUsageHeatmapCells(rows)
	}

	if cells == nil {
		cells = []map[string]any{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"dimension": dimension,
		"days":      days,
		"since":     since,
		"source":    source,
		"cellLimit": usageHeatmapCellLimit,
		"count":     len(cells),
		"cells":     cells,
	})
}

// ---- Slow Requests ----
// GET /api/stats/slow-requests?limit=50&minLatencyMs=1000&hours=24

// Top proxy_logs by latency_ms within a bounded time window.
// Never returns request/response bodies or chat content.
func dayBucketSQLExpr(db *sqlx.DB, column string) string {
	driver := ""
	if db != nil {
		driver = strings.ToLower(strings.TrimSpace(db.DriverName()))
	}
	switch driver {
	case "pgx", "postgres", "postgresql":
		// created_at is stored as TEXT RFC3339; cast for date_trunc then re-format.
		return `to_char(date_trunc('day', (` + column + `)::timestamptz), 'YYYY-MM-DD')`
	default:
		// SQLite stores UTC RFC3339 without fractional seconds in our writers.
		return `substr(` + column + `, 1, 10)`
	}
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

func round4(v float64) float64 { return math.Round(v*10000) / 10000 }

// hourBucketSQLExpr returns a dual-dialect SQL expression that truncates a
// TEXT RFC3339 timestamp column to an hour bucket string (…T%H:00:00Z).
func hourBucketSQLExpr(db *sqlx.DB, column string) string {
	driver := ""
	if db != nil {
		driver = strings.ToLower(strings.TrimSpace(db.DriverName()))
	}
	switch driver {
	case "pgx", "postgres", "postgresql":
		// created_at is stored as TEXT RFC3339; cast for date_trunc then re-format.
		return `to_char(date_trunc('hour', (` + column + `)::timestamptz), 'YYYY-MM-DD"T"HH24:00:00"Z"')`
	default:
		// SQLite stores UTC RFC3339 without fractional seconds in our writers.
		// substr keeps the query portable and avoids full-table function scans
		// beyond the created_at range predicate + LIMIT.
		return `substr(` + column + `, 1, 13) || ':00:00Z'`
	}
}

func makeUsageHeatmapCells(rows []map[string]any) []map[string]any {
	cells := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		bucket := coerceString(row["bucket"])
		key := coerceString(row["key"])
		if bucket == "" || key == "" {
			continue
		}
		cells = append(cells, map[string]any{
			"bucket": bucket,
			"key":    key,
			"label":  coerceString(row["label"]),
			"calls":  coerceInt64(row["calls"]),
			"tokens": coerceInt64(row["tokens"]),
			"spend":  roundMicro(coerceFloat(row["spend"])),
		})
	}
	return cells
}

// ---- Model Marketplace ----
// GET /api/models/marketplace?refresh=&includePricing=

// Builds the operator marketplace view from local DB availability / routes.
// Does not scrape remote marketplace/pricing catalogs. When includePricing=1,
// pricingSources remain empty and meta.pricingStatus labels the gap
// (
