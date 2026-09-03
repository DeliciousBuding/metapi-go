package admin

import (
	"encoding/base64"
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/internal/pgtest"
	backupsvc "github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/store"
)

func setupBackupTestDB(t *testing.T) *store.DB {
	t.Helper()

	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	return db
}

func allowPrivateWebdavTargetsForTest(t *testing.T) {
	t.Helper()
	old := allowPrivateWebdavTargets
	allowPrivateWebdavTargets = true
	t.Cleanup(func() { allowPrivateWebdavTargets = old })
}

func setBackupImportLimitsForTest(t *testing.T, maxRows, maxColumns, maxCellBytes int) {
	t.Helper()
	oldMaxRows := backupImportMaxRowsPerTable
	oldMaxColumns := backupImportMaxColumnsPerRow
	oldMaxCellBytes := backupImportMaxCellBytes
	backupImportMaxRowsPerTable = maxRows
	backupImportMaxColumnsPerRow = maxColumns
	backupImportMaxCellBytes = maxCellBytes
	t.Cleanup(func() {
		backupImportMaxRowsPerTable = oldMaxRows
		backupImportMaxColumnsPerRow = oldMaxColumns
		backupImportMaxCellBytes = oldMaxCellBytes
	})
}

func setBackupExportLimitsForTest(t *testing.T, maxRows int, maxCellBytes int, maxPayloadBytes int64) {
	t.Helper()
	oldMaxRows := backupsvc.MaxExportRowsPerTable
	oldMaxCellBytes := backupsvc.MaxExportCellBytes
	oldMaxPayloadBytes := backupsvc.MaxExportPayloadBytes
	backupsvc.MaxExportRowsPerTable = maxRows
	backupsvc.MaxExportCellBytes = maxCellBytes
	backupsvc.MaxExportPayloadBytes = maxPayloadBytes
	t.Cleanup(func() {
		backupsvc.MaxExportRowsPerTable = oldMaxRows
		backupsvc.MaxExportCellBytes = oldMaxCellBytes
		backupsvc.MaxExportPayloadBytes = oldMaxPayloadBytes
	})
}

func TestImportTableRowsWithConnRejectsUnknownColumns(t *testing.T) {
	db := setupBackupTestDB(t)

	_, _, err := importTableRowsWithConn(db.DB, "settings", []map[string]any{
		{
			"key":                      "safe-key",
			"value":                    "safe-value",
			"key) VALUES ('x','y') --": "malicious",
		},
	})
	if err == nil {
		t.Fatal("expected unknown column error")
	}
	if !strings.Contains(err.Error(), "unknown column") {
		t.Fatalf("error = %v, want unknown column", err)
	}

	var count int
	if err := db.Get(&count, "SELECT COUNT(*) FROM settings WHERE key = ?", "safe-key"); err != nil {
		t.Fatalf("count settings: %v", err)
	}
	if count != 0 {
		t.Fatalf("malformed import inserted %d rows, want 0", count)
	}
}

func TestImportTableRowsWithConnAllowsKnownColumns(t *testing.T) {
	db := setupBackupTestDB(t)

	n, skipped, err := importTableRowsWithConn(db.DB, "settings", []map[string]any{
		{
			"key":   "theme",
			"value": "dark",
		},
	})
	if err != nil {
		t.Fatalf("importTableRowsWithConn: %v", err)
	}
	if n != 1 {
		t.Fatalf("imported = %d, want 1", n)
	}
	if skipped != 0 {
		t.Fatalf("skipped = %d, want 0", skipped)
	}

	var value string
	if err := db.Get(&value, "SELECT value FROM settings WHERE key = ?", "theme"); err != nil {
		t.Fatalf("get imported setting: %v", err)
	}
	if value != "dark" {
		t.Fatalf("value = %q, want dark", value)
	}
}

