package admin

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// ---- PUT /api/channels/batch (Wave 17 P1-3: batch test closure loop) ----
//
// The model-tester batch comparison's "disable failed channels" action calls
// this endpoint with `enabled:false` items. These tests pin the per-item
// truth contract (successIds + failedItems), the validation surface, the
// partial-update semantics (only present fields change), and the audit trail.

func seedBatchUpdateChannels(t *testing.T, db *store.DB, routeID, accountID, tokenID int64, count int) []int64 {
	t.Helper()
	ids := []int64{}
	for i := 0; i < count; i++ {
		sourceModel := "gpt-4o"
		if i > 0 {
			sourceModel = "gpt-4o-mini-" + itoa(int64(i))
		}
		id, err := execInsertID(db.DB,
			`INSERT INTO route_channels
				(route_id, account_id, token_id, source_model, priority, weight, enabled, manual_override)
			 VALUES (?, ?, ?, ?, 3, 15, 1, 0)`,
			routeID, accountID, tokenID, sourceModel,
		)
		if err != nil {
			t.Fatalf("insert channel %d: %v", i, err)
		}
		ids = append(ids, id)
	}
	return ids
}

func putChannelsBatch(t *testing.T, r chi.Router, updates any) map[string]any {
	t.Helper()
	resp := doPutJSON(t, r, "/api/channels/batch", map[string]any{"updates": updates})
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}
	var envelope map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode envelope: %v (body=%s)", err, resp.Body.String())
	}
	return envelope
}

func TestBatchUpdateChannels_DisablesChannelsTruthfully(t *testing.T) {
	db, r := setupTokenRoutesTest(t)
	routeID, accountID, tokenID := seedRouteChannelRefs(t, db)
	ids := seedBatchUpdateChannels(t, db, routeID, accountID, tokenID, 2)

	envelope := putChannelsBatch(t, r, []map[string]any{
		{"id": ids[0], "enabled": false},
		{"id": ids[1], "enabled": false},
	})

	if envelope["success"] != true {
		t.Fatalf("success = %v, want true; envelope=%#v", envelope["success"], envelope)
	}
	successIDs, _ := envelope["successIds"].([]any)
	if len(successIDs) != 2 {
		t.Fatalf("successIds = %#v, want both channel ids", envelope["successIds"])
	}
	if failed, _ := envelope["failedItems"].([]any); len(failed) != 0 {
		t.Fatalf("failedItems = %#v, want empty", envelope["failedItems"])
	}

	// DB truth: both rows disabled and marked manual_override so a route
	// rebuild cannot silently re-enable them.
	var enabledCount, overrideCount int
	if err := db.Get(&enabledCount, "SELECT COUNT(*) FROM route_channels WHERE id IN (?, ?) AND enabled = 1", ids[0], ids[1]); err != nil {
		t.Fatalf("count enabled: %v", err)
	}
	if enabledCount != 0 {
		t.Fatalf("enabled rows = %d, want 0", enabledCount)
	}
	if err := db.Get(&overrideCount, "SELECT COUNT(*) FROM route_channels WHERE id IN (?, ?) AND manual_override = 1", ids[0], ids[1]); err != nil {
		t.Fatalf("count manual_override: %v", err)
	}
	if overrideCount != 2 {
		t.Fatalf("manual_override rows = %d, want 2", overrideCount)
	}

	// The channels list projection reports the manual disable.
	list := doGet(t, r, "/api/channels")
	var listBody struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode channels list: %v", err)
	}
	if len(listBody.Items) != 2 {
		t.Fatalf("channels items = %d, want 2", len(listBody.Items))
	}
	for _, item := range listBody.Items {
		if item["status"] != "manually_disabled" {
			t.Fatalf("status = %v, want manually_disabled", item["status"])
		}
	}
}

func TestBatchUpdateChannels_PartialFailureReportsPerItemTruth(t *testing.T) {
	db, r := setupTokenRoutesTest(t)
	routeID, accountID, tokenID := seedRouteChannelRefs(t, db)
	ids := seedBatchUpdateChannels(t, db, routeID, accountID, tokenID, 1)

	envelope := putChannelsBatch(t, r, []map[string]any{
		{"id": ids[0], "enabled": false},
		{"id": 999999, "enabled": false},
	})

	if envelope["success"] != false {
		t.Fatalf("success = %v, want false on partial failure", envelope["success"])
	}
	successIDs, _ := envelope["successIds"].([]any)
	if len(successIDs) != 1 || successIDs[0] != float64(ids[0]) {
		t.Fatalf("successIds = %#v, want [%d]", envelope["successIds"], ids[0])
	}
	failed, _ := envelope["failedItems"].([]any)
	if len(failed) != 1 {
		t.Fatalf("failedItems = %#v, want exactly one entry", envelope["failedItems"])
	}
	failedItem, _ := failed[0].(map[string]any)
	if failedItem["id"] != float64(999999) || failedItem["message"] != "channel not found" {
		t.Fatalf("failedItems[0] = %#v, want id=999999 message=channel not found", failedItem)
	}

	// The valid item was still applied — partial failure must not roll back.
	var enabled int
	if err := db.Get(&enabled, "SELECT enabled FROM route_channels WHERE id = ?", ids[0]); err != nil {
		t.Fatalf("read enabled: %v", err)
	}
	if enabled != 0 {
		t.Fatalf("enabled = %d, want 0 (successful item applied despite sibling failure)", enabled)
	}
}

