package scheduler

import (
	"testing"
	"time"
)

// ---- E1b: missed-window catch-up (check-in reliability, 2026-08-01) ----
// A daily cron/window schedule must not lose the day when the instance
// restarts after the trigger time.

func fixedNow(h, m int) time.Time {
	return time.Date(2026, 8, 1, h, m, 0, 0, time.Local)
}

func TestShouldCatchUpCheckin_MissedTriggerCompensates(t *testing.T) {
	// Scheduled 06:07, instance back at 09:30, nothing ran today → catch up.
	if !shouldCatchUpCheckin(fixedNow(9, 30), "7 6 * * *", false, 2) {
		t.Fatal("missed today's trigger with accounts and no run: want catch-up")
	}
}

func TestShouldCatchUpCheckin_TriggerAheadNoCatchUp(t *testing.T) {
	// Scheduled 06:07, instance back at 05:00 → cron fires normally.
	if shouldCatchUpCheckin(fixedNow(5, 0), "7 6 * * *", false, 2) {
		t.Fatal("trigger still ahead today: want no catch-up")
	}
}

func TestShouldCatchUpCheckin_AlreadyRanTodayNoCatchUp(t *testing.T) {
	// Ran at 06:07 already (checkin_logs has entries) → no double run.
	if shouldCatchUpCheckin(fixedNow(9, 30), "7 6 * * *", true, 2) {
		t.Fatal("already ran today: want no catch-up")
	}
}

func TestShouldCatchUpCheckin_NoEnabledAccountsNoCatchUp(t *testing.T) {
	if shouldCatchUpCheckin(fixedNow(9, 30), "7 6 * * *", false, 0) {
		t.Fatal("no enabled accounts: want no catch-up")
	}
}

func TestShouldCatchUpCheckin_NoTriggerTodayNoCatchUp(t *testing.T) {
	// Monday-only schedule, today is Saturday (2026-08-01 is a Saturday) →
	// next trigger is next Monday, nothing to catch up.
	if shouldCatchUpCheckin(fixedNow(9, 30), "0 6 * * 1", false, 2) {
		t.Fatal("no trigger today: want no catch-up")
	}
}

func TestShouldCatchUpCheckin_InvalidSpecNoCatchUp(t *testing.T) {
	if shouldCatchUpCheckin(fixedNow(9, 30), "not-a-cron", false, 2) {
		t.Fatal("invalid spec: want no catch-up")
	}
}

func TestShouldCatchUpCheckin_ExactlyAtTrigger(t *testing.T) {
	// Now == trigger minute: not "missed" yet — cron fires at this minute.
	if shouldCatchUpCheckin(fixedNow(6, 7), "7 6 * * *", false, 2) {
		t.Fatal("at trigger time: want no catch-up (cron fires now)")
	}
}

func TestShouldCatchUpCheckin_WindowModeSpec(t *testing.T) {
	// Window mode cron shape is "m h * * *" (5-field) — same as above.
	if !shouldCatchUpCheckin(fixedNow(10, 0), "12 8 * * *", false, 1) {
		t.Fatal("5-field window spec after trigger: want catch-up")
	}
}
