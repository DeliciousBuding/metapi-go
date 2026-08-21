package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestChannels_ListProjection(t *testing.T) {
	db, r := setupTokenRoutesTest(t)
	routeID, accountID, tokenID := seedRouteChannelRefs(t, db)

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(
		`INSERT INTO route_channels
			(route_id, account_id, token_id, source_model, priority, weight, enabled, manual_override, success_count, total_latency_ms)
		 VALUES (?, ?, ?, 'gpt-4o', 5, 20, 1, 0, 2, 300)`,
		routeID, accountID, tokenID,
	); err != nil {
		t.Fatalf("insert channel: %v", err)
	}
	_ = now

	req := httptest.NewRequest(http.MethodGet, "/api/channels", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if resp["page"] != float64(1) {
		t.Fatalf("page = %v, want 1", resp["page"])
	}
	if resp["pageSize"] != float64(1) {
		t.Fatalf("pageSize = %v, want 1 (unbounded mode reports the full row count)", resp["pageSize"])
	}
	if resp["total"] != float64(1) {
		t.Fatalf("total = %v, want 1", resp["total"])
	}
	rawItems, ok := resp["items"].([]any)
	if !ok {
		t.Fatalf("items missing/wrong type: %#v", resp["items"])
	}
	if len(rawItems) != 1 {
		t.Fatalf("items = %d, want 1", len(rawItems))
	}
	item := rawItems[0].(map[string]any)
	if item["status"] != "enabled" {
		t.Fatalf("status = %v, want enabled", item["status"])
	}
	if item["type"] != "token" {
		t.Fatalf("type = %v, want token", item["type"])
	}
	if item["models"] != "gpt-4o" {
		t.Fatalf("models = %v, want gpt-4o", item["models"])
	}
	if item["priority"] != float64(5) {
		t.Fatalf("priority = %v, want 5", item["priority"])
	}
	if item["weight"] != float64(20) {
		t.Fatalf("weight = %v, want 20", item["weight"])
	}
	if item["responseMs"] != float64(150) {
		t.Fatalf("responseMs = %v, want 150", item["responseMs"])
	}
	if item["enabled"] != true {
		t.Fatalf("enabled = %v, want true", item["enabled"])
	}
	if item["manualOverride"] != false {
		t.Fatalf("manualOverride = %v, want false", item["manualOverride"])
	}
}

func TestChannels_ManuallyDisabledStatus(t *testing.T) {
	db, r := setupTokenRoutesTest(t)
	routeID, accountID, tokenID := seedRouteChannelRefs(t, db)

	if _, err := db.Exec(
		`INSERT INTO route_channels
			(route_id, account_id, token_id, source_model, priority, weight, enabled, manual_override)
		 VALUES (?, ?, ?, 'gpt-4o', 0, 10, 0, 1)`,
		routeID, accountID, tokenID,
	); err != nil {
		t.Fatalf("insert channel: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/channels", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	rawItems, ok := resp["items"].([]any)
	if !ok || len(rawItems) != 1 {
		t.Fatalf("items = %v, want 1 item", resp["items"])
	}
	item := rawItems[0].(map[string]any)
	if item["status"] != "manually_disabled" {
		t.Fatalf("status = %v, want manually_disabled", item["status"])
	}
}

// TestChannels_List_UnboundedReturnsAllRows verifies GET /api/channels no
// longer hard-truncates at the default pageSize of 50 when the client omits
// pagination params: the channels page paginates client-side, so a fleet
// larger than 50 must come back in full. Explicit ?page/?pageSize still opts
// into server-side paging. Regression for the Round 3 contract audit.
func TestChannels_List_UnboundedReturnsAllRows(t *testing.T) {
	db, r := setupTokenRoutesTest(t)
	routeID, accountID, tokenID := seedRouteChannelRefs(t, db)

	const fleetSize = 120
	for i := 0; i < fleetSize; i++ {
		if _, err := db.Exec(
			`INSERT INTO route_channels
				(route_id, account_id, token_id, source_model, priority, weight, enabled, manual_override)
			 VALUES (?, ?, ?, ?, 0, 10, 1, 0)`,
			routeID, accountID, tokenID, "gpt-unbounded-"+itoa(int64(i))); err != nil {
			t.Fatalf("insert channel %d: %v", i, err)
		}
	}

	// No pagination params → full list, no 50-row truncation.
	full := doGet(t, r, "/api/channels")
	if full.Code != http.StatusOK {
		t.Fatalf("unbounded list: %d %s", full.Code, full.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(full.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode unbounded: %v", err)
	}
	items, _ := resp["items"].([]any)
	if len(items) != fleetSize {
		t.Fatalf("unbounded items = %d, want %d (no truncation)", len(items), fleetSize)
	}
	if resp["total"] != float64(fleetSize) {
		t.Fatalf("unbounded total = %v, want %d", resp["total"], fleetSize)
	}
	if resp["pageSize"] != float64(fleetSize) {
		t.Fatalf("unbounded pageSize = %v, want %d", resp["pageSize"], fleetSize)
	}

	// Explicit paging still works and reports the true total.
	paged := doGet(t, r, "/api/channels?page=1&pageSize=50")
	if paged.Code != http.StatusOK {
		t.Fatalf("paged list: %d %s", paged.Code, paged.Body.String())
	}
	var pagedResp map[string]any
	if err := json.Unmarshal(paged.Body.Bytes(), &pagedResp); err != nil {
		t.Fatalf("decode paged: %v", err)
	}
	pagedItems, _ := pagedResp["items"].([]any)
	if len(pagedItems) != 50 {
		t.Fatalf("paged items = %d, want 50", len(pagedItems))
	}
	if pagedResp["total"] != float64(fleetSize) {
		t.Fatalf("paged total = %v, want %d", pagedResp["total"], fleetSize)
	}
}
