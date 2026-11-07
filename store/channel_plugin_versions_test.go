package store_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/deliciousbuding/metapi-go/internal/pgtest"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestCapturedPluginVersionsAreImmutableBoundedAndNotPortable(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			dsn := filepath.Join(t.TempDir(), "versions.db")
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
			t.Cleanup(func() { _ = db.Close() })
			if dialect == store.DialectPostgres {
				pgtest.Reset(t, db.DB)
			}
			if err = store.AutoMigrate(db); err != nil {
				t.Fatal(err)
			}
			if slices.Contains(store.BackupTableNames(), "channel_plugin_versions") {
				t.Fatal("portable backup would install historical code")
			}
			archive := func(row store.ChannelPluginVersion) error {
				tx, err := db.BeginTxx(t.Context(), nil)
				if err != nil {
					return err
				}
				defer func() { _ = tx.Rollback() }()
				if err = store.LockChannelPluginBindings(t.Context(), tx); err != nil {
					return err
				}
				if err = store.ArchiveChannelPluginVersion(t.Context(), tx, row); err != nil {
					return err
				}
				return tx.Commit()
			}
			version := func(n int) store.ChannelPluginVersion {
				row := store.ChannelPluginVersion{Key: "fixture-version", Source: fmt.Sprintf("// captured %d\nexport const meta={};", n), Manifest: `{"key":"fixture-version"}`}
				digest := sha256.Sum256([]byte(row.Source + "\x00" + row.Manifest))
				row.Fingerprint = fmt.Sprintf("%x", digest)
				return row
			}
			first := version(0)
			for i := 0; i < 32; i++ {
				if err = archive(version(i)); err != nil {
					t.Fatal(err)
				}
			}
			if err = archive(first); err != nil {
				t.Fatalf("existing capture consumed another slot: %v", err)
			}
			if err = archive(version(32)); err == nil {
				t.Fatal("unbounded executable archive accepted")
			}
			otherVersion := func(n int) store.ChannelPluginVersion {
				row := version(n)
				row.Key = fmt.Sprintf("fixture-version-%d", n/32)
				row.Manifest = fmt.Sprintf(`{"key":%q}`, row.Key)
				row.Fingerprint = fmt.Sprintf("%x", sha256.Sum256([]byte(row.Source+"\x00"+row.Manifest)))
				return row
			}
			for i := 32; i < 512; i++ {
				if err = archive(otherVersion(i)); err != nil {
					t.Fatal(err)
				}
			}
			if err = archive(otherVersion(512)); err == nil {
				t.Fatal("global executable archive bound bypassed")
			}
			loaded, err := store.LoadChannelPluginVersion(t.Context(), db.DB, first.Key, first.Fingerprint)
			if err != nil || loaded.Source != first.Source || loaded.Manifest != first.Manifest {
				t.Fatal("immutable capture changed")
			}
			if _, err = db.Exec(db.Rebind(`UPDATE channel_plugin_versions SET source=? WHERE fingerprint=?`), "damaged", first.Fingerprint); err != nil {
				t.Fatal(err)
			}
			if err = archive(first); err == nil {
				t.Fatal("damaged archive falsely confirmed durable capture")
			}
		})
	}
}
