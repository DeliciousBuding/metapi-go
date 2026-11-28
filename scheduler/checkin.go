package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/service/checkin"
	"github.com/deliciousbuding/metapi-go/store"
)

// intervalCandidate holds the fields needed for interval-based checkin filtering.
type intervalCandidate struct {
	id            int64
	lastCheckinAt *string
}

// ResetAttempts clears the interval attempt map (for tests).
func (s *CheckinScheduler) ResetAttempts() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attemptByAccount = make(map[int64]int64)
}

func (s *CheckinScheduler) runCronJob() {
	slog.Info("checkin: cron job starting")
	dbw := s.db
	if dbw == nil {
		slog.Error("checkin: database not available")
		return
	}
	// Derive the job timeout from the lifecycle ctx (cancellable on Stop),
	// not context.Background(). runWithSchedulerLease still releases the
	// lease via context.Background() when this ctx is already done.
	jobCtx, cancel := context.WithTimeout(s.ctx, checkinJobTimeout)
	defer cancel()
	runWithSchedulerLease(jobCtx, dbw, s.Name(), func() {
		results := checkin.CheckinAllContext(jobCtx, s.cfg, dbw.DB, nil, "cron")
		ok, bad := countResults(results)
		slog.Info("checkin: cron job done", "success", ok, "failed", bad)
	})
}

func (s *CheckinScheduler) runIntervalPass() {
	dbw := s.db
	if dbw == nil {
		return
	}
	// Derive the job timeout from the lifecycle ctx (cancellable on Stop),
	// not context.Background(). runWithSchedulerLease still releases the
	// lease via context.Background() when this ctx is already done.
	jobCtx, cancel := context.WithTimeout(s.ctx, checkinJobTimeout)
	defer cancel()
	runWithSchedulerLease(jobCtx, dbw, s.Name(), func() {
		s.runIntervalPassLocked(jobCtx, dbw)
	})
}

// runIntervalPassLocked runs one interval pass. ctx is the job ctx (derived
// from the lifecycle ctx): it is what the checkin round's pacing wait is
// cancellable by, so Stop() ends a pass instead of leaving it sleeping between
// accounts of the same site.
func (s *CheckinScheduler) runIntervalPassLocked(ctx context.Context, dbw *store.DB) {
	now := time.Now()

	// Query all account+site pairs
	rows, err := dbw.Query(`
		SELECT a.id, a.last_checkin_at
		FROM accounts a
		INNER JOIN sites s ON a.site_id = s.id
		WHERE a.checkin_enabled = TRUE
		  AND a.status = 'active'
		  AND s.status <> 'disabled'
	`)
	if err != nil {
		slog.Error("checkin interval: query failed", "error", err)
		return
	}
	defer rows.Close()

	var candidates []intervalCandidate
	var scanErr error
	for rows.Next() {
		var c intervalCandidate
		if err := rows.Scan(&c.id, &c.lastCheckinAt); err != nil {
			if scanErr == nil {
				scanErr = err
			}
			continue
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil && scanErr == nil {
		scanErr = err
	}
	if scanErr != nil {
		slog.Warn("checkin interval: candidate rows degraded", "error", scanErr)
	}

	dueIDs := s.filterDue(candidates, now)
	if len(dueIDs) == 0 {
		return
	}

	results := s.checkinAll(ctx, s.cfg, dbw.DB, dueIDs, "interval")

	nowMs := now.UnixMilli()
	s.mu.Lock()
	for _, r := range results {
		s.attemptByAccount[r.AccountID] = nowMs
	}
	s.mu.Unlock()

	ok, bad := countResults(results)
	slog.Info("checkin: interval pass done",
		"due", len(dueIDs), "success", ok, "failed", bad)
}

// filterDue mirrors TS selectDueIntervalCheckinAccountIds().
func (s *CheckinScheduler) filterDue(rows []intervalCandidate, now time.Time) []int64 {
	nowMs := now.UnixMilli()
	intervalHours := config.ClampInt(config.Runtime().CheckinIntervalHours, 1, 24)
	intervalMs := int64(intervalHours) * 3600 * 1000

	s.mu.Lock()
	defer s.mu.Unlock()

	var due []int64
	for _, row := range rows {
		hasCheckin := false
		var checkinMs int64
		if row.lastCheckinAt != nil && *row.lastCheckinAt != "" {
			if t, err := time.Parse(time.RFC3339, *row.lastCheckinAt); err == nil {
				checkinMs = t.UnixMilli()
				hasCheckin = true
			}
		}
		attemptMs, hasAttempt := s.attemptByAccount[row.id]

		if hasCheckin {
			if nowMs-checkinMs < intervalMs {
				continue // already checked in within window
			}
			if hasAttempt && attemptMs >= checkinMs && nowMs-attemptMs < intervalMs {
				continue // mid-flight checkin
			}
		} else {
			if hasAttempt && nowMs-attemptMs < intervalMs {
				continue // recently attempted, no known checkin
			}
		}
		due = append(due, row.id)
	}
	return due
}

func countResults(results []checkin.CheckinAllResult) (success, failed int) {
	for _, r := range results {
		if r.Result.Success {
			success++
		} else {
			failed++
		}
	}
	return
}
