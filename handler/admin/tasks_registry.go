package admin

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"sync"
	"time"
)

const (
	backgroundTaskTTLMs           = 6 * 60 * 60 * 1000
	backgroundTaskCleanupInterval = 60 * time.Second
	backgroundTaskLogLimit        = 200
)

var (
	backgroundTasksMu     sync.Mutex
	backgroundTasks       = map[string]*BackgroundTask{}
	backgroundDedupeIDs   = map[string]string{}
	backgroundCleanupOnce sync.Once
)

// StartBackgroundTask queues a job or reuses a pending/running task with the
// same process-local dedupe key. It also stores the task when a DB is set.
func StartBackgroundTask(opts BackgroundTaskStartOptions, runner func() (any, error)) (task *BackgroundTask, reused bool) {
	ensureBackgroundTaskCleanup()
	now := time.Now().UTC()
	nowISO := now.Format(time.RFC3339Nano)

	backgroundTasksMu.Lock()
	defer backgroundTasksMu.Unlock()

	dedupeKey := opts.DedupeKey
	if dedupeKey != "" {
		if existingID, ok := backgroundDedupeIDs[dedupeKey]; ok {
			if existing := backgroundTasks[existingID]; existing != nil &&
				(existing.Status == BackgroundTaskPending || existing.Status == BackgroundTaskRunning) {
				cp := snapshotBackgroundTask(existing)
				return &cp, true
			}
			delete(backgroundDedupeIDs, dedupeKey)
		}
	}

	keepMs := opts.KeepMs
	if keepMs < 60_000 {
		keepMs = backgroundTaskTTLMs
	}

	var dedupePtr *string
	if dedupeKey != "" {
		key := dedupeKey
		dedupePtr = &key
	}

	task = &BackgroundTask{
		ID:        newBackgroundTaskID(),
		Type:      opts.Type,
		Title:     opts.Title,
		Status:    BackgroundTaskPending,
		Message:   opts.Title + " started",
		Error:     nil,
		Result:    nil,
		DedupeKey: dedupePtr,
		CreatedAt: nowISO,
		UpdatedAt: nowISO,
		Logs:      []BackgroundTaskLogEntry{},
		expiresAt: now.Add(time.Duration(keepMs) * time.Millisecond),
	}
	backgroundTasks[task.ID] = task
	if dedupeKey != "" {
		backgroundDedupeIDs[dedupeKey] = task.ID
	}

	// Snapshot id for goroutine
	taskID := task.ID
	title := opts.Title

	// Dual-write to DB best-effort. Memory is the primary layer.
	insertBackgroundTaskDB(task)

	go runBackgroundTask(taskID, title, dedupeKey, runner)

	cp := snapshotBackgroundTask(task)
	return &cp, false
}

func runBackgroundTask(taskID, title, dedupeKey string, runner func() (any, error)) {
	started := time.Now().UTC().Format(time.RFC3339Nano)
	backgroundTasksMu.Lock()
	if task, ok := backgroundTasks[taskID]; ok {
		task.Status = BackgroundTaskRunning
		task.StartedAt = &started
		task.Message = title + " running"
		task.UpdatedAt = started
	}
	backgroundTasksMu.Unlock()

	// DB update best-effort
	updateBackgroundTaskDBStatus(taskID, string(BackgroundTaskRunning), &started, nil, nil, nil)

	result, err := runner()
	finished := time.Now().UTC().Format(time.RFC3339Nano)

	backgroundTasksMu.Lock()
	defer backgroundTasksMu.Unlock()
	task, ok := backgroundTasks[taskID]
	if !ok {
		return
	}
	task.FinishedAt = &finished
	task.UpdatedAt = finished
	task.Result = result
	var errPtr *string
	var status BackgroundTaskStatus
	if err != nil {
		msg := err.Error()
		errPtr = &msg
		task.Status = BackgroundTaskFailed
		task.Error = &msg
		task.Message = title + " failed: " + msg
		status = BackgroundTaskFailed
	} else {
		task.Status = BackgroundTaskSucceeded
		task.Error = nil
		task.Message = title + " completed"
		status = BackgroundTaskSucceeded
	}
	if dedupeKey != "" && backgroundDedupeIDs[dedupeKey] == taskID {
		delete(backgroundDedupeIDs, dedupeKey)
	}

	// DB update best-effort
	updateBackgroundTaskDBStatus(taskID, string(status), nil, &finished, errPtr, result)
}

