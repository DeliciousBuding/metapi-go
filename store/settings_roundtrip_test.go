package store

import (
	"bytes"
	"log/slog"
	"reflect"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
)

// Settings round-trip tests: what the admin API persists must come back out of
// startup hydration with the same meaning, clamped exactly like the same value
// would be clamped when it arrives through env. The key-coverage half of this
// contract lives in settings_rehydration_gate_test.go.

// hydrateTestDB publishes a fresh in-memory SQLite DB as the process-wide
// store DB so LoadRuntimeSettings (which reads through GetDB) can be exercised
// end to end. The singleton is package-private, so these tests must not run in
// parallel with each other.
func hydrateTestDB(t *testing.T) *SettingsStore {
	t.Helper()
	db, err := Open(DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	if err := AutoMigrate(db); err != nil {
		db.Close()
		t.Fatalf("AutoMigrate failed: %v", err)
	}
	previous := activeDB.Swap(db)
	t.Cleanup(func() {
		activeDB.Store(previous)
		db.Close()
	})
	return NewSettingsStore(db)
}

// quietSecretEnv keeps config.Load from emitting its "insecure default
// credential secret" warning on every call; the secret is unrelated to the
// round-trip behavior under test.
func quietSecretEnv(extra map[string]string) map[string]string {
	env := map[string]string{"ACCOUNT_CREDENTIAL_SECRET": "round-trip-test-secret"}
	for key, value := range extra {
		env[key] = value
	}
	return env
}

// ---- Startup hydration: keys that used to be persisted but never read back ----

func TestApplyRuntimeSettingsHydratesAdminIpAllowlist(t *testing.T) {
	rt := &config.RuntimeSettings{AdminIpAllowlist: []string{"203.0.113.9"}}
	ApplyRuntimeSettings(&config.Config{}, rt, map[string]string{
		"admin_ip_allowlist": `["203.0.113.7"," 198.51.100.9 ","203.0.113.7"]`,
	})
	want := []string{"203.0.113.7", "198.51.100.9"}
	if !reflect.DeepEqual(rt.AdminIpAllowlist, want) {
		t.Fatalf("AdminIpAllowlist = %#v, want %#v", rt.AdminIpAllowlist, want)
	}
}

func TestApplyRuntimeSettingsAdminIpAllowlistExplicitEmptyClears(t *testing.T) {
	rt := &config.RuntimeSettings{AdminIpAllowlist: []string{"203.0.113.7"}}
	ApplyRuntimeSettings(&config.Config{}, rt, map[string]string{
		"admin_ip_allowlist": `[]`,
	})
	if rt.AdminIpAllowlist == nil || len(rt.AdminIpAllowlist) != 0 {
		t.Fatalf("AdminIpAllowlist = %#v, want empty non-nil slice", rt.AdminIpAllowlist)
	}
}

// An unparseable row must never wipe a persisted allowlist: an empty list
// means "every IP may reach the admin API".
func TestApplyRuntimeSettingsAdminIpAllowlistInvalidDoesNotWipe(t *testing.T) {
	rt := &config.RuntimeSettings{AdminIpAllowlist: []string{"203.0.113.7"}}
	ApplyRuntimeSettings(&config.Config{}, rt, map[string]string{
		"admin_ip_allowlist": `{"oops":true}`,
	})
	if !reflect.DeepEqual(rt.AdminIpAllowlist, []string{"203.0.113.7"}) {
		t.Fatalf("invalid value wiped the allowlist: %#v", rt.AdminIpAllowlist)
	}
}

func TestApplyRuntimeSettingsHydratesProxyErrorKeywords(t *testing.T) {
	rt := &config.RuntimeSettings{}
	ApplyRuntimeSettings(&config.Config{}, rt, map[string]string{
		"proxy_error_keywords": `["rate limit","overloaded"]`,
	})
	want := []string{"rate limit", "overloaded"}
	if !reflect.DeepEqual(rt.ProxyErrorKeywords, want) {
		t.Fatalf("ProxyErrorKeywords = %#v, want %#v", rt.ProxyErrorKeywords, want)
	}
}

func TestApplyRuntimeSettingsHydratesRoutingKeys(t *testing.T) {
	rt := &config.RuntimeSettings{
		RoutingFallbackUnitCost:          1,
		TokenRouterFailureCooldownMaxSec: config.TokenRouterFailureCooldownMaxSecCeiling,
		ProxyFirstByteTimeoutSec:         0,
		RoutingWeights:                   config.RoutingWeights{BaseWeightFactor: 0.5, ValueScoreFactor: 0.5, CostWeight: 0.4, BalanceWeight: 0.3, UsageWeight: 0.3},
	}
	ApplyRuntimeSettings(&config.Config{}, rt, map[string]string{
		"routing_fallback_unit_cost":            `0.25`,
		"token_router_failure_cooldown_max_sec": `3600`,
		"proxy_first_byte_timeout_sec":          `45`,
		"routing_weights":                       `{"BaseWeightFactor":0.7,"CostWeight":0.2}`,
	})
	if rt.RoutingFallbackUnitCost != 0.25 {
		t.Errorf("RoutingFallbackUnitCost = %v, want 0.25", rt.RoutingFallbackUnitCost)
	}
	if rt.TokenRouterFailureCooldownMaxSec != 3600 {
		t.Errorf("TokenRouterFailureCooldownMaxSec = %d, want 3600", rt.TokenRouterFailureCooldownMaxSec)
	}
	if rt.ProxyFirstByteTimeoutSec != 45 {
		t.Errorf("ProxyFirstByteTimeoutSec = %d, want 45", rt.ProxyFirstByteTimeoutSec)
	}
	// Weights are persisted as a whole object and merged into the
	// env-resolved vector: components the row does not mention survive.
	want := config.RoutingWeights{BaseWeightFactor: 0.7, ValueScoreFactor: 0.5, CostWeight: 0.2, BalanceWeight: 0.3, UsageWeight: 0.3}
	if rt.RoutingWeights != want {
		t.Errorf("RoutingWeights = %+v, want %+v", rt.RoutingWeights, want)
	}
}

// A cooldown cap that config.Load would reject must keep the env-resolved cap
// instead of silently disabling failure cooldowns.
func TestApplyRuntimeSettingsRejectsInvalidCooldownCap(t *testing.T) {
	for _, value := range []string{`-5`, `0`, `nonsense`} {
		rt := &config.RuntimeSettings{TokenRouterFailureCooldownMaxSec: 900}
		ApplyRuntimeSettings(&config.Config{}, rt, map[string]string{
			"token_router_failure_cooldown_max_sec": value,
		})
		if rt.TokenRouterFailureCooldownMaxSec != 900 {
			t.Errorf("value %s: TokenRouterFailureCooldownMaxSec = %d, want the env-resolved 900",
				value, rt.TokenRouterFailureCooldownMaxSec)
		}
	}
}

func TestApplyRuntimeSettingsHydratesProxyPolicyToggles(t *testing.T) {
	rt := &config.RuntimeSettings{}
	ApplyRuntimeSettings(&config.Config{}, rt, map[string]string{
		"disable_cross_protocol_fallback":                 `true`,
		"responses_compact_fallback_to_responses_enabled": `true`,
		"proxy_empty_content_fail_enabled":                `true`,
		"proxy_session_channel_concurrency_limit":         `4`,
		"proxy_session_channel_queue_wait_ms":             `2500`,
	})
	if !rt.DisableCrossProtocolFallback {
		t.Error("DisableCrossProtocolFallback = false, want true")
	}
	if !rt.ResponsesCompactFallbackToResponsesEnabled {
		t.Error("ResponsesCompactFallbackToResponsesEnabled = false, want true")
	}
	if !rt.ProxyEmptyContentFailEnabled {
		t.Error("ProxyEmptyContentFailEnabled = false, want true")
	}
	if rt.ProxySessionChannelConcurrencyLimit != 4 {
		t.Errorf("ProxySessionChannelConcurrencyLimit = %d, want 4", rt.ProxySessionChannelConcurrencyLimit)
	}
	if rt.ProxySessionChannelQueueWaitMs != 2500 {
		t.Errorf("ProxySessionChannelQueueWaitMs = %d, want 2500", rt.ProxySessionChannelQueueWaitMs)
	}
}

func TestApplyRuntimeSettingsHydratesProxyDebugKnobs(t *testing.T) {
	rt := &config.RuntimeSettings{ProxyDebugCaptureHeaders: true}
	ApplyRuntimeSettings(&config.Config{}, rt, map[string]string{
		"proxy_debug_capture_headers":       `false`,
		"proxy_debug_capture_bodies":        `true`,
		"proxy_debug_capture_stream_chunks": `true`,
		"proxy_debug_target_session_id":     `"sess-1"`,
		"proxy_debug_target_client_kind":    `"codex"`,
		"proxy_debug_target_model":          `"gpt-5"`,
		"proxy_debug_retention_hours":       `12`,
		"proxy_debug_max_body_bytes":        `4096`,
	})
	if rt.ProxyDebugCaptureHeaders {
		t.Error("ProxyDebugCaptureHeaders = true, want false")
	}
	if !rt.ProxyDebugCaptureBodies || !rt.ProxyDebugCaptureStreamChunks {
		t.Errorf("capture bodies/chunks = %v/%v, want true/true",
			rt.ProxyDebugCaptureBodies, rt.ProxyDebugCaptureStreamChunks)
	}
	if rt.ProxyDebugTargetSessionId != "sess-1" || rt.ProxyDebugTargetClientKind != "codex" || rt.ProxyDebugTargetModel != "gpt-5" {
		t.Errorf("debug targets = (%q, %q, %q), want (sess-1, codex, gpt-5)",
			rt.ProxyDebugTargetSessionId, rt.ProxyDebugTargetClientKind, rt.ProxyDebugTargetModel)
	}
	if rt.ProxyDebugRetentionHours != 12 {
		t.Errorf("ProxyDebugRetentionHours = %d, want 12", rt.ProxyDebugRetentionHours)
	}
	if rt.ProxyDebugMaxBodyBytes != 4096 {
		t.Errorf("ProxyDebugMaxBodyBytes = %d, want 4096", rt.ProxyDebugMaxBodyBytes)
	}
}

func TestApplyRuntimeSettingsHydratesSchedulerKillSwitches(t *testing.T) {
	rt := &config.RuntimeSettings{
		CheckinCron:        config.DefaultCheckinCron,
		BalanceRefreshCron: config.DefaultBalanceRefreshCron,
		ModelSyncCron:      config.DefaultModelSyncCron,
		LogCleanupCron:     config.DefaultLogCleanupCron,
	}
	ApplyRuntimeSettings(&config.Config{}, rt, map[string]string{
		"checkin_enabled":         `false`,
		"balance_refresh_enabled": `false`,
		"balance_refresh_cron":    `"15 3 * * *"`,
		"model_sync_cron":         `"25 2 * * *"`,
		"log_cleanup_cron":        `"30 4 * * *"`,
	})
	// Persisted as the positive switch, published inverted (zero value =
	// enabled), exactly like config.Load's CHECKIN_ENABLED handling.
	if !rt.CheckinDisabled {
		t.Error("CheckinDisabled = false, want true after checkin_enabled=false")
	}
	if !rt.BalanceRefreshDisabled {
		t.Error("BalanceRefreshDisabled = false, want true after balance_refresh_enabled=false")
	}
	if rt.BalanceRefreshCron != "15 3 * * *" || rt.ModelSyncCron != "25 2 * * *" || rt.LogCleanupCron != "30 4 * * *" {
		t.Errorf("crons = (%q, %q, %q), want the persisted expressions",
			rt.BalanceRefreshCron, rt.ModelSyncCron, rt.LogCleanupCron)
	}
}

// An invalid cron must keep the resolved default: the schedulers use the
// snapshot value as their fallback, so a bad row would otherwise take the
// whole job down at Start.
func TestApplyRuntimeSettingsRejectsInvalidCrons(t *testing.T) {
	rt := &config.RuntimeSettings{
		CheckinCron:        config.DefaultCheckinCron,
		BalanceRefreshCron: config.DefaultBalanceRefreshCron,
		ModelSyncCron:      config.DefaultModelSyncCron,
		LogCleanupCron:     config.DefaultLogCleanupCron,
	}
	ApplyRuntimeSettings(&config.Config{}, rt, map[string]string{
		"checkin_cron":         `"not a cron"`,
		"balance_refresh_cron": `""`,
		"model_sync_cron":      `"* * * * * * *"`,
		"log_cleanup_cron":     `"61 0 * * *"`,
	})
	if rt.CheckinCron != config.DefaultCheckinCron {
		t.Errorf("CheckinCron = %q, want the default %q", rt.CheckinCron, config.DefaultCheckinCron)
	}
	if rt.BalanceRefreshCron != config.DefaultBalanceRefreshCron {
		t.Errorf("BalanceRefreshCron = %q, want the default %q", rt.BalanceRefreshCron, config.DefaultBalanceRefreshCron)
	}
	if rt.ModelSyncCron != config.DefaultModelSyncCron {
		t.Errorf("ModelSyncCron = %q, want the default %q", rt.ModelSyncCron, config.DefaultModelSyncCron)
	}
	if rt.LogCleanupCron != config.DefaultLogCleanupCron {
		t.Errorf("LogCleanupCron = %q, want the default %q", rt.LogCleanupCron, config.DefaultLogCleanupCron)
	}
}

// ---- Startup hydration must clamp exactly like config.Load ----

// TestApplyRuntimeSettingsClampsMatchConfigLoad feeds the same value through
// both configuration paths — env into config.Load, and the JSON-encoded
// settings row into ApplyRuntimeSettings — and requires identical results. A
// settings-path shortcut around a clamp is how an operator ends up with two
// different products depending on where a number was typed.
func TestApplyRuntimeSettingsClampsMatchConfigLoad(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		// settingsEnv seeds the settings-path config.Load. Rows that must be
		// read on top of an env-resolved value set it to the same env; every
		// other row leaves it nil and compares against the built-in defaults.
		settingsEnv map[string]string
		settings    map[string]string
		get         func(*config.Config, *config.RuntimeSettings) any
	}{
		{
			// config.Load resolves CHECKIN_INTERVAL_HOURS as
			// ClampInt(trunc(n), 1, 24), so 30 becomes 24. The settings path
			// used to drop an out-of-range row instead of clamping it, leaving
			// the env-resolved value in the snapshot with nothing in the log.
			name:     "checkin interval ceiling",
			env:      map[string]string{"CHECKIN_INTERVAL_HOURS": "30"},
			settings: map[string]string{"checkin_interval_hours": `30`},
			get:      func(_ *config.Config, rt *config.RuntimeSettings) any { return rt.CheckinIntervalHours },
		},
		{
			name:     "checkin interval floor",
			env:      map[string]string{"CHECKIN_INTERVAL_HOURS": "0"},
			settings: map[string]string{"checkin_interval_hours": `0`},
			get:      func(_ *config.Config, rt *config.RuntimeSettings) any { return rt.CheckinIntervalHours },
		},
		{
			name:     "checkin interval truncates",
			env:      map[string]string{"CHECKIN_INTERVAL_HOURS": "12.9"},
			settings: map[string]string{"checkin_interval_hours": `12.9`},
			get:      func(_ *config.Config, rt *config.RuntimeSettings) any { return rt.CheckinIntervalHours },
		},
		{
			name:     "first byte timeout floor",
			env:      map[string]string{"PROXY_FIRST_BYTE_TIMEOUT_SEC": "-5"},
			settings: map[string]string{"proxy_first_byte_timeout_sec": `-5`},
			get:      func(_ *config.Config, rt *config.RuntimeSettings) any { return rt.ProxyFirstByteTimeoutSec },
		},
		{
			name:     "first byte timeout truncates",
			env:      map[string]string{"PROXY_FIRST_BYTE_TIMEOUT_SEC": "45.9"},
			settings: map[string]string{"proxy_first_byte_timeout_sec": `45.9`},
			get:      func(_ *config.Config, rt *config.RuntimeSettings) any { return rt.ProxyFirstByteTimeoutSec },
		},
		{
			name:     "first byte timeout unparsable",
			env:      map[string]string{"PROXY_FIRST_BYTE_TIMEOUT_SEC": "nonsense"},
			settings: map[string]string{"proxy_first_byte_timeout_sec": `nonsense`},
			get:      func(_ *config.Config, rt *config.RuntimeSettings) any { return rt.ProxyFirstByteTimeoutSec },
		},
		{
			name:     "log cleanup retention floor",
			env:      map[string]string{"LOG_CLEANUP_RETENTION_DAYS": "0"},
			settings: map[string]string{"log_cleanup_retention_days": `0`},
			get:      func(_ *config.Config, rt *config.RuntimeSettings) any { return rt.LogCleanupRetentionDays },
		},
		{
			name:     "log cleanup retention truncates",
			env:      map[string]string{"LOG_CLEANUP_RETENTION_DAYS": "45.9"},
			settings: map[string]string{"log_cleanup_retention_days": `45.9`},
			get:      func(_ *config.Config, rt *config.RuntimeSettings) any { return rt.LogCleanupRetentionDays },
		},
		{
			name:     "proxy debug retention floor",
			env:      map[string]string{"PROXY_DEBUG_RETENTION_HOURS": "-3"},
			settings: map[string]string{"proxy_debug_retention_hours": `-3`},
			get:      func(_ *config.Config, rt *config.RuntimeSettings) any { return rt.ProxyDebugRetentionHours },
		},
		{
			name:     "proxy debug capture size floor",
			env:      map[string]string{"PROXY_DEBUG_MAX_BODY_BYTES": "10"},
			settings: map[string]string{"proxy_debug_max_body_bytes": `10`},
			get:      func(_ *config.Config, rt *config.RuntimeSettings) any { return rt.ProxyDebugMaxBodyBytes },
		},
		{
			name:     "session concurrency floor",
			env:      map[string]string{"PROXY_SESSION_CHANNEL_CONCURRENCY_LIMIT": "-3"},
			settings: map[string]string{"proxy_session_channel_concurrency_limit": `-3`},
			get:      func(_ *config.Config, rt *config.RuntimeSettings) any { return rt.ProxySessionChannelConcurrencyLimit },
		},
		{
			name:     "session queue wait floor",
			env:      map[string]string{"PROXY_SESSION_CHANNEL_QUEUE_WAIT_MS": "-3"},
			settings: map[string]string{"proxy_session_channel_queue_wait_ms": `-3`},
			get:      func(_ *config.Config, rt *config.RuntimeSettings) any { return rt.ProxySessionChannelQueueWaitMs },
		},
		{
			name:     "fallback unit cost floor",
			env:      map[string]string{"ROUTING_FALLBACK_UNIT_COST": "-1"},
			settings: map[string]string{"routing_fallback_unit_cost": `-1`},
			get:      func(_ *config.Config, rt *config.RuntimeSettings) any { return rt.RoutingFallbackUnitCost },
		},
		{
			name:     "cooldown cap rejects non-positive",
			env:      map[string]string{"TOKEN_ROUTER_FAILURE_COOLDOWN_MAX_SEC": "-5"},
			settings: map[string]string{"token_router_failure_cooldown_max_sec": `-5`},
			get:      func(_ *config.Config, rt *config.RuntimeSettings) any { return rt.TokenRouterFailureCooldownMaxSec },
		},
		{
			name:     "cooldown cap ceiling",
			env:      map[string]string{"TOKEN_ROUTER_FAILURE_COOLDOWN_MAX_SEC": "99999999"},
			settings: map[string]string{"token_router_failure_cooldown_max_sec": `99999999`},
			get:      func(_ *config.Config, rt *config.RuntimeSettings) any { return rt.TokenRouterFailureCooldownMaxSec },
		},
		{
			name:     "cooldown cap accepted",
			env:      map[string]string{"TOKEN_ROUTER_FAILURE_COOLDOWN_MAX_SEC": "3600"},
			settings: map[string]string{"token_router_failure_cooldown_max_sec": `3600`},
			get:      func(_ *config.Config, rt *config.RuntimeSettings) any { return rt.TokenRouterFailureCooldownMaxSec },
		},
		{
			name:     "admin ip allowlist list shape",
			env:      map[string]string{"ADMIN_IP_ALLOWLIST": "203.0.113.7, 198.51.100.9"},
			settings: map[string]string{"admin_ip_allowlist": `["203.0.113.7","198.51.100.9"]`},
			get:      func(_ *config.Config, rt *config.RuntimeSettings) any { return rt.AdminIpAllowlist },
		},
		{
			name:     "proxy error keywords list shape",
			env:      map[string]string{"PROXY_ERROR_KEYWORDS": "rate limit,overloaded"},
			settings: map[string]string{"proxy_error_keywords": `["rate limit","overloaded"]`},
			get:      func(_ *config.Config, rt *config.RuntimeSettings) any { return rt.ProxyErrorKeywords },
		},
		{
			name: "routing weights vector",
			env: map[string]string{
				"BASE_WEIGHT_FACTOR": "0.7", "VALUE_SCORE_FACTOR": "0.11", "COST_WEIGHT": "0.22",
				"BALANCE_WEIGHT": "0.33", "USAGE_WEIGHT": "0.44",
			},
			settings: map[string]string{"routing_weights": `{"BaseWeightFactor":0.7,"ValueScoreFactor":0.11,"CostWeight":0.22,"BalanceWeight":0.33,"UsageWeight":0.44}`},
			get:      func(_ *config.Config, rt *config.RuntimeSettings) any { return rt.RoutingWeights },
		},
		{
			name:     "cross protocol fallback toggle",
			env:      map[string]string{"DISABLE_CROSS_PROTOCOL_FALLBACK": "true"},
			settings: map[string]string{"disable_cross_protocol_fallback": `true`},
			get:      func(_ *config.Config, rt *config.RuntimeSettings) any { return rt.DisableCrossProtocolFallback },
		},
		{
			name:     "empty content fail toggle",
			env:      map[string]string{"PROXY_EMPTY_CONTENT_FAIL": "true"},
			settings: map[string]string{"proxy_empty_content_fail_enabled": `true`},
			get:      func(_ *config.Config, rt *config.RuntimeSettings) any { return rt.ProxyEmptyContentFailEnabled },
		},
		{
			name:     "debug capture headers toggle",
			env:      map[string]string{"PROXY_DEBUG_CAPTURE_HEADERS": "false"},
			settings: map[string]string{"proxy_debug_capture_headers": `false`},
			get:      func(_ *config.Config, rt *config.RuntimeSettings) any { return rt.ProxyDebugCaptureHeaders },
		},
		{
			name:     "debug target model trims",
			env:      map[string]string{"PROXY_DEBUG_TARGET_MODEL": "  gpt-5  "},
			settings: map[string]string{"proxy_debug_target_model": `"  gpt-5  "`},
			get:      func(_ *config.Config, rt *config.RuntimeSettings) any { return rt.ProxyDebugTargetModel },
		},
		{
			// Both paths decode the same cell text. An unreadable value must
			// leave the settings path on what env resolved (nil here) instead
			// of inventing a value of its own, and a readable one must produce
			// an identical structure.
			name:     "payload rules unparsable",
			env:      map[string]string{"PAYLOAD_RULES": "[{not json"},
			settings: map[string]string{"payload_rules": "[{not json"},
			get:      func(_ *config.Config, rt *config.RuntimeSettings) any { return rt.PayloadRules },
		},
		{
			name:     "payload rules object",
			env:      map[string]string{"PAYLOAD_RULES": `{"gpt-4o":{"stream":false}}`},
			settings: map[string]string{"payload_rules": `{"gpt-4o":{"stream":false}}`},
			get:      func(_ *config.Config, rt *config.RuntimeSettings) any { return rt.PayloadRules },
		},
		{
			name:     "service tier rules unparsable",
			env:      map[string]string{"OPENAI_SERVICE_TIER_RULES": "[{not json"},
			settings: map[string]string{"openai_service_tier_rules": "[{not json"},
			get:      func(cfg *config.Config, _ *config.RuntimeSettings) any { return cfg.OpenAiServiceTierRules },
		},
		{
			name:     "service tier rules object",
			env:      map[string]string{"OPENAI_SERVICE_TIER_RULES": `{"gpt-5":"priority"}`},
			settings: map[string]string{"openai_service_tier_rules": `{"gpt-5":"priority"}`},
			get:      func(cfg *config.Config, _ *config.RuntimeSettings) any { return cfg.OpenAiServiceTierRules },
		},
		{
			// CHECKIN_SCHEDULE_MODE is a raw word in env and a JSON string in
			// the settings table; both resolve through the same three-value
			// enum, and an unknown mode keeps the "cron" fallback on both paths
			// instead of reaching the snapshot, where RuntimeSettings.Validate
			// would turn it into a critical boot error.
			name:     "checkin schedule mode unknown",
			env:      map[string]string{"CHECKIN_SCHEDULE_MODE": "every-5-minutes"},
			settings: map[string]string{"checkin_schedule_mode": `"every-5-minutes"`},
			get:      func(_ *config.Config, rt *config.RuntimeSettings) any { return rt.CheckinScheduleMode },
		},
		{
			name:     "checkin schedule mode window",
			env:      map[string]string{"CHECKIN_SCHEDULE_MODE": "window"},
			settings: map[string]string{"checkin_schedule_mode": `"window"`},
			get:      func(_ *config.Config, rt *config.RuntimeSettings) any { return rt.CheckinScheduleMode },
		},
		{
			// The destructive half of the contract, and the only parity shape
			// that catches it: the settings path is seeded with the same env,
			// so this compares "garbage row on top of a configured value" with
			// "the configured value alone". Assigning the parse result directly
			// used to make the row win with nil.
			name:        "payload rules garbage row keeps the env value",
			env:         map[string]string{"PAYLOAD_RULES": `{"gpt-4o":{"stream":false}}`},
			settingsEnv: map[string]string{"PAYLOAD_RULES": `{"gpt-4o":{"stream":false}}`},
			settings:    map[string]string{"payload_rules": "[{not json"},
			get:         func(_ *config.Config, rt *config.RuntimeSettings) any { return rt.PayloadRules },
		},
		{
			name:        "service tier rules garbage row keeps the env value",
			env:         map[string]string{"OPENAI_SERVICE_TIER_RULES": `{"gpt-5":"priority"}`},
			settingsEnv: map[string]string{"OPENAI_SERVICE_TIER_RULES": `{"gpt-5":"priority"}`},
			settings:    map[string]string{"openai_service_tier_rules": "[{not json"},
			get:         func(cfg *config.Config, _ *config.RuntimeSettings) any { return cfg.OpenAiServiceTierRules },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			envCfg, envRt := config.Load(quietSecretEnv(tc.env))
			setCfg, setRt := config.Load(quietSecretEnv(tc.settingsEnv))
			ApplyRuntimeSettings(setCfg, setRt, tc.settings)

			want, got := tc.get(envCfg, envRt), tc.get(setCfg, setRt)
			if !reflect.DeepEqual(want, got) {
				t.Fatalf("env path resolved %#v but the settings path resolved %#v", want, got)
			}
		})
	}
}

