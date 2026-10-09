package backup

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/service/upstream"
	"github.com/deliciousbuding/metapi-go/store"
)

func lifecycleSource(t *testing.T, kind string) (full, reduced []byte) {
	t.Helper()
	if kind == "octopus" {
		full = []byte(octopusV5RealShapeFixture)
		d, err := ParseOctopusV5(full)
		if err != nil {
			t.Fatal(err)
		}
		d.Channels = d.Channels[:1]
		d.Credentials = d.Credentials[:1]
		d.Models = d.Models[:1]
		d.Grants = d.Grants[:1]
		d.GroupItems = d.GroupItems[:1]
		reduced, err = json.Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
		return
	}
	full = []byte(`{"version":"1.4","timestamp":"2026-10-10T00:00:00Z","channels":[{"id":11,"type":"moonshot","name":"first","base_url":"https://first.invalid","credentials":{"apiKey":"fixture-first"},"supported_models":["provider-model"]},{"id":12,"type":"moonshot","name":"second","base_url":"https://second.invalid","credentials":{"apiKey":"fixture-second"},"supported_models":["provider-model"]}],"models":[{"id":21,"model_id":"client-model","settings":{"associations":[{"type":"channel_model","channelModel":{"channelId":11,"modelId":"provider-model"}},{"type":"channel_model","channelModel":{"channelId":12,"modelId":"provider-model"}}]}}]}`)
	var obj map[string]any
	_ = json.Unmarshal(full, &obj)
	obj["channels"] = obj["channels"].([]any)[:1]
	settings := obj["models"].([]any)[0].(map[string]any)["settings"].(map[string]any)
	settings["associations"] = settings["associations"].([]any)[:1]
	reduced, _ = json.Marshal(obj)
	return
}

func lifecycleImport(db *store.DB, kind string, raw []byte, confirm bool, revision ...string) error {
	var err error
	if kind == "octopus" {
		_, err = ImportOctopusV5WithUnsupportedMode(db, raw, "lifecycle", false, confirm, revision...)
	} else {
		_, err = ImportAxonHubV14(db, raw, "lifecycle", confirm, revision...)
	}
	return err
}

func lifecycleImpact(t *testing.T, db *store.DB, kind string, raw []byte) *upstream.DeletionPreview {
	t.Helper()
	if kind == "octopus" {
		p, err := PreviewOctopusV5(db, raw, "lifecycle")
		if err != nil {
			t.Fatal(err)
		}
		return p.RemovalImpact
	}
	p, err := PreviewAxonHubV14(db, raw, "lifecycle")
	if err != nil {
		t.Fatal(err)
	}
	return p.RemovalImpact
}

