package oauth

import (
	"testing"
	"time"
)

func TestOAuthAccountSiteQueriesUseExplicitProjection(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	now := time.Now().UTC().Format(time.RFC3339)
	siteResult, err := db.Exec(`INSERT INTO sites (name, url, platform, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`, "OAuth Test", "https://oauth-test.invalid", "new-api", "active", now, now)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	siteID, err := siteResult.LastInsertId()
	if err != nil {
		t.Fatalf("site id: %v", err)
	}
	extraConfig := `{"oauth":{"provider":"codex","accountId":"account-1","accountKey":"account-1","email":"demo@example.com","refreshToken":"refresh-token","tokenExpiresAt":4102444800000}}`
	_, err = db.Exec(`INSERT INTO accounts
		(site_id, username, access_token, status, oauth_provider, oauth_account_key, extra_config, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		siteID, "demo", "access-token", "active", "codex", "account-1", extraConfig, now, now)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}

	rows, err := selectOAuthAccountSiteRows(db, "ORDER BY a.id")
	if err != nil {
		t.Fatalf("select OAuth account/site rows: %v", err)
	}
	if len(rows) != 1 || rows[0].Account.SiteID != siteID || rows[0].SiteStatus != "active" {
		t.Fatalf("unexpected joined rows: %+v", rows)
	}

	candidates, err := ListOAuthRefreshCandidates()
	if err != nil {
		t.Fatalf("list refresh candidates: %v", err)
	}
	if len(candidates) != 1 || candidates[0].Account.OAuthProvider == nil || *candidates[0].Account.OAuthProvider != "codex" {
		t.Fatalf("unexpected refresh candidates: %+v", candidates)
	}

	connections, err := ListOauthConnections(ListConnectionsInput{Limit: 10})
	if err != nil {
		t.Fatalf("list OAuth connections: %v", err)
	}
	if connections.Total != 1 || len(connections.Items) != 1 {
		t.Fatalf("unexpected connection result: %+v", connections)
	}
	item := connections.Items[0]
	if item.AccountID != candidates[0].Account.ID || item.Site == nil || item.Site.Name != "OAuth Test" {
		t.Fatalf("unexpected connection item: %+v", item)
	}
}
