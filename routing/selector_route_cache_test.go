package routing

// Route-list cache wiring tests (perf: routing hot path). The selector must
// serve LoadEnabledRoutes from the TTL route cache, re-query the DB after
// invalidation, stay safe under concurrency, and reflect route CRUD changes
// immediately once the mutation path calls routing.InvalidateCache().

import (
	"context"
	"regexp"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// countingRoutesDB embeds preferredDB and counts LoadEnabledRoutes calls.
// LoadRouteChannels is filtered by the requested route IDs so each route only
// ever sees its own channels.
type countingRoutesDB struct {
	*preferredDB
	loadCalls atomic.Int64
	failures  atomic.Int64
}

func (db *countingRoutesDB) LoadEnabledRoutes(ctx context.Context) ([]store.TokenRoute, error) {
	db.loadCalls.Add(1)
	db.mu.Lock()
	defer db.mu.Unlock()
	out := make([]store.TokenRoute, len(db.routes))
	copy(out, db.routes)
	return out, nil
}

func (db *countingRoutesDB) LoadRouteChannels(ctx context.Context, routeIDs []int64) ([]struct {
	Channel store.RouteChannel
	Account store.Account
	Site    store.Site
	Token   *store.AccountToken
}, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	want := make(map[int64]bool, len(routeIDs))
	for _, id := range routeIDs {
		want[id] = true
	}
	var out []struct {
		Channel store.RouteChannel
		Account store.Account
		Site    store.Site
		Token   *store.AccountToken
	}
	for _, j := range db.joined {
		if want[j.Channel.RouteID] {
			out = append(out, j)
		}
	}
	return out, nil
}

func (db *countingRoutesDB) setRoutes(routes []store.TokenRoute) {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.routes = routes
}

func (db *countingRoutesDB) setJoined(rows ...struct {
	Channel store.RouteChannel
	Account store.Account
	Site    store.Site
	Token   *store.AccountToken
}) {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.joined = rows
}

// countingEligibleJoined builds an eligible channel/account/site row for the
// given route (preferredEligibleJoined hardcodes RouteID 1).
func countingEligibleJoined(routeID, channelID, siteID, accountID int64, model string) struct {
	Channel store.RouteChannel
	Account store.Account
	Site    store.Site
	Token   *store.AccountToken
} {
	m := model
	token := "tok-" + strconv.FormatInt(accountID, 10)
	return struct {
		Channel store.RouteChannel
		Account store.Account
		Site    store.Site
		Token   *store.AccountToken
	}{
		Channel: store.RouteChannel{
			ID:          channelID,
			RouteID:     routeID,
			AccountID:   accountID,
			SourceModel: &m,
			Priority:    int64Ptr(0),
			Weight:      int64Ptr(10),
			Enabled:     true,
		},
		Account: store.Account{
			ID:       accountID,
			SiteID:   siteID,
			Status:   "active",
			APIToken: &token,
			Balance:  ptrFloat(100),
		},
		Site: store.Site{
			ID:     siteID,
			Status: "active",
		},
	}
}

func newCountingSelector(db *countingRoutesDB, cache *RouteCache) *ChannelSelector {
	return NewChannelSelector(db, cache, 3600, defaultRoutingWeights(), nil, 1.0, nil)
}

func resetHealthForCacheTest(t *testing.T) {
	t.Helper()
	ResetSiteRuntimeHealthState()
	siteRuntimeHealthLoaded = true
	t.Cleanup(ResetSiteRuntimeHealthState)
}

func countingTestRoute(id int64, pattern string) store.TokenRoute {
	return store.TokenRoute{
		ID:              id,
		ModelPattern:    pattern,
		RouteMode:       "pattern",
		RoutingStrategy: "weighted",
		Enabled:         true,
	}
}

// TestSelectChannel_CacheHitSkipsDB proves repeated selections serve the
// route list from the cache without re-querying the DB.
func TestSelectChannel_CacheHitSkipsDB(t *testing.T) {
	resetHealthForCacheTest(t)

	model := "gpt-cache-hit"
	db := &countingRoutesDB{preferredDB: &preferredDB{
		routes: []store.TokenRoute{countingTestRoute(1, model)},
	}}
	db.setJoined(countingEligibleJoined(1, 101, 10, 1001, model))
	selector := newCountingSelector(db, NewRouteCache(60_000))

	for i := 0; i < 5; i++ {
		sel, err := selector.SelectChannel(context.Background(), model, EmptyDownstreamRoutingPolicy)
		if err != nil {
			t.Fatalf("SelectChannel #%d error: %v", i, err)
		}
		if sel == nil || sel.Channel.ID != 101 {
			t.Fatalf("SelectChannel #%d = %+v, want channel 101", i, sel)
		}
	}
	if got := db.loadCalls.Load(); got != 1 {
		t.Fatalf("LoadEnabledRoutes called %d times for 5 selections, want 1 (cache hits must not query DB)", got)
	}
}

// TestSelectChannel_InvalidationForcesReload proves routing.InvalidateCache()
// (the entry point used by every admin/service mutation path) drops the cached
// route list so the next selection re-queries the DB and sees new routes.
func TestSelectChannel_InvalidationForcesReload(t *testing.T) {
	resetHealthForCacheTest(t)

	cache := NewRouteCache(60_000)
	globalCacheMu.RLock()
	prev := globalCache
	globalCacheMu.RUnlock()
	SetGlobalCache(cache)
	t.Cleanup(func() { SetGlobalCache(prev) })

	db := &countingRoutesDB{preferredDB: &preferredDB{
		routes: []store.TokenRoute{countingTestRoute(1, "gpt-old")},
	}}
	db.setJoined(countingEligibleJoined(1, 101, 10, 1001, "gpt-old"))
	selector := newCountingSelector(db, cache)

	sel, err := selector.SelectChannel(context.Background(), "gpt-old", EmptyDownstreamRoutingPolicy)
	if err != nil || sel == nil || sel.Channel.ID != 101 {
		t.Fatalf("initial select = %+v, err=%v; want channel 101", sel, err)
	}
	if got := db.loadCalls.Load(); got != 1 {
		t.Fatalf("loadCalls = %d, want 1", got)
	}

	// Admin adds a route; the handler calls routing.InvalidateCache().
	db.setRoutes([]store.TokenRoute{
		countingTestRoute(1, "gpt-old"),
		countingTestRoute(2, "gpt-new"),
	})
	db.setJoined(
		countingEligibleJoined(1, 101, 10, 1001, "gpt-old"),
		countingEligibleJoined(2, 201, 20, 2001, "gpt-new"),
	)
	InvalidateCache()

	sel, err = selector.SelectChannel(context.Background(), "gpt-new", EmptyDownstreamRoutingPolicy)
	if err != nil || sel == nil || sel.Channel.ID != 201 {
		t.Fatalf("post-invalidation select = %+v, err=%v; want channel 201", sel, err)
	}
	if got := db.loadCalls.Load(); got != 2 {
		t.Fatalf("loadCalls = %d after invalidation, want 2", got)
	}
}

// TestSelectChannel_TTLExpiryRefetches proves a stale cache falls back to the
// DB (TTL is the staleness backstop even without explicit invalidation).
func TestSelectChannel_TTLExpiryRefetches(t *testing.T) {
	resetHealthForCacheTest(t)

	model := "gpt-ttl-expiry"
	db := &countingRoutesDB{preferredDB: &preferredDB{
		routes: []store.TokenRoute{countingTestRoute(1, model)},
	}}
	db.setJoined(countingEligibleJoined(1, 101, 10, 1001, model))
	selector := newCountingSelector(db, NewRouteCache(100)) // minimum TTL

	if _, err := selector.SelectChannel(context.Background(), model, EmptyDownstreamRoutingPolicy); err != nil {
		t.Fatal(err)
	}
	if got := db.loadCalls.Load(); got != 1 {
		t.Fatalf("loadCalls = %d, want 1", got)
	}

	time.Sleep(150 * time.Millisecond)

	sel, err := selector.SelectChannel(context.Background(), model, EmptyDownstreamRoutingPolicy)
	if err != nil || sel == nil {
		t.Fatalf("post-expiry select = %+v, err=%v", sel, err)
	}
	if got := db.loadCalls.Load(); got != 2 {
		t.Fatalf("loadCalls = %d after TTL expiry, want 2", got)
	}
}

// TestSelectChannel_ConcurrentSafe hammers the selector from many goroutines:
// warm-cache reads must not re-query the DB, and mixed invalidate/read traffic
// must stay race- and panic-free. Run with -race.
func TestSelectChannel_ConcurrentSafe(t *testing.T) {
	resetHealthForCacheTest(t)

	model := "gpt-concurrent"
	db := &countingRoutesDB{preferredDB: &preferredDB{
		routes: []store.TokenRoute{countingTestRoute(1, model)},
	}}
	db.setJoined(countingEligibleJoined(1, 101, 10, 1001, model))
	cache := NewRouteCache(60_000)
	selector := newCountingSelector(db, cache)

	// Warm the cache so the concurrent phase is pure cache-hit.
	if sel, err := selector.SelectChannel(context.Background(), model, EmptyDownstreamRoutingPolicy); err != nil || sel == nil {
		t.Fatalf("warm-up select = %+v, err=%v", sel, err)
	}
	if got := db.loadCalls.Load(); got != 1 {
		t.Fatalf("warm-up loadCalls = %d, want 1", got)
	}

	const goroutines = 16
	const iters = 25
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				sel, err := selector.SelectChannel(context.Background(), model, EmptyDownstreamRoutingPolicy)
				if err != nil || sel == nil || sel.Channel.ID != 101 {
					db.failures.Add(1)
				}
			}
		}()
	}
	wg.Wait()
	if f := db.failures.Load(); f != 0 {
		t.Fatalf("%d concurrent selections failed", f)
	}
	if got := db.loadCalls.Load(); got != 1 {
		t.Fatalf("loadCalls = %d during %d concurrent cache-hit selections, want 1", got, goroutines*iters)
	}

	// Mixed invalidation + reads: must stay race-free and keep serving.
	stop := make(chan struct{})
	var writer sync.WaitGroup
	writer.Add(1)
	go func() {
		defer writer.Done()
		for {
			select {
			case <-stop:
				return
			default:
				cache.InvalidateAll()
			}
		}
	}()
	var readers sync.WaitGroup
	for g := 0; g < 8; g++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			deadline := time.Now().Add(50 * time.Millisecond)
			for time.Now().Before(deadline) {
				sel, err := selector.SelectChannel(context.Background(), model, EmptyDownstreamRoutingPolicy)
				if err != nil || sel == nil {
					db.failures.Add(1)
				}
			}
		}()
	}
	readers.Wait()
	close(stop)
	writer.Wait()
	if f := db.failures.Load(); f != 0 {
		t.Fatalf("%d selections failed during concurrent invalidation", f)
	}
}