func lifecycleInsert(t *testing.T, db *store.DB, query string, args ...any) int64 {
	t.Helper()
	var id int64
	if err := db.Get(&id, db.Rebind(query+" RETURNING id"), args...); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestSourceReplacementIncludesNativeAndCrossOriginClosure(t *testing.T) {
	for _, kind := range []string{"octopus", "axonhub"} {
		t.Run(kind, func(t *testing.T) {
			db := openAxonHubTestDB(t)
			full, reduced := lifecycleSource(t, kind)
			if err := lifecycleImport(db, kind, full, false); err != nil {
				t.Fatal(err)
			}
			before := lifecycleImpact(t, db, kind, reduced)
			if before == nil {
				t.Fatal("missing original removal impact")
			}
			var channel, group int64
			if err := db.Get(&channel, `SELECT id FROM upstream_channels WHERE source_id=12`); err != nil {
				t.Fatal(err)
			}
			if err := db.Get(&group, `SELECT id FROM upstream_groups LIMIT 1`); err != nil {
				t.Fatal(err)
			}
			model := lifecycleInsert(t, db, `INSERT INTO upstream_models(origin_key,source_id,channel_id,name,enabled) VALUES ('native:local',0,?,'local-model',?)`, channel, true)
			credential := lifecycleInsert(t, db, `INSERT INTO upstream_credentials(origin_key,source_id,channel_id,name,secret,enabled) VALUES ('native:local',0,?,'local-key','fixture-local-secret',?)`, channel, true)
			grant := lifecycleInsert(t, db, `INSERT INTO upstream_grants(origin_key,source_id,model_id,credential_id,protocols,enabled) VALUES ('native:local',0,?,?,2,?)`, model, credential, true)
			member := lifecycleInsert(t, db, `INSERT INTO upstream_group_items(origin_key,source_id,group_id,grant_id,priority,weight) VALUES ('native:local',0,?,?,1,1)`, group, grant)
			for table, id := range map[string]int64{"upstream_models": model, "upstream_credentials": credential, "upstream_grants": grant, "upstream_group_items": member} {
				if _, err := db.Exec(db.Rebind("UPDATE "+table+" SET source_id=? WHERE id=?"), -id, id); err != nil {
					t.Fatal(err)
				}
			}
			foreignGroup := lifecycleInsert(t, db, `INSERT INTO upstream_groups(origin_key,source_id,name,mode,active_item_id,relay_config,enabled) VALUES ('other-origin',91,'foreign-group','manual',0,'{}',?)`, true)
			foreignMember := lifecycleInsert(t, db, `INSERT INTO upstream_group_items(origin_key,source_id,group_id,grant_id,priority,weight) VALUES ('other-origin',92,?,?,1,1)`, foreignGroup, grant)
			foreignRoute := lifecycleInsert(t, db, `INSERT INTO token_routes(model_pattern,route_mode,routing_strategy,enabled) VALUES ('foreign-model','pattern','weighted',?)`, true)
			if _, err := db.Exec(db.Rebind(`INSERT INTO upstream_route_groups(route_id,group_id) VALUES (?,?)`), foreignRoute, foreignGroup); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(db.Rebind(`UPDATE upstream_groups SET active_item_id=? WHERE id=?`), foreignMember, foreignGroup); err != nil {
				t.Fatal(err)
			}
			for i, alias := range []string{"group_items", "upstream_group_items"} {
				if _, err := db.Exec(db.Rebind(`INSERT INTO external_source_ids(origin_key,entity_type,source_id,target_id) VALUES ('other-origin',?,?,?)`), alias, 90+i, foreignMember); err != nil {
					t.Fatal(err)
				}
			}
			after := lifecycleImpact(t, db, kind, reduced)
			for _, label := range []string{"models", "credentials", "grants"} {
				if after.Counts[label] != before.Counts[label]+1 {
					t.Fatalf("native %s absent: before=%+v after=%+v", label, before, after)
				}
			}
			if after.Counts["members"] != before.Counts["members"]+2 || after.Counts["groups"] != before.Counts["groups"] {
				t.Fatalf("wrong dependent member closure: %+v", after)
			}
			if after.Revision == before.Revision {
				t.Fatal("adding native children did not change revision")
			}
			if err := lifecycleImport(db, kind, reduced, false); err == nil {
				t.Fatal("native descendants deleted without replacement confirmation")
			}
			if err := lifecycleImport(db, kind, reduced, true); err == nil {
				t.Fatal("cross-origin deletion accepted without reviewed revision")
			}
			if err := lifecycleImport(db, kind, reduced, true, before.Revision); !errors.Is(err, ErrImportedEntityConflict) {
				t.Fatalf("native additions escaped old source preview: %v", err)
			}
			if err := lifecycleImport(db, kind, reduced, true, after.Revision); err != nil {
				t.Fatal(err)
			}
			for table, id := range map[string]int64{"upstream_models": model, "upstream_credentials": credential, "upstream_grants": grant, "upstream_group_items": member, "upstream_channels": channel} {
				var n int
				if err := db.Get(&n, db.Rebind("SELECT COUNT(*) FROM "+table+" WHERE id=?"), id); err != nil || n != 0 {
					t.Fatalf("closure retained %s: %d %v", table, n, err)
				}
			}
			var active int64
			if err := db.Get(&active, db.Rebind(`SELECT active_item_id FROM upstream_groups WHERE id=?`), foreignGroup); err != nil || active != 0 {
				t.Fatalf("foreign manual group was deleted or auto-reselected: %d %v", active, err)
			}
			var mappings int
			if err := db.Get(&mappings, `SELECT COUNT(*) FROM external_source_ids WHERE origin_key='other-origin'`); err != nil || mappings != 0 {
				t.Fatalf("foreign member mappings orphaned: %d %v", mappings, err)
			}
			if err := lifecycleImport(db, kind, reduced, false); err != nil {
				t.Fatalf("idempotent replacement: %v", err)
			}
			for i := 0; i < 2; i++ {
				if err := lifecycleImport(db, kind, full, false); err != nil {
					t.Fatalf("restore source snapshot %d: %v", i, err)
				}
			}
		})
	}
}

func TestSourceReimportRejectsDanglingOrForeignMappings(t *testing.T) {
	for _, kind := range []string{"octopus", "axonhub"} {
		for _, mutation := range []string{"dangling", "foreign"} {
			t.Run(kind+"/"+mutation, func(t *testing.T) {
				db := openAxonHubTestDB(t)
				full, _ := lifecycleSource(t, kind)
				if err := lifecycleImport(db, kind, full, false); err != nil {
					t.Fatal(err)
				}
				if mutation == "dangling" {
					if _, err := db.Exec(`DELETE FROM upstream_group_items`); err != nil {
						t.Fatal(err)
					}
				} else {
					if _, err := db.Exec(`UPDATE upstream_models SET origin_key='native:local'`); err != nil {
						t.Fatal(err)
					}
				}
				err := lifecycleImport(db, kind, full, false)
				if !errors.Is(err, ErrImportedEntityConflict) {
					t.Fatalf("invalid mapping reported success or wrong failure: %v", err)
				}
				if mutation == "dangling" {
					var n int
					if err := db.Get(&n, `SELECT COUNT(*) FROM upstream_group_items`); err != nil || n != 0 {
						t.Fatalf("failed import partially rebuilt dangling rows: %d %v", n, err)
					}
				}
			})
		}
	}
}

func TestSourceReimportDoesNotClaimNativeName(t *testing.T) {
	for _, kind := range []string{"octopus", "axonhub"} {
		t.Run(kind, func(t *testing.T) {
			db := openAxonHubTestDB(t)
			full, _ := lifecycleSource(t, kind)
			if err := lifecycleImport(db, kind, full, false); err != nil {
				t.Fatal(err)
			}
			var channel int64
			if err := db.Get(&channel, `SELECT id FROM upstream_channels WHERE source_id=11`); err != nil {
				t.Fatal(err)
			}
			native := lifecycleInsert(t, db, `INSERT INTO upstream_models(origin_key,source_id,channel_id,name,enabled) VALUES ('native:local',-900,?,'new-source-model',?)`, channel, true)
			var changed []byte
			if kind == "octopus" {
				d, err := ParseOctopusV5(full)
				if err != nil {
					t.Fatal(err)
				}
				d.Models = append(d.Models, octopusModel{ID: 901, ChannelID: 11, Name: "new-source-model"})
				d.Channels[0].Name = "must-roll-back"
				changed, _ = json.Marshal(d)
			} else {
				var obj map[string]any
				_ = json.Unmarshal(full, &obj)
				c := obj["channels"].([]any)[0].(map[string]any)
				c["name"] = "must-roll-back"
				c["supported_models"] = []string{"provider-model", "new-source-model"}
				obj["models"] = append(obj["models"].([]any), map[string]any{"id": 22, "model_id": "new-alias", "settings": map[string]any{"associations": []any{map[string]any{"type": "channel_model", "channelModel": map[string]any{"channelId": 11, "modelId": "new-source-model"}}}}})
				changed, _ = json.Marshal(obj)
			}
			if err := lifecycleImport(db, kind, changed, false); !errors.Is(err, ErrImportedEntityConflict) {
				t.Fatalf("native name collision not explicit: %v", err)
			}
			var name string
			if err := db.Get(&name, db.Rebind(`SELECT name FROM upstream_channels WHERE id=?`), channel); err != nil || name == "must-roll-back" {
				t.Fatalf("name conflict did not roll back earlier writes: %q %v", name, err)
			}
			var origin string
			if err := db.Get(&origin, db.Rebind(`SELECT origin_key FROM upstream_models WHERE id=?`), native); err != nil || origin != "native:local" {
				t.Fatalf("native ownership stolen: %q %v", origin, err)
			}
		})
	}
}

func TestAxonHubLifecycleDeleteClearsAliasesForReimport(t *testing.T) {
	db := openAxonHubTestDB(t)
	full, _ := lifecycleSource(t, "axonhub")
	if err := lifecycleImport(db, "axonhub", full, false); err != nil {
		t.Fatal(err)
	}
	var channel int64
	if err := db.Get(&channel, `SELECT id FROM upstream_channels WHERE source_id=12`); err != nil {
		t.Fatal(err)
	}
	p, err := upstream.PreviewDeletion(t.Context(), db.DB, upstream.KindChannel, channel)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = upstream.Delete(t.Context(), db.DB, upstream.KindChannel, channel, upstream.DeleteOptions{Cascade: true, ExpectedRevision: p.Revision}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = lifecycleImport(db, "axonhub", full, false); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	if err = db.Get(&n, `SELECT COUNT(*) FROM upstream_channels`); err != nil || n != 2 {
		t.Fatalf("AxonHub reimport duplicated graph: %d %v", n, err)
	}
	encoded, _ := json.Marshal(p)
	if strings.Contains(string(encoded), "fixture") || strings.Contains(string(encoded), "secret") {
		t.Fatalf("unsafe lifecycle preview: %s", encoded)
	}
}

func TestAxonHubLifecycleRevisionBindsSourceKeyDeletions(t *testing.T) {
	db := openAxonHubTestDB(t)
	payload := accessPayload(t)
	if _, err := ImportAxonHubV14(db, encodeAccessPayload(t, payload), "keys-only", false); err != nil {
		t.Fatal(err)
	}
	payload["api_keys"] = []any{}
	reduced := encodeAccessPayload(t, payload)
	before, err := PreviewAxonHubV14(db, reduced, "keys-only")
	if err != nil {
		t.Fatal(err)
	}
	if before.RemovalImpact == nil || before.Removals["downstream_api_keys"] != 1 {
		t.Fatalf("missing key-only deletion impact: %+v", before)
	}
	key := lifecycleInsert(t, db, `INSERT INTO downstream_api_keys(name,key) VALUES ('later-source-key','fixture-later-source-key')`)
	if _, err = db.Exec(db.Rebind(`INSERT INTO external_source_ids(origin_key,entity_type,source_id,target_id) VALUES ('keys-only','downstream_api_keys',999999,?)`), key); err != nil {
		t.Fatal(err)
	}
	if _, err = ImportAxonHubV14(db, reduced, "keys-only", true, before.RemovalImpact.Revision); !errors.Is(err, ErrImportedEntityConflict) {
		t.Fatalf("later source key escaped reviewed replacement: %v", err)
	}
	after, err := PreviewAxonHubV14(db, reduced, "keys-only")
	if err != nil {
		t.Fatal(err)
	}
	if after.RemovalImpact.Revision == before.RemovalImpact.Revision || after.Removals["downstream_api_keys"] != 2 {
		t.Fatal("source key identities not bound to replacement revision")
	}
	if _, err = ImportAxonHubV14(db, reduced, "keys-only", true, after.RemovalImpact.Revision); err != nil {
		t.Fatal(err)
	}
	var remaining int
	if err = db.Get(&remaining, `SELECT COUNT(*) FROM downstream_api_keys`); err != nil || remaining != 0 {
		t.Fatalf("confirmed source key deletion incomplete: %d %v", remaining, err)
	}
}
