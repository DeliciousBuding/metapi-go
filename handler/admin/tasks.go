package admin

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// BackgroundTaskStatus mirrors the TS background task lifecycle.
type BackgroundTaskStatus string

const (
	BackgroundTaskPending   BackgroundTaskStatus = "pending"
	BackgroundTaskRunning   BackgroundTaskStatus = "running"
	BackgroundTaskSucceeded BackgroundTaskStatus = "succeeded"
	BackgroundTaskFailed    BackgroundTaskStatus = "failed"
)

// BackgroundTaskLogEntry is a single task log line.
type BackgroundTaskLogEntry struct {
	Seq       int    `json:"seq"`
	Message   string `json:"message"`
	CreatedAt string `json:"createdAt"`
}

// BackgroundTask is an admin task registry entry (camelCase JSON).
type BackgroundTask struct {
	ID         string                   `json:"id"`
	Type       string                   `json:"type"`
	Title      string                   `json:"title"`
	Status     BackgroundTaskStatus     `json:"status"`
	Message    string                   `json:"message"`
	Error      *string                  `json:"error"`
	Result     any                      `json:"result"`
	DedupeKey  *string                  `json:"dedupeKey"`
	CreatedAt  string                   `json:"createdAt"`
	UpdatedAt  string                   `json:"updatedAt"`
	StartedAt  *string                  `json:"startedAt"`
	FinishedAt *string                  `json:"finishedAt"`
	Logs       []BackgroundTaskLogEntry `json:"logs"`
	expiresAt  time.Time
}

// BackgroundTaskStartOptions configures StartBackgroundTask.
type BackgroundTaskStartOptions struct {
	Type      string
	Title     string
	DedupeKey string
	KeepMs    int64
}

// RegisterTasksRoutes registers /api/tasks routes.
// list/get observe memory-visible tasks AND durable tasks from admin_background_tasks
// when the handler DB is set. Cold DB fallback enables cross-instance visibility.
func RegisterTasksRoutes(r chi.Router, db *store.DB) {
	SetBackgroundTaskDB(db)
	handler := &tasksHandler{db: db}
	r.Get("/api/tasks", handler.listTasks)
	r.Get("/api/tasks/{id}", handler.getTask)
}

type tasksHandler struct {
	db *store.DB
}

// GET /api/tasks?limit=
func (h *tasksHandler) listTasks(w http.ResponseWriter, r *http.Request) {
	limit, _ := parseLimitOffset(r, 50, 200)
	tasks := listBackgroundTasks(h.db, limit)
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"tasks":   tasks,
	})
}

// GET /api/tasks/:id
func (h *tasksHandler) getTask(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	task := getBackgroundTask(h.db, id)
	if task == nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"task":    task,
	})
}

func getBackgroundTask(db *store.DB, id string) *BackgroundTask {
	backgroundTasksMu.Lock()
	cleanupExpiredBackgroundTasksLocked(time.Now())
	task, ok := backgroundTasks[id]
	if ok {
		// Snapshot under the lock: runBackgroundTask mutates Status/Result/Error
		// while HTTP get/list may observe the same *BackgroundTask concurrently.
		// Copying after Unlock races with Result assignment (DATA RACE under -race).
		cp := snapshotBackgroundTask(task)
		backgroundTasksMu.Unlock()
		return &cp
	}
	backgroundTasksMu.Unlock()
	// Cold DB fallback
	return loadBackgroundTaskDB(db, id)
}

func listBackgroundTasks(db *store.DB, limit int) []BackgroundTask {
	if limit < 1 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	backgroundTasksMu.Lock()
	cleanupExpiredBackgroundTasksLocked(time.Now())
	seen := make(map[string]bool)
	var all []BackgroundTask
	for _, task := range backgroundTasks {
		cp := snapshotBackgroundTask(task)
		all = append(all, cp)
		seen[cp.ID] = true
	}
	backgroundTasksMu.Unlock()

	// Cold DB fallback: merge rows from other processes not in memory.
	dbTasks := listBackgroundTasksDB(db, limit*2)
	for _, t := range dbTasks {
		if seen[t.ID] {
			continue
		}
		if t.Logs == nil {
			t.Logs = []BackgroundTaskLogEntry{}
		}
		all = append(all, t)
		seen[t.ID] = true
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].CreatedAt > all[j].CreatedAt
	})
	if len(all) > limit {
		all = all[:limit]
	}
	if all == nil {
		all = []BackgroundTask{}
	}
	return all
}