func TestImportBackupUsesBackupPayloadLimitNotGenericAdminLimit(t *testing.T) {
	db := setupBackupTestDB(t)
	handler := &backupHandler{db: db.DB}

	oldAdminLimit := adminJSONBodyLimitBytes
	oldBackupLimit := backupWebdavImportMaxBytes
	adminJSONBodyLimitBytes = 8
	backupWebdavImportMaxBytes = 1024
	t.Cleanup(func() {
		adminJSONBodyLimitBytes = oldAdminLimit
		backupWebdavImportMaxBytes = oldBackupLimit
	})

	req := httptest.NewRequest(http.MethodPost, "/api/settings/backup/import", strings.NewReader(
		`{"tables":{"settings":[{"key":"theme","value":"\"dark\""}]}}`,
	))
	rec := httptest.NewRecorder()

	handler.importBackup(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var value string
	if err := db.Get(&value, "SELECT value FROM settings WHERE key = ?", "theme"); err != nil {
		t.Fatalf("get imported setting: %v", err)
	}
	if value != `"dark"` {
		t.Fatalf("value = %q, want dark JSON string", value)
	}
}

func TestImportBackupRejectsPayloadOverBackupLimit(t *testing.T) {
	db := setupBackupTestDB(t)
	handler := &backupHandler{db: db.DB}

	oldBackupLimit := backupWebdavImportMaxBytes
	backupWebdavImportMaxBytes = 8
	t.Cleanup(func() { backupWebdavImportMaxBytes = oldBackupLimit })

	req := httptest.NewRequest(http.MethodPost, "/api/settings/backup/import", strings.NewReader(
		`{"tables":{"settings":[{"key":"theme","value":"\"dark\""}]}}`,
	))
	rec := httptest.NewRecorder()

	handler.importBackup(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413; body=%s", rec.Code, rec.Body.String())
	}
	var count int
	if err := db.Get(&count, "SELECT COUNT(*) FROM settings"); err != nil {
		t.Fatalf("count settings: %v", err)
	}
	if count != 0 {
		t.Fatalf("settings rows inserted after oversized import, count=%d", count)
	}
}

func TestExportBackupRejectsPayloadOverLimit(t *testing.T) {
	setBackupExportLimitsForTest(t, 50_000, 4<<20, 32)
	db := setupBackupTestDB(t)
	handler := &backupHandler{db: db.DB}

	req := httptest.NewRequest(http.MethodGet, "/api/settings/backup/export?type=preferences", nil)
	rec := httptest.NewRecorder()

	handler.exportBackup(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413; body=%s", rec.Code, rec.Body.String())
	}
}

// F1: import plan preview reports toInsert/duplicates
// and writes nothing.
func TestPreviewBackupImportReportsPlanWithoutWriting(t *testing.T) {
	db := setupBackupTestDB(t)
	handler := &backupHandler{db: db.DB}
	now := time.Now().UTC().Format(time.RFC3339)

	// Seed one existing site (id 1) and one existing settings key.
	if _, err := db.Exec(
		"INSERT INTO sites (name, url, platform, status, created_at, updated_at) VALUES (?, ?, ?, 'active', ?, ?)",
		"existing-site", "https://existing.test", "new-api", now, now); err != nil {
		t.Fatalf("seed site: %v", err)
	}
	if _, err := db.Exec(
		"INSERT INTO settings (key, value) VALUES (?, ?)", "theme", `"dark"`); err != nil {
		t.Fatalf("seed setting: %v", err)
	}

	// Backup contains: site id 1 (duplicate), site id 2 (new), theme key
	// (duplicate), auth_token key (skipped runtime-local).
	payload := `{"tables":{
		"sites":[
			{"id":1,"name":"existing-site","url":"https://existing.test","platform":"new-api","status":"active","created_at":"` + now + `","updated_at":"` + now + `"},
			{"id":2,"name":"new-site","url":"https://new.test","platform":"new-api","status":"active","created_at":"` + now + `","updated_at":"` + now + `"}
		],
		"settings":[
			{"key":"theme","value":"\"dark\""},
			{"key":"auth_token","value":"\"secret\""}
		]
	}}`
	req := httptest.NewRequest(http.MethodPost, "/api/settings/backup/import/preview", strings.NewReader(payload))
	rec := httptest.NewRecorder()
	handler.previewBackupImport(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	plan, ok := body["plan"].(map[string]any)
	if !ok {
		t.Fatalf("plan missing: %v", body)
	}
	sites := plan["sites"].(map[string]any)
	if sites["rows"].(float64) != 2 || sites["toInsert"].(float64) != 1 || sites["duplicates"].(float64) != 1 {
		t.Fatalf("sites preview = %v, want rows=2 toInsert=1 duplicates=1", sites)
	}
	settings := plan["settings"].(map[string]any)
	if settings["rows"].(float64) != 2 || settings["toInsert"].(float64) != 0 ||
		settings["duplicates"].(float64) != 1 || settings["skippedRows"].(float64) != 1 {
		t.Fatalf("settings preview = %v, want rows=2 toInsert=0 duplicates=1 skipped=1", settings)
	}

	// Preview must not have written anything: still exactly 1 site, no new settings.
	var siteCount int
	if err := db.Get(&siteCount, "SELECT COUNT(*) FROM sites"); err != nil {
		t.Fatalf("count sites: %v", err)
	}
	if siteCount != 1 {
		t.Fatalf("preview wrote %d sites, want 1", siteCount)
	}
	var authToken string
	if err := db.Get(&authToken, "SELECT value FROM settings WHERE key = 'auth_token'"); err == nil {
		t.Fatalf("preview wrote auth_token (%q), want untouched", authToken)
	}
}

// Regression: the frontend api.importBackup wraps the pasted export as
// {"data": {"tables": ...}} — the handler must accept that shape (previously
// always 400'd because the top-level key was "data" not "tables").
func TestImportBackupAcceptsFrontendDataWrapper(t *testing.T) {
	db := setupBackupTestDB(t)
	handler := &backupHandler{db: db.DB}

	req := httptest.NewRequest(http.MethodPost, "/api/settings/backup/import", strings.NewReader(
		`{"data":{"tables":{"settings":[{"key":"theme","value":"\"dark\""}]}}}`,
	))
	rec := httptest.NewRecorder()
	handler.importBackup(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("data-wrapper status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var value string
	if err := db.Get(&value, "SELECT value FROM settings WHERE key = ?", "theme"); err != nil {
		t.Fatalf("get imported setting: %v", err)
	}
	if value != `"dark"` {
		t.Fatalf("value = %q, want dark JSON string", value)
	}
}

func TestWebdavConfigRoundTripMasksPassword(t *testing.T) {
	db := setupBackupTestDB(t)
	handler := &backupHandler{db: db.DB}

	req := httptest.NewRequest(http.MethodPut, "/api/settings/backup/webdav", strings.NewReader(`{
		"enabled": true,
		"fileUrl": "https://dav.example.com/backups/metapi.json",
		"username": "alice",
		"password": "secret-pass",
		"exportType": "accounts",
		"autoSyncEnabled": true,
		"autoSyncCron": "0 */6 * * *"
	}`))
	rec := httptest.NewRecorder()

	handler.saveWebdavConfig(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("save status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "secret-pass") {
		t.Fatalf("save response leaked password: %s", rec.Body.String())
	}

	var stored string
	if err := db.Get(&stored, "SELECT value FROM settings WHERE key = ?", "backup_webdav_config_v1"); err != nil {
		t.Fatalf("read stored webdav config: %v", err)
	}
	if !strings.Contains(stored, "secret-pass") {
		t.Fatalf("stored config did not preserve password: %s", stored)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/settings/backup/webdav", nil)
	getRec := httptest.NewRecorder()
	handler.getWebdavConfig(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200; body=%s", getRec.Code, getRec.Body.String())
	}
	if strings.Contains(getRec.Body.String(), "secret-pass") {
		t.Fatalf("get response leaked password: %s", getRec.Body.String())
	}

	var payload struct {
		Config struct {
			Enabled         bool   `json:"enabled"`
			FileURL         string `json:"fileUrl"`
			Username        string `json:"username"`
			Password        string `json:"password"`
			PasswordMasked  string `json:"passwordMasked"`
			HasPassword     bool   `json:"hasPassword"`
			ExportType      string `json:"exportType"`
			AutoSyncEnabled bool   `json:"autoSyncEnabled"`
			AutoSyncCron    string `json:"autoSyncCron"`
		} `json:"config"`
	}
	if err := json.NewDecoder(getRec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if !payload.Config.Enabled || payload.Config.FileURL != "https://dav.example.com/backups/metapi.json" || payload.Config.Username != "alice" {
		t.Fatalf("config = %+v, want saved fields", payload.Config)
	}
	if payload.Config.Password != "" {
		t.Fatalf("password field = %q, want empty", payload.Config.Password)
	}
	if !payload.Config.HasPassword || payload.Config.PasswordMasked == "" || payload.Config.PasswordMasked == "secret-pass" {
		t.Fatalf("password mask fields = has:%v masked:%q", payload.Config.HasPassword, payload.Config.PasswordMasked)
	}
	if payload.Config.ExportType != "accounts" || !payload.Config.AutoSyncEnabled || payload.Config.AutoSyncCron != "0 */6 * * *" {
		t.Fatalf("config = %+v, want saved sync fields", payload.Config)
	}
}

func TestExportToWebdavUploadsBackupPayload(t *testing.T) {
	allowPrivateWebdavTargetsForTest(t)

	db := setupBackupTestDB(t)
	handler := &backupHandler{db: db.DB}

	if _, err := db.Exec("INSERT INTO settings (key, value) VALUES (?, ?)", "theme", `"dark"`); err != nil {
		t.Fatalf("insert setting: %v", err)
	}

	var observedMethod string
	var observedAuth string
	var observedContentType string
	var observedPayload map[string]any
	webdav := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observedMethod = r.Method
		observedAuth = r.Header.Get("Authorization")
		observedContentType = r.Header.Get("Content-Type")
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read upload body: %v", err)
		}
		if err := json.Unmarshal(data, &observedPayload); err != nil {
			t.Fatalf("uploaded body is not JSON: %v; body=%s", err, string(data))
		}
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(webdav.Close)

	cfg := map[string]any{
		"enabled":         true,
		"fileUrl":         webdav.URL + "/backup.json",
		"username":        "alice",
		"password":        "secret-pass",
		"exportType":      "preferences",
		"autoSyncEnabled": false,
		"autoSyncCron":    "0 */6 * * *",
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if _, err := db.Exec("INSERT INTO settings (key, value) VALUES (?, ?)", "backup_webdav_config_v1", string(raw)); err != nil {
		t.Fatalf("insert webdav config: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/settings/backup/webdav/export", strings.NewReader(`{"type":"preferences"}`))
	rec := httptest.NewRecorder()

	handler.exportToWebdav(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("export status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if observedMethod != http.MethodPut {
		t.Fatalf("method = %q, want PUT", observedMethod)
	}
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("alice:secret-pass"))
	if observedAuth != wantAuth {
		t.Fatalf("Authorization = %q, want basic auth", observedAuth)
	}
	if !strings.Contains(observedContentType, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", observedContentType)
	}
	if observedPayload["type"] != "preferences" {
		t.Fatalf("uploaded type = %v, want preferences", observedPayload["type"])
	}
	tables, ok := observedPayload["tables"].(map[string]any)
	if !ok {
		t.Fatalf("uploaded tables = %#v, want object", observedPayload["tables"])
	}
	settingsRows, ok := tables["settings"].([]any)
	if !ok || len(settingsRows) == 0 {
		t.Fatalf("uploaded settings rows = %#v, want non-empty array", tables["settings"])
	}

	if strings.Contains(rec.Body.String(), "secret-pass") {
		t.Fatalf("export response leaked password: %s", rec.Body.String())
	}
}

func TestExportToWebdavRejectsOversizedPayloadBeforeUpload(t *testing.T) {
	allowPrivateWebdavTargetsForTest(t)
	setBackupExportLimitsForTest(t, 50_000, 4<<20, 32)

	db := setupBackupTestDB(t)
	handler := &backupHandler{db: db.DB}

	called := false
	webdav := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(webdav.Close)

	cfg := map[string]any{
		"enabled":      true,
		"fileUrl":      webdav.URL + "/backup.json",
		"exportType":   "preferences",
		"autoSyncCron": "0 */6 * * *",
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if _, err := db.Exec("INSERT INTO settings (key, value) VALUES (?, ?)", "backup_webdav_config_v1", string(raw)); err != nil {
		t.Fatalf("insert webdav config: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/settings/backup/webdav/export", strings.NewReader(`{"type":"preferences"}`))
	rec := httptest.NewRecorder()

	handler.exportToWebdav(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413; body=%s", rec.Code, rec.Body.String())
	}
	if called {
		t.Fatal("WebDAV server was called after export payload exceeded limit")
	}
}

func TestImportFromWebdavDownloadsAndImportsBackupPayload(t *testing.T) {
	allowPrivateWebdavTargetsForTest(t)

	db := setupBackupTestDB(t)
	handler := &backupHandler{db: db.DB}

	backup := map[string]any{
		"metadata": map[string]any{
			"version": "1.0",
		},
		"type": "preferences",
		"tables": map[string]any{
			"settings": []map[string]any{
				{"key": "theme", "value": `"dark"`},
			},
		},
	}
	backupRaw, err := json.Marshal(backup)
	if err != nil {
		t.Fatalf("marshal backup: %v", err)
	}

	var observedMethod string
	var observedAuth string
	webdav := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observedMethod = r.Method
		observedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(backupRaw)
	}))
	t.Cleanup(webdav.Close)

	cfg := map[string]any{
		"enabled":      true,
		"fileUrl":      webdav.URL + "/backup.json",
		"username":     "alice",
		"password":     "secret-pass",
		"exportType":   "preferences",
		"autoSyncCron": "0 */6 * * *",
	}
	cfgRaw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if _, err := db.Exec("INSERT INTO settings (key, value) VALUES (?, ?)", "backup_webdav_config_v1", string(cfgRaw)); err != nil {
		t.Fatalf("insert webdav config: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/settings/backup/webdav/import", nil)
	rec := httptest.NewRecorder()

	handler.importFromWebdav(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("import status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if observedMethod != http.MethodGet {
		t.Fatalf("method = %q, want GET", observedMethod)
	}
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("alice:secret-pass"))
	if observedAuth != wantAuth {
		t.Fatalf("Authorization = %q, want basic auth", observedAuth)
	}

	var value string
	if err := db.Get(&value, "SELECT value FROM settings WHERE key = ?", "theme"); err != nil {
		t.Fatalf("read imported setting: %v", err)
	}
	if value != `"dark"` {
		t.Fatalf("value = %q, want dark JSON string", value)
	}
	if strings.Contains(rec.Body.String(), "secret-pass") {
		t.Fatalf("import response leaked password: %s", rec.Body.String())
	}

	var payload struct {
		Imported map[string]int64 `json:"imported"`
		State    struct {
			LastSyncAt string  `json:"lastSyncAt"`
			LastError  *string `json:"lastError"`
		} `json:"state"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Imported["settings"] != 1 {
		t.Fatalf("imported = %#v, want settings=1", payload.Imported)
	}
	if payload.State.LastSyncAt == "" || payload.State.LastError != nil {
		t.Fatalf("state = %+v, want successful sync state", payload.State)
	}
}

func TestImportFromWebdavRejectsOversizedBackupPayload(t *testing.T) {
	allowPrivateWebdavTargetsForTest(t)

	db := setupBackupTestDB(t)
	handler := &backupHandler{db: db.DB}

	oldLimit := backupWebdavImportMaxBytes
	backupWebdavImportMaxBytes = 8
	t.Cleanup(func() { backupWebdavImportMaxBytes = oldLimit })

	webdav := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tables":{"settings":[]}}`))
	}))
	t.Cleanup(webdav.Close)

	cfg := map[string]any{
		"enabled":      true,
		"fileUrl":      webdav.URL + "/backup.json",
		"password":     "secret-pass",
		"exportType":   "preferences",
		"autoSyncCron": "0 */6 * * *",
	}
	cfgRaw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if _, err := db.Exec("INSERT INTO settings (key, value) VALUES (?, ?)", "backup_webdav_config_v1", string(cfgRaw)); err != nil {
		t.Fatalf("insert webdav config: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/settings/backup/webdav/import", nil)
	rec := httptest.NewRecorder()

	handler.importFromWebdav(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413; body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "secret-pass") {
		t.Fatalf("response leaked password: %s", rec.Body.String())
	}
}

func TestImportFromWebdavFailurePreservesLastSuccessfulSync(t *testing.T) {
	allowPrivateWebdavTargetsForTest(t)

	db := setupBackupTestDB(t)
	handler := &backupHandler{db: db.DB}

	webdav := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("remote failed for secret-pass"))
	}))
	t.Cleanup(webdav.Close)

	cfg := map[string]any{
		"enabled":      true,
		"fileUrl":      webdav.URL + "/backup.json",
		"password":     "secret-pass",
		"exportType":   "preferences",
		"autoSyncCron": "0 */6 * * *",
	}
	cfgRaw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if _, err := db.Exec("INSERT INTO settings (key, value) VALUES (?, ?)", "backup_webdav_config_v1", string(cfgRaw)); err != nil {
		t.Fatalf("insert webdav config: %v", err)
	}

	const previousSuccess = "2026-07-01T00:00:00Z"
	stateRaw := `{"lastSyncAt":"` + previousSuccess + `","lastAttemptAt":"2026-07-01T00:00:00Z","lastError":null}`
	if _, err := db.Exec("INSERT INTO settings (key, value) VALUES (?, ?)", "backup_webdav_state_v1", stateRaw); err != nil {
		t.Fatalf("insert webdav state: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/settings/backup/webdav/import", nil)
	rec := httptest.NewRecorder()

	handler.importFromWebdav(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502; body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "secret-pass") {
		t.Fatalf("response leaked password: %s", rec.Body.String())
	}

	var payload struct {
		State struct {
			LastSyncAt    string  `json:"lastSyncAt"`
			LastAttemptAt string  `json:"lastAttemptAt"`
			LastError     *string `json:"lastError"`
		} `json:"state"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.State.LastSyncAt != previousSuccess {
		t.Fatalf("lastSyncAt = %q, want previous success %q", payload.State.LastSyncAt, previousSuccess)
	}
	if payload.State.LastAttemptAt == "" || payload.State.LastAttemptAt == previousSuccess {
		t.Fatalf("lastAttemptAt = %q, want fresh attempt time", payload.State.LastAttemptAt)
	}
	if payload.State.LastError == nil || strings.Contains(*payload.State.LastError, "secret-pass") {
		t.Fatalf("lastError = %v, want sanitized failure", payload.State.LastError)
	}

	var savedRaw string
	if err := db.Get(&savedRaw, "SELECT value FROM settings WHERE key = ?", "backup_webdav_state_v1"); err != nil {
		t.Fatalf("read saved state: %v", err)
	}
	var saved struct {
		LastSyncAt    string  `json:"lastSyncAt"`
		LastAttemptAt string  `json:"lastAttemptAt"`
		LastError     *string `json:"lastError"`
	}
	if err := json.Unmarshal([]byte(savedRaw), &saved); err != nil {
		t.Fatalf("decode saved state: %v", err)
	}
	if saved.LastSyncAt != previousSuccess || saved.LastAttemptAt == "" || saved.LastError == nil {
		t.Fatalf("saved state = %+v, want preserved success plus failed attempt", saved)
	}
}

func TestWebdavFileURLRejectsPrivateTargets(t *testing.T) {
	tests := []string{
		"http://localhost/backup.json",
		"http://localhost./backup.json",
		"http://127.0.0.1/backup.json",
		"http://[::1]/backup.json",
		"http://10.0.0.5/backup.json",
		"http://172.16.0.5/backup.json",
		"http://192.168.1.5/backup.json",
		"http://169.254.169.254/latest/meta-data",
		"http://[fe80::1]/backup.json",
		"http://224.0.0.1/backup.json",
		"http://0.0.0.0/backup.json",
	}
	for _, raw := range tests {
		if isValidWebdavFileURL(raw) {
			t.Fatalf("isValidWebdavFileURL(%q) = true, want false", raw)
		}
	}

	if !isValidWebdavFileURL("https://webdav.example.com/backups/metapi.json") {
		t.Fatal("expected public HTTPS WebDAV URL to be valid")
	}
}

func TestWebdavHTTPClientRejectsPrivateRedirectTarget(t *testing.T) {
	client := newWebdavHTTPClient()
	if client.CheckRedirect == nil {
		t.Fatal("CheckRedirect is nil, want redirect validation")
	}

	from := httptest.NewRequest(http.MethodGet, "https://webdav.example.com/backup.json", nil)
	to := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/backup.json", nil)

	if err := client.CheckRedirect(to, []*http.Request{from}); err == nil {
		t.Fatal("redirect to loopback was allowed, want rejection")
	}
}

func TestWebdavHTTPTransportDoesNotUseEnvironmentProxy(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:9")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:9")

	transport := newWebdavHTTPTransport()
	if transport.Proxy != nil {
		t.Fatal("WebDAV transport uses an environment proxy hook, want direct dial with SSRF checks")
	}
}

func TestImportBackupTablesRollsBackWhenLaterTableFails(t *testing.T) {
	db := setupBackupTestDB(t)

	tables := map[string]json.RawMessage{
		"settings": json.RawMessage(`[{"key":"partial-import-sentinel","value":"\"should-not-persist\""}]`),
		"downstream_api_keys": json.RawMessage(
			`[{"id":"key-1","key_hash":"hash-1","name":"bad-key","unknown_column":"boom"}]`,
		),
	}

	if _, err := importBackupTables(db.DB, tables, false); err == nil {
		t.Fatalf("importBackupTables succeeded, want failure on invalid downstream_api_keys column")
	}

	var count int
	if err := db.Get(&count, "SELECT COUNT(*) FROM settings WHERE key = ?", "partial-import-sentinel"); err != nil {
		t.Fatalf("count partial setting: %v", err)
	}
	if count != 0 {
		t.Fatalf("partial setting persisted after failed import, count=%d", count)
	}
}

func TestImportBackupTablesRejectsUnknownTableKey(t *testing.T) {
	db := setupBackupTestDB(t)

	tables := map[string]json.RawMessage{
		"settings_typo": json.RawMessage(`[{"key":"theme","value":"\"dark\""}]`),
	}

	if _, err := importBackupTables(db.DB, tables, false); err == nil {
		t.Fatal("importBackupTables succeeded, want unknown table error")
	} else if !strings.Contains(err.Error(), "unknown table settings_typo") {
		t.Fatalf("error = %v, want unknown table", err)
	}

	var count int
	if err := db.Get(&count, "SELECT COUNT(*) FROM settings"); err != nil {
		t.Fatalf("count settings: %v", err)
	}
	if count != 0 {
		t.Fatalf("settings rows inserted after unknown table import, count=%d", count)
	}
}

func TestImportBackupTablesRejectsTooManyRowsBeforeInsert(t *testing.T) {
	setBackupImportLimitsForTest(t, 1, 128, 4<<20)
	db := setupBackupTestDB(t)

	tables := map[string]json.RawMessage{
		"settings": json.RawMessage(`[
			{"key":"theme","value":"\"dark\""},
			{"key":"language","value":"\"zh\""}
		]`),
	}

	if _, err := importBackupTables(db.DB, tables, false); err == nil {
		t.Fatal("importBackupTables succeeded, want row limit error")
	} else if !strings.Contains(err.Error(), "exceeds the max rows of 1") {
		t.Fatalf("error = %v, want row limit", err)
	}

	var count int
	if err := db.Get(&count, "SELECT COUNT(*) FROM settings"); err != nil {
		t.Fatalf("count settings: %v", err)
	}
	if count != 0 {
		t.Fatalf("settings rows inserted after row limit failure, count=%d", count)
	}
}

func TestImportBackupTablesRejectsTooManyColumnsBeforeInsert(t *testing.T) {
	setBackupImportLimitsForTest(t, 50_000, 1, 4<<20)
	db := setupBackupTestDB(t)

	tables := map[string]json.RawMessage{
		"settings": json.RawMessage(`[{"key":"theme","value":"\"dark\""}]`),
	}

	if _, err := importBackupTables(db.DB, tables, false); err == nil {
		t.Fatal("importBackupTables succeeded, want column limit error")
	} else if !strings.Contains(err.Error(), "exceeds limit 1") {
		t.Fatalf("error = %v, want column limit", err)
	}

	var count int
	if err := db.Get(&count, "SELECT COUNT(*) FROM settings"); err != nil {
		t.Fatalf("count settings: %v", err)
	}
	if count != 0 {
		t.Fatalf("settings rows inserted after column limit failure, count=%d", count)
	}
}

func TestImportBackupTablesRejectsOversizedCellBeforeInsert(t *testing.T) {
	setBackupImportLimitsForTest(t, 50_000, 128, 4)
	db := setupBackupTestDB(t)

	tables := map[string]json.RawMessage{
		"settings": json.RawMessage(`[{"key":"theme","value":"12345"}]`),
	}

	if _, err := importBackupTables(db.DB, tables, false); err == nil {
		t.Fatal("importBackupTables succeeded, want cell size error")
	} else if !strings.Contains(err.Error(), "exceeds limit 4 bytes") {
		t.Fatalf("error = %v, want cell size limit", err)
	}

	var count int
	if err := db.Get(&count, "SELECT COUNT(*) FROM settings"); err != nil {
		t.Fatalf("count settings: %v", err)
	}
	if count != 0 {
		t.Fatalf("settings rows inserted after cell size failure, count=%d", count)
	}
}

func TestImportBackupTablesSkipsRuntimeLocalSettings(t *testing.T) {
	db := setupBackupTestDB(t)

	tables := map[string]json.RawMessage{
		"settings": json.RawMessage(`[
			{"key":"theme","value":"\"dark\""},
			{"key":"auth_token","value":"\"remote-admin-token\""},
			{"key":"db_url","value":"\"postgres://remote.example/db\""},
			{"key":"backup_webdav_config_v1","value":"{\"enabled\":true,\"fileUrl\":\"https://evil.example/dump.json\"}"},
			{"key":"monitor_ldoh_cookie","value":"\"ld_auth_session=attacker\""},
			{"key":"proxy_token","value":"\"attacker-proxy-token\""},
			{"key":"account_credential_secret","value":"\"attacker-master-key\""}
		]`),
	}

	result, err := importBackupTables(db.DB, tables, false)
	if err != nil {
		t.Fatalf("importBackupTables: %v", err)
	}
	if result.imported["settings"] != 1 {
		t.Fatalf("imported settings = %d, want only non-local setting", result.imported["settings"])
	}
	if result.skippedSettings != 6 {
		t.Fatalf("skippedSettings = %d, want 6 runtime-local settings", result.skippedSettings)
	}

	var theme string
	if err := db.Get(&theme, "SELECT value FROM settings WHERE key = ?", "theme"); err != nil {
		t.Fatalf("theme setting was not imported: %v", err)
	}
	if theme != `"dark"` {
		t.Fatalf("theme = %q, want dark JSON string", theme)
	}

	for _, key := range []string{
		"auth_token",
		"db_url",
		"backup_webdav_config_v1",
		"monitor_ldoh_cookie",
		"proxy_token",
		"account_credential_secret",
	} {
		var count int
		if err := db.Get(&count, "SELECT COUNT(*) FROM settings WHERE key = ?", key); err != nil {
			t.Fatalf("count %s: %v", key, err)
		}
		if count != 0 {
			t.Fatalf("runtime-local setting %s was imported", key)
		}
	}
}

// Regression (P0): a malicious backup planting backup_webdav_config_v1 makes
// the backup-webdav scheduler export the whole database to an attacker
// endpoint after the next restart; monitor_ldoh_cookie pins the monitor
// session to an attacker credential. The import endpoint must count both in
// skippedSettings and never store them, while benign keys keep importing.
func TestImportBackupRejectsMaliciousRuntimeLocalSettings(t *testing.T) {
	db := setupBackupTestDB(t)
	handler := &backupHandler{db: db.DB}

	payload := `{"tables":{"settings":[
		{"key":"theme","value":"\"dark\""},
		{"key":"backup_webdav_config_v1","value":"{\"enabled\":true,\"fileUrl\":\"https://attacker.example/exfil.json\",\"username\":\"a\",\"password\":\"b\",\"exportType\":\"all\",\"autoSyncEnabled\":true,\"autoSyncCron\":\"0 * * * *\"}"},
		{"key":"monitor_ldoh_cookie","value":"\"ld_auth_session=attacker-session\""}
	]}}`

	req := httptest.NewRequest(http.MethodPost, "/api/settings/backup/import", strings.NewReader(payload))
	rec := httptest.NewRecorder()
	handler.importBackup(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if skipped, ok := body["skippedSettings"].(float64); !ok || skipped != 2 {
		t.Fatalf("skippedSettings = %v, want 2; body=%s", body["skippedSettings"], rec.Body.String())
	}
	imported, _ := body["imported"].(map[string]any)
	if settings, ok := imported["settings"].(float64); !ok || settings != 1 {
		t.Fatalf("imported[settings] = %v, want 1 (only the benign key)", imported)
	}

	for _, key := range []string{"backup_webdav_config_v1", "monitor_ldoh_cookie"} {
		var count int
		if err := db.Get(&count, "SELECT COUNT(*) FROM settings WHERE key = ?", key); err != nil {
			t.Fatalf("count %s: %v", key, err)
		}
		if count != 0 {
			t.Fatalf("runtime-local setting %s was imported", key)
		}
	}
	var theme string
	if err := db.Get(&theme, "SELECT value FROM settings WHERE key = ?", "theme"); err != nil {
		t.Fatalf("benign setting was not imported: %v", err)
	}
}

// Same regression through the TS v2.1 branch of the import endpoint.
func TestImportBackupTSV21RejectsMaliciousRuntimeLocalSettings(t *testing.T) {
	db := setupBackupTestDB(t)
	handler := &backupHandler{db: db.DB}

	payload := `{
		"version": "2.1",
		"timestamp": 1755678900000,
		"preferences": {"settings": [
			{"key": "theme", "value": "dark"},
			{"key": "backup_webdav_config_v1", "value": {"enabled": true, "fileUrl": "https://attacker.example/exfil.json"}},
			{"key": "monitor_ldoh_cookie", "value": "ld_auth_session=attacker-session"},
			{"key": "proxy_token", "value": "attacker-proxy-token"}
		]}
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/settings/backup/import", strings.NewReader(payload))
	rec := httptest.NewRecorder()
	handler.importBackup(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if skipped, ok := body["skippedSettings"].(float64); !ok || skipped != 3 {
		t.Fatalf("skippedSettings = %v, want 3; body=%s", body["skippedSettings"], rec.Body.String())
	}

	for _, key := range []string{"backup_webdav_config_v1", "monitor_ldoh_cookie", "proxy_token"} {
		var count int
		if err := db.Get(&count, "SELECT COUNT(*) FROM settings WHERE key = ?", key); err != nil {
			t.Fatalf("count %s: %v", key, err)
		}
		if count != 0 {
			t.Fatalf("runtime-local setting %s was imported", key)
		}
	}
	var theme string
	if err := db.Get(&theme, "SELECT value FROM settings WHERE key = ?", "theme"); err != nil {
		t.Fatalf("benign setting was not imported: %v", err)
	}
}

// Importing an untrusted backup is equivalent to accepting the downstream
// API keys it carries. The import capability is kept (migration scenario),
// but newly admitted keys must be listed by name — never by key value — in
// the import response and in the events audit trail.
func TestImportBackupReportsNewDownstreamApiKeys(t *testing.T) {
	db := setupBackupTestDB(t)
	handler := &backupHandler{db: db.DB}

	first := `{"tables":{"downstream_api_keys":[
		{"name":"客户端A","key":"sk-first-secret","enabled":1}
	]}}`
	req := httptest.NewRequest(http.MethodPost, "/api/settings/backup/import", strings.NewReader(first))
	rec := httptest.NewRecorder()
	handler.importBackup(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first import status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var firstBody map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &firstBody); err != nil {
		t.Fatalf("unmarshal first response: %v", err)
	}
	newKeys, _ := firstBody["newDownstreamApiKeys"].([]any)
	if len(newKeys) != 1 || newKeys[0] != "客户端A" {
		t.Fatalf("first import newDownstreamApiKeys = %v, want [客户端A]", firstBody["newDownstreamApiKeys"])
	}
	if strings.Contains(rec.Body.String(), "sk-first-secret") {
		t.Fatal("import response leaks the downstream key value")
	}

	// Second import: 客户端A is a duplicate, 客户端B is new.
	second := `{"tables":{"downstream_api_keys":[
		{"name":"客户端A","key":"sk-first-secret","enabled":1},
		{"name":"客户端B","key":"sk-second-secret","enabled":1}
	]}}`
	req = httptest.NewRequest(http.MethodPost, "/api/settings/backup/import", strings.NewReader(second))
	rec = httptest.NewRecorder()
	handler.importBackup(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("second import status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var secondBody map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &secondBody); err != nil {
		t.Fatalf("unmarshal second response: %v", err)
	}
	newKeys, _ = secondBody["newDownstreamApiKeys"].([]any)
	if len(newKeys) != 1 || newKeys[0] != "客户端B" {
		t.Fatalf("second import newDownstreamApiKeys = %v, want [客户端B] only", secondBody["newDownstreamApiKeys"])
	}
	if strings.Contains(rec.Body.String(), "sk-first-secret") || strings.Contains(rec.Body.String(), "sk-second-secret") {
		t.Fatal("import response leaks the downstream key value")
	}

	// The events audit trail lists names and never the key values.
	var events []struct {
		Message string `db:"message"`
	}
	if err := db.Select(&events, "SELECT message FROM events WHERE type = 'backup' ORDER BY id"); err != nil {
		t.Fatalf("query events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("backup events = %d, want 2 (one per import admitting keys)", len(events))
	}
	if !strings.Contains(events[0].Message, "客户端A") {
		t.Fatalf("first event message = %q, want it to list 客户端A", events[0].Message)
	}
	if !strings.Contains(events[1].Message, "客户端B") {
		t.Fatalf("second event message = %q, want it to list 客户端B", events[1].Message)
	}
	for _, ev := range events {
		if strings.Contains(ev.Message, "sk-first-secret") || strings.Contains(ev.Message, "sk-second-secret") {
			t.Fatalf("event message leaks downstream key value: %q", ev.Message)
		}
	}
}

// setupBackupPostgresTestDB opens the shared PostgreSQL test database behind
// PG_TEST_DSN (skips when unset), mirroring the other PG integration tests.
// The database is shared across packages (-p 1 in CI), so assertions must use
// unique suffixes and before/after deltas instead of absolute state.
func setupBackupPostgresTestDB(t *testing.T) *store.DB {
	t.Helper()

	dsn := strings.TrimSpace(os.Getenv("PG_TEST_DSN"))
	if dsn == "" {
		t.Skip("PG_TEST_DSN not set; skipping PostgreSQL integration test")
	}

	db, err := store.Open(store.DialectPostgres, dsn, false)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Empty the database before migrating so this test starts from the same
	// state CI gives it: a local loop reuses one PG database, and the
	// previous run's rows turn fixed-identity fixtures into duplicate-key
	// failures and whole-table counts into "want 3, got 4".
	pgtest.Reset(t, db.DB)

	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("migrate postgres: %v", err)
	}
	return db
}

// TestImportBackupRejectsMaliciousSettingsPostgres mirrors the SQLite
// regression through the pgx driver so the $N placeholder path of the import
// is covered too.
func TestImportBackupRejectsMaliciousSettingsPostgres(t *testing.T) {
	db := setupBackupPostgresTestDB(t)
	handler := &backupHandler{db: db.DB}

	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	benignKey := "theme_v6sec_" + suffix

	payload := `{"tables":{"settings":[
		{"key":"` + benignKey + `","value":"\"dark\""},
		{"key":"backup_webdav_config_v1","value":"{\"enabled\":true,\"fileUrl\":\"https://attacker.example/exfil.json\"}"},
		{"key":"monitor_ldoh_cookie","value":"\"ld_auth_session=attacker-session\""}
	]}}`

	var webdavBefore, cookieBefore int
	if err := db.Get(&webdavBefore, "SELECT COUNT(*) FROM settings WHERE key = 'backup_webdav_config_v1'"); err != nil {
		t.Fatalf("count webdav config before: %v", err)
	}
	if err := db.Get(&cookieBefore, "SELECT COUNT(*) FROM settings WHERE key = 'monitor_ldoh_cookie'"); err != nil {
		t.Fatalf("count monitor cookie before: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/settings/backup/import", strings.NewReader(payload))
	rec := httptest.NewRecorder()
	handler.importBackup(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if skipped, ok := body["skippedSettings"].(float64); !ok || skipped != 2 {
		t.Fatalf("skippedSettings = %v, want 2; body=%s", body["skippedSettings"], rec.Body.String())
	}

	for key, before := range map[string]int{"backup_webdav_config_v1": webdavBefore, "monitor_ldoh_cookie": cookieBefore} {
		var count int
		if err := db.Get(&count, "SELECT COUNT(*) FROM settings WHERE key = $1", key); err != nil {
			t.Fatalf("count %s: %v", key, err)
		}
		if count != before {
			t.Fatalf("runtime-local setting %s count changed from %d to %d via import", key, before, count)
		}
	}
	var theme string
	if err := db.Get(&theme, "SELECT value FROM settings WHERE key = $1", benignKey); err != nil {
		t.Fatalf("benign setting was not imported: %v", err)
	}
}

// TestImportBackupTSV21MaliciousSettingsPostgres mirrors the TS v2.1 branch
// through the pgx driver (Rebind / $N placeholders).
func TestImportBackupTSV21MaliciousSettingsPostgres(t *testing.T) {
	db := setupBackupPostgresTestDB(t)
	handler := &backupHandler{db: db.DB}

	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	payload := `{
		"version": "2.1",
		"timestamp": 1755678900000,
		"preferences": {"settings": [
			{"key": "theme_v6sec_` + suffix + `", "value": "dark"},
			{"key": "backup_webdav_config_v1", "value": {"enabled": true, "fileUrl": "https://attacker.example/exfil.json"}},
			{"key": "monitor_ldoh_cookie", "value": "ld_auth_session=attacker-session"}
		]}
	}`

	var webdavBefore, cookieBefore int
	if err := db.Get(&webdavBefore, "SELECT COUNT(*) FROM settings WHERE key = 'backup_webdav_config_v1'"); err != nil {
		t.Fatalf("count webdav config before: %v", err)
	}
	if err := db.Get(&cookieBefore, "SELECT COUNT(*) FROM settings WHERE key = 'monitor_ldoh_cookie'"); err != nil {
		t.Fatalf("count monitor cookie before: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/settings/backup/import", strings.NewReader(payload))
	rec := httptest.NewRecorder()
	handler.importBackup(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if skipped, ok := body["skippedSettings"].(float64); !ok || skipped != 2 {
		t.Fatalf("skippedSettings = %v, want 2; body=%s", body["skippedSettings"], rec.Body.String())
	}

	for key, before := range map[string]int{"backup_webdav_config_v1": webdavBefore, "monitor_ldoh_cookie": cookieBefore} {
		var count int
		if err := db.Get(&count, "SELECT COUNT(*) FROM settings WHERE key = $1", key); err != nil {
			t.Fatalf("count %s: %v", key, err)
		}
		if count != before {
			t.Fatalf("runtime-local setting %s count changed from %d to %d via import", key, before, count)
		}
	}
}

// TestImportBackupReportsNewDownstreamApiKeysPostgres exercises the
// downstream-key admission audit on PostgreSQL ($N placeholders in the
// exists-snapshot queries and the Rebind'd events insert).
func TestImportBackupReportsNewDownstreamApiKeysPostgres(t *testing.T) {
	db := setupBackupPostgresTestDB(t)
	handler := &backupHandler{db: db.DB}

	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	nameA := "v6sec-" + suffix + "-A"
	nameB := "v6sec-" + suffix + "-B"
	keyA := "sk-v6sec-" + suffix + "-a"
	keyB := "sk-v6sec-" + suffix + "-b"

	first := `{"tables":{"downstream_api_keys":[
		{"name":"` + nameA + `","key":"` + keyA + `","enabled":true}
	]}}`
	req := httptest.NewRequest(http.MethodPost, "/api/settings/backup/import", strings.NewReader(first))
	rec := httptest.NewRecorder()
	handler.importBackup(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first import status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var firstBody map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &firstBody); err != nil {
		t.Fatalf("unmarshal first response: %v", err)
	}
	newKeys, _ := firstBody["newDownstreamApiKeys"].([]any)
	if len(newKeys) != 1 || newKeys[0] != nameA {
		t.Fatalf("first import newDownstreamApiKeys = %v, want [%s]", firstBody["newDownstreamApiKeys"], nameA)
	}
	if strings.Contains(rec.Body.String(), keyA) {
		t.Fatal("import response leaks the downstream key value")
	}

	// Second import: A is a duplicate, B is new.
	second := `{"tables":{"downstream_api_keys":[
		{"name":"` + nameA + `","key":"` + keyA + `","enabled":true},
		{"name":"` + nameB + `","key":"` + keyB + `","enabled":true}
	]}}`
	req = httptest.NewRequest(http.MethodPost, "/api/settings/backup/import", strings.NewReader(second))
	rec = httptest.NewRecorder()
	handler.importBackup(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("second import status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var secondBody map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &secondBody); err != nil {
		t.Fatalf("unmarshal second response: %v", err)
	}
	newKeys, _ = secondBody["newDownstreamApiKeys"].([]any)
	if len(newKeys) != 1 || newKeys[0] != nameB {
		t.Fatalf("second import newDownstreamApiKeys = %v, want [%s] only", secondBody["newDownstreamApiKeys"], nameB)
	}
	if strings.Contains(rec.Body.String(), keyA) || strings.Contains(rec.Body.String(), keyB) {
		t.Fatal("import response leaks the downstream key value")
	}

	// Audit events list the admitted names and never the key values.
	var messages []string
	if err := db.Select(&messages, "SELECT message FROM events WHERE type = 'backup' AND message LIKE $1", "%"+suffix+"%"); err != nil {
		t.Fatalf("query events: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("backup audit events for suffix %s = %d, want 2", suffix, len(messages))
	}
	for _, msg := range messages {
		if strings.Contains(msg, keyA) || strings.Contains(msg, keyB) {
			t.Fatalf("event message leaks downstream key value: %q", msg)
		}
	}
}

func TestWebdavSaveRejectsInvalidAutoSyncCron(t *testing.T) {
	db := setupBackupTestDB(t)
	allowPrivateWebdavTargetsForTest(t)
	r := chi.NewRouter()
	RegisterBackupRoutes(r, db.DB)

	rec := doPutJSON(t, r, "/api/settings/backup/webdav", map[string]any{
		"enabled":         true,
		"fileUrl":         "http://dav.example.com/backup.json",
		"autoSyncEnabled": true,
		"autoSyncCron":    "not-a-cron",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400", rec.Code, rec.Body.String())
	}
}

func TestWebdavSaveAcceptsValidAutoSyncCron(t *testing.T) {
	db := setupBackupTestDB(t)
	allowPrivateWebdavTargetsForTest(t)
	r := chi.NewRouter()
	RegisterBackupRoutes(r, db.DB)

	rec := doPutJSON(t, r, "/api/settings/backup/webdav", map[string]any{
		"enabled":         true,
		"fileUrl":         "http://dav.example.com/backup.json",
		"autoSyncEnabled": true,
		"autoSyncCron":    "0 */3 * * *",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}
}

// tsV21EndpointBackup is a minimal TS (cita-777/metapi) v2.1 payload used to
// exercise the endpoint-level branch in importBackup / previewBackupImport.
const tsV21EndpointBackup = `{
  "version": "2.1",
  "timestamp": 1755678900000,
  "accounts": {
    "sites": [{"id": 1, "name": "站A", "url": "https://a.example.com", "platform": "new-api"}],
    "futureSection": {"x": 1}
  },
  "preferences": {
    "settings": [{"key": "theme", "value": "dark"}]
  }
}`

func TestImportTSV21Endpoint(t *testing.T) {
	db := setupBackupTestDB(t)
	r := chi.NewRouter()
	RegisterBackupRoutes(r, db.DB)

	rec := doPostJSON(t, r, "/api/settings/backup/import", json.RawMessage(tsV21EndpointBackup))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}

	var response struct {
		Success  bool             `json:"success"`
		Imported map[string]int64 `json:"imported"`
		Warnings []string         `json:"warnings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v (body=%s)", err, rec.Body.String())
	}
	if !response.Success {
		t.Fatalf("success = false, body=%s", rec.Body.String())
	}
	if response.Imported["sites"] != 1 || response.Imported["settings"] != 1 {
		t.Fatalf("imported = %#v, want sites=1 settings=1", response.Imported)
	}
	if len(response.Warnings) == 0 || !strings.Contains(strings.Join(response.Warnings, "\n"), "accounts.futureSection") {
		t.Fatalf("warnings = %q, want unknown section warning", response.Warnings)
	}

	var siteName string
	if err := db.Get(&siteName, "SELECT name FROM sites WHERE id = 1"); err != nil {
		t.Fatalf("read site: %v", err)
	}
	if siteName != "站A" {
		t.Fatalf("site name = %q, want 站A", siteName)
	}
	var themeValue string
	if err := db.Get(&themeValue, "SELECT value FROM settings WHERE key = ?", "theme"); err != nil {
		t.Fatalf("read setting: %v", err)
	}
	if themeValue != `"dark"` {
		t.Fatalf("settings[theme] = %q, want %q", themeValue, `"dark"`)
	}
}

func TestPreviewTSV21Endpoint(t *testing.T) {
	db := setupBackupTestDB(t)
	r := chi.NewRouter()
	RegisterBackupRoutes(r, db.DB)

	rec := doPostJSON(t, r, "/api/settings/backup/import/preview", json.RawMessage(tsV21EndpointBackup))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}

	var response struct {
		Success bool `json:"success"`
		Plan    map[string]struct {
			Rows     int64 `json:"rows"`
			ToInsert int64 `json:"toInsert"`
		} `json:"plan"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v (body=%s)", err, rec.Body.String())
	}
	if !response.Success {
		t.Fatalf("success = false, body=%s", rec.Body.String())
	}
	if plan := response.Plan["sites"]; plan.Rows != 1 || plan.ToInsert != 1 {
		t.Fatalf("sites plan = %+v, want rows=1 toInsert=1", plan)
	}

	var siteCount int
	if err := db.Get(&siteCount, "SELECT COUNT(*) FROM sites"); err != nil {
		t.Fatalf("count sites: %v", err)
	}
	if siteCount != 0 {
		t.Fatalf("preview wrote %d site rows, want 0", siteCount)
	}
}

func TestImportBackupTablesDropsForbiddenSiteURLs(t *testing.T) {
	db := setupBackupTestDB(t)

	tables := map[string]json.RawMessage{
		"sites": json.RawMessage(`[
			{"id": 1, "name": "Clean", "url": "https://clean.example.com", "platform": "new-api", "status": "active"},
			{"id": 2, "name": "Metadata", "url": "http://169.254.169.254/latest/meta-data/", "platform": "new-api", "status": "active"},
			{"id": 3, "name": "LinkLocalEndpoint", "url": "https://ok.example.com", "external_checkin_url": "http://169.254.169.254/checkin", "platform": "new-api", "status": "active"}
		]`),
	}

	result, err := importBackupTables(db.DB, tables, false)
	if err != nil {
		t.Fatalf("importBackupTables: %v", err)
	}
	if got := result.imported["sites"]; got != 1 {
		t.Fatalf("imported[sites] = %d, want 1 (only the clean site)", got)
	}
	if result.droppedForbiddenURLs != 2 {
		t.Fatalf("droppedForbiddenURLs = %d, want 2", result.droppedForbiddenURLs)
	}

	var count int
	if err := db.Get(&count, "SELECT COUNT(*) FROM sites"); err != nil {
		t.Fatalf("count sites: %v", err)
	}
	if count != 1 {
		t.Fatalf("sites rows = %d, want 1", count)
	}
}
