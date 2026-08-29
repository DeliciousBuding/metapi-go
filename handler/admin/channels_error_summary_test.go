package admin

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestChannelsErrorSummaryAndStatusFilter(t *testing.T) {
	db, r := setupTokenRoutesTest(t)
	globalChannelsCache.clear()
	globalChannelsErrorSummaryCache.clear()
	routeID, accountID, tokenID := seedRouteChannelRefs(t, db)
	ids := seedBatchUpdateChannels(t, db, routeID, accountID, tokenID, 3)

	future := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	if _, err := db.Exec(`UPDATE route_channels SET enabled = false, manual_override = true WHERE id = ?`, ids[0]); err != nil {
		t.Fatalf("disable fixture: %v", err)
	}
	if _, err := db.Exec(`UPDATE route_channels SET cooldown_until = ? WHERE id = ?`, future, ids[1]); err != nil {
		t.Fatalf("cooldown fixture: %v", err)
	}

	resp := doGet(t, r, "/api/channels/error-summary")
	if resp.Code != http.StatusOK {
		t.Fatalf("error summary status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}
	var summary map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &summary); err != nil {
		t.Fatalf("decode error summary: %v", err)
	}
	if summary["total"] != float64(3) {
		t.Fatalf("summary total = %v, want 3", summary["total"])
	}
	if summary["errorCount"] != float64(1) {
		t.Fatalf("summary errorCount = %v, want 1", summary["errorCount"])
	}
	byStatus, ok := summary["byStatus"].(map[string]any)
	if !ok {
		t.Fatalf("summary byStatus missing: %#v", summary["byStatus"])
	}
	if byStatus["cooldown"] != float64(1) || byStatus["manually_disabled"] != float64(1) || byStatus["enabled"] != float64(1) {
		t.Fatalf("summary byStatus = %#v, want cooldown=1/manually_disabled=1/enabled=1", byStatus)
	}
	if byStatus["breaker_open"] != float64(0) {
		t.Fatalf("summary breaker_open = %v, want 0", byStatus["breaker_open"])
	}

	filtered := decodePagedEnvelope(t, doGet(t, r, "/api/channels?page=1&pageSize=10&status=cooldown,breaker_open"))
	if filtered.Total != 1 || len(filtered.Items) != 1 {
		t.Fatalf("status filter envelope = total %d items %d, want 1/1", filtered.Total, len(filtered.Items))
	}
	if filtered.Items[0]["status"] != "cooldown" {
		t.Fatalf("status filter item status = %v, want cooldown", filtered.Items[0]["status"])
	}

	bad := doGet(t, r, "/api/channels?page=1&status=not-a-status")
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("invalid status filter code = %d, want 400; body=%s", bad.Code, bad.Body.String())
	}
}

func TestChannelsErrorSummaryAndStatusFilterPostgres(t *testing.T) {
	db, r := setupTokenRoutesPostgresTest(t)
	globalChannelsCache.clear()
	globalChannelsErrorSummaryCache.clear()
	routeID, accountID, tokenID := seedRouteChannelRefs(t, db)
	ids := seedBatchUpdateChannels(t, db, routeID, accountID, tokenID, 3)

	future := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	if _, err := db.Exec(db.Rebind(`UPDATE route_channels SET enabled = false, manual_override = true WHERE id = ?`), ids[0]); err != nil {
		t.Fatalf("disable fixture: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`UPDATE route_channels SET cooldown_until = ? WHERE id = ?`), future, ids[1]); err != nil {
		t.Fatalf("cooldown fixture: %v", err)
	}

	resp := doGet(t, r, "/api/channels/error-summary")
	if resp.Code != http.StatusOK {
		t.Fatalf("pg error summary status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}
	var summary map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &summary); err != nil {
		t.Fatalf("decode pg error summary: %v", err)
	}
	if summary["total"] != float64(3) || summary["errorCount"] != float64(1) {
		t.Fatalf("pg summary total/error = %v/%v, want 3/1", summary["total"], summary["errorCount"])
	}
	filtered := decodePagedEnvelope(t, doGet(t, r, "/api/channels?page=1&pageSize=10&status=cooldown,breaker_open"))
	if filtered.Total != 1 || len(filtered.Items) != 1 {
		t.Fatalf("pg status filter envelope = total %d items %d, want 1/1", filtered.Total, len(filtered.Items))
	}
}
