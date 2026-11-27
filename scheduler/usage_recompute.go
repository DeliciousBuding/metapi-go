package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

func (s *UsageAggregationScheduler) applyRecompute(ctx context.Context, dbw *store.DB, cp projectionCheckpoint) (projectionCheckpoint, error) {
	recomputeFromID := int64(0)
	if cp.RecomputeFromID != nil {
		recomputeFromID = *cp.RecomputeFromID
	}
	if recomputeFromID <= 0 {
		return cp, nil
	}

	// Find affected row
	var affectedID int64
	var affectedCreatedAt string
	row := dbw.QueryRow(`
		SELECT id, created_at FROM proxy_logs
		WHERE id >= ?
		ORDER BY id ASC LIMIT 1
	`, recomputeFromID)
	if err := row.Scan(&affectedID, &affectedCreatedAt); err != nil {
		// Affected row no longer exists - clear recompute. On failure we keep
		// recompute_from_id at its prior value so the next pass retries the
		// clear instead of silently dropping the recompute request.
		now := time.Now().UTC().Format(time.RFC3339)
		result, err := dbw.ExecContext(ctx, `UPDATE analytics_projection_checkpoints
			SET recompute_from_id = NULL, recompute_requested_at = NULL, updated_at = ?
			WHERE projector_key = ?`, now, usageProjectorKey)
		if err != nil {
			slog.Error("usage-aggregation: recompute clear (row gone) failed",
				"error", err, "recompute_from_id", recomputeFromID)
			return cp, fmt.Errorf("recompute: clear checkpoint (row gone): %w", err)
		}
		if affected, _ := result.RowsAffected(); affected == 0 {
			slog.Warn("usage-aggregation: recompute clear (row gone) affected 0 rows",
				"recompute_from_id", recomputeFromID)
		}
		return projectionCheckpoint{
			ProjectorKey:         cp.ProjectorKey,
			TimeZone:             cp.TimeZone,
			LastProxyLogID:       cp.LastProxyLogID,
			RecomputeFromID:      nil,
			RecomputeRequestedAt: nil,
		}, nil
	}

	// Parse day from affected row
	t, err := time.Parse(time.RFC3339, affectedCreatedAt)
	if err != nil {
		t, err = time.Parse("2006-01-02 15:04:05", affectedCreatedAt)
	}
	if err != nil {
		return cp, fmt.Errorf("failed to resolve recompute boundary for usage aggregates")
	}

	affectedDay := t.UTC().Format("2006-01-02")
	dayStartUTC := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)

	now := time.Now().UTC().Format(time.RFC3339)

	// Delete aggregates for the affected day and beyond. Each DELETE is
	// idempotent, so a mid-sequence failure is safe to retry on the next
	// pass: recompute_from_id is only cleared by the checkpoint reset below,
	// which never runs when an earlier step returns an error. This is the
	// core fix for silent usage-aggregate corruption: a failed DELETE no
	// longer advances the watermark past the gap it failed to clear.
	deleteResult, err := dbw.ExecContext(ctx, "DELETE FROM site_day_usage WHERE local_day >= ?", affectedDay)
	if err != nil {
		slog.Error("usage-aggregation: recompute delete site_day_usage failed",
			"error", err, "affected_day", affectedDay)
		return cp, fmt.Errorf("recompute: delete site_day_usage: %w", err)
	}
	if deleted, _ := deleteResult.RowsAffected(); deleted > 0 {
		slog.Info("usage-aggregation: recompute deleted site_day_usage",
			"rows", deleted, "affected_day", affectedDay)
	}

	hourCutoff := dayStartUTC.Format(time.RFC3339)
	deleteResult, err = dbw.ExecContext(ctx, "DELETE FROM site_hour_usage WHERE bucket_start_utc >= ?", hourCutoff)
	if err != nil {
		slog.Error("usage-aggregation: recompute delete site_hour_usage failed",
			"error", err, "hour_cutoff", hourCutoff)
		return cp, fmt.Errorf("recompute: delete site_hour_usage: %w", err)
	}
	if deleted, _ := deleteResult.RowsAffected(); deleted > 0 {
		slog.Info("usage-aggregation: recompute deleted site_hour_usage",
			"rows", deleted, "hour_cutoff", hourCutoff)
	}

	deleteResult, err = dbw.ExecContext(ctx, "DELETE FROM model_day_usage WHERE local_day >= ?", affectedDay)
	if err != nil {
		slog.Error("usage-aggregation: recompute delete model_day_usage failed",
			"error", err, "affected_day", affectedDay)
		return cp, fmt.Errorf("recompute: delete model_day_usage: %w", err)
	}
	if deleted, _ := deleteResult.RowsAffected(); deleted > 0 {
		slog.Info("usage-aggregation: recompute deleted model_day_usage",
			"rows", deleted, "affected_day", affectedDay)
	}

	// Reset checkpoint so projection replays from just before the affected row.
	// This runs only after all three DELETEs succeeded, so a partial failure
	// leaves recompute_from_id set and the next pass replays the whole rewind.
	restartFromID := affectedID - 1
	if restartFromID < 0 {
		restartFromID = 0
	}
	result, err := dbw.ExecContext(ctx, `UPDATE analytics_projection_checkpoints
		SET last_proxy_log_id = ?, recompute_from_id = NULL, recompute_requested_at = NULL, updated_at = ?
		WHERE projector_key = ?`, restartFromID, now, usageProjectorKey)
	if err != nil {
		slog.Error("usage-aggregation: recompute checkpoint reset failed",
			"error", err, "restart_from_id", restartFromID)
		return cp, fmt.Errorf("recompute: reset checkpoint: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		slog.Error("usage-aggregation: recompute checkpoint reset affected 0 rows",
			"restart_from_id", restartFromID)
		return cp, fmt.Errorf("recompute: reset checkpoint affected 0 rows")
	}

	return projectionCheckpoint{
		ProjectorKey:         cp.ProjectorKey,
		TimeZone:             cp.TimeZone,
		LastProxyLogID:       restartFromID,
		RecomputeFromID:      nil,
		RecomputeRequestedAt: nil,
	}, nil
}

// RequestRecompute requests a recompute of usage aggregates starting from fromLogID.
// When a recompute is already pending, the earlier (smaller) log id is kept so the
// rewind window is never silently reduced.
func (s *UsageAggregationScheduler) RequestRecompute(fromLogID int64) {
	dbw := s.db
	if dbw == nil {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	fromID := fromLogID
	if fromID < 1 {
		fromID = 1
	}
	cp := s.readCheckpoint(dbw)
	if cp.RecomputeFromID != nil && *cp.RecomputeFromID > 0 && *cp.RecomputeFromID < fromID {
		fromID = *cp.RecomputeFromID
	}
	if _, err := dbw.Exec(`UPDATE analytics_projection_checkpoints
		SET recompute_from_id = ?, recompute_requested_at = ?, updated_at = ?
		WHERE projector_key = ?`, fromID, now, now, usageProjectorKey); err != nil {
		slog.Warn("usage-aggregation: failed to request recompute", "from_log_id", fromID, "error", err)
		return
	}
	slog.Info("usage-aggregation: recompute requested", "from_log_id", fromID)
}
