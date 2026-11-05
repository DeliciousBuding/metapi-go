package channellimit

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func waitFor(t *testing.T, manager *Manager, id int64, config Config, predicate func(Snapshot) bool) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		if predicate(manager.Snapshot(id, config)) {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for channel state: %+v", manager.Snapshot(id, config))
		case <-tick.C:
		}
	}
}

func TestChannelAdmissionFIFOAndBoundedCapacity(t *testing.T) {
	for _, queue := range []int64{0, 2} {
		t.Run(map[int64]string{0: "unbounded", 2: "bounded"}[queue], func(t *testing.T) {
			manager := NewManager()
			config := Config{MaxConcurrent: 1, QueueSize: queue}
			first, err := manager.Acquire(t.Context(), 7, config)
			if err != nil {
				t.Fatal(err)
			}
			defer first.Release()
			results := []chan *Permit{make(chan *Permit, 1), make(chan *Permit, 1)}
			for i := range results {
				go func() {
					permit, err := manager.Acquire(t.Context(), 7, config)
					if err != nil {
						t.Error(err)
					}
					results[i] <- permit
				}()
				waitFor(t, manager, 7, config, func(s Snapshot) bool { return s.Waiting == int64(i+1) })
			}
			if queue > 0 {
				if _, err := manager.Acquire(t.Context(), 7, config); !errors.Is(err, ErrQueueFull) {
					t.Fatalf("full queue: %v", err)
				}
			}
			if snapshot := manager.Snapshot(7, config); snapshot.Active != 1 {
				t.Fatalf("hard cap exceeded: %+v", snapshot)
			}
			// A different channel remains independent.
			other, err := manager.Acquire(t.Context(), 8, config)
			if err != nil {
				t.Fatal(err)
			}
			other.Release()
			first.Release()
			second := <-results[0]
			select {
			case <-results[1]:
				t.Fatal("later waiter overtook the FIFO head")
			default:
			}
			second.Release()
			third := <-results[1]
			third.Release()
			third.Release()
			if snapshot := manager.Snapshot(7, config); snapshot.Active != 0 || snapshot.Waiting != 0 {
				t.Fatalf("permits leaked: %+v", snapshot)
			}
		})
	}
}

func TestChannelAdmissionTimeoutCancellationAndPolicyChange(t *testing.T) {
	manager := NewManager()
	config := Config{MaxConcurrent: 1, QueueSize: 3, QueueTimeoutMs: 10}
	held, err := manager.Acquire(t.Context(), 1, config)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()
	if _, err := manager.Acquire(t.Context(), 1, config); !errors.Is(err, ErrQueueTimeout) {
		t.Fatalf("timeout=%v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	config.QueueTimeoutMs = 0
	go func() {
		permit, err := manager.Acquire(ctx, 1, config)
		permit.Release()
		result <- err
	}()
	waitFor(t, manager, 1, config, func(s Snapshot) bool { return s.Waiting == 1 })
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation=%v", err)
	}
	// Raising capacity preserves the existing active permit. Lowering it does
	// not create a new counter that forgets already running attempts.
	raised := config
	raised.MaxConcurrent = 2
	second, err := manager.Acquire(t.Context(), 1, raised)
	if err != nil {
		t.Fatal(err)
	}
	lowered := config
	lowered.QueueTimeoutMs = 10
	if _, err := manager.Acquire(t.Context(), 1, lowered); !errors.Is(err, ErrQueueTimeout) {
		t.Fatalf("lowered cap forgot active permits: %v", err)
	}
	if snapshot := manager.Snapshot(1, lowered); snapshot.Active != 2 || snapshot.Waiting != 0 {
		t.Fatalf("policy update state=%+v", snapshot)
	}
	second.Release()
	held.Release()
}

func TestChannelAdmissionGrantCancellationRace(t *testing.T) {
	manager := NewManager()
	config := Config{MaxConcurrent: 1, QueueSize: 1}
	for range 100 {
		first, err := manager.Acquire(t.Context(), 1, config)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() {
			permit, err := manager.Acquire(ctx, 1, config)
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Error(err)
			}
			permit.Release()
			close(done)
		}()
		waitFor(t, manager, 1, config, func(s Snapshot) bool { return s.Waiting == 1 })
		go cancel()
		first.Release()
		<-done
		cancel()
		if snapshot := manager.Snapshot(1, config); snapshot.Active != 0 || snapshot.Waiting != 0 {
			t.Fatalf("cancel/grant race leaked capacity: %+v", snapshot)
		}
	}
}

func TestChannelRPMAtomicAndNaturalMinuteRollover(t *testing.T) {
	manager := NewManager()
	now := time.Unix(120, 0)
	manager.now = func() time.Time { return now }
	config := Config{RPM: 3, TPM: 10}
	var passed atomic.Int64
	var wait sync.WaitGroup
	for range 30 {
		wait.Go(func() {
			if manager.TryRequest(t.Context(), 1, config) == nil {
				passed.Add(1)
			}
		})
	}
	wait.Wait()
	if passed.Load() != 3 {
		t.Fatalf("RPM admitted %d requests", passed.Load())
	}
	manager.RecordUsage(1, 12, true)
	manager.RecordUsage(1, 0, false)
	snapshot := manager.Snapshot(1, config)
	if snapshot.Requests != 3 || snapshot.Tokens != 12 || snapshot.UnknownUsage != 1 || snapshot.ExhaustedBy != "rpm" {
		t.Fatalf("rate snapshot=%+v", snapshot)
	}
	if err := manager.TryRequest(t.Context(), 2, config); err != nil {
		t.Fatalf("another channel inherited rate state: %v", err)
	}
	now = now.Add(time.Minute)
	if err := manager.TryRequest(t.Context(), 1, config); err != nil {
		t.Fatalf("new natural-minute bucket: %v", err)
	}
	if snapshot = manager.Snapshot(1, config); snapshot.Requests != 1 || snapshot.Tokens != 0 || snapshot.UnknownUsage != 0 {
		t.Fatalf("old bucket retained: %+v", snapshot)
	}
	manager.RecordUsage(1, 12, true)
	if err := manager.TryRequest(t.Context(), 1, config); err != nil {
		t.Fatalf("TPM became a hard admission limit: %v", err)
	}
	if snapshot = manager.Snapshot(1, config); snapshot.ExhaustedBy != "tpm" {
		t.Fatalf("TPM did not affect ranking: %+v", snapshot)
	}
}
