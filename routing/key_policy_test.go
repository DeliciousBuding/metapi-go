package routing

import (
	"fmt"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/internal/channellimit"
	"github.com/deliciousbuding/metapi-go/internal/routecondition"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestKeyRoutingOverridePrecedenceAndAffinityBounds(t *testing.T) {
	route := store.TokenRoute{ID: 1, RoutingStrategy: "least_busy", TraceStickyMode: "prefer_previous_channel"}
	for _, fallback := range []string{"", "default", "system_default"} {
		policy := DownstreamRoutingPolicy{AccessPolicy: &store.DownstreamAccessPolicy{RoutingStrategy: fallback, TraceStickyMode: fallback}, TraceIdentity: "key-a-session"}
		if effectiveStrategy(route, policy) != StrategyLeastBusy || affinityScope(route, "model", policy) == "" {
			t.Fatal("explicit default did not inherit route policy")
		}
	}
	policy := DownstreamRoutingPolicy{AccessPolicy: &store.DownstreamAccessPolicy{RoutingStrategy: "round_robin", TraceStickyMode: "disabled"}, TraceIdentity: "key-a"}
	if effectiveStrategy(route, policy) != StrategyRoundRobin || affinityScope(route, "model", policy) != "" {
		t.Fatal("Key override did not replace model defaults")
	}
	a := newTraceAffinity()
	now := time.Now()
	for i := range affinityCapacity + 5 {
		a.remember(fmt.Sprint(i), affinityEntry{channelID: int64(i + 1)}, now.Add(time.Duration(i)*time.Millisecond))
	}
	if len(a.entries) != affinityCapacity || a.get("0", now).channelID != 0 {
		t.Fatal("trace affinity cache was not bounded")
	}
	if a.get("5", now.Add(affinityTTL+time.Second)).channelID != 0 {
		t.Fatal("trace affinity outlived TTL")
	}
}

func TestTraceAffinityCannotBypassConditionsOrChannelLimits(t *testing.T) {
	resetHealthForCacheTest(t)
	selector := newPreferredSelector(&preferredDB{})
	manager := channellimit.NewManager()
	selector.channelLimits = manager
	first, second := explanationDirectCandidate(), explanationDirectCandidate()
	copySecond := *second.Direct
	second.Direct = &copySecond
	second.Channel.ID, second.Direct.ItemID, second.Direct.ChannelID = -32, 32, 12
	condition, err := routecondition.Decode([]byte(`{"conditions":[{"field":"request_header.X-Tier","operator":"eq","value":"gold"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	first.Direct.AssociationConditions = store.DirectAssociationConditions{{Priority: 0, ProtocolOrder: store.DirectProtocolOrder{store.DirectProtocolChat}, Condition: condition}}
	first.Direct.RateLimit.RPM = 1
	match := &RouteMatch{Route: store.TokenRoute{ID: 1, ModelPattern: "model", RoutingStrategy: "round_robin", TraceStickyMode: "prefer_previous_channel"}, Channels: []RouteChannelCandidate{first, second}}
	policy := DownstreamRoutingPolicy{RequiredUpstreamProtocol: store.DirectProtocolChat, TraceIdentity: "same-key-trace", AssociationRequest: &routecondition.Request{Headers: map[string]string{"X-Tier": "gold"}}}
	scope := affinityScope(match.Route, "model", policy)
	selector.affinity.remember(scope, affinityEntry{channelID: -31, directID: 11}, time.Now())
	selected, err := selector.selectFromMatch(t.Context(), match, "model", policy, nil, false)
	if err != nil || selected == nil || selected.Direct.ChannelID != 11 || !selected.AffinityPreferred {
		t.Fatalf("eligible preference missing: %+v %v", selected, err)
	}
	policy.AssociationRequest = &routecondition.Request{Headers: map[string]string{"X-Tier": "silver"}}
	selected, err = selector.selectFromMatch(t.Context(), match, "model", policy, nil, false)
	if err != nil || selected == nil || selected.Direct.ChannelID != 12 {
		t.Fatalf("sticky bypassed condition: %+v %v", selected, err)
	}
	policy.AssociationRequest = &routecondition.Request{Headers: map[string]string{"X-Tier": "gold"}}
	if err := manager.TryRequest(t.Context(), 11, channellimit.Config{RPM: 1}); err != nil {
		t.Fatal(err)
	}
	selected, err = selector.selectFromMatch(t.Context(), match, "model", policy, nil, false)
	if err != nil || selected == nil || selected.Direct.ChannelID != 12 {
		t.Fatalf("sticky bypassed rate exhaustion: %+v %v", selected, err)
	}
}

func TestTraceAffinityScopeIncludesMappedModelAndKey(t *testing.T) {
	route := store.TokenRoute{ID: 1, TraceStickyMode: "prefer_previous_channel"}
	policy := DownstreamRoutingPolicy{TraceIdentity: "namespace-a", AccessPolicy: &store.DownstreamAccessPolicy{}}
	original := affinityScope(route, "model", policy)
	policy.TraceIdentity = "namespace-b"
	if affinityScope(route, "model", policy) == original {
		t.Fatal("cross-Key trace scope collision")
	}
	policy.TraceIdentity = "namespace-a"
	policy.AccessPolicy.ModelMappings = []store.DownstreamModelMapping{{From: "model", To: "other"}}
	if affinityScope(route, "model", policy) == original {
		t.Fatal("changed model mapping reused prior affinity")
	}
	if route.TraceStickyMode = "disabled"; affinityScope(route, "model", policy) != "" {
		t.Fatal("disabled route retained affinity")
	}
}

func TestTraceAffinityRouteMappingChangeDoesNotReusePreference(t *testing.T) {
	resetHealthForCacheTest(t)
	db := &preferredDB{
		routes: []store.TokenRoute{{ID: 1, ModelPattern: "public", RoutingStrategy: "round_robin", TraceStickyMode: "prefer_previous_channel", Enabled: true}},
		joined: []struct {
			Channel store.RouteChannel
			Account store.Account
			Site    store.Site
			Token   *store.AccountToken
		}{
			preferredEligibleJoined(11, 1, 1, "public"),
			preferredEligibleJoined(12, 2, 2, "public"),
		},
	}
	mappingA, mappingB := `{"public":"provider-a"}`, `{"public":"provider-b"}`
	db.routes[0].ModelMapping = &mappingA
	selector := newPreferredSelector(db)
	policy := DownstreamRoutingPolicy{TraceIdentity: "key-session"}
	selected, err := selector.SelectChannel(t.Context(), "public", policy)
	if err != nil || selected == nil {
		t.Fatalf("initial selection: %+v %v", selected, err)
	}
	selector.affinity.remember(selected.AffinityScope, affinityEntry{channelID: selected.Channel.ID}, time.Now())
	future := time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)
	for i := range db.joined {
		if db.joined[i].Channel.ID == selected.Channel.ID {
			db.joined[i].Channel.LastSelectedAt = &future
		}
	}
	db.routes[0].ModelMapping = &mappingB
	selector.cache.InvalidateAll()
	changed, err := selector.SelectChannel(t.Context(), "public", policy)
	if err != nil || changed == nil || changed.Channel.ID == selected.Channel.ID || changed.ActualModel != "provider-b" || changed.AffinityScope == selected.AffinityScope {
		t.Fatalf("route mapping reused old preference: before=%+v after=%+v err=%v", selected, changed, err)
	}
}

func TestKeyStrategiesDoNotMutateAnotherKeysRoute(t *testing.T) {
	resetHealthForCacheTest(t)
	selector := newPreferredSelector(&preferredDB{})
	firstRow, secondRow := preferredEligibleJoined(11, 1, 1, "model"), preferredEligibleJoined(12, 2, 2, "model")
	expensive, cheap := 10.0, 1.0
	firstRow.Account.UnitCost, secondRow.Account.UnitCost = &expensive, &cheap
	first := RouteChannelCandidate{Channel: firstRow.Channel, Account: firstRow.Account, Site: firstRow.Site}
	second := RouteChannelCandidate{Channel: secondRow.Channel, Account: secondRow.Account, Site: secondRow.Site}
	match := &RouteMatch{Route: store.TokenRoute{ID: 1, ModelPattern: "model", RoutingStrategy: "lowest_cost"}, Channels: []RouteChannelCandidate{first, second}}
	override := DownstreamRoutingPolicy{AccessPolicy: &store.DownstreamAccessPolicy{RoutingStrategy: "round_robin"}}
	one, err := selector.selectFromMatch(t.Context(), match, "model", override, nil, false)
	if err != nil || one == nil || one.Channel.ID != 11 {
		t.Fatalf("Key round-robin did not override cost: %+v %v", one, err)
	}
	two, err := selector.selectFromMatch(t.Context(), match, "model", DownstreamRoutingPolicy{}, nil, false)
	if err != nil || two == nil || two.Channel.ID != 12 || match.Route.RoutingStrategy != "lowest_cost" {
		t.Fatalf("Key strategy contaminated cached route: %+v %v", two, err)
	}
}
