package admin

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// ---- listRoutes (GET /api/routes) pagination ----

// TestListRoutes_NoPaginationReturnsFullArray verifies backward compatibility:
// when ?page is absent the handler returns a bare JSON array of every route
// (the pre-pagination response shape) so existing frontend code is untouched.
func TestListRoutes_NoPaginationReturnsFullArray(t *testing.T) {
	db, r := setupTokenRoutesTest(t)
	now := time.Now().UTC().Format(time.RFC3339)
	seedRoutes(t, db, 3, now)

	resp := doGet(t, r, "/api/routes")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}

	// Backward-compat shape: a bare JSON array, NOT the {items,total,...} envelope.
	var arr []any
	if err := json.Unmarshal(resp.Body.Bytes(), &arr); err != nil {
		t.Fatalf("expected bare array, decode failed: %v; body=%s", err, resp.Body.String())
	}
	if len(arr) != 3 {
		t.Fatalf("routes len = %d, want 3 (full list, no LIMIT)", len(arr))
	}
}

// TestListRoutes_PaginationReturnsSubsetAndTotal verifies that ?page=&pageSize=
// applies LIMIT/OFFSET and returns the {items,total,page,pageSize} envelope
// with the real total (not the page size).
func TestListRoutes_PaginationReturnsSubsetAndTotal(t *testing.T) {
	db, r := setupTokenRoutesTest(t)
	now := time.Now().UTC().Format(time.RFC3339)
	seedRoutes(t, db, 3, now)

	// page=1 pageSize=2 → 2 items, total=3
	resp := doGet(t, r, "/api/routes?page=1&pageSize=2")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}
	var page struct {
		Items    []map[string]any `json:"items"`
		Total    int              `json:"total"`
		Page     int              `json:"page"`
		PageSize int              `json:"pageSize"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode paginated envelope: %v; body=%s", err, resp.Body.String())
	}
	if page.Total != 3 {
		t.Fatalf("total = %d, want 3 (real total, not page size)", page.Total)
	}
	if len(page.Items) != 2 {
		t.Fatalf("items len = %d, want 2 (pageSize)", len(page.Items))
	}
	if page.Page != 1 || page.PageSize != 2 {
		t.Fatalf("page=%d pageSize=%d, want 1/2", page.Page, page.PageSize)
	}

	// page=2 → the remaining single route, total still 3.
	resp2 := doGet(t, r, "/api/routes?page=2&pageSize=2")
	if resp2.Code != http.StatusOK {
		t.Fatalf("page 2 status = %d, want 200; body=%s", resp2.Code, resp2.Body.String())
	}
	var page2 struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
		Page  int              `json:"page"`
	}
	if err := json.Unmarshal(resp2.Body.Bytes(), &page2); err != nil {
		t.Fatalf("decode page 2: %v; body=%s", err, resp2.Body.String())
	}
	if page2.Total != 3 {
		t.Fatalf("page 2 total = %d, want 3", page2.Total)
	}
	if len(page2.Items) != 1 {
		t.Fatalf("page 2 items len = %d, want 1 (remainder)", len(page2.Items))
	}
	if page2.Page != 2 {
		t.Fatalf("page 2 page field = %d, want 2", page2.Page)
	}
}

// TestListRoutes_PaginationScopesChannelBatchLoad confirms that paginated calls
// only load channels for the routes on the current page, so a route on page 2
// still receives its channels but a route on a different page does not leak in.
func TestListRoutes_PaginationScopesChannelBatchLoad(t *testing.T) {
	db, r := setupTokenRoutesTest(t)
	routeID, accountID, tokenID := seedRouteChannelRefs(t, db)

	// Attach a channel to the seeded route so we can confirm it is still
	// enriched on the page that contains it.
	if _, err := db.Exec(
		`INSERT INTO route_channels (route_id, account_id, token_id, source_model, priority, weight, enabled, manual_override)
		 VALUES (?, ?, ?, 'gpt-4o', 1, 10, 1, 0)`,
		routeID, accountID, tokenID,
	); err != nil {
		t.Fatalf("insert channel: %v", err)
	}

	// The seeded route is the only one; pageSize=1 puts it on page 1 with its
	// channel, and page 2 is empty.
	resp := doGet(t, r, "/api/routes?page=1&pageSize=1")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", resp.Code, resp.Body.String())
	}
	var page struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode: %v; body=%s", err, resp.Body.String())
	}
	if page.Total != 1 {
		t.Fatalf("total = %d, want 1", page.Total)
	}
	if len(page.Items) != 1 {
		t.Fatalf("items len = %d, want 1", len(page.Items))
	}
	channels, ok := page.Items[0]["channels"].([]any)
	if !ok || len(channels) != 1 {
		t.Fatalf("channels = %#v, want 1 enriched channel for the page-1 route", page.Items[0]["channels"])
	}
}

func seedRoutes(t *testing.T, db *store.DB, count int, now string) {
	t.Helper()
	for i := 0; i < count; i++ {
		if _, err := db.Exec(
			`INSERT INTO token_routes (model_pattern, enabled, sort_order, created_at, updated_at)
			 VALUES (?, 1, ?, ?, ?)`,
			"model-"+strconv.Itoa(i)+"-*", i, now, now,
		); err != nil {
			t.Fatalf("seed route %d: %v", i, err)
		}
	}
}

// ---- listAccounts (GET /api/accounts) pagination ----

func setupAccountsPaginationTest(t *testing.T) (*store.DB, chi.Router) {
	t.Helper()
	db, r, _ := setupAccountsTest(t)
	now := time.Now().UTC().Format(time.RFC3339)

	res, err := db.Exec(
		`INSERT INTO sites (name, url, platform, status, created_at, updated_at)
		 VALUES ('Acct Site', 'https://acct.example.com', 'openai', 'active', ?, ?)`,
		now, now,
	)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	siteID, _ := res.LastInsertId()

	for i := 0; i < 3; i++ {
		if _, err := db.Exec(
			`INSERT INTO accounts (site_id, username, access_token, status, checkin_enabled, created_at, updated_at)
			 VALUES (?, ?, ?, 'active', 1, ?, ?)`,
			siteID, "user-"+strconv.Itoa(i), "token-"+strconv.Itoa(i), now, now,
		); err != nil {
			t.Fatalf("seed account %d: %v", i, err)
		}
	}
	return db, r
}

// TestListAccounts_NoPaginationReturnsCachedSnapshotShape verifies backward
// compatibility: when ?page is absent the response shape is {generatedAt,
// accounts, sites} with the full account list (no LIMIT).
func TestListAccounts_NoPaginationReturnsCachedSnapshotShape(t *testing.T) {
	_, r := setupAccountsPaginationTest(t)

	resp := doGet(t, r, "/api/accounts?refresh=true")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", resp.Code, resp.Body.String())
	}
	var snapshot map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("decode snapshot: %v; body=%s", err, resp.Body.String())
	}
	// Legacy shape: "accounts" key (not "items"), no total/page/pageSize.
	accounts, ok := snapshot["accounts"].([]any)
	if !ok {
		t.Fatalf("expected 'accounts' array in legacy shape; got keys: %#v", mapKeys(snapshot))
	}
	if len(accounts) != 3 {
		t.Fatalf("accounts len = %d, want 3 (full list, no LIMIT)", len(accounts))
	}
	if _, hasTotal := snapshot["total"]; hasTotal {
		t.Fatalf("legacy shape must not include 'total'; got %#v", snapshot["total"])
	}
}

// TestListAccounts_PaginationReturnsSubsetAndTotal verifies ?page=&pageSize=
// bypasses the cache and returns the {items,total,page,pageSize} envelope.
func TestListAccounts_PaginationReturnsSubsetAndTotal(t *testing.T) {
	_, r := setupAccountsPaginationTest(t)

	resp := doGet(t, r, "/api/accounts?page=1&pageSize=2")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", resp.Code, resp.Body.String())
	}
	var page struct {
		Items    []map[string]any `json:"items"`
		Total    int              `json:"total"`
		Page     int              `json:"page"`
		PageSize int              `json:"pageSize"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode paginated envelope: %v; body=%s", err, resp.Body.String())
	}
	if page.Total != 3 {
		t.Fatalf("total = %d, want 3", page.Total)
	}
	if len(page.Items) != 2 {
		t.Fatalf("items len = %d, want 2", len(page.Items))
	}
	if page.Page != 1 || page.PageSize != 2 {
		t.Fatalf("page=%d pageSize=%d, want 1/2", page.Page, page.PageSize)
	}

	// page 2 → the remaining account.
	resp2 := doGet(t, r, "/api/accounts?page=2&pageSize=2")
	if resp2.Code != http.StatusOK {
		t.Fatalf("page 2 status = %d; body=%s", resp2.Code, resp2.Body.String())
	}
	var page2 struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(resp2.Body.Bytes(), &page2); err != nil {
		t.Fatalf("decode page 2: %v; body=%s", err, resp2.Body.String())
	}
	if page2.Total != 3 {
		t.Fatalf("page 2 total = %d, want 3", page2.Total)
	}
	if len(page2.Items) != 1 {
		t.Fatalf("page 2 items len = %d, want 1", len(page2.Items))
	}
}

