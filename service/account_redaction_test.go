package service

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestRedactAccountSecrets_StripsNestedCredentials covers the H7 contract: the
// admin account policy must strip credential-shaped keys recursively, not only
// the three names it happened to know about, while leaving display metadata
// (provider, email, expiry) intact.
func TestRedactAccountSecrets_StripsNestedCredentials(t *testing.T) {
	extraConfig := `{
		"credentialMode": "session",
		"proxyUrl": "http://proxy.example:8080",
		"platformUserId": 4242,
		"oauth": {
			"provider": "codex",
			"email": "operator@example.com",
			"planType": "pro",
			"tokenExpiresAt": 1760000000000,
			"refreshToken": "synthetic-refresh-token",
			"idToken": "synthetic-id-token",
			"sessionToken": "synthetic-session-token",
			"accessToken": "synthetic-oauth-access-token",
			"access_token": "synthetic-oauth-access-token-snake"
		},
		"sub2apiAuth": {
			"refreshToken": "synthetic-sub2api-refresh-token",
			"tokenExpiresAt": 1760000000000
		},
		"autoRelogin": {
			"username": "operator@example.com",
			"passwordCipher": "synthetic-password-cipher"
		},
		"providerData": {
			"accountId": "acct-1",
			"sessionSecret": "synthetic-session-secret",
			"nested": {"client_cipher": "synthetic-client-cipher"}
		},
		"hasApiKey": true,
		"apiKeyMasked": "abcd****wxyz",
		"siteApiKey": "synthetic-site-api-key",
		"refresh_token": "synthetic-refresh-token-snake",
		"authPassword": "synthetic-password"
	}`
	account := map[string]any{
		"id":                12,
		"username":          "operator",
		"accessToken":       "synthetic-access-token-123456",
		"apiToken":          "synthetic-api-token-abcdef",
		"extraConfig":       extraConfig,
		"accessTokenMasked": "preserved****mask",
	}

	RedactAccountSecrets(account)

	if _, ok := account["accessToken"]; ok {
		t.Fatal("top-level accessToken survived redaction")
	}
	if _, ok := account["apiToken"]; ok {
		t.Fatal("top-level apiToken survived redaction")
	}
	if got := account["accessTokenMasked"]; got != "synt****3456" {
		t.Fatalf("accessTokenMasked = %v, want synt****3456", got)
	}
	if got := account["apiTokenMasked"]; got != "synt****cdef" {
		t.Fatalf("apiTokenMasked = %v, want synt****cdef", got)
	}

	encoded, _ := json.Marshal(account)
	body := string(encoded)
	for _, secret := range []string{
		"synthetic-refresh-token",
		"synthetic-id-token",
		"synthetic-session-token",
		"synthetic-oauth-access-token",
		"synthetic-sub2api-refresh-token",
		"synthetic-password-cipher",
		"synthetic-session-secret",
		"synthetic-client-cipher",
		"synthetic-site-api-key",
		"synthetic-refresh-token-snake",
		"synthetic-password",
	} {
		if strings.Contains(body, secret) {
			t.Errorf("response leaked %q: %s", secret, body)
		}
	}

	var nested map[string]any
	if err := json.Unmarshal([]byte(account["extraConfig"].(string)), &nested); err != nil {
		t.Fatalf("extraConfig is no longer valid JSON: %v", err)
	}
	oauth := nested["oauth"].(map[string]any)
	if oauth["provider"] != "codex" || oauth["email"] != "operator@example.com" || oauth["planType"] != "pro" {
		t.Fatalf("oauth display metadata was dropped: %#v", oauth)
	}
	if oauth["tokenExpiresAt"] != float64(1760000000000) {
		t.Fatalf("oauth tokenExpiresAt was dropped: %#v", oauth)
	}
	if nested["proxyUrl"] != "http://proxy.example:8080" || nested["platformUserId"] != float64(4242) {
		t.Fatalf("non-credential config was dropped: %#v", nested)
	}
	if nested["hasApiKey"] != true || nested["apiKeyMasked"] != "abcd****wxyz" {
		t.Fatalf("metadata flags/masks were dropped: %#v", nested)
	}
	auto := nested["autoRelogin"].(map[string]any)
	if auto["username"] != "operator@example.com" {
		t.Fatalf("autoRelogin username was dropped: %#v", auto)
	}
	sub2api := nested["sub2apiAuth"].(map[string]any)
	if sub2api["tokenExpiresAt"] != float64(1760000000000) {
		t.Fatalf("sub2api tokenExpiresAt was dropped: %#v", sub2api)
	}
}

func TestRedactAccountSecrets_HandlesJSONArrayAndByteConfig(t *testing.T) {
	account := map[string]any{
		"extraConfig": []byte(`[{"name":"x","refreshToken":"synthetic-array-token","keep":1}]`),
	}
	RedactAccountSecrets(account)

	body, ok := account["extraConfig"].(string)
	if !ok {
		t.Fatalf("extraConfig type = %T, want string after normalization", account["extraConfig"])
	}
	if strings.Contains(body, "synthetic-array-token") {
		t.Fatalf("array extraConfig leaked a credential: %s", body)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		t.Fatalf("extraConfig is no longer valid JSON: %v", err)
	}
	if len(rows) != 1 || rows[0]["name"] != "x" || rows[0]["keep"] != float64(1) {
		t.Fatalf("metadata was dropped: %#v", rows)
	}
}

func TestRedactAccountSecrets_HandlesPointerConfig(t *testing.T) {
	raw := `{"autoRelogin":{"username":"operator","passwordCipher":"synthetic-pointer-cipher"}}`
	account := map[string]any{"extraConfig": &raw}
	RedactAccountSecrets(account)

	ec, ok := account["extraConfig"].(string)
	if !ok {
		t.Fatalf("extraConfig type = %T, want string", account["extraConfig"])
	}
	if strings.Contains(ec, "synthetic-pointer-cipher") {
		t.Fatalf("pointer extraConfig leaked a credential: %s", ec)
	}
	var nested map[string]any
	if err := json.Unmarshal([]byte(ec), &nested); err != nil {
		t.Fatalf("extraConfig is no longer valid JSON: %v", err)
	}
	auto := nested["autoRelogin"].(map[string]any)
	if auto["username"] != "operator" {
		t.Fatalf("autoRelogin metadata was dropped: %#v", nested)
	}
}

func TestRedactAccountSecrets_LeavesMalformedExtraConfigAlone(t *testing.T) {
	const raw = "not-json password=visible-in-invalid-config"
	account := map[string]any{"extraConfig": raw}
	RedactAccountSecrets(account)
	// The admin surface only rewrites values it can parse; an unparseable
	// blob is not a shape the app writes, and pretending to redact it would
	// hide a real storage problem.
	if account["extraConfig"] != raw {
		t.Fatalf("extraConfig = %v, want unchanged malformed value", account["extraConfig"])
	}
}
