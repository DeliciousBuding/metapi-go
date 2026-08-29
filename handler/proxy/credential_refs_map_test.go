package proxyhandler

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/auth"
)

// TestRoutingPolicyFromAuth_CredentialRefs pins the auth -> routing mapping of
// both credential-ref dimensions. The selector receives exactly these values,
// so a dropped or zeroed field here silently disables the dimension even when
// storage and parsing are correct.
func TestRoutingPolicyFromAuth_CredentialRefs(t *testing.T) {
	t.Parallel()

	tokenID := int64(23)
	policy := auth.DownstreamRoutingPolicy{
		ExcludedCredentialRefs: []auth.ExcludedCredentialRef{
			{Kind: auth.CredentialRefAccountToken, SiteID: 11, AccountID: 12, TokenID: &tokenID},
		},
		AllowedCredentialRefs: []auth.ExcludedCredentialRef{
			{Kind: auth.CredentialRefAccountToken, SiteID: 21, AccountID: 22, TokenID: &tokenID},
			{Kind: auth.CredentialRefDefaultApiKey, SiteID: 21, AccountID: 24},
		},
	}

	got := routingPolicyFromAuth(policy)

	if len(got.ExcludedCredentialRefs) != 1 {
		t.Fatalf("expected 1 excluded ref, got %d", len(got.ExcludedCredentialRefs))
	}
	ex := got.ExcludedCredentialRefs[0]
	if ex.Kind != "account_token" || ex.SiteID != 11 || ex.AccountID != 12 || ex.TokenID != 23 {
		t.Fatalf("excluded ref mapped wrong: %+v", ex)
	}

	if len(got.AllowedCredentialRefs) != 2 {
		t.Fatalf("expected 2 allowed refs, got %d", len(got.AllowedCredentialRefs))
	}
	tok := got.AllowedCredentialRefs[0]
	if tok.Kind != "account_token" || tok.SiteID != 21 || tok.AccountID != 22 || tok.TokenID != 23 {
		t.Fatalf("allowed account_token ref mapped wrong: %+v", tok)
	}
	def := got.AllowedCredentialRefs[1]
	if def.Kind != "default_api_key" || def.SiteID != 21 || def.AccountID != 24 || def.TokenID != 0 {
		t.Fatalf("allowed default_api_key ref mapped wrong: %+v", def)
	}
}
