package sharedcount

import (
	"context"
	"testing"
	"time"
)

// BenchmarkRedisCounterIncr measures one INCR against the fake server with AUTH
// and a non-zero SELECT configured, i.e. the handshake shape production uses.
// Per call the unpooled client paid connect + AUTH + SELECT + INCR + close; the
// pooled client pays INCR alone. peak-conns/auths show the difference directly.
func BenchmarkRedisCounterIncr(b *testing.B) { benchRedisCounterIncr(b, 0) }

// BenchmarkRedisCounterIncrDelayed2ms adds a 2ms per-command server latency so the
// handshake round trips (which pooling removes) are visible as wall time.
func BenchmarkRedisCounterIncrDelayed2ms(b *testing.B) {
	benchRedisCounterIncr(b, 2*time.Millisecond)
}

func benchRedisCounterIncr(b *testing.B, delay time.Duration) {
	b.Helper()
	f := newFakeRedisWithAuth(b, "bench-pw")
	f.setDelay(delay)
	defer f.close()
	c, err := NewRedisCounter("redis://:bench-pw@" + f.addr() + "/2")
	if err != nil {
		b.Fatalf("NewRedisCounter: %v", err)
	}
	c.timeout = 10 * time.Second
	ctx := context.Background()
	// Warm-up is intentionally skipped: the first call's connect+AUTH+SELECT is
	// part of what the two implementations differ on.
	b.ResetTimer()
	begin := time.Now()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := c.Incr(ctx, "bench", time.Minute); err != nil {
				b.Errorf("Incr: %v", err)
				return
			}
		}
	})
	elapsed := time.Since(begin)
	b.StopTimer()
	b.ReportMetric(float64(b.N)/elapsed.Seconds(), "incrs/s")
	b.ReportMetric(float64(f.peakConns()), "peak-conns")
	b.ReportMetric(float64(f.auths.Load()), "auths")
	b.ReportMetric(float64(f.selects.Load()), "selects")
}