func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// ---- listKeys (GET /api/downstream-keys) pagination ----

func setupDownstreamKeysPaginationTest(t *testing.T) (*store.DB, chi.Router) {
	t.Helper()
	db, r := setupDownstreamKeysTest(t)
	now := time.Now().UTC().Format(time.RFC3339)
	for i := 0; i < 3; i++ {
		if _, err := db.Exec(
			`INSERT INTO downstream_api_keys (name, key, enabled, created_at, updated_at)
			 VALUES (?, ?, 1, ?, ?)`,
			"key-"+strconv.Itoa(i), "sk-key-"+strconv.Itoa(i), now, now,
		); err != nil {
			t.Fatalf("seed key %d: %v", i, err)
		}
	}
	return db, r
}

// TestListKeys_NoPaginationReturnsLegacyShape verifies backward compatibility:
// when ?page is absent the response shape is {success, items} with every key.
func TestListKeys_NoPaginationReturnsLegacyShape(t *testing.T) {
	_, r := setupDownstreamKeysPaginationTest(t)

	resp := doGet(t, r, "/api/downstream-keys")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v; body=%s", err, resp.Body.String())
	}
	if body["success"] != true {
		t.Fatalf("success = %#v, want true (legacy shape)", body["success"])
	}
	items, ok := body["items"].([]any)
	if !ok {
		t.Fatalf("expected 'items' array; got %#v", body["items"])
	}
	if len(items) != 3 {
		t.Fatalf("items len = %d, want 3 (full list, no LIMIT)", len(items))
	}
	if _, hasTotal := body["total"]; hasTotal {
		t.Fatalf("legacy shape must not include 'total'; got %#v", body["total"])
	}
}