func TestBatchUpdateChannels_ValidationFailures(t *testing.T) {
	db, r := setupTokenRoutesTest(t)
	routeID, accountID, tokenID := seedRouteChannelRefs(t, db)
	ids := seedBatchUpdateChannels(t, db, routeID, accountID, tokenID, 1)

	// Empty batch is a request-shape error, not a per-item failure.
	emptyResp := doPutJSON(t, r, "/api/channels/batch", map[string]any{"updates": []map[string]any{}})
	if emptyResp.Code != http.StatusBadRequest {
		t.Fatalf("empty batch status = %d, want 400", emptyResp.Code)
	}

	envelope := putChannelsBatch(t, r, []map[string]any{
		{"id": 0, "enabled": false},
		{"id": ids[0]}, // no updatable fields
		{"id": ids[0], "enabled": true},
		{"id": ids[0], "enabled": false}, // duplicate id
	})

	if envelope["success"] != false {
		t.Fatalf("success = %v, want false", envelope["success"])
	}
	successIDs, _ := envelope["successIds"].([]any)
	if len(successIDs) != 1 {
		t.Fatalf("successIds = %#v, want exactly one applied item", envelope["successIds"])
	}
	failed, _ := envelope["failedItems"].([]any)
	if len(failed) != 3 {
		t.Fatalf("failedItems = %#v, want 3 entries (invalid id, no fields, duplicate)", envelope["failedItems"])
	}
	messages := map[string]bool{}
	for _, raw := range failed {
		item, _ := raw.(map[string]any)
		msg, _ := item["message"].(string)
		messages[msg] = true
	}
	for _, want := range []string{"invalid id", "no updatable fields (priority, weight or enabled required)", "duplicate id in payload"} {
		if !messages[want] {
			t.Fatalf("failedItems messages = %#v, want to include %q", messages, want)
		}
	}

	// Oversized payloads are rejected before touching the database.
	big := make([]map[string]any, 0, 1001)
	for i := 0; i < 1001; i++ {
		big = append(big, map[string]any{"id": i + 1, "enabled": false})
	}
	bigResp := doPutJSON(t, r, "/api/channels/batch", map[string]any{"updates": big})
	if bigResp.Code != http.StatusBadRequest {
		t.Fatalf("oversized batch status = %d, want 400", bigResp.Code)
	}
}

func TestBatchUpdateChannels_AppliesOnlyPresentFields(t *testing.T) {
	db, r := setupTokenRoutesTest(t)
	routeID, accountID, tokenID := seedRouteChannelRefs(t, db)
	ids := seedBatchUpdateChannels(t, db, routeID, accountID, tokenID, 1)

	putChannelsBatch(t, r, []map[string]any{
		{"id": ids[0], "enabled": false},
	})

	var priority, weight, enabled int
	if err := db.Get(&priority, "SELECT priority FROM route_channels WHERE id = ?", ids[0]); err != nil {
		t.Fatalf("read priority: %v", err)
	}
	if err := db.Get(&weight, "SELECT weight FROM route_channels WHERE id = ?", ids[0]); err != nil {
		t.Fatalf("read weight: %v", err)
	}
	if err := db.Get(&enabled, "SELECT enabled FROM route_channels WHERE id = ?", ids[0]); err != nil {
		t.Fatalf("read enabled: %v", err)
	}
	if priority != 3 || weight != 15 {
		t.Fatalf("priority/weight = %d/%d, want 3/15 untouched", priority, weight)
	}
	if enabled != 0 {
		t.Fatalf("enabled = %d, want 0", enabled)
	}
}

func TestBatchUpdateChannels_WriteIsAudited(t *testing.T) {
	db, r := setupTokenRoutesTest(t)
	routeID, accountID, tokenID := seedRouteChannelRefs(t, db)
	ids := seedBatchUpdateChannels(t, db, routeID, accountID, tokenID, 1)

	// Mirror the production wiring (router mounts AuditMiddleware over the
	// admin group) so the disable action's audit row is proven, not assumed.
	wrapped := chi.NewRouter()
	wrapped.Use(AuditMiddleware(db.DB))
	wrapped.Mount("/", r)

	resp := doPutJSON(t, wrapped, "/api/channels/batch", map[string]any{
		"updates": []map[string]any{{"id": ids[0], "enabled": false}},
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.Code)
	}

	var count int
	if err := db.Get(&count, "SELECT COUNT(*) FROM admin_audit_logs WHERE method = ? AND path = ?", http.MethodPut, "/api/channels/batch"); err != nil {
		t.Fatalf("count audit rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("audit rows for PUT /api/channels/batch = %d, want 1", count)
	}
	var status int
	if err := db.Get(&status, "SELECT status FROM admin_audit_logs WHERE method = ? AND path = ?", http.MethodPut, "/api/channels/batch"); err != nil {
		t.Fatalf("read audit status: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("audit status = %d, want 200", status)
	}
}

func TestBatchUpdateChannels_Postgres_PartialFailure(t *testing.T) {
	db, r := setupTokenRoutesPostgresTest(t)
	routeID, accountID, tokenID := seedRouteChannelRefs(t, db)
	ids := seedBatchUpdateChannels(t, db, routeID, accountID, tokenID, 1)

	envelope := putChannelsBatch(t, r, []map[string]any{
		{"id": ids[0], "enabled": false},
		{"id": 999999, "enabled": false},
	})

	if envelope["success"] != false {
		t.Fatalf("success = %v, want false on partial failure", envelope["success"])
	}
	var enabled bool
	if err := db.Get(&enabled, "SELECT enabled FROM route_channels WHERE id = ?", ids[0]); err != nil {
		t.Fatalf("read enabled: %v", err)
	}
	if enabled {
		t.Fatalf("enabled = true, want false (successful item applied)")
	}
}
