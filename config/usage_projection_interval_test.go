package config

import "testing"

// TestLoadUsageProjectionIntervalDefault pins the 5s default: admin dashboards
// read the site/model rollups, so the projection pass stays near-real-time out
// of the box.
func TestLoadUsageProjectionIntervalDefault(t *testing.T) {
	if DefaultUsageProjectionIntervalMs != 5000 {
		t.Fatalf("DefaultUsageProjectionIntervalMs = %d, want 5000", DefaultUsageProjectionIntervalMs)
	}
	cfg, _ := Load(map[string]string{})
	if cfg.UsageProjectionIntervalMs != DefaultUsageProjectionIntervalMs {
		t.Fatalf("UsageProjectionIntervalMs = %d, want default %d",
			cfg.UsageProjectionIntervalMs, DefaultUsageProjectionIntervalMs)
	}
}

// TestLoadUsageProjectionIntervalClamped verifies the floor/ceiling: tiny or
// negative values clamp to 1s (the pass must never spin), huge values clamp to
// 1h, and an unparseable value falls back to the default instead of clamping.
func TestLoadUsageProjectionIntervalClamped(t *testing.T) {
	for _, env := range []map[string]string{
		{"USAGE_PROJECTION_INTERVAL_MS": "0"},
		{"USAGE_PROJECTION_INTERVAL_MS": "-5"},
		{"USAGE_PROJECTION_INTERVAL_MS": "1"},
	} {
		cfg, _ := Load(env)
		if cfg.UsageProjectionIntervalMs != 1000 {
			t.Fatalf("Load(%v).UsageProjectionIntervalMs = %d, want clamp floor 1000",
				env, cfg.UsageProjectionIntervalMs)
		}
	}
	cfg, _ := Load(map[string]string{"USAGE_PROJECTION_INTERVAL_MS": "99999999"})
	if cfg.UsageProjectionIntervalMs != 3600000 {
		t.Fatalf("UsageProjectionIntervalMs = %d, want clamp ceiling 3600000", cfg.UsageProjectionIntervalMs)
	}
	cfg, _ = Load(map[string]string{"USAGE_PROJECTION_INTERVAL_MS": "not-a-number"})
	if cfg.UsageProjectionIntervalMs != DefaultUsageProjectionIntervalMs {
		t.Fatalf("invalid value = %d, want fallback default %d",
			cfg.UsageProjectionIntervalMs, DefaultUsageProjectionIntervalMs)
	}
}

// TestLoadUsageProjectionIntervalExplicit verifies an operator-supplied cadence
// wins over the default (small single-node deployments relax it to cut passes).
func TestLoadUsageProjectionIntervalExplicit(t *testing.T) {
	cfg, _ := Load(map[string]string{"USAGE_PROJECTION_INTERVAL_MS": "30000"})
	if cfg.UsageProjectionIntervalMs != 30000 {
		t.Fatalf("UsageProjectionIntervalMs = %d, want 30000", cfg.UsageProjectionIntervalMs)
	}
}
