package service

import (
	"context"
	"fmt"

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/jmoiron/sqlx"
)

// PopulateRouteChannelsByModelPattern is the create-path helper: insert automatic
// channels for a newly created pattern route without wiping existing rows.
func PopulateRouteChannelsByModelPattern(ctx context.Context, db *sqlx.DB, routeID int64, modelPattern string) (inserted int, err error) {
	if db == nil {
		return 0, fmt.Errorf("populate channels: db is nil")
	}
	routeRebuildMu.Lock()
	defer routeRebuildMu.Unlock()

	desired, err := collectDesiredChannels(ctx, db, routeID, modelPattern)
	if err != nil {
		return 0, err
	}
	existing, err := loadRouteChannelKeys(ctx, db, routeID)
	if err != nil {
		return 0, err
	}
	for key, cand := range desired {
		if _, ok := existing[key]; ok {
			continue
		}
		if err := insertAutoChannel(ctx, db, routeID, cand); err != nil {
			return inserted, err
		}
		inserted++
	}
	routing.InvalidateCache()
	return inserted, nil
}

func rebuildAutomaticRouteChannelsByModelPattern(ctx context.Context, db *sqlx.DB, routeID int64, modelPattern string, opts RebuildOptions) (inserted, removed, kept int, err error) {
	desired, err := collectDesiredChannels(ctx, db, routeID, modelPattern)
	if err != nil {
		return 0, 0, 0, err
	}
	if err := applyProbeFilter(ctx, db, desired, opts); err != nil {
		return 0, 0, 0, err
	}

	type existingRow struct {
		ID             int64   `db:"id"`
		AccountID      int64   `db:"account_id"`
		TokenID        *int64  `db:"token_id"`
		SourceModel    *string `db:"source_model"`
		ManualOverride bool    `db:"manual_override"`
	}
	var rows []existingRow
	if err := db.SelectContext(ctx, &rows,
		db.Rebind(`SELECT id, account_id, token_id, source_model, manual_override
		 FROM route_channels WHERE route_id = ?`), routeID); err != nil {
		return 0, 0, 0, fmt.Errorf("load route_channels: %w", err)
	}

	// No-change short-circuit: when the automatic channel key set already equals
	// the desired key set, skip every write. The caller uses the zero-diff result
	// to skip cache invalidation on the probe-filter path.
	autoExisting := make(map[channelIdentity]struct{}, len(rows))
	manualCount := 0
	for _, row := range rows {
		if row.ManualOverride {
			manualCount++
			continue
		}
		autoExisting[channelKey(row.AccountID, row.TokenID, row.SourceModel)] = struct{}{}
	}
	if sameChannelKeySet(desired, autoExisting) {
		return 0, 0, manualCount + len(autoExisting), nil
	}

	present := make(map[channelIdentity]struct{}, len(rows))
	for _, row := range rows {
		key := channelKey(row.AccountID, row.TokenID, row.SourceModel)
		if row.ManualOverride {
			// Manual attachments are never deleted by rebuild.
			present[key] = struct{}{}
			kept++
			continue
		}
		if _, ok := desired[key]; ok {
			present[key] = struct{}{}
			kept++
			continue
		}
		if _, err := db.ExecContext(ctx, db.Rebind(`DELETE FROM route_channels WHERE id = ?`), row.ID); err != nil {
			return inserted, removed, kept, fmt.Errorf("delete stale channel %d: %w", row.ID, err)
		}
		removed++
	}

	for key, cand := range desired {
		if _, ok := present[key]; ok {
			continue
		}
		if err := insertAutoChannel(ctx, db, routeID, cand); err != nil {
			return inserted, removed, kept, err
		}
		inserted++
	}
	return inserted, removed, kept, nil
}

func loadRouteChannelKeys(ctx context.Context, db *sqlx.DB, routeID int64) (map[channelIdentity]struct{}, error) {
	type row struct {
		AccountID   int64   `db:"account_id"`
		TokenID     *int64  `db:"token_id"`
		SourceModel *string `db:"source_model"`
	}
	var rows []row
	if err := db.SelectContext(ctx, &rows,
		db.Rebind(`SELECT account_id, token_id, source_model FROM route_channels WHERE route_id = ?`), routeID); err != nil {
		return nil, err
	}
	out := make(map[channelIdentity]struct{}, len(rows))
	for _, r := range rows {
		out[channelKey(r.AccountID, r.TokenID, r.SourceModel)] = struct{}{}
	}
	return out, nil
}

func insertAutoChannel(ctx context.Context, db *sqlx.DB, routeID int64, cand desiredChannel) error {
	_, err := db.ExecContext(ctx,
		db.Rebind(`INSERT INTO route_channels
			(route_id, account_id, token_id, source_model, priority, weight, enabled, manual_override)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		routeID, cand.AccountID, cand.TokenID, cand.SourceModel,
		cand.Priority, cand.Weight, cand.Enabled, false,
	)
	if err != nil {
		return fmt.Errorf("insert auto channel route=%d account=%d: %w", routeID, cand.AccountID, err)
	}
	return nil
}

func sameChannelKeySet(desired map[channelIdentity]desiredChannel, existing map[channelIdentity]struct{}) bool {
	if len(desired) != len(existing) {
		return false
	}
	for k := range desired {
		if _, ok := existing[k]; !ok {
			return false
		}
	}
	return true
}
