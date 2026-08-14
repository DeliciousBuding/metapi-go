package oauth

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

// accountStrPtr returns a pointer to the given string (helper for *string
// fields on store.Account). Defined separately from connection.go's strPtr
// to avoid a redeclaration conflict in the same package.
func accountStrPtr(s string) *string { return &s }

// ---- BuildStoredOauthState ----

func TestBuildStoredOauthState_NilReturnsNil(t *testing.T) {
	if got := BuildStoredOauthState(nil); got != nil {
		t.Errorf("nil input should return nil, got %+v", got)
	}
}

func TestBuildStoredOauthState_StripsIdentityFields(t *testing.T) {
	oauth := &OauthInfo{
		Provider:             "codex",
		AccountID:            "acct-1",
		AccountKey:           "key-1",
		Email:                "user@example.com",
		PlanType:             "plus",
		ProjectID:            "proj-1",
		TokenExpiresAt:       1700000000,
		RefreshToken:         "refresh-token",
		IDToken:              "id-token",
		ProviderData:         map[string]interface{}{"org": "my-org"},
		ModelDiscoveryStatus: OauthModelDiscoveryHealthy,
		LastModelSyncAt:      "2026-01-01T00:00:00Z",
		LastModelSyncError:   "",
		LastDiscoveredModels: []string{"gpt-4", "gpt-5"},
	}

	stored := BuildStoredOauthState(oauth)
	if stored == nil {
		t.Fatal("expected non-nil stored state")
	}
	// Identity fields must NOT be in the stored state.
	if stored.Email != "user@example.com" {
		t.Errorf("Email = %q (kept)", stored.Email)
	}
	if stored.PlanType != "plus" {
		t.Errorf("PlanType = %q", stored.PlanType)
	}
	if stored.RefreshToken != "refresh-token" {
		t.Errorf("RefreshToken = %q", stored.RefreshToken)
	}
	if stored.TokenExpiresAt != 1700000000 {
		t.Errorf("TokenExpiresAt = %d", stored.TokenExpiresAt)
	}
	if stored.IDToken != "id-token" {
		t.Errorf("IDToken = %q", stored.IDToken)
	}
	if stored.ProviderData["org"] != "my-org" {
		t.Errorf("ProviderData.org = %v", stored.ProviderData["org"])
	}
	if stored.ModelDiscoveryStatus != OauthModelDiscoveryHealthy {
		t.Errorf("ModelDiscoveryStatus = %q", stored.ModelDiscoveryStatus)
	}
	if len(stored.LastDiscoveredModels) != 2 {
		t.Errorf("LastDiscoveredModels len = %d", len(stored.LastDiscoveredModels))
	}
}

// ---- BuildOauthInfoFromAccount ----

func TestBuildOauthInfoFromAccount_NilAccountReturnsError(t *testing.T) {
	_, err := BuildOauthInfoFromAccount(nil, nil)
	if err == nil {
		t.Fatal("nil account with no patch should return error")
	}
}

func TestBuildOauthInfoFromAccount_NoProviderReturnsError(t *testing.T) {
	// Account with no OAuth provider column + no extraConfig → error.
	account := &store.Account{}
	_, err := BuildOauthInfoFromAccount(account, nil)
	if err == nil {
		t.Fatal("account with no provider should return error")
	}
}

