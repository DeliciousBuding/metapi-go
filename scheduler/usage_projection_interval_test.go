package scheduler

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
)

// TestUsageAggregationProjectionInterval covers the resolution order: the
// configured cadence wins, and callers that build config.Config by hand (tests,
// embedders) or pass nil fall back to the package default instead of spinning at
// a zero interval.
func TestUsageAggregationProjectionInterval(t *testing.T) {
	if got := NewUsageAggregationScheduler(&config.Config{}).projectionIntervalMs(); got != defaultUsageProjectionIntervalMs {
		t.Fatalf("zero-value config interval = %d, want default %d", got, defaultUsageProjectionIntervalMs)
	}
	if got := NewUsageAggregationScheduler(nil).projectionIntervalMs(); got != defaultUsageProjectionIntervalMs {
		t.Fatalf("nil config interval = %d, want default %d", got, defaultUsageProjectionIntervalMs)
	}
	cfg, _ := config.Load(map[string]string{"USAGE_PROJECTION_INTERVAL_MS": "30000"})
	if got := NewUsageAggregationScheduler(cfg).projectionIntervalMs(); got != 30000 {
		t.Fatalf("configured interval = %d, want 30000", got)
	}
}
