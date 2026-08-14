package oauth

import (
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

// ---- ListOauthConnections ----

func TestListOauthConnections_NoDatabase(t *testing.T) {
	store.CloseDatabase()
	t.Cleanup(func() { _ = store.CloseDatabase() })

	_, err := ListOauthConnections(ListConnectionsInput{Limit: 10})
	if err == nil {
		t.Fatal("expected error when database is not initialized")
	}
	if !strings.Contains(err.Error(), "database not initialized") {
		t.Errorf("error = %v", err)
	}
}

func TestListOauthConnections_EmptyDB(t *testing.T) {
	_, cleanup := setupTestDB(t)
	defer cleanup()

	result, err := ListOauthConnections(ListConnectionsInput{Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Total != 0 {
		t.Errorf("Total = %d, want 0 for empty DB", result.Total)
	}
	if len(result.Items) != 0 {
		t.Errorf("Items len = %d, want 0", len(result.Items))
	}
}

func TestListOauthConnections_ReturnsInsertedAccount(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Insert a site + codex account.
	now := "2026-01-01T00:00:00Z"
	_, err := db.Exec(
		`INSERT INTO sites (name, url, platform, status, created_at, updated_at)
		 VALUES ('conn-site', 'https://conn.example.com', 'codex', 'active', ?, ?)`,
		now, now,
	)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	provider := "codex"
	_, err = db.Exec(
		`INSERT INTO accounts (site_id, username, access_token, status, oauth_provider, oauth_account_key, extra_config, created_at, updated_at)
		 VALUES (1, 'conn-user', 'conn-access', 'active', ?, 'conn-key', '{"oauth":{"provider":"codex","refreshToken":"rt"}}', ?, ?)`,
		provider, now, now,
	)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}

	result, err := ListOauthConnections(ListConnectionsInput{Limit: 50})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("Total = %d, want 1", result.Total)
	}
	if len(result.Items) != 1 {
		t.Fatalf("Items len = %d, want 1", len(result.Items))
	}
	item := result.Items[0]
	if item.Provider != "codex" {
		t.Errorf("Provider = %q", item.Provider)
	}
}

func TestListOauthConnections_LimitClampedToMax(t *testing.T) {
	_, cleanup := setupTestDB(t)
	defer cleanup()

	// Limit > 200 should be clamped; no error expected.
	result, err := ListOauthConnections(ListConnectionsInput{Limit: 999})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestListOauthConnections_DefaultLimitWhenZero(t *testing.T) {
	_, cleanup := setupTestDB(t)
	defer cleanup()

	// Limit = 0 should default to 50.
	result, err := ListOauthConnections(ListConnectionsInput{Limit: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestListOauthConnections_WithOffset(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Insert a site + two accounts so offset has something to skip.
	now := "2026-01-01T00:00:00Z"
	_, err := db.Exec(
		`INSERT INTO sites (name, url, platform, status, created_at, updated_at)
		 VALUES ('off-site', 'https://off.example.com', 'codex', 'active', ?, ?)`,
		now, now,
	)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	provider := "codex"
	for i := 0; i < 2; i++ {
		_, err = db.Exec(
			`INSERT INTO accounts (site_id, username, access_token, status, oauth_provider, oauth_account_key, extra_config, created_at, updated_at)
			 VALUES (1, ?, ?, 'active', ?, ?, '{}', ?, ?)`,
			"off-user-"+string(rune('a'+i)), "off-access-"+string(rune('a'+i)),
			provider, "off-key-"+string(rune('a'+i)), now, now,
		)
		if err != nil {
			t.Fatalf("insert account %d: %v", i, err)
		}
	}

	// Offset=1 should skip the first account (ordered by id DESC).
	result, err := ListOauthConnections(ListConnectionsInput{Limit: 50, Offset: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 2 {
		t.Errorf("Total = %d, want 2", result.Total)
	}
	if len(result.Items) != 1 {
		t.Fatalf("Items len = %d, want 1 (offset skipped one)", len(result.Items))
	}
}

// ---- ListOAuthRefreshCandidates ----

func TestListOAuthRefreshCandidates_NoDatabase(t *testing.T) {
	store.CloseDatabase()
	t.Cleanup(func() { _ = store.CloseDatabase() })

	_, err := ListOAuthRefreshCandidates()
	if err == nil {
		t.Fatal("expected error when database is not initialized")
	}
}

func TestListOAuthRefreshCandidates_ReturnsAccounts(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	now := "2026-01-01T00:00:00Z"
	_, err := db.Exec(
		`INSERT INTO sites (name, url, platform, status, created_at, updated_at)
		 VALUES ('rc-site', 'https://rc.example.com', 'codex', 'active', ?, ?)`,
		now, now,
	)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	provider := "codex"
	_, err = db.Exec(
		`INSERT INTO accounts (site_id, username, access_token, status, oauth_provider, oauth_account_key, extra_config, created_at, updated_at)
		 VALUES (1, 'rc-user', 'rc-access', 'active', ?, 'rc-key', '{"oauth":{"provider":"codex","refreshToken":"rt"}}', ?, ?)`,
		provider, now, now,
	)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}

	candidates, err := ListOAuthRefreshCandidates()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("candidates len = %d, want 1", len(candidates))
	}
	if candidates[0].Account.OAuthProvider == nil || *candidates[0].Account.OAuthProvider != "codex" {
		t.Errorf("Provider = %v, want codex", candidates[0].Account.OAuthProvider)
	}
}
