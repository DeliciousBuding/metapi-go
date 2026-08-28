package routing

import (
	"sync"
	"testing"
)

// TestEnsureSiteRuntimeHealthStateLoadedConcurrent reproduces the unsynchronized
// fast-path read of siteRuntimeHealthLoaded: the lazy-load check runs on every
// SelectChannel hot path while ResetSiteRuntimeHealthState (and the slow-path
// load) write the flag under healthStateMu. Run with -race; before the fix the
// lock-free read races the locked writes.
func TestEnsureSiteRuntimeHealthStateLoadedConcurrent(t *testing.T) {
	ResetSiteRuntimeHealthState()
	t.Cleanup(ResetSiteRuntimeHealthState)

	stop := make(chan struct{})
	var writers sync.WaitGroup
	var readers sync.WaitGroup

	// Writers: reset flips siteRuntimeHealthLoaded back to false under
	// healthStateMu, mirroring the reset path.
	for i := 0; i < 2; i++ {
		writers.Add(1)
		go func() {
			defer writers.Done()
			for {
				select {
				case <-stop:
					return
				default:
					ResetSiteRuntimeHealthState()
				}
			}
		}()
	}

	// Readers: the hot-path lazy-load fast path.
	for i := 0; i < 4; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for j := 0; j < 5000; j++ {
				if err := EnsureSiteRuntimeHealthStateLoaded(); err != nil {
					t.Errorf("EnsureSiteRuntimeHealthStateLoaded: %v", err)
					return
				}
			}
		}()
	}

	readers.Wait()
	close(stop)
	writers.Wait()
}
