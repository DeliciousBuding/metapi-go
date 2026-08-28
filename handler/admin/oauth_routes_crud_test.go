package admin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// doPatchJSON issues a PATCH request with a JSON body. Mirrors doPostJSON /
// doPutJSON so the PATCH-based oauth connection endpoints have the same
// ergonomic helper used everywhere else in the admin test suite.
func doPatchJSON(t *testing.T, r chi.Router, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest("PATCH", path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// doPatchRaw issues a PATCH request with an arbitrary raw body. Used to send
// malformed payloads that json.Marshal could never produce.
func doPatchRaw(t *testing.T, r chi.Router, path, rawBody string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("PATCH", path, bytes.NewReader([]byte(rawBody)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// insertOAuthAccountUnique inserts an OAuth account under a fresh site whose
// (platform, url) pair is unique per call. The sites table enforces a UNIQUE
// constraint on (platform, url), so the shared insertOAuthAccount helper
// cannot be reused when a test needs more than one account.
func insertOAuthAccountUnique(t *testing.T, db *store.DB, provider, suffix string) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	siteURL := "https://chatgpt.com/backend-api/" + suffix
	res, err := db.Exec(
		`INSERT INTO sites (name, url, platform, status, use_system_proxy, is_pinned, global_weight, sort_order, created_at, updated_at)
		 VALUES (?, ?, ?, 'active', FALSE, FALSE, 1, 0, ?, ?)`,
		"Codex OAuth Test "+suffix, siteURL, provider, now, now,
	)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	siteID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("site id: %v", err)
	}

	accountKey := "acc-" + suffix
	email := "user" + suffix + "@example.com"
	extra := `{"oauth":{"provider":"` + provider + `","accountId":"` + accountKey + `","accountKey":"` + accountKey + `","email":"` + email + `","refreshToken":"rt-test"}}`
	res, err = db.Exec(
		`INSERT INTO accounts (site_id, username, access_token, checkin_enabled, status, oauth_provider, oauth_account_key, extra_config, is_pinned, sort_order, balance, balance_used, quota, value_score, created_at, updated_at)
		 VALUES (?, ?, ?, FALSE, 'active', ?, ?, ?, FALSE, 0, 0, 0, 0, 0, ?, ?)`,
		siteID, email, "at-test", provider, accountKey, extra, now, now,
	)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	accountID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("account id: %v", err)
	}
	return accountID
}

// ---- listConnections ----

func TestOAuthListConnections_EmptyDB(t *testing.T) {
	_, r := setupOAuthRoutesTest(t)

	resp := doGet(t, r, "/api/oauth/connections")
	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	connections, ok := body["connections"].([]any)
	if !ok {
		t.Fatalf("connections not an array: %#v", body["connections"])
	}
	if len(connections) != 0 {
		t.Fatalf("expected 0 connections on empty DB, got %d", len(connections))
	}
	if got := int64(body["total"].(float64)); got != 0 {
		t.Fatalf("total=%d, want 0", got)
	}
}

func TestOAuthListConnections_WithAccounts(t *testing.T) {
	db, r := setupOAuthRoutesTest(t)
	insertOAuthAccount(t, db, "codex")

	resp := doGet(t, r, "/api/oauth/connections")
	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	connections, ok := body["connections"].([]any)
	if !ok || len(connections) != 1 {
		t.Fatalf("expected 1 connection, got %#v", body["connections"])
	}
	if got := int64(body["total"].(float64)); got != 1 {
		t.Fatalf("total=%d, want 1", got)
	}

	item, ok := connections[0].(map[string]any)
	if !ok {
		t.Fatalf("connection item not an object: %#v", connections[0])
	}
	if item["provider"] != "codex" {
		t.Fatalf("provider=%v, want codex", item["provider"])
	}
}

func TestOAuthListConnections_Pagination(t *testing.T) {
	db, r := setupOAuthRoutesTest(t)
	// Two distinct OAuth accounts under unique sites so the UNIQUE(platform,url)
	// constraint is satisfied while limit/offset is observable.
	firstID := insertOAuthAccountUnique(t, db, "codex", "page-a")
	secondID := insertOAuthAccountUnique(t, db, "codex", "page-b")
	if firstID == secondID {
		t.Fatalf("insertOAuthAccountUnique returned duplicate ids: %d", firstID)
	}

	// Page 1 (limit=1, offset=0): service orders by a.id DESC, so the most
	// recently inserted account is returned while total stays 2.
	page1 := doGet(t, r, "/api/oauth/connections?limit=1&offset=0")
	if page1.Code != http.StatusOK {
		t.Fatalf("page1 status=%d body=%s", page1.Code, page1.Body.String())
	}
	var body1 map[string]any
	if err := json.Unmarshal(page1.Body.Bytes(), &body1); err != nil {
		t.Fatalf("decode page1: %v", err)
	}
	if got := int64(body1["total"].(float64)); got != 2 {
		t.Fatalf("page1 total=%d, want 2", got)
	}
	conns1, _ := body1["connections"].([]any)
	if len(conns1) != 1 {
		t.Fatalf("page1 expected 1 connection, got %d", len(conns1))
	}

	// Page 2 (limit=1, offset=1): the older account.
	page2 := doGet(t, r, "/api/oauth/connections?limit=1&offset=1")
	if page2.Code != http.StatusOK {
		t.Fatalf("page2 status=%d body=%s", page2.Code, page2.Body.String())
	}
	var body2 map[string]any
	if err := json.Unmarshal(page2.Body.Bytes(), &body2); err != nil {
		t.Fatalf("decode page2: %v", err)
	}
	conns2, _ := body2["connections"].([]any)
	if len(conns2) != 1 {
		t.Fatalf("page2 expected 1 connection, got %d", len(conns2))
	}

	// The two pages must surface distinct accounts.
	id1, _ := conns1[0].(map[string]any)["accountId"].(float64)
	id2, _ := conns2[0].(map[string]any)["accountId"].(float64)
	if id1 == id2 {
		t.Fatalf("pagination returned the same account twice: %v", id1)
	}
}

// ---- updateProxy ----

func TestOAuthUpdateProxy_Success(t *testing.T) {
	db, r := setupOAuthRoutesTest(t)
	accountID := insertOAuthAccount(t, db, "codex")

	resp := doPatchJSON(t, r, fmt.Sprintf("/api/oauth/connections/%d/proxy", accountID), map[string]any{
		"proxyUrl": "http://127.0.0.1:7890",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["success"] != true {
		t.Fatalf("success=%v, want true", body["success"])
	}
	if got, _ := body["proxyUrl"].(string); got != "http://127.0.0.1:7890" {
		t.Fatalf("proxyUrl=%v, want http://127.0.0.1:7890", body["proxyUrl"])
	}
	if got := int64(body["accountId"].(float64)); got != accountID {
		t.Fatalf("accountId=%d, want %d", got, accountID)
	}
}

func TestOAuthUpdateProxy_NotFound(t *testing.T) {
	_, r := setupOAuthRoutesTest(t)

	resp := doPatchJSON(t, r, "/api/oauth/connections/999999/proxy", map[string]any{
		"proxyUrl": "http://127.0.0.1:7890",
	})
	if resp.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s, want 404", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if msg, _ := body["message"].(string); !strings.Contains(msg, "not found") {
		t.Fatalf("message=%q, want 'not found'", msg)
	}
}

func TestOAuthUpdateProxy_InvalidAccountID(t *testing.T) {
	_, r := setupOAuthRoutesTest(t)

	resp := doPatchJSON(t, r, "/api/oauth/connections/abc/proxy", map[string]any{
		"proxyUrl": "http://127.0.0.1:7890",
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s, want 400", resp.Code, resp.Body.String())
	}
}

func TestOAuthUpdateProxy_MalformedBody(t *testing.T) {
	db, r := setupOAuthRoutesTest(t)
	accountID := insertOAuthAccount(t, db, "codex")

	resp := doPatchRaw(t, r, fmt.Sprintf("/api/oauth/connections/%d/proxy", accountID), "{not-json")
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s, want 400", resp.Code, resp.Body.String())
	}
}

// ---- deleteConnection ----

func TestOAuthDeleteConnection_Success(t *testing.T) {
	db, r := setupOAuthRoutesTest(t)
	accountID := insertOAuthAccount(t, db, "codex")

	resp := doDelete(t, r, fmt.Sprintf("/api/oauth/connections/%d", accountID))
	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["success"] != true {
		t.Fatalf("success=%v, want true", body["success"])
	}

	// The deleted connection must no longer appear in the listing.
	listResp := doGet(t, r, "/api/oauth/connections")
	if listResp.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listResp.Code, listResp.Body.String())
	}
	var listBody map[string]any
	if err := json.Unmarshal(listResp.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if got := int64(listBody["total"].(float64)); got != 0 {
		t.Fatalf("total after delete=%d, want 0", got)
	}
}

func TestOAuthDeleteConnection_NotFound(t *testing.T) {
	_, r := setupOAuthRoutesTest(t)

	resp := doDelete(t, r, "/api/oauth/connections/999999")
	if resp.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s, want 404", resp.Code, resp.Body.String())
	}
}

func TestOAuthDeleteConnection_InvalidAccountID(t *testing.T) {
	_, r := setupOAuthRoutesTest(t)

	resp := doDelete(t, r, "/api/oauth/connections/not-a-number")
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s, want 400", resp.Code, resp.Body.String())
	}
}

