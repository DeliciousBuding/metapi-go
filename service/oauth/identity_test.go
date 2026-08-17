package oauth

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

// buildTestJWT assembles an unsigned JWT whose payload is the JSON-marshaled
// claim map. Only the payload segment carries real data; header and signature
// are filler since ParseCodexIDToken / ParseCodexAccessToken do not verify.
func buildTestJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	header := map[string]string{"alg": "none", "typ": "JWT"}
	headerBytes, _ := json.Marshal(header)
	payloadBytes, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	encode := func(b []byte) string {
		return strings.TrimRight(base64.URLEncoding.EncodeToString(b), "=")
	}
	return encode(headerBytes) + "." + encode(payloadBytes) + ".sig"
}

func idTokenClaims(email, accountID, planType string) map[string]any {
	claims := map[string]any{}
	if email != "" {
		claims["email"] = email
	}
	auth := map[string]any{}
	if accountID != "" {
		auth["chatgpt_account_id"] = accountID
	}
	if planType != "" {
		auth["chatgpt_plan_type"] = planType
	}
	if len(auth) > 0 {
		claims["https://api.openai.com/auth"] = auth
	}
	return claims
}

func accessTokenClaims(profileEmail, accountID, planType string) map[string]any {
	claims := map[string]any{}
	if profileEmail != "" {
		claims["https://api.openai.com/profile"] = map[string]any{"email": profileEmail}
	}
	auth := map[string]any{}
	if accountID != "" {
		auth["chatgpt_account_id"] = accountID
	}
	if planType != "" {
		auth["chatgpt_plan_type"] = planType
	}
	if len(auth) > 0 {
		claims["https://api.openai.com/auth"] = auth
	}
	return claims
}

func TestParseCodexIDToken_FullIdentity(t *testing.T) {
	token := buildTestJWT(t, idTokenClaims("user@example.com", "acc_123", "plus"))
	got := ParseCodexIDToken(token)
	if got.Email != "user@example.com" {
		t.Errorf("Email = %q", got.Email)
	}
	if got.ChatGPTAccountID != "acc_123" {
		t.Errorf("ChatGPTAccountID = %q", got.ChatGPTAccountID)
	}
	if got.PlanType != "plus" {
		t.Errorf("PlanType = %q", got.PlanType)
	}
}

func TestParseCodexIDToken_EmptyTokenReturnsEmptyIdentity(t *testing.T) {
	got := ParseCodexIDToken("")
	if got == nil {
		t.Fatal("expected non-nil identity")
	}
	if got.Email != "" || got.ChatGPTAccountID != "" || got.PlanType != "" {
		t.Errorf("expected empty identity, got %+v", got)
	}
}

func TestParseCodexIDToken_MalformedJWTReturnsEmpty(t *testing.T) {
	got := ParseCodexIDToken("not.a.jwt")
	if got == nil {
		t.Fatal("expected non-nil identity")
	}
	if got.Email != "" {
		t.Errorf("expected empty Email, got %q", got.Email)
	}
}

func TestParseCodexIDToken_MissingPlanType(t *testing.T) {
	token := buildTestJWT(t, idTokenClaims("user@example.com", "acc_123", ""))
	got := ParseCodexIDToken(token)
	if got.Email != "user@example.com" {
		t.Errorf("Email = %q", got.Email)
	}
	if got.ChatGPTAccountID != "acc_123" {
		t.Errorf("ChatGPTAccountID = %q", got.ChatGPTAccountID)
	}
	if got.PlanType != "" {
		t.Errorf("expected empty PlanType, got %q", got.PlanType)
	}
}