// TestSelectChannel_ReflectsRouteCRUDAfterInvalidation is the behavior
// regression: route create/update/delete take effect on the very next
// selection once the mutation path invalidates (as every CRUD handler does).
func TestSelectChannel_ReflectsRouteCRUDAfterInvalidation(t *testing.T) {
	resetHealthForCacheTest(t)

	cache := NewRouteCache(60_000)
	globalCacheMu.RLock()
	prev := globalCache
	globalCacheMu.RUnlock()
	SetGlobalCache(cache)
	t.Cleanup(func() { SetGlobalCache(prev) })

	db := &countingRoutesDB{preferredDB: &preferredDB{}}
	selector := newCountingSelector(db, cache)
	ctx := context.Background()

	// Create: route appears -> immediately selectable.
	db.setRoutes([]store.TokenRoute{countingTestRoute(1, "gpt-crud")})
	db.setJoined(countingEligibleJoined(1, 101, 10, 1001, "gpt-crud"))
	InvalidateCache()
	sel, err := selector.SelectChannel(ctx, "gpt-crud", EmptyDownstreamRoutingPolicy)
	if err != nil || sel == nil || sel.Channel.ID != 101 {
		t.Fatalf("after create: select = %+v, err=%v; want channel 101", sel, err)
	}

	// Update: model_pattern changes (and the rebuild refreshes the channel's
	// source model) -> old name stops routing, new name routes.
	db.setRoutes([]store.TokenRoute{countingTestRoute(1, "gpt-crud-v2")})
	db.setJoined(countingEligibleJoined(1, 101, 10, 1001, "gpt-crud-v2"))
	InvalidateCache()
	if sel, err := selector.SelectChannel(ctx, "gpt-crud", EmptyDownstreamRoutingPolicy); err != nil || sel != nil {
		t.Fatalf("after update: stale name select = %+v, err=%v; want nil", sel, err)
	}
	sel, err = selector.SelectChannel(ctx, "gpt-crud-v2", EmptyDownstreamRoutingPolicy)
	if err != nil || sel == nil || sel.Channel.ID != 101 {
		t.Fatalf("after update: new name select = %+v, err=%v; want channel 101", sel, err)
	}

	// Delete: route removed -> model no longer routes.
	db.setRoutes(nil)
	db.setJoined()
	InvalidateCache()
	if sel, err := selector.SelectChannel(ctx, "gpt-crud-v2", EmptyDownstreamRoutingPolicy); err != nil || sel != nil {
		t.Fatalf("after delete: select = %+v, err=%v; want nil", sel, err)
	}

	// Each invalidation forced exactly one reload on the next selection.
	if got := db.loadCalls.Load(); got != 3 {
		t.Fatalf("loadCalls = %d across create/update/delete, want 3", got)
	}
}

