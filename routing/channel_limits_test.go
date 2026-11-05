package routing

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/internal/channellimit"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestChannelRateLimitsRankWithoutExpandingAuthorization(t *testing.T) {
	resetHealthForCacheTest(t)
	manager := channellimit.NewManager()
	selector := newPreferredSelector(&preferredDB{})
	selector.channelLimits = manager
	first := explanationDirectCandidate()
	first.Direct.RateLimit.Config = channellimit.Config{TPM: 5, MaxConcurrent: 2}
	second := explanationDirectCandidate()
	secondDirect := *second.Direct
	second.Direct = &secondDirect
	second.Channel.ID, second.Direct.ItemID, second.Direct.ChannelID = -32, 32, 12
	second.Direct.RateLimit.Config = first.Direct.RateLimit.Config
	match := &RouteMatch{
		Route:    store.TokenRoute{ID: 1, ModelPattern: "model", RoutingStrategy: "least_busy"},
		Channels: []RouteChannelCandidate{first, second},
	}
	policy := DownstreamRoutingPolicy{RequiredUpstreamProtocol: store.DirectProtocolChat}
	manager.RecordUsage(first.Direct.ChannelID, 6, true)
	selected, err := selector.selectFromMatch(t.Context(), match, "model", policy, nil, false)
	if err != nil || selected == nil || selected.Direct.ChannelID != 12 {
		t.Fatalf("TPM exhaustion did not demote busy channel: %+v %v", selected, err)
	}
	allowed := []int64{11}
	policy.AccessPolicy = &store.DownstreamAccessPolicy{AllowedUpstreamChannelIDs: &allowed}
	selected, err = selector.selectFromMatch(t.Context(), match, "model", policy, nil, false)
	if err != nil || selected == nil || selected.Direct.ChannelID != 11 {
		t.Fatalf("rate ranking expanded allowed channels or hard-blocked TPM fallback: %+v %v", selected, err)
	}
	allowed = nil
	selected, err = selector.selectFromMatch(t.Context(), match, "model", policy, nil, false)
	if err != nil || selected != nil {
		t.Fatalf("empty allow-list bypass: %+v %v", selected, err)
	}
	for _, original := range match.Channels {
		if original.RateLimitLoad != nil {
			t.Fatal("selection mutated cached candidate load")
		}
	}
	decision := explainSelectionFromMatch(selector, match, "model", nil, DownstreamRoutingPolicy{RequiredUpstreamProtocol: store.DirectProtocolChat}, 60)
	if decision.Candidates[0].ChannelRateLimit == nil || decision.Candidates[0].ChannelRateLimit.ExhaustedBy != "tpm" || !decision.Candidates[0].Eligible {
		t.Fatalf("explanation hid soft TPM pressure or pretended hard refusal: %+v", decision.Candidates[0])
	}
}

func TestChannelRateLimitsLoadChangesWeightedAndLeastBusyRanking(t *testing.T) {
	manager := channellimit.NewManager()
	selector := &ChannelSelector{channelLimits: manager}
	first, second := explanationDirectCandidate(), explanationDirectCandidate()
	secondCopy := *second.Direct
	second.Direct = &secondCopy
	second.Channel.ID, second.Direct.ChannelID = -32, 12
	config := channellimit.Config{MaxConcurrent: 2}
	first.Direct.RateLimit.Config, second.Direct.RateLimit.Config = config, config
	held, err := manager.Acquire(t.Context(), 11, config)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()
	pool := selector.preferChannelCapacity([]RouteChannelCandidate{first, second})
	if len(pool) != 2 || channelCapacityMultiplier(pool[0]) >= channelCapacityMultiplier(pool[1]) {
		t.Fatalf("active channel retained idle score: %+v", pool)
	}
	if selected := SelectLeastBusyCandidate(pool, nil); selected == nil || selected.Direct.ChannelID != 12 {
		t.Fatalf("least busy ignored live channel load: %+v", selected)
	}
	result := CalculateWeightedSelection(pool, nil, defaultRoutingWeights(), nil, 1, nil, 0, WeightedMode, "", nil, 1)
	if len(result.Details) != 2 || result.Details[0].Probability >= result.Details[1].Probability {
		t.Fatalf("weighted route ignored channel capacity: %+v", result.Details)
	}
}