// TestApplyRuntimeSettingsClampsCheckinIntervalHours pins the absolute
// post-hydration value rather than only its parity with config.Load, so the
// clamp cannot be replaced by a range check that silently keeps the previous
// value. A persisted 30 used to leave the process on the env-resolved 6:
// GET /api/settings/runtime echoed 6, the settings row still said 30, and no
// log line explained the gap. Reachable through a backup import
// (checkin_interval_hours is not in backup.RuntimeLocalSettingKeys, and the
// import validates cell types only), a hand-edited row, or a row left behind
// by an older version — the admin write path itself rejects out-of-range
// values before persisting.
func TestApplyRuntimeSettingsClampsCheckinIntervalHours(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  int
	}{
		{"above the ceiling clamps to 24", `30`, 24},
		{"far above the ceiling clamps to 24", `1000`, 24},
		{"zero clamps to 1", `0`, 1},
		{"negative clamps to 1", `-5`, 1},
		{"fraction truncates like the env path", `24.9`, 24},
		{"lower bound passes through", `1`, 1},
		{"in-range value passes through", `12`, 12},
		{"upper bound passes through", `24`, 24},
		{"unparsable keeps the resolved value", `"nonsense"`, config.DefaultCheckinIntervalHours},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := &config.RuntimeSettings{CheckinIntervalHours: config.DefaultCheckinIntervalHours}
			ApplyRuntimeSettings(&config.Config{}, rt, map[string]string{
				"checkin_interval_hours": tc.value,
			})
			if rt.CheckinIntervalHours != tc.want {
				t.Fatalf("persisted checkin_interval_hours = %s rehydrated to %d, want %d",
					tc.value, rt.CheckinIntervalHours, tc.want)
			}
		})
	}
}

