package routing

import (
	"fmt"
	"strings"
	"testing"
)

func TestSiteModelHealthStateMapIsBounded(t *testing.T) {
	healthStateMu.Lock()
	previous := siteModelRuntimeHealthStates
	siteModelRuntimeHealthStates = make(map[int64]map[string]*SiteRuntimeHealthState)
	healthStateMu.Unlock()
	t.Cleanup(func() {
		healthStateMu.Lock()
		siteModelRuntimeHealthStates = previous
		healthStateMu.Unlock()
	})

	healthStateMu.Lock()
	for i := 0; i < maxSiteModelHealthStates+200; i++ {
		if state := getOrCreateSiteModelRuntimeHealthState(1, fmt.Sprintf("model-%d", i)); state == nil {
			healthStateMu.Unlock()
			t.Fatalf("state for model-%d is nil", i)
		}
	}
	entries := len(siteModelRuntimeHealthStates[1])
	healthStateMu.Unlock()

	if entries > maxSiteModelHealthStates {
		t.Fatalf("per-site model states = %d, want at most %d", entries, maxSiteModelHealthStates)
	}

	// A client-supplied model name must not become an unbounded map key.
	long := strings.Repeat("m", 8<<10)
	healthStateMu.Lock()
	if state := getOrCreateSiteModelRuntimeHealthState(2, long); state == nil {
		healthStateMu.Unlock()
		t.Fatal("long model state is nil")
	}
	keys := make([]string, 0, len(siteModelRuntimeHealthStates[2]))
	for key := range siteModelRuntimeHealthStates[2] {
		keys = append(keys, key)
	}
	healthStateMu.Unlock()

	for _, key := range keys {
		if len(key) > 256 {
			t.Fatalf("health map key = %d bytes, want a bounded key", len(key))
		}
	}
}
