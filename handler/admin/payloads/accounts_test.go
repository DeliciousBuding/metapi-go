package payloads

import (
	"encoding/json"
	"testing"
)

// TestAccountCreatePayload_Decode covers valid/invalid/edge parsing of
// AccountCreatePayload — the request body for POST /api/accounts.
func TestAccountCreatePayload_Decode(t *testing.T) {
	t.Parallel()
	cases := []sitesRoundTripCase{
		{
			name:  "full valid single token",
			input: `{"siteId":5,"username":"u","accessToken":"tok","apiToken":"sk","platformUserId":9,"checkinEnabled":true,"credentialMode":"session","refreshToken":"rt","tokenExpiresAt":1700000000,"skipModelFetch":true}`,
			check: func(t *testing.T, d any) {
				p := d.(*AccountCreatePayload)
				if p.SiteID != 5 {
					t.Fatalf("siteId = %d", p.SiteID)
				}
				if p.Username == nil || *p.Username != "u" {
					t.Fatalf("username = %#v", p.Username)
				}
				if p.AccessToken == nil || *p.AccessToken != "tok" {
					t.Fatalf("accessToken = %#v", p.AccessToken)
				}
				if p.APIToken == nil || *p.APIToken != "sk" {
					t.Fatalf("apiToken = %#v", p.APIToken)
				}
				if p.PlatformUserID == nil || *p.PlatformUserID != 9 {
					t.Fatalf("platformUserId = %#v", p.PlatformUserID)
				}
				if p.CheckinEnabled == nil || !*p.CheckinEnabled {
					t.Fatalf("checkinEnabled = %#v", p.CheckinEnabled)
				}
				if p.CredentialMode == nil || *p.CredentialMode != "session" {
					t.Fatalf("credentialMode = %#v", p.CredentialMode)
				}
				if p.TokenExpiresAt == nil || *p.TokenExpiresAt != 1700000000 {
					t.Fatalf("tokenExpiresAt = %#v", p.TokenExpiresAt)
				}
				if p.SkipModelFetch == nil || !*p.SkipModelFetch {
					t.Fatalf("skipModelFetch = %#v", p.SkipModelFetch)
				}
			},
		},
		{
			name:  "multi access tokens",
			input: `{"siteId":1,"accessTokens":["a","b","c"]}`,
			check: func(t *testing.T, d any) {
				p := d.(*AccountCreatePayload)
				if len(p.AccessTokens) != 3 || p.AccessTokens[2] != "c" {
					t.Fatalf("accessTokens = %#v", p.AccessTokens)
				}
				if p.AccessToken != nil {
					t.Fatalf("single accessToken should be nil: %#v", p.AccessToken)
				}
			},
		},
		{
			name:    "empty object leaves siteId zero",
			input:   `{}`,
			check: func(t *testing.T, d any) {
				p := d.(*AccountCreatePayload)
				if p.SiteID != 0 || len(p.AccessTokens) != 0 {
					t.Fatalf("expected zero values: %#v", p)
				}
			},
		},
		{
			name:    "malformed json rejected",
			input:   `{"siteId":`,
			wantErr: true,
		},
		{
			name:    "wrong siteId type rejected",
			input:   `{"siteId":"x"}`,
			wantErr: true,
		},
		{
			name:    "wrong accessTokens element type rejected",
			input:   `{"siteId":1,"accessTokens":[1,2]}`,
			wantErr: true,
		},
		{
			name:    "wrong tokenExpiresAt type rejected",
			input:   `{"siteId":1,"tokenExpiresAt":"abc"}`,
			wantErr: true,
		},
	}
	runSitesDecode(t, cases, func() *AccountCreatePayload { return &AccountCreatePayload{} })
}

// TestAccountUpdatePayload_Decode covers the partial update shape, including
// the any-typed apiToken and extraConfig fields.
func TestAccountUpdatePayload_Decode(t *testing.T) {
	t.Parallel()
	cases := []sitesRoundTripCase{
		{
			name:  "partial update with any fields",
			input: `{"status":"disabled","unitCost":0.5,"apiToken":null,"extraConfig":{"sub2apiAuth":{"refreshToken":"rt"}},"sortOrder":2}`,
			check: func(t *testing.T, d any) {
				p := d.(*AccountUpdatePayload)
				if p.Status == nil || *p.Status != "disabled" {
					t.Fatalf("status = %#v", p.Status)
				}
				if p.UnitCost == nil || *p.UnitCost != 0.5 {
					t.Fatalf("unitCost = %#v", p.UnitCost)
				}
				// apiToken is any — JSON null leaves it nil.
				if p.APIToken != nil {
					t.Fatalf("apiToken = %#v, want nil", p.APIToken)
				}
				if p.ExtraConfig == nil {
					t.Fatalf("extraConfig should be present")
				}
				if p.SortOrder == nil || *p.SortOrder != 2 {
					t.Fatalf("sortOrder = %#v", p.SortOrder)
				}
			},
		},
		{
			name:    "empty object",
			input:   `{}`,
			check: func(t *testing.T, d any) {
				p := d.(*AccountUpdatePayload)
				if p.Status != nil || p.SortOrder != nil {
					t.Fatalf("expected nil optionals: %#v", p)
				}
			},
		},
		{
			name:    "malformed json rejected",
			input:   `{"status":`,
			wantErr: true,
		},
		{
			name:    "wrong unitCost type rejected",
			input:   `{"unitCost":"x"}`,
			wantErr: true,
		},
		{
			name:    "wrong sortOrder type rejected",
			input:   `{"sortOrder":"x"}`,
			wantErr: true,
		},
	}
	runSitesDecode(t, cases, func() *AccountUpdatePayload { return &AccountUpdatePayload{} })
}

