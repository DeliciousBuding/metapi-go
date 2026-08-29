package routing

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

// These tests pin ChannelSelector enforcement of the credential-ref
// dimensions (excludedCredentialRefs / allowedCredentialRefs) at the routing
// layer, mirroring the allowedSiteIds coverage in allowlist_test.go.
// Ref shape contract: {"kind","siteId","accountId","tokenId"} — see
// auth.ExcludedCredentialRef and docs/api.md (Downstream API Keys).

func tokenChannelCandidate(siteID, accountID, tokenID int64, tokenValue string) RouteChannelCandidate {
	return RouteChannelCandidate{
		Site:    store.Site{ID: siteID},
		Account: store.Account{ID: accountID},
		Channel: store.RouteChannel{TokenID: &tokenID},
		Token:   &store.AccountToken{ID: tokenID, Token: tokenValue, Enabled: true},
	}
}

func apiTokenChannelCandidate(siteID, accountID int64, apiToken string) RouteChannelCandidate {
	return RouteChannelCandidate{
		Site:    store.Site{ID: siteID},
		Account: store.Account{ID: accountID, APIToken: &apiToken},
		Channel: store.RouteChannel{TokenID: nil}, // no explicit token → account default api key
		Token:   nil,
	}
}

func TestResolveDownstreamExclusionReason_AllowedCredentialListDefaultApiKey(t *testing.T) {
	t.Parallel()
	s := &ChannelSelector{}
	cand := apiTokenChannelCandidate(2, 3, "sk-default")

	// Listed default_api_key credential passes the allow gate.
	reason := s.resolveDownstreamExclusionReason(cand, DownstreamRoutingPolicy{
		AllowedCredentialRefs: []CredentialRef{
			{Kind: "default_api_key", SiteID: 2, AccountID: 3},
		},
	})
	if reason != "" {
		t.Fatalf("listed default_api_key credential should be eligible, got %q", reason)
	}

	// A different account's default key is rejected.
	reason = s.resolveDownstreamExclusionReason(cand, DownstreamRoutingPolicy{
		AllowedCredentialRefs: []CredentialRef{
			{Kind: "default_api_key", SiteID: 2, AccountID: 99},
		},
	})
	if reason == "" {
		t.Fatal("unlisted default_api_key credential should be rejected")
	}
}

func TestResolveDownstreamExclusionReason_AllowedCredentialCrossKind(t *testing.T) {
	t.Parallel()
	s := &ChannelSelector{}

	// An account_token allow-ref must not admit the same account's
	// default-api-key channel (different credential class).
	apiCand := apiTokenChannelCandidate(2, 3, "sk-default")
	reason := s.resolveDownstreamExclusionReason(apiCand, DownstreamRoutingPolicy{
		AllowedCredentialRefs: []CredentialRef{
			{Kind: "account_token", SiteID: 2, AccountID: 3, TokenID: 7},
		},
	})
	if reason == "" {
		t.Fatal("account_token allow-ref must not match a default-api-key channel")
	}

	// And a default_api_key allow-ref must not admit an explicit-token channel.
	tokCand := tokenChannelCandidate(2, 3, 7, "sk-token")
	reason = s.resolveDownstreamExclusionReason(tokCand, DownstreamRoutingPolicy{
		AllowedCredentialRefs: []CredentialRef{
			{Kind: "default_api_key", SiteID: 2, AccountID: 3},
		},
	})
	if reason == "" {
		t.Fatal("default_api_key allow-ref must not match an explicit-token channel")
	}
}

func TestResolveDownstreamExclusionReason_AllowedCredentialEmptyUnrestricted(t *testing.T) {
	t.Parallel()
	s := &ChannelSelector{}
	cand := tokenChannelCandidate(2, 3, 7, "sk-token")

	// Empty credential allow-list leaves the credential dimension unrestricted.
	reason := s.resolveDownstreamExclusionReason(cand, DownstreamRoutingPolicy{})
	if reason != "" {
		t.Fatalf("empty policy should be unrestricted, got %q", reason)
	}
}

func TestResolveDownstreamExclusionReason_ExcludeBeatsAllowCredential(t *testing.T) {
	t.Parallel()
	s := &ChannelSelector{}
	cand := tokenChannelCandidate(2, 3, 7, "sk-token")
	ref := CredentialRef{Kind: "account_token", SiteID: 2, AccountID: 3, TokenID: 7}

	// Same credential in both lists: the exclude list wins (deny).
	reason := s.resolveDownstreamExclusionReason(cand, DownstreamRoutingPolicy{
		AllowedCredentialRefs:  []CredentialRef{ref},
		ExcludedCredentialRefs: []CredentialRef{ref},
	})
	if reason == "" {
		t.Fatal("exclude should still reject an allowed credential")
	}
}

func TestResolveDownstreamExclusionReason_LegacyEmptyKindBehavesAsDefaultApiKey(t *testing.T) {
	t.Parallel()
	s := &ChannelSelector{}
	cand := apiTokenChannelCandidate(2, 3, "sk-default")

	// Rows migrated from the TS version may carry refs without a kind;
	// the selector treats any non-account_token kind with the
	// default_api_key semantics. Pin that legacy behavior explicitly.
	reason := s.resolveDownstreamExclusionReason(cand, DownstreamRoutingPolicy{
		ExcludedCredentialRefs: []CredentialRef{
			{Kind: "", SiteID: 2, AccountID: 3},
		},
	})
	if reason == "" {
		t.Fatal("empty-kind legacy ref should exclude the default-api-key channel")
	}
}

func TestCredentialRefMatchesOne_TokenChannelRequiresAllTripleFields(t *testing.T) {
	t.Parallel()
	s := &ChannelSelector{}
	ref := CredentialRef{Kind: "account_token", SiteID: 2, AccountID: 3, TokenID: 7}

	// Full triple matches.
	if !credentialRefMatchesOne(tokenChannelCandidate(2, 3, 7, "sk-token"), ref, s) {
		t.Fatal("exact triple should match")
	}
	// Any single field off → no match.
	if credentialRefMatchesOne(tokenChannelCandidate(9, 3, 7, "sk-token"), ref, s) {
		t.Fatal("different site must not match")
	}
	if credentialRefMatchesOne(tokenChannelCandidate(2, 9, 7, "sk-token"), ref, s) {
		t.Fatal("different account must not match")
	}
	if credentialRefMatchesOne(tokenChannelCandidate(2, 3, 9, "sk-token"), ref, s) {
		t.Fatal("different token must not match")
	}
	// Channel whose token row failed to load (Token nil) must not match.
	broken := tokenChannelCandidate(2, 3, 7, "sk-token")
	broken.Token = nil
	if credentialRefMatchesOne(broken, ref, s) {
		t.Fatal("candidate with missing token row must not match")
	}
}