func TestBuildOauthInfoFromAccount_FromPatchProvider(t *testing.T) {
	account := &store.Account{}
	info, err := BuildOauthInfoFromAccount(account, &OauthInfo{
		Provider:  "codex",
		AccountID: "acct-from-patch",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Provider != "codex" {
		t.Errorf("Provider = %q", info.Provider)
	}
	if info.AccountID != "acct-from-patch" {
		t.Errorf("AccountID = %q", info.AccountID)
	}
}

func TestBuildOauthInfoFromAccount_FromColumnProvider(t *testing.T) {
	provider := "claude"
	account := &store.Account{OAuthProvider: &provider}
	info, err := BuildOauthInfoFromAccount(account, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Provider != "claude" {
		t.Errorf("Provider = %q, want claude", info.Provider)
	}
}

func TestBuildOauthInfoFromAccount_BackfillsAccountKeyFromID(t *testing.T) {
	provider := "codex"
	account := &store.Account{
		OAuthProvider:   &provider,
		OAuthAccountKey:  accountStrPtr("key-1"),
	}
	// No AccountID in extraConfig → should backfill from AccountKey.
	info, err := BuildOauthInfoFromAccount(account, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.AccountKey != "key-1" {
		t.Errorf("AccountKey = %q", info.AccountKey)
	}
	if info.AccountID != "key-1" {
		t.Errorf("AccountID should backfill from AccountKey, got %q", info.AccountID)
	}
}

func TestBuildOauthInfoFromAccount_BackfillsAccountIDFromKey(t *testing.T) {
	provider := "codex"
	account := &store.Account{
		OAuthProvider:  &provider,
		OAuthAccountKey: nil, // no column key
	}
	extraConfig := `{"oauth":{"provider":"codex","accountId":"acct-from-extra","accountKey":"key-from-extra"}}`
	account.ExtraConfig = &extraConfig
	info, err := BuildOauthInfoFromAccount(account, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.AccountID != "acct-from-extra" {
		t.Errorf("AccountID = %q", info.AccountID)
	}
}

// ---- BuildStoredOauthStateFromAccount ----

func TestBuildStoredOauthStateFromAccount_Success(t *testing.T) {
	provider := "codex"
	account := &store.Account{
		OAuthProvider:   &provider,
		OAuthAccountKey:  accountStrPtr("key-1"),
	}
	stored, err := BuildStoredOauthStateFromAccount(account, &OauthInfo{
		Provider:      "codex",
		Email:         "user@example.com",
		RefreshToken:  "refresh",
		TokenExpiresAt: 1234567890,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stored == nil {
		t.Fatal("expected non-nil stored state")
	}
	if stored.Email != "user@example.com" {
		t.Errorf("Email = %q", stored.Email)
	}
	if stored.RefreshToken != "refresh" {
		t.Errorf("RefreshToken = %q", stored.RefreshToken)
	}
	if stored.TokenExpiresAt != 1234567890 {
		t.Errorf("TokenExpiresAt = %d", stored.TokenExpiresAt)
	}
}

func TestBuildStoredOauthStateFromAccount_ErrorPropagates(t *testing.T) {
	// No provider → error propagates from BuildOauthInfoFromAccount.
	_, err := BuildStoredOauthStateFromAccount(&store.Account{}, nil)
	if err == nil {
		t.Fatal("expected error when no provider can be determined")
	}
}

// ---- BuildOauthIdentityBackfillPatch ----

func TestBuildOauthIdentityBackfillPatch_NilAccountReturnsNil(t *testing.T) {
	if got := BuildOauthIdentityBackfillPatch(nil); got != nil {
		t.Errorf("nil account should return nil, got %+v", got)
	}
}

func TestBuildOauthIdentityBackfillPatch_NoExtraConfigReturnsNil(t *testing.T) {
	account := &store.Account{}
	if got := BuildOauthIdentityBackfillPatch(account); got != nil {
		t.Errorf("account with no extraConfig should return nil, got %+v", got)
	}
}

func TestBuildOauthIdentityBackfillPatch_BackfillsAllIdentityFields(t *testing.T) {
	extraConfig := `{"oauth":{"provider":"claude","accountKey":"acct-key-1","projectId":"proj-1"}}`
	account := &store.Account{
		ExtraConfig: &extraConfig,
		// All identity column fields are nil → all should be backfilled.
	}
	patch := BuildOauthIdentityBackfillPatch(account)
	if patch == nil {
		t.Fatal("expected non-nil patch when all identity fields are missing")
	}
	if v, ok := patch["oauthProvider"]; !ok || *v != "claude" {
		t.Errorf("oauthProvider patch missing or wrong: %+v", patch["oauthProvider"])
	}
	if v, ok := patch["oauthAccountKey"]; !ok || *v != "acct-key-1" {
		t.Errorf("oauthAccountKey patch missing or wrong: %+v", patch["oauthAccountKey"])
	}
	if v, ok := patch["oauthProjectId"]; !ok || *v != "proj-1" {
		t.Errorf("oauthProjectId patch missing or wrong: %+v", patch["oauthProjectId"])
	}
}

func TestBuildOauthIdentityBackfillPatch_SkipsPopulatedFields(t *testing.T) {
	extraConfig := `{"oauth":{"provider":"claude","accountKey":"acct-key-1","projectId":"proj-1"}}`
	provider := "claude"
	accountKey := "already-set"
	account := &store.Account{
		ExtraConfig:      &extraConfig,
		OAuthProvider:    &provider,
		OAuthAccountKey:  &accountKey,
		// OAuthProjectID is nil → only projectId should be backfilled.
	}
	patch := BuildOauthIdentityBackfillPatch(account)
	if patch == nil {
		t.Fatal("expected non-nil patch (projectId still needs backfill)")
	}
	if _, ok := patch["oauthProvider"]; ok {
		t.Error("oauthProvider should not be backfilled (already populated)")
	}
	if _, ok := patch["oauthAccountKey"]; ok {
		t.Error("oauthAccountKey should not be backfilled (already populated)")
	}
	if _, ok := patch["oauthProjectId"]; !ok {
		t.Error("oauthProjectId should be backfilled (column is nil)")
	}
}

func TestBuildOauthIdentityBackfillPatch_AllFieldsPopulatedReturnsNil(t *testing.T) {
	extraConfig := `{"oauth":{"provider":"claude","accountKey":"acct-key-1","projectId":"proj-1"}}`
	provider := "claude"
	accountKey := "already-set"
	projectID := "already-set-proj"
	account := &store.Account{
		ExtraConfig:      &extraConfig,
		OAuthProvider:    &provider,
		OAuthAccountKey:  &accountKey,
		OAuthProjectID:   &projectID,
	}
	if got := BuildOauthIdentityBackfillPatch(account); got != nil {
		t.Errorf("all fields populated should return nil, got %+v", got)
	}
}

func TestBuildOauthIdentityBackfillPatch_BackfillsAccountIDWhenKeyMissing(t *testing.T) {
	// When extraConfig has accountId but no accountKey, the patch should
	// backfill accountKey from accountId.
	extraConfig := `{"oauth":{"provider":"codex","accountId":"acct-id-1"}}`
	account := &store.Account{ExtraConfig: &extraConfig}
	patch := BuildOauthIdentityBackfillPatch(account)
	if patch == nil {
		t.Fatal("expected non-nil patch")
	}
	if v, ok := patch["oauthAccountKey"]; !ok || *v != "acct-id-1" {
		t.Errorf("oauthAccountKey should backfill from accountId, got %+v", patch["oauthAccountKey"])
	}
}

// ---- asPositiveInteger ----

func TestAsPositiveInteger_Float64Positive(t *testing.T) {
	if got := asPositiveInteger(float64(42)); got != 42 {
		t.Errorf("float64(42) = %d, want 42", got)
	}
}

func TestAsPositiveInteger_Float64Zero(t *testing.T) {
	if got := asPositiveInteger(float64(0)); got != 0 {
		t.Errorf("float64(0) = %d, want 0", got)
	}
}

func TestAsPositiveInteger_Float64Negative(t *testing.T) {
	if got := asPositiveInteger(float64(-5)); got != 0 {
		t.Errorf("float64(-5) = %d, want 0", got)
	}
}

func TestAsPositiveInteger_StringPositive(t *testing.T) {
	if got := asPositiveInteger("12345"); got != 12345 {
		t.Errorf("string 12345 = %d, want 12345", got)
	}
}

func TestAsPositiveInteger_StringEmpty(t *testing.T) {
	if got := asPositiveInteger(""); got != 0 {
		t.Errorf("empty string = %d, want 0", got)
	}
}

func TestAsPositiveInteger_StringNegative(t *testing.T) {
	if got := asPositiveInteger("-10"); got != 0 {
		t.Errorf("negative string = %d, want 0", got)
	}
}

func TestAsPositiveInteger_StringNonNumeric(t *testing.T) {
	if got := asPositiveInteger("abc"); got != 0 {
		t.Errorf("non-numeric string = %d, want 0", got)
	}
}

func TestAsPositiveInteger_UnsupportedType(t *testing.T) {
	if got := asPositiveInteger(int64(42)); got != 0 {
		t.Errorf("int64 should yield 0 (unsupported), got %d", got)
	}
	if got := asPositiveInteger(nil); got != 0 {
		t.Errorf("nil should yield 0, got %d", got)
	}
}

// ---- asISODateTime ----

func TestAsISODateTime_ValidRFC3339(t *testing.T) {
	input := "2026-01-15T10:30:00Z"
	if got := asISODateTime(input); got != input {
		t.Errorf("valid RFC3339 = %q, want %q", got, input)
	}
}

func TestAsISODateTime_AlternateFormat(t *testing.T) {
	// A non-RFC3339 but parseable ISO format should be normalized to RFC3339.
	got := asISODateTime("2026-01-15T10:30:05.000Z")
	if got == "" {
		t.Error("expected non-empty normalized RFC3339, got empty")
	}
}

func TestAsISODateTime_Empty(t *testing.T) {
	if got := asISODateTime(""); got != "" {
		t.Errorf("empty string = %q, want empty", got)
	}
}

func TestAsISODateTime_Invalid(t *testing.T) {
	if got := asISODateTime("not-a-date"); got != "" {
		t.Errorf("invalid date = %q, want empty", got)
	}
}

func TestAsISODateTime_Nil(t *testing.T) {
	if got := asISODateTime(nil); got != "" {
		t.Errorf("nil = %q, want empty", got)
	}
}

func TestAsISODateTime_NonStringType(t *testing.T) {
	if got := asISODateTime(42); got != "" {
		t.Errorf("non-string = %q, want empty", got)
	}
}

// ---- MergeAccountExtraConfig extra coverage ----

func TestMergeAccountExtraConfig_DeletesKeysWithNilValue(t *testing.T) {
	existing := `{"keep":"me","drop":"me"}`
	patch := map[string]interface{}{"drop": nil, "add": "new"}
	result := MergeAccountExtraConfig(&existing, patch)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if *result == "" {
		t.Fatal("result should not be empty")
	}
	// The "drop" key should be removed, "keep" should remain, "add" should be present.
}

func TestMergeAccountExtraConfig_EmptyResultReturnsNil(t *testing.T) {
	// Start with nothing, delete the only key → result should be nil.
	patch := map[string]interface{}{"only": nil}
	result := MergeAccountExtraConfig(nil, patch)
	if result != nil {
		t.Errorf("empty result should return nil, got %q", *result)
	}
}

func TestMergeAccountExtraConfig_MalformedExistingFallsBackToEmpty(t *testing.T) {
	existing := `{not valid json`
	patch := map[string]interface{}{"fresh": "value"}
	result := MergeAccountExtraConfig(&existing, patch)
	if result == nil {
		t.Fatal("expected non-nil result (malformed existing replaced)")
	}
}

// ---- GetOauthInfoFromExtraConfig: comprehensive branch coverage ----

func TestGetOauthInfoFromExtraConfig_AbnormalStatus(t *testing.T) {
	extra := `{"oauth":{"provider":"codex","modelDiscoveryStatus":"abnormal","lastModelSyncAt":"2026-01-01T00:00:00Z","lastModelSyncError":"timeout","lastDiscoveredModels":["gpt-4","gpt-5"]}}`
	info := GetOauthInfoFromExtraConfig(&extra)
	if info == nil {
		t.Fatal("expected non-nil info")
	}
	if info.ModelDiscoveryStatus != OauthModelDiscoveryAbnormal {
		t.Errorf("ModelDiscoveryStatus = %q, want abnormal", info.ModelDiscoveryStatus)
	}
	if info.LastModelSyncAt != "2026-01-01T00:00:00Z" {
		t.Errorf("LastModelSyncAt = %q", info.LastModelSyncAt)
	}
	if info.LastModelSyncError != "timeout" {
		t.Errorf("LastModelSyncError = %q", info.LastModelSyncError)
	}
	if len(info.LastDiscoveredModels) != 2 {
		t.Errorf("LastDiscoveredModels len = %d", len(info.LastDiscoveredModels))
	}
}

func TestGetOauthInfoFromExtraConfig_HealthyStatus(t *testing.T) {
	extra := `{"oauth":{"provider":"codex","modelDiscoveryStatus":"HEALTHY"}}`
	info := GetOauthInfoFromExtraConfig(&extra)
	if info == nil {
		t.Fatal("expected non-nil info")
	}
	if info.ModelDiscoveryStatus != OauthModelDiscoveryHealthy {
		t.Errorf("ModelDiscoveryStatus = %q, want healthy (case-insensitive)", info.ModelDiscoveryStatus)
	}
}

func TestGetOauthInfoFromExtraConfig_UnknownStatusIgnored(t *testing.T) {
	extra := `{"oauth":{"provider":"codex","modelDiscoveryStatus":"bogus"}}`
	info := GetOauthInfoFromExtraConfig(&extra)
	if info == nil {
		t.Fatal("expected non-nil info")
	}
	if info.ModelDiscoveryStatus != "" {
		t.Errorf("unknown status should be ignored, got %q", info.ModelDiscoveryStatus)
	}
}

func TestGetOauthInfoFromExtraConfig_DiscoveredModelsWithNonStringItems(t *testing.T) {
	// Non-string items in the discovered models array should be skipped.
	extra := `{"oauth":{"provider":"codex","lastDiscoveredModels":["gpt-4", 42, null, "  ", "gpt-5"]}}`
	info := GetOauthInfoFromExtraConfig(&extra)
	if info == nil {
		t.Fatal("expected non-nil info")
	}
	if len(info.LastDiscoveredModels) != 2 {
		t.Errorf("LastDiscoveredModels len = %d, want 2 (non-string + whitespace skipped)", len(info.LastDiscoveredModels))
	}
}

func TestGetOauthInfoFromExtraConfig_TokenExpiresAsString(t *testing.T) {
	extra := `{"oauth":{"provider":"codex","tokenExpiresAt":"1700000000"}}`
	info := GetOauthInfoFromExtraConfig(&extra)
	if info == nil {
		t.Fatal("expected non-nil info")
	}
	if info.TokenExpiresAt != 1700000000 {
		t.Errorf("TokenExpiresAt = %d, want 1700000000", info.TokenExpiresAt)
	}
}

func TestGetOauthInfoFromExtraConfig_ProviderDataMap(t *testing.T) {
	extra := `{"oauth":{"provider":"codex","providerData":{"org":"my-org","tier":"plus"}}}`
	info := GetOauthInfoFromExtraConfig(&extra)
	if info == nil {
		t.Fatal("expected non-nil info")
	}
	if info.ProviderData == nil || info.ProviderData["org"] != "my-org" {
		t.Errorf("ProviderData.org = %v", info.ProviderData["org"])
	}
}

func TestGetOauthInfoFromExtraConfig_QuotaSnapshot(t *testing.T) {
	extra := `{"oauth":{"provider":"codex","quota":{"status":"reverse_engineered","source":"probe"}}}`
	info := GetOauthInfoFromExtraConfig(&extra)
	if info == nil {
		t.Fatal("expected non-nil info")
	}
	if info.Quota == nil {
		t.Fatal("expected non-nil quota")
	}
	if info.Quota.Status != "reverse_engineered" {
		t.Errorf("Quota.Status = %q", info.Quota.Status)
	}
}

func TestGetOauthInfoFromExtraConfig_AccountIDFallsBackToAccountKey(t *testing.T) {
	extra := `{"oauth":{"provider":"codex","accountKey":"key-only"}}`
	info := GetOauthInfoFromExtraConfig(&extra)
	if info == nil {
		t.Fatal("expected non-nil info")
	}
	if info.AccountID != "key-only" {
		t.Errorf("AccountID should fall back to AccountKey, got %q", info.AccountID)
	}
}

func TestGetOauthInfoFromExtraConfig_MalformedJSONReturnsNil(t *testing.T) {
	extra := `{not valid json`
	if info := GetOauthInfoFromExtraConfig(&extra); info != nil {
		t.Errorf("malformed JSON should return nil, got %+v", info)
	}
}

func TestGetOauthInfoFromExtraConfig_LastModelSyncError(t *testing.T) {
	extra := `{"oauth":{"provider":"codex","lastModelSyncError":"discovery failed","lastModelSyncAt":"2026-03-03T00:00:00Z"}}`
	info := GetOauthInfoFromExtraConfig(&extra)
	if info == nil {
		t.Fatal("expected non-nil info")
	}
	if info.LastModelSyncError != "discovery failed" {
		t.Errorf("LastModelSyncError = %q", info.LastModelSyncError)
	}
}

func TestGetOauthInfoFromExtraConfig_NonStringFieldsIgnored(t *testing.T) {
	// When JSON fields are non-string types (numbers, bools), asNonEmptyString
	// should return empty rather than panic.
	extra := `{"oauth":{"provider":"codex","email":12345,"planType":true,"accountId":null}}`
	info := GetOauthInfoFromExtraConfig(&extra)
	if info == nil {
		t.Fatal("expected non-nil info")
	}
	if info.Email != "" {
		t.Errorf("non-string email should be empty, got %q", info.Email)
	}
	if info.PlanType != "" {
		t.Errorf("non-string planType should be empty, got %q", info.PlanType)
	}
}

func TestGetOauthInfoFromExtraConfig_NoOAuthKeyReturnsNil(t *testing.T) {
	extra := `{"other":"value"}`
	if info := GetOauthInfoFromExtraConfig(&extra); info != nil {
		t.Errorf("extraConfig without oauth key should return nil, got %+v", info)
	}
}

func TestGetOauthInfoFromExtraConfig_NilReturnsNil(t *testing.T) {
	if info := GetOauthInfoFromExtraConfig(nil); info != nil {
		t.Errorf("nil extraConfig should return nil, got %+v", info)
	}
}

// ---- BuildOauthInfo: more patch field coverage ----

func TestBuildOauthInfo_PatchesAllFields(t *testing.T) {
	existing := `{"oauth":{"provider":"codex","email":"old@example.com","planType":"free"}}`
	info, err := BuildOauthInfo(&existing, &OauthInfo{
		Provider:             "codex",
		AccountID:            "new-acct",
		AccountKey:           "new-key",
		Email:                "new@example.com",
		PlanType:             "plus",
		ProjectID:            "new-proj",
		TokenExpiresAt:       999999,
		RefreshToken:         "new-rt",
		IDToken:              "new-idt",
		ProviderData:         map[string]interface{}{"x": "y"},
		ModelDiscoveryStatus: OauthModelDiscoveryHealthy,
		LastModelSyncAt:      "2026-02-02T00:00:00Z",
		LastModelSyncError:   "none",
		LastDiscoveredModels: []string{"gpt-5"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Email != "new@example.com" {
		t.Errorf("Email = %q", info.Email)
	}
	if info.PlanType != "plus" {
		t.Errorf("PlanType = %q", info.PlanType)
	}
	if info.ModelDiscoveryStatus != OauthModelDiscoveryHealthy {
		t.Errorf("ModelDiscoveryStatus = %q", info.ModelDiscoveryStatus)
	}
	if info.LastModelSyncAt != "2026-02-02T00:00:00Z" {
		t.Errorf("LastModelSyncAt = %q", info.LastModelSyncAt)
	}
	if len(info.LastDiscoveredModels) != 1 || info.LastDiscoveredModels[0] != "gpt-5" {
		t.Errorf("LastDiscoveredModels = %v", info.LastDiscoveredModels)
	}
}

func TestBuildOauthInfo_NoProviderReturnsError(t *testing.T) {
	_, err := BuildOauthInfo(nil, nil)
	if err == nil {
		t.Fatal("expected error when no provider can be determined")
	}
}

func TestBuildOauthInfo_PatchProviderUsedWhenNoExisting(t *testing.T) {
	// When extraConfig is nil, there's no existing provider to copy, so the
	// patch's Provider is what survives (info = &OauthInfo{Provider: provider}
	// and *info = *current is a no-op since current is nil).
	info, err := BuildOauthInfo(nil, &OauthInfo{Provider: "codex", Email: "patch@example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Provider != "codex" {
		t.Errorf("Provider = %q, want codex (patch provider used)", info.Provider)
	}
	if info.Email != "patch@example.com" {
		t.Errorf("Email = %q", info.Email)
	}
}

// ---- parseRawExtraConfig / parseRawExtraConfigGo: malformed JSON ----

func TestParseRawExtraConfig_MalformedJSONReturnsNil(t *testing.T) {
	malformed := `{not valid json}`
	if got := parseRawExtraConfig(&malformed); got != nil {
		t.Errorf("malformed JSON should return nil, got %+v", got)
	}
}

func TestParseRawExtraConfigGo_MalformedJSONReturnsNil(t *testing.T) {
	malformed := `{not valid json}`
	if got := parseRawExtraConfigGo(&malformed); got != nil {
		t.Errorf("malformed JSON should return nil, got %+v", got)
	}
}

func TestParseRawExtraConfigGo_EmptyStringReturnsNil(t *testing.T) {
	empty := `   `
	if got := parseRawExtraConfigGo(&empty); got != nil {
		t.Errorf("whitespace-only should return nil, got %+v", got)
	}
}

// ---- resolveAccountProxyURL: extraConfig with proxy ----

func TestResolveAccountProxyURL_FromExtraConfig(t *testing.T) {
	// resolveAccountProxyURL (in refresh.go) reads proxyUrl from extraConfig.
	extra := `{"proxyUrl":"http://refresh-proxy:3128"}`
	v := resolveAccountProxyURL(1, &extra)
	if v == nil || *v != "http://refresh-proxy:3128" {
		t.Errorf("got %v, want http://refresh-proxy:3128", v)
	}
}

func TestResolveAccountProxyURL_NoProxyInExtraConfig(t *testing.T) {
	extra := `{"other":"value"}`
	if v := resolveAccountProxyURL(1, &extra); v != nil {
		t.Errorf("no proxy in extraConfig = %v, want nil", v)
	}
}
