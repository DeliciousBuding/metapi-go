package routing

import (
	"slices"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

func TestDirectGrantAllowedRouteMatchesNativeSupportedModelOrPolicy(t *testing.T) {
	selector := &ChannelSelector{}
	candidate := RouteChannelCandidate{
		Channel: store.RouteChannel{Enabled: true},
		Direct: &store.DirectUpstreamCandidate{
			RouteID: 42, ChannelID: 9, ChannelEnabled: true, ModelEnabled: true,
			CredentialEnabled: true, GrantEnabled: true, Protocols: 2,
		},
	}
	policy := DownstreamRoutingPolicy{
		AllowedRouteIDs: []int64{99}, SupportedModels: []string{"gpt-*"},
		RequiredUpstreamProtocol: 2,
	}
	routeDenied := CandidateBlocker{"route_not_allowed", "direct upstream route is not allowed by downstream key"}
	blockers := selector.getCandidateBlockers(candidate, "gpt-4o", false, nil, "", policy)
	if slices.Contains(blockers, routeDenied) {
		t.Fatalf("direct route was ANDed with supported model policy: %v", blockers)
	}
	blockers = selector.getCandidateBlockers(candidate, "claude-3", false, nil, "", policy)
	if !slices.Contains(blockers, routeDenied) {
		t.Fatalf("route-id-only policy did not restrict non-matching model: %v", blockers)
	}
}
