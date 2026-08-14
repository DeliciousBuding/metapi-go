package oauth

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

// ---- BuildOauthProviderHeaders ----

func TestBuildOauthProviderHeaders_NoOAuthReturnsEmpty(t *testing.T) {
	account := &store.Account{}
	headers := BuildOauthProviderHeaders(account, nil, nil)
	if len(headers) != 0 {
		t.Errorf("account with no OAuth provider should yield empty headers, got %v", headers)
	}
}

func TestBuildOauthProviderHeaders_CodexFromColumn(t *testing.T) {
	provider := "codex"
	accountKey := "codex-acct-key"
	account := &store.Account{
		OAuthProvider:   &provider,
		OAuthAccountKey: &accountKey,
	}
	headers := BuildOauthProviderHeaders(account, nil, nil)
	if len(headers) == 0 {
		t.Fatal("expected non-empty headers for codex account")
	}
	if headers["Originator"] != "codex_cli_rs" {
		t.Errorf("Originator = %q, want codex_cli_rs", headers["Originator"])
	}
	if headers["Chatgpt-Account-Id"] != "codex-acct-key" {
		t.Errorf("Chatgpt-Account-Id = %q", headers["Chatgpt-Account-Id"])
	}
}

func TestBuildOauthProviderHeaders_CodexWithDownstreamOriginator(t *testing.T) {
	provider := "codex"
	accountKey := "acct-1"
	account := &store.Account{
		OAuthProvider:   &provider,
		OAuthAccountKey: &accountKey,
	}
	headers := BuildOauthProviderHeaders(account, nil, map[string]interface{}{
		"originator": "custom-cli",
	})
	if headers["Originator"] != "custom-cli" {
		t.Errorf("downstream originator should win, got %q", headers["Originator"])
	}
}

func TestBuildOauthProviderHeaders_ClaudeFromExtraConfig(t *testing.T) {
	extraConfig := `{"oauth":{"provider":"claude","accountId":"acct-uuid","accountKey":"acct-uuid"}}`
	account := &store.Account{ExtraConfig: &extraConfig}
	headers := BuildOauthProviderHeaders(account, &extraConfig, nil)
	if len(headers) == 0 {
		t.Fatal("expected non-empty headers for claude account")
	}
	if headers["anthropic-version"] != claudeDefaultAnthropicVersion {
		t.Errorf("anthropic-version = %q", headers["anthropic-version"])
	}
}

func TestBuildOauthProviderHeaders_AntigravityIdentityHeaders(t *testing.T) {
	provider := "antigravity"
	accountKey := "ag-acct"
	account := &store.Account{
		OAuthProvider:   &provider,
		OAuthAccountKey: &accountKey,
	}
	headers := BuildOauthProviderHeaders(account, nil, nil)
	if headers["User-Agent"] != antigravityUserAgent {
		t.Errorf("User-Agent = %q", headers["User-Agent"])
	}
	if headers["X-Goog-Api-Client"] != antigravityGoogleAPIClient {
		t.Errorf("X-Goog-Api-Client = %q", headers["X-Goog-Api-Client"])
	}
	if headers["Client-Metadata"] != antigravityClientMetadata {
		t.Errorf("Client-Metadata = %q", headers["Client-Metadata"])
	}
}

func TestBuildOauthProviderHeaders_UnknownProviderReturnsEmpty(t *testing.T) {
	provider := "nonexistent-provider"
	account := &store.Account{OAuthProvider: &provider}
	headers := BuildOauthProviderHeaders(account, nil, nil)
	if len(headers) != 0 {
		t.Errorf("unknown provider should yield empty headers, got %v", headers)
	}
}

// ---- BuildCodexOauthProviderHeaders ----

