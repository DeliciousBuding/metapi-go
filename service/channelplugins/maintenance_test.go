package channelplugins_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/internal/channelplugin"
	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/store"
)

func maintenanceFixture(t *testing.T, db *store.DB) channelplugins.MaintenanceVersion {
	t.Helper()
	program, err := channelplugin.Compile(t.Context(), pruneModule)
	if err != nil {
		t.Fatal(err)
	}
	metadata, _ := json.Marshal(program.Manifest())
	if _, err = db.Exec(`INSERT INTO channel_plugin_versions(plugin_key,fingerprint,source,manifest) VALUES(?,?,?,?)`, "prune", program.Fingerprint(), pruneModule, string(metadata)); err != nil {
		t.Fatal(err)
	}
	identity, _ := json.Marshal(map[string]any{"version": 3, "driverKey": "prune", "driverFingerprint": program.Fingerprint(), "owner": "private-owner", "query": "https://private.example/task"})
	if _, err = db.Exec(`INSERT INTO proxy_video_tasks(public_id,upstream_video_id,site_url,token_value,direct_identity,accounting_state,created_at,updated_at) VALUES('video_direct_maintenance','','','',?,'{"version":1}','2000-01-01T00:00:00Z','2000-01-01T00:00:00Z')`, string(identity)); err != nil {
		t.Fatal(err)
	}
	return channelplugins.MaintenanceVersion{Key: "prune", Fingerprint: program.Fingerprint()}
}