// AppendBackgroundTaskLog appends a progress log entry to a pending/running
// background task's in-memory Logs slice (capped at backgroundTaskLogLimit)
// and updates its human-readable Message so /api/tasks/{id} polling surfaces
// meaningful progress while the runner is still executing. It is a no-op for
// unknown or already-terminal task IDs so callers can call it unconditionally
// from inside a runner without tracking liveness themselves.
//
// Logs are memory-only (admin_background_tasks has no logs column); the
// durable row's message column is updated best-effort so a cross-process
// /api/tasks/{id} GET still shows the latest progress line.
func AppendBackgroundTaskLog(taskID, message string) {
	if taskID == "" {
		return
	}
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return
	}
	now := time.Now().UTC()
	nowISO := now.Format(time.RFC3339Nano)

	backgroundTasksMu.Lock()
	task, ok := backgroundTasks[taskID]
	if !ok {
		backgroundTasksMu.Unlock()
		return
	}
	// Only append while the task is still active; terminal tasks are frozen.
	if task.Status != BackgroundTaskPending && task.Status != BackgroundTaskRunning {
		backgroundTasksMu.Unlock()
		return
	}
	seq := len(task.Logs) + 1
	task.Logs = append(task.Logs, BackgroundTaskLogEntry{
		Seq:       seq,
		Message:   trimmed,
		CreatedAt: nowISO,
	})
	if len(task.Logs) > backgroundTaskLogLimit {
		task.Logs = task.Logs[len(task.Logs)-backgroundTaskLogLimit:]
	}
	task.Message = trimmed
	task.UpdatedAt = nowISO
	backgroundTasksMu.Unlock()

	// Best-effort durable message update (no logs column; message only).
	updateBackgroundTaskDBMessage(taskID, trimmed)
}

// snapshotBackgroundTask returns a value copy safe for callers outside
// backgroundTasksMu. Logs are slice-copied so later appends on the live task
// do not race readers. Callers must hold backgroundTasksMu when task is live.
func snapshotBackgroundTask(task *BackgroundTask) BackgroundTask {
	if task == nil {
		return BackgroundTask{Logs: []BackgroundTaskLogEntry{}}
	}
	cp := *task
	if cp.Logs == nil {
		cp.Logs = []BackgroundTaskLogEntry{}
	} else {
		logs := make([]BackgroundTaskLogEntry, len(cp.Logs))
		copy(logs, cp.Logs)
		cp.Logs = logs
	}
	return cp
}

func ensureBackgroundTaskCleanup() {
	backgroundCleanupOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(backgroundTaskCleanupInterval)
			defer ticker.Stop()
			for range ticker.C {
				backgroundTasksMu.Lock()
				cleanupExpiredBackgroundTasksLocked(time.Now())
				backgroundTasksMu.Unlock()
			}
		}()
	})
}

func cleanupExpiredBackgroundTasksLocked(now time.Time) {
	for id, task := range backgroundTasks {
		if !task.expiresAt.IsZero() && !task.expiresAt.After(now) {
			if task.DedupeKey != nil && backgroundDedupeIDs[*task.DedupeKey] == id {
				delete(backgroundDedupeIDs, *task.DedupeKey)
			}
			delete(backgroundTasks, id)
		}
	}
}

func newBackgroundTaskID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(b[:])
}

// waitAllBackgroundTasksForTests polls until every in-memory task is terminal
// or the timeout elapses. Used by tests so cleanup does not race runners that
// still touch package globals (e.g. globalAccountsCache.clear).
func waitAllBackgroundTasksForTests(timeout time.Duration) {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		backgroundTasksMu.Lock()
		pending := false
		for _, task := range backgroundTasks {
			if task.Status == BackgroundTaskPending || task.Status == BackgroundTaskRunning {
				pending = true
				break
			}
		}
		backgroundTasksMu.Unlock()
		if !pending {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// resetBackgroundTasksForTests clears the in-memory registry (tests only).
// It first waits briefly for runners so map teardown does not race active work.
func resetBackgroundTasksForTests() {
	waitAllBackgroundTasksForTests(3 * time.Second)
	backgroundTasksMu.Lock()
	defer backgroundTasksMu.Unlock()
	backgroundTasks = map[string]*BackgroundTask{}
	backgroundDedupeIDs = map[string]string{}
}
