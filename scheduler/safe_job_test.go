package scheduler

import (
	"sync/atomic"
	"testing"
)

// TestSafeJobRecoversPanic proves the scheduler-boundary recovery helper:
// a panicking job must not propagate the panic to the caller's goroutine and
// the wrapped function must have run. Before the fix safeJob does not exist
// (compile failure) and every bare-goroutine job site can take the process
// down with a single panic.
func TestSafeJobRecoversPanic(t *testing.T) {
	var ran atomic.Bool
	safeJob("test-job", func() {
		ran.Store(true)
		panic("boom: test panic")
	})
	if !ran.Load() {
		t.Fatal("safeJob did not run the wrapped function")
	}
}
