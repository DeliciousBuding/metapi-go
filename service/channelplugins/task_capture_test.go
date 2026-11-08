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
	"github.com/jmoiron/sqlx"
)

func insertManagerCapture(ctx context.Context, tx *sqlx.Tx, key, fingerprint, vendorID, identity string) (string, error) {
	if identity == "" {
		raw, _ := json.Marshal(map[string]any{"version": 3, "driverKey": key, "driverFingerprint": fingerprint})
		identity = string(raw)
	}
	_, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO proxy_video_tasks(public_id,upstream_video_id,site_url,token_value,direct_identity) VALUES('manager-capture',?,'','',?)`), vendorID, identity)
	return "manager-capture", err
}

func TestTaskCaptureRejectsMissingOrMismatchedPendingReference(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, mode := range []string{"missing", "callback-error", "wrong-key", "wrong-fingerprint", "wrong-version", "confirmed"} {
			t.Run(dialect+"/"+mode, func(t *testing.T) {
				db := database(t, dialect)
				source := strings.Replace(pruneModule, `apiVersion:2,`, `apiVersion:2,taskCapture:"durable",`, 1)
				if _, err := channelplugins.Save(t.Context(), db.DB, "prune", source, true, 0); err != nil {
					t.Fatal(err)
				}
				binding := store.DirectProviderBinding{Key: "prune", Protocols: []string{"video"}}
				manager := channelplugins.New(db.DB)
				if _, err := manager.ResolveTaskDriver(t.Context(), binding, "video", "api_key", ""); !errors.Is(err, channelplugins.ErrBinding) {
					t.Fatalf("durable program accepted an unpinned resolver: %v", err)
				}
				_, err := manager.CaptureTaskDriver(t.Context(), binding, "video", "api_key", "json", "", func(tx *sqlx.Tx, fingerprint string) (string, error) {
					if mode == "missing" {
						return "missing-capture", nil
					}
					key, version, vendorID := "prune", 3, ""
					if mode == "wrong-key" {
						key = "different"
					}
					if mode == "wrong-fingerprint" {
						fingerprint = strings.Repeat("a", 64)
					}
					if mode == "wrong-version" {
						version = 2
					}
					if mode == "confirmed" {
						vendorID = "existing-task"
					}
					raw, _ := json.Marshal(map[string]any{"version": version, "driverKey": key, "driverFingerprint": fingerprint})
					id, err := insertManagerCapture(t.Context(), tx, key, fingerprint, vendorID, string(raw))
					if err == nil && mode == "callback-error" {
						err = errors.New("synthetic capture failure")
					}
					return id, err
				})
				if err == nil {
					t.Fatal("invalid capture was published")
				}
				for _, table := range []string{"channel_plugin_versions", "proxy_video_tasks"} {
					var count int
					if err := db.Get(&count, "SELECT COUNT(*) FROM "+table); err != nil || count != 0 {
						t.Fatalf("failed capture left half a transaction in %s: %d %v", table, count, err)
					}
				}
			})
		}
	}
}

func TestTaskCaptureCommitFailureRollsBackArchiveAndReference(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := database(t, dialect)
			for _, query := range []string{
				`CREATE TABLE capture_parent(id INTEGER PRIMARY KEY)`,
				`CREATE TABLE capture_guard(parent_id INTEGER REFERENCES capture_parent(id) DEFERRABLE INITIALLY DEFERRED)`,
			} {
				if _, err := db.Exec(query); err != nil {
					t.Fatal(err)
				}
			}
			defer func() { _, _ = db.Exec(`DROP TABLE capture_guard`); _, _ = db.Exec(`DROP TABLE capture_parent`) }()
			source := strings.Replace(pruneModule, `apiVersion:2,`, `apiVersion:2,taskCapture:"durable",`, 1)
			if _, err := channelplugins.Save(t.Context(), db.DB, "prune", source, true, 0); err != nil {
				t.Fatal(err)
			}
			binding := store.DirectProviderBinding{Key: "prune", Protocols: []string{"video"}}
			callbackFinished := false
			_, err := channelplugins.New(db.DB).CaptureTaskDriver(t.Context(), binding, "video", "api_key", "json", "", func(tx *sqlx.Tx, fingerprint string) (string, error) {
				id, err := insertManagerCapture(t.Context(), tx, "prune", fingerprint, "", "")
				if err == nil {
					_, err = tx.ExecContext(t.Context(), `INSERT INTO capture_guard(parent_id) VALUES(1)`)
				}
				callbackFinished = err == nil
				return id, err
			})
			if !callbackFinished || err == nil {
				t.Fatalf("fixture did not fail at commit: callback=%v err=%v", callbackFinished, err)
			}
			var count int
			if err := db.Get(&count, `SELECT COUNT(*) FROM channel_plugin_versions`); err != nil || count != 0 {
				t.Fatalf("failed commit retained executable: %d %v", count, err)
			}
			if err := db.Get(&count, `SELECT COUNT(*) FROM proxy_video_tasks`); err != nil || count != 0 {
				t.Fatalf("failed commit retained a capture: %d %v", count, err)
			}
		})
	}
}

func TestTaskCaptureAndPruneSerializeAcrossIndependentConnections(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := database(t, dialect)
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
			source := strings.Replace(pruneModule, `apiVersion:2,`, `apiVersion:2,taskCapture:"durable",`, 1)
			if _, err := channelplugins.Save(t.Context(), db.DB, "prune", source, true, 0); err != nil {
				t.Fatal(err)
			}
			program, err := channelplugin.Compile(t.Context(), source)
			if err != nil {
				t.Fatal(err)
			}
			metadata, _ := json.Marshal(program.Manifest())
			tx, err := db.BeginTxx(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			if err = store.LockChannelPluginBindings(t.Context(), tx); err == nil {
				err = store.ArchiveChannelPluginVersion(t.Context(), tx, store.ChannelPluginVersion{Key: "prune", Fingerprint: program.Fingerprint(), Source: source, Manifest: string(metadata)})
			}
			if err != nil {
				_ = tx.Rollback()
				t.Fatal(err)
			}
			if err = tx.Commit(); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			entered, release := make(chan struct{}), make(chan struct{})
			captureDone, pruneDone := make(chan error, 1), make(chan error, 1)
			binding := store.DirectProviderBinding{Key: "prune", Protocols: []string{"video"}}
			go func() {
				_, err := channelplugins.New(db.DB).CaptureTaskDriver(ctx, binding, "video", "api_key", "json", "", func(tx *sqlx.Tx, fingerprint string) (string, error) {
					id, err := insertManagerCapture(ctx, tx, "prune", fingerprint, "", "")
					close(entered)
					select {
					case <-release:
					case <-ctx.Done():
						return "", ctx.Err()
					}
					return id, err
				})
				captureDone <- err
			}()
			select {
			case <-entered:
			case err := <-captureDone:
				t.Fatalf("capture failed before its transaction barrier: %v", err)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			go func() { pruneDone <- channelplugins.PruneArchivedVersion(ctx, peer.DB, "prune", program.Fingerprint()) }()
			select {
			case err := <-pruneDone:
				close(release)
				t.Fatalf("prune escaped the capture transaction: %v", err)
			case <-time.After(100 * time.Millisecond):
			}
			close(release)
			if err := <-captureDone; err != nil {
				t.Fatal(err)
			}
			if err := <-pruneDone; !errors.Is(err, channelplugins.ErrVersionNotPrunable) {
				t.Fatalf("committed capture did not protect archive: %v", err)
			}
		})
	}
}

func TestTaskCapturePinsArchivedProgramWithoutArchivingNewInstallation(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, mode := range []string{"success", "rollback", "disabled", "missing-archive", "wrong-kind"} {
			t.Run(dialect+"/"+mode, func(t *testing.T) {
				db := database(t, dialect)
				if _, err := channelplugins.Save(t.Context(), db.DB, "prune", pruneModule, true, 0); err != nil {
					t.Fatal(err)
				}
				binding := store.DirectProviderBinding{Key: "prune", Protocols: []string{"video"}}
				manager := channelplugins.New(db.DB)
				old, err := manager.ResolveTaskDriver(t.Context(), binding, "video", "api_key", "")
				if err != nil {
					t.Fatal(err)
				}
				upgraded := strings.Replace(pruneModule, `version:"1.0.0"`, `version:"1.0.1"`, 1)
				if upgraded == pruneModule {
					t.Fatal("fixture did not upgrade the installation")
				}
				if _, err := channelplugins.Save(t.Context(), db.DB, "prune", upgraded, mode != "disabled", 1); err != nil {
					t.Fatal(err)
				}
				if mode == "missing-archive" {
					if _, err := db.Exec(`DELETE FROM channel_plugin_versions`); err != nil {
						t.Fatal(err)
					}
				}
				called := false
				kind := "api_key"
				if mode == "wrong-kind" {
					kind = "none"
				}
				pinned, err := manager.CaptureTaskDriver(t.Context(), binding, "video", kind, "json", old.Fingerprint(), func(tx *sqlx.Tx, fingerprint string) (string, error) {
					called = true
					if fingerprint != old.Fingerprint() {
						return "", errors.New("capture switched program")
					}
					id, err := insertManagerCapture(t.Context(), tx, "prune", fingerprint, "", "")
					if err == nil && mode == "rollback" {
						err = errors.New("synthetic capture failure")
					}
					return id, err
				})
				if mode == "success" {
					if err != nil || !called || pinned.Fingerprint() != old.Fingerprint() {
						t.Fatalf("archive not pinned: called=%t %v", called, err)
					}
				} else if err == nil || called != (mode == "rollback") {
					t.Fatalf("capture ignored current eligibility/rollback: called=%t %v", called, err)
				}
				var captures, archives int
				if err := db.Get(&captures, `SELECT COUNT(*) FROM proxy_video_tasks`); err != nil {
					t.Fatal(err)
				}
				if err := db.Get(&archives, `SELECT COUNT(*) FROM channel_plugin_versions`); err != nil {
					t.Fatal(err)
				}
				wantCaptures, wantArchives := 0, 1
				if mode == "success" {
					wantCaptures = 1
				}
				if mode == "missing-archive" {
					wantArchives = 0
				}
				if captures != wantCaptures || archives != wantArchives {
					t.Fatalf("half capture or unused installation archived: captures=%d archives=%d", captures, archives)
				}
			})
		}
	}
}
