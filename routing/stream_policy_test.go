package routing

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

func TestStreamPolicyMissingContextAndHardPermissions(t *testing.T) {
	row := explanationDirectCandidate()
	row.Direct.StreamPolicy = store.DirectStreamRequire
	match := &RouteMatch{Route: store.TokenRoute{ID: 1, ModelPattern: "coding"}, Channels: []RouteChannelCandidate{row}}
	selector := &ChannelSelector{}
	policy := DownstreamRoutingPolicy{RequiredUpstreamProtocol: store.DirectProtocolChat}
	decision := explainSelectionFromMatch(selector, match, "coding", nil, policy, 60)
	setExplanationContext(&decision, policy)
	candidate := decision.Candidates[0]
	if candidate.Eligible || candidate.Eligibility != "notEvaluated" || !candidate.StreamContextRequired || len(candidate.Blockers) != 0 {
		t.Fatalf("missing stream context was guessed: %+v", candidate)
	}
	foundStream := false
	for _, dimension := range decision.NotEvaluated {
		foundStream = foundStream || dimension == "stream"
	}
	if !foundStream {
		t.Fatal("missing stream dimension was not exposed")
	}
	nonstream := false
	policy.RequestStream = &nonstream
	if blocker := streamPolicyBlocker(row, policy); blocker.Code != "stream_aggregation_unsupported" {
		t.Fatalf("nonaggregate request allowed: %+v", blocker)
	}
	policy.AllowStreamAggregation = true
	policy.AllowedSiteIDs = []int64{99}
	decision = explainSelectionFromMatch(selector, match, "coding", nil, policy, 60)
	if decision.Candidates[0].Eligible {
		t.Fatal("require bypassed hard Key scope")
	}
}

func TestStreamPolicyNativePreferenceAfterHardEligibility(t *testing.T) {
	required := explanationDirectCandidate()
	required.Channel.ID = -1
	required.Direct.StreamPolicy = store.DirectStreamRequire
	native := explanationDirectCandidate()
	native.Channel.ID = -2
	native.Direct.StreamPolicy = store.DirectStreamForbid
	match := &RouteMatch{Route: store.TokenRoute{ID: 1, ModelPattern: "coding"}, Channels: []RouteChannelCandidate{required, native}}
	stream := false
	policy := DownstreamRoutingPolicy{RequiredUpstreamProtocol: store.DirectProtocolChat, RequestStream: &stream, AllowStreamAggregation: true}
	decision := explainSelectionFromMatch(&ChannelSelector{}, match, "coding", nil, policy, 60)
	if decision.Candidates[0].Eligible || !decision.Candidates[1].Eligible || decision.Candidates[0].Blockers[0].Code != "stream_native_preferred" {
		t.Fatalf("native preference absent: %+v", decision.Candidates)
	}
	native.Direct.ChannelEnabled = false
	match.Channels[1] = native
	decision = explainSelectionFromMatch(&ChannelSelector{}, match, "coding", nil, policy, 60)
	if !decision.Candidates[0].Eligible || decision.Candidates[1].Eligible {
		t.Fatalf("disabled native suppressed require: %+v", decision.Candidates)
	}
	stream = true
	native.Direct.ChannelEnabled = true
	match.Channels[1] = native
	decision = explainSelectionFromMatch(&ChannelSelector{}, match, "coding", nil, policy, 60)
	if !decision.Candidates[0].Eligible || decision.Candidates[1].Eligible {
		t.Fatal("stream reached forbidden channel")
	}
}

func TestQueuedStreamPolicyRevalidationHonorsNewNativeCandidate(t *testing.T) {
	resetHealthForCacheTest(t)
	required, native := explanationDirectCandidate(), explanationDirectCandidate()
	required.Channel.ID, required.Channel.RouteID = -1, 1
	required.Direct.StreamPolicy = store.DirectStreamRequire
	native.Channel.ID, native.Channel.RouteID = -2, 1
	native.Direct.ChannelID = 12
	db := &preferredDB{routes: []store.TokenRoute{{ID: 1, ModelPattern: "model", Enabled: true}}}
	for _, candidate := range []RouteChannelCandidate{required, native} {
		candidate.Channel.Direct = candidate.Direct
		db.joined = append(db.joined, struct {
			Channel store.RouteChannel
			Account store.Account
			Site    store.Site
			Token   *store.AccountToken
		}{Channel: candidate.Channel})
	}
	router := &TokenRouter{selector: newPreferredSelector(db)}
	stream := false
	policy := DownstreamRoutingPolicy{RequiredUpstreamProtocol: store.DirectProtocolChat, RequestStream: &stream, AllowStreamAggregation: true}
	selected := &SelectedChannel{Channel: required.Channel, Direct: required.Direct}
	revalidated, err := router.RevalidateQueuedChannel(t.Context(), selected, "model", policy)
	if err != nil || revalidated != nil {
		t.Fatalf("queued require survived a new eligible native candidate: %+v %v", revalidated, err)
	}
}
