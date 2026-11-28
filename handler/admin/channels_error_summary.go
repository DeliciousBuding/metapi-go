package admin

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/deliciousbuding/metapi-go/routing"
)

type channelSummaryRow struct {
	SiteID            int64   `db:"site_id"`
	SourceModel       *string `db:"source_model"`
	RouteModelPattern string  `db:"route_model_pattern"`
	Enabled           bool    `db:"enabled"`
	SiteStatus        string  `db:"site_status"`
	AccountStatus     string  `db:"account_status"`
	CooldownUntil     *string `db:"cooldown_until"`
}

// channelErrorSummary handles GET /api/channels/error-summary. The summary is
// read-only and deliberately separate from the list page: it computes the
// fleet-wide runtime status counts that cannot be derived from a SQL aggregate
// because breaker state lives in the routing in-memory health maps.
func (h *tokenRoutesHandler) channelErrorSummary(w http.ResponseWriter, r *http.Request) {
	if parseTruthyQuery(r.URL.Query().Get("refresh")) {
		globalChannelsErrorSummaryCache.clear()
	}
	data, cacheHit, err := globalChannelsErrorSummaryCache.getOrCompute("summary", func() ([]byte, error) {
		return h.computeChannelErrorSummary(r)
	})
	if err != nil {
		slog.Error("channels error summary failed", "error", err)
		writeErrorCodeWithRequest(w, r, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load channel error summary")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if cacheHit {
		w.Header().Set("x-channels-error-summary-cache", "hit")
	} else {
		w.Header().Set("x-channels-error-summary-cache", "miss")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (h *tokenRoutesHandler) computeChannelErrorSummary(r *http.Request) ([]byte, error) {
	var rows []channelSummaryRow
	err := h.db.SelectContext(r.Context(), &rows, `
		SELECT a.site_id, rc.source_model,
		       COALESCE(tr.model_pattern, '') AS route_model_pattern,
		       rc.enabled, rc.cooldown_until,
		       COALESCE(s.status, '') AS site_status,
		       COALESCE(a.status, '') AS account_status
		FROM route_channels rc
		LEFT JOIN accounts a ON rc.account_id = a.id
		LEFT JOIN sites s ON a.site_id = s.id
		LEFT JOIN token_routes tr ON rc.route_id = tr.id`)
	if err != nil {
		return nil, err
	}

	byStatus := map[string]int64{
		routing.ChannelStatusEnabled:          0,
		routing.ChannelStatusCooldown:         0,
		routing.ChannelStatusBreakerOpen:      0,
		routing.ChannelStatusManuallyDisabled: 0,
	}
	var total int64
	var errorCount int64
	for _, row := range rows {
		modelName := ""
		if row.SourceModel != nil {
			modelName = *row.SourceModel
		}
		status := routing.ChannelRuntimeStatus(row.SiteID, modelName, channelEffectivelyEnabled(row.Enabled, row.SiteStatus, row.AccountStatus), row.CooldownUntil)
		total++
		byStatus[status]++
		if status == routing.ChannelStatusCooldown || status == routing.ChannelStatusBreakerOpen {
			errorCount++
		}
	}
	return json.Marshal(map[string]any{
		"total":      total,
		"errorCount": errorCount,
		"byStatus":   byStatus,
	})
}
