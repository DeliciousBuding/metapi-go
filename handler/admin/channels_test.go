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
	if resp["pageSize"] != float64(50) {
		t.Fatalf("pageSize = %v, want 50", resp["pageSize"])
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
