package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// The backup table set is derived from the store schema registry minus
// store.BackupExcludedTables(). These tests pin the two halves of that
// contract end to end through the real export/import code path:
//
//   - a table that used to be missing (product_announcements,
//     announcement_dismissals, model_name_redirects, balance_history,
//     model_verify_history) survives export → wipe → import;
//   - a deliberately excluded table (admin_sessions, admin_audit_logs,
//     model_probe_results, catalog_sources) is not in the payload, is not
//     restored, and is refused when a caller hands it over anyway;
//   - a pre-change backup that carries none of the new tables still imports.

// newlyIncludedBackupTables are the registry tables the hand-copied
// service/backup.AllTables list dropped before the set was derived.
var newlyIncludedBackupTables = []string{
	"product_announcements",
	"announcement_dismissals",
	"model_name_redirects",
	"balance_history",
	"model_verify_history",
}

func insertBackupFixtureID(t *testing.T, db *store.DB, query string, args ...any) int64 {
	t.Helper()
	if db.Dialect == store.DialectPostgres {
		// pgx does not support LastInsertId.
		var id int64
		if err := db.QueryRow(db.Rebind(query+" RETURNING id"), args...).Scan(&id); err != nil {
			t.Fatalf("insert fixture: %v", err)
		}
		return id
	}
	res, err := db.Exec(db.Rebind(query), args...)
	if err != nil {
		t.Fatalf("insert fixture: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("fixture id: %v", err)
	}
	return id
}

func countBackupFixtureRows(t *testing.T, db *store.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.Get(&n, db.Rebind(query), args...); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

// seedBackupTableSetFixture writes one identifiable row into every table under
// test and returns the values the assertions look for after the round trip.
func seedBackupTableSetFixture(t *testing.T, db *store.DB) map[string]string {
	t.Helper()
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	now := time.Now().UTC().Format(time.RFC3339)
	fx := map[string]string{"suffix": suffix, "now": now}

	siteID := insertBackupFixtureID(t, db,
		`INSERT INTO sites (name, url, platform, status, created_at, updated_at) VALUES (?, ?, 'sub2api', 'active', ?, ?)`,
		"backup-roundtrip-"+suffix, "https://backup-roundtrip-"+suffix+".example.com", now, now)
	accountID := insertBackupFixtureID(t, db,
		`INSERT INTO accounts (site_id, username, access_token, status, is_pinned, sort_order,
		 checkin_enabled, extra_config, created_at, updated_at)
		 VALUES (?, ?, 'sk-backup-roundtrip', 'active', FALSE, 0, TRUE, '{}', ?, ?)`,
		siteID, "backup-user-"+suffix, now, now)
	fx["siteID"] = strconv.FormatInt(siteID, 10)
	fx["accountID"] = strconv.FormatInt(accountID, 10)

	// Tables the hand-copied list dropped.
	fx["announcementTitle"] = "backup-roundtrip-banner-" + suffix
	announcementID := insertBackupFixtureID(t, db,
		`INSERT INTO product_announcements (title, message, severity, link, enabled, created_at, updated_at)
		 VALUES (?, 'restored banner', 'info', NULL, TRUE, ?, ?)`, fx["announcementTitle"], now, now)
	execBackupFixture(t, db,
		`INSERT INTO announcement_dismissals (announcement_id, dismissed_at) VALUES (?, ?)`, announcementID, now)
	fx["redirectCanonical"] = "claude-roundtrip-" + suffix
	fx["redirectActual"] = "claude-roundtrip-20260903-" + suffix
	execBackupFixture(t, db,
		`INSERT INTO model_name_redirects (account_id, canonical, actual, source, last_seen_at, created_at, updated_at)
		 VALUES (?, ?, ?, 'manual', ?, ?, ?)`, accountID, fx["redirectCanonical"], fx["redirectActual"], now, now, now)
	fx["balanceDay"] = "2026-09-03"
	execBackupFixture(t, db,
		`INSERT INTO balance_history (account_id, balance, balance_used, quota, local_day, captured_at, created_at)
		 VALUES (?, 12.5, 3.25, 100, ?, ?, ?)`, accountID, fx["balanceDay"], now, now)
	fx["verifyBatch"] = "backup-roundtrip-batch-" + suffix
	execBackupFixture(t, db,
		`INSERT INTO model_verify_history (batch_id, model_name, channel_id, account_id, site_id, status,
		 latency_ms, http_status, error_text, created_at)
		 VALUES (?, ?, NULL, ?, ?, 'success', 12.5, 200, NULL, ?)`,
		fx["verifyBatch"], "gpt-roundtrip-"+suffix, accountID, siteID, now)

	// Tables the backup deliberately excludes: seeded so the assertions can
	// prove they are dropped rather than merely absent from an empty database.
	fx["sessionHash"] = "backup-roundtrip-session-" + suffix
	execBackupFixture(t, db,
		`INSERT INTO admin_sessions (token_hash, created_at, last_seen_at, expires_at, client_ip, user_agent)
		 VALUES (?, ?, ?, ?, '203.0.113.7', 'roundtrip')`, fx["sessionHash"], now, now, now)
	execBackupFixture(t, db,
		`INSERT INTO admin_audit_logs (actor, method, path, status, request_id, remote_ip, created_at)
		 VALUES ('roundtrip-actor', 'POST', '/api/roundtrip', 200, ?, '203.0.113.7', ?)`,
		"roundtrip-"+suffix, now)
	execBackupFixture(t, db,
		`INSERT INTO model_probe_results (channel_id, account_id, site_id, model_name, status, latency_ms,
		 http_status, error_text, created_at)
		 VALUES (NULL, ?, ?, ?, 'success', 12.5, 200, NULL, ?)`, accountID, siteID, "gpt-roundtrip-"+suffix, now)
	execBackupFixture(t, db,
		`INSERT INTO catalog_sources (name, url, enabled, type, sort_order, last_success_at, last_error,
		 last_count, last_attempt_at, created_at, updated_at)
		 VALUES (?, ?, TRUE, 'custom', 0, NULL, NULL, 0, NULL, ?, ?)`,
		"roundtrip-source-"+suffix, "https://catalog-roundtrip-"+suffix+".example.com/models.json", now, now)

	return fx
}

func execBackupFixture(t *testing.T, db *store.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(db.Rebind(query), args...); err != nil {
		t.Fatalf("seed fixture: %v", err)
	}
}

// runBackupTableSetRoundTrip exports everything, wipes the database in FK-safe
// order, imports the payload and asserts what came back — and what did not.
func runBackupTableSetRoundTrip(t *testing.T, db *store.DB) {
	t.Helper()
	fx := seedBackupTableSetFixture(t, db)

	payload, err := buildBackupPayload(db.DB, "all")
	if err != nil {
		t.Fatalf("buildBackupPayload: %v", err)
	}
	tables, ok := payload["tables"].(map[string]any)
	if !ok {
		t.Fatalf("payload tables = %#v, want object", payload["tables"])
	}
	for _, table := range newlyIncludedBackupTables {
		if _, carried := tables[table]; !carried {
			t.Fatalf("export payload omits %s, which the registry-derived backup set must carry", table)
		}
	}
	for table, reason := range store.BackupExcludedTables() {
		if _, carried := tables[table]; carried {
			t.Fatalf("export payload carries %s, which is excluded (%q)", table, reason)
		}
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	importBody, err := decodeBackupImportBodyFrom(raw)
	if err != nil {
		t.Fatalf("decode own export: %v", err)
	}

	// Wipe in the registry's FK-safe delete order: this is the "restored onto a
	// fresh instance" half of the round trip.
	for _, table := range store.ClearTableNames() {
		if _, err := db.Exec(fmt.Sprintf(`DELETE FROM %q`, table)); err != nil {
			t.Fatalf("wipe %s: %v", table, err)
		}
	}
	if n := countBackupFixtureRows(t, db, `SELECT COUNT(*) FROM product_announcements`); n != 0 {
		t.Fatalf("product_announcements still has %d rows after the wipe", n)
	}

	result, err := importBackupTables(db.DB, importBody)
	if err != nil {
		t.Fatalf("importBackupTables: %v", err)
	}
	for _, table := range newlyIncludedBackupTables {
		if result.imported[table] != 1 {
			t.Fatalf("imported[%s] = %d, want 1 (imported map: %v)", table, result.imported[table], result.imported)
		}
	}

	// The rows are back, with their content, not just their row count.
	if n := countBackupFixtureRows(t, db,
		`SELECT COUNT(*) FROM product_announcements WHERE title = ?`, fx["announcementTitle"]); n != 1 {
		t.Fatalf("product_announcements title %q rows = %d, want 1", fx["announcementTitle"], n)
	}
	if n := countBackupFixtureRows(t, db,
		`SELECT COUNT(*) FROM announcement_dismissals d JOIN product_announcements p ON p.id = d.announcement_id
		 WHERE p.title = ?`, fx["announcementTitle"]); n != 1 {
		t.Fatalf("announcement_dismissals rows for the restored banner = %d, want 1", n)
	}
	var actual string
	if err := db.Get(&actual, db.Rebind(
		`SELECT actual FROM model_name_redirects WHERE canonical = ?`), fx["redirectCanonical"]); err != nil {
		t.Fatalf("read restored model_name_redirects: %v", err)
	}
	if actual != fx["redirectActual"] {
		t.Fatalf("model_name_redirects actual = %q, want %q", actual, fx["redirectActual"])
	}
	if n := countBackupFixtureRows(t, db,
		`SELECT COUNT(*) FROM balance_history WHERE local_day = ? AND account_id = ?`,
		fx["balanceDay"], fx["accountID"]); n != 1 {
		t.Fatalf("balance_history rows = %d, want 1", n)
	}
	if n := countBackupFixtureRows(t, db,
		`SELECT COUNT(*) FROM model_verify_history WHERE batch_id = ?`, fx["verifyBatch"]); n != 1 {
		t.Fatalf("model_verify_history rows = %d, want 1", n)
	}

	// The deliberate exclusions stay excluded: a restore must not resurrect
	// session credentials, another deployment's audit trail, its probe
	// telemetry or its catalog fetch targets.
	for table := range store.BackupExcludedTables() {
		if n := countBackupFixtureRows(t, db, fmt.Sprintf(`SELECT COUNT(*) FROM %q`, table)); n != 0 {
			t.Fatalf("%s has %d rows after the round trip, want 0 (it is excluded from backups)", table, n)
		}
	}
	if n := countBackupFixtureRows(t, db,
		`SELECT COUNT(*) FROM admin_sessions WHERE token_hash = ?`, fx["sessionHash"]); n != 0 {
		t.Fatalf("the pre-wipe admin session survived the restore (%d rows), want 0", n)
	}
}

func TestBackupTableSetRoundTrip(t *testing.T) {
	runBackupTableSetRoundTrip(t, setupBackupTestDB(t))
}

func TestBackupTableSetRoundTripPostgres(t *testing.T) {
	runBackupTableSetRoundTrip(t, setupBackupPostgresTestDB(t))
}

// TestImportAcceptsBackupWithoutTheNewTables is the backward-compatibility
// gate: a payload written before the table set was derived carries none of the
// newly included tables, and a missing table must be skipped, not an error.
func TestImportAcceptsBackupWithoutTheNewTables(t *testing.T) {
	db := setupBackupTestDB(t)
	handler := &backupHandler{db: db.DB}

	legacy := map[string]any{"metadata": map[string]any{
		"exported_at": "2026-01-01T00:00:00Z",
		"version":     "1.0",
	}}
	legacyTables := map[string]any{}
	for _, table := range legacyBackupTables {
		legacyTables[table] = []any{}
	}
	legacyTables["settings"] = []any{map[string]any{"key": "legacy_roundtrip_key", "value": `"dark"`}}
	legacy["tables"] = legacyTables
	legacy["type"] = "all"
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal legacy payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/settings/backup/import", strings.NewReader(string(raw)))
	rec := httptest.NewRecorder()
	handler.importBackup(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	imported, ok := body["imported"].(map[string]any)
	if !ok {
		t.Fatalf("imported = %#v, want object; body = %s", body["imported"], rec.Body.String())
	}
	if got := imported["settings"]; got != float64(1) {
		t.Fatalf("imported.settings = %v, want 1; body = %s", got, rec.Body.String())
	}
	for _, table := range newlyIncludedBackupTables {
		if _, present := imported[table]; present {
			t.Fatalf("imported reports %s from a legacy payload that does not carry it", table)
		}
	}
}

// legacyBackupTables is the 28-table set the hand-copied list carried before it
// was derived from the registry; a backup written by that build has exactly
// these keys.
var legacyBackupTables = []string{
	"sites", "site_api_endpoints", "site_disabled_models", "accounts", "account_tokens",
	"checkin_logs", "model_availability", "token_model_availability", "token_routes",
	"route_group_sources", "oauth_route_units", "oauth_route_unit_members", "route_channels",
	"proxy_logs", "proxy_debug_traces", "proxy_debug_attempts", "proxy_video_tasks",
	"admin_background_tasks", "proxy_files", "settings", "admin_snapshots",
	"analytics_projection_checkpoints", "site_day_usage", "site_hour_usage", "model_day_usage",
	"downstream_api_keys", "site_announcements", "events",
}

// TestImportRefusesBackupExcludedTables pins the exclusion on the import side:
// a payload that hand-crafts an excluded table is rejected as unknown, so the
// exclusion cannot be bypassed by a caller who writes the JSON themselves.
func TestImportRefusesBackupExcludedTables(t *testing.T) {
	for table := range store.BackupExcludedTables() {
		t.Run(table, func(t *testing.T) {
			db := setupBackupTestDB(t)
			handler := &backupHandler{db: db.DB}

			payload := fmt.Sprintf(`{"tables":{%s:[]}}`, strconv.Quote(table))
			req := httptest.NewRequest(http.MethodPost, "/api/settings/backup/import", strings.NewReader(payload))
			rec := httptest.NewRecorder()
			handler.importBackup(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "unknown table "+table) {
				t.Fatalf("body = %s, want an unknown-table rejection naming %s", rec.Body.String(), table)
			}
		})
	}
}

// TestExportResponseDisclosesExcludedTables is the operator-visibility gate at
// the HTTP boundary: the export response states which registry tables the
// backup does not carry and why, without changing the pre-existing fields.
func TestExportResponseDisclosesExcludedTables(t *testing.T) {
	db := setupBackupTestDB(t)
	handler := &backupHandler{db: db.DB}

	req := httptest.NewRequest(http.MethodGet, "/api/settings/backup/export?type=all", nil)
	rec := httptest.NewRecorder()
	handler.exportBackup(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Metadata map[string]json.RawMessage `json:"metadata"`
		Type     string                     `json:"type"`
		Tables   map[string]json.RawMessage `json:"tables"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal export response: %v", err)
	}
	if payload.Type != "all" {
		t.Fatalf("type = %q, want all", payload.Type)
	}
	if _, ok := payload.Metadata["exported_at"]; !ok {
		t.Fatalf("metadata lost exported_at: %v", payload.Metadata)
	}
	var version string
	if err := json.Unmarshal(payload.Metadata["version"], &version); err != nil || version != "1.0" {
		t.Fatalf("metadata.version = %s (%v), want \"1.0\"", payload.Metadata["version"], err)
	}
	excludedRaw, ok := payload.Metadata["excluded_tables"]
	if !ok {
		t.Fatalf("metadata has no excluded_tables: %v", payload.Metadata)
	}
	var excluded map[string]string
	if err := json.Unmarshal(excludedRaw, &excluded); err != nil {
		t.Fatalf("excluded_tables = %s, want an object of table -> reason: %v", excludedRaw, err)
	}
	want := store.BackupExcludedTables()
	if len(excluded) != len(want) {
		t.Fatalf("excluded_tables has %d entries (%s), want the %d deliberate exclusions",
			len(excluded), excludedRaw, len(want))
	}
	for table, reason := range want {
		if excluded[table] != reason {
			t.Fatalf("excluded_tables[%s] = %q, want %q", table, excluded[table], reason)
		}
		if _, carried := payload.Tables[table]; carried {
			t.Fatalf("payload carries %s while metadata lists it as excluded", table)
		}
	}
	for _, table := range newlyIncludedBackupTables {
		if _, carried := payload.Tables[table]; !carried {
			t.Fatalf("export response omits %s", table)
		}
	}
}
