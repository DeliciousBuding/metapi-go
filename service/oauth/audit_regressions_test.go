package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/internal/sqlitetest"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestPersistQuotaSnapshotPreservesSiblingOAuthCredentials(t *testing.T) {
	db := sqlitetest.Open(t)
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(`INSERT INTO sites (name, url, platform, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"codex-site", "https://api.example.invalid", "codex", "active", now, now)
	if err != nil {
		t.Fatal(err)
	}
	siteID, _ := res.LastInsertId()

	initialExtra := `{"credentialMode":"session","proxyUrl":"http://proxy.invalid:8080","oauth":{"provider":"codex","refreshToken":"rt-keep-me","sessionToken":"sess-keep-me","idToken":"id-keep-me","tokenExpiresAt":1999999999000,"email":"user@example.com"}}`
	res, err = db.Exec(`INSERT INTO accounts (site_id, username, access_token, status, oauth_provider, extra_config, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		siteID, "user@example.com", "at-initial", "active", "codex", initialExtra, now, now)
	if err != nil {
		t.Fatal(err)
	}
	accountID, _ := res.LastInsertId()

	limit := 100.0
	used := 25.0
	persistQuotaSnapshot(db, accountID, &OauthQuotaSnapshot{
		Status:     "supported",
		Source:     "codex",
		LastSyncAt: now,
		Windows: &OauthQuotaWindows{
			FiveHour: &OauthQuotaWindowSnapshot{Supported: true, Limit: &limit, Used: &used},
		},
	})

	var reloaded store.Account
	if err := db.Get(&reloaded, `SELECT * FROM accounts WHERE id = ?`, accountID); err != nil {
		t.Fatal(err)
	}
	info := GetOauthInfoFromAccount(&reloaded)
	if info == nil {
		t.Fatal("expected OauthInfo after quota persist")
	}
	if info.RefreshToken != "rt-keep-me" || info.SessionToken != "sess-keep-me" || info.IDToken != "id-keep-me" || info.TokenExpiresAt != 1999999999000 {
		t.Fatalf("persistQuotaSnapshot clobbered sibling oauth fields: %+v", info)
	}
	if info.Quota == nil || info.Quota.Source != "codex" {
		t.Fatalf("expected persisted quota snapshot, got %+v", info.Quota)
	}
	if got := GetProxyURLFromExtraConfig(reloaded.ExtraConfig); got != "http://proxy.invalid:8080" {
		t.Fatalf("proxyUrl lost after quota persist: %q", got)
	}
}

