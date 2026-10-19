package admin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/sqlitetest"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

const quotaReservationBackupTable = "downstream_quota_reservations"

// TestBackupRestoreDropsInFlightQuotaReservations is the user-task regression:
// a type=all backup must not turn a source deployment's in-flight admission
// hold into quota used on the target. The hold is 30-minute TTL state owned by
// the process that admitted the request; after a restore no such request
// exists to settle it, so importing the row would reserve quota for work that
// never ran. The settled downstream_quota_usage ledger, by contrast, is real
// billing history and must survive.
func TestBackupRestoreDropsInFlightQuotaReservations(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		runBackupQuotaReservationRestore(t, sqlitetest.Open(t), sqlitetest.Open(t))
	})
	t.Run("postgres", func(t *testing.T) {
		source, target := openQuotaBackupPostgresSchemas(t)
		runBackupQuotaReservationRestore(t, source, target)
	})
}

func runBackupQuotaReservationRestore(t *testing.T, source, target *store.DB) {
	t.Helper()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	now := time.Now()
	quota := &store.DownstreamQuota{
		Requests: int64Ptr(2),
		Period:   store.DownstreamQuotaPeriod{Type: "all_time"},
	}
	policy, err := json.Marshal(store.DownstreamAccessPolicy{Quota: quota})
	if err != nil {
		t.Fatalf("marshal access policy: %v", err)
	}
	key := "sk-quota-backup-" + suffix
	keyID := insertBackupFixtureID(t, source,
		`INSERT INTO downstream_api_keys (name,key,supported_models,access_policy) VALUES (?,?,'["*"]',?)`,
		"quota-backup-"+suffix, key, string(policy))

	settledEvent := "settled-" + suffix
	execBackupFixture(t, source,
		`INSERT INTO downstream_quota_usage (key_id,event_key,occurred_at,requests,total_tokens,cost)
		 VALUES (?,?,?,?,?,?)`,
		keyID, settledEvent, now.UnixMilli(), 1, 7, 0.25)
	reservationID := "hold-" + suffix
	execBackupFixture(t, source,
		`INSERT INTO downstream_quota_reservations
		 (reservation_id,key_id,window_start,window_end,requests,tokens,cost,created_at)
		 VALUES (?,?,?,?,?,?,?,?)`,
		reservationID, keyID,
		now.Add(-time.Minute).UnixMilli(), now.Add(time.Hour).UnixMilli(),
		1, nil, nil, now.UnixMilli())

	exportBody := exportBackupAll(t, source)
	var exported struct {
		Metadata struct {
			ExcludedTables map[string]string `json:"excluded_tables"`
		} `json:"metadata"`
		Tables map[string]json.RawMessage `json:"tables"`
	}
	if err := json.Unmarshal(exportBody, &exported); err != nil {
		t.Fatalf("decode export: %v", err)
	}
	if _, carried := exported.Tables[quotaReservationBackupTable]; carried {
		t.Errorf("type=all export carried %s; importing the hold would reserve quota on the target for a request that never ran there",
			quotaReservationBackupTable)
	}
	if reason := strings.TrimSpace(exported.Metadata.ExcludedTables[quotaReservationBackupTable]); reason == "" {
		t.Errorf("metadata.excluded_tables[%s] is missing or empty; the backup must state this gap",
			quotaReservationBackupTable)
	}
	if _, carried := exported.Tables["downstream_quota_usage"]; !carried {
		t.Fatalf("type=all export dropped the settled downstream_quota_usage ledger")
	}

	status, body := postBackupJSON(t, target, "/api/settings/backup/import", exportBody)
	if status != http.StatusOK {
		t.Fatalf("import status = %d, want 200; body = %s", status, body)
	}

	if n := countBackupFixtureRows(t, target, `SELECT COUNT(*) FROM downstream_quota_reservations`); n != 0 {
		t.Errorf("target has %d restored in-flight reservation(s); the source hold became ghost quota", n)
	}
	if n := countBackupFixtureRows(t, target,
		`SELECT COUNT(*) FROM downstream_quota_usage WHERE key_id=? AND event_key=?`, keyID, settledEvent); n != 1 {
		t.Fatalf("target settled usage rows = %d, want the restored ledger row", n)
	}
	var tokens int64
	var cost float64
	if err := target.QueryRowx(target.Rebind(
		`SELECT total_tokens,cost FROM downstream_quota_usage WHERE key_id=? AND event_key=?`),
		keyID, settledEvent).Scan(&tokens, &cost); err != nil {
		t.Fatalf("read restored usage row: %v", err)
	}
	if tokens != 7 || cost != 0.25 {
		t.Fatalf("restored usage row = tokens %d cost %v, want 7 / 0.25", tokens, cost)
	}

	explanation := auth.ExplainManagedKeyQuota(target.DB, keyID, quota)
	if explanation.RequestsUsed != 1 || explanation.Blocker != "" {
		t.Fatalf("restored quota window reports requestsUsed=%d blocker=%q: the source in-flight reservation is being counted as used quota on the target",
			explanation.RequestsUsed, explanation.Blocker)
	}

	// The read-only explanation is not enough: drive the real managed-key
	// admission middleware. The restored usage leaves one of two request slots,
	// so the first probe must be admitted and settle its own hold (as the
	// completion path does), and the second must be denied with 429.
	previousDB := store.GetDB()
	store.OverrideDB(target)
	t.Cleanup(func() { store.OverrideDB(previousDB) })
	previousRuntime := config.RuntimeSafe()
	config.SetRuntime(&config.RuntimeSettings{})
	t.Cleanup(func() { config.SetRuntime(previousRuntime) })

	router := chi.NewRouter()
	router.Use(auth.ProxyAuth())
	router.Get("/probe", func(w http.ResponseWriter, r *http.Request) {
		pac := auth.GetProxyAuth(r.Context())
		if pac == nil || pac.KeyID == nil || pac.QuotaReservationID == "" {
			t.Errorf("admitted request has no managed quota reservation: %+v", pac)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		tokens := int64(1)
		auth.RecordManagedKeyQuotaUsage(*pac.KeyID, pac.QuotaReservationID, &tokens, nil)
		w.WriteHeader(http.StatusOK)
	})
	probe := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/probe", nil)
		req.Header.Set("Authorization", "Bearer "+key)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	if rec := probe(); rec.Code != http.StatusOK {
		t.Fatalf("first admission after restore = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	if rec := probe(); rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Body.String(), "insufficient_quota") {
		t.Fatalf("second admission after restore = %d body=%s, want 429 insufficient_quota", rec.Code, rec.Body.String())
	}
	if n := countBackupFixtureRows(t, target, `SELECT COUNT(*) FROM downstream_quota_reservations`); n != 0 {
		t.Fatalf("test-owned admission left %d reservation row(s), want 0 after settlement", n)
	}
}

// TestBackupImportSkipsLegacyQuotaReservations covers the upgrade path: a
// genuine export written before the table was excluded still carries the
// transient rows. The import must skip and report that table instead of making
// the whole backup unrestorable, and the preview must describe the same plan.
// It must not, however, relax the exclusion of other sensitive tables or start
// accepting hand-written payloads.
func TestBackupImportSkipsLegacyQuotaReservations(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		runLegacyQuotaReservationImport(t, sqlitetest.Open(t))
	})
	t.Run("postgres", func(t *testing.T) {
		target, err := openQuotaBackupPostgresSchema(t, "legacy_target")
		if err != nil {
			t.Fatalf("open target schema: %v", err)
		}
		runLegacyQuotaReservationImport(t, target)
	})
}

