package routing

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

func TestDecisionsRequiresExplicitDirectEndpointAndGrant(t *testing.T) {
	model, token, provider := "fixture-model", "synthetic-credential", "codex"
	unitID := int64(7)
	for _, candidate := range []RouteChannelCandidate{
		{Channel: store.RouteChannel{ID: 1, SourceModel: &model, Enabled: true}, Account: store.Account{ID: 1, Status: "active", APIToken: &token}, Site: store.Site{ID: 1, Status: "active"}},
		{Channel: store.RouteChannel{ID: 2, SourceModel: &model, Enabled: true}, Account: store.Account{ID: 2, Status: "active", OAuthProvider: &provider, AccessToken: token}, Site: store.Site{ID: 1, Status: "active"}},
		{Channel: store.RouteChannel{ID: 3, OAuthRouteUnitID: &unitID, SourceModel: &model, Enabled: true}, RouteUnit: &OAuthRouteUnitSummary{ID: 7, Enabled: true}, RouteUnitMembers: []OAuthRouteUnitMemberCandidate{{Account: store.Account{ID: 2, Status: "active", AccessToken: token}}}},
	} {
		selector := &ChannelSelector{}
		if blockers := selector.getCandidateBlockers(candidate, model, false, nil, "", DownstreamRoutingPolicy{RequiredUpstreamProtocol: store.DirectProtocolChat}); len(blockers) != 0 {
			t.Fatalf("invalid positive-control fixture: %+v", blockers)
		}
		if blockers := selector.getCandidateBlockers(candidate, model, false, nil, "", DownstreamRoutingPolicy{RequiredUpstreamProtocol: -1}); len(blockers) != 0 {
			t.Fatalf("model discovery lost a shared account: %+v", blockers)
		}
		blockers := selector.getCandidateBlockers(candidate, model, false, nil, "", DownstreamRoutingPolicy{RequiredUpstreamProtocol: store.DirectProtocolDecisions})
		if len(blockers) != 1 || blockers[0].Code != "protocol_not_authorized" {
			t.Fatalf("account or OAuth pool borrowed a Decisions grant: %+v", blockers)
		}
	}
	candidate := explanationDirectCandidate()
	candidate.Direct.Protocols = store.DirectProtocolDecisions
	candidate.Direct.Endpoints = store.DirectEndpoints{Decisions: &store.DirectEndpoint{URL: "https://fixture.invalid/decide", Auth: store.DirectAuthBearer}}
	if blockers := (&ChannelSelector{}).getCandidateBlockers(candidate, model, false, nil, "", DownstreamRoutingPolicy{RequiredUpstreamProtocol: store.DirectProtocolDecisions}); len(blockers) != 0 {
		t.Fatalf("explicit Decisions capability was rejected: %+v", blockers)
	}
}
