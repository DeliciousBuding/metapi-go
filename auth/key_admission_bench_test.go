package auth

import (
	"context"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// benchWindow stands in for the shared Redis counter with an injectable
// round-trip latency. Totals are atomic and nothing is held across the sleep, so
// the numbers reflect the limiter's own serialization rather than the fake's.
// This file deliberately touches only the public admission surface, so the same
// benchmark compiles and runs against both the pre-refactor and post-refactor
// limiter (that is how the before/after numbers were produced).
type benchWindow struct {
	delay time.Duration
	rpm   atomic.Int64
	tpm   atomic.Int64
}

func (c *benchWindow) roundTrip() {
	if c.delay > 0 {
		time.Sleep(c.delay)
	}
}

func (c *benchWindow) isTPM(key string) bool { return strings.HasPrefix(key, "metapi:tpm:") }

func (c *benchWindow) Incr(ctx context.Context, key string, window time.Duration) (int64, error) {
	_ = ctx
	_ = window
	c.roundTrip()
	if c.isTPM(key) {
		return c.tpm.Add(1), nil
	}
	return c.rpm.Add(1), nil
}

func (c *benchWindow) Decr(ctx context.Context, key string, window time.Duration) (int64, error) {
	_ = ctx
	_ = window
	c.roundTrip()
	if c.isTPM(key) {
		return c.tpm.Add(-1), nil
	}
	return c.rpm.Add(-1), nil
}

func (c *benchWindow) IncrBy(ctx context.Context, key string, delta int64, window time.Duration) (int64, error) {
	_ = ctx
	_ = window
	c.roundTrip()
	if c.isTPM(key) {
		return c.tpm.Add(delta), nil
	}
	return c.rpm.Add(delta), nil
}

func (c *benchWindow) Get(ctx context.Context, key string) (int64, error) {
	_ = ctx
	if c.isTPM(key) {
		return c.tpm.Load(), nil
	}
	return c.rpm.Load(), nil
}

// reportLatency reports the observed per-call latency distribution.
func reportLatency(b *testing.B, d []time.Duration) {
	if len(d) == 0 {
		return
	}
	sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
	at := func(p float64) float64 {
		idx := int(float64(len(d)-1) * p)
		return float64(d[idx].Microseconds()) / 1000
	}
	b.ReportMetric(float64(len(d)), "calls")
	b.ReportMetric(at(0.50), "p50-ms")
	b.ReportMetric(at(0.99), "p99-ms")
	b.ReportMetric(at(1.00), "max-ms")
}

// runAllowBench drives Allow from GOMAXPROCS goroutines (set with -cpu) and
// reports throughput plus the latency each caller observed. keyFor maps a worker
// to the key it hammers.
func runAllowBench(b *testing.B, setup func(l *KeyAdmissionLimiter), keyFor func(worker int64) int64) {
	b.Helper()
	l := NewKeyAdmissionLimiter()
	setup(l)
	// High enough that nothing is denied: every call is exactly one shared round
	// trip (no compensating Decr), which is the steady-state admission path.
	limit := int64(1_000_000_000)
	var seq atomic.Int64
	var mu sync.Mutex
	var all []time.Duration
	begin := time.Now()
	b.RunParallel(func(pb *testing.PB) {
		keyID := keyFor(seq.Add(1))
		durs := make([]time.Duration, 0, 4096)
		for pb.Next() {
			t0 := time.Now()
			l.Allow(keyID, &limit, nil, 0)
			durs = append(durs, time.Since(t0))
		}
		mu.Lock()
		all = append(all, durs...)
		mu.Unlock()
	})
	elapsed := time.Since(begin)
	b.ReportMetric(float64(b.N)/elapsed.Seconds(), "calls/s")
	reportLatency(b, all)
}

// BenchmarkAllowSharedManyKeys is the audit scenario: REDIS_URL configured, many
// managed keys, ~2ms per shared round trip. The old code held one global mutex
// across that round trip, so every key queued behind every other key.
func BenchmarkAllowSharedManyKeys(b *testing.B) {
	runAllowBench(b, func(l *KeyAdmissionLimiter) {
		l.SetSharedRPMCounter(&benchWindow{delay: 2 * time.Millisecond})
	}, func(worker int64) int64 { return worker })
}

// BenchmarkAllowSharedOneKey is the control: one hot key. Admissions for a single
// key must stay serialized (an Incr and its compensating Decr may not be
// reordered), so this number is expected to be latency-bound before and after.
func BenchmarkAllowSharedOneKey(b *testing.B) {
	runAllowBench(b, func(l *KeyAdmissionLimiter) {
		l.SetSharedRPMCounter(&benchWindow{delay: 2 * time.Millisecond})
	}, func(worker int64) int64 { return 1 })
}

// BenchmarkAllowMemoryOnlyManyKeys isolates the sharding win for the default
// deployment (no REDIS_URL): same in-memory work, but the global mutex is now 64
// stripes.
func BenchmarkAllowMemoryOnlyManyKeys(b *testing.B) {
	runAllowBench(b, func(l *KeyAdmissionLimiter) {}, func(worker int64) int64 { return worker })
}
