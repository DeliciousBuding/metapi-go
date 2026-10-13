package admin

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// Request-attempt contract (route-decision D batch): every dispatch attempt
// persists its own proxy_logs row correlated by request_id with retry_count
// as the loop ordinal. The detail endpoint answers the request-level question
// — how many attempts, which channels, why each failed, what the client
// finally got — and honestly marks the persisted set partial when incomplete.

type requestAttemptsFixture struct {
	siteID    int64
	accountID int64
}

func seedRequestAttemptsSite(t *testing.T, db *store.DB) requestAttemptsFixture {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO sites (name, url, platform, status, created_at, updated_at)
		VALUES ('attempts-site', 'https://attempts.example.test', 'openai', 'active', ?, ?)`, now, now); err != nil {
		t.Fatalf("insert site: %v", err)
	}
	var siteID int64
	if err := db.Get(&siteID, "SELECT id FROM sites WHERE name = 'attempts-site'"); err != nil {
		t.Fatalf("site id: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO accounts (site_id, username, access_token, status, balance, checkin_enabled, created_at, updated_at)
		VALUES (?, 'attempts-user', 'sk-attempts', 'active', 0, FALSE, ?, ?)`, siteID, now, now); err != nil {
		t.Fatalf("insert account: %v", err)
	}
	var accountID int64
	if err := db.Get(&accountID, "SELECT id FROM accounts WHERE username = 'attempts-user'"); err != nil {
		t.Fatalf("account id: %v", err)
	}
	return requestAttemptsFixture{siteID: siteID, accountID: accountID}
}

func insertAttemptRow(t *testing.T, db *store.DB, requestID any, ordinal int, status string, httpStatus int, accountID int64, errText string) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	id, err := execInsertID(db.DB, `INSERT INTO proxy_logs
		(account_id, model_requested, status, http_status, retry_count, request_id, error_message, latency_ms, created_at)
		VALUES (?, 'gpt-6', ?, ?, ?, ?, ?, ?, ?)`,
		accountID, status, httpStatus, ordinal, requestID, errText, int64(100+ordinal), now)
	if err != nil {
		t.Fatalf("insert attempt row: %v", err)
	}
	return id
}

func getRequestAttempts(t *testing.T, r chi.Router, logID int64) map[string]any {
	t.Helper()
	out := doGet(t, r, "/api/stats/proxy-logs/"+itoa(logID))
	var body map[string]any
	if out.Code != 200 || json.Unmarshal(out.Body.Bytes(), &body) != nil {
		t.Fatalf("detail failed: code=%d body=%s", out.Code, out.Body.String())
	}
	ra, ok := body["requestAttempts"].(map[string]any)
	if !ok {
		t.Fatalf("requestAttempts missing or wrong type: %s", out.Body.String())
	}
	return ra
}

func TestProxyLogRequestAttempts_TwoFailuresOneSuccess(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			var db *store.DB
			var mux chi.Router
			if dialect == store.DialectPostgres {
				db, mux = setupStatsPostgresTest(t)
			} else {
				db, mux = setupStatsSQLiteTest(t)
			}
			fixture := seedRequestAttemptsSite(t, db)
			reqID := "req-" + dialect + "-three-attempts"
			first := insertAttemptRow(t, db, reqID, 0, "failed", 502, fixture.accountID, "upstream boom")
			insertAttemptRow(t, db, reqID, 1, "failed", 429, fixture.accountID, "rate limited")
			third := insertAttemptRow(t, db, reqID, 2, "success", 200, fixture.accountID, "")

			// The invariant: two failures + one success = one request, three
			// attempts; readable from any one of the three rows.
			for _, logID := range []int64{first, third} {
				ra := getRequestAttempts(t, mux, logID)
				if ra["requestId"] != reqID {
					t.Fatalf("requestId: got %#v want %s", ra["requestId"], reqID)
				}
				if ra["partial"] != false {
					t.Fatalf("partial: got %#v, reasons=%#v", ra["partial"], ra["partialReasons"])
				}
				attempts, ok := ra["attempts"].([]any)
				if !ok || len(attempts) != 3 {
					t.Fatalf("attempts: got %#v", ra["attempts"])
				}
				for i, raw := range attempts {
					attempt := raw.(map[string]any)
					if attempt["ordinal"] != float64(i) {
						t.Fatalf("attempt %d ordinal: got %#v", i, attempt["ordinal"])
					}
					if attempt["isCurrent"] != (attempt["id"] == float64(logID)) {
						t.Fatalf("attempt %d isCurrent wrong: %#v (viewing %d)", i, attempt["isCurrent"], logID)
					}
				}
				if attempts[0].(map[string]any)["siteName"] != "attempts-site" ||
					attempts[0].(map[string]any)["accountUsername"] != "attempts-user" {
					t.Fatalf("attempt identity missing: %#v", attempts[0])
				}
				terminal := ra["terminal"].(map[string]any)
				if terminal["status"] != "success" || terminal["logId"] != float64(third) {
					t.Fatalf("terminal: got %#v, want success on row %d", terminal, third)
				}
			}
			if got := getRequestAttempts(t, mux, first)["currentOrdinal"]; got != float64(0) {
				t.Fatalf("currentOrdinal: got %#v", got)
			}
		})
	}
}