// TestParseRegexModelPattern_CachedCompile proves re: patterns compile once
// per body and reuse the shared instance (including across prefix casings),
// and that invalid bodies cache their failure instead of recompiling.
func TestParseRegexModelPattern_CachedCompile(t *testing.T) {
	re1 := ParseRegexModelPattern("re:^claude-3-[0-9]+$")
	re2 := ParseRegexModelPattern("re:^claude-3-[0-9]+$")
	if re1 == nil {
		t.Fatal("expected compiled regexp")
	}
	if re1 != re2 {
		t.Fatal("expected the same cached *regexp.Regexp instance for identical bodies")
	}
	if !re1.MatchString("claude-3-20240101") {
		t.Fatal("cached regexp must still match")
	}

	// Prefix casing differs, body identical -> same cache entry.
	if re3 := ParseRegexModelPattern("RE:^claude-3-[0-9]+$"); re3 != re1 {
		t.Fatal("expected body-level caching regardless of re: prefix casing")
	}

	// Invalid body -> nil, stable across calls.
	if ParseRegexModelPattern("re:[unclosed") != nil {
		t.Fatal("expected nil for invalid body")
	}
	if ParseRegexModelPattern("re:[unclosed") != nil {
		t.Fatal("expected cached nil for invalid body")
	}
}