// TestListKeys_PaginationReturnsSubsetAndTotal verifies ?page=&pageSize=
// applies LIMIT/OFFSET and returns the {items,total,page,pageSize} envelope.
// downstream_api_keys are ordered by id DESC, so page 1 holds the newest keys.
func TestListKeys_PaginationReturnsSubsetAndTotal(t *testing.T) {
	_, r := setupDownstreamKeysPaginationTest(t)

	resp := doGet(t, r, "/api/downstream-keys?page=1&pageSize=2")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", resp.Code, resp.Body.String())
	}
	var page struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
		Page  int              `json:"page"`
		PageSize int           `json:"pageSize"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode paginated envelope: %v; body=%s", err, resp.Body.String())
	}
	if page.Total != 3 {
		t.Fatalf("total = %d, want 3", page.Total)
	}
	if len(page.Items) != 2 {
		t.Fatalf("items len = %d, want 2", len(page.Items))
	}
	if page.Page != 1 || page.PageSize != 2 {
		t.Fatalf("page=%d pageSize=%d, want 1/2", page.Page, page.PageSize)
	}

	// DESC ordering: page 1 holds the two newest keys (key-2, key-1).
	firstName, _ := page.Items[0]["name"].(string)
	if firstName != "key-2" {
		t.Fatalf("page 1 first item name = %q, want 'key-2' (id DESC)", firstName)
	}

	// page 2 → the oldest key.
	resp2 := doGet(t, r, "/api/downstream-keys?page=2&pageSize=2")
	if resp2.Code != http.StatusOK {
		t.Fatalf("page 2 status = %d; body=%s", resp2.Code, resp2.Body.String())
	}
	var page2 struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(resp2.Body.Bytes(), &page2); err != nil {
		t.Fatalf("decode page 2: %v; body=%s", err, resp2.Body.String())
	}
	if page2.Total != 3 {
		t.Fatalf("page 2 total = %d, want 3", page2.Total)
	}
	if len(page2.Items) != 1 {
		t.Fatalf("page 2 items len = %d, want 1", len(page2.Items))
	}
	oldestName, _ := page2.Items[0]["name"].(string)
	if oldestName != "key-0" {
		t.Fatalf("page 2 first item name = %q, want 'key-0' (id DESC remainder)", oldestName)
	}

	// Confirm the plaintext key is still redacted on the paginated path.
	if rawKey, present := page.Items[0]["key"]; present && rawKey != "" {
		t.Fatalf("paginated response leaked plaintext key: %#v", rawKey)
	}
}
