package routing

import (
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/internal/routecondition"
	"github.com/deliciousbuding/metapi-go/store"
)

func routingCondition(t *testing.T, raw string) *routecondition.Condition {
	t.Helper()
	value, err := routecondition.Decode([]byte(`{"type":"group","conditions":[` + raw + `]}`))
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestAssociationPriorityIsRequestLocalAndAlternativesAreOR(t *testing.T) {
	row := explanationDirectCandidate()
	row.Direct.ProtocolOrder = store.DirectProtocolOrder{store.DirectProtocolChat, store.DirectProtocolResponses}
	row.Direct.AssociationConditions = store.DirectAssociationConditions{
		{Priority: 1, ProtocolOrder: store.DirectProtocolOrder{store.DirectProtocolResponses}, Condition: routingCondition(t, `{"field":"stream","operator":"eq","value":true}`)},
		{Priority: 4, ProtocolOrder: store.DirectProtocolOrder{store.DirectProtocolChat}, Condition: routingCondition(t, `{"field":"request_header.X-Tier","operator":"eq","value":"gold"}`)},
		{Priority: 9, ProtocolOrder: store.DirectProtocolOrder{store.DirectProtocolChat}},
	}
	original := row.Channel.PriorityOrZero()
	for _, tc := range []struct {
		stream   bool
		tier     string
		priority int64
		protocol int
	}{{false, "", 9, store.DirectProtocolChat}, {false, "gold", 4, store.DirectProtocolChat}, {true, "", 1, store.DirectProtocolResponses}, {false, "", 9, store.DirectProtocolChat}} {
		policy := DownstreamRoutingPolicy{AssociationRequest: &routecondition.Request{Stream: tc.stream, Headers: map[string]string{"X-Tier": tc.tier}, Now: time.Now()}}
		resolved, blocker := resolveAssociationCandidate(row, policy)
		if blocker.Code != "" || resolved.Channel.PriorityOrZero() != tc.priority || len(resolved.Direct.ProtocolOrder) != 1 || resolved.Direct.ProtocolOrder[0] != tc.protocol {
			t.Fatalf("wrong matching association: row=%+v blocker=%+v", resolved, blocker)
		}
		if row.Channel.PriorityOrZero() != original || len(row.Direct.ProtocolOrder) != 2 {
			t.Fatal("request altered the cached candidate")
		}
	}
	if _, blocker := resolveAssociationCandidate(row, DownstreamRoutingPolicy{}); blocker.Code != "association_context_missing" {
		t.Fatal("missing context chose a lower-priority unconditional rule")
	}
}

func TestUnconditionalCandidatesNeverBuildRequestDescriptor(t *testing.T) {
	calls := 0
	policy := DownstreamRoutingPolicy{ResolveAssociationRequest: func() *routecondition.Request {
		calls++
		return &routecondition.Request{Stream: true}
	}}
	row := explanationDirectCandidate()
	if _, blocker := resolveAssociationCandidate(row, policy); blocker.Code != "" || calls != 0 {
		t.Fatal("ordinary grant unnecessarily parsed the request")
	}
	row.Direct.AssociationConditions = store.DirectAssociationConditions{
		{Priority: 1, ProtocolOrder: store.DirectProtocolOrder{store.DirectProtocolChat}, Condition: routingCondition(t, `{"field":"stream","operator":"eq","value":true}`)},
	}
	if _, blocker := resolveAssociationCandidate(row, policy); blocker.Code != "" || calls != 1 {
		t.Fatal("conditional grant did not request its descriptor")
	}
}

func TestAssociationExplanationDistinguishesMissingMismatchAndHardDenial(t *testing.T) {
	row := explanationDirectCandidate()
	row.Direct.AssociationConditions = store.DirectAssociationConditions{
		{Priority: 3, ProtocolOrder: store.DirectProtocolOrder{store.DirectProtocolChat}, Condition: routingCondition(t, `{"field":"stream","operator":"eq","value":true}`)},
	}
	selector := &ChannelSelector{}
	match := &RouteMatch{Route: store.TokenRoute{ID: 1, ModelPattern: "coding"}, Channels: []RouteChannelCandidate{row}}
	for _, tc := range []struct {
		input *routecondition.Request
		want  string
	}{{nil, "notEvaluated"}, {&routecondition.Request{}, "blocked"}, {&routecondition.Request{Stream: true}, "eligible"}} {
		policy := DownstreamRoutingPolicy{RequiredUpstreamProtocol: store.DirectProtocolChat, AssociationRequest: tc.input}
		decision := explainSelectionFromMatch(selector, match, "coding", nil, policy, 60)
		setExplanationContext(&decision, policy)
		if decision.Candidates[0].Eligibility != tc.want {
			t.Fatalf("got %s want %s", decision.Candidates[0].Eligibility, tc.want)
		}
		if tc.input == nil && !decision.Candidates[0].AssociationContextRequired {
			t.Fatal("missing association context was not exposed")
		}
	}
	policy := DownstreamRoutingPolicy{RequiredUpstreamProtocol: store.DirectProtocolChat, AssociationRequest: &routecondition.Request{Stream: true}, AllowedSiteIDs: []int64{99}}
	decision := explainSelectionFromMatch(selector, match, "coding", nil, policy, 60)
	if decision.Candidates[0].Eligible || decision.Candidates[0].Eligibility != "blocked" {
		t.Fatal("matching condition bypassed hard key policy")
	}
	row.Direct.ProtocolOrder = store.DirectProtocolOrder{store.DirectProtocolResponses}
	if _, blocker := resolveAssociationCandidate(row, policy); blocker.Code != "protocol_not_authorized" {
		t.Fatal("matching source association expanded the member's current protocol restriction")
	}
}