func maintenancePlan(t *testing.T, db *store.DB, version channelplugins.MaintenanceVersion) channelplugins.MaintenancePlan {
	t.Helper()
	plan, err := channelplugins.InspectMaintenancePlan(t.Context(), db.DB, []channelplugins.MaintenanceVersion{version}, []string{"video_direct_maintenance"})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func assertMaintenanceRows(t *testing.T, db *store.DB, captures, archives int) {
	t.Helper()
	for _, item := range []struct {
		query string
		want  int
	}{
		{`SELECT COUNT(*) FROM proxy_video_tasks WHERE public_id='video_direct_maintenance'`, captures},
		{`SELECT COUNT(*) FROM channel_plugin_versions WHERE plugin_key='prune'`, archives},
	} {
		var got int
		if err := db.Get(&got, item.query); err != nil || got != item.want {
			t.Fatalf("maintenance rows: got=%d want=%d err=%v", got, item.want, err)
		}
	}
}

func TestOfflineMaintenanceLegacyArchiveAndPendingCapture(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := database(t, dialect)
			version := maintenanceFixture(t, db)
			installed, err := channelplugins.Save(t.Context(), db.DB, "prune", pruneModule, true, 0)
			if err != nil {
				t.Fatal(err)
			}
			plan := maintenancePlan(t, db, version)
			raw, _ := json.Marshal(plan)
			if strings.Contains(string(raw), "private-") || strings.Contains(string(raw), "private.example") || strings.Contains(string(raw), "export") || len(plan.Captures[0].SnapshotSHA256) != 64 {
				t.Fatal("inspect exposed private program/task data")
			}
			if err = channelplugins.PruneArchivedVersion(t.Context(), db.DB, version.Key, version.Fingerprint); !errors.Is(err, channelplugins.ErrVersionNotPrunable) {
				t.Fatalf("online deletion was expanded: %v", err)
			}
			if _, err = channelplugins.MaintainArchivedVersions(t.Context(), db.DB, plan, true, false); !errors.Is(err, channelplugins.ErrMaintenanceOffline) {
				t.Fatalf("missing offline acknowledgement: %v", err)
			}
			result, err := channelplugins.MaintainArchivedVersions(t.Context(), db.DB, plan, false, false)
			if err != nil || result.Applied || result.Archives != 1 || result.Captures != 1 {
				t.Fatalf("check: %+v %v", result, err)
			}
			assertMaintenanceRows(t, db, 1, 1)
			// A completed task protects the same archive even when the pending
			// capture has been explicitly selected for release.
			if _, err = db.Exec(`INSERT INTO proxy_video_tasks(public_id,upstream_video_id,site_url,token_value,direct_identity) SELECT 'video_direct_completed','vendor-completed','','',direct_identity FROM proxy_video_tasks WHERE public_id='video_direct_maintenance'`); err != nil {
				t.Fatal(err)
			}
			if _, err = channelplugins.MaintainArchivedVersions(t.Context(), db.DB, plan, true, true); !errors.Is(err, channelplugins.ErrVersionNotPrunable) {
				t.Fatalf("completed reference ignored: %v", err)
			}
			assertMaintenanceRows(t, db, 1, 1)
			if _, err = db.Exec(`DELETE FROM proxy_video_tasks WHERE public_id='video_direct_completed'`); err != nil {
				t.Fatal(err)
			}
			result, err = channelplugins.MaintainArchivedVersions(t.Context(), db.DB, plan, true, true)
			if err != nil || !result.Applied || result.Archives != 1 || result.Captures != 1 {
				t.Fatalf("apply: %+v %v", result, err)
			}
			assertMaintenanceRows(t, db, 0, 0)
			current, err := store.LoadChannelPlugin(t.Context(), db.DB, "prune")
			if err != nil || current != installed {
				t.Fatal("maintenance altered the current installation")
			}
			if _, err = channelplugins.MaintainArchivedVersions(t.Context(), db.DB, plan, true, true); err == nil {
				t.Fatal("stale one-shot plan claimed new success")
			}
			// A legacy program with no task references can be reclaimed without
			// deleting the currently installed copy or expanding online rights.
			maintenanceFixture(t, db)
			if _, err = db.Exec(`DELETE FROM proxy_video_tasks WHERE public_id='video_direct_maintenance'`); err != nil {
				t.Fatal(err)
			}
			archiveOnly := channelplugins.MaintenancePlan{Archives: []channelplugins.MaintenanceVersion{version}}
			if err = channelplugins.PruneArchivedVersion(t.Context(), db.DB, version.Key, version.Fingerprint); !errors.Is(err, channelplugins.ErrVersionNotPrunable) {
				t.Fatalf("legacy online contract changed: %v", err)
			}
			if _, err = channelplugins.MaintainArchivedVersions(t.Context(), db.DB, archiveOnly, true, true); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestOfflineMaintenanceRejectsChangedAndUnsettledSelection(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := database(t, dialect)
			for _, query := range []string{
				`UPDATE proxy_video_tasks SET upstream_video_id='confirmed'`,
				`UPDATE proxy_video_tasks SET updated_at='2001-01-01T00:00:00Z'`,
				`UPDATE proxy_video_tasks SET accounting_state='{"version":1,"initialized":true}'`,
				`UPDATE proxy_video_tasks SET accounting_state='{"version":1,"usage":{"totalTokens":7}}'`,
				`UPDATE proxy_video_tasks SET accounting_state='{"version":1,"future":true}'`,
				`UPDATE proxy_video_tasks SET owner='another-owner'`,
				`UPDATE proxy_video_tasks SET site_url='https://private.example'`,
				`UPDATE proxy_video_tasks SET token_value='synthetic-private-value'`,
				`UPDATE proxy_video_tasks SET direct_identity='{invalid'`,
				`UPDATE channel_plugin_versions SET source='broken program'`,
				`UPDATE channel_plugin_versions SET manifest='{}'`,
				`UPDATE proxy_video_tasks SET id=id+10000`,
			} {
				t.Run(query, func(t *testing.T) {
					version := maintenanceFixture(t, db)
					plan := maintenancePlan(t, db, version)
					if _, err := db.Exec(query); err != nil {
						t.Fatal(err)
					}
					if _, err := channelplugins.MaintainArchivedVersions(t.Context(), db.DB, plan, true, true); err == nil {
						t.Fatal("changed or corrupt row was reclaimed")
					}
					assertMaintenanceRows(t, db, 1, 1)
					if _, err := db.Exec(`DELETE FROM proxy_video_tasks WHERE public_id='video_direct_maintenance'`); err != nil {
						t.Fatal(err)
					}
					if _, err := db.Exec(`DELETE FROM channel_plugin_versions WHERE plugin_key='prune'`); err != nil {
						t.Fatal(err)
					}
				})
			}
			version := maintenanceFixture(t, db)
			plan := maintenancePlan(t, db, version)
			for _, invalid := range []channelplugins.MaintenancePlan{
				{}, {Captures: append(plan.Captures, plan.Captures...)}, {Archives: append(plan.Archives, plan.Archives...)},
				{Captures: []channelplugins.MaintenanceCapture{{ID: "../unrelated", SnapshotSHA256: strings.Repeat("0", 64)}}},
				{Archives: []channelplugins.MaintenanceVersion{{Key: "prune", Fingerprint: "invalid"}}},
			} {
				if _, err := channelplugins.MaintainArchivedVersions(t.Context(), db.DB, invalid, true, true); !errors.Is(err, channelplugins.ErrMaintenancePlan) {
					t.Fatalf("invalid plan accepted: %v", err)
				}
			}
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			if _, err := channelplugins.MaintainArchivedVersions(ctx, db.DB, plan, true, true); err == nil {
				t.Fatal("canceled maintenance succeeded")
			}
			assertMaintenanceRows(t, db, 1, 1)
		})
	}
}

