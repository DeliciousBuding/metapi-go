package shared

import (
	"sync/atomic"
	"time"
)

// ---- B2: realtime QPS ring buffer ----

// 1-second-granularity ring buffer of terminal proxy outcomes for the live
// ops WebSocket: total requests vs successes per second over a 300s window.
// Hot path is two atomic increments (no locks); reads take a snapshot.
// Multi-instance: each process counts its own traffic (same honesty rule as
// single-instance sticky — the panel shows this instance's live traffic).

const realtimeWindowSecs = 300

var realtimeMetrics struct {
	// Bucket index = unix seconds % realtimeWindowSecs.
	total    [realtimeWindowSecs]atomic.Int64
	success  [realtimeWindowSecs]atomic.Int64
	requests atomic.Int64 // lifetime total (cheap monotonic counter)
}

// RecordRealtimeOutcome records one terminal proxy outcome into the live ring
// buffer. Called from ObserveProxyOutcome (the unified terminal observation
// point) — never from per-attempt intermediate steps.
func RecordRealtimeOutcome(success bool) {
	now := time.Now().Unix()
	idx := now % realtimeWindowSecs
	realtimeMetrics.total[idx].Add(1)
	realtimeMetrics.requests.Add(1)
	if success {
		realtimeMetrics.success[idx].Add(1)
	}
}

// RealtimePoint is one second of live traffic.
type RealtimePoint struct {
	Ts      int64 `json:"ts"` // unix seconds
	Total   int64 `json:"total"`
	Success int64 `json:"success"`
}

// RealtimeSnapshot returns the live traffic series, oldest first, plus the
// lifetime request count. Missing seconds are zero-filled so the frontend can
// render a contiguous line without guessing.
func RealtimeSnapshot() (points []RealtimePoint, lifetime int64) {
	now := time.Now().Unix()
	points = make([]RealtimePoint, 0, realtimeWindowSecs)
	for sec := now - realtimeWindowSecs + 1; sec <= now; sec++ {
		idx := sec % realtimeWindowSecs
		points = append(points, RealtimePoint{
			Ts:      sec,
			Total:   realtimeMetrics.total[idx].Load(),
			Success: realtimeMetrics.success[idx].Load(),
		})
	}
	return points, realtimeMetrics.requests.Load()
}

// ResetRealtimeForTest clears the ring buffer (test-only).
func ResetRealtimeForTest() {
	for i := 0; i < realtimeWindowSecs; i++ {
		realtimeMetrics.total[i].Store(0)
		realtimeMetrics.success[i].Store(0)
	}
	realtimeMetrics.requests.Store(0)
}
