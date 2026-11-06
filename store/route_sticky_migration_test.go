package store

import (
	"strings"
	"testing"
)

func TestRouteTraceStickyMigrationKeepsLegacyDisabled(t *testing.T) {
	db, err := Open(DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ddl := strings.ReplaceAll(buildTokenRoutesDDL(DialectSQLite), "trace_sticky_mode TEXT NOT NULL DEFAULT 'disabled',", "")
	if _, err := db.Exec(ddl); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO token_routes(model_pattern,enabled) VALUES('legacy',1)`); err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	var mode string
	if err := db.Get(&mode, `SELECT trace_sticky_mode FROM token_routes`); err != nil || mode != "disabled" {
		t.Fatalf("legacy affinity changed: %q %v", mode, err)
	}
	if _, err := db.Exec(`UPDATE token_routes SET trace_sticky_mode='prefer_previous_channel'`); err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&mode, `SELECT trace_sticky_mode FROM token_routes`); err != nil || mode != "prefer_previous_channel" {
		t.Fatalf("migration reset policy: %q %v", mode, err)
	}
}