func TestProxyLogRequestAttempts_OrdinalGapMarkedPartial(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			var db *store.DB
			var mux chi.Router
			if dialect == store.DialectPostgres {
				db, mux = setupStatsPostgresTest(t)
			} else {
				db, mux = setupStatsSQLiteTest(t)
			}
			fixture := seedRequestAttemptsSite(t, db)
			reqID := "req-" + dialect + "-gap"
			// Ordinal 1 is missing: the loop iteration was skipped (e.g. site
			// concurrency saturation) and deliberately left no row.
			first := insertAttemptRow(t, db, reqID, 0, "failed", 502, fixture.accountID, "boom")
			insertAttemptRow(t, db, reqID, 2, "failed", 502, fixture.accountID, "boom again")

			ra := getRequestAttempts(t, mux, first)
			if ra["partial"] != true {
				t.Fatalf("gap must be partial: %#v", ra)
			}
			reasons, _ := ra["partialReasons"].([]any)
			if len(reasons) != 1 || reasons[0] != "ordinalGap" {
				t.Fatalf("partialReasons: got %#v", ra["partialReasons"])
			}
			if got := len(ra["attempts"].([]any)); got != 2 {
				t.Fatalf("attempts count: got %d — gaps are reported, never back-filled", got)
			}
			if ra["terminal"].(map[string]any)["status"] != "failed" {
				t.Fatalf("terminal: got %#v", ra["terminal"])
			}
		})
	}
}

func TestProxyLogRequestAttempts_MissingRequestID(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			var db *store.DB
			var mux chi.Router
			if dialect == store.DialectPostgres {
				db, mux = setupStatsPostgresTest(t)
			} else {
				db, mux = setupStatsSQLiteTest(t)
			}
			fixture := seedRequestAttemptsSite(t, db)
			// Legacy row without request correlation: the sibling set cannot be
			// verified, so no attempts list — the row itself is the terminal.
			row := insertAttemptRow(t, db, nil, 0, "success", 200, fixture.accountID, "")

			ra := getRequestAttempts(t, mux, row)
			if ra["requestId"] != nil {
				t.Fatalf("requestId: got %#v", ra["requestId"])
			}
			if ra["partial"] != true {
				t.Fatalf("missing requestId must be partial: %#v", ra)
			}
			reasons, _ := ra["partialReasons"].([]any)
			if len(reasons) != 1 || reasons[0] != "requestIdMissing" {
				t.Fatalf("partialReasons: got %#v", ra["partialReasons"])
			}
			if got := len(ra["attempts"].([]any)); got != 0 {
				t.Fatalf("attempts must stay empty without correlation: got %d", got)
			}
			terminal := ra["terminal"].(map[string]any)
			if terminal["status"] != "success" || terminal["logId"] != float64(row) {
				t.Fatalf("terminal should be the row itself: %#v", terminal)
			}
		})
	}
}

func TestProxyLogRequestAttempts_IsolationAcrossRequests(t *testing.T) {
	db, mux := setupStatsSQLiteTest(t)
	fixture := seedRequestAttemptsSite(t, db)
	other := insertAttemptRow(t, db, "req-other", 0, "success", 200, fixture.accountID, "")
	mine := insertAttemptRow(t, db, "req-mine", 0, "failed", 500, fixture.accountID, "boom")
	_ = other

	ra := getRequestAttempts(t, mux, mine)
	if got := len(ra["attempts"].([]any)); got != 1 {
		t.Fatalf("sibling query leaked across requestIds: got %d attempts", got)
	}
}
