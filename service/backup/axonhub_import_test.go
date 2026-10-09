package backup

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

func openAxonHubTestDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func countRows(t *testing.T, db *store.DB, query string, args ...any) int {
	t.Helper()
	var count int
	if err := db.Get(&count, db.Rebind(query), args...); err != nil {
		t.Fatalf("count %q: %v", query, err)
	}
	return count
}

func TestImportAxonHubV14PersistsExecutableGraph(t *testing.T) {
	db := openAxonHubTestDB(t)
	raw := readAxonHubFixture(t, "axonhub-associations.json")
	counts, err := ImportAxonHubV14(db, raw, "fixture-axonhub", false)
	if err != nil {
		t.Fatal(err)
	}
	if counts["channels"] != 3 || counts["models"] != 4 || counts["grants"] != 6 || counts["routes"] != 5 {
		t.Fatalf("unexpected import counts: %#v", counts)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM upstream_channels`); got != 3 {
		t.Fatalf("upstream_channels = %d, want 3", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM upstream_credentials`); got != 4 {
		t.Fatalf("upstream_credentials = %d, want 4", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM upstream_group_items`); got != 9 {
		t.Fatalf("upstream_group_items = %d, want 9", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM token_routes WHERE routing_strategy = 'weighted'`); got != 5 {
		t.Fatalf("weighted routes = %d, want 5", got)
	}
	// A disabled source channel must stay disabled: it carries operator intent.
	if got := countRows(t, db, `SELECT COUNT(*) FROM upstream_channels WHERE enabled = ?`, false); got != 1 {
		t.Fatalf("disabled channels = %d, want 1", got)
	}
	// The selected grant protocol union must match each channel's declared wire.
	var protocolMask int
	if err := db.Get(&protocolMask, db.Rebind(`SELECT protocols FROM upstream_grants g
		JOIN upstream_models m ON m.id = g.model_id
		JOIN upstream_channels c ON c.id = m.channel_id
		WHERE c.name = ? LIMIT 1`), "Fixture Anthropic relay"); err != nil {
		t.Fatal(err)
	}
	if protocolMask != protoMessages {
		t.Fatalf("anthropic grant protocols = %d, want %d", protocolMask, protoMessages)
	}

	// The compiled graph must be loadable by the real routing store, not just
	// present in tables: a channel nobody can select is not an import.
	routingStore := service.NewProxyRoutingStore(db)
	var routeIDs []int64
	if err := db.Select(&routeIDs, db.Rebind(`SELECT id FROM token_routes ORDER BY id`)); err != nil {
		t.Fatal(err)
	}
	candidates, err := routingStore.LoadRouteChannels(context.Background(), routeIDs)
	if err != nil {
		t.Fatal(err)
	}
	direct := 0
	for _, candidate := range candidates {
		if candidate.Channel.Direct != nil {
			direct++
		}
	}
	if direct != 9 {
		t.Fatalf("routing store loaded %d direct candidates, want 9", direct)
	}

	// Re-importing the same backup must update the same rows, preserving the
	// health counters the runtime writes onto them.
	if _, err := db.Exec(`UPDATE upstream_grants SET success_count = 7, total_latency_ms = 420`); err != nil {
		t.Fatal(err)
	}
	second, err := ImportAxonHubV14(db, raw, "fixture-axonhub", false)
	if err != nil {
		t.Fatalf("re-import: %v", err)
	}
	if second["channels"] != 3 || second["routes"] != 5 {
		t.Fatalf("re-import counts drifted: %#v", second)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM upstream_channels`); got != 3 {
		t.Fatalf("re-import duplicated channels: %d", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM upstream_grants WHERE success_count = 7`); got != 6 {
		t.Fatalf("re-import replaced rows instead of updating them: %d grants kept their counters", got)
	}
}

func TestImportAxonHubV14IsOriginScoped(t *testing.T) {
	db := openAxonHubTestDB(t)
	raw := readAxonHubFixture(t, "axonhub-associations.json")
	if _, err := ImportAxonHubV14(db, raw, "origin-a", false); err != nil {
		t.Fatal(err)
	}
	// A second origin shares the same model pattern, so the route claim must be
	// refused rather than silently stolen.
	if _, err := ImportAxonHubV14(db, raw, "origin-b", false); err == nil {
		t.Fatal("a second origin claimed routes an existing origin owns")
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM token_routes`); got != 5 {
		t.Fatalf("routes after refused second origin = %d, want 5", got)
	}
}

func TestImportAxonHubV14RequiresReplacementConfirmation(t *testing.T) {
	db := openAxonHubTestDB(t)
	full := readAxonHubFixture(t, "axonhub-associations.json")
	if _, err := ImportAxonHubV14(db, full, "fixture-axonhub", false); err != nil {
		t.Fatal(err)
	}
	reduced := removeAxonHubChannelAndItsModels(t, full, 4)

	preview, err := PreviewAxonHubV14(db, reduced, "fixture-axonhub")
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Removals) == 0 {
		t.Fatal("preview of a reduced snapshot reported no removals")
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM token_routes`); got != 5 {
		t.Fatalf("preview wrote %d routes", got)
	}

	if _, err := ImportAxonHubV14(db, reduced, "fixture-axonhub", false); !errors.Is(err, ErrAxonHubReplacementRequired) {
		t.Fatalf("import without confirmation = %v, want ErrAxonHubReplacementRequired", err)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM upstream_channels`); got != 3 {
		t.Fatalf("refused import changed channels: %d", got)
	}

	counts, err := ImportAxonHubV14(db, reduced, "fixture-axonhub", true)
	if err != nil {
		t.Fatal(err)
	}
	if counts["channels"] != 2 || counts["routes"] != 3 {
		t.Fatalf("replacement counts = %#v", counts)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM token_routes`); got != 3 {
		t.Fatalf("routes after replacement = %d, want 3", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM external_source_ids WHERE origin_key = ?`, "fixture-axonhub"); got == 0 {
		t.Fatal("replacement dropped the origin mapping")
	}
	// Nothing from a different origin may be touched.
	if got := countRows(t, db, `SELECT COUNT(*) FROM external_source_ids WHERE origin_key <> ?`, "fixture-axonhub"); got != 0 {
		t.Fatalf("replacement touched another origin: %d rows", got)
	}
}

func TestCompileAxonHubPlanIsDeterministic(t *testing.T) {
	src, err := ParseAxonHubSource(readAxonHubFixture(t, "axonhub-associations.json"))
	if err != nil {
		t.Fatal(err)
	}
	first, err := CompileAxonHubPlan(src)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CompileAxonHubPlan(src)
	if err != nil {
		t.Fatal(err)
	}
	firstIDs := axonHubPlanSourceIDs(first)
	secondIDs := axonHubPlanSourceIDs(second)
	if len(firstIDs) != len(secondIDs) {
		t.Fatalf("entity types differ between compiles: %v vs %v", firstIDs, secondIDs)
	}
	for entity, ids := range firstIDs {
		if len(ids) != len(secondIDs[entity]) {
			t.Fatalf("%s entity counts differ between compiles", entity)
		}
		for id := range ids {
			if !secondIDs[entity][id] {
				t.Fatalf("%s source id %d is not stable across compiles", entity, id)
			}
		}
	}
}

// removeAxonHubChannelAndItsModels drops one channel and the models that only
// that channel could serve, producing a smaller snapshot of the same origin.
func removeAxonHubChannelAndItsModels(t *testing.T, raw []byte, channelID int) []byte {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	channels := payload["channels"].([]any)
	keptChannels := channels[:0]
	for _, entry := range channels {
		channel := entry.(map[string]any)
		if int(channel["id"].(float64)) != channelID {
			keptChannels = append(keptChannels, entry)
		}
	}
	payload["channels"] = keptChannels
	models := payload["models"].([]any)
	keptModels := models[:0]
	for _, entry := range models {
		model := entry.(map[string]any)
		settings, _ := model["settings"].(map[string]any)
		associations, _ := settings["associations"].([]any)
		drop := false
		for _, item := range associations {
			association := item.(map[string]any)
			for _, key := range []string{"channelModel", "channelRegex"} {
				reference, ok := association[key].(map[string]any)
				if !ok {
					continue
				}
				if id, ok := reference["channelId"].(float64); ok && int(id) == channelID {
					drop = true
				}
			}
		}
		if !drop {
			keptModels = append(keptModels, entry)
		}
	}
	payload["models"] = keptModels
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
