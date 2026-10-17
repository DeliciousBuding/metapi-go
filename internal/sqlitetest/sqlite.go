// Package sqlitetest provides isolated, migrated SQLite fixtures for business
// tests. Migration and database-opening tests must use store.Open directly.
package sqlitetest

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

// Each test process builds its own template from the production migrations.
// The driver copies the image into each connection; no mutable DB is shared.
var schemaImage = sync.OnceValues(func() ([]byte, error) {
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err := store.AutoMigrate(db); err != nil {
		return nil, err
	}
	var image []byte
	err = withConnection(db, func(raw any) error {
		serializer, ok := raw.(interface{ Serialize() ([]byte, error) })
		if !ok {
			return fmt.Errorf("SQLite driver %T does not support serialization", raw)
		}
		image, err = serializer.Serialize()
		return err
	})
	return image, err
})

// Open returns a fresh in-memory database with the current production schema.
// Its connection settings still come from store.Open, and it closes at cleanup.
func Open(t testing.TB) *store.DB {
	t.Helper()
	db, err := open()
	if err != nil {
		t.Fatalf("open SQLite fixture: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close SQLite fixture: %v", err)
		}
	})
	return db
}

func open() (*store.DB, error) {
	image, err := schemaImage()
	if err != nil {
		return nil, fmt.Errorf("build schema template: %w", err)
	}
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		return nil, err
	}
	// SQLite deserialize replaces the main database's pager and resets these
	// settings. Preserve store.Open's values rather than duplicating its policy.
	settings := make(map[string]int, 2)
	for _, name := range []string{"synchronous", "cache_size"} {
		var value int
		if err := db.Get(&value, "PRAGMA "+name); err != nil {
			return nil, errors.Join(err, db.Close())
		}
		settings[name] = value
	}
	if err := withConnection(db, func(raw any) error {
		restorer, ok := raw.(interface{ Deserialize([]byte) error })
		if !ok {
			return fmt.Errorf("SQLite driver %T does not support deserialization", raw)
		}
		return restorer.Deserialize(image)
	}); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	for name, value := range settings {
		if _, err := db.Exec(fmt.Sprintf("PRAGMA %s = %d", name, value)); err != nil {
			return nil, errors.Join(err, db.Close())
		}
	}
	return db, nil
}

func withConnection(db *store.DB, fn func(any) error) error {
	conn, err := db.Conn(context.Background())
	if err != nil {
		return err
	}
	err = conn.Raw(fn)
	return errors.Join(err, conn.Close())
}
