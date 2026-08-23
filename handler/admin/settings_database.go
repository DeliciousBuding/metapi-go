package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

// RegisterDatabaseRoutes registers all /api/settings/database routes.
func RegisterDatabaseRoutes(r chi.Router, db *sqlx.DB, cfg *config.Config) {
	handler := &databaseHandler{db: db, cfg: cfg}

	r.Get("/api/settings/database/runtime", handler.getRuntime)
	r.Put("/api/settings/database/runtime", handler.saveRuntime)
	r.Post("/api/settings/database/test-connection", handler.testConnection)
	r.Post("/api/settings/database/migrate", handler.migrate)
}

type databaseHandler struct {
	db  *sqlx.DB
	cfg *config.Config
}

type savedDatabaseConfig struct {
	Dialect                string `json:"dialect"`
	Connection             string `json:"connection"`
	ConnectionStringMasked string `json:"connectionStringMasked"`
	HasConnectionString    bool   `json:"hasConnectionString"`
	Ssl                    bool   `json:"ssl"`
	rawConnectionString    string
}

const runtimeDatabaseConnectionTestTimeoutSec = "5"

func normalizeRuntimeDatabaseDialect(dialect string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(dialect)) {
	case "sqlite":
		return "sqlite", true
	case "postgres", "postgresql":
		return "postgres", true
	default:
		return "", false
	}
}

func testRuntimeDatabaseConnection(dialect string, connectionString string, ssl bool) (string, error) {
	trimmed := strings.TrimSpace(connectionString)
	if trimmed == "" {
		return "", fmt.Errorf("connection string is required")
	}

	dsn := trimmed
	if dialect == store.DialectSQLite {
		dsn = store.ResolveSQLitePath(trimmed, ".")
	} else if dialect == store.DialectPostgres {
		dsn = applyPostgresTestConnectTimeout(trimmed)
	}

	sslMode := ""
	if ssl {
		sslMode = "require"
	}
	db, err := store.OpenWithPostgresSSLMode(dialect, dsn, sslMode)
	if err != nil {
		return "", err
	}
	if err := db.Close(); err != nil {
		return "", err
	}
	return maskConnectionString(trimmed), nil
}

func applyPostgresTestConnectTimeout(dsn string) string {
	trimmed := strings.TrimSpace(dsn)
	if strings.Contains(strings.ToLower(trimmed), "connect_timeout=") {
		return dsn
	}
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "postgres://") || strings.HasPrefix(lower, "postgresql://") {
		parsed, err := url.Parse(trimmed)
		if err == nil {
			q := parsed.Query()
			q.Set("connect_timeout", runtimeDatabaseConnectionTestTimeoutSec)
			parsed.RawQuery = q.Encode()
			return parsed.String()
		}
	}
	if trimmed == "" {
		return dsn
	}
	return strings.TrimRight(dsn, " ") + " connect_timeout=" + runtimeDatabaseConnectionTestTimeoutSec
}

func sanitizeConnectionError(err error, connectionString string) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	raw := strings.TrimSpace(connectionString)
	if raw != "" {
		message = strings.ReplaceAll(message, raw, maskConnectionString(raw))
	}
	if at := strings.LastIndex(raw, "@"); at > 0 {
		for i := at - 1; i >= 0; i-- {
			if raw[i] == ':' {
				password := raw[i+1 : at]
				if password != "" {
					message = strings.ReplaceAll(message, password, "***")
				}
				break
			}
		}
	}
	return message
}

func activeRuntimeDatabaseConfig(cfg *config.Config) map[string]any {
	if cfg == nil {
		return map[string]any{
			"dialect":    store.DialectSQLite,
			"connection": "(default sqlite path)",
			"ssl":        false,
		}
	}

	dialect, ok := normalizeRuntimeDatabaseDialect(cfg.DbType)
	if !ok {
		dialect = store.DialectSQLite
	}

	connection := strings.TrimSpace(cfg.DbUrl)
	if dialect == store.DialectSQLite {
		if connection == "" {
			connection = "(default sqlite path)"
		}
	} else {
		connection = maskConnectionString(connection)
	}

	return map[string]any{
		"dialect":    dialect,
		"connection": connection,
		"ssl":        cfg.PostgresSSLMode() != "",
	}
}

// GET /api/settings/database/runtime
func (h *databaseHandler) getRuntime(w http.ResponseWriter, r *http.Request) {
	saved, _ := loadSavedDatabaseConfig(h.db)

	writeJSON(w, http.StatusOK, map[string]any{
		"success":         true,
		"active":          activeRuntimeDatabaseConfig(h.cfg),
		"saved":           saved,
		"restartRequired": savedDatabaseRequiresRestart(saved, h.cfg),
	})
}

// PUT /api/settings/database/runtime
func (h *databaseHandler) saveRuntime(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Dialect          *string `json:"dialect"`
		ConnectionString *string `json:"connectionString"`
		Ssl              *bool   `json:"ssl"`
		Overwrite        *bool   `json:"overwrite"`
	}
	if err := decodeJSONRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if body.Dialect != nil {
		dialect, ok := normalizeRuntimeDatabaseDialect(*body.Dialect)
		if !ok {
			writeError(w, http.StatusBadRequest, "database type must be sqlite or postgres")
			return
		}
		upsertSettingDB(h.db, "db_type", dialect)
	}
	if body.ConnectionString != nil {
		upsertSettingDB(h.db, "db_url", strings.TrimSpace(*body.ConnectionString))
	}
	if body.Ssl != nil {
		upsertSettingDB(h.db, "db_ssl", *body.Ssl)
	}

	saved, _ := loadSavedDatabaseConfig(h.db)

	writeJSON(w, http.StatusOK, map[string]any{
		"success":         true,
		"message":         "database runtime configuration saved",
		"active":          activeRuntimeDatabaseConfig(h.cfg),
		"saved":           saved,
		"restartRequired": savedDatabaseRequiresRestart(saved, h.cfg),
	})
}

