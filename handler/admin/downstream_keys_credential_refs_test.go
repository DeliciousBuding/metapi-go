package admin

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Credential-ref contract tests for the #1026 credential dimension
// (excludedCredentialRefs / allowedCredentialRefs on downstream keys).
// Ref shape: {"kind":"account_token","siteId":>0,"accountId":>0,"tokenId":>0}
// or {"kind":"default_api_key","siteId":>0,"accountId":>0}.
// Full contract: docs/api.md → Downstream API Keys.

func TestDownstreamKeysCreateStoresAllowedCredentialRefs(t *testing.T) {
	db, r := setupDownstreamKeysTest(t)
	siteID, _, accountID, tokenID := seedDownstreamPolicyRefs(t, db)

	resp := doPostJSON(t, r, "/api/downstream-keys", map[string]any{
		"name": "allow-cred-client",
		"key":  "sk-allow-cred-client",
		"allowedCredentialRefs": []map[string]any{
			{"kind": "account_token", "siteId": siteID, "accountId": accountID, "tokenId": tokenID},
			{"kind": "default_api_key", "siteId": siteID, "accountId": accountID},
		},
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("create returned %d: %s", resp.Code, resp.Body.String())
	}

	var stored sql.NullString
	if err := db.QueryRow("SELECT allowed_credential_refs FROM downstream_api_keys WHERE key = 'sk-allow-cred-client'").Scan(&stored); err != nil {
		t.Fatalf("select allowed refs: %v", err)
	}
	if !stored.Valid {
		t.Fatal("allowed_credential_refs was not stored")
	}
	var refs []map[string]any
	if err := json.Unmarshal([]byte(stored.String), &refs); err != nil {
		t.Fatalf("stored allowed refs is not valid JSON: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("stored allowed refs len = %d, want 2: %s", len(refs), stored.String)
	}
	// Pin the persisted camelCase shape consumed by auth.parseExcludedCredentialRefs.
	tok := refs[0]
	if tok["kind"] != "account_token" || tok["tokenId"] == nil || tok["siteId"] == nil || tok["accountId"] == nil {
		t.Fatalf("account_token ref lost camelCase fields: %#v", tok)
	}
	def := refs[1]
	if def["kind"] != "default_api_key" || def["siteId"] == nil || def["accountId"] == nil {
		t.Fatalf("default_api_key ref lost camelCase fields: %#v", def)
	}
	if _, hasToken := def["tokenId"]; hasToken {
		t.Fatalf("default_api_key ref must not carry tokenId: %#v", def)
	}
}

func TestDownstreamKeysCreateEmptyAllowedCredentialRefsUnrestricted(t *testing.T) {
	db, r := setupDownstreamKeysTest(t)
	seedDownstreamPolicyRefs(t, db)

	resp := doPostJSON(t, r, "/api/downstream-keys", map[string]any{
		"name":                  "allow-cred-empty",
		"key":                   "sk-allow-cred-empty",
		"allowedCredentialRefs": []map[string]any{},
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("create returned %d: %s", resp.Code, resp.Body.String())
	}
	var stored sql.NullString
	if err := db.QueryRow("SELECT allowed_credential_refs FROM downstream_api_keys WHERE key = 'sk-allow-cred-empty'").Scan(&stored); err != nil {
		t.Fatalf("select allowed refs: %v", err)
	}
	// Empty list persists as NULL = unrestricted (no allow-list gate).
	if stored.Valid && strings.TrimSpace(stored.String) != "" && stored.String != "null" {
		t.Fatalf("empty allow-list should persist as NULL, got %q", stored.String)
	}
}

func TestDownstreamKeysCreateRejectsInvalidCredentialRefs(t *testing.T) {
	db, r := setupDownstreamKeysTest(t)
	siteID, _, accountID, tokenID := seedDownstreamPolicyRefs(t, db)

	// Account without an api_token for the default_api_key rejection case.
	now := time.Now().UTC().Format(time.RFC3339)
	noApiAccountID := insertDownstreamFixtureID(t, db,
		`INSERT INTO accounts (site_id, access_token, api_token, status, checkin_enabled, created_at, updated_at)
		 VALUES (?, 'session-no-api', NULL, 'active', TRUE, ?, ?)`,
		siteID, now, now,
	)

	cases := []struct {
		name    string
		refs    []map[string]any
		wantSub string
	}{
		{
			name:    "account_token unknown token",
			refs:    []map[string]any{{"kind": "account_token", "siteId": siteID, "accountId": accountID, "tokenId": 999999}},
			wantSub: "unknown token",
		},
		{
			name:    "account_token wrong account",
			refs:    []map[string]any{{"kind": "account_token", "siteId": siteID, "accountId": accountID + 999, "tokenId": tokenID}},
			wantSub: "does not match",
		},
		{
			name:    "account_token wrong site",
			refs:    []map[string]any{{"kind": "account_token", "siteId": siteID + 999, "accountId": accountID, "tokenId": tokenID}},
			wantSub: "does not match",
		},
		{
			name:    "default_api_key unknown account",
			refs:    []map[string]any{{"kind": "default_api_key", "siteId": siteID, "accountId": accountID + 999}},
			wantSub: "unknown account",
		},
		{
			name:    "default_api_key wrong site",
			refs:    []map[string]any{{"kind": "default_api_key", "siteId": siteID + 999, "accountId": accountID}},
			wantSub: "does not match the site",
		},
		{
			name:    "default_api_key account has no api token",
			refs:    []map[string]any{{"kind": "default_api_key", "siteId": siteID, "accountId": noApiAccountID}},
			wantSub: "no default API key",
		},
	}

	for i, tc := range cases {
		key := "sk-cred-reject-" + strings.ReplaceAll(tc.name, " ", "-")
		resp := doPostJSON(t, r, "/api/downstream-keys", map[string]any{
			"name":                   "cred-reject-" + tc.name,
			"key":                    key,
			"excludedCredentialRefs": tc.refs,
		})
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("[%d %s] expected 400, got %d: %s", i, tc.name, resp.Code, resp.Body.String())
		}
		if !strings.Contains(resp.Body.String(), tc.wantSub) {
			t.Fatalf("[%d %s] response missing %q: %s", i, tc.name, tc.wantSub, resp.Body.String())
		}
	}

	// Same validation applies to the allow-list dimension.
	resp := doPostJSON(t, r, "/api/downstream-keys", map[string]any{
		"name":                  "cred-reject-allow",
		"key":                   "sk-cred-reject-allow",
		"allowedCredentialRefs": []map[string]any{{"kind": "account_token", "siteId": siteID, "accountId": accountID, "tokenId": 999999}},
	})
	if resp.Code != http.StatusBadRequest || !strings.Contains(resp.Body.String(), "unknown token") {
		t.Fatalf("allow-list unknown token should 400: %d %s", resp.Code, resp.Body.String())
	}
}

func TestDownstreamKeysRejectsMalformedCredentialRefs(t *testing.T) {
	db, r := setupDownstreamKeysTest(t)
	siteID, _, accountID, tokenID := seedDownstreamPolicyRefs(t, db)

	// Malformed entries are rejected with 400 instead of being silently
	// dropped: dropping an allow-list entry would silently widen access.
	cases := []struct {
		name    string
		refs    []any
		wantSub string
	}{
		{
			name:    "non-object entry",
			refs:    []any{"account_token"},
			wantSub: "must be an object",
		},
		{
			name:    "unknown kind",
			refs:    []any{map[string]any{"kind": "bogus", "siteId": siteID, "accountId": accountID}},
			wantSub: "unknown kind",
		},
		{
			name:    "missing kind",
			refs:    []any{map[string]any{"siteId": siteID, "accountId": accountID}},
			wantSub: "unknown kind",
		},
		{
			name:    "account_token without tokenId",
			refs:    []any{map[string]any{"kind": "account_token", "siteId": siteID, "accountId": accountID}},
			wantSub: "requires a positive tokenId",
		},
		{
			name:    "zero siteId",
			refs:    []any{map[string]any{"kind": "account_token", "siteId": 0, "accountId": accountID, "tokenId": tokenID}},
			wantSub: "requires positive siteId and accountId",
		},
	}

	for i, tc := range cases {
		resp := doPostJSON(t, r, "/api/downstream-keys", map[string]any{
			"name":                   "cred-malformed-" + tc.name,
			"key":                    "sk-cred-malformed-" + strings.ReplaceAll(tc.name, " ", "-"),
			"excludedCredentialRefs": tc.refs,
		})
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("[%d %s] expected 400, got %d: %s", i, tc.name, resp.Code, resp.Body.String())
		}
		if !strings.Contains(resp.Body.String(), tc.wantSub) {
			t.Fatalf("[%d %s] response missing %q: %s", i, tc.name, tc.wantSub, resp.Body.String())
		}
	}
}

func TestDownstreamKeysUpdateAllowedCredentialRefsPreserveAndClear(t *testing.T) {
	db, r := setupDownstreamKeysTest(t)
	siteID, _, accountID, tokenID := seedDownstreamPolicyRefs(t, db)

	resp := doPostJSON(t, r, "/api/downstream-keys", map[string]any{
		"name": "cred-update-client",
		"key":  "sk-cred-update-client",
		"allowedCredentialRefs": []map[string]any{
			{"kind": "account_token", "siteId": siteID, "accountId": accountID, "tokenId": tokenID},
		},
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("create returned %d: %s", resp.Code, resp.Body.String())
	}
	var createBody map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &createBody); err != nil {
		t.Fatalf("unmarshal create: %v", err)
	}
	keyID := int64(createBody["item"].(map[string]any)["id"].(float64))
	idStr := itoa(keyID)

	readStored := func() sql.NullString {
		var stored sql.NullString
		if err := db.QueryRow("SELECT allowed_credential_refs FROM downstream_api_keys WHERE id = ?", keyID).Scan(&stored); err != nil {
			t.Fatalf("select allowed refs: %v", err)
		}
		return stored
	}

	// Omitted field → preserved.
	resp = doPutJSON(t, r, "/api/downstream-keys/"+idStr, map[string]any{"description": "touch"})
	if resp.Code != http.StatusOK {
		t.Fatalf("partial update returned %d: %s", resp.Code, resp.Body.String())
	}
	if stored := readStored(); !stored.Valid || !strings.Contains(stored.String, "account_token") {
		t.Fatalf("omitted allowedCredentialRefs must be preserved, got %#v", stored)
	}

	// Malformed field → 400 and the stored value is untouched.
	resp = doPutJSON(t, r, "/api/downstream-keys/"+idStr, map[string]any{
		"allowedCredentialRefs": []any{map[string]any{"kind": "bogus", "siteId": siteID, "accountId": accountID}},
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("malformed update should 400, got %d: %s", resp.Code, resp.Body.String())
	}
	if stored := readStored(); !stored.Valid || !strings.Contains(stored.String, "account_token") {
		t.Fatalf("rejected update must not clobber stored refs, got %#v", stored)
	}

	// Explicit empty list → cleared to NULL (unrestricted).
	resp = doPutJSON(t, r, "/api/downstream-keys/"+idStr, map[string]any{
		"allowedCredentialRefs": []map[string]any{},
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("clear update returned %d: %s", resp.Code, resp.Body.String())
	}
	if stored := readStored(); stored.Valid && strings.TrimSpace(stored.String) != "" && stored.String != "null" {
		t.Fatalf("explicit empty allow-list should clear to NULL, got %q", stored.String)
	}
}

func TestDownstreamKeysDanglingCredentialRefsPersist(t *testing.T) {
	db, r := setupDownstreamKeysTest(t)
	siteID, _, accountID, tokenID := seedDownstreamPolicyRefs(t, db)

	resp := doPostJSON(t, r, "/api/downstream-keys", map[string]any{
		"name": "cred-dangling-client",
		"key":  "sk-cred-dangling-client",
		"allowedCredentialRefs": []map[string]any{
			{"kind": "account_token", "siteId": siteID, "accountId": accountID, "tokenId": tokenID},
		},
		"excludedCredentialRefs": []map[string]any{
			{"kind": "account_token", "siteId": siteID, "accountId": accountID, "tokenId": tokenID},
		},
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("create returned %d: %s", resp.Code, resp.Body.String())
	}

	// Delete the referenced token and account directly (the account/token
	// delete endpoints perform no downstream-key reference checks).
	if _, err := db.Exec("DELETE FROM account_tokens WHERE id = ?", tokenID); err != nil {
		t.Fatalf("delete token: %v", err)
	}
	if _, err := db.Exec("DELETE FROM accounts WHERE id = ?", accountID); err != nil {
		t.Fatalf("delete account: %v", err)
	}

	// Documented behavior: refs are not cascade-cleaned. They stay stored and
	// simply never match a candidate (allow ref = never eligible, fail-closed;
	// exclude ref = harmless no-op).
	var allowed, excluded sql.NullString
	if err := db.QueryRow("SELECT allowed_credential_refs, excluded_credential_refs FROM downstream_api_keys WHERE key = 'sk-cred-dangling-client'").Scan(&allowed, &excluded); err != nil {
		t.Fatalf("select refs: %v", err)
	}
	if !allowed.Valid || !strings.Contains(allowed.String, "account_token") {
		t.Fatalf("dangling allowed refs should persist unchanged, got %#v", allowed)
	}
	if !excluded.Valid || !strings.Contains(excluded.String, "account_token") {
		t.Fatalf("dangling excluded refs should persist unchanged, got %#v", excluded)
	}
}