// ---- Unknown keys are reported, not silently skipped ----

// captureSettingsLogs routes slog output into a buffer for the duration of the
// test so a startup log line can be asserted on instead of eyeballed.
func captureSettingsLogs(t *testing.T, level slog.Level) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: level})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return &buf
}

// settingsLogLine returns the single line carrying msg, failing when the count
// is not exactly one (a duplicated or missing startup line is the defect).
func settingsLogLine(t *testing.T, output, msg string) string {
	t.Helper()
	var matches []string
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, msg) {
			matches = append(matches, line)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("expected exactly one %q line, got %d in:\n%s", msg, len(matches), output)
	}
	return matches[0]
}

func TestApplyRuntimeSettingsWarnsAboutKeysNothingReadsBack(t *testing.T) {
	buf := captureSettingsLogs(t, slog.LevelWarn)

	ApplyRuntimeSettings(&config.Config{}, &config.RuntimeSettings{}, map[string]string{
		"zeta_unknown":      `1`,
		"alpha_unknown":     `2`,
		"db_type":           `"postgres"`,   // allowlisted: resolved before hydration
		"home_page_content": `"legacy row"`, // allowlisted: setting was removed
	})

	out := buf.String()
	line := settingsLogLine(t, out, "persisted keys not applied at startup hydration")
	if !strings.Contains(line, "count=2") {
		t.Errorf("warning does not report the key count:\n%s", out)
	}
	if !strings.Contains(out, "keys=alpha_unknown,zeta_unknown") {
		t.Errorf("warning does not list the unknown keys sorted:\n%s", out)
	}
	for _, allowlisted := range []string{"db_type", "home_page_content"} {
		if strings.Contains(out, allowlisted) {
			t.Errorf("allowlisted key %q must not be reported as unknown:\n%s", allowlisted, out)
		}
	}
}

