package config

import "testing"

// TestLoadProxyVideoTaskRetentionDefaults pins the audit-driven default: video
// task mappings (publicId -> upstream video id) are short-lived, so retention
// must be on out of the box. The same knob drives the proxy_video_tasks pruner
// and the TTL of the process-local rewrite cache in handler/proxy.
func TestLoadProxyVideoTaskRetentionDefaults(t *testing.T) {
	if DefaultProxyVideoTaskRetentionDays != 7 {
		t.Fatalf("DefaultProxyVideoTaskRetentionDays = %d, want 7", DefaultProxyVideoTaskRetentionDays)
	}
	cfg, _ := Load(map[string]string{})
	if cfg.ProxyVideoTaskRetentionDays != DefaultProxyVideoTaskRetentionDays {
		t.Fatalf("ProxyVideoTaskRetentionDays = %d, want default %d",
			cfg.ProxyVideoTaskRetentionDays, DefaultProxyVideoTaskRetentionDays)
	}
	if cfg.ProxyVideoTaskRetentionPruneIntervalMinutes != DefaultProxyVideoTaskRetentionPruneIntervalMinutes {
		t.Fatalf("ProxyVideoTaskRetentionPruneIntervalMinutes = %d, want %d",
			cfg.ProxyVideoTaskRetentionPruneIntervalMinutes, DefaultProxyVideoTaskRetentionPruneIntervalMinutes)
	}
}

// TestLoadProxyVideoTaskRetentionExplicitDisable keeps the opt-out semantics:
// 0 (or anything negative, clamped to 0) still means "operator disabled
// retention" — the scheduler no-ops and the cache keeps lines until the
// capacity guardrail trims them.
func TestLoadProxyVideoTaskRetentionExplicitDisable(t *testing.T) {
	for _, env := range []map[string]string{
		{"PROXY_VIDEO_TASK_RETENTION_DAYS": "0"},
		{"PROXY_VIDEO_TASK_RETENTION_DAYS": "-5"},
	} {
		cfg, _ := Load(env)
		if cfg.ProxyVideoTaskRetentionDays != 0 {
			t.Fatalf("Load(%v).ProxyVideoTaskRetentionDays = %d, want 0 (disabled)",
				env, cfg.ProxyVideoTaskRetentionDays)
		}
	}
}

// TestLoadProxyVideoTaskRetentionExplicitValue verifies an operator-supplied
// window wins over the default, and that an unparseable value falls back to the
// default instead of silently disabling retention.
func TestLoadProxyVideoTaskRetentionExplicitValue(t *testing.T) {
	if cfg, _ := Load(map[string]string{"PROXY_VIDEO_TASK_RETENTION_DAYS": "not-a-number"}); cfg.ProxyVideoTaskRetentionDays != DefaultProxyVideoTaskRetentionDays {
		t.Fatalf("invalid value = %d, want fallback default %d",
			cfg.ProxyVideoTaskRetentionDays, DefaultProxyVideoTaskRetentionDays)
	}

	cfg, _ := Load(map[string]string{
		"PROXY_VIDEO_TASK_RETENTION_DAYS":                   "3",
		"PROXY_VIDEO_TASK_RETENTION_PRUNE_INTERVAL_MINUTES": "15",
	})
	if cfg.ProxyVideoTaskRetentionDays != 3 {
		t.Fatalf("ProxyVideoTaskRetentionDays = %d, want 3", cfg.ProxyVideoTaskRetentionDays)
	}
	if cfg.ProxyVideoTaskRetentionPruneIntervalMinutes != 15 {
		t.Fatalf("ProxyVideoTaskRetentionPruneIntervalMinutes = %d, want 15",
			cfg.ProxyVideoTaskRetentionPruneIntervalMinutes)
	}
}
