package notify

import (
	"fmt"
	"testing"
)

func TestNotificationThrottle_BoundsEntries(t *testing.T) {
	throttle := NewNotificationThrottle()
	throttle.SetMaxEntries(32)
	now := int64(1000)
	for i := 0; i < 500; i++ {
		throttle.EvaluateNotificationThrottle(fmt.Sprintf("signature-%d", i), now, 60_000)
	}
	throttle.mu.Lock()
	entries := len(throttle.state)
	throttle.mu.Unlock()
	if entries > 32 {
		t.Fatalf("throttle holds %d entries, want at most 32", entries)
	}
}
