package scheduler

import (
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
)

// TestAdminBackgroundTaskRetentionDeletesOldTerminalRowsKeepsRest verifies the
// daily retention job prunes only terminal (succeeded/failed) rows older than
// the 30-day window. Pending/running rows and in-window rows are always kept so
// operators never lose visibility of in-flight or recent tasks.
func TestAdminBackgroundTaskRetentionDeletesOldTerminalRowsKeepsRest(t *testing.T) {
	db := openRetentionTestDB(t)

	// Absolute ages relative to real now: retention uses time.Now() - retentionDays.
	oldAt := time.Now().UTC().Add(-31 * 24 * time.Hour).Format(time.RFC3339) // outside 30d window
	keepAt := time.Now().UTC().Add(-1 * 24 * time.Hour).Format(time.RFC3339) // inside 30d window

	tasks := []struct {
		taskID string
		status string
		createdAt string
	}{
		{"t-old-succeeded", "succeeded", oldAt}, // deleted (terminal + old)
		{"t-old-failed", "failed", oldAt},       // deleted (terminal + old)
		{"t-old-pending", "pending", oldAt},     // kept (non-terminal)
		{"t-old-running", "running", oldAt},     // kept (non-terminal)
		{"t-new-succeeded", "succeeded", keepAt}, // kept (in window)
		{"t-new-failed", "failed", keepAt},       // kept (in window)
	}
	for _, tk := range tasks {
		if _, err := db.Exec(
			`INSERT INTO admin_background_tasks (task_id, type, title, status, created_at, updated_at)
			 VALUES (?, 'test', ?, ?, ?, ?)`,
			tk.taskID, tk.taskID, tk.status, tk.createdAt, tk.createdAt); err != nil {
			t.Fatalf("seed admin_background_tasks %s: %v", tk.taskID, err)
		}
	}

	s := NewAdminBackgroundTaskRetentionScheduler(&config.Config{})
	s.runCleanupLocked(db, adminBackgroundTaskRetentionDays)

	if got := countRows(t, db, `SELECT COUNT(*) FROM admin_background_tasks`); got != 4 {
		t.Fatalf("admin_background_tasks count = %d, want 4 (2 old terminal deleted)", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM admin_background_tasks WHERE status IN ('succeeded','failed')`); got != 2 {
		t.Fatalf("terminal rows = %d, want 2 (only in-window retained)", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM admin_background_tasks WHERE status = 'pending'`); got != 1 {
		t.Errorf("pending rows = %d, want 1 (never pruned)", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM admin_background_tasks WHERE status = 'running'`); got != 1 {
		t.Errorf("running rows = %d, want 1 (never pruned)", got)
	}
	// Old non-terminal rows survive even though they are past the cutoff.
	if got := countRows(t, db, `SELECT COUNT(*) FROM admin_background_tasks WHERE task_id = ?`, "t-old-pending"); got != 1 {
		t.Errorf("old pending row deleted; retention must only touch terminal statuses")
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM admin_background_tasks WHERE task_id = ?`, "t-old-running"); got != 1 {
		t.Errorf("old running row deleted; retention must only touch terminal statuses")
	}
}