func TestBuildCodexOauthProviderHeaders_NoConfigReturnsDefaultOriginator(t *testing.T) {
	// BuildCodexOauthProviderHeaders forces Provider=codex via the patch,
	// so even nil extraConfig yields headers with the default Originator but
	// no Chatgpt-Account-Id (account fields are empty).
	headers := BuildCodexOauthProviderHeaders(nil, nil)
	if headers["Originator"] != "codex_cli_rs" {
		t.Errorf("Originator = %q, want codex_cli_rs (default)", headers["Originator"])
	}
	if _, hasAccount := headers["Chatgpt-Account-Id"]; hasAccount {
		t.Error("should not set Chatgpt-Account-Id when no account info present")
	}
}

func TestBuildCodexOauthProviderHeaders_FromExtraConfig(t *testing.T) {
	extraConfig := `{"oauth":{"provider":"codex","accountId":"acct-uuid","accountKey":"acct-uuid"}}`
	headers := BuildCodexOauthProviderHeaders(&extraConfig, nil)
	if headers["Originator"] != "codex_cli_rs" {
		t.Errorf("Originator = %q", headers["Originator"])
	}
	if headers["Chatgpt-Account-Id"] != "acct-uuid" {
		t.Errorf("Chatgpt-Account-Id = %q", headers["Chatgpt-Account-Id"])
	}
}

func TestBuildCodexOauthProviderHeaders_DownstreamOriginatorWins(t *testing.T) {
	extraConfig := `{"oauth":{"provider":"codex","accountId":"acct-1"}}`
	headers := BuildCodexOauthProviderHeaders(&extraConfig, map[string]interface{}{
		"originator": "from-downstream",
	})
	if headers["Originator"] != "from-downstream" {
		t.Errorf("downstream originator should win, got %q", headers["Originator"])
	}
}

// ---- grok providerData helpers ----

func TestProviderDataGet_NilMap(t *testing.T) {
	v, ok := providerDataGet(nil, "key")
	if ok || v != nil {
		t.Errorf("nil map should yield (nil, false), got (%v, %v)", v, ok)
	}
}

func TestProviderDataGet_Present(t *testing.T) {
	data := map[string]interface{}{"key": "value"}
	v, ok := providerDataGet(data, "key")
	if !ok || v != "value" {
		t.Errorf("present key = (%v, %v), want (value, true)", v, ok)
	}
}

func TestProviderDataGet_Absent(t *testing.T) {
	data := map[string]interface{}{"other": "value"}
	v, ok := providerDataGet(data, "key")
	if ok || v != nil {
		t.Errorf("absent key = (%v, %v), want (nil, false)", v, ok)
	}
}

func TestProviderDataSet_NilMapIsNoOp(t *testing.T) {
	providerDataSet(nil, "key", "value") // must not panic
}

func TestProviderDataSet_SetsValue(t *testing.T) {
	data := map[string]interface{}{}
	providerDataSet(data, "scope", "new-scope")
	if data["scope"] != "new-scope" {
		t.Errorf("scope = %v, want new-scope", data["scope"])
	}
}

// ---- buildUnsupportedQuotaSnapshot ----

func TestBuildUnsupportedQuotaSnapshot_Shape(t *testing.T) {
	snap := buildUnsupportedQuotaSnapshot()
	if snap == nil {
		t.Fatal("expected non-nil snapshot")
	}
	if snap.Status != "unsupported" {
		t.Errorf("Status = %q, want unsupported", snap.Status)
	}
	if snap.Source != "reverse_engineered" {
		t.Errorf("Source = %q", snap.Source)
	}
	if snap.LastSyncAt == "" {
		t.Error("LastSyncAt should be set to current time")
	}
	if snap.Windows == nil {
		t.Fatal("Windows should be non-nil")
	}
	if snap.Windows.FiveHour == nil || snap.Windows.FiveHour.Supported {
		t.Error("FiveHour should be non-nil and unsupported")
	}
	if snap.Windows.SevenDay == nil || snap.Windows.SevenDay.Supported {
		t.Error("SevenDay should be non-nil and unsupported")
	}
}
