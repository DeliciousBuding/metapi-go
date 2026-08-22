package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestRuntimeDatabaseGetRuntimeReportsActivePostgres(t *testing.T) {
	db := setupBackupTestDB(t)
	cfg := config.Load(map[string]string{
		"DB_TYPE":    "postgresql",
		"DB_URL":     "postgres://user:secret-pass@example.invalid:5432/metapi?sslmode=require",
		"DB_SSLMODE": "verify-full",
	})
	handler := &databaseHandler{db: db.DB, cfg: cfg}

	req := httptest.NewRequest(http.MethodGet, "/api/settings/database/runtime", nil)
	rec := httptest.NewRecorder()

	handler.getRuntime(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "secret-pass") {
		t.Fatalf("response leaked postgres password: %s", rec.Body.String())
	}

	var payload struct {
		Success bool `json:"success"`
		Active  struct {
			Dialect    string `json:"dialect"`
			Connection string `json:"connection"`
			Ssl        bool   `json:"ssl"`
		} `json:"active"`
		RestartRequired bool `json:"restartRequired"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Success {
		t.Fatalf("success = false, want true")
	}
	if payload.Active.Dialect != "postgres" {
		t.Fatalf("active dialect = %q, want postgres", payload.Active.Dialect)
	}
	if !payload.Active.Ssl {
		t.Fatalf("active ssl = false, want true")
	}
	if strings.Contains(payload.Active.Connection, "secret-pass") || !strings.Contains(payload.Active.Connection, "example.invalid:5432") {
		t.Fatalf("active connection = %q, want masked postgres host", payload.Active.Connection)
	}
	if payload.RestartRequired {
		t.Fatalf("restartRequired = true, want false with no saved override")
	}
}

func TestRuntimeDatabaseSaveRuntimeKeepsActiveDatabaseSeparateFromSavedOverride(t *testing.T) {
	db := setupBackupTestDB(t)
	cfg := config.Load(map[string]string{
		"DB_TYPE": "sqlite",
	})
	handler := &databaseHandler{db: db.DB, cfg: cfg}

	req := httptest.NewRequest(http.MethodPut, "/api/settings/database/runtime", strings.NewReader(`{
		"dialect": "postgres",
		"connectionString": "postgres://user:future-pass@example.invalid:5432/metapi",
		"ssl": true
	}`))
	rec := httptest.NewRecorder()

	handler.saveRuntime(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "future-pass") {
		t.Fatalf("response leaked saved postgres password: %s", rec.Body.String())
	}

	var payload struct {
		Success bool `json:"success"`
		Active  struct {
			Dialect string `json:"dialect"`
		} `json:"active"`
		Saved struct {
			Dialect                string `json:"dialect"`
			Connection             string `json:"connection"`
			ConnectionStringMasked string `json:"connectionStringMasked"`
			HasConnectionString    bool   `json:"hasConnectionString"`
			Ssl                    bool   `json:"ssl"`
		} `json:"saved"`
		RestartRequired bool `json:"restartRequired"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Success {
		t.Fatalf("success = false, want true")
	}
	if payload.Active.Dialect != "sqlite" {
		t.Fatalf("active dialect = %q, want current sqlite until restart", payload.Active.Dialect)
	}
	if payload.Saved.Dialect != "postgres" {
		t.Fatalf("saved dialect = %q, want postgres", payload.Saved.Dialect)
	}
	if !payload.Saved.Ssl {
		t.Fatalf("saved ssl = false, want true")
	}
	if strings.Contains(payload.Saved.Connection, "future-pass") || !strings.Contains(payload.Saved.Connection, "example.invalid:5432") {
		t.Fatalf("saved connection = %q, want masked postgres host", payload.Saved.Connection)
	}
	if payload.Saved.ConnectionStringMasked != payload.Saved.Connection || !payload.Saved.HasConnectionString {
		t.Fatalf("saved connection metadata = %+v", payload.Saved)
	}
	if !payload.RestartRequired {
		t.Fatalf("restartRequired = false, want true after saved override")
	}
}

func TestRuntimeDatabaseRejectsUnsupportedDialect(t *testing.T) {
	db := setupBackupTestDB(t)
	handler := &databaseHandler{db: db.DB}

	cases := []struct {
		name   string
		method string
		path   string
		call   http.HandlerFunc
	}{
		{
			name:   "save runtime",
			method: http.MethodPut,
			path:   "/api/settings/database/runtime",
			call:   handler.saveRuntime,
		},
		{
			name:   "test connection",
			method: http.MethodPost,
			path:   "/api/settings/database/test-connection",
			call:   handler.testConnection,
		},
		{
			name:   "migrate",
			method: http.MethodPost,
			path:   "/api/settings/database/migrate",
			call:   handler.migrate,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{
				"dialect": "mysql",
				"connectionString": "mysql://user:pass@example.invalid:3306/metapi"
			}`))
			rec := httptest.NewRecorder()

			tc.call(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "sqlite") || !strings.Contains(rec.Body.String(), "postgres") {
				t.Fatalf("body = %s, want supported dialect message", rec.Body.String())
			}
		})
	}

	var count int
	if err := db.Get(&count, "SELECT COUNT(*) FROM settings WHERE key = ?", "db_type"); err != nil {
		t.Fatalf("count db_type: %v", err)
	}
	if count != 0 {
		t.Fatalf("unsupported dialect was persisted, db_type rows=%d", count)
	}
}

func TestRuntimeDatabaseNormalizesPostgresqlAlias(t *testing.T) {
	dialect, ok := normalizeRuntimeDatabaseDialect(" postgresql ")
	if !ok {
		t.Fatal("postgresql alias was rejected")
	}
	if dialect != "postgres" {
		t.Fatalf("dialect = %q, want postgres", dialect)
	}
}

func TestApplyPostgresTestConnectTimeout(t *testing.T) {
	withDefault := applyPostgresTestConnectTimeout("postgres://user:pass@example.invalid:5432/metapi?sslmode=disable")
	if !strings.Contains(withDefault, "connect_timeout=5") {
		t.Fatalf("dsn = %q, want default connect_timeout", withDefault)
	}
	if !strings.Contains(withDefault, "sslmode=disable") {
		t.Fatalf("dsn = %q, want existing query preserved", withDefault)
	}

	withExisting := applyPostgresTestConnectTimeout("postgres://user:pass@example.invalid/metapi?connect_timeout=2")
	if !strings.Contains(withExisting, "connect_timeout=2") || strings.Contains(withExisting, "connect_timeout=5") {
		t.Fatalf("dsn = %q, want existing connect_timeout preserved", withExisting)
	}

	keyword := applyPostgresTestConnectTimeout("host=example.invalid user=metapi dbname=metapi")
	if !strings.Contains(keyword, "connect_timeout=5") {
		t.Fatalf("keyword dsn = %q, want default connect_timeout", keyword)
	}
}

func TestRuntimeDatabaseTestConnectionSQLiteSuccess(t *testing.T) {
	db := setupBackupTestDB(t)
	handler := &databaseHandler{db: db.DB}
	target := filepath.Join(t.TempDir(), "target.db")

	req := httptest.NewRequest(http.MethodPost, "/api/settings/database/test-connection", strings.NewReader(`{
		"dialect": "sqlite",
		"connectionString": "`+strings.ReplaceAll(target, `\`, `\\`)+`"
	}`))
	rec := httptest.NewRecorder()

	handler.testConnection(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"dialect":"sqlite"`) {
		t.Fatalf("body = %s, want sqlite dialect", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"success":true`) {
		t.Fatalf("body = %s, want success", rec.Body.String())
	}
}

func TestRuntimeDatabaseTestConnectionPostgresFailureDoesNotLeakPassword(t *testing.T) {
	db := setupBackupTestDB(t)
	handler := &databaseHandler{db: db.DB}

	const dsn = "postgres://user:secret-pass@127.0.0.1:1/metapi?sslmode=disable&connect_timeout=1"
	req := httptest.NewRequest(http.MethodPost, "/api/settings/database/test-connection", strings.NewReader(`{
		"dialect": "postgres",
		"connectionString": "`+dsn+`"
	}`))
	rec := httptest.NewRecorder()

	handler.testConnection(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "secret-pass") || strings.Contains(rec.Body.String(), dsn) {
		t.Fatalf("response leaked connection secret: %s", rec.Body.String())
	}

	var payload struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.Contains(payload.Error, "数据库测试连接失败") {
		t.Fatalf("error = %q, want connection failure message", payload.Error)
	}
}

// TestRuntimeDatabaseMigrateQueuesBackgroundTask asserts the migrate handler
// returns 202 Accepted with a task ID instead of 501, and that the registered
// background task completes successfully for a real SQLite→SQLite copy. The
// migration logic itself is exercised by store.RunMigration round-trip tests;
// this test only verifies the handler wiring (202 + task ID + terminal state).
func TestRuntimeDatabaseMigrateQueuesBackgroundTask(t *testing.T) {
	resetBackgroundTasksForTests()
	t.Cleanup(func() { resetBackgroundTasksForTests() })

	// Source = a file-backed SQLite DB the runtime config points at. The
	// in-memory handler DB is only for the settings table and is not the
	// migration source. Seed one sites row so the copy has something to move.
	sourcePath := filepath.Join(t.TempDir(), "source.db")
	srcDB, err := store.Open(store.DialectSQLite, sourcePath, false)
	if err != nil {
		t.Fatalf("open source: %v", err)
	}
	if err := store.AutoMigrate(srcDB); err != nil {
		t.Fatalf("auto-migrate source: %v", err)
	}
	if _, err := srcDB.Exec(`INSERT INTO sites (name, url, platform) VALUES (?, ?, ?)`,
		"Migrate Handler Site", "https://migrate.example.com", "openai"); err != nil {
		t.Fatalf("seed source site: %v", err)
	}
	if err := srcDB.Close(); err != nil {
		t.Fatalf("close source: %v", err)
	}

	settingsDB := setupBackupTestDB(t)
	cfg := config.Load(map[string]string{
		"DB_TYPE": "sqlite",
		"DB_URL":  sourcePath,
	})
	handler := &databaseHandler{db: settingsDB.DB, cfg: cfg}

	targetPath := filepath.Join(t.TempDir(), "target.db")
	req := httptest.NewRequest(http.MethodPost, "/api/settings/database/migrate", strings.NewReader(`{
		"dialect": "sqlite",
		"connectionString": "`+strings.ReplaceAll(targetPath, `\`, `\\`)+`"
	}`))
	rec := httptest.NewRecorder()
	handler.migrate(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Success bool   `json:"success"`
		TaskID  string `json:"taskId"`
		Task    struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"task"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Success {
		t.Fatalf("success = false, want true; body=%s", rec.Body.String())
	}
	if payload.TaskID == "" {
		t.Fatalf("taskId missing; body=%s", rec.Body.String())
	}
	if payload.Task.ID != payload.TaskID {
		t.Fatalf("task.id = %q, want %q", payload.Task.ID, payload.TaskID)
	}
	if payload.Task.Type != "database_migration" {
		t.Fatalf("task.type = %q, want database_migration", payload.Task.Type)
	}

	// Poll the in-memory registry until the task reaches a terminal state.
	// The migration copies one row SQLite→SQLite; it should succeed quickly.
	deadline := time.Now().Add(5 * time.Second)
	var final *BackgroundTask
	for time.Now().Before(deadline) {
		task := getBackgroundTask(nil, payload.TaskID)
		if task != nil && (task.Status == BackgroundTaskSucceeded || task.Status == BackgroundTaskFailed) {
			final = task
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if final == nil {
		t.Fatalf("migrate task %s did not reach a terminal state within timeout", payload.TaskID)
	}
	if final.Status != BackgroundTaskSucceeded {
		t.Fatalf("migrate task status = %s, want succeeded; message=%q error=%v logs=%d",
			final.Status, final.Message, final.Error, len(final.Logs))
	}
	// Progress logging: the runner must have appended at least the "migration started"
	// start line plus RunMigration's structural output.
	if len(final.Logs) == 0 {
		t.Fatalf("migrate task produced no progress logs; message=%q", final.Message)
	}
	var hasStartLine bool
	for _, entry := range final.Logs {
		if strings.Contains(entry.Message, "migration started") {
			hasStartLine = true
			break
		}
	}
	if !hasStartLine {
		t.Fatalf("migrate task logs missing start line: %+v", final.Logs)
	}

	// Verify the target received the row.
	tgtDB, err := store.Open(store.DialectSQLite, targetPath, false)
	if err != nil {
		t.Fatalf("open target: %v", err)
	}
	defer tgtDB.Close()
	var siteCount int
	if err := tgtDB.QueryRow(`SELECT COUNT(*) FROM sites WHERE name = ?`, "Migrate Handler Site").Scan(&siteCount); err != nil {
		t.Fatalf("count migrated site: %v", err)
	}
	if siteCount != 1 {
		t.Fatalf("target site count = %d, want 1", siteCount)
	}
}

// TestRuntimeDatabaseMigrateRejectsSameTarget guards the in-place overwrite
// prevention: migrating onto the live runtime DB must 400, not enqueue a task.
func TestRuntimeDatabaseMigrateRejectsSameTarget(t *testing.T) {
	resetBackgroundTasksForTests()
	t.Cleanup(func() { resetBackgroundTasksForTests() })

	settingsDB := setupBackupTestDB(t)
	const sourcePath = "C:/data/hub.db"
	cfg := config.Load(map[string]string{
		"DB_TYPE": "sqlite",
		"DB_URL":  sourcePath,
	})
	handler := &databaseHandler{db: settingsDB.DB, cfg: cfg}

	// Target spells the same SQLite file with a sqlite:// prefix.
	req := httptest.NewRequest(http.MethodPost, "/api/settings/database/migrate", strings.NewReader(`{
		"dialect": "sqlite",
		"connectionString": "sqlite://`+strings.ReplaceAll(sourcePath, `\`, `\\`)+`"
	}`))
	rec := httptest.NewRecorder()
	handler.migrate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "相同") {
		t.Fatalf("body = %s, want same-target rejection message", rec.Body.String())
	}
}

// TestRuntimeDatabaseMigrateRejectsEmptyConnectionString asserts a missing
// target connection string 400s before any task is enqueued.
func TestRuntimeDatabaseMigrateRejectsEmptyConnectionString(t *testing.T) {
	resetBackgroundTasksForTests()
	t.Cleanup(func() { resetBackgroundTasksForTests() })

	settingsDB := setupBackupTestDB(t)
	cfg := config.Load(map[string]string{"DB_TYPE": "sqlite", "DB_URL": "C:/data/hub.db"})
	handler := &databaseHandler{db: settingsDB.DB, cfg: cfg}

	req := httptest.NewRequest(http.MethodPost, "/api/settings/database/migrate", strings.NewReader(`{
		"dialect": "sqlite",
		"connectionString": ""
	}`))
	rec := httptest.NewRecorder()
	handler.migrate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "连接字符串") {
		t.Fatalf("body = %s, want empty-connection-string rejection", rec.Body.String())
	}
}

func TestRuntimeDatabaseGetDoesNotRequireRestartWhenSavedMatchesActive(t *testing.T) {
	db := setupBackupTestDB(t)
	const dsn = "postgres://user:secret@example.invalid:5432/metapi"
	for key, value := range map[string]any{
		"db_type": "postgres",
		"db_url":  dsn,
		"db_ssl":  true,
	} {
		if err := upsertSettingDB(db.DB, key, value); err != nil {
			t.Fatalf("save %s: %v", key, err)
		}
	}
	cfg := config.Load(map[string]string{
		"DB_TYPE":    "postgres",
		"DB_URL":     dsn,
		"DB_SSLMODE": "require",
	})
	handler := &databaseHandler{db: db.DB, cfg: cfg}
	req := httptest.NewRequest(http.MethodGet, "/api/settings/database/runtime", nil)
	rec := httptest.NewRecorder()
	handler.getRuntime(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		RestartRequired bool `json:"restartRequired"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.RestartRequired {
		t.Fatalf("restartRequired = true, want false for matching saved and active config")
	}
}

func TestRuntimeDatabaseGetDoesNotRequireRestartForEquivalentSQLitePath(t *testing.T) {
	db := setupBackupTestDB(t)
	const savedPath = "C:/data/hub.db"
	for key, value := range map[string]any{
		"db_type": "sqlite",
		"db_url":  savedPath,
		"db_ssl":  true,
	} {
		if err := upsertSettingDB(db.DB, key, value); err != nil {
			t.Fatalf("save %s: %v", key, err)
		}
	}
	// Active runtime spells the same SQLite file with the sqlite:// prefix.
	cfg := config.Load(map[string]string{
		"DB_TYPE": "sqlite",
		"DB_URL":  "sqlite://" + savedPath,
	})
	handler := &databaseHandler{db: db.DB, cfg: cfg}
	req := httptest.NewRequest(http.MethodGet, "/api/settings/database/runtime", nil)
	rec := httptest.NewRecorder()
	handler.getRuntime(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		RestartRequired bool `json:"restartRequired"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.RestartRequired {
		t.Fatalf("restartRequired = true, want false for equivalent sqlite path spellings")
	}
}

func TestRuntimeDatabaseLegacyStringSSLDoesNotForceRestart(t *testing.T) {
	db := setupBackupTestDB(t)
	const dsn = "postgres://user:secret@example.invalid:5432/metapi"
	for key, value := range map[string]any{
		"db_type": "postgres",
		"db_url":  dsn,
		"db_ssl":  "true",
	} {
		if err := upsertSettingDB(db.DB, key, value); err != nil {
			t.Fatalf("save %s: %v", key, err)
		}
	}
	cfg := config.Load(map[string]string{
		"DB_TYPE":    "postgres",
		"DB_URL":     dsn,
		"DB_SSLMODE": "require",
	})
	handler := &databaseHandler{db: db.DB, cfg: cfg}
	req := httptest.NewRequest(http.MethodGet, "/api/settings/database/runtime", nil)
	rec := httptest.NewRecorder()
	handler.getRuntime(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		RestartRequired bool `json:"restartRequired"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.RestartRequired {
		t.Fatalf("restartRequired = true, want false for legacy string ssl matching active sslmode")
	}
}
