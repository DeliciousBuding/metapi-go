package store

import (
	"fmt"

	"github.com/jmoiron/sqlx"
)

// ProbeHistoryRow is one model_probe_results row projected for the health
// bar: newest-first within its entity group (probe_rank 1 = latest).
type ProbeHistoryRow struct {
	ID         int64    `db:"id" json:"id"`
	EntityID   int64    `db:"entity_id" json:"-"`
	Status     string   `db:"status" json:"status"`
	LatencyMs  *float64 `db:"latency_ms" json:"latencyMs"`
	HTTPStatus *int64   `db:"http_status" json:"httpStatus"`
	ErrorText  *string  `db:"error_text" json:"errorText"`
	ModelName  string   `db:"model_name" json:"modelName"`
	CreatedAt  string   `db:"created_at" json:"createdAt"`
}

// RecentProbeHistory returns the most recent `limit` probe results per entity
// (entityColumn = "channel_id" | "account_id") in one window-function query
// that runs on both SQLite and PostgreSQL. Rows are ordered by entity, then
// newest first. Entities without any probe results are simply absent.
func RecentProbeHistory(db *sqlx.DB, entityColumn string, limit int) ([]ProbeHistoryRow, error) {
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

	var rows []ProbeHistoryRow
	if err := db.Select(&rows, db.Rebind(query), limit); err != nil {
		return nil, err
	}
	return rows, nil
}
