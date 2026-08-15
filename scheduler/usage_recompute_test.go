package scheduler

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// seedRecomputeCheckpoint installs (or upserts) the projection checkpoint with a
// pending recompute starting at recomputeFromID, mirroring how RequestRecompute
// records the rewind point for the next pass.
func seedRecomputeCheckpoint(t *testing.T, db *store.DB, watermark, recomputeFromID int64) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO analytics_projection_checkpoints
		(projector_key, time_zone, last_proxy_log_id, lease_owner, lease_token, lease_expires_at, created_at, updated_at, recompute_from_id, recompute_requested_at)
		VALUES (?, 'UTC', ?, NULL, NULL, NULL, ?, ?, ?, ?)
		ON CONFLICT(projector_key) DO UPDATE SET
			last_proxy_log_id = excluded.last_proxy_log_id,
			lease_owner = NULL,
			lease_token = NULL,
			lease_expires_at = NULL,
			last_error = NULL,
			recompute_from_id = excluded.recompute_from_id,
			recompute_requested_at = excluded.recompute_requested_at,
			updated_at = excluded.updated_at`,
		usageProjectorKey, watermark, now, now, recomputeFromID, now); err != nil {
		t.Fatalf("seed recompute checkpoint: %v", err)
	}
}

func seedDayUsageRow(t *testing.T, db *store.DB, siteID int64, day, now string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO site_day_usage (local_day, site_id, total_calls, created_at, updated_at)
		VALUES (?, ?, 1, ?, ?)`, day, siteID, now, now); err != nil {
		t.Fatalf("seed site_day_usage: %v", err)
	}
}

func seedHourUsageRow(t *testing.T, db *store.DB, siteID int64, hour, now string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO site_hour_usage (bucket_start_utc, site_id, total_calls, created_at, updated_at)
		VALUES (?, ?, 1, ?, ?)`, hour, siteID, now, now); err != nil {
		t.Fatalf("seed site_hour_usage: %v", err)
	}
}

func seedModelDayUsageRow(t *testing.T, db *store.DB, siteID int64, day, model, now string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO model_day_usage (local_day, site_id, model, total_calls, created_at, updated_at)
		VALUES (?, ?, ?, 1, ?, ?)`, day, siteID, model, now, now); err != nil {
		t.Fatalf("seed model_day_usage: %v", err)
	}
}

// readCheckpointRecompute reads last_proxy_log_id + recompute_from_id back from
// the DB so tests can assert the on-disk state after applyRecompute.
func readCheckpointRecompute(t *testing.T, db *store.DB) (watermark int64, recompute *int64) {
	t.Helper()
	row := db.QueryRow(`SELECT last_proxy_log_id, recompute_from_id
		FROM analytics_projection_checkpoints WHERE projector_key = ?`, usageProjectorKey)
	if err := row.Scan(&watermark, &recompute); err != nil {
		t.Fatalf("read checkpoint recompute: %v", err)
	}
	return watermark, recompute
}

// TestApplyRecomputeDeletesAggregatesAndResetsCheckpoint is the positive path:
// a recompute from a known log id wipes the three usage tables for the affected
// day onward and rewinds the watermark so the next pass replays that window.
func TestApplyRecomputeDeletesAggregatesAndResetsCheckpoint(t *testing.T) {
	db := openRetentionTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	suffix := strings.ReplaceAll(t.Name(), "/", "-")

	siteID := insertProjectionSite(t, db, suffix, now)
	accountID := insertProjectionAccount(t, db, siteID, suffix, now)

	logTime := time.Date(2026, 7, 6, 10, 37, 42, 0, time.UTC)
	logID := insertProjectionProxyLog(t, db, accountID, "success", "gpt-4.1", 120, 0.42, 230, logTime)

	affectedDay := logTime.Format("2006-01-02")
	affectedHour := logTime.Truncate(time.Hour).Format(time.RFC3339)

	// Stale aggregates for the affected day must be deleted by the recompute.
	seedDayUsageRow(t, db, siteID, affectedDay, now)
	seedHourUsageRow(t, db, siteID, affectedHour, now)
	seedModelDayUsageRow(t, db, siteID, affectedDay, "gpt-4.1", now)

	seedRecomputeCheckpoint(t, db, logID, logID)

	s := NewUsageAggregationScheduler(testConfig())
	cp := projectionCheckpoint{
		ProjectorKey:   usageProjectorKey,
		TimeZone:       "UTC",
		LastProxyLogID: logID,
		RecomputeFromID: &logID,
	}

	out, err := s.applyRecompute(context.Background(), db, cp)
	if err != nil {
		t.Fatalf("applyRecompute returned error: %v", err)
	}

	wantRestart := logID - 1
	if wantRestart < 0 {
		wantRestart = 0
	}
	if out.LastProxyLogID != wantRestart {
		t.Errorf("returned watermark = %d, want %d", out.LastProxyLogID, wantRestart)
	}
	if out.RecomputeFromID != nil {
		t.Errorf("returned RecomputeFromID = %v, want nil (cleared)", *out.RecomputeFromID)
	}

	if n := countRows(t, db, `SELECT COUNT(*) FROM site_day_usage WHERE site_id = ? AND local_day >= ?`, siteID, affectedDay); n != 0 {
		t.Errorf("site_day_usage rows after recompute = %d, want 0", n)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM site_hour_usage WHERE site_id = ? AND bucket_start_utc >= ?`, siteID, affectedHour); n != 0 {
		t.Errorf("site_hour_usage rows after recompute = %d, want 0", n)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM model_day_usage WHERE site_id = ? AND local_day >= ?`, siteID, affectedDay); n != 0 {
		t.Errorf("model_day_usage rows after recompute = %d, want 0", n)
	}

	watermark, recompute := readCheckpointRecompute(t, db)
	if watermark != wantRestart {
		t.Errorf("db watermark = %d, want %d", watermark, wantRestart)
	}
	if recompute != nil {
		t.Errorf("db recompute_from_id = %v, want NULL (cleared)", *recompute)
	}
}