// TestAccountBatchPayload_Decode covers the batch action shape.
func TestAccountBatchPayload_Decode(t *testing.T) {
	t.Parallel()
	cases := []sitesRoundTripCase{
		{
			name:  "valid batch",
			input: `{"ids":[10,20],"action":"pin"}`,
			check: func(t *testing.T, d any) {
				p := d.(*AccountBatchPayload)
				if len(p.IDs) != 2 || p.IDs[1] != 20 {
					t.Fatalf("ids = %#v", p.IDs)
				}
				if p.Action != "pin" {
					t.Fatalf("action = %q", p.Action)
				}
			},
		},
		{
			name:    "empty object",
			input:   `{}`,
			check: func(t *testing.T, d any) {
				p := d.(*AccountBatchPayload)
				if len(p.IDs) != 0 || p.Action != "" {
					t.Fatalf("expected zero values: %#v", p)
				}
			},
		},
		{
			name:    "malformed json rejected",
			input:   `{"ids":[`,
			wantErr: true,
		},
		{
			name:    "wrong ids element type rejected",
			input:   `{"ids":["x"]}`,
			wantErr: true,
		},
	}
	runSitesDecode(t, cases, func() *AccountBatchPayload { return &AccountBatchPayload{} })
}

// TestAccountLoginPayload_Decode covers the login shape.
func TestAccountLoginPayload_Decode(t *testing.T) {
	t.Parallel()
	cases := []sitesRoundTripCase{
		{
			name:  "valid login",
			input: `{"siteId":3,"username":"u","password":"p"}`,
			check: func(t *testing.T, d any) {
				p := d.(*AccountLoginPayload)
				if p.SiteID != 3 || p.Username != "u" || p.Password != "p" {
					t.Fatalf("login = %+v", p)
				}
			},
		},
		{
			name:    "empty object leaves fields zero",
			input:   `{}`,
			check: func(t *testing.T, d any) {
				p := d.(*AccountLoginPayload)
				if p.SiteID != 0 || p.Username != "" || p.Password != "" {
					t.Fatalf("expected zero values: %#v", p)
				}
			},
		},
		{
			name:    "malformed json rejected",
			input:   `{"siteId":`,
			wantErr: true,
		},
		{
			name:    "wrong password type rejected",
			input:   `{"siteId":1,"username":"u","password":5}`,
			wantErr: true,
		},
	}
	runSitesDecode(t, cases, func() *AccountLoginPayload { return &AccountLoginPayload{} })
}

// TestAccountVerifyTokenPayload_Decode covers the verify shape.
func TestAccountVerifyTokenPayload_Decode(t *testing.T) {
	t.Parallel()
	cases := []sitesRoundTripCase{
		{
			name:  "valid verify",
			input: `{"siteId":1,"accessToken":"tok","platformUserId":7,"credentialMode":"api"}`,
			check: func(t *testing.T, d any) {
				p := d.(*AccountVerifyTokenPayload)
				if p.SiteID != 1 || *p.AccessToken != "tok" || *p.PlatformUserID != 7 || *p.CredentialMode != "api" {
					t.Fatalf("verify = %+v", p)
				}
			},
		},
		{
			name:    "empty object",
			input:   `{}`,
			check: func(t *testing.T, d any) {
				p := d.(*AccountVerifyTokenPayload)
				if p.SiteID != 0 || p.AccessToken != nil {
					t.Fatalf("expected zero values: %#v", p)
				}
			},
		},
		{
			name:    "malformed json rejected",
			input:   `{"siteId":`,
			wantErr: true,
		},
		{
			name:    "wrong platformUserId type rejected",
			input:   `{"siteId":1,"platformUserId":"x"}`,
			wantErr: true,
		},
	}
	runSitesDecode(t, cases, func() *AccountVerifyTokenPayload { return &AccountVerifyTokenPayload{} })
}

