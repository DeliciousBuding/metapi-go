package app

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

// TestReadinessProbeCacheTTL covers audit L15: /ready must not ping the
// database on every unauthenticated request.
func TestReadinessProbeCacheTTL(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var cache readinessProbeCache
		key := &struct{}{}
		calls := 0
		probe := func() error { calls++; return nil }

		if err := cache.probe(key, time.Second, probe); err != nil {
			t.Fatalf("first probe: %v", err)
		}
		if err := cache.probe(key, time.Second, probe); err != nil {
			t.Fatalf("cached probe: %v", err)
		}
		if calls != 1 {
			t.Fatalf("probe calls within TTL = %d, want 1", calls)
		}

		time.Sleep(time.Second)
		if err := cache.probe(key, time.Second, probe); err != nil {
			t.Fatalf("refreshed probe: %v", err)
		}
		if calls != 2 {
			t.Fatalf("probe calls after TTL = %d, want 2", calls)
		}
	})
}

// TestReadinessProbeCacheCachesFailure keeps readiness at 503 until the TTL
// expires; a failing probe must not turn into a per-request storm.
func TestReadinessProbeCacheCachesFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var cache readinessProbeCache
		key := &struct{}{}
		failure := errors.New("database unavailable")
		calls := 0
		probe := func() error { calls++; return failure }

		if err := cache.probe(key, time.Second, probe); !errors.Is(err, failure) {
			t.Fatalf("first probe error = %v, want failure", err)
		}
		if err := cache.probe(key, time.Second, probe); !errors.Is(err, failure) {
			t.Fatalf("cached probe error = %v, want failure", err)
		}
		if calls != 1 {
			t.Fatalf("failing probe calls within TTL = %d, want 1", calls)
		}
	})
}

func TestReadinessProbeCacheKeyIsolation(t *testing.T) {
	var cache readinessProbeCache
	keyA, keyB := &struct{ n int }{1}, &struct{ n int }{2}
	calls := 0
	probe := func() error { calls++; return nil }

	_ = cache.probe(keyA, time.Minute, probe)
	_ = cache.probe(keyB, time.Minute, probe)
	if calls != 2 {
		t.Fatalf("probe calls across handles = %d, want 2", calls)
	}
}

// TestReadinessProbeCacheMergesConcurrentCallers proves concurrent /ready
// requests share one database round trip.
func TestReadinessProbeCacheMergesConcurrentCallers(t *testing.T) {
	var cache readinessProbeCache
	var calls atomic.Int32
	entered := make(chan struct{})
	release := make(chan struct{})
	probe := func() error {
		calls.Add(1)
		close(entered)
		<-release
		return nil
	}

	const callers = 8
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(callers)
	for i := 0; i < callers; i++ {
		go func() {
			defer wg.Done()
			<-start
			_ = cache.probe(nil, time.Minute, probe)
		}()
	}
	close(start)
	<-entered
	close(release)
	wg.Wait()

	if got := calls.Load(); got != 1 {
		t.Fatalf("concurrent probes executed %d database calls, want 1", got)
	}
}
