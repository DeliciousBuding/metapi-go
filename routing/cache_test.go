package routing

import (
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// =============================================================================
// Route cache creation and TTL
// =============================================================================

func TestNewRouteCache_MinimumTTL(t *testing.T) {
	cache := NewRouteCache(0)
	if cache.ttlMs != 100 {
		t.Errorf("expected minimum TTL 100ms, got %d", cache.ttlMs)
	}

	cache = NewRouteCache(50)
	if cache.ttlMs != 100 {
		t.Errorf("expected minimum TTL 100ms for 50ms input, got %d", cache.ttlMs)
	}

	cache = NewRouteCache(5000)
	if cache.ttlMs != 5000 {
		t.Errorf("expected TTL 5000ms, got %d", cache.ttlMs)
	}
}

func TestRouteCache_RoutesGetSet(t *testing.T) {
	cache := NewRouteCache(1000)

	// Initially not fresh
	if cache.IsRoutesFresh() {
		t.Error("expected routes NOT fresh initially")
	}
	if routes := cache.GetRoutes(); routes != nil {
		t.Error("expected nil routes initially")
	}

	// Set routes
	routes := []store.TokenRoute{
		{ID: 1, ModelPattern: "gpt-4", Enabled: true},
		{ID: 2, ModelPattern: "claude-*", Enabled: true},
	}
	cache.SetRoutes(routes)

	// Should be fresh now
	if !cache.IsRoutesFresh() {
		t.Error("expected routes fresh after SetRoutes")
	}
	if got := cache.GetRoutes(); got == nil {
		t.Error("expected non-nil routes")
	} else if len(got) != 2 {
		t.Errorf("expected 2 routes, got %d", len(got))
	}
}

func TestRouteCache_RoutesExpiry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := NewRouteCache(1) // Clamped to the 100ms minimum.
		cache.SetRoutes([]store.TokenRoute{{ID: 1, ModelPattern: "gpt-4", Enabled: true}})
		if !cache.IsRoutesFresh() {
			t.Fatal("expected fresh immediately")
		}

		time.Sleep(99 * time.Millisecond)
		if !cache.IsRoutesFresh() || cache.GetRoutes() == nil {
			t.Fatal("expected routes fresh before the TTL boundary")
		}
		time.Sleep(time.Millisecond)
		if cache.IsRoutesFresh() || cache.GetRoutes() != nil {
			t.Fatal("expected routes expired exactly at the TTL boundary")
		}
	})
}

// =============================================================================
// Route match cache
// =============================================================================

func TestRouteCache_MatchGetSet(t *testing.T) {
	cache := NewRouteCache(5000)

	match := &RouteMatch{
		Route:    store.TokenRoute{ID: 1, ModelPattern: "gpt-4"},
		Channels: []RouteChannelCandidate{},
	}

	// Get non-existent
	if got := cache.GetMatch(1); got != nil {
		t.Error("expected nil for non-existent match")
	}

	// Set and get
	cache.SetMatch(1, match)
	got := cache.GetMatch(1)
	if got == nil {
		t.Fatal("expected non-nil match")
	}
	if got.Route.ID != 1 {
		t.Errorf("expected route ID 1, got %d", got.Route.ID)
	}
}

func TestRouteCache_MatchExpiry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := NewRouteCache(1) // Clamped to the 100ms minimum.
		cache.SetMatch(1, &RouteMatch{Route: store.TokenRoute{ID: 1}})
		if cache.GetMatch(1) == nil {
			t.Fatal("expected non-nil immediately")
		}

		time.Sleep(99 * time.Millisecond)
		if cache.GetMatch(1) == nil {
			t.Fatal("expected match fresh before the TTL boundary")
		}
		time.Sleep(time.Millisecond)
		if cache.GetMatch(1) != nil {
			t.Fatal("expected match expired exactly at the TTL boundary")
		}
	})
}