func runLegacyQuotaReservationImport(t *testing.T, target *store.DB) {
	t.Helper()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	now := time.Now()
	key := "sk-legacy-quota-" + suffix
	marker := "legacy_quota_marker_" + suffix
	payload := map[string]any{
		"metadata": map[string]any{
			"exported_at": "2026-10-19T00:00:00Z",
			"version":     "1.0",
		},
		"type": "all",
		"tables": map[string]any{
			"downstream_api_keys": []any{map[string]any{
				"id": 1, "name": "legacy-quota", "key": key, "supported_models": `["*"]`,
			}},
			"downstream_quota_usage": []any{map[string]any{
				"key_id": 1, "event_key": "legacy-settled", "occurred_at": now.UnixMilli(),
				"requests": 1, "total_tokens": 3, "cost": 0.5,
			}},
			quotaReservationBackupTable: []any{map[string]any{
				"reservation_id": "legacy-hold", "key_id": 1,
				"window_start": now.Add(-time.Minute).UnixMilli(),
				"window_end":   now.Add(time.Hour).UnixMilli(),
				"requests":     1, "tokens": 9, "cost": nil, "created_at": now.UnixMilli(),
			}},
			"settings": []any{map[string]any{"key": marker, "value": `"kept"`}},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal legacy payload: %v", err)
	}

	status, previewBody := postBackupJSON(t, target, "/api/settings/backup/import/preview", body)
	if status != http.StatusOK {
		t.Fatalf("preview status = %d, want 200; body = %s", status, previewBody)
	}
	var preview struct {
		Plan          map[string]backupImportPreview `json:"plan"`
		IgnoredTables []string                       `json:"ignoredTables"`
	}
	if err := json.Unmarshal(previewBody, &preview); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if !slices.Contains(preview.IgnoredTables, quotaReservationBackupTable) {
		t.Fatalf("preview ignoredTables = %v, want %s", preview.IgnoredTables, quotaReservationBackupTable)
	}
	if _, planned := preview.Plan[quotaReservationBackupTable]; planned {
		t.Fatalf("preview planned rows for %s, which the import must skip", quotaReservationBackupTable)
	}
	if got := preview.Plan["settings"].ToInsert; got != 1 {
		t.Fatalf("preview settings toInsert = %d, want 1; plan = %v", got, preview.Plan)
	}
	if n := countBackupFixtureRows(t, target, `SELECT COUNT(*) FROM settings WHERE key=?`, marker); n != 0 {
		t.Fatalf("preview wrote %d settings row(s); it must not write", n)
	}

	status, importBody := postBackupJSON(t, target, "/api/settings/backup/import", body)
	if status != http.StatusOK {
		t.Fatalf("import status = %d, want 200; body = %s", status, importBody)
	}
	var imported struct {
		Imported      map[string]float64 `json:"imported"`
		IgnoredTables []string           `json:"ignoredTables"`
	}
	if err := json.Unmarshal(importBody, &imported); err != nil {
		t.Fatalf("decode import response: %v", err)
	}
	if !slices.Contains(imported.IgnoredTables, quotaReservationBackupTable) {
		t.Fatalf("import ignoredTables = %v, want %s", imported.IgnoredTables, quotaReservationBackupTable)
	}
	if imported.Imported[quotaReservationBackupTable] != 0 {
		t.Fatalf("import reported %v rows for the skipped table", imported.Imported[quotaReservationBackupTable])
	}
	if imported.Imported["settings"] != 1 || imported.Imported["downstream_quota_usage"] != 1 {
		t.Fatalf("imported = %v, want one settings row and one settled usage row", imported.Imported)
	}
	if n := countBackupFixtureRows(t, target,
		`SELECT COUNT(*) FROM downstream_quota_reservations`); n != 0 {
		t.Fatalf("legacy import restored %d reservation row(s), want 0 (transient state must be skipped)", n)
	}
}

// TestBackupImportExcludedTableRejectionsStayClosed pins the other half of the
// exception: a genuine export may skip only the transient quota table. Every
// other excluded table keeps the reject-never-skip rule even with provenance,
// and a hand-written payload still cannot name the transient table.
func TestBackupImportExcludedTableRejectionsStayClosed(t *testing.T) {
	db := sqlitetest.Open(t)
	const settingsKey = "excluded_rejection_marker"

	cases := []struct {
		name    string
		payload string
	}{
		{
			name: "hand-written quota reservations payload",
			payload: fmt.Sprintf(
				`{"tables":{%q:[],"settings":[{"key":%q,"value":"\"x\""}]}}`,
				quotaReservationBackupTable, settingsKey),
		},
		{
			name: "exported admin sessions payload",
			payload: fmt.Sprintf(
				`{"metadata":{"exported_at":"2026-10-19T00:00:00Z","version":"1.0"},"tables":{"admin_sessions":[],"settings":[{"key":%q,"value":"\"x\""}]}}`,
				settingsKey),
		},
		{
			name: "exported audit log payload",
			payload: fmt.Sprintf(
				`{"metadata":{"exported_at":"2026-10-19T00:00:00Z","version":"1.0"},"tables":{"admin_audit_logs":[],"settings":[{"key":%q,"value":"\"x\""}]}}`,
				settingsKey),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, path := range []string{"/api/settings/backup/import/preview", "/api/settings/backup/import"} {
				status, body := postBackupJSON(t, db, path, []byte(tc.payload))
				if status != http.StatusBadRequest {
					t.Fatalf("%s status = %d, want 400; body = %s", path, status, body)
				}
				if !strings.Contains(string(body), "unknown table") {
					t.Fatalf("%s body = %s, want the existing unknown-table rejection", path, body)
				}
			}
			if n := countBackupFixtureRows(t, db, `SELECT COUNT(*) FROM settings WHERE key=?`, settingsKey); n != 0 {
				t.Fatalf("rejected payload wrote %d settings row(s); a failed import must not partially write", n)
			}
		})
	}
}

