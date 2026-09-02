package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

// blockingWindow is a WindowCounter whose Incr parks until the caller's context
// is done (or release is closed), so a test can observe cancellation instead of
// waiting out the counter's own timeout.
type blockingWindow struct {
	release chan struct{}
	started chan struct{}

	mu       chan struct{} // closed once; guards lastErr writes below
	lastErr  error
	lastKey  string
	incrHits int
}

func newBlockingWindow() *blockingWindow {
	return &blockingWindow{
		release: make(chan struct{}),
		started: make(chan struct{}, 8),
		mu:      make(chan struct{}, 1),
	}
}

func (b *blockingWindow) record(key string, err error) {
	b.mu <- struct{}{}
	defer func() { <-b.mu }()
	b.lastKey = key
	b.lastErr = err
	b.incrHits++
}

func (b *blockingWindow) Incr(ctx context.Context, key string, window time.Duration) (int64, error) {
	select {
	case b.started <- struct{}{}:
	default:
	}
	select {
	case <-ctx.Done():
		b.record(key, ctx.Err())
		return 0, ctx.Err()
	case <-b.release:
		b.record(key, nil)
		return 1, nil
	}
}

func (b *blockingWindow) Decr(context.Context, string, time.Duration) (int64, error) { return 0, nil }

func (b *blockingWindow) IncrBy(context.Context, string, int64, time.Duration) (int64, error) {
	return 0, nil
}

func (b *blockingWindow) Get(context.Context, string) (int64, error) { return 0, nil }

func (b *blockingWindow) err() error {
	b.mu <- struct{}{}
	defer func() { <-b.mu }()
	return b.lastErr
}

// TestKeyAdmission_AllowCancelledContextFailsOpenFast pins the ctx contract: a
// client that goes away cancels its own shared-counter round trip instead of
// holding the per-key serialization mutex for the full counter timeout. The
// cancelled round trip is an error, so it takes the existing fail-open path and
// the local window still records the reservation.
func TestKeyAdmission_AllowCancelledContextFailsOpenFast(t *testing.T) {
	l := NewKeyAdmissionLimiter()
	bw := newBlockingWindow()
	l.SetSharedRPMCounter(bw)
	limit := int64(5)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-bw.started
		cancel()
	}()

	start := time.Now()
	d := l.Allow(ctx, 1, &limit, nil, 0)
	elapsed := time.Since(start)

	if !d.Allowed {
		t.Fatalf("cancelled shared round trip = %+v, want fail-open allow", d)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("Allow took %v after cancellation, want prompt return", elapsed)
	}
	if !errors.Is(bw.err(), context.Canceled) {
		t.Fatalf("counter error = %v, want context.Canceled", bw.err())
	}
	if used, _ := l.Snapshot(1); used != 1 {
		t.Fatalf("local usedRPM after fail-open = %d, want 1 (fallback still reserves)", used)
	}
}

// TestKeyAdmission_AllowPreCancelledContext covers the already-gone client: the
// decision must not block on a shared round trip at all.
func TestKeyAdmission_AllowPreCancelledContext(t *testing.T) {
	l := NewKeyAdmissionLimiter()
	bw := newBlockingWindow()
	l.SetSharedRPMCounter(bw)
	limit := int64(5)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	d := l.Allow(ctx, 2, &limit, nil, 0)
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("pre-cancelled Allow took %v, want immediate", elapsed)
	}
	if !d.Allowed {
		t.Fatalf("pre-cancelled decision = %+v, want fail-open allow", d)
	}
	if !errors.Is(bw.err(), context.Canceled) {
		t.Fatalf("counter error = %v, want context.Canceled", bw.err())
	}
}

// TestKeyAdmission_AllowNilContextIsBackground keeps the exported method safe
// for callers that have no request context (CLI, scheduler-driven probes).
func TestKeyAdmission_AllowNilContextIsBackground(t *testing.T) {
	l := NewKeyAdmissionLimiter()
	fw := &fakeWindow{}
	l.SetSharedRPMCounter(fw)
	limit := int64(1)

	//lint:ignore SA1012 nil ctx is an explicitly supported input here.
	if d := l.Allow(nil, 3, &limit, nil, 0); !d.Allowed {
		t.Fatalf("nil ctx decision = %+v, want allow", d)
	}
	//lint:ignore SA1012 nil ctx is an explicitly supported input here.
	if d := l.Allow(nil, 3, &limit, nil, 0); d.Allowed || d.Reason != "over_rpm" {
		t.Fatalf("second nil ctx decision = %+v, want over_rpm", d)
	}
}
