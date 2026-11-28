package admin

import (
	"log/slog"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

var (
	bgTasksMu sync.Mutex
	bgTasksDB *store.DB
)

// SetBackgroundTaskDB stores the DB handle used by free-function start/update.
// Called from server boot or test wiring.
func SetBackgroundTaskDB(db *store.DB) {
	bgTasksMu.Lock()
	defer bgTasksMu.Unlock()
	bgTasksDB = db
}

// updateBackgroundTaskDBMessage refreshes only the message/updated_at columns
// of a durable admin_background_tasks row, leaving status/error/started_at/
// finished_at untouched. Used by AppendBackgroundTaskLog so progress lines
// survive a process-local miss (cold cross-process /api/tasks/{id} GET).
func updateBackgroundTaskDBMessage(taskID, message string) {
	bgTasksMu.Lock()
	db := bgTasksDB
	bgTasksMu.Unlock()
	if db == nil {
		return
	}
	nowISO := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(`UPDATE admin_background_tasks SET message=?, updated_at=? WHERE task_id=?`,
		message, nowISO, taskID)
	if err != nil {
		slog.Debug("admin tasks: durable message update failed", "task_id", taskID, "error", err)
	}
}

func insertBackgroundTaskDB(task *BackgroundTask) {
	if task == nil || task.ID == "" {
		return
	}
	nowISO := time.Now().UTC().Format(time.RFC3339)
	var dedupePtr any
	if task.DedupeKey != nil {
		dedupePtr = *task.DedupeKey
	}
	// StartBackgroundTask and its durable-write helpers are free functions
	// invoked from many admin call sites, so the *store.DB cannot be threaded
	// through every caller without churning the public signature. The
	// package-level handle set by SetBackgroundTaskDB (wired once at boot in
	// RegisterTasksRoutes) is the intended bridge from the routed handler
	// layer to these free functions: memory stays the fast path and the DB is
	// the cold cross-process fallback (best-effort; nil DB → memory-only).
	bgTasksMu.Lock()
	db := bgTasksDB
	bgTasksMu.Unlock()
	if db == nil {
		return
	}
	_, err := db.Exec(`
		INSERT INTO admin_background_tasks (task_id, type, title, status, message, dedupe_key, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id) DO NOTHING
	`, task.ID, task.Type, task.Title, string(task.Status), task.Message, dedupePtr, task.CreatedAt, nowISO)
	if err != nil {
		slog.Warn("admin tasks: durable insert failed (memory set)", "task_id", task.ID, "error", err)
	}
}

func updateBackgroundTaskDBStatus(taskID, status string, startedAt, finishedAt, errMsg *string, result any) {
	bgTasksMu.Lock()
	db := bgTasksDB
	bgTasksMu.Unlock()
	if db == nil {
		return
	}
	nowISO := time.Now().UTC().Format(time.RFC3339)
	_, e := db.Exec(`
		UPDATE admin_background_tasks SET status=?, message=?, error=?, started_at=COALESCE(?,started_at),
		finished_at=?, updated_at=? WHERE task_id=? AND status NOT IN ('succeeded','failed')
	`, status, "updated", errMsg, startedAt, finishedAt, nowISO, taskID)
	if e != nil {
		slog.Debug("admin tasks: durable status update failed", "task_id", taskID, "error", e)
	}
	_ = result // result_json column available for future serialization
}

type backgroundTaskRow struct {
	ID         string  `db:"task_id"`
	Type       string  `db:"type"`
	Title      string  `db:"title"`
	Status     string  `db:"status"`
	Message    *string `db:"message"`
	ErrorTxt   *string `db:"error"`
	DedupeKey  *string `db:"dedupe_key"`
	CreatedAt  string  `db:"created_at"`
	UpdatedAt  string  `db:"updated_at"`
	StartedAt  *string `db:"started_at"`
	FinishedAt *string `db:"finished_at"`
}

func (rec backgroundTaskRow) task() BackgroundTask {
	return BackgroundTask{
		ID:         rec.ID,
		Type:       rec.Type,
		Title:      rec.Title,
		Status:     BackgroundTaskStatus(rec.Status),
		Message:    strOrEmpty(rec.Message),
		Error:      rec.ErrorTxt,
		DedupeKey:  rec.DedupeKey,
		CreatedAt:  rec.CreatedAt,
		UpdatedAt:  rec.UpdatedAt,
		StartedAt:  rec.StartedAt,
		FinishedAt: rec.FinishedAt,
		Logs:       []BackgroundTaskLogEntry{},
	}
}

func loadBackgroundTaskDB(db *store.DB, id string) *BackgroundTask {
	if db == nil {
		return nil
	}
	var rec backgroundTaskRow
	err := db.Get(&rec, `
		SELECT task_id, type, title, status, message, error, dedupe_key,
		       created_at, updated_at, started_at, finished_at
		FROM admin_background_tasks WHERE task_id = ?`, id)
	if err != nil {
		return nil
	}
	task := rec.task()
	return &task
}

func listBackgroundTasksDB(db *store.DB, limit int) []BackgroundTask {
	if db == nil {
		return nil
	}
	var rows []backgroundTaskRow
	if err := db.Select(&rows, `
		SELECT task_id, type, title, status, message, error, dedupe_key,
		       created_at, updated_at, started_at, finished_at
		FROM admin_background_tasks ORDER BY created_at DESC LIMIT ?`, limit); err != nil {
		return nil
	}
	var out []BackgroundTask
	for _, r := range rows {
		out = append(out, r.task())
	}
	return out
}

func strOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