// BenchmarkMatchesModelPattern_Regex measures the cached hot path.
func BenchmarkMatchesModelPattern_Regex(b *testing.B) {
	const pattern = "re:^claude-3-[a-z0-9-]+$"
	const model = "claude-3-5-sonnet-20241022"
	if !MatchesModelPattern(model, pattern) {
		b.Fatal("pattern must match")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		MatchesModelPattern(model, pattern)
	}
}

// BenchmarkMatchesModelPattern_RegexCompilePerCall is the pre-fix reference:
// MatchesModelPattern recompiled the pattern on every call.
func BenchmarkMatchesModelPattern_RegexCompilePerCall(b *testing.B) {
	const model = "claude-3-5-sonnet-20241022"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		re, err := regexp.Compile(`^claude-3-[a-z0-9-]+$`)
		if err != nil {
			b.Fatal(err)
		}
		_ = re.MatchString(model)
	}
}

// BenchmarkFindRoute_RouteListCached measures the hot findRoute path with a
// warm route cache over 100 routes (99 re: patterns + one exact match).
func BenchmarkFindRoute_RouteListCached(b *testing.B) {
	routes := make([]store.TokenRoute, 0, 100)
	for i := 0; i < 100; i++ {
		if i == 50 {
			routes = append(routes, countingTestRoute(int64(i+1), "gpt-bench"))
			continue
		}
		routes = append(routes, countingTestRoute(int64(i+1), "re:^bench-model-"+strconv.Itoa(i)+"$"))
	}
	db := &countingRoutesDB{preferredDB: &preferredDB{routes: routes}}
	db.setJoined(countingEligibleJoined(51, 101, 10, 1001, "gpt-bench"))
	selector := newCountingSelector(db, NewRouteCache(60_000))
	ctx := context.Background()

	if _, err := selector.findRoute(ctx, "gpt-bench", EmptyDownstreamRoutingPolicy); err != nil {
		b.Fatalf("warm-up findRoute: %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = selector.findRoute(ctx, "gpt-bench", EmptyDownstreamRoutingPolicy)
	}
}

// BenchmarkFindRoute_RouteListCold is the legacy-equivalent path: the route
// list is reloaded every call (cache invalidated each iteration).
func BenchmarkFindRoute_RouteListCold(b *testing.B) {
	routes := make([]store.TokenRoute, 0, 100)
	for i := 0; i < 100; i++ {
		if i == 50 {
			routes = append(routes, countingTestRoute(int64(i+1), "gpt-bench"))
			continue
		}
		routes = append(routes, countingTestRoute(int64(i+1), "re:^bench-model-"+strconv.Itoa(i)+"$"))
	}
	db := &countingRoutesDB{preferredDB: &preferredDB{routes: routes}}
	db.setJoined(countingEligibleJoined(51, 101, 10, 1001, "gpt-bench"))
	selector := newCountingSelector(db, NewRouteCache(60_000))
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		selector.cache.InvalidateAll()
		_, _ = selector.findRoute(ctx, "gpt-bench", EmptyDownstreamRoutingPolicy)
	}
}
