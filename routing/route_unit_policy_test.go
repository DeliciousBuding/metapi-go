package routing

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

func TestOAuthPoolPolicyPreviewPreservesSharedMembers(t *testing.T) {
	model := "pool-model"
	pool := RouteChannelCandidate{
		Channel:   store.RouteChannel{ID: 1, Enabled: true, SourceModel: &model},
		RouteUnit: &OAuthRouteUnitSummary{ID: 1, Enabled: true},
		RouteUnitMembers: []OAuthRouteUnitMemberCandidate{
			makeMember(1, "active", "active", "", "fixture-one"),
			makeMember(2, "active", "active", "", "fixture-two"),
		},
	}
	selector := &ChannelSelector{}
	match := &RouteMatch{Route: store.TokenRoute{ID: 1, ModelPattern: model}, Channels: []RouteChannelCandidate{pool}}
	for _, tc := range []struct {
		sites []int64
		want  string
	}{
		{[]int64{20}, "eligible"},
		{[]int64{30}, "blocked"},
		{nil, "eligible"},
	} {
		d := explainSelectionFromMatch(selector, match, model, nil, DownstreamRoutingPolicy{AllowedSiteIDs: tc.sites, RequiredUpstreamProtocol: store.DirectProtocolChat}, 60)
		if got := d.Candidates[0]; got.Eligibility != tc.want || got.AccountID != 0 || got.ActualModel != "" {
			t.Fatalf("pool preview sites=%v: %+v", tc.sites, got)
		}
		if len(match.Channels[0].RouteUnitMembers) != 2 || match.Channels[0].RouteUnitMembers[0].Account.ID != 1 {
			t.Fatal("key policy changed shared pool membership")
		}
	}
}

func TestOAuthPoolCredentialScopeUsesDispatchedToken(t *testing.T) {
	member := makeMember(1, "active", "active", "fixture-oauth", "fixture-api")
	pool := RouteChannelCandidate{RouteUnit: &OAuthRouteUnitSummary{ID: 1}, RouteUnitMembers: []OAuthRouteUnitMemberCandidate{member}}
	policy := DownstreamRoutingPolicy{AllowedCredentialRefs: []CredentialRef{{Kind: "default_api_key", SiteID: 10, AccountID: 1}}}
	selector := &ChannelSelector{}
	if got := selector.filterRouteUnitMembersByPolicy(pool, policy); len(got.RouteUnitMembers) != 0 {
		t.Fatal("default API key scope authorized a different access token")
	}
	pool.RouteUnitMembers[0].Account.AccessToken = ""
	if got := selector.filterRouteUnitMembersByPolicy(pool, policy); len(got.RouteUnitMembers) != 1 {
		t.Fatal("default API key scope rejected the dispatched API token")
	}
}
