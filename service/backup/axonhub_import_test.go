package backup

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/internal/pgtest"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

func openAxonHubTestDB(t *testing.T) *store.DB {
	t.Helper()
	dialect, dsn := store.DialectSQLite, ":memory:"
	if pgDSN := os.Getenv("PG_TEST_DSN"); pgDSN != "" {
		dialect, dsn = store.DialectPostgres, pgDSN
	}
	db, err := store.Open(dialect, dsn, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if dialect == store.DialectPostgres {
		pgtest.Reset(t, db.DB)
	}
	if err := store.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestAxonHubImportOverlappingAssociationsKeepsHighestPriority(t *testing.T) {
	db := openAxonHubTestDB(t)
	raw := []byte(`{"version":"1.4","timestamp":"2026-10-09T00:00:00Z","channels":[
		{"id":1,"type":"openai","name":"overlap","base_url":"https://relay.invalid","credentials":{"apiKey":"fixture-key"},"supported_models":["gpt-6"],"endpoints":[{"api_format":"openai/chat_completions"}]}
	],"models":[{"id":1,"model_id":"gpt-6","type":"chat","settings":{"associations":[
		{"type":"model","priority":5,"modelId":{"modelId":"gpt-6"}},
		{"type":"channel_model","priority":1,"channelModel":{"channelId":1,"modelId":"gpt-6"}}
	]}}]}`)
	if _, err := ImportAxonHubV14(db, raw, "overlap", false); err != nil {
		t.Fatalf("overlapping associations must import one candidate: %v", err)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM upstream_group_items WHERE priority = 1`); got != 1 {
		t.Fatalf("highest-priority candidate count = %d, want 1", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM upstream_group_items`); got != 1 {
		t.Fatalf("duplicate candidates = %d, want one", got)
	}
}

func TestAxonHubDisabledConditionDoesNotDropAssociation(t *testing.T) {
	db := openAxonHubTestDB(t)
	raw := []byte(`{"version":"1.4","timestamp":"2026-10-09T00:00:00Z","channels":[
		{"id":1,"type":"openai","name":"unconditional","base_url":"https://relay.invalid","credentials":{"apiKey":"fixture-key"},"supported_models":["gpt-6"],"endpoints":[{"api_format":"openai/chat_completions"}]}
	],"models":[{"id":1,"model_id":"gpt-6","type":"chat","settings":{"associations":[
		{"type":"channel_model","when":{"enabled":false},"channelModel":{"channelId":1,"modelId":"gpt-6"}}
	]}}]}`)
	counts, err := ImportAxonHubV14(db, raw, "condition-disabled", false)
	if err != nil {
		t.Fatal(err)
	}
	if counts["routes"] != 1 || counts["groupItems"] != 1 {
		t.Fatalf("disabled condition removed the unconditional route: %#v", counts)
	}
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
	if counts["channels"] != 4 || counts["models"] != 5 || counts["grants"] != 7 || counts["routes"] != 6 {
		t.Fatalf("unexpected import counts: %#v", counts)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM upstream_channels`); got != 4 {
		t.Fatalf("upstream_channels = %d, want 4", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM upstream_credentials`); got != 5 {
		t.Fatalf("upstream_credentials = %d, want 5", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM upstream_group_items`); got != 10 {
		t.Fatalf("upstream_group_items = %d, want 10", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM token_routes WHERE routing_strategy = 'weighted'`); got != 6 {
		t.Fatalf("weighted routes = %d, want 6", got)
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
	if direct != 10 {
		t.Fatalf("routing store loaded %d direct candidates, want 10", direct)
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
	if second["channels"] != 4 || second["routes"] != 6 {
		t.Fatalf("re-import counts drifted: %#v", second)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM upstream_channels`); got != 4 {
		t.Fatalf("re-import duplicated channels: %d", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM upstream_grants WHERE success_count = 7`); got != 7 {
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
	if got := countRows(t, db, `SELECT COUNT(*) FROM token_routes`); got != 6 {
		t.Fatalf("routes after refused second origin = %d, want 6", got)
	}
}

func TestAxonHubReimportCannotRenameOntoNativeRoute(t *testing.T) {
	db := openAxonHubTestDB(t)
	raw := axonHubSettingsPayload(`{}`)
	if _, err := ImportAxonHubV14(db, raw, "rename-conflict", false); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO token_routes (model_pattern, routing_strategy, enabled) VALUES ('native-model', 'weighted', true)`); err != nil {
		t.Fatal(err)
	}
	renamed := []byte(strings.Replace(string(raw), `"model_id":"gpt-6"`, `"model_id":"native-model"`, 1))
	if _, err := ImportAxonHubV14(db, renamed, "rename-conflict", false); err == nil {
		t.Fatal("reimport renamed an owned route onto an unrelated native route")
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM token_routes WHERE model_pattern = 'gpt-6'`); got != 1 {
		t.Fatal("failed reimport did not roll back the existing model route")
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM token_routes WHERE model_pattern = 'native-model'`); got != 1 {
		t.Fatal("failed reimport modified the native route")
	}
}

func TestAxonHubSourceModelIdentityRejectsBeforeWrites(t *testing.T) {
	for _, tc := range []struct{ name, models string }{
		{"missing id", `[{"model_id":"a"}]`},
		{"duplicate id", `[{"id":1,"model_id":"a"},{"id":1,"model_id":"b"}]`},
		{"unknown status", `[{"id":1,"model_id":"a","status":"unknown"}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openAxonHubTestDB(t)
			var payload map[string]json.RawMessage
			if err := json.Unmarshal(axonHubSettingsPayload(`{}`), &payload); err != nil {
				t.Fatal(err)
			}
			payload["models"] = json.RawMessage(tc.models)
			raw, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ImportAxonHubV14(db, raw, "invalid-model", false); err == nil {
				t.Fatal("invalid model identity was accepted")
			}
			if got := countRows(t, db, `SELECT COUNT(*) FROM upstream_channels`); got != 0 {
				t.Fatalf("invalid source persisted %d channels", got)
			}
		})
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
	if got := countRows(t, db, `SELECT COUNT(*) FROM token_routes`); got != 6 {
		t.Fatalf("preview wrote %d routes", got)
	}

	if _, err := ImportAxonHubV14(db, reduced, "fixture-axonhub", false); !errors.Is(err, ErrAxonHubReplacementRequired) {
		t.Fatalf("import without confirmation = %v, want ErrAxonHubReplacementRequired", err)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM upstream_channels`); got != 4 {
		t.Fatalf("refused import changed channels: %d", got)
	}

	counts, err := ImportAxonHubV14(db, reduced, "fixture-axonhub", true)
	if err != nil {
		t.Fatal(err)
	}
	if counts["channels"] != 3 || counts["routes"] != 4 {
		t.Fatalf("replacement counts = %#v", counts)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM token_routes`); got != 4 {
		t.Fatalf("routes after replacement = %d, want 4", got)
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
