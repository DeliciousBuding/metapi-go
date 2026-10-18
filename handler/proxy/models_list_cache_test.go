package proxyhandler

import (
	"context"
	"fmt"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/routing"
)

// policyFilteringRouter implements AvailableModelsSource and ExplainSelection.
// It maps each catalog model to a route ID, so a downstream policy granting
// only some routes sees only the models owned by those routes.
type policyFilteringRouter struct {
	modelsTestRouter
	routeForModel map[string]int64
	explainCalls  atomic.Int64
}

func (r *policyFilteringRouter) ExplainSelection(_ context.Context, model string, _ []int64, policy routing.DownstreamRoutingPolicy) (routing.RouteDecisionExplanation, error) {
	r.explainCalls.Add(1)
	eligible := false
	if routeID, ok := r.routeForModel[model]; ok {
		for _, allowed := range policy.AllowedRouteIDs {
			if allowed == routeID {
				eligible = true
				break
			}
		}
	}
	return routing.RouteDecisionExplanation{
		RequestedModel: model,
		Matched:        eligible,
		Candidates:     []routing.RouteDecisionCandidate{{Eligible: eligible}},
	}, nil
}

var _ routeSelectionExplainer = (*policyFilteringRouter)(nil)

func newPolicyFilteringRouter() *policyFilteringRouter {
	return &policyFilteringRouter{
		modelsTestRouter: modelsTestRouter{models: []string{"model-a", "model-b"}},
		routeForModel:    map[string]int64{"model-a": 1, "model-b": 2},
	}
}

func listModelIDs(t *testing.T, policy auth.DownstreamRoutingPolicy) []string {
	t.Helper()
	req := httptest.NewRequest("GET", "/v1/models", nil)
	req = auth.ProxyAuthFromRequest(req, &auth.ProxyAuthContext{
		Token:  "managed-key",
		Source: "managed",
		Policy: policy,
	})
	rec := httptest.NewRecorder()
	HandleModels(rec, req)
	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	m := unmarshalResponse(t, rec)
	data, _ := m["data"].([]any)
	ids := make([]string, 0, len(data))
	for _, raw := range data {
		ids = append(ids, raw.(map[string]any)["id"].(string))
	}
	return ids
}

func assertModelIDs(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("models = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("models = %v, want %v", got, want)
		}
	}
}

// Two managed keys with different route grants must never share a cached view,
// even when their requests alternate back-to-back.
func TestHandleModels_AlternatingPoliciesDoNotShareCache(t *testing.T) {
	router := newPolicyFilteringRouter()
	withModelsRouter(t, router)

	policyA := auth.DownstreamRoutingPolicy{AllowedRouteIDs: []int64{1}}
	policyB := auth.DownstreamRoutingPolicy{AllowedRouteIDs: []int64{2}}

	assertModelIDs(t, listModelIDs(t, policyA), []string{"model-a"})
	assertModelIDs(t, listModelIDs(t, policyB), []string{"model-b"})
	assertModelIDs(t, listModelIDs(t, policyA), []string{"model-a"})
	assertModelIDs(t, listModelIDs(t, policyB), []string{"model-b"})

	// Each policy was evaluated once: two catalog models per cold miss.
	if got := router.explainCalls.Load(); got != 4 {
		t.Fatalf("ExplainSelection calls = %d, want 4 (two cold misses x 2 models)", got)
	}
}

func TestHandleModels_SamePolicyHitsCache(t *testing.T) {
	router := newPolicyFilteringRouter()
	withModelsRouter(t, router)

	policy := auth.DownstreamRoutingPolicy{AllowedRouteIDs: []int64{1}}
	assertModelIDs(t, listModelIDs(t, policy), []string{"model-a"})
	assertModelIDs(t, listModelIDs(t, policy), []string{"model-a"})
	assertModelIDs(t, listModelIDs(t, policy), []string{"model-a"})

	if got := router.explainCalls.Load(); got != 2 {
		t.Fatalf("ExplainSelection calls = %d, want 2 (one cold miss over 2 models)", got)
	}
}