func TestOfflineMaintenanceCommitFailurePreservesCaptureAndArchive(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := database(t, dialect)
			version := maintenanceFixture(t, db)
			plan := maintenancePlan(t, db, version)
			if _, err := db.Exec(`CREATE TABLE maintenance_guard(fp TEXT REFERENCES channel_plugin_versions(fingerprint) DEFERRABLE INITIALLY DEFERRED)`); err != nil {
				t.Fatal(err)
			}
			defer func() { _, _ = db.Exec(`DROP TABLE maintenance_guard`) }()
			if _, err := db.Exec(`INSERT INTO maintenance_guard(fp) VALUES(?)`, version.Fingerprint); err != nil {
				t.Fatal(err)
			}
			if _, err := channelplugins.MaintainArchivedVersions(t.Context(), db.DB, plan, false, false); err != nil {
				t.Fatal(err)
			}
			if _, err := channelplugins.MaintainArchivedVersions(t.Context(), db.DB, plan, true, true); err == nil {
				t.Fatal("deferred FK commit failure was hidden")
			}
			assertMaintenanceRows(t, db, 1, 1)
		})
	}
}

func TestOfflineMaintenanceRechecksLateConfirmationAcrossConnections(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := database(t, dialect)
			version := maintenanceFixture(t, db)
			plan := maintenancePlan(t, db, version)
			dsn := os.Getenv("PG_TEST_DSN")
			if dialect == store.DialectSQLite {
				if err := db.Get(&dsn, `SELECT file FROM pragma_database_list WHERE name='main'`); err != nil {
					t.Fatal(err)
				}
			}
			peer, err := store.Open(dialect, dsn, false)
			if err != nil {
				t.Fatal(err)
			}
			defer peer.Close()
			tx, err := db.BeginTxx(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback() }()
			if _, err = tx.ExecContext(t.Context(), `UPDATE proxy_video_tasks SET upstream_video_id='late-confirmation' WHERE public_id='video_direct_maintenance'`); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() { _, err := channelplugins.MaintainArchivedVersions(ctx, peer.DB, plan, true, true); done <- err }()
			select {
			case err := <-done:
				t.Fatalf("maintenance escaped pending row lock: %v", err)
			case <-time.After(100 * time.Millisecond):
			}
			if err = tx.Commit(); err != nil {
				t.Fatal(err)
			}
			if err = <-done; !errors.Is(err, channelplugins.ErrMaintenanceCapture) {
				t.Fatalf("late confirmation was reclaimed: %v", err)
			}
			assertMaintenanceRows(t, db, 1, 1)
		})
	}
}
