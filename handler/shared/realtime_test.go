package shared

import (
	"testing"
	"time"
)

func TestRealtimeRingBuffer_RecordsAndSnapshots(t *testing.T) {
	ResetRealtimeForTest()
	t.Cleanup(ResetRealtimeForTest)

	RecordRealtimeOutcome(true)
	RecordRealtimeOutcome(false)
	RecordRealtimeOutcome(true)

	points, lifetime, uptime := RealtimeSnapshot()
	if lifetime != 3 {
		t.Fatalf("lifetime = %d, want 3", lifetime)
	}
	// Uptime is wall-clock seconds of the test process (not the request counter).
	if uptime < 0 {
		t.Fatalf("uptime = %d, want >= 0", uptime)
	}
	if uptime > 600 {
		t.Fatalf("uptime = %d, want < 600 (fresh test process)", uptime)
	}
	if len(points) != realtimeWindowSecs {
		t.Fatalf("points = %d, want %d window seconds", len(points), realtimeWindowSecs)
	}
	// The last point is the current second with our 3 records.
	last := points[len(points)-1]
	if last.Total != 3 || last.Success != 2 {
		t.Fatalf("last point = %+v, want total 3 success 2", last)
	}
	// Series is ascending and contiguous.
	for i := 1; i < len(points); i++ {
		if points[i].Ts != points[i-1].Ts+1 {
			t.Fatalf("series not contiguous at %d: %d → %d", i, points[i-1].Ts, points[i].Ts)
		}
	}
}

func TestRealtimeRingBuffer_MissingSecondsZeroFilled(t *testing.T) {
	ResetRealtimeForTest()
	t.Cleanup(ResetRealtimeForTest)

	RecordRealtimeOutcome(true)

	points, _, _ := RealtimeSnapshot()
	// Every non-current second must be zero.
	for i := 0; i < len(points)-1; i++ {
		if points[i].Total != 0 {
			t.Fatalf("points[%d].Total = %d, want 0 (zero-filled)", i, points[i].Total)
		}
	}
}

func TestRealtimeRingBuffer_ConcurrentSafe(t *testing.T) {
	ResetRealtimeForTest()
	t.Cleanup(ResetRealtimeForTest)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 50_000; i++ {
			RecordRealtimeOutcome(true)
		}
	}()
	// Concurrent readers must not race or panic.
	for i := 0; i < 200; i++ {
		RealtimeSnapshot()
	}
	<-done
	_, lifetime, _ := RealtimeSnapshot()
	if lifetime != 50_000 {
		t.Fatalf("lifetime = %d, want 50000", lifetime)
	}
	_ = time.Now // keep time import (bucket math uses it internally)
}