func exportBackupAll(t *testing.T, db *store.DB) []byte {
	t.Helper()
	handler := &backupHandler{db: db.DB}
	req := httptest.NewRequest(http.MethodGet, "/api/settings/backup/export?type=all", nil)
	rec := httptest.NewRecorder()
	handler.exportBackup(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("export status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	return rec.Body.Bytes()
}

func postBackupJSON(t *testing.T, db *store.DB, path string, body []byte) (int, []byte) {
	t.Helper()
	handler := &backupHandler{db: db.DB}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	rec := httptest.NewRecorder()
	switch path {
	case "/api/settings/backup/import":
		handler.importBackup(rec, req)
	case "/api/settings/backup/import/preview":
		handler.previewBackupImport(rec, req)
	default:
		t.Fatalf("unsupported backup path %s", path)
	}
	return rec.Code, rec.Body.Bytes()
}

// openQuotaBackupPostgresSchemas returns two independently migrated schemas in
// the disposable PostgreSQL database selected by PG_TEST_DSN: a source that
// holds the in-flight reservation and a target that receives the restore. It
// never touches the shared default schema, so it cannot collide with another
// package's fixtures.
func openQuotaBackupPostgresSchemas(t *testing.T) (*store.DB, *store.DB) {
	t.Helper()
	source, err := openQuotaBackupPostgresSchema(t, "source")
	if err != nil {
		t.Fatalf("open source schema: %v", err)
	}
	target, err := openQuotaBackupPostgresSchema(t, "target")
	if err != nil {
		t.Fatalf("open target schema: %v", err)
	}
	return source, target
}

func openQuotaBackupPostgresSchema(t *testing.T, role string) (*store.DB, error) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("PG_TEST_DSN"))
	if dsn == "" {
		t.Skip("PG_TEST_DSN not set; skipping PostgreSQL integration test")
	}
	base, err := url.Parse(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse PG_TEST_DSN: %w", err)
	}
	admin, err := store.Open(store.DialectPostgres, dsn, false)
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL: %w", err)
	}
	t.Cleanup(func() { _ = admin.Close() })

	schema := fmt.Sprintf("backup_quota_%s_%d", role, time.Now().UnixNano())
	if _, err := admin.Exec(`CREATE SCHEMA ` + schema); err != nil {
		return nil, fmt.Errorf("create schema %s: %w", schema, err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(`DROP SCHEMA ` + schema + ` CASCADE`) })

	u := *base
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := store.Open(store.DialectPostgres, u.String(), false)
	if err != nil {
		return nil, fmt.Errorf("open schema %s: %w", schema, err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		return nil, fmt.Errorf("migrate schema %s: %w", schema, err)
	}
	return db, nil
}
