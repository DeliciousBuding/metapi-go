package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// #1174 — two ways the same operator action destroyed routing state:
//
//  1. POST /api/routes/rebuild ran the whole pass on r.Context(). With dozens of
//     accounts the pass takes minutes; the browser or a reverse proxy gives up
//     first, the request context is canceled, the model-sync loop breaks
//     mid-pass and the rebuild that follows fails with "context canceled" — so
//     the operator's rebuild never happened, every time.
//  2. POST /api/settings/maintenance/clear-cache ("清除缓存并重建路由") deleted
//     token_routes, model_availability and route_channels before queueing a
//     rebuild that recomposes channels *from* those tables. The promised
//     rebuild therefore had nothing to rebuild from, and the manual channel
//     attachments the rebuild is careful never to delete were wiped anyway.
//
// These tests pin the honest contract: a rebuild survives the death of the
// request that asked for it, and a cache clear clears caches.

func TestTokenRoutes_Rebuild_SurvivesACanceledRequestContext(t *testing.T) {
	db, r := setupTokenRoutesTest(t)
	routeID, _, tokenID := seedRouteChannelRefs(t, db)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(
		`INSERT INTO token_model_availability (token_id, model_name, available, checked_at)
		 VALUES (?, 'gpt-4o', TRUE, ?)`, tokenID, now); err != nil {
		t.Fatalf("seed availability: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM route_channels WHERE route_id = ?`, routeID); err != nil {
		t.Fatalf("clear channels so the rebuild has real work to do: %v", err)
	}

	// The client is already gone: a browser that timed out, an nginx
	// proxy_read_timeout, a closed tab. The work must not die with it.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodPost, "/api/routes/rebuild",
		strings.NewReader(`{"refreshModels":false}`)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("rebuild with a canceled request context: status = %d, want 200; body = %s",
			rec.Code, rec.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result["success"] != true || result["status"] != "completed" {
		t.Fatalf("unexpected envelope: %v", result)
	}

	var channels int
	if err := db.Get(&channels, `SELECT COUNT(*) FROM route_channels WHERE route_id = ?`, routeID); err != nil {
		t.Fatalf("count route_channels: %v", err)
	}
	if channels == 0 {
		t.Fatalf("no channels rebuilt for route %d despite seeded availability: %v", routeID, result)
	}
}

func TestMaintenanceClearCache_KeepsTheInputsTheRebuildNeeds(t *testing.T) {
	db, r, _ := setupMaintenanceTest(t)
	now := time.Now().UTC().Format(time.RFC3339)

	siteID, err := execInsertID(db.DB,
		`INSERT INTO sites (name, url, platform, status, created_at, updated_at)
		 VALUES ('clear-cache site', 'https://clearcache.example.com', 'openai', 'active', ?, ?)`, now, now)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	accountID, err := execInsertID(db.DB,
		`INSERT INTO accounts (site_id, access_token, status, checkin_enabled, created_at, updated_at)
		 VALUES (?, 'session-token', 'active', true, ?, ?)`, siteID, now, now)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	routeID, err := execInsertID(db.DB,
		`INSERT INTO token_routes (model_pattern, enabled, created_at, updated_at)
		 VALUES ('gpt-*', true, ?, ?)`, now, now)
	if err != nil {
		t.Fatalf("insert route: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO model_availability (account_id, model_name, available, is_manual, checked_at)
		 VALUES (?, 'gpt-4o', TRUE, FALSE, ?)`, accountID, now); err != nil {
		t.Fatalf("insert model_availability: %v", err)
	}
	// A manual attachment: rebuild never deletes these, so a maintenance button
	// must not either.
	manualChannelID, err := execInsertID(db.DB,
		`INSERT INTO route_channels (route_id, account_id, source_model, enabled, manual_override)
		 VALUES (?, ?, 'gpt-4o', true, true)`, routeID, accountID)
	if err != nil {
		t.Fatalf("insert manual route_channel: %v", err)
	}

	resp := doPostJSON(t, r, "/api/settings/maintenance/clear-cache", map[string]any{})
	if resp.Code != http.StatusAccepted {
		t.Fatalf("clear-cache status = %d, want 202; body = %s", resp.Code, resp.Body.String())
	}

	var routes, availability, manualChannels int64
	if err := db.Get(&routes, `SELECT COUNT(*) FROM token_routes WHERE id = ?`, routeID); err != nil {
		t.Fatalf("count token_routes: %v", err)
	}
	if routes != 1 {
		t.Fatalf("token_routes row %d deleted by a cache clear (count = %d); route definitions are operator state, not cache", routeID, routes)
	}
	if err := db.Get(&availability, `SELECT COUNT(*) FROM model_availability WHERE account_id = ?`, accountID); err != nil {
		t.Fatalf("count model_availability: %v", err)
	}
	if availability != 1 {
		t.Fatalf("model_availability deleted by a cache clear (count = %d); re-fetching it costs an upstream round-trip per account and can fail outright on an expired credential", availability)
	}
	if err := db.Get(&manualChannels, `SELECT COUNT(*) FROM route_channels WHERE id = ?`, manualChannelID); err != nil {
		t.Fatalf("count manual route_channel: %v", err)
	}
	if manualChannels != 1 {
		t.Fatalf("manual route_channel %d deleted by a cache clear; the rebuild it is supposed to feed never deletes manual attachments", manualChannelID)
	}
}