func TestTruncateKeyListCapsLongLists(t *testing.T) {
	keys := make([]string, 0, unknownKeyLogLimit+5)
	for i := 0; i < unknownKeyLogLimit+5; i++ {
		keys = append(keys, "key")
	}
	got := truncateKeyList(keys, unknownKeyLogLimit)
	if len(got) != unknownKeyLogLimit+1 {
		t.Fatalf("len = %d, want %d", len(got), unknownKeyLogLimit+1)
	}
	if got[len(got)-1] != "(+5 more)" {
		t.Fatalf("marker = %q, want (+5 more)", got[len(got)-1])
	}
	if short := truncateKeyList([]string{"a", "b"}, unknownKeyLogLimit); len(short) != 2 {
		t.Fatalf("short list changed: %#v", short)
	}
}

// ---- Log cleanup: one key namespace, no inference over explicit intent ----

func TestHasExplicitLogCleanupSettingsReadsTheKeysTheAdminApiWrites(t *testing.T) {
	for _, key := range []string{
		"log_cleanup_usage_logs_enabled",
		"log_cleanup_program_logs_enabled",
		"log_cleanup_retention_days",
	} {
		if !HasExplicitLogCleanupSettings(map[string]string{key: `false`}) {
			t.Errorf("%s: HasExplicitLogCleanupSettings = false, want true", key)
		}
	}
	// Deprecated dotted spellings still count during the compatibility window.
	if !HasExplicitLogCleanupSettings(map[string]string{"log_cleanup.retention_days": `7`}) {
		t.Error("dotted legacy key: HasExplicitLogCleanupSettings = false, want true")
	}
}

