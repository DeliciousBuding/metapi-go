package admin

// Read-only exposure of model_probe_results history for the row-level probe
// health bars on the channels/accounts pages (P0-2). The scheduler
// (scheduler/model_probe.go) is the only writer; these endpoints answer
// "recent N probe results per channel/account" in ONE bounded query per page
// render so the tables never issue per-row requests.

import (
	"fmt"
	"net/http"

	"github.com/jmoiron/sqlx"
)

const (
	probeHistoryDefaultLimit = 20
	probeHistoryMaxLimit     = 50
)

// probeHistoryRow is one model_probe_results row projected for the health
// bar: newest-first within its entity group (probe_rank 1 = latest).
type probeHistoryRow struct {
	ID         int64    `db:"id"`
	EntityID   int64    `db:"entity_id"`
	Status     string   `db:"status"`
	LatencyMs  *float64 `db:"latency_ms"`
	HTTPStatus *int64   `db:"http_status"`
	ErrorText  *string  `db:"error_text"`
	ModelName  string   `db:"model_name"`
	CreatedAt  string   `db:"created_at"`
}

// probeHistoryResult is the camelCase JSON shape of one probe result.
type probeHistoryResult struct {
	// Id is the model_probe_results primary key — a stable identity the
	// frontend uses as the React key for each bar.
	Id         int64    `json:"id"`
	Status     string   `json:"status"`
	LatencyMs  *float64 `json:"latencyMs"`
	HTTPStatus *int64   `json:"httpStatus"`
	ErrorText  *string  `json:"errorText"`
	ModelName  string   `json:"modelName"`
	CreatedAt  string   `json:"createdAt"`
}

// queryProbeHistory returns the most recent `limit` probe results per entity
// (entityColumn = "channel_id" | "account_id") in one window-function query
// that runs on both SQLite and PostgreSQL. Rows are ordered by entity, then
// newest first. Entities without any probe results are simply absent.
func queryProbeHistory(db *sqlx.DB, entityColumn string, limit int) ([]probeHistoryRow, error) {
	switch entityColumn {
	case "channel_id", "account_id":
	default:
		return nil, fmt.Errorf("unsupported probe history entity column: %s", entityColumn)
	}
	query := fmt.Sprintf(`
		SELECT id, entity_id, status, latency_ms, http_status, error_text, model_name, created_at
		FROM (
			SELECT id, %[1]s AS entity_id, status, latency_ms, http_status, error_text, model_name, created_at,
			       ROW_NUMBER() OVER (PARTITION BY %[1]s ORDER BY id DESC) AS probe_rank
			FROM model_probe_results
			WHERE %[1]s IS NOT NULL
		) ranked
		WHERE probe_rank <= ?
		ORDER BY entity_id ASC, probe_rank ASC`, entityColumn)

	var rows []probeHistoryRow
	if err := db.Select(&rows, db.Rebind(query), limit); err != nil {
		return nil, err
	}
	return rows, nil
}

func toProbeHistoryResult(row probeHistoryRow) probeHistoryResult {
	return probeHistoryResult{
		Id:         row.ID,
		Status:     row.Status,
		LatencyMs:  row.LatencyMs,
		HTTPStatus: row.HTTPStatus,
		ErrorText:  row.ErrorText,
		ModelName:  row.ModelName,
		CreatedAt:  row.CreatedAt,
	}
}

// writeProbeHistoryResponse groups flat rows by entity and writes the
// `{limit, items}` envelope. groupKey is the camelCase JSON field carrying
// the entity id ("channelId" | "accountId").
func writeProbeHistoryResponse(w http.ResponseWriter, db *sqlx.DB, entityColumn string, groupKey string, limit int) {
	rows, err := queryProbeHistory(db, entityColumn, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to load probe history")
		return
	}

	// Rows arrive ordered by entity, then newest-first (queryProbeHistory),
	// so a single sequential pass groups them without any lookups.
	items := make([]map[string]any, 0)
	var currentID int64
	var currentResults []probeHistoryResult
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
			currentResults = []probeHistoryResult{}
		}
		currentResults = append(currentResults, toProbeHistoryResult(row))
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
	writeProbeHistoryResponse(w, h.db, "channel_id", "channelId", limit)
}

// accountProbeHistory handles GET /api/accounts/probe-history.
func (h *accountsHandler) accountProbeHistory(w http.ResponseWriter, r *http.Request) {
	limit, _ := parseLimitOffset(r, probeHistoryDefaultLimit, probeHistoryMaxLimit)
	writeProbeHistoryResponse(w, h.db, "account_id", "accountId", limit)
}