// POST /api/settings/database/test-connection
func (h *databaseHandler) testConnection(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Dialect          string `json:"dialect"`
		ConnectionString string `json:"connectionString"`
		Ssl              bool   `json:"ssl"`
	}
	if err := decodeJSONRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	dialect, ok := normalizeRuntimeDatabaseDialect(body.Dialect)
	if !ok {
		writeError(w, http.StatusBadRequest, "database type must be sqlite or postgres")
		return
	}

	maskedConnection, err := testRuntimeDatabaseConnection(dialect, body.ConnectionString, body.Ssl)
	if err != nil {
		writeError(w, http.StatusBadRequest, "database connection test failed: "+sanitizeConnectionError(err, body.ConnectionString))
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":    true,
		"message":    "target database connection succeeded",
		"dialect":    dialect,
		"connection": maskedConnection,
	})
}

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
		writeError(w, http.StatusBadRequest, "database type must be sqlite or postgres")
		return
	}

	targetURL := strings.TrimSpace(body.ConnectionString)
	if targetURL == "" {
		writeError(w, http.StatusBadRequest, "target database connection string must not be empty")
		return
	}

	sourceURL, sourceDialect, err := resolveMigrationSource(h.cfg)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Refuse to migrate onto the live runtime database: overwriting the DB
	// the server is currently using would be destructive mid-request.
	if sameMigrationTarget(sourceURL, sourceDialect, targetURL, targetDialect) {
		writeError(w, http.StatusBadRequest, "target database is the same as the running database; migration aborted")
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
func resolveMigrationSource(cfg *config.Config) (connection, dialect string, err error) {
	if cfg == nil {
		return "", "", fmt.Errorf("runtime config not loaded; unable to determine migration source")
	}
	resolved, ok := normalizeRuntimeDatabaseDialect(cfg.DbType)
	if !ok {
		resolved = store.DialectSQLite
	}
	conn := strings.TrimSpace(cfg.DbUrl)
	if resolved == store.DialectSQLite {
		dataDir := cfg.DataDir
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

func loadSavedDatabaseConfig(db *sqlx.DB) (*savedDatabaseConfig, error) {
	rows, err := queryRowsErr(db, "SELECT key, value FROM settings WHERE key IN ('db_type', 'db_url', 'db_ssl')")
	if err != nil {
		return nil, err
	}
	values := map[string]any{}
	for _, row := range rows {
		key, _ := row["key"].(string)
		value, _ := row["value"].(string)
		if value == "" {
			continue
		}
		var parsed any
		if err := json.Unmarshal([]byte(value), &parsed); err == nil {
			values[key] = parsed
		}
	}

	dialect, _ := values["db_type"].(string)
	connection, _ := values["db_url"].(string)
	if dialect == "" || connection == "" {
		return nil, nil
	}
	ssl := parseRuntimeDatabaseSsl(values["db_ssl"])
	masked := maskConnectionString(connection)
	return &savedDatabaseConfig{
		Dialect:                dialect,
		Connection:             masked,
		ConnectionStringMasked: masked,
		HasConnectionString:    true,
		Ssl:                    ssl,
		rawConnectionString:    connection,
	}, nil
}

func savedDatabaseRequiresRestart(saved *savedDatabaseConfig, cfg *config.Config) bool {
	if saved == nil || cfg == nil {
		return false
	}
	activeDialect, ok := normalizeRuntimeDatabaseDialect(cfg.DbType)
	if !ok {
		activeDialect = store.DialectSQLite
	}
	if saved.Dialect != activeDialect {
		return true
	}
	if saved.Dialect == store.DialectSQLite {
		// SQLite connection strings appear in several equivalent spellings
		// (sqlite://, file://, bare path, default hub.db). Normalize both
		// sides so the same database does not demand a pointless restart.
		dataDir := cfg.DataDir
		if dataDir == "" {
			dataDir = config.DefaultDataDir
		}
		return store.ResolveSQLitePath(saved.rawConnectionString, dataDir) !=
			store.ResolveSQLitePath(cfg.DbUrl, dataDir)
	}
	if strings.TrimSpace(saved.rawConnectionString) != strings.TrimSpace(cfg.DbUrl) {
		return true
	}
	return saved.Ssl != (cfg.PostgresSSLMode() != "")
}

// parseRuntimeDatabaseSsl tolerates legacy string-encoded values ("true"/"1")
// in addition to the JSON bool written by the current save path.
func parseRuntimeDatabaseSsl(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(v))
		return err == nil && parsed
	case float64:
		return v != 0
	}
	return false
}

func maskConnectionString(conn string) string {
	if conn == "" {
		return ""
	}
	// Mask password in connection string
	// Simple approach: redact everything after @ for display
	if idx := strings.LastIndex(conn, "@"); idx >= 0 {
		return "****@" + conn[idx+1:]
	}
	return "****"
}