// Saving an unrelated setting must not switch the log retention regime.
func TestHasExplicitLogCleanupSettingsIgnoresUnrelatedKeys(t *testing.T) {
	if HasExplicitLogCleanupSettings(map[string]string{
		"system_name":         `"Ops Gateway"`,
		"log_cleanup_cron":    `"30 4 * * *"`,
		"monitor_ldoh_cookie": `"ld_auth_session=x"`,
	}) {
		t.Fatal("HasExplicitLogCleanupSettings = true for unrelated settings")
	}
	if HasExplicitLogCleanupSettings(map[string]string{}) {
		t.Fatal("HasExplicitLogCleanupSettings = true for an empty settings table")
	}
}

// The reported defect end to end: LOG_CLEANUP_*_ENABLED=false in env used to be
// flipped to true at boot as soon as the settings table held any row at all,
// because "configured" was inferred from retention > 0 and config.Load floors
// retention at 1 day. An explicit false and an unset variable are the same
// case: neither asks for the regime, so the legacy pruner keeps proxy_logs.
func TestLoadRuntimeSettingsKeepsEnvDisabledLogCleanup(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
	}{
		{
			name: "explicit false",
			env: map[string]string{
				"LOG_CLEANUP_USAGE_LOGS_ENABLED":   "false",
				"LOG_CLEANUP_PROGRAM_LOGS_ENABLED": "false",
			},
		},
		{name: "unset", env: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			settings := hydrateTestDB(t)
			if err := settings.Set("system_name", `"Ops Gateway"`); err != nil {
				t.Fatalf("persist unrelated setting: %v", err)
			}

			env := map[string]string{"SYSTEM_NAME": "Metapi"}
			for key, value := range tc.env {
				env[key] = value
			}
			cfg, rt := config.Load(quietSecretEnv(env))
			if rt.LogCleanupUsageLogsEnabled || rt.LogCleanupProgramLogsEnabled {
				t.Fatal("precondition failed: env already enabled log cleanup")
			}
			if err := LoadRuntimeSettings(cfg, rt); err != nil {
				t.Fatalf("LoadRuntimeSettings: %v", err)
			}

			if rt.LogCleanupUsageLogsEnabled || rt.LogCleanupProgramLogsEnabled {
				t.Fatalf("boot flipped a disabled toggle to true (usage=%v program=%v)",
					rt.LogCleanupUsageLogsEnabled, rt.LogCleanupProgramLogsEnabled)
			}
			if cfg.LogCleanupConfigured {
				t.Fatal("an unrelated setting switched the log retention regime (LogCleanupConfigured = true)")
			}
			if rt.SystemName != "Ops Gateway" {
				t.Fatalf("SystemName = %q, want the persisted value", rt.SystemName)
			}
		})
	}
}

