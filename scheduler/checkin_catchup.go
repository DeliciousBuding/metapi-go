package scheduler

import (
	"time"

	"github.com/robfig/cron/v3"
)

// shouldCatchUpCheckin decides whether today's scheduled check-in time has
// already passed without a run (instance was down / restarted after the
// trigger) and must be compensated with an immediate run. Pure decision
// logic, no I/O — fully testable.
//
// E1b (check-in reliability, 2026-08-01): a daily cron/window schedule must
// not lose the day when the instance restarts after the trigger time.
// CheckinAll itself is idempotent (already-checked-in upstream responses
// classify as success without re-advancing), so an immediate run never
// double-signs.
func shouldCatchUpCheckin(now time.Time, spec string, ranToday bool, enabledAccounts int) bool {
	if ranToday || enabledAccounts == 0 || spec == "" {
		return false
	}
	parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	sch, err := parser.Parse(normalizeCronExpr(spec))
	if err != nil {
		return false
	}
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	next := sch.Next(startOfDay)
	// Trigger still ahead (or exactly now — the armed cron fires this minute):
	// no catch-up needed.
	if !next.Before(now) {
		return false
	}
	// No trigger today (e.g. day-of-week-limited spec) — nothing to catch up.
	if next.Year() != now.Year() || next.Month() != now.Month() || next.Day() != now.Day() {
		return false
	}
	return true // today's trigger already passed and nothing ran yet
}