// ---- refreshQuota ----

func TestOAuthRefreshQuota_Success(t *testing.T) {
	db, r := setupOAuthRoutesTest(t)
	accountID := insertOAuthAccount(t, db, "codex")

	resp := doPostJSON(t, r, fmt.Sprintf("/api/oauth/connections/%d/quota/refresh", accountID), map[string]any{})
	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["success"] != true {
		t.Fatalf("success=%v, want true", body["success"])
	}
	if body["quota"] == nil {
		t.Fatalf("quota snapshot should be present in response")
	}
}

func TestOAuthRefreshQuota_NotFound(t *testing.T) {
	_, r := setupOAuthRoutesTest(t)

	resp := doPostJSON(t, r, "/api/oauth/connections/999999/quota/refresh", map[string]any{})
	if resp.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s, want 404", resp.Code, resp.Body.String())
	}
}

func TestOAuthRefreshQuota_InvalidAccountID(t *testing.T) {
	_, r := setupOAuthRoutesTest(t)

	resp := doPostJSON(t, r, "/api/oauth/connections/abc/quota/refresh", map[string]any{})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s, want 400", resp.Code, resp.Body.String())
	}
}

// ---- refreshQuotaBatch ----

func TestOAuthRefreshQuotaBatch_Empty(t *testing.T) {
	_, r := setupOAuthRoutesTest(t)

	resp := doPostJSON(t, r, "/api/oauth/connections/quota/refresh-batch", map[string]any{
		"accountIds": []int64{},
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["success"] != true {
		t.Fatalf("success=%v, want true for empty batch", body["success"])
	}
	if got := int(body["refreshed"].(float64)); got != 0 {
		t.Fatalf("refreshed=%d, want 0", got)
	}
}

func TestOAuthRefreshQuotaBatch_MissingAccountID(t *testing.T) {
	db, r := setupOAuthRoutesTest(t)
	goodID := insertOAuthAccount(t, db, "codex")

	// Batch with one valid + one non-existent account: the valid one refreshes
	// (success) and the missing one records an error item, but the overall
	// request still returns 200 with the per-account breakdown.
	resp := doPostJSON(t, r, "/api/oauth/connections/quota/refresh-batch", map[string]any{
		"accountIds": []int64{goodID, 999999},
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	items, ok := body["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("expected 2 batch items, got %#v", body["items"])
	}
}
