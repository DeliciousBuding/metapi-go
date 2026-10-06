package routing

import (
	"strings"
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
	reasons := selector.getCandidateEligibilityReasons(candidate, "gpt-4o", false, nil, "", policy)
	if strings.Contains(strings.Join(reasons, " "), "route is not allowed") {
		t.Fatalf("direct route was ANDed with supported model policy: %v", reasons)
	}
	reasons = selector.getCandidateEligibilityReasons(candidate, "claude-3", false, nil, "", policy)
	if !strings.Contains(strings.Join(reasons, " "), "route is not allowed") {
		t.Fatalf("route-id-only policy did not restrict non-matching model: %v", reasons)
	}
}