// TestAccountRebindSessionPayload_Decode covers the rebind-session shape.
func TestAccountRebindSessionPayload_Decode(t *testing.T) {
	t.Parallel()
	cases := []sitesRoundTripCase{
		{
			name:  "valid rebind",
			input: `{"accessToken":"new","platformUserId":9,"refreshToken":"rt","tokenExpiresAt":1700000000}`,
			check: func(t *testing.T, d any) {
				p := d.(*AccountRebindSessionPayload)
				if p.AccessToken == nil || *p.AccessToken != "new" {
					t.Fatalf("accessToken = %#v", p.AccessToken)
				}
				if p.PlatformUserID == nil || *p.PlatformUserID != 9 {
					t.Fatalf("platformUserId = %#v", p.PlatformUserID)
				}
				if p.TokenExpiresAt == nil || *p.TokenExpiresAt != 1700000000 {
					t.Fatalf("tokenExpiresAt = %#v", p.TokenExpiresAt)
				}
			},
		},
		{
			name:    "empty object",
			input:   `{}`,
			check: func(t *testing.T, d any) {
				p := d.(*AccountRebindSessionPayload)
				if p.AccessToken != nil || p.TokenExpiresAt != nil {
					t.Fatalf("expected nil optionals: %#v", p)
				}
			},
		},
		{
			name:    "malformed json rejected",
			input:   `{"accessToken":`,
			wantErr: true,
		},
		{
			name:    "wrong tokenExpiresAt type rejected",
			input:   `{"tokenExpiresAt":"x"}`,
			wantErr: true,
		},
	}
	runSitesDecode(t, cases, func() *AccountRebindSessionPayload { return &AccountRebindSessionPayload{} })
}

// TestAccountHealthRefreshPayload_Decode covers the health refresh shape.
func TestAccountHealthRefreshPayload_Decode(t *testing.T) {
	t.Parallel()
	cases := []sitesRoundTripCase{
		{
			name:  "valid health refresh",
			input: `{"accountId":42,"wait":true}`,
			check: func(t *testing.T, d any) {
				p := d.(*AccountHealthRefreshPayload)
				if p.AccountID == nil || *p.AccountID != 42 {
					t.Fatalf("accountId = %#v", p.AccountID)
				}
				if p.Wait == nil || !*p.Wait {
					t.Fatalf("wait = %#v", p.Wait)
				}
			},
		},
		{
			name:    "empty object",
			input:   `{}`,
			check: func(t *testing.T, d any) {
				p := d.(*AccountHealthRefreshPayload)
				if p.AccountID != nil || p.Wait != nil {
					t.Fatalf("expected nil optionals: %#v", p)
				}
			},
		},
		{
			name:    "malformed json rejected",
			input:   `{"accountId":`,
			wantErr: true,
		},
		{
			name:    "wrong accountId type rejected",
			input:   `{"accountId":"x"}`,
			wantErr: true,
		},
	}
	runSitesDecode(t, cases, func() *AccountHealthRefreshPayload { return &AccountHealthRefreshPayload{} })
}

// TestAccountManualModelsPayload_Decode covers the manual models shape.
func TestAccountManualModelsPayload_Decode(t *testing.T) {
	t.Parallel()
	cases := []sitesRoundTripCase{
		{
			name:  "valid models",
			input: `{"models":["m1","m2"]}`,
			check: func(t *testing.T, d any) {
				p := d.(*AccountManualModelsPayload)
				if len(p.Models) != 2 || p.Models[0] != "m1" {
					t.Fatalf("models = %#v", p.Models)
				}
			},
		},
		{
			name:    "empty object",
			input:   `{}`,
			check: func(t *testing.T, d any) {
				if len(d.(*AccountManualModelsPayload).Models) != 0 {
					t.Fatalf("expected empty models")
				}
			},
		},
		{
			name:    "malformed json rejected",
			input:   `{"models":[`,
			wantErr: true,
		},
		{
			name:    "wrong model element type rejected",
			input:   `{"models":[1]}`,
			wantErr: true,
		},
	}
	runSitesDecode(t, cases, func() *AccountManualModelsPayload { return &AccountManualModelsPayload{} })
}

// TestAccountCreatePayload_OmitEmptyMarshal verifies optional pointer fields
// are omitted when nil.
func TestAccountCreatePayload_OmitEmptyMarshal(t *testing.T) {
	t.Parallel()
	p := AccountCreatePayload{SiteID: 5}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	encoded := string(raw)
	if !contains(encoded, `"siteId":5`) {
		t.Fatalf("siteId missing: %s", encoded)
	}
	if contains(encoded, "username") {
		t.Fatalf("nil username should be omitted: %s", encoded)
	}
	if contains(encoded, "accessToken") {
		t.Fatalf("nil accessToken should be omitted: %s", encoded)
	}
}

// contains is a small local helper to avoid pulling strings just for this.
func contains(s, sub string) bool { return len(s) >= len(sub) && indexOf(s, sub) >= 0 }

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