func TestRevertPersistedOauthAccountDoesNotClobberNewerCommit(t *testing.T) {
	db := sqlitetest.Open(t)
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(`INSERT INTO sites (name, url, platform, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"codex-site", "https://api.example.invalid", "codex", "active", now, now)
	if err != nil {
		t.Fatal(err)
	}
	siteID, _ := res.LastInsertId()

	origExtra := `{"oauth":{"refreshToken":"rt-orig"}}`
	res, err = db.Exec(`INSERT INTO accounts (site_id, username, access_token, status, oauth_provider, extra_config, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		siteID, "user@example.com", "at-orig", "active", "codex", origExtra, now, now)
	if err != nil {
		t.Fatal(err)
	}
	accountID, _ := res.LastInsertId()

	var origAccount store.Account
	if err := db.Get(&origAccount, `SELECT * FROM accounts WHERE id = ?`, accountID); err != nil {
		t.Fatal(err)
	}
	snapshot := captureAccountSnapshot(db, &origAccount)

	// Rebind A writes token A, then Rebind B commits token B before A rolls back.
	extraA := `{"oauth":{"refreshToken":"rt-A"}}`
	extraB := `{"oauth":{"refreshToken":"rt-B"}}`
	if _, err := db.Exec(`UPDATE accounts SET access_token = ?, extra_config = ? WHERE id = ?`, "at-B", extraB, accountID); err != nil {
		t.Fatal(err)
	}

	if err := revertPersistedOauthAccount(db, accountID, false, snapshot, "at-A", &extraA); err != nil {
		t.Fatalf("revertPersistedOauthAccount: %v", err)
	}

	var current store.Account
	if err := db.Get(&current, `SELECT * FROM accounts WHERE id = ?`, accountID); err != nil {
		t.Fatal(err)
	}
	if current.AccessToken != "at-B" || current.ExtraConfig == nil || *current.ExtraConfig != extraB {
		t.Fatalf("stale rollback clobbered newer rebind B: token=%q extra=%v", current.AccessToken, current.ExtraConfig)
	}

	// When the row still holds A's version, rollback must restore the snapshot.
	if _, err := db.Exec(`UPDATE accounts SET access_token = ?, extra_config = ? WHERE id = ?`, "at-A", extraA, accountID); err != nil {
		t.Fatal(err)
	}
	if err := revertPersistedOauthAccount(db, accountID, false, snapshot, "at-A", &extraA); err != nil {
		t.Fatalf("revert matching attempt: %v", err)
	}
	if err := db.Get(&current, `SELECT * FROM accounts WHERE id = ?`, accountID); err != nil {
		t.Fatal(err)
	}
	if current.AccessToken != "at-orig" || current.ExtraConfig == nil || *current.ExtraConfig != origExtra {
		t.Fatalf("expected rollback to restore original snapshot, got token=%q extra=%v", current.AccessToken, current.ExtraConfig)
	}
}

func TestDirectCredentialMaterialRejectsCustomGrokClientID(t *testing.T) {
	endpoints := store.DirectEndpoints{Chat: &store.DirectEndpoint{URL: "https://api.x.ai/v1/chat/completions", Auth: store.DirectAuthBearer}}
	bad := &DirectOAuthReplacement{
		AccessToken: "at-xai",
		DirectOAuthState: store.DirectOAuthState{
			RefreshToken: "rt-xai",
			ClientID:     "custom-bogus-client-id",
			ExpiresAt:    time.Now().Add(time.Hour).UnixMilli(),
		},
	}
	if _, _, _, _, err := directCredentialMaterial("xai_subscription", endpoints, store.DirectCredentialOAuth, nil, bad, nil); !errors.Is(err, ErrInvalidDirectCredential) {
		t.Fatalf("expected ErrInvalidDirectCredential for mismatched xai_subscription clientId, got %v", err)
	}

	good := &DirectOAuthReplacement{
		AccessToken: "at-xai",
		DirectOAuthState: store.DirectOAuthState{
			RefreshToken: "rt-xai",
			ExpiresAt:    time.Now().Add(time.Hour).UnixMilli(),
		},
	}
	_, kind, state, _, err := directCredentialMaterial("xai_subscription", endpoints, store.DirectCredentialOAuth, nil, good, nil)
	if err != nil {
		t.Fatalf("expected empty xai_subscription clientId to normalize to grokClientID, got %v", err)
	}
	if kind != store.DirectCredentialOAuth || state.ClientID != grokClientID {
		t.Fatalf("got kind=%q clientID=%q, want oauth + %q", kind, state.ClientID, grokClientID)
	}
}

func TestRefreshFamilyWinnerAllowsActiveFamilyWhenPriorWinnerDeleted(t *testing.T) {
	db := sqlitetest.Open(t)
	scope, err := refreshFamilyScope(string(ProviderCodex), "")
	if err != nil {
		t.Fatal(err)
	}
	// Active family pointing at a deleted direct credential ID (99999).
	activeFamily := &store.CredentialRefreshFamily{
		Provider:           scope,
		CurrentFingerprint: store.OAuthRefreshFingerprint(scope, "rt-current"),
		Generation:         2,
		ResultScope:        "direct",
		ResultID:           99999,
		Retired:            false,
	}
	winner, err := refreshFamilyWinner(context.Background(), db.DB, activeFamily, string(ProviderCodex), "at-old", true)
	if err != nil || winner != nil {
		t.Fatalf("active family with deleted winner should fall through to refresh, got winner=%v err=%v", winner, err)
	}

	// Retired alias pointing at a deleted winner must still fail closed.
	retiredFamily := *activeFamily
	retiredFamily.Retired = true
	if _, err := refreshFamilyWinner(context.Background(), db.DB, &retiredFamily, string(ProviderCodex), "at-old", true); !errors.Is(err, ErrOAuthRefreshUncertain) {
		t.Fatalf("retired alias with deleted winner must fail closed with ErrOAuthRefreshUncertain, got %v", err)
	}
}

func TestSummarizeOAuthHTTPErrorRedactsRawBody(t *testing.T) {
	rawBody, _ := json.Marshal(map[string]any{
		"error":             "invalid_grant",
		"error_description": "secret-token-value-must-not-leak",
	})
	err := summarizeOAuthHTTPError("codex token exchange", http.StatusBadRequest, rawBody)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid_grant") || strings.Contains(err.Error(), "secret-token-value-must-not-leak") {
		t.Fatalf("unexpected error summary: %q", err.Error())
	}
}
