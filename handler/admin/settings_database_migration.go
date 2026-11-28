package admin

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
)

// POST /api/settings/database/migrate
//
// Queues a SQLite→PostgreSQL (or reverse) migration as an admin background
// task and returns 202 Accepted with the task ID so the UI can poll
// /api/tasks/{id} for progress. The migration itself runs store.RunMigration
// — the exact same code path the metapi-migrate CLI uses — in a goroutine,
// streaming its progress lines into the task's Logs slice via
// AppendBackgroundTaskLog.
//
// The source is the currently-running runtime database (cfg); the target is
// the dialect + connection string supplied in the request body. A saved
// runtime override (settings table) only takes effect after restart, so it
// is intentionally NOT used as the source here.
func (h *databaseHandler) migrate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Dialect          string `json:"dialect"`
		ConnectionString string `json:"connectionString"`
		Ssl              bool   `json:"ssl"`
		Overwrite        *bool  `json:"overwrite"`
	}
	if err := decodeJSONRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Overwrite defaults to true (matches the TS admin migration behaviour);
	// the admin UI sends an explicit flag so an operator can keep target data.
	overwrite := true
	if body.Overwrite != nil {
		overwrite = *body.Overwrite
	}

	targetDialect, ok := normalizeRuntimeDatabaseDialect(body.Dialect)
	if !ok {
		writeErrorCode(w, http.StatusBadRequest, ErrorCodeInvalidDatabaseType, "database type must be sqlite or postgres")
		return
	}

	targetURL := strings.TrimSpace(body.ConnectionString)
	if targetURL == "" {
		writeErrorCode(w, http.StatusBadRequest, ErrorCodeEmptyMigrationTarget, "target database connection string must not be empty")
		return
	}

	sourceURL, sourceDialect, err := resolveMigrationSource(h.cfg, config.Runtime())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Refuse to migrate onto the live runtime database: overwriting the DB
	// the server is currently using would be destructive mid-request.
	// errorCode pins the rejection for clients (the admin UI historically
	// substring-matched the message text; see docs/api.md registry).
	if sameMigrationTarget(sourceURL, sourceDialect, targetURL, targetDialect) {
		writeErrorCode(w, http.StatusBadRequest, ErrorCodeSameMigrationTarget, "target database is the same as the running database; migration aborted")
		return
	}

	// idCh decouples the runner from the StartBackgroundTask return value:
	// the goroutine launched inside StartBackgroundTask may begin executing
	// before the caller observes the returned task, so the runner reads the
	// task ID from this buffered channel once the handler has it in hand.
	idCh := make(chan string, 1)
	task, reused := StartBackgroundTask(BackgroundTaskStartOptions{
		Type:      databaseMigrationTaskType,
		Title:     databaseMigrationTaskTitle,
		DedupeKey: databaseMigrationTaskDedupeKey,
	}, func() (any, error) {
		taskID := <-idCh
		progress := &backgroundTaskLogWriter{taskID: taskID}
		AppendBackgroundTaskLog(taskID, "migration started: source="+describeMigrationEndpoint(sourceURL, sourceDialect)+
			" target="+describeMigrationEndpoint(targetURL, targetDialect))
		summary, runErr := store.RunMigration(store.RunMigrationOptions{
			FromPath:  sourceURL,
			ToURL:     targetURL,
			Overwrite: overwrite,
			Progress:  true,
			Verify:    false,
			LogWriter: progress,
		})
		if runErr != nil {
			return nil, runErr
		}
		return summary, nil
	})
	// Hand the task ID to the runner goroutine. Buffered (size 1) so this
	// never blocks even if the task was reused and no runner is listening.
	idCh <- task.ID
	_ = reused

	writeJSON(w, http.StatusAccepted, map[string]any{
		"success": true,
		"message": "database migration queued as a background task; poll progress by task ID",
		"taskId":  task.ID,
		"task":    task,
	})
}

const (
	databaseMigrationTaskType      = "database_migration"
	databaseMigrationTaskTitle     = "Database migration"
	databaseMigrationTaskDedupeKey = "database_migration"
)

// resolveMigrationSource returns the live runtime database as the migration
// source. A saved runtime override (settings table) only takes effect after a
// restart, so the active cfg is the authoritative source for a migration
// triggered from the admin UI.
func resolveMigrationSource(cfg *config.Config, rt *config.RuntimeSettings) (connection, dialect string, err error) {
	if rt == nil {
		return "", "", fmt.Errorf("runtime config not loaded; unable to determine migration source")
	}
	resolved, ok := normalizeRuntimeDatabaseDialect(rt.DbType)
	if !ok {
		resolved = store.DialectSQLite
	}
	conn := strings.TrimSpace(rt.DbUrl)
	if resolved == store.DialectSQLite {
		dataDir := ""
		if cfg != nil {
			dataDir = cfg.DataDir
		}
		if dataDir == "" {
			dataDir = config.DefaultDataDir
		}
		conn = store.ResolveSQLitePath(conn, dataDir)
	}
	if conn == "" {
		return "", "", fmt.Errorf("current running database connection is empty; cannot be used as migration source")
	}
	return conn, resolved, nil
}

// sameMigrationTarget reports whether the source and target resolve to the
// same physical database. It is a safety guard against an in-place overwrite
// of the live runtime DB; the common SQLite→Postgres path has differing
// dialects and short-circuits to false.
func sameMigrationTarget(sourceURL, sourceDialect, targetURL, targetDialect string) bool {
	if sourceDialect != targetDialect {
		return false
	}
	if sourceDialect == store.DialectPostgres {
		// Compare host/port/db/query, ignoring credentials (maskConnectionString
		// redacts everything before the last @).
		return maskConnectionString(sourceURL) == maskConnectionString(targetURL)
	}
	return store.ResolveSQLitePath(sourceURL, ".") == store.ResolveSQLitePath(targetURL, ".")
}

// describeMigrationEndpoint returns a non-secret, human-readable label for a
// migration endpoint used in progress log lines.
func describeMigrationEndpoint(connection, dialect string) string {
	if dialect == store.DialectPostgres {
		return maskConnectionString(connection)
	}
	return connection
}

// backgroundTaskLogWriter adapts a background task ID into an io.Writer so
// store.RunMigration's progress lines are appended to the task's in-memory
// Logs slice and surface through /api/tasks/{id} polling. Multi-line writes
// (RunMigration emits several lines per Fprintf) are split into one log
// entry per logical line.
type backgroundTaskLogWriter struct {
	taskID string
}

func (w *backgroundTaskLogWriter) Write(p []byte) (int, error) {
	chunk := strings.TrimRight(string(p), "\n")
	if chunk == "" {
		return len(p), nil
	}
	for _, line := range strings.Split(chunk, "\n") {
		AppendBackgroundTaskLog(w.taskID, line)
	}
	return len(p), nil
}
