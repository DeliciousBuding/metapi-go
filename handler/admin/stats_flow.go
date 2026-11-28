package admin

import (
	"net/http"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/service"
)

func (h *statsHandler) flow(w http.ResponseWriter, r *http.Request) {
	win, ok := readStatsWindow(w, r)
	if !ok {
		return
	}
	topN := config.ClampInt(getQueryInt(r, "topN", 20), 1, 50)
	condition, args := win.attemptFilter()
	// Compute totals before limiting edges, in the same database snapshot.
	rows, err := queryRowsErr(h.db, `WITH grouped AS (
  SELECT pl.downstream_api_key_id AS downstream_key_id,
   COALESCE(NULLIF(pl.model_actual,''),NULLIF(pl.model_requested,''),'unknown') AS model,
   a.site_id, pl.channel_id, COUNT(*) AS requests,
   COUNT(*) - COUNT(CASE WHEN pl.status='success' THEN 1 END) AS failed_count,
   SUM(`+service.EffectiveProxyTokensSQL+`) AS total_tokens,
   COALESCE(SUM(pl.estimated_cost),0) AS total_cost,
   AVG(CASE WHEN pl.latency_ms>0 THEN CAST(pl.latency_ms AS DOUBLE PRECISION) END) AS average_latency_ms
  FROM proxy_logs pl LEFT JOIN accounts a ON a.id=pl.account_id
  WHERE `+condition+`
  GROUP BY pl.downstream_api_key_id, model, a.site_id, pl.channel_id
 ), ranked AS (
  SELECT grouped.*, CAST(SUM(requests) OVER () AS BIGINT) AS window_requests,
   CAST(SUM(failed_count) OVER () AS BIGINT) AS window_failed,
   CAST(SUM(total_tokens) OVER () AS BIGINT) AS window_tokens,
   SUM(total_cost) OVER () AS window_cost FROM grouped
 )
 SELECT ranked.*, k.name AS downstream_key_name, s.name AS site_name
 FROM ranked LEFT JOIN downstream_api_keys k ON k.id=ranked.downstream_key_id
 LEFT JOIN sites s ON s.id=ranked.site_id
 ORDER BY requests DESC, downstream_key_id ASC NULLS LAST, model, ranked.site_id ASC NULLS LAST, channel_id ASC NULLS LAST
 LIMIT ?`, append(args, topN)...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load request flow")
		return
	}
	var requests, failed, tokens int64
	var cost float64
	if len(rows) > 0 {
		requests = coerceInt64(rows[0]["windowRequests"])
		failed = coerceInt64(rows[0]["windowFailed"])
		tokens = coerceInt64(rows[0]["windowTokens"])
		cost = coerceFloat(rows[0]["windowCost"])
	}
	for _, row := range rows {
		delete(row, "windowRequests")
		delete(row, "windowFailed")
		delete(row, "windowTokens")
		delete(row, "windowCost")
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"period": win.period, "window": win.bounds(),
		"totalRequests": requests, "totalFailed": failed,
		"totalTokens": tokens, "totalCost": cost,
		"edges": normalizeSlice(rows),
	})
}
