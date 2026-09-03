package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// In-flight flag race tests (run under -race).
//
// Observed defect: RunProjectionPass read projectionInFlight without holding
// the mutex while runPass writes it under the mutex. The unsynchronized
// read/write pair is a real data race, not a theoretical one.

// TestRunProjectionPassInFlightReadIsSynchronized races the external entry
// point against the ticker path. Before the fix the race detector reports
// the lock-free projectionInFlight read vs the under-mutex write.
func TestRunProjectionPassInFlightReadIsSynchronized(t *testing.T) {
	// Keep store.GetDB() nil so runPass stays a fast no-op after the
	// in-flight bookkeeping (no lease/DB machinery in the loop).
	store.OverrideDB(nil)
	t.Cleanup(func() { store.OverrideDB(nil) })

	s := NewUsageAggregationScheduler(testConfig())
	done := make(chan struct{})
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
				s.runPass(context.Background())
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
				s.RunProjectionPass()
			}
		}
	}()

	time.Sleep(200 * time.Millisecond)
	close(done)
	wg.Wait()
}