func TestHandleModels_PolicyChangeInvalidatesCachedList(t *testing.T) {
	router := newPolicyFilteringRouter()
	withModelsRouter(t, router)

	assertModelIDs(t, listModelIDs(t, auth.DownstreamRoutingPolicy{AllowedRouteIDs: []int64{1}}), []string{"model-a"})
	assertModelIDs(t, listModelIDs(t, auth.DownstreamRoutingPolicy{AllowedRouteIDs: []int64{1, 2}}), []string{"model-a", "model-b"})
	// A new grant now resolves the previously hidden model.
	assertModelIDs(t, listModelIDs(t, auth.DownstreamRoutingPolicy{AllowedRouteIDs: []int64{2}}), []string{"model-b"})
}

// Catalog content changes produce a new cache key, so they apply immediately
// rather than waiting for the TTL.
func TestHandleModels_CatalogChangeInvalidatesCachedList(t *testing.T) {
	router := newPolicyFilteringRouter()
	withModelsRouter(t, router)

	policy := auth.DownstreamRoutingPolicy{AllowedRouteIDs: []int64{1}}
	assertModelIDs(t, listModelIDs(t, policy), []string{"model-a"})

	router.models = []string{"model-a", "model-b", "model-c"}
	router.routeForModel["model-c"] = 1
	assertModelIDs(t, listModelIDs(t, policy), []string{"model-a", "model-c"})
}

func TestModelsListCache_TTLExpiryRecomputes(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	cache := newModelsListCache(time.Minute, 8)
	cache.now = func() time.Time { return now }
	computeCalls := 0
	compute := func() []string {
		computeCalls++
		return []string{fmt.Sprintf("model-%d", computeCalls)}
	}

	assertModelIDs(t, cache.getOrCompute("key", compute), []string{"model-1"})
	now = now.Add(59 * time.Second)
	assertModelIDs(t, cache.getOrCompute("key", compute), []string{"model-1"})
	if computeCalls != 1 {
		t.Fatalf("compute calls = %d, want 1 before TTL expiry", computeCalls)
	}

	now = now.Add(time.Second) // exactly at expiry
	assertModelIDs(t, cache.getOrCompute("key", compute), []string{"model-2"})
	if computeCalls != 2 {
		t.Fatalf("compute calls = %d, want 2 after TTL expiry", computeCalls)
	}
}

func TestModelsListCache_CapacityIsBounded(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	cache := newModelsListCache(time.Minute, 4)
	cache.now = func() time.Time { return now }
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("key-%d", i)
		_ = cache.getOrCompute(key, func() []string { return []string{key} })
	}
	if got := cache.len(); got > 4 {
		t.Fatalf("cache entries = %d, want <= 4", got)
	}

	// Once everything is expired, a fresh insert drops the expired entries
	// instead of retaining dead views.
	now = now.Add(2 * time.Minute)
	_ = cache.getOrCompute("fresh", func() []string { return []string{"fresh"} })
	if got := cache.len(); got != 1 {
		t.Fatalf("cache entries after expiry = %d, want 1", got)
	}
}

func TestModelsListCache_ConcurrentMissesShareOneCompute(t *testing.T) {
	cache := newModelsListCache(time.Minute, 8)
	var computeCalls atomic.Int64
	entered := make(chan struct{})
	release := make(chan struct{})
	var enteredOnce sync.Once
	compute := func() []string {
		computeCalls.Add(1)
		enteredOnce.Do(func() { close(entered) })
		<-release
		return []string{"model-a"}
	}

	const callers = 16
	var wg sync.WaitGroup
	results := make([][]string, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = cache.getOrCompute("same-key", compute)
		}(i)
	}
	<-entered
	close(release)
	wg.Wait()

	if got := computeCalls.Load(); got != 1 {
		t.Fatalf("compute calls = %d, want 1 (singleflight)", got)
	}
	for _, got := range results {
		assertModelIDs(t, got, []string{"model-a"})
	}
}

func TestModelsListCache_ReturnsDefensiveCopy(t *testing.T) {
	cache := newModelsListCache(time.Minute, 8)
	first := cache.getOrCompute("key", func() []string { return []string{"model-a"} })
	first[0] = "mutated"
	second := cache.getOrCompute("key", func() []string {
		t.Fatal("cache hit should not recompute")
		return nil
	})
	assertModelIDs(t, second, []string{"model-a"})
}
