package app

import "testing"

// The update functions must be safe no-ops before StartBackgroundServices has
// published scheduler pointers (e.g. an admin PUT arriving during startup).
func TestSettingsUpdateFunctionsNoOpBeforeStart(t *testing.T) {
	if err := UpdateCheckinSchedule("cron", "0 8 * * *", 6, "00:00", "23:59"); err != nil {
		t.Fatalf("UpdateCheckinSchedule: %v", err)
	}
	if err := UpdateBalanceCron("0 9 * * *"); err != nil {
		t.Fatalf("UpdateBalanceCron: %v", err)
	}
	if err := UpdateLogCleanupSettings("0 6 * * *", true, true, 30); err != nil {
		t.Fatalf("UpdateLogCleanupSettings: %v", err)
	}
	if err := ReloadWebdavBackup(); err != nil {
		t.Fatalf("ReloadWebdavBackup: %v", err)
	}
}