// The mirror case: an env-driven deployment has no admin-saved log-cleanup
// rows, and its explicit true must still claim the regime — otherwise the
// cleanup job skips every run ("legacy fallback mode active") and the legacy
// pruner is disabled by nothing, so the documented env toggle does nothing.
func TestLoadRuntimeSettingsEnvToggleClaimsRegimeWithoutAdminRows(t *testing.T) {
	cases := []struct {
		name       string
		env        map[string]string
		wantUsage  bool
		wantProg   bool
		wantSource string
	}{
		{
			name:       "usage toggle",
			env:        map[string]string{"LOG_CLEANUP_USAGE_LOGS_ENABLED": "true"},
			wantUsage:  true,
			wantProg:   false,
			wantSource: "env_toggle",
		},
		{
			name:       "program toggle",
			env:        map[string]string{"LOG_CLEANUP_PROGRAM_LOGS_ENABLED": "true"},
			wantUsage:  false,
			wantProg:   true,
			wantSource: "env_toggle",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logs := captureSettingsLogs(t, slog.LevelInfo)
			settings := hydrateTestDB(t)
			// Only unrelated rows: nothing in the table mentions log cleanup.
			if err := settings.Set("system_name", `"Ops Gateway"`); err != nil {
				t.Fatalf("persist unrelated setting: %v", err)
			}

			cfg, rt := config.Load(quietSecretEnv(tc.env))
			if err := LoadRuntimeSettings(cfg, rt); err != nil {
				t.Fatalf("LoadRuntimeSettings: %v", err)
			}

			if !cfg.LogCleanupConfigured {
				t.Fatal("LogCleanupConfigured = false, want the env toggle to claim the regime")
			}
			if !cfg.LogCleanupEnvEnabled {
				t.Fatal("LogCleanupEnvEnabled = false, want the explicit env true recorded")
			}
			if rt.LogCleanupUsageLogsEnabled != tc.wantUsage || rt.LogCleanupProgramLogsEnabled != tc.wantProg {
				t.Fatalf("toggles = (%v, %v), want (%v, %v) — hydration must not drop the env intent",
					rt.LogCleanupUsageLogsEnabled, rt.LogCleanupProgramLogsEnabled, tc.wantUsage, tc.wantProg)
			}
			line := settingsLogLine(t, logs.String(), "settings: log retention regime")
			if !strings.Contains(line, "source="+tc.wantSource) {
				t.Errorf("regime line does not report source=%s:\n%s", tc.wantSource, line)
			}
		})
	}
}

