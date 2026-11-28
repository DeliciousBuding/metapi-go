package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/service/checkin"
	"github.com/deliciousbuding/metapi-go/store"
)

// maybeCatchUpCheckin queries whether today's scheduled check-in already
// passed without a run and, if so, triggers an immediate run (guarded by the
// scheduler lease like every other run). Idempotent by construction.
//
// When the global catch-up does not apply (today's run already happened, or
// the trigger is still ahead), it falls back to a per-account stale catch-up
// (issue #669): enqueue accounts whose last_checkin_at is nil or older than
// 24h. This covers cases the global "ranToday" gate misses — e.g. an account
// was added mid-day, recovered after a transient failure, or was skipped by
// an aborted prior run while other accounts succeeded.
func (s *CheckinScheduler) maybeCatchUpCheckin() {
	dbw := s.db
	if dbw == nil {
		return
	}
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	var ranToday int
	if err := dbw.QueryRow(dbw.Rebind(`SELECT COUNT(*) FROM checkin_logs WHERE created_at >= ?`), startOfDay.Format(time.RFC3339)).Scan(&ranToday); err != nil {
		slog.Warn("checkin catch-up: ran-today query failed", "error", err)
		return
	}
	var enabled int
	if err := dbw.QueryRow(`SELECT COUNT(*) FROM accounts WHERE checkin_enabled = TRUE AND status = 'active'`).Scan(&enabled); err != nil {
		slog.Warn("checkin catch-up: enabled-count query failed", "error", err)
		return
	}

	if shouldCatchUpCheckin(now, config.Runtime().CheckinCron, ranToday > 0, enabled) {
		slog.Info("checkin: missed today's scheduled time, compensating with immediate run", "spec", config.Runtime().CheckinCron)
		// Bare goroutine: cronRunner's recover only covers cron-fired
		// runs, so the boundary wrapper must protect this one too.
		go safeJob("checkin-catchup", s.runCronJob)
		return
	}

	// Per-account catch-up (issue #669). Runs in a goroutine so startup is
	// not blocked; pacing and the per-account lease are reused via CheckinAll.
	go safeJob("checkin-stale-catchup", func() { s.runStaleAccountCatchUp(dbw) })
}

// runStaleAccountCatchUp enqueues an immediate checkin for every enabled,
// active account whose last_checkin_at is nil/empty or older than 24h. Shares
// the scheduler lease with cron/interval runs so a concurrent tick cannot
// double-run. CheckinAll's same-site pacing and CheckinAccount's per-account
// lease apply unchanged (issue #669).
func (s *CheckinScheduler) runStaleAccountCatchUp(dbw *store.DB) {
	staleIDs, err := selectStaleCheckinAccountIDs(dbw, time.Now().Add(-24*time.Hour).UTC().Format(time.RFC3339))
	if err != nil {
		slog.Warn("checkin stale catch-up: query failed", "error", err)
		return
	}
	if len(staleIDs) == 0 {
		return
	}
	slog.Info("checkin: stale account catch-up", "count", len(staleIDs))
	// Derive the job timeout from the lifecycle ctx (cancellable on Stop),
	// not context.Background(). runWithSchedulerLease still releases the
	// lease via context.Background() when this ctx is already done.
	jobCtx, cancel := context.WithTimeout(s.ctx, checkinJobTimeout)
	defer cancel()
	runWithSchedulerLease(jobCtx, dbw, s.Name(), func() {
		results := checkin.CheckinAllContext(jobCtx, s.cfg, dbw.DB, staleIDs, "catchup")
		ok, bad := countResults(results)
		slog.Info("checkin: stale account catch-up done", "enqueued", len(staleIDs), "success", ok, "failed", bad)
	})
}

// selectStaleCheckinAccountIDs returns IDs of enabled, active accounts on
// non-disabled sites whose last_checkin_at is nil/empty or older than the
// cutoff (RFC3339). Pure query — pacing and execution happen via CheckinAll.
// String comparison of RFC3339 timestamps is chronological because both
// last_checkin_at and the cutoff are persisted in UTC.
func selectStaleCheckinAccountIDs(dbw *store.DB, cutoffRFC3339 string) ([]int64, error) {
	rows, err := dbw.Query(`
		SELECT a.id FROM accounts a
		INNER JOIN sites s ON a.site_id = s.id
		WHERE a.checkin_enabled = TRUE AND a.status = 'active' AND s.status <> 'disabled'
		  AND (a.last_checkin_at IS NULL OR a.last_checkin_at = '' OR a.last_checkin_at < ?)
	`, cutoffRFC3339)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	var scanErr error
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			if scanErr == nil {
				scanErr = err
			}
			continue
		}
		ids = append(ids, id)
	}
	if scanErr != nil {
		return ids, scanErr
	}
	return ids, rows.Err()
}
