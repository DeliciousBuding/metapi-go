package payloads

import (
	"encoding/json"
	"testing"
)

// TestAccountTokenCreatePayload_Decode covers valid/invalid/edge parsing of
// AccountTokenCreatePayload — the request body for POST /api/account-tokens.
func TestAccountTokenCreatePayload_Decode(t *testing.T) {
	t.Parallel()
	cases := []sitesRoundTripCase{
		{
			name:  "full valid create",
			input: `{"accountId":5,"name":"primary","token":"sk-1","enabled":true,"isDefault":false,"source":"manual","group":"vip","unlimitedQuota":true,"remainQuota":1000.5,"expiredTime":"2026-12-31","allowIps":"10.0.0.0/8","modelLimitsEnabled":true,"modelLimits":"gpt-4,claude-3"}`,
			check: func(t *testing.T, d any) {
				p := d.(*AccountTokenCreatePayload)
				if p.AccountID != 5 {
					t.Fatalf("accountId = %d", p.AccountID)
				}
				if p.Name == nil || *p.Name != "primary" {
					t.Fatalf("name = %#v", p.Name)
				}
				if p.Token == nil || *p.Token != "sk-1" {
					t.Fatalf("token = %#v", p.Token)
				}
				if p.Enabled == nil || !*p.Enabled {
					t.Fatalf("enabled = %#v", p.Enabled)
				}
				if p.IsDefault == nil || *p.IsDefault {
					t.Fatalf("isDefault = %#v", p.IsDefault)
				}
				if p.Source == nil || *p.Source != "manual" {
					t.Fatalf("source = %#v", p.Source)
				}
				if p.Group == nil || *p.Group != "vip" {
					t.Fatalf("group = %#v", p.Group)
				}
				if p.UnlimitedQuota == nil || !*p.UnlimitedQuota {
					t.Fatalf("unlimitedQuota = %#v", p.UnlimitedQuota)
				}
				if p.RemainQuota == nil {
					t.Fatalf("remainQuota should be present")
				}
				if p.AllowIPs == nil || *p.AllowIPs != "10.0.0.0/8" {
					t.Fatalf("allowIps = %#v", p.AllowIPs)
				}
				if p.ModelLimitsEnabled == nil || !*p.ModelLimitsEnabled {
					t.Fatalf("modelLimitsEnabled = %#v", p.ModelLimitsEnabled)
				}
				if p.ModelLimits == nil || *p.ModelLimits != "gpt-4,claude-3" {
					t.Fatalf("modelLimits = %#v", p.ModelLimits)
				}
			},
		},
		{
			name:  "any-typed remainQuota accepts string",
			input: `{"accountId":1,"remainQuota":"unlimited"}`,
			check: func(t *testing.T, d any) {
				p := d.(*AccountTokenCreatePayload)
				if p.RemainQuota == nil {
					t.Fatal("remainQuota should be set")
				}
				if s, ok := p.RemainQuota.(string); !ok || s != "unlimited" {
					t.Fatalf("remainQuota = %#v, want string unlimited", p.RemainQuota)
				}
			},
		},
		{
			name:  "any-typed remainQuota accepts number",
			input: `{"accountId":1,"remainQuota":500}`,
			check: func(t *testing.T, d any) {
				p := d.(*AccountTokenCreatePayload)
				n, ok := p.RemainQuota.(float64)
				if !ok || n != 500 {
					t.Fatalf("remainQuota = %#v, want float 500", p.RemainQuota)
				}
			},
		},
		{
			name:  "any-typed expiredTime accepts number",
			input: `{"accountId":1,"expiredTime":1700000000}`,
			check: func(t *testing.T, d any) {
				p := d.(*AccountTokenCreatePayload)
				n, ok := p.ExpiredTime.(float64)
				if !ok || n != 1700000000 {
					t.Fatalf("expiredTime = %#v, want float", p.ExpiredTime)
				}
			},
		},
		{
			name:    "empty object leaves accountId zero",
			input:   `{}`,
			check: func(t *testing.T, d any) {
				p := d.(*AccountTokenCreatePayload)
				if p.AccountID != 0 || p.Name != nil || p.RemainQuota != nil {
					t.Fatalf("expected zero values: %#v", p)
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
		{
			name:    "wrong name type rejected",
			input:   `{"accountId":1,"name":5}`,
			wantErr: true,
		},
		{
			name:    "wrong enabled type rejected",
			input:   `{"accountId":1,"enabled":"yes"}`,
			wantErr: true,
		},
	}
	runSitesDecode(t, cases, func() *AccountTokenCreatePayload { return &AccountTokenCreatePayload{} })
}

// TestAccountTokenUpdatePayload_Decode covers the partial update shape.
func TestAccountTokenUpdatePayload_Decode(t *testing.T) {
	t.Parallel()
	cases := []sitesRoundTripCase{
		{
			name:  "partial update",
			input: `{"name":"renamed","enabled":false,"group":"g2","isDefault":true}`,
			check: func(t *testing.T, d any) {
				p := d.(*AccountTokenUpdatePayload)
				if p.Name == nil || *p.Name != "renamed" {
					t.Fatalf("name = %#v", p.Name)
				}
				if p.Enabled == nil || *p.Enabled {
					t.Fatalf("enabled = %#v", p.Enabled)
				}
				if p.Group == nil || *p.Group != "g2" {
					t.Fatalf("group = %#v", p.Group)
				}
				if p.IsDefault == nil || !*p.IsDefault {
					t.Fatalf("isDefault = %#v", p.IsDefault)
				}
				if p.Token != nil {
					t.Fatalf("untouched token should be nil: %#v", p.Token)
				}
			},
		},
		{
			name:    "empty object",
			input:   `{}`,
			check: func(t *testing.T, d any) {
				p := d.(*AccountTokenUpdatePayload)
				if p.Name != nil || p.Enabled != nil {
					t.Fatalf("expected nil optionals: %#v", p)
				}
			},
		},
		{
			name:    "malformed json rejected",
			input:   `{"name":`,
			wantErr: true,
		},
		{
			name:    "wrong isDefault type rejected",
			input:   `{"isDefault":"x"}`,
			wantErr: true,
		},
	}
	runSitesDecode(t, cases, func() *AccountTokenUpdatePayload { return &AccountTokenUpdatePayload{} })
}

// TestAccountTokenBatchPayload_Decode covers the batch action shape.
func TestAccountTokenBatchPayload_Decode(t *testing.T) {
	t.Parallel()
	cases := []sitesRoundTripCase{
		{
			name:  "valid batch",
			input: `{"ids":[1,2,3],"action":"disable"}`,
			check: func(t *testing.T, d any) {
				p := d.(*AccountTokenBatchPayload)
				if len(p.IDs) != 3 || p.IDs[0] != 1 {
					t.Fatalf("ids = %#v", p.IDs)
				}
				if p.Action != "disable" {
					t.Fatalf("action = %q", p.Action)
				}
			},
		},
		{
			name:    "empty object",
			input:   `{}`,
			check: func(t *testing.T, d any) {
				p := d.(*AccountTokenBatchPayload)
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
		{
			name:    "wrong action type rejected",
			input:   `{"ids":[1],"action":5}`,
			wantErr: true,
		},
	}
	runSitesDecode(t, cases, func() *AccountTokenBatchPayload { return &AccountTokenBatchPayload{} })
}

// TestAccountTokenSyncAllPayload_Decode covers the sync-all shape.
func TestAccountTokenSyncAllPayload_Decode(t *testing.T) {
	t.Parallel()
	cases := []sitesRoundTripCase{
		{
			name:  "wait true",
			input: `{"wait":true}`,
			check: func(t *testing.T, d any) {
				p := d.(*AccountTokenSyncAllPayload)
				if p.Wait == nil || !*p.Wait {
					t.Fatalf("wait = %#v", p.Wait)
				}
			},
		},
		{
			name:  "wait false",
			input: `{"wait":false}`,
			check: func(t *testing.T, d any) {
				p := d.(*AccountTokenSyncAllPayload)
				if p.Wait == nil || *p.Wait {
					t.Fatalf("wait = %#v", p.Wait)
				}
			},
		},
		{
			name:    "empty object",
			input:   `{}`,
			check: func(t *testing.T, d any) {
				if d.(*AccountTokenSyncAllPayload).Wait != nil {
					t.Fatalf("expected nil wait")
				}
			},
		},
		{
			name:    "malformed json rejected",
			input:   `{"wait":`,
			wantErr: true,
		},
		{
			name:    "wrong wait type rejected",
			input:   `{"wait":"yes"}`,
			wantErr: true,
		},
	}
	runSitesDecode(t, cases, func() *AccountTokenSyncAllPayload { return &AccountTokenSyncAllPayload{} })
}

// TestAccountTokenCreatePayload_OmitEmptyMarshal verifies optional pointer
// fields are omitted when nil.
func TestAccountTokenCreatePayload_OmitEmptyMarshal(t *testing.T) {
	t.Parallel()
	p := AccountTokenCreatePayload{AccountID: 5}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	encoded := string(raw)
	if !contains(encoded, `"accountId":5`) {
		t.Fatalf("accountId missing: %s", encoded)
	}
	if contains(encoded, "name") {
		t.Fatalf("nil name should be omitted: %s", encoded)
	}
	if contains(encoded, "token") {
		t.Fatalf("nil token should be omitted: %s", encoded)
	}
}