// Admin-saved settings keep precedence for the values: env can claim the
// regime, but a persisted false is what the operator last said, so the cleanup
// job runs and skips for want of a target instead of pruning against env.
func TestLoadRuntimeSettingsDbExplicitWinsOverEnvToggle(t *testing.T) {
	settings := hydrateTestDB(t)
	if err := settings.Set("log_cleanup_usage_logs_enabled", "false"); err != nil {
		t.Fatalf("persist log cleanup toggle: %v", err)
	}
	if err := settings.Set("log_cleanup_retention_days", "9"); err != nil {
		t.Fatalf("persist log cleanup retention: %v", err)
	}

	cfg, rt := config.Load(quietSecretEnv(map[string]string{
		"LOG_CLEANUP_USAGE_LOGS_ENABLED": "true",
		"LOG_CLEANUP_RETENTION_DAYS":     "30",
	}))
	if err := LoadRuntimeSettings(cfg, rt); err != nil {
		t.Fatalf("LoadRuntimeSettings: %v", err)
	}

	if !cfg.LogCleanupConfigured {
		t.Error("LogCleanupConfigured = false, want true (both env and DB claim the regime)")
	}
	if rt.LogCleanupUsageLogsEnabled {
		t.Error("LogCleanupUsageLogsEnabled = true, want the persisted false to win over env")
	}
	if rt.LogCleanupRetentionDays != 9 {
		t.Errorf("LogCleanupRetentionDays = %d, want the persisted 9", rt.LogCleanupRetentionDays)
	}
}

// Retention and cron alone are not intent: the toggles default to false, so
// "configured" would mean the new scheduler runs, skips for want of a target,
// and the legacy PROXY_LOG_RETENTION_DAYS pruner is disabled — nothing would
// ever be pruned.
func TestLoadRuntimeSettingsEnvRetentionAloneDoesNotClaimRegime(t *testing.T) {
	settings := hydrateTestDB(t)
	if err := settings.Set("system_name", `"Ops Gateway"`); err != nil {
		t.Fatalf("persist unrelated setting: %v", err)
	}

	cfg, rt := config.Load(quietSecretEnv(map[string]string{
		"LOG_CLEANUP_RETENTION_DAYS": "7",
		"LOG_CLEANUP_CRON":           "30 4 * * *",
	}))
	if cfg.LogCleanupEnvEnabled {
		t.Fatal("LogCleanupEnvEnabled = true for retention/cron only")
	}
	if err := LoadRuntimeSettings(cfg, rt); err != nil {
		t.Fatalf("LoadRuntimeSettings: %v", err)
	}
	if cfg.LogCleanupConfigured {
		t.Fatal("LogCleanupConfigured = true, want retention/cron alone to leave the legacy pruner in charge")
	}
	if rt.LogCleanupRetentionDays != 7 || rt.LogCleanupCron != "30 4 * * *" {
		t.Fatalf("retention/cron = (%d, %q), want the env values carried through",
			rt.LogCleanupRetentionDays, rt.LogCleanupCron)
	}
}

// Exactly one startup line names the winning regime, so an operator upgrading
// past the removed auto-enable inference can see what now owns the log tables.
func TestLoadRuntimeSettingsLogsTheWinningLogRetentionRegime(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		rows map[string]string
		want []string
	}{
		{
			name: "admin-saved settings",
			rows: map[string]string{
				"log_cleanup_usage_logs_enabled": "true",
				"log_cleanup_retention_days":     "9",
				"log_cleanup_cron":               `"30 4 * * *"`,
			},
			want: []string{
				"regime=log_cleanup", "configured=true", "source=db_settings",
				"usage_logs_enabled=true", "program_logs_enabled=false", "retention_days=9",
			},
		},
		{
			name: "env toggle",
			env:  map[string]string{"LOG_CLEANUP_PROGRAM_LOGS_ENABLED": "true"},
			rows: map[string]string{"system_name": `"Ops Gateway"`},
			want: []string{
				"regime=log_cleanup", "configured=true", "source=env_toggle",
				"usage_logs_enabled=false", "program_logs_enabled=true",
			},
		},
		{
			name: "no intent anywhere",
			rows: map[string]string{"system_name": `"Ops Gateway"`},
			want: []string{
				"regime=legacy_fallback", "configured=false", "source=none",
				"usage_logs_enabled=false", "program_logs_enabled=false", "retention_days=30",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logs := captureSettingsLogs(t, slog.LevelInfo)
			settings := hydrateTestDB(t)
			for key, value := range tc.rows {
				if err := settings.Set(key, value); err != nil {
					t.Fatalf("persist %s: %v", key, err)
				}
			}
			cfg, rt := config.Load(quietSecretEnv(tc.env))
			if err := LoadRuntimeSettings(cfg, rt); err != nil {
				t.Fatalf("LoadRuntimeSettings: %v", err)
			}
			line := settingsLogLine(t, logs.String(), "settings: log retention regime")
			for _, want := range tc.want {
				if !strings.Contains(line, want) {
					t.Errorf("regime line is missing %q:\n%s", want, line)
				}
			}
		})
	}
}

