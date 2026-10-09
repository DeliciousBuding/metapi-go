package backup

import (
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/deliciousbuding/metapi-go/internal/pgtest"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestOctopusSnapshotRemovesStaleSourceGraphOnly(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			dsn := ":memory:"
			if dialect == store.DialectPostgres {
				dsn = os.Getenv("PG_TEST_DSN")
				if dsn == "" {
					t.Skip("PG_TEST_DSN not set")
				}
			}
			db, err := store.Open(dialect, dsn, false)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if dialect == store.DialectPostgres {
				pgtest.Reset(t, db.DB)
			}
			testOctopusSnapshot(t, db)
		})
	}
}

func testOctopusSnapshot(t *testing.T, db *store.DB) {
	t.Helper()
	err := store.AutoMigrate(db)
	if err != nil {
		t.Fatal(err)
	}
	original, err := ParseOctopusV5([]byte(octopusV5RealShapeFixture))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ImportOctopusV5(db, []byte(octopusV5RealShapeFixture), "source-a"); err != nil {
		t.Fatal(err)
	}
	other, err := ParseOctopusV5([]byte(octopusV5RealShapeFixture))
	if err != nil {
		t.Fatal(err)
	}
	other.Groups[0].Name = "other-origin-model"
	rawOther, _ := json.Marshal(other)
	if _, err := ImportOctopusV5(db, rawOther, "source-b"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO token_routes (model_pattern,route_mode,routing_strategy,enabled,created_at,updated_at) VALUES ('native-model','pattern','weighted',?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, true); err != nil {
		t.Fatal(err)
	}
	original.Channels = original.Channels[:1]
	original.Credentials = original.Credentials[:1]
	original.Models = original.Models[:1]
	original.Grants = original.Grants[:1]
	original.GroupItems = original.GroupItems[:1]
	reduced, _ := json.Marshal(original)
	preview, err := PreviewOctopusV5(db, reduced, "source-a")
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"channels", "channelKeys", "channelModels", "channelGrants", "groupItems"} {
		if preview.Removals[kind] != 1 {
			t.Fatalf("removal preview for %s=%d", kind, preview.Removals[kind])
		}
	}
	var total int
	if err := db.Get(&total, `SELECT COUNT(*) FROM upstream_channels`); err != nil || total != 4 {
		t.Fatalf("preview mutated channels: %d %v", total, err)
	}
	if _, err := ImportOctopusV5(db, reduced, "source-a"); !errors.Is(err, ErrOctopusReplacementRequired) {
		t.Fatalf("service accepted unconfirmed source replacement: %v", err)
	}
	if err := db.Get(&total, `SELECT COUNT(*) FROM upstream_channels`); err != nil || total != 4 {
		t.Fatalf("denied replacement mutated channels: %d %v", total, err)
	}
	if _, err := ImportOctopusV5WithUnsupportedMode(db, reduced, "source-a", false, true); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"upstream_channels", "upstream_credentials", "upstream_models", "upstream_grants", "upstream_group_items"} {
		if err := db.Get(&total, "SELECT COUNT(*) FROM "+table+" WHERE origin_key=?", "source-a"); err != nil || total != 1 {
			t.Fatalf("%s retained stale source entries: %d %v", table, total, err)
		}
		if err := db.Get(&total, "SELECT COUNT(*) FROM "+table+" WHERE origin_key=?", "source-b"); err != nil || total != 2 {
			t.Fatalf("%s altered another origin: %d %v", table, total, err)
		}
	}
	original.Groups = original.Groups[:0]
	original.GroupItems = original.GroupItems[:0]
	rawUngrouped, _ := json.Marshal(original)
	var routeID, groupID int64
	if err := db.Get(&routeID, `SELECT target_id FROM external_source_ids WHERE origin_key=? AND entity_type='token_routes'`, "source-a"); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&groupID, `SELECT target_id FROM external_source_ids WHERE origin_key=? AND entity_type='groups'`, "source-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM upstream_route_groups WHERE route_id=?`, routeID); err != nil {
		t.Fatal(err)
	}
	original.Channels[0].Name = "must-roll-back"
	rawChanged, _ := json.Marshal(original)
	if _, err := ImportOctopusV5WithUnsupportedMode(db, rawChanged, "source-a", false, true); err == nil {
		t.Fatal("replacement accepted a route no longer owned by this origin")
	}
	if err := db.Get(&total, `SELECT COUNT(*) FROM upstream_channels WHERE name='must-roll-back'`); err != nil || total != 0 {
		t.Fatalf("failed replacement did not roll back upserts: %d %v", total, err)
	}
	if _, err := db.Exec(`INSERT INTO upstream_route_groups (route_id,group_id) VALUES (?,?)`, routeID, groupID); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportOctopusV5WithUnsupportedMode(db, rawUngrouped, "source-a", false, true); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&total, `SELECT COUNT(*) FROM external_source_ids WHERE origin_key=? AND entity_type IN ('groups','group_items','token_routes')`, "source-a"); err != nil || total != 0 {
		t.Fatalf("stale source mappings retained: %d %v", total, err)
	}
	if err := db.Get(&total, `SELECT COUNT(*) FROM token_routes WHERE model_pattern IN ('native-model','other-origin-model')`); err != nil || total != 2 {
		t.Fatalf("native/other-origin route removed: %d %v", total, err)
	}
	if err := db.Get(&total, `SELECT COUNT(*) FROM token_routes WHERE model_pattern='client-model'`); err != nil || total != 0 {
		t.Fatalf("removed source alias still advertised: %d %v", total, err)
	}
}