// TestApplyRecomputeAbortsOnErrorKeepsRecomputeFlag is the regression guard for
// the silent-corruption bug: when a DELETE/UPDATE fails, the recompute must
// abort and leave recompute_from_id at its prior value so the next pass retries
// the whole rewind instead of advancing the watermark past the un-cleared gap.
//
// The error is injected by cancelling the job context; database/sql then returns
// context.Canceled from ExecContext without ever issuing the DELETE, so the
// stale aggregates survive and the checkpoint is left untouched.
func TestApplyRecomputeAbortsOnErrorKeepsRecomputeFlag(t *testing.T) {
	db := openRetentionTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	suffix := strings.ReplaceAll(t.Name(), "/", "-")

	siteID := insertProjectionSite(t, db, suffix, now)
	accountID := insertProjectionAccount(t, db, siteID, suffix, now)

	logTime := time.Date(2026, 7, 6, 10, 37, 42, 0, time.UTC)
	logID := insertProjectionProxyLog(t, db, accountID, "success", "gpt-4.1", 120, 0.42, 230, logTime)

	affectedDay := logTime.Format("2006-01-02")
	seedDayUsageRow(t, db, siteID, affectedDay, now)

	seedRecomputeCheckpoint(t, db, logID, logID)

	s := NewUsageAggregationScheduler(testConfig())
	cp := projectionCheckpoint{
		ProjectorKey:   usageProjectorKey,
		TimeZone:       "UTC",
		LastProxyLogID: logID,
		RecomputeFromID: &logID,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	out, err := s.applyRecompute(ctx, db, cp)
	if err == nil {
		t.Fatal("applyRecompute with cancelled ctx returned nil error; want non-nil")
	}

	// The returned checkpoint must still carry recompute_from_id so the caller
	// does not believe the rewind completed.
	if out.RecomputeFromID == nil {
		t.Fatal("returned RecomputeFromID = nil; want kept for retry")
	}
	if *out.RecomputeFromID != logID {
		t.Errorf("returned RecomputeFromID = %d, want %d", *out.RecomputeFromID, logID)
	}

	// On-disk checkpoint must be unchanged: watermark unchanged and
	// recompute_from_id still set so the next pass replays the rewind.
	watermark, recompute := readCheckpointRecompute(t, db)
	if watermark != logID {
		t.Errorf("db watermark = %d, want %d (must not advance on abort)", watermark, logID)
	}
	if recompute == nil {
		t.Fatal("db recompute_from_id = NULL; want still set so next run retries")
	}
	if *recompute != logID {
		t.Errorf("db recompute_from_id = %d, want %d", *recompute, logID)
	}

	// The stale aggregate must survive because the DELETE never executed.
	if n := countRows(t, db, `SELECT COUNT(*) FROM site_day_usage WHERE site_id = ? AND local_day = ?`, siteID, affectedDay); n != 1 {
		t.Errorf("site_day_usage rows = %d, want 1 (DELETE must not run on aborted ctx)", n)
	}
}

// TestApplyRecomputeNoopWhenRecomputeFlagUnset guards the early return so a pass
// with no pending recompute does not touch any usage table or the checkpoint.
func TestApplyRecomputeNoopWhenRecomputeFlagUnset(t *testing.T) {
	db := openRetentionTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	suffix := strings.ReplaceAll(t.Name(), "/", "-")

	siteID := insertProjectionSite(t, db, suffix, now)
	accountID := insertProjectionAccount(t, db, siteID, suffix, now)
	logID := insertProjectionProxyLog(t, db, accountID, "success", "gpt-4.1", 120, 0.42, 230,
		time.Date(2026, 7, 6, 10, 37, 42, 0, time.UTC))

	affectedDay := "2026-07-06"
	seedDayUsageRow(t, db, siteID, affectedDay, now)
	seedRecomputeCheckpoint(t, db, logID, 0) // recompute_from_id = 0 → no-op

	s := NewUsageAggregationScheduler(testConfig())
	cp := projectionCheckpoint{
		ProjectorKey:   usageProjectorKey,
		TimeZone:       "UTC",
		LastProxyLogID: logID,
		// RecomputeFromID intentionally nil
	}

	out, err := s.applyRecompute(context.Background(), db, cp)
	if err != nil {
		t.Fatalf("applyRecompute noop returned error: %v", err)
	}
	if out.LastProxyLogID != logID {
		t.Errorf("noop watermark = %d, want %d (unchanged)", out.LastProxyLogID, logID)
	}
	if out.RecomputeFromID != nil {
		t.Errorf("noop RecomputeFromID = %v, want nil", *out.RecomputeFromID)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM site_day_usage WHERE site_id = ? AND local_day = ?`, siteID, affectedDay); n != 1 {
		t.Errorf("site_day_usage rows = %d, want 1 (noop must not delete)", n)
	}
}
