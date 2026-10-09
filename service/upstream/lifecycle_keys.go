package upstream

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/jmoiron/sqlx"
)

type routeKeyChange struct {
	ID     int64
	Before []int64
	After  []int64
}

func loadRouteKeyChanges(ctx context.Context, tx *sqlx.Tx, gone map[int64]bool) ([]routeKeyChange, error) {
	if len(gone) == 0 {
		return nil, nil
	}
	var rows []struct {
		ID     int64          `db:"id"`
		Routes sql.NullString `db:"allowed_route_ids"`
	}
	if err := tx.SelectContext(ctx, &rows, `SELECT id,allowed_route_ids FROM downstream_api_keys WHERE allowed_route_ids IS NOT NULL AND allowed_route_ids<>'' ORDER BY id`); err != nil {
		return nil, err
	}
	var changes []routeKeyChange
	for _, row := range rows {
		var ids []int64
		if json.Unmarshal([]byte(row.Routes.String), &ids) != nil {
			continue
		}
		kept := make([]int64, 0, len(ids))
		removed := false
		for _, id := range ids {
			if gone[id] {
				removed = true
			} else {
				kept = append(kept, id)
			}
		}
		if removed {
			changes = append(changes, routeKeyChange{row.ID, ids, kept})
		}
	}
	return changes, nil
}

func applyRouteKeyChanges(ctx context.Context, tx *sqlx.Tx, changes []routeKeyChange) ([]int64, error) {
	var onlyDeleted []int64
	for _, change := range changes {
		// Empty allowed_route_ids means unrestricted. Retain the dead IDs when
		// pruning the final grant would broaden access to every route.
		if len(change.After) == 0 {
			onlyDeleted = append(onlyDeleted, change.ID)
			continue
		}
		raw, _ := json.Marshal(change.After)
		if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE downstream_api_keys SET allowed_route_ids=? WHERE id=?`), string(raw), change.ID); err != nil {
			return nil, err
		}
	}
	return onlyDeleted, nil
}