func TestRouteCache_PatchDoesNotExtendTTL(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := NewRouteCache(100)
		cache.SetMatch(1, &RouteMatch{
			Route:    store.TokenRoute{ID: 1},
			Channels: []RouteChannelCandidate{{Channel: store.RouteChannel{ID: 42}}},
		})
		time.Sleep(75 * time.Millisecond)
		cache.PatchCachedChannel(42, func(channel *store.RouteChannel) {
			channel.Priority = int64Ptr(3)
		})
		if match := cache.GetMatch(1); match == nil || match.Channels[0].Channel.PriorityOrZero() != 3 {
			t.Fatal("expected the patched channel before expiry")
		}
		time.Sleep(25 * time.Millisecond)
		if cache.GetMatch(1) != nil {
			t.Fatal("patching must retain the original cache deadline")
		}
	})
}

// =============================================================================
// Cache patching
// =============================================================================

func TestRouteCache_PatchCachedChannel(t *testing.T) {
	cache := NewRouteCache(5000)

	ch := store.RouteChannel{ID: 42, Priority: int64Ptr(1)}
	match := &RouteMatch{
		Route: store.TokenRoute{ID: 1},
		Channels: []RouteChannelCandidate{
			{Channel: store.RouteChannel{ID: 10, Priority: int64Ptr(5)}},
			{Channel: ch},
			{Channel: store.RouteChannel{ID: 99, Priority: int64Ptr(9)}},
		},
	}
	cache.SetMatch(1, match)

	// Patch channel 42
	var patchedPriority int64
	cache.PatchCachedChannel(42, func(ch *store.RouteChannel) {
		ch.Priority = int64Ptr(100)
		patchedPriority = ch.PriorityOrZero()
	})

	if patchedPriority != 100 {
		t.Errorf("expected patched priority 100, got %d", patchedPriority)
	}

	// Verify patch persisted in cache
	got := cache.GetMatch(1)
	if got == nil {
		t.Fatal("match should still be in cache")
	}
	for _, c := range got.Channels {
		if c.Channel.ID == 42 {
			if c.Channel.PriorityOrZero() != 100 {
				t.Errorf("expected priority 100 in cached match, got %d", c.Channel.PriorityOrZero())
			}
		}
	}

	// Patch non-existent channel (no-op)
	cache.PatchCachedChannel(99999, func(ch *store.RouteChannel) {
		ch.Priority = int64Ptr(999)
	})
}

// =============================================================================
// Cache invalidation
// =============================================================================

func TestRouteCache_InvalidateRouteScopedCache(t *testing.T) {
	cache := NewRouteCache(5000)

	cache.SetMatch(1, &RouteMatch{Route: store.TokenRoute{ID: 1}})
	cache.SetMatch(2, &RouteMatch{Route: store.TokenRoute{ID: 2}})

	// Invalidate route 1
	cache.InvalidateRouteScopedCache(1)

	if got := cache.GetMatch(1); got != nil {
		t.Error("expected nil match for route 1 after invalidation")
	}
	if got := cache.GetMatch(2); got == nil {
		t.Error("expected non-nil match for route 2 (unaffected)")
	}

	// Invalidate with 0 or negative ID (no-op)
	cache.InvalidateRouteScopedCache(0)
	cache.InvalidateRouteScopedCache(-1)
}

func TestRouteCache_InvalidateAll(t *testing.T) {
	cache := NewRouteCache(5000)

	routes := []store.TokenRoute{{ID: 1}}
	cache.SetRoutes(routes)
	cache.SetMatch(1, &RouteMatch{Route: store.TokenRoute{ID: 1}})
	cache.SetMatch(2, &RouteMatch{Route: store.TokenRoute{ID: 2}})

	cache.InvalidateAll()

	if cache.IsRoutesFresh() {
		t.Error("routes should NOT be fresh after InvalidateAll")
	}
	if got := cache.GetRoutes(); got != nil {
		t.Error("routes should be nil after InvalidateAll")
	}
	if got := cache.GetMatch(1); got != nil {
		t.Error("match 1 should be nil after InvalidateAll")
	}
	if got := cache.GetMatch(2); got != nil {
		t.Error("match 2 should be nil after InvalidateAll")
	}
}

// =============================================================================
// Stable-first cache clearing via InvalidateAll
// =============================================================================

