package admin

// Wave 4 contract audit — list filters actually reach the SQL WHERE clause.
//
// Positive filter coverage already exists elsewhere:
//   - proxy-logs: TestStats_SQLiteProxyLogs*Filter* (status/search/client/
//     siteId/channelId/from/to/latency bounds, combined, NULL handling)
//   - checkin logs: TestCheckinLogsFiltersByStatusAndDateRange (incl. a
//     future from-bound that must return zero rows)
//   - audit-logs: TestAuditLogs_ListWithFilters (method + path substring)
//
// This file adds the gaps: account-tokens accountId scoping, downstream-keys
// summary search/status scoping, and explicit "filter matches nothing →
// empty result" negative cases so a filter silently degrading to "return
// everything" cannot pass.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// seedDownstreamKeyFixture inserts a downstream API key row and returns its
// id. Shared by the envelope, pagination-bounds and filter audits.
func seedDownstreamKeyFixture(t *testing.T, db *store.DB, name string, enabled bool) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(
		`INSERT INTO downstream_api_keys (name, key, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)`,
		name, "sk-test-"+name+"-secret-value", enabled, now, now,
	)
	if err != nil {
		t.Fatalf("insert downstream key %s: %v", name, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("downstream key id: %v", err)
	}
	return id
}

// ---- GET /api/account-tokens?accountId= ----

func TestFilters_AccountTokens_AccountIDScopesList(t *testing.T) {
	db, r := setupTokensTest(t)
	siteID, accountA := tokenFixture(t, db, r)
	createTokenFixture(t, db, accountA, "token-a", "sk-aaa-123456", "", true, true)

	// Second account on the same site with its own token.
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(
		`INSERT INTO accounts (site_id, username, access_token, status, checkin_enabled, created_at, updated_at)
		 VALUES (?, 'tokuser-b', 'sk-session-b', 'active', TRUE, ?, ?)`,
		siteID, now, now,
	)
	if err != nil {
		t.Fatalf("insert account B: %v", err)
	}
	accountB, _ := res.LastInsertId()
	createTokenFixture(t, db, accountB, "token-b", "sk-bbb-123456", "", true, true)

	// Filter scopes to account A only.
	filteredA := doGet(t, r, "/api/account-tokens?accountId="+itoa(accountA))
	if filteredA.Code != http.StatusOK {
		t.Fatalf("filtered list: %d %s", filteredA.Code, filteredA.Body.String())
	}
	var tokensA []map[string]any
	if err := json.Unmarshal(filteredA.Body.Bytes(), &tokensA); err != nil {
		t.Fatalf("decode filtered A: %v", err)
	}
	if len(tokensA) != 1 {
		t.Fatalf("accountId=A: tokens = %d, want 1 (filter must reach WHERE)", len(tokensA))
	}
	if got := int64(tokensA[0]["accountId"].(float64)); got != accountA {
		t.Fatalf("accountId=A: row accountId = %d, want %d", got, accountA)
	}

	filteredB := doGet(t, r, "/api/account-tokens?accountId="+itoa(accountB))
	var tokensB []map[string]any
	if err := json.Unmarshal(filteredB.Body.Bytes(), &tokensB); err != nil {
		t.Fatalf("decode filtered B: %v", err)
	}
	if len(tokensB) != 1 {
		t.Fatalf("accountId=B: tokens = %d, want 1", len(tokensB))
	}

	// Negative case: an accountId with no tokens returns an empty array,
	// not the full list (filter degenerating to no-op) and not an error.
	empty := doGet(t, r, "/api/account-tokens?accountId=999999")
	if empty.Code != http.StatusOK {
		t.Fatalf("empty filter: %d %s", empty.Code, empty.Body.String())
	}
	var tokensEmpty []map[string]any
	if err := json.Unmarshal(empty.Body.Bytes(), &tokensEmpty); err != nil {
		t.Fatalf("decode empty: %v; body=%s", err, empty.Body.String())
	}
	if len(tokensEmpty) != 0 {
		t.Fatalf("accountId=999999: tokens = %d, want 0 (negative filter case)", len(tokensEmpty))
	}

	// No filter returns both accounts' tokens.
	all := doGet(t, r, "/api/account-tokens")
	var tokensAll []map[string]any
	if err := json.Unmarshal(all.Body.Bytes(), &tokensAll); err != nil {
		t.Fatalf("decode all: %v", err)
	}
	if len(tokensAll) != 2 {
		t.Fatalf("unfiltered: tokens = %d, want 2", len(tokensAll))
	}
}

