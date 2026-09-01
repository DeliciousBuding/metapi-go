package events_test

import (
	"encoding/json"
	"testing"

	"github.com/deliciousbuding/metapi-go/service/events"
	"github.com/deliciousbuding/metapi-go/store"
)

func newTestDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

// TestWriteEventPersistsStructuredRow verifies the registry write path:
// a structured event is persisted with the legacy English title/message
// (non-UI consumer fallback) AND titleKey + params JSON alongside.
func TestWriteEventPersistsStructuredRow(t *testing.T) {
	db := newTestDB(t)

	err := events.WriteEvent(db.DB, events.Ref{
		Key: "checkinSuccess",
		Params: map[string]any{
			"account": "user@example.com",
			"site":    "NewAPI 公益站",
			"reward":  "$1.23",
		},
	}, events.Options{Level: "info", RelatedID: 7, RelatedType: "account"})
	if err != nil {
		t.Fatalf("WriteEvent: %v", err)
	}

	var title, message, titleKey, params string
	err = db.QueryRow(
		`SELECT title, message, title_key, params FROM events ORDER BY id DESC LIMIT 1`,
	).Scan(&title, &message, &titleKey, &params)
	if err != nil {
		t.Fatalf("read row: %v", err)
	}
	if title != "checkin success" {
		t.Fatalf("title = %q, want legacy English 'checkin success'", title)
	}
	if message != "user@example.com @ NewAPI 公益站: $1.23" {
		t.Fatalf("message = %q, want rendered %q", message, "user@example.com @ NewAPI 公益站: $1.23")
	}
	if titleKey != "checkinSuccess" {
		t.Fatalf("title_key = %q, want %q", titleKey, "checkinSuccess")
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(params), &parsed); err != nil {
		t.Fatalf("params not JSON: %v", err)
	}
	if parsed["account"] != "user@example.com" || parsed["site"] != "NewAPI 公益站" {
		t.Fatalf("params = %v, want account/site", parsed)
	}
}

// TestWriteEventRejectsUnknownKey: an unregistered key must fail loudly at
// write time instead of silently persisting a row with no structured data.
func TestWriteEventRejectsUnknownKey(t *testing.T) {
	db := newTestDB(t)
	err := events.WriteEvent(db.DB, events.Ref{Key: "noSuchEvent", Params: nil},
		events.Options{Level: "info"})
	if err == nil {
		t.Fatal("WriteEvent with unknown key: want error, got nil")
	}
}

// TestWriteEventValidatesParams: a required param missing / an undeclared
// param present are both rejected (typo protection).
func TestWriteEventValidatesParams(t *testing.T) {
	db := newTestDB(t)

	if err := events.WriteEvent(db.DB, events.Ref{Key: "checkinSuccess", Params: map[string]any{
		"account": "a",
		// site required — missing
		"reward": "1",
	}}, events.Options{Level: "info"}); err == nil {
		t.Fatal("missing required param: want error")
	}

	if err := events.WriteEvent(db.DB, events.Ref{Key: "checkinSuccess", Params: map[string]any{
		"account": "a",
		"site":    "s",
		"bogus":   "x",
	}}, events.Options{Level: "info"}); err == nil {
		t.Fatal("undeclared param: want error")
	}
}

// TestRegistryKeysCoverFrontendLocales: every registered key must have a
// locale entry in BOTH en and zh-CN. The frontend mirrors this list (the
// i18n-existence test), so a registry/UI key drift fails on both sides.
func TestRegistryKeysCoverFrontendLocales(t *testing.T) {
	// The frontend mirrors this same list (web/.../events-structured.test.ts
	// REGISTRY_KEYS); pin the exact checkin-batch-1 set so a redefined or
	// mistyped key fails on BOTH sides.
	keys := events.Keys()
	want := map[string]bool{
		"checkinSuccess":          true,
		"checkinFailed":           true,
		"checkinFailedCloudflare": true,
		"checkinSkipped":          true,
	}
	if len(keys) != len(want) {
		t.Fatalf("registry keys = %v, want exactly checkin batch 1", keys)
	}
	for _, key := range keys {
		if !want[key] {
			t.Fatalf("unexpected registry key %q (frontend locale sync required)", key)
		}
	}
}