func TestInvalidateAll_ClearsStableFirstCaches(t *testing.T) {
	// Pre-populate stable-first caches
	stableFirstLastSelectedSiteByKey["1:gpt-4"] = 100
	stableFirstObservationProgressByKey["1:gpt-4"] = StableFirstObservationProgressState{RequestCount: 5}
	scopedKey := "1:gpt-4:200"
	stableFirstObservationSiteCooldownByKey[scopedKey] = 12345

	cache := NewRouteCache(5000)
	cache.InvalidateAll()

	if len(stableFirstLastSelectedSiteByKey) != 0 {
		t.Errorf("expected empty lastSelectedSite map, got %d", len(stableFirstLastSelectedSiteByKey))
	}
	if len(stableFirstObservationProgressByKey) != 0 {
		t.Errorf("expected empty observation progress map, got %d", len(stableFirstObservationProgressByKey))
	}
	if len(stableFirstObservationSiteCooldownByKey) != 0 {
		t.Errorf("expected empty site cooldown map, got %d", len(stableFirstObservationSiteCooldownByKey))
	}
}

// =============================================================================
// Immutable snapshots under concurrent access
// =============================================================================

func TestRouteCache_ConcurrentReads(t *testing.T) {
	t.Parallel()
	cache := NewRouteCache(int64(time.Hour / time.Millisecond))
	routes := []store.TokenRoute{{ID: 1}}
	cache.SetRoutes(routes)
	match := &RouteMatch{Route: store.TokenRoute{ID: 1}}
	cache.SetMatch(1, match)

	start := make(chan struct{})
	var workers sync.WaitGroup
	for range 8 {
		workers.Go(func() {
			<-start
			for range 64 {
				if !cache.IsRoutesFresh() {
					t.Error("warm route cache unexpectedly stale")
				}
				if got := cache.GetRoutes(); len(got) != 1 || got[0].ID != 1 {
					t.Errorf("cached routes = %+v, want route 1", got)
				}
				if got := cache.GetMatch(1); got != match {
					t.Errorf("cached match = %p, want unchanged snapshot %p", got, match)
				}
			}
		})
	}
	close(start)
	workers.Wait()
}

func TestRouteCache_ConcurrentPatchPreservesSnapshots(t *testing.T) {
	t.Parallel()
	cache := NewRouteCache(int64(time.Hour / time.Millisecond))
	cache.SetMatch(1, &RouteMatch{
		Route: store.TokenRoute{ID: 1},
		Channels: []RouteChannelCandidate{{Channel: store.RouteChannel{
			ID: 42, Priority: int64Ptr(0), Weight: int64Ptr(0),
		}}},
	})
	retained := cache.GetMatch(1)
	patch := func(value int64) {
		cache.PatchCachedChannel(42, func(channel *store.RouteChannel) {
			channel.Priority = int64Ptr(value)
			channel.Weight = int64Ptr(value)
		})
	}
	patch(1)
	if retained.Channels[0].Channel.PriorityOrZero() != 0 || retained.Channels[0].Channel.WeightOrZero() != 0 {
		t.Fatal("patch mutated a previously returned snapshot")
	}

	start := make(chan struct{})
	var workers sync.WaitGroup
	workers.Go(func() {
		<-start
		for value := int64(2); value <= 64; value++ {
			patch(value)
		}
	})
	for range 8 {
		workers.Go(func() {
			<-start
			for range 64 {
				// Read fields after GetMatch releases the cache lock: callers
				// must retain a coherent snapshot while the writer patches it.
				match := cache.GetMatch(1)
				if match == nil || match.Route.ID != 1 || len(match.Channels) != 1 {
					t.Errorf("invalid cached match: %+v", match)
					return
				}
				channel := match.Channels[0].Channel
				if priority, weight := channel.PriorityOrZero(), channel.WeightOrZero(); priority < 1 || priority > 64 || priority != weight {
					t.Errorf("incoherent patched channel: priority=%d weight=%d", priority, weight)
				}
			}
		})
	}
	close(start)
	workers.Wait()
	if got := cache.GetMatch(1).Channels[0].Channel.PriorityOrZero(); got != 64 {
		t.Errorf("final priority = %d, want 64", got)
	}
	if retained.Channels[0].Channel.PriorityOrZero() != 0 || retained.Channels[0].Channel.WeightOrZero() != 0 {
		t.Error("concurrent patch mutated the retained snapshot")
	}
}
