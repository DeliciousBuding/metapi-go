package service

import (
	"context"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
)

// ---- K1b: registry reload from model_name_redirects ----

func TestReloadRedirectRegistry_LoadsFromDB(t *testing.T) {
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Ensure a clean registry for this test.
	routing.SetModelRedirects(nil)
	t.Cleanup(func() { routing.SetModelRedirects(nil) })

	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(`INSERT INTO sites (name, url, platform, status, created_at, updated_at)
		VALUES (?, ?, 'anthropic', 'active', ?, ?)`, "redirect-site", "https://redirect.example.test", now, now)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	siteID, _ := res.LastInsertId()

	res, err = db.Exec(`INSERT INTO accounts (site_id, username, access_token, status, checkin_enabled, created_at, updated_at)
		VALUES (?, ?, 'tok', 'active', 1, ?, ?)`, siteID, "redirect-user", now, now)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	accountID, _ := res.LastInsertId()

	if _, err := db.Exec(`INSERT INTO model_name_redirects (account_id, canonical, actual, source, created_at, updated_at)
		VALUES (?, ?, ?, 'sync', ?, ?)`, accountID, "claude-3-5-sonnet", "claude-3-5-sonnet-20241022", now, now); err != nil {
		t.Fatalf("insert redirect: %v", err)
	}

	ReloadRedirectRegistry(context.Background(), db.DB)

	if got := routing.ModelRedirectActual(accountID, "claude-3-5-sonnet"); got != "claude-3-5-sonnet-20241022" {
		t.Fatalf("ModelRedirectActual = %q, want claude-3-5-sonnet-20241022", got)
	}
	if got := routing.ModelRedirectCanonical(accountID, "claude-3-5-sonnet-20241022"); got != "claude-3-5-sonnet" {
		t.Fatalf("ModelRedirectCanonical = %q, want claude-3-5-sonnet", got)
	}
	// Unknown account / unknown model → no mapping.
	if got := routing.ModelRedirectActual(999, "claude-3-5-sonnet"); got != "" {
		t.Fatalf("unknown account mapped to %q, want empty", got)
	}
	if got := routing.ModelRedirectActual(accountID, "gpt-4o"); got != "" {
		t.Fatalf("unknown model mapped to %q, want empty", got)
	}

	// After deleting the row and reloading, the registry forgets it.
	if _, err := db.Exec(`DELETE FROM model_name_redirects WHERE account_id = ?`, accountID); err != nil {
		t.Fatalf("delete redirect: %v", err)
	}
	ReloadRedirectRegistry(context.Background(), db.DB)
	if got := routing.ModelRedirectActual(accountID, "claude-3-5-sonnet"); got != "" {
		t.Fatalf("stale mapping %q after delete+reload, want empty", got)
	}
}
