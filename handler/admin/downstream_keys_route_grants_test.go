package admin

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// A deleted route used to stay behind in downstream_api_keys.allowed_route_ids:
// the delete transaction cleaned route_group_sources, route_channels and
// token_routes and stopped there. The key update path validates that list and
// carries the stored list forward when a request omits it, and the admin UI has
// no control for the field — so one deleted route made every later edit of that
// key fail with "allowedRouteIds contains unknown routes: <id>" and there was no
// way to remove the id. An operator running 100+ sites reported exactly that.
//
// The fix has two halves, and one hard constraint: pruning must never widen a
// credential. An empty allowlist means "no route restriction", so dropping the
// LAST grant of a restricted key would promote it from "serves nothing" to
// "serves every route". TestDeleteTheLastAuthorizedRouteDoesNotWidenTheKey is
// the one that protects that; the others protect the unblocking.

func setupRouteGrantTest(t *testing.T) (*store.DB, chi.Router) {
	t.Helper()
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("failed to open SQLite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}
	r := chi.NewRouter()
	RegisterTokenRoutesWithDeps(r, db.DB, TokenRoutesDeps{})
	RegisterDownstreamKeysRoutes(r, db.DB)
	return db, r
}

func seedRouteGrantRoute(t *testing.T, db *store.DB, pattern string) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(db.Rebind(
		`INSERT INTO token_routes (model_pattern, display_name, route_mode, enabled, created_at, updated_at)
		 VALUES (?, ?, 'pattern', ?, ?, ?)`), pattern, pattern, true, now, now)
	if err != nil {
		t.Fatalf("seed route %s: %v", pattern, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("route %s LastInsertId: %v", pattern, err)
	}
	return id
}

func seedRouteGrantKey(t *testing.T, r chi.Router, name string, allowedRouteIDs []int64) int64 {
	t.Helper()
	resp := doPostJSON(t, r, "/api/downstream-keys", map[string]any{
		"name":            name,
		"key":             "sk-" + name,
		"supportedModels": []string{"*"},
		"allowedRouteIds": allowedRouteIDs,
	})
	if resp.Code != http.StatusOK && resp.Code != http.StatusCreated {
		t.Fatalf("create key %s returned %d: %s", name, resp.Code, resp.Body.String())
	}
	var body struct {
		Item struct {
			ID int64 `json:"id"`
		} `json:"item"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode create response for %s: %v (%s)", name, err, resp.Body.String())
	}
	if body.Item.ID == 0 {
		t.Fatalf("create key %s returned no id: %s", name, resp.Body.String())
	}
	return body.Item.ID
}

func storedRouteGrants(t *testing.T, db *store.DB, keyID int64) []int64 {
	t.Helper()
	row := queryRow(db.DB, "SELECT * FROM downstream_api_keys WHERE id = ?", keyID)
	if row == nil {
		t.Fatalf("downstream key %d is gone", keyID)
	}
	return parseIntArrayFromDB(row, "allowed_route_ids")
}

func decodeGrantBody(t *testing.T, body []byte) map[string]any {
	t.Helper()
	out := map[string]any{}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode response: %v (%s)", err, body)
	}
	return out
}

func idList(t *testing.T, value any) []int64 {
	t.Helper()
	if value == nil {
		return nil
	}
	raw, ok := value.([]any)
	if !ok {
		t.Fatalf("expected a JSON array, got %T (%v)", value, value)
	}
	out := make([]int64, 0, len(raw))
	for _, item := range raw {
		number, ok := item.(float64)
		if !ok {
			t.Fatalf("expected numbers in %v, got %T", raw, item)
		}
		out = append(out, int64(number))
	}
	return out
}

// The common case: a key authorized for several routes loses one to a delete and
// must stay editable afterwards.
func TestDeleteRoutePrunesTheGrantFromDownstreamKeys(t *testing.T) {
	db, r := setupRouteGrantTest(t)
	routeA := seedRouteGrantRoute(t, db, "grant-a-*")
	routeB := seedRouteGrantRoute(t, db, "grant-b-*")
	keyID := seedRouteGrantKey(t, r, "multi-grant", []int64{routeA, routeB})

	resp := doDelete(t, r, "/api/routes/"+strconv.FormatInt(routeA, 10))
	if resp.Code != http.StatusOK {
		t.Fatalf("delete route returned %d: %s", resp.Code, resp.Body.String())
	}
	body := decodeGrantBody(t, resp.Body.Bytes())
	if got := idList(t, body["downstreamKeysWithOnlyDeletedRoutes"]); len(got) != 0 {
		t.Errorf("delete reported keys it should have pruned cleanly: %v", got)
	}

	got := storedRouteGrants(t, db, keyID)
	if len(got) != 1 || got[0] != routeB {
		t.Fatalf("stored grants after delete = %v, want [%d] (the deleted route must not stay behind)", got, routeB)
	}

	// The point of the cascade: this save used to fail on the dead id.
	rename := doPutJSON(t, r, "/api/downstream-keys/"+strconv.FormatInt(keyID, 10), map[string]any{"name": "renamed-after-delete"})
	if rename.Code != http.StatusOK {
		t.Fatalf("renaming the key after a route delete returned %d: %s", rename.Code, rename.Body.String())
	}
}

// The constraint that outranks the convenience: pruning the last grant would
// turn "serves nothing" into "serves every route", so it must not happen.
func TestDeleteTheLastAuthorizedRouteDoesNotWidenTheKey(t *testing.T) {
	db, r := setupRouteGrantTest(t)
	routeA := seedRouteGrantRoute(t, db, "only-grant-*")
	keyID := seedRouteGrantKey(t, r, "single-grant", []int64{routeA})

	resp := doDelete(t, r, "/api/routes/"+strconv.FormatInt(routeA, 10))
	if resp.Code != http.StatusOK {
		t.Fatalf("delete route returned %d: %s", resp.Code, resp.Body.String())
	}
	body := decodeGrantBody(t, resp.Body.Bytes())
	reported := idList(t, body["downstreamKeysWithOnlyDeletedRoutes"])
	if len(reported) != 1 || reported[0] != keyID {
		t.Fatalf("delete must name the key it left authorizing only deleted routes, got %v (want [%d])", reported, keyID)
	}

	got := storedRouteGrants(t, db, keyID)
	if len(got) != 1 || got[0] != routeA {
		t.Fatalf("stored grants = %v, want the dead id [%d] kept: an empty allowlist means EVERY route, so pruning here would widen the key", got, routeA)
	}

	// The key must still be editable — that is the whole unblock — and the save
	// must say out loud that its grants point at nothing.
	rename := doPutJSON(t, r, "/api/downstream-keys/"+strconv.FormatInt(keyID, 10), map[string]any{"name": "renamed-single-grant"})
	if rename.Code != http.StatusOK {
		t.Fatalf("saving a key whose only grant was deleted returned %d: %s (this is the reported defect)", rename.Code, rename.Body.String())
	}
	renameBody := decodeGrantBody(t, rename.Body.Bytes())
	if stale := idList(t, renameBody["staleRouteIds"]); len(stale) != 1 || stale[0] != routeA {
		t.Errorf("staleRouteIds = %v, want [%d]", stale, routeA)
	}
	notice, _ := renameBody["staleRouteGrantNotice"].(string)
	if !strings.Contains(notice, "every route") {
		t.Errorf("the notice must explain why the grant was kept, got %q", notice)
	}
	if pruned := idList(t, renameBody["prunedRouteIds"]); len(pruned) != 0 {
		t.Errorf("prunedRouteIds = %v, want none: pruning the last grant is the widening this test forbids", pruned)
	}
	if got := storedRouteGrants(t, db, keyID); len(got) != 1 || got[0] != routeA {
		t.Fatalf("stored grants after save = %v, want the dead id still there", got)
	}

	// Widening stays an explicit operator act: clearing the list is allowed.
	clear := doPutJSON(t, r, "/api/downstream-keys/"+strconv.FormatInt(keyID, 10), map[string]any{"allowedRouteIds": []int64{}})
	if clear.Code != http.StatusOK {
		t.Fatalf("clearing the allowlist returned %d: %s", clear.Code, clear.Body.String())
	}
	if got := storedRouteGrants(t, db, keyID); len(got) != 0 {
		t.Fatalf("stored grants after an explicit clear = %v, want empty", got)
	}
}

// Rows that are already dangling in the wild (written before the cascade
// existed) heal on the next save instead of blocking it.
func TestUpdateKeyHealsAnInheritedDeadGrantAndKeepsTheLiveOnes(t *testing.T) {
	db, r := setupRouteGrantTest(t)
	routeB := seedRouteGrantRoute(t, db, "heal-live-*")
	keyID := seedRouteGrantKey(t, r, "inherited-dead", []int64{routeB})

	dead := int64(999999)
	if _, err := db.Exec(db.Rebind("UPDATE downstream_api_keys SET allowed_route_ids = ? WHERE id = ?"),
		"["+strconv.FormatInt(routeB, 10)+","+strconv.FormatInt(dead, 10)+"]", keyID); err != nil {
		t.Fatalf("simulate a pre-fix dangling grant: %v", err)
	}

	resp := doPutJSON(t, r, "/api/downstream-keys/"+strconv.FormatInt(keyID, 10), map[string]any{"name": "renamed-healed"})
	if resp.Code != http.StatusOK {
		t.Fatalf("saving a key with an inherited dead grant returned %d: %s", resp.Code, resp.Body.String())
	}
	body := decodeGrantBody(t, resp.Body.Bytes())
	if pruned := idList(t, body["prunedRouteIds"]); len(pruned) != 1 || pruned[0] != dead {
		t.Errorf("prunedRouteIds = %v, want [%d]", pruned, dead)
	}
	if stale := idList(t, body["staleRouteIds"]); len(stale) != 0 {
		t.Errorf("staleRouteIds = %v, want none: a live grant survived, so the dead one is pruned", stale)
	}
	if got := storedRouteGrants(t, db, keyID); len(got) != 1 || got[0] != routeB {
		t.Fatalf("stored grants = %v, want [%d]", got, routeB)
	}
}

// Healing must not become a way to smuggle in a route id that does not exist:
// what the request chose is still validated.
func TestUpdateKeyStillRejectsANewlyChosenUnknownRoute(t *testing.T) {
	db, r := setupRouteGrantTest(t)
	routeB := seedRouteGrantRoute(t, db, "reject-live-*")
	keyID := seedRouteGrantKey(t, r, "reject-unknown", []int64{routeB})

	resp := doPutJSON(t, r, "/api/downstream-keys/"+strconv.FormatInt(keyID, 10), map[string]any{
		"allowedRouteIds": []int64{routeB, 999999},
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("a newly chosen unknown route returned %d, want 400: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "allowedRouteIds contains unknown routes: 999999") {
		t.Errorf("the rejection must name the id, got %s", resp.Body.String())
	}
	if got := storedRouteGrants(t, db, keyID); len(got) != 1 || got[0] != routeB {
		t.Fatalf("a rejected save must not change the stored grants, got %v", got)
	}

	created := doPostJSON(t, r, "/api/downstream-keys", map[string]any{
		"name":            "create-unknown",
		"key":             "sk-create-unknown",
		"allowedRouteIds": []int64{999999},
	})
	if created.Code != http.StatusBadRequest {
		t.Fatalf("create with an unknown route returned %d, want 400: %s", created.Code, created.Body.String())
	}
}
