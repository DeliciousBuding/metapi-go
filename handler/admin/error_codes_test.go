package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
)

// Audit S4: machine-readable errorCode on admin 400 responses.
//
// These tests pin the errorCode contract so it cannot silently regress:
//
//  1. Envelope semantics — "error" stays the human-readable message and
//     "errorCode" is additive camelCase, present exactly on registered sites
//     and absent everywhere else (byte-identical legacy body).
//  2. Code pins — every code the frontend's message-substring matching
//     migrates onto (sameMigrationTarget first) is asserted with its exact
//     identifier and status on the real handler.
//  3. Regression guard — assertErrorCodeBody fails the moment a registered
//     site falls back to a message-only body, and the registry-stability
//     test fails on any drive-by rename of a public identifier.

// assertErrorCodeBody pins the unified error envelope for a coded site: the
// body must carry exactly wantCode plus a non-empty human-readable "error".
// A failure here means the site regressed to message-only (or drifted to an
// unregistered code), breaking the docs/api.md registry contract.
func assertErrorCodeBody(t *testing.T, raw []byte, wantCode string) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("error body is not valid JSON: %v raw=%s", err, string(raw))
	}
	if got, _ := body["errorCode"].(string); got != wantCode {
		t.Fatalf("errorCode = %q, want %q (site regressed or drifted; body: %s)", got, wantCode, string(raw))
	}
	if msg, _ := body["error"].(string); strings.TrimSpace(msg) == "" {
		t.Fatalf("human-readable error message missing alongside errorCode %q: %s", wantCode, string(raw))
	}
	if success, has := body["success"]; has && success == true {
		t.Fatalf("error body must not claim success=true: %s", string(raw))
	}
	return body
}

// ---- Registry stability: the literal values are a public contract ----

func TestErrorCodeRegistry_StableIdentifiers(t *testing.T) {
	// docs/api.md publishes these identifiers and API clients (soon the admin
	// UI) branch on them. Renaming one is a breaking change: update the
	// registry table and consumers deliberately, never as a side effect.
	pinned := map[string]string{
		"invalidId":            ErrorCodeInvalidID,
		"invalidDatabaseType":  ErrorCodeInvalidDatabaseType,
		"emptyMigrationTarget": ErrorCodeEmptyMigrationTarget,
		"sameMigrationTarget":  ErrorCodeSameMigrationTarget,
	}
	for want, got := range pinned {
		if got != want {
			t.Errorf("registered errorCode drifted: got %q, want %q", got, want)
		}
	}
}

// ---- Frontend-matched flagship: same-target migration rejection ----

// newMigrationHandlerForTest builds a databaseHandler whose migration source
// is a file-backed SQLite runtime DB (the settings DB backing the handler is
// a separate in-memory store).
func newMigrationHandlerForTest(t *testing.T, sourcePath string) *databaseHandler {
	t.Helper()
	settingsDB := setupBackupTestDB(t)
	cfg := config.Load(map[string]string{
		"DB_TYPE": "sqlite",
		"DB_URL":  sourcePath,
	})
	return &databaseHandler{db: settingsDB.DB, cfg: cfg}
}

func doPostMigration(t *testing.T, h *databaseHandler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/settings/database/migrate", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.migrate(rec, req)
	return rec
}

// TestErrorCode_MigrateSameTarget pins the rejection the admin UI historically
// detected by substring-matching the message text (message.includes("相同"));
// the UI migrates to errorCode == "sameMigrationTarget".
func TestErrorCode_MigrateSameTarget_PinnedCodeAndMessage(t *testing.T) {
	resetBackgroundTasksForTests()
	t.Cleanup(func() { resetBackgroundTasksForTests() })

	h := newMigrationHandlerForTest(t, "C:/data/hub.db")
	rec := doPostMigration(t, h, `{
		"dialect": "sqlite",
		"connectionString": "sqlite://C:/data/hub.db"
	}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	body := assertErrorCodeBody(t, rec.Body.Bytes(), ErrorCodeSameMigrationTarget)
	// The human message is still toasted verbatim by UIs that have not
	// adopted the code; it must not change shape under this contract.
	if msg := body["error"].(string); !strings.Contains(msg, "same as the running database") {
		t.Fatalf("human-readable message changed: %q", msg)
	}
}

func TestErrorCode_MigrateEmptyConnectionString_PinnedCode(t *testing.T) {
	resetBackgroundTasksForTests()
	t.Cleanup(func() { resetBackgroundTasksForTests() })

	h := newMigrationHandlerForTest(t, "C:/data/hub.db")
	rec := doPostMigration(t, h, `{"dialect": "sqlite", "connectionString": "   "}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	assertErrorCodeBody(t, rec.Body.Bytes(), ErrorCodeEmptyMigrationTarget)
}

func TestErrorCode_MigrateInvalidDialect_PinnedCode(t *testing.T) {
	resetBackgroundTasksForTests()
	t.Cleanup(func() { resetBackgroundTasksForTests() })

	h := newMigrationHandlerForTest(t, "C:/data/hub.db")
	rec := doPostMigration(t, h, `{"dialect": "mysql", "connectionString": "x"}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	assertErrorCodeBody(t, rec.Body.Bytes(), ErrorCodeInvalidDatabaseType)
}

// ---- pathID choke point: every {id} route rejection ----

func TestErrorCode_PathID_PinnedCode(t *testing.T) {
	_, r := setupSitesTest(t)

	cases := []struct {
		name   string
		method string
		path   string
	}{
		{"non-numeric id on update", http.MethodPut, "/api/sites/not-a-number"},
		{"zero id on update", http.MethodPut, "/api/sites/0"},
		{"non-numeric id on delete", http.MethodDelete, "/api/sites/abc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{"name":"x"}`))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
			}
			assertErrorCodeBody(t, rec.Body.Bytes(), ErrorCodeInvalidID)
		})
	}
}

// ---- Absent-semantics: uncoded sites keep the legacy body ----

// TestErrorCode_UncodedSite_OmitsField guards the non-breaking half of the
// contract: 400 sites without a registered code must NOT gain an errorCode
// key (no errorCode:null, no errorCode:""), so existing clients observe the
// exact pre-existing body.
func TestErrorCode_UncodedSite_OmitsField(t *testing.T) {
	_, r := setupSitesTest(t)
	resp := doPostJSON(t, r, "/api/sites", map[string]any{"name": "", "url": ""})

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v raw=%s", err, resp.Body.String())
	}
	if raw := resp.Body.String(); strings.Contains(raw, "errorCode") {
		t.Fatalf("uncoded site must not emit errorCode, got body %s", raw)
	}
	if msg, _ := body["error"].(string); strings.TrimSpace(msg) == "" {
		t.Fatalf("human-readable error message missing: %s", resp.Body.String())
	}
}