// The same boot path with the log-cleanup keys actually persisted: the values
// win over env, and the log_cleanup regime owns retention.
func TestLoadRuntimeSettingsHydratesPersistedLogCleanup(t *testing.T) {
	settings := hydrateTestDB(t)
	// Exactly the encoding handler/admin/settings_apply.go persists.
	for key, value := range map[string]string{
		"log_cleanup_usage_logs_enabled":   "true",
		"log_cleanup_program_logs_enabled": "false",
		"log_cleanup_retention_days":       "9",
		"log_cleanup_cron":                 `"30 4 * * *"`,
	} {
		if err := settings.Set(key, value); err != nil {
			t.Fatalf("persist %s: %v", key, err)
		}
	}

	cfg, rt := config.Load(quietSecretEnv(map[string]string{
		"LOG_CLEANUP_USAGE_LOGS_ENABLED": "false",
		"LOG_CLEANUP_RETENTION_DAYS":     "30",
		"LOG_CLEANUP_CRON":               "0 6 * * *",
	}))
	if err := LoadRuntimeSettings(cfg, rt); err != nil {
		t.Fatalf("LoadRuntimeSettings: %v", err)
	}

	if !rt.LogCleanupUsageLogsEnabled {
		t.Error("LogCleanupUsageLogsEnabled = false, want the persisted true")
	}
	if rt.LogCleanupProgramLogsEnabled {
		t.Error("LogCleanupProgramLogsEnabled = true, want the persisted false")
	}
	if rt.LogCleanupRetentionDays != 9 {
		t.Errorf("LogCleanupRetentionDays = %d, want 9", rt.LogCleanupRetentionDays)
	}
	if rt.LogCleanupCron != "30 4 * * *" {
		t.Errorf("LogCleanupCron = %q, want the persisted cron", rt.LogCleanupCron)
	}
	if !cfg.LogCleanupConfigured {
		t.Error("LogCleanupConfigured = false, want true once log cleanup is explicitly configured")
	}
}

// ---- Empty-value semantics: one rule for both write paths ----

// The reported defect end to end: clearing the branding in the admin UI stuck
// until the next restart, which brought the env value back.
func TestLoadRuntimeSettingsHonorsExplicitlyClearedBranding(t *testing.T) {
	settings := hydrateTestDB(t)
	for key, value := range map[string]string{
		"system_name":    `""`,
		"logo":           `""`,
		"footer":         `""`,
		"about":          `""`,
		"server_address": `""`,
	} {
		if err := settings.Set(key, value); err != nil {
			t.Fatalf("persist %s: %v", key, err)
		}
	}

	cfg, rt := config.Load(quietSecretEnv(map[string]string{
		"SYSTEM_NAME":    "Metapi",
		"LOGO":           "https://cdn.example/env-logo.png",
		"FOOTER":         "env footer",
		"ABOUT":          "env about",
		"SERVER_ADDRESS": "https://env.example.com",
	}))
	if err := LoadRuntimeSettings(cfg, rt); err != nil {
		t.Fatalf("LoadRuntimeSettings: %v", err)
	}

	for name, got := range map[string]string{
		"SystemName":    rt.SystemName,
		"Logo":          rt.Logo,
		"Footer":        rt.Footer,
		"About":         rt.About,
		"ServerAddress": rt.ServerAddress,
	} {
		if got != "" {
			t.Errorf("%s = %q, want the explicitly cleared empty value", name, got)
		}
	}
}

// Credentials are the deliberate exception: a blank persisted value must not
// replace a configured token, or a restart would silently downgrade the
// deployment to the built-in default.
func TestApplyRuntimeSettingsBlankCredentialsDoNotOverrideConfigured(t *testing.T) {
	cfg := &config.Config{AccountCredentialSecret: "configured-secret"}
	rt := &config.RuntimeSettings{AuthToken: "configured-auth-token", ProxyToken: "sk-configured"}
	ApplyRuntimeSettings(cfg, rt, map[string]string{
		"auth_token":                `""`,
		"proxy_token":               `""`,
		"account_credential_secret": `""`,
	})
	if rt.AuthToken != "configured-auth-token" {
		t.Errorf("AuthToken = %q, want the configured token preserved", rt.AuthToken)
	}
	if rt.ProxyToken != "sk-configured" {
		t.Errorf("ProxyToken = %q, want the configured token preserved", rt.ProxyToken)
	}
	if cfg.AccountCredentialSecret != "configured-secret" {
		t.Errorf("AccountCredentialSecret = %q, want the configured secret preserved", cfg.AccountCredentialSecret)
	}
}

// Settings rows are JSON-encoded, so a persisted token must be decoded like
// every other string setting. Reading the raw row used to rehydrate the token
// with its surrounding quotes, which broke admin/proxy auth after a restart
// following a token rotation through the API.
func TestApplyRuntimeSettingsDecodesPersistedCredentials(t *testing.T) {
	cfg := &config.Config{}
	rt := &config.RuntimeSettings{}
	ApplyRuntimeSettings(cfg, rt, map[string]string{
		"auth_token":                `"rotated-admin-token"`,
		"proxy_token":               `"sk-rotated"`,
		"account_credential_secret": `"rotated-secret"`,
	})
	if rt.AuthToken != "rotated-admin-token" {
		t.Errorf("AuthToken = %q, want the decoded token", rt.AuthToken)
	}
	if rt.ProxyToken != "sk-rotated" {
		t.Errorf("ProxyToken = %q, want the decoded token", rt.ProxyToken)
	}
	if cfg.AccountCredentialSecret != "rotated-secret" {
		t.Errorf("AccountCredentialSecret = %q, want the decoded secret", cfg.AccountCredentialSecret)
	}
}

// A legacy row written without JSON encoding still hydrates (the decoder falls
// back to the raw value), so the fix above cannot strand existing databases.
func TestApplyRuntimeSettingsAcceptsLegacyUnencodedCredentials(t *testing.T) {
	rt := &config.RuntimeSettings{}
	ApplyRuntimeSettings(&config.Config{}, rt, map[string]string{
		"proxy_token": `sk-legacy-plain`,
	})
	if rt.ProxyToken != "sk-legacy-plain" {
		t.Fatalf("ProxyToken = %q, want the legacy plain value", rt.ProxyToken)
	}
}

// An empty settings table must leave the resolved config untouched.
func TestLoadRuntimeSettingsEmptyTableIsANoOp(t *testing.T) {
	hydrateTestDB(t)
	cfg, rt := config.Load(quietSecretEnv(map[string]string{"SYSTEM_NAME": "Metapi"}))
	if err := LoadRuntimeSettings(cfg, rt); err != nil {
		t.Fatalf("LoadRuntimeSettings: %v", err)
	}
	if rt.SystemName != "Metapi" {
		t.Errorf("SystemName = %q, want the env value", rt.SystemName)
	}
	if cfg.LogCleanupConfigured {
		t.Error("LogCleanupConfigured = true for an empty settings table")
	}
}