// ---- GET /api/downstream-keys/summary?search=&status= ----

func TestFilters_DownstreamKeys_SummarySearchAndStatusScoping(t *testing.T) {
	db, r := setupDownstreamKeysTest(t)
	seedDownstreamKeyFixture(t, db, "alpha-prod-key", true)
	seedDownstreamKeyFixture(t, db, "beta-staging-key", false)

	decodeItems := func(resp *httptest.ResponseRecorder) []map[string]any {
		t.Helper()
		if resp.Code != http.StatusOK {
			t.Fatalf("summary: %d %s", resp.Code, resp.Body.String())
		}
		var envelope struct {
			Items []map[string]any `json:"items"`
		}
		if err := json.Unmarshal(resp.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("decode summary: %v; body=%s", err, resp.Body.String())
		}
		return envelope.Items
	}

	// search narrows to the matching name.
	items := decodeItems(doGet(t, r, "/api/downstream-keys/summary?search=alpha"))
	if len(items) != 1 {
		t.Fatalf("search=alpha: items = %d, want 1 (filter must reach WHERE)", len(items))
	}

	// status narrows to the disabled key.
	items = decodeItems(doGet(t, r, "/api/downstream-keys/summary?status=disabled"))
	if len(items) != 1 {
		t.Fatalf("status=disabled: items = %d, want 1", len(items))
	}

	// Negative case: search matching nothing returns zero items, proving the
	// LIKE condition is applied rather than silently dropped.
	items = decodeItems(doGet(t, r, "/api/downstream-keys/summary?search=zzz-no-such-key"))
	if len(items) != 0 {
		t.Fatalf("search=zzz-no-such-key: items = %d, want 0 (negative filter case)", len(items))
	}

	// No filters returns both keys.
	items = decodeItems(doGet(t, r, "/api/downstream-keys/summary"))
	if len(items) != 2 {
		t.Fatalf("unfiltered summary: items = %d, want 2", len(items))
	}
}

// ---- GET /api/admin/audit-logs?method=&path= (negative cases) ----

func TestFilters_AuditLogs_NegativeFiltersReturnEmpty(t *testing.T) {
	db, _ := setupStatsSQLiteTest(t)
	now := time.Now().UTC().Format(time.RFC3339)
	for _, row := range []struct {
		method string
		path   string
	}{
		{"POST", "/api/sites"},
		{"DELETE", "/api/sites/7"},
	} {
		if _, err := db.Exec(`INSERT INTO admin_audit_logs (actor, method, path, status, request_id, remote_ip, created_at)
			VALUES ('aabbccdd', ?, ?, 200, 'req-x', '1.2.3.4', ?)`, row.method, row.path, now); err != nil {
			t.Fatalf("seed audit row: %v", err)
		}
	}
	r := chi.NewRouter()
	RegisterAuditLogsRoutes(r, db.DB)

	decode := func(resp *httptest.ResponseRecorder) (int, []any) {
		t.Helper()
		if resp.Code != http.StatusOK {
			t.Fatalf("audit-logs: %d %s", resp.Code, resp.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode audit-logs: %v; body=%s", err, resp.Body.String())
		}
		items, _ := body["items"].([]any)
		return int(body["total"].(float64)), items
	}

	// Positive control: method=POST finds the seeded row.
	total, items := decode(doGet(t, r, "/api/admin/audit-logs?method=POST"))
	if total != 1 || len(items) != 1 {
		t.Fatalf("method=POST: total=%d items=%d, want 1/1", total, len(items))
	}

	// Negative case: a method with no rows returns total=0 and empty items.
	total, items = decode(doGet(t, r, "/api/admin/audit-logs?method=PATCH"))
	if total != 0 || len(items) != 0 {
		t.Fatalf("method=PATCH: total=%d items=%d, want 0/0 (negative filter case)", total, len(items))
	}

	// Negative case: path substring matching nothing returns zero rows.
	total, items = decode(doGet(t, r, "/api/admin/audit-logs?path=zzz-no-such-endpoint"))
	if total != 0 || len(items) != 0 {
		t.Fatalf("path=zzz-no-such-endpoint: total=%d items=%d, want 0/0 (negative filter case)", total, len(items))
	}
}