func TestParseCodexAccessToken_ProfileEmailNamespace(t *testing.T) {
	token := buildTestJWT(t, accessTokenClaims("at-user@example.com", "acc_456", "pro"))
	got := ParseCodexAccessToken(token)
	if got == nil {
		t.Fatal("expected non-nil identity")
	}
	if got.Email != "at-user@example.com" {
		t.Errorf("Email = %q", got.Email)
	}
	if got.ChatGPTAccountID != "acc_456" {
		t.Errorf("ChatGPTAccountID = %q", got.ChatGPTAccountID)
	}
	if got.PlanType != "pro" {
		t.Errorf("PlanType = %q", got.PlanType)
	}
}

func TestParseCodexAccessToken_EmptyTokenReturnsNil(t *testing.T) {
	if got := ParseCodexAccessToken(""); got != nil {
		t.Errorf("expected nil for empty token, got %+v", got)
	}
}

func TestParseCodexAccessToken_MalformedReturnsNil(t *testing.T) {
	if got := ParseCodexAccessToken("not-a-jwt"); got != nil {
		t.Errorf("expected nil for malformed token, got %+v", got)
	}
}

func TestMergeCodexIdentity_FillsEmptyFieldsFromFallback(t *testing.T) {
	primary := &AccountIdentity{Email: "primary@example.com"} // accountID + planType empty
	fallback := &AccountIdentity{
		Email:             "fallback@example.com",
		ChatGPTAccountID:  "acc_fb",
		PlanType:          "pro",
	}
	merged := MergeCodexIdentity(primary, fallback)
	if merged.Email != "primary@example.com" {
		t.Errorf("Email should stay primary, got %q", merged.Email)
	}
	if merged.ChatGPTAccountID != "acc_fb" {
		t.Errorf("ChatGPTAccountID should be filled from fallback, got %q", merged.ChatGPTAccountID)
	}
	if merged.PlanType != "pro" {
		t.Errorf("PlanType should be filled from fallback, got %q", merged.PlanType)
	}
}

func TestMergeCodexIdentity_NilFallbackIsNoOp(t *testing.T) {
	primary := &AccountIdentity{Email: "keep@example.com", ChatGPTAccountID: "acc_1"}
	merged := MergeCodexIdentity(primary, nil)
	if merged.Email != "keep@example.com" || merged.ChatGPTAccountID != "acc_1" {
		t.Errorf("nil fallback should not mutate primary, got %+v", merged)
	}
}

func TestMergeCodexIdentity_NilPrimaryReturnsFallbackCopy(t *testing.T) {
	fallback := &AccountIdentity{Email: "fb@example.com"}
	merged := MergeCodexIdentity(nil, fallback)
	if merged.Email != "fb@example.com" {
		t.Errorf("expected fallback email, got %q", merged.Email)
	}
}

func TestIdentityIncomplete(t *testing.T) {
	cases := []struct {
		name    string
		identity *AccountIdentity
		want    bool
	}{
		{"nil", nil, true},
		{"all empty", &AccountIdentity{}, true},
		{"missing plan", &AccountIdentity{Email: "e", ChatGPTAccountID: "a"}, true},
		{"missing account", &AccountIdentity{Email: "e", PlanType: "p"}, true},
		{"missing email", &AccountIdentity{ChatGPTAccountID: "a", PlanType: "p"}, true},
		{"complete", &AccountIdentity{Email: "e", ChatGPTAccountID: "a", PlanType: "p"}, false},
		{"whitespace only", &AccountIdentity{Email: "  "}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := identityIncomplete(tc.identity); got != tc.want {
				t.Errorf("identityIncomplete(%+v) = %v, want %v", tc.identity, got, tc.want)
			}
		})
	}
}

func TestDecodeJWTPayload_StandardBase64Fallback(t *testing.T) {
	header := base64.StdEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload := base64.StdEncoding.EncodeToString([]byte(`{"email":"std@example.com"}`))
	token := header + "." + payload + ".sig"
	got := ParseCodexIDToken(token)
	if got.Email != "std@example.com" {
		t.Errorf("expected standard-base64 fallback to parse, got Email=%q", got.Email)
	}
}
