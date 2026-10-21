package routing

import (
	"context"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

func TestUserRouteBoundaryCannotBeWidenedOrPoisonCache(t *testing.T) {
	resetHealthForCacheTest(t)
	db := &countingRoutesDB{preferredDB: &preferredDB{routes: []store.TokenRoute{countingTestRoute(1, "model-a"), countingTestRoute(2, "model-b")}}}
	db.setJoined(countingEligibleJoined(1, 101, 10, 1001, "model-a"), countingEligibleJoined(2, 102, 10, 1001, "model-b"))
	selector := newCountingSelector(db, NewRouteCache(60_000))
	for _, test := range []struct {
		routes  []int64
		model   string
		allowed bool
	}{
		{[]int64{1}, "model-a", true}, {[]int64{1}, "model-b", false},
		{[]int64{}, "model-a", false}, {[]int64{2}, "model-b", true}, {[]int64{1}, "model-a", true},
	} {
		policy := DownstreamRoutingPolicy{UserRouteIDs: &test.routes, SupportedModels: []string{"*"}, AllowedRouteIDs: []int64{1, 2}}
		selected, err := selector.SelectChannel(context.Background(), test.model, policy)
		if (selected != nil && err == nil) != test.allowed {
			t.Fatalf("model=%s routes=%v selected=%+v error=%v", test.model, test.routes, selected, err)
		}
	}
	if len(selector.cache.GetRoutes()) != 2 {
		t.Fatal("user filter mutated global route cache")
	}
	if selected, err := selector.SelectChannel(context.Background(), "model-b", EmptyDownstreamRoutingPolicy); err != nil || selected == nil {
		t.Fatalf("legacy Key changed: %+v %v", selected, err)
	}
	tr := &TokenRouter{db: db, selector: selector}
	empty := []int64{}
	if match, err := tr.findRouteByID(context.Background(), 1, DownstreamRoutingPolicy{UserRouteIDs: &empty}); err != nil || match != nil {
		t.Fatalf("route diagnostic widened empty scope: %+v %v", match, err)
	}
}
