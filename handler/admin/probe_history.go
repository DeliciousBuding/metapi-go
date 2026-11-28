package admin

// Read-only exposure of model_probe_results history for the row-level probe
// health bars on the channels/accounts pages. The scheduler
// (scheduler/model_probe.go) is the only writer; these endpoints answer
// "recent N probe results per channel/account" in ONE bounded query per page
// render so the tables never issue per-row requests.

import (
	"net/http"

	"github.com/deliciousbuding/metapi-go/store"
)

const (
	probeHistoryDefaultLimit = 20
	probeHistoryMaxLimit     = 50
)

// writeProbeHistoryResponse groups flat rows by entity and writes the
// `{limit, items}` envelope. groupKey is the camelCase JSON field carrying
// the entity id ("channelId" | "accountId").
func writeProbeHistoryResponse(w http.ResponseWriter, groupKey string, limit int, rows []store.ProbeHistoryRow, err error) {
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "Failed to load probe history")
		return
	}

	// Rows arrive ordered by entity, then newest-first (RecentProbeHistory),
	// so a single sequential pass groups them without any lookups.
	items := make([]map[string]any, 0)
	var currentID int64
	var currentResults []store.ProbeHistoryRow
	flush := func() {
		if len(currentResults) > 0 {
			items = append(items, map[string]any{
				groupKey:  currentID,
				"results": currentResults,
			})
		}
	}
	for _, row := range rows {
		if row.EntityID != currentID || currentResults == nil {
			flush()
			currentID = row.EntityID
			currentResults = []store.ProbeHistoryRow{}
		}
		currentResults = append(currentResults, row)
	}
	flush()

	writeJSON(w, http.StatusOK, map[string]any{
		"limit": limit,
		"items": items,
	})
}

// channelProbeHistory handles GET /api/channels/probe-history.
// Registered on tokenRoutesHandler because channels belong to the routing
// domain; auth is the shared admin Bearer middleware on the router.
func (h *tokenRoutesHandler) channelProbeHistory(w http.ResponseWriter, r *http.Request) {
	limit, _ := parseLimitOffset(r, probeHistoryDefaultLimit, probeHistoryMaxLimit)
	rows, err := store.RecentProbeHistory(h.db, "channel_id", limit)
	writeProbeHistoryResponse(w, "channelId", limit, rows, err)
}

// accountProbeHistory handles GET /api/accounts/probe-history.
func (h *accountsHandler) accountProbeHistory(w http.ResponseWriter, r *http.Request) {
	limit, _ := parseLimitOffset(r, probeHistoryDefaultLimit, probeHistoryMaxLimit)
	rows, err := store.RecentProbeHistory(h.db.DB, "account_id", limit)
	writeProbeHistoryResponse(w, "accountId", limit, rows, err)
}
