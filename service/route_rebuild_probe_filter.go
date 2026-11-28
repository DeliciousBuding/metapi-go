package service

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// ProbeFilterConfig gates the route-rebuild probe filter (#625).
// IncludeModels/ExcludeModels are optional exact source-model names; empty means
// no-op. When Enabled, desired channels whose latest connectivity probe failed
// are dropped before any route_channels diff is written.
type ProbeFilterConfig struct {
	Enabled       bool
	IncludeModels []string
	ExcludeModels []string
}

func toStringSet(items []string) map[string]bool {
	if len(items) == 0 {
		return nil
	}
	out := make(map[string]bool, len(items))
	for _, item := range items {
		out[item] = true
	}
	return out
}

type probeFailureKey struct {
	AccountID int64
	Model     string
}

// applyProbeFilter removes desired channels that the probe filter rejects.
// It is intentionally a pure in-memory filter over the desired set; the caller
// has not yet written any route_channels diff, so a rejected channel is simply
// not materialised (soft isolation) rather than hard-disabled.
func applyProbeFilter(ctx context.Context, db *sqlx.DB, desired map[channelIdentity]desiredChannel, opts RebuildOptions) error {
	if !opts.ProbeFilter.Enabled {
		return nil
	}
	include := toStringSet(opts.ProbeFilter.IncludeModels)
	exclude := toStringSet(opts.ProbeFilter.ExcludeModels)
	failed, err := loadLatestProbeFailures(ctx, db)
	if err != nil {
		return err
	}
	for key := range desired {
		model := key.SourceModel
		if len(include) > 0 && !include[model] {
			delete(desired, key)
			continue
		}
		if exclude[model] {
			delete(desired, key)
			continue
		}
		if failed[probeFailureKey{AccountID: key.AccountID, Model: model}] {
			delete(desired, key)
		}
	}
	return nil
}

// loadLatestProbeFailures returns the latest model_probe_results row per
// (account_id, model_name), marking only rows whose latest status is "failure".
// MAX(id) is monotonically increasing with time (SERIAL/AUTOINCREMENT), so this
// works on both SQLite and PostgreSQL without dialect-specific window syntax.
func loadLatestProbeFailures(ctx context.Context, db *sqlx.DB) (map[probeFailureKey]bool, error) {
	type row struct {
		AccountID int64  `db:"account_id"`
		ModelName string `db:"model_name"`
		Status    string `db:"status"`
	}
	var rows []row
	if err := db.SelectContext(ctx, &rows, db.Rebind(`
		SELECT account_id, model_name, status
		FROM model_probe_results
		WHERE id IN (SELECT MAX(id) FROM model_probe_results GROUP BY account_id, model_name)`)); err != nil {
		return nil, fmt.Errorf("load latest probe results: %w", err)
	}
	out := make(map[probeFailureKey]bool, len(rows))
	for _, r := range rows {
		if r.Status == "failure" {
			out[probeFailureKey{AccountID: r.AccountID, Model: r.ModelName}] = true
		}
	}
	return out, nil
}
