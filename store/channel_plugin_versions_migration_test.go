package store_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/deliciousbuding/metapi-go/internal/pgtest"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestCapturedPluginVersionsSurviveBothDialectMigrations(t *testing.T) {
	pgDSN := os.Getenv("PG_TEST_DSN")
	if pgDSN == "" {
		t.Skip("PG_TEST_DSN not set")
	}
	pg, err := store.Open(store.DialectPostgres, pgDSN, false)
	if err != nil {
		t.Fatal(err)
	}
	pgtest.Reset(t, pg.DB)
	_ = pg.Close()
	sourcePath := filepath.Join(t.TempDir(), "source.db")
	restoredPath := filepath.Join(t.TempDir(), "restored.db")
	db, err := store.Open(store.DialectSQLite, sourcePath, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	source, manifest := "export const meta={};", `{"key":"migration-fixture"}`
	fingerprint := fmt.Sprintf("%x", sha256.Sum256([]byte(source+"\x00"+manifest)))
	tx, err := db.BeginTxx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.LockChannelPluginBindings(t.Context(), tx); err != nil {
		t.Fatal(err)
	}
	if err = store.ArchiveChannelPluginVersion(t.Context(), tx, store.ChannelPluginVersion{Key: "migration-fixture", Fingerprint: fingerprint, Source: source, Manifest: manifest}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	for _, pair := range [][2]string{{sourcePath, pgDSN}, {pgDSN, restoredPath}} {
		if _, err = store.RunMigration(store.RunMigrationOptions{FromPath: pair[0], ToURL: pair[1], Overwrite: true, Verify: true}); err != nil {
			t.Fatal(err)
		}
	}
	restored, err := store.Open(store.DialectSQLite, restoredPath, false)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	row, err := store.LoadChannelPluginVersion(t.Context(), restored.DB, "migration-fixture", fingerprint)
	if err != nil || row.Source != source || row.Manifest != manifest {
		t.Fatalf("historical executable lost in dialect cutover: %v", err)
	}
}
