package upstream

import (
	"context"

	"github.com/jmoiron/sqlx"
)

// LockLifecycleTx excludes graph writers, including raw SQL and FK cascades,
// while confirming and deleting the closure. PostgreSQL ordinary reads remain
// available. SQLite takes its write reservation before any snapshot reads.
func LockLifecycleTx(ctx context.Context, tx *sqlx.Tx) error {
	query := `UPDATE upstream_channels SET id=id WHERE 1=0`
	if tx.DriverName() == "pgx" || tx.DriverName() == "postgres" {
		query = `LOCK TABLE upstream_channels, upstream_models, upstream_credentials, upstream_grants, upstream_groups, upstream_group_items, upstream_route_groups, token_routes, route_channels, route_group_sources, downstream_api_keys, external_source_ids IN SHARE ROW EXCLUSIVE MODE`
	}
	_, err := tx.ExecContext(ctx, query)
	return err
}
