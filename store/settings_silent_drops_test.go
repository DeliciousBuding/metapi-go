package store

import (
	"log/slog"
	"reflect"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
)

// Settings hydration must not translate "this row cannot be read" into "the
// operator asked for the zero value".
//
// The destructive instance of that mistake was payload_rules /
// openai_service_tier_rules: both assigned config.ParseJsonValue(value)
// directly, and ParseJsonValue returns nil for an unparseable cell, so a
// single bad row — planted by a backup import, a hand edit, or left behind by
// an older version — emptied a configured rule set on the next restart with
// nothing in the log. The silent (non-destructive) instance was
// checkin_schedule_mode and notify_task_toggles, whose guards dropped an
// unreadable row without saying so: the settings table and
// GET /api/settings/runtime then disagreed forever with no line to explain
// the gap.
//
// The contract asserted here for every key that stores JSON:
//
//	C1 an unreadable row keeps the value hydration already resolved, and the
//	   process says so at WARN with the key, the reason and what it kept;
//	C2 a readable row is applied — including the explicit clears (JSON null,
//	   the empty array the admin write path persists when the UI clears the
//	   field), which are intent and must stay destructive;
//	C3 the settings path and the env path reach the same conclusion for the
//	   same cell text (parity table in settings_roundtrip_test.go).

// settingsWarnForKey returns the single WARN line naming key, failing when the
// count is not exactly one: a missing line is the silent drop, a duplicated
// one means the branch fired twice for a single row.
func settingsWarnForKey(t *testing.T, output, key string) string {
	t.Helper()
	var matches []string
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "level=WARN") && strings.Contains(line, key) {
			matches = append(matches, line)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("expected exactly one WARN line naming %q, got %d in:\n%s", key, len(matches), output)
	}
	return matches[0]
}

// settingsNoWarnForKey fails when hydration warned about a row it could read.
func settingsNoWarnForKey(t *testing.T, output, key string) {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "level=WARN") && strings.Contains(line, key) {
			t.Fatalf("a readable row must not warn, got:\n%s", output)
		}
	}
}

// ---- C1: unreadable JSON rule rows keep the configured rules ----

// unreadableJSONRows are cells that survive the settings-table write paths but
// carry no readable intent: a truncated export, a value column planted by the
// tables-path backup import (validateBackupImportCellValue checks only the
// scalar type and the byte limit), or a hand edit.
var unreadableJSONRows = []string{
	`[{not json`,
	`not json at all`,
	`{"match":}`,
	`{"a":1,}`,
	`[1,2`,
	`{"rules":[1,2,]}`,
}

func TestApplyRuntimeSettingsUnreadablePayloadRulesRowKeepsConfiguredRules(t *testing.T) {
	configured := map[string]any{"gpt-4o": map[string]any{"stream": false}}
	for _, row := range unreadableJSONRows {
		t.Run(strings.ReplaceAll(row, " ", "_"), func(t *testing.T) {
			rt := &config.RuntimeSettings{PayloadRules: configured}
			buf := captureSettingsLogs(t, slog.LevelWarn)
			ApplyRuntimeSettings(&config.Config{}, rt, map[string]string{"payload_rules": row})

			if rt.PayloadRules == nil {
				t.Fatalf("row %s wiped the configured payload rules to nil", row)
			}
			if !reflect.DeepEqual(rt.PayloadRules, configured) {
				t.Fatalf("row %s changed the payload rules to %#v, want the configured %#v",
					row, rt.PayloadRules, configured)
			}
			line := settingsWarnForKey(t, buf.String(), "payload_rules")
			if !strings.Contains(line, "reason=") || !strings.Contains(line, "kept=") {
				t.Fatalf("warning must carry the reason and the kept value: %s", line)
			}
		})
	}
}

func TestApplyRuntimeSettingsUnreadableServiceTierRulesRowKeepsConfiguredRules(t *testing.T) {
	configured := map[string]any{"gpt-5": "priority"}
	for _, row := range unreadableJSONRows {
		t.Run(strings.ReplaceAll(row, " ", "_"), func(t *testing.T) {
			cfg := &config.Config{OpenAiServiceTierRules: configured}
			buf := captureSettingsLogs(t, slog.LevelWarn)
			ApplyRuntimeSettings(cfg, &config.RuntimeSettings{}, map[string]string{"openai_service_tier_rules": row})

			if cfg.OpenAiServiceTierRules == nil {
				t.Fatalf("row %s wiped the configured service tier rules to nil", row)
			}
			if !reflect.DeepEqual(cfg.OpenAiServiceTierRules, configured) {
				t.Fatalf("row %s changed the service tier rules to %#v, want the configured %#v",
					row, cfg.OpenAiServiceTierRules, configured)
			}
			line := settingsWarnForKey(t, buf.String(), "openai_service_tier_rules")
			if !strings.Contains(line, "reason=") || !strings.Contains(line, "kept=") {
				t.Fatalf("warning must carry the reason and the kept value: %s", line)
			}
		})
	}
}

// ---- C2: readable rows are applied, explicit clears stay destructive ----

func TestApplyRuntimeSettingsAppliesReadablePayloadRulesRow(t *testing.T) {
	cases := []struct {
		name string
		row  string
		want any
	}{
		{"object", `{"gpt-4o":{"stream":false}}`, map[string]any{"gpt-4o": map[string]any{"stream": false}}},
		{"array", `[{"match":"gpt-4o"}]`, []any{map[string]any{"match": "gpt-4o"}}},
		{"explicit null clears", `null`, nil},
		// upsertSettingDB normalizes a nil body value to []string{}, so this is
		// what the admin write path persists when the UI clears the textarea.
		{"explicit empty array clears", `[]`, []any{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := &config.RuntimeSettings{PayloadRules: map[string]any{"stale": true}}
			buf := captureSettingsLogs(t, slog.LevelWarn)
			ApplyRuntimeSettings(&config.Config{}, rt, map[string]string{"payload_rules": tc.row})
			if !reflect.DeepEqual(rt.PayloadRules, tc.want) {
				t.Fatalf("row %s rehydrated to %#v, want %#v", tc.row, rt.PayloadRules, tc.want)
			}
			settingsNoWarnForKey(t, buf.String(), "payload_rules")
		})
	}
}

func TestApplyRuntimeSettingsAppliesReadableServiceTierRulesRow(t *testing.T) {
	cases := []struct {
		name string
		row  string
		want any
	}{
		{"object", `{"gpt-5":"priority"}`, map[string]any{"gpt-5": "priority"}},
		{"explicit null clears", `null`, nil},
		{"explicit empty object clears", `{}`, map[string]any{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{OpenAiServiceTierRules: map[string]any{"stale": "flex"}}
			buf := captureSettingsLogs(t, slog.LevelWarn)
			ApplyRuntimeSettings(cfg, &config.RuntimeSettings{}, map[string]string{"openai_service_tier_rules": tc.row})
			if !reflect.DeepEqual(cfg.OpenAiServiceTierRules, tc.want) {
				t.Fatalf("row %s rehydrated to %#v, want %#v", tc.row, cfg.OpenAiServiceTierRules, tc.want)
			}
			settingsNoWarnForKey(t, buf.String(), "openai_service_tier_rules")
		})
	}
}

// ---- C1/C2: checkin_schedule_mode ----

func TestApplyRuntimeSettingsUnreadableCheckinScheduleModeRowKeepsResolvedMode(t *testing.T) {
	for _, row := range []string{`"every-5-minutes"`, `every-5-minutes`, `"CRON_JOB"`, `123`, `null`, `""`} {
		t.Run(row, func(t *testing.T) {
			rt := &config.RuntimeSettings{CheckinScheduleMode: "window", CheckinWindowStart: "01:00", CheckinWindowEnd: "05:00"}
			buf := captureSettingsLogs(t, slog.LevelWarn)
			ApplyRuntimeSettings(&config.Config{}, rt, map[string]string{"checkin_schedule_mode": row})

			if rt.CheckinScheduleMode != "window" {
				t.Fatalf("row %s changed the schedule mode to %q, want the resolved %q",
					row, rt.CheckinScheduleMode, "window")
			}
			// config/validate.go:384 treats an unknown mode as a critical boot
			// error, so the kept value must stay one Validate accepts.
			for _, err := range rt.Validate() {
				if strings.Contains(err.Error(), "checkin_schedule_mode") {
					t.Fatalf("hydration left a schedule mode that fails Validate: %v", err)
				}
			}
			line := settingsWarnForKey(t, buf.String(), "checkin_schedule_mode")
			if !strings.Contains(line, "reason=") || !strings.Contains(line, "kept=") {
				t.Fatalf("warning must carry the reason and the kept value: %s", line)
			}
		})
	}
}

func TestApplyRuntimeSettingsAppliesReadableCheckinScheduleModes(t *testing.T) {
	for row, want := range map[string]string{
		`"interval"`: "interval",
		`"window"`:   "window",
		`"cron"`:     "cron",
		`"WINDOW"`:   "window",
		`window`:     "window", // legacy unencoded cell
	} {
		t.Run(row, func(t *testing.T) {
			rt := &config.RuntimeSettings{CheckinScheduleMode: "cron"}
			buf := captureSettingsLogs(t, slog.LevelWarn)
			ApplyRuntimeSettings(&config.Config{}, rt, map[string]string{"checkin_schedule_mode": row})
			if rt.CheckinScheduleMode != want {
				t.Fatalf("row %s rehydrated to %q, want %q", row, rt.CheckinScheduleMode, want)
			}
			settingsNoWarnForKey(t, buf.String(), "checkin_schedule_mode")
		})
	}
}

// ---- C1/C2: notify_task_toggles ----

// service/notify gates each alert type on this map and treats a missing key as
// enabled, so a dropped row unmutes every alert type the operator had muted.
func TestApplyRuntimeSettingsUnreadableNotifyTaskTogglesRowKeepsMutes(t *testing.T) {
	muted := map[string]bool{"low_balance": false, "token_expired": false}
	for _, row := range []string{`{"low_balance":"no"}`, `[1,2]`, `"all"`, `{"low_balance":`, `123`, `{"a":1}`} {
		t.Run(strings.ReplaceAll(row, " ", "_"), func(t *testing.T) {
			rt := &config.RuntimeSettings{NotifyTaskToggles: map[string]bool{"low_balance": false, "token_expired": false}}
			buf := captureSettingsLogs(t, slog.LevelWarn)
			ApplyRuntimeSettings(&config.Config{}, rt, map[string]string{"notify_task_toggles": row})

			if !reflect.DeepEqual(rt.NotifyTaskToggles, muted) {
				t.Fatalf("row %s changed the toggles to %#v, want the configured %#v", row, rt.NotifyTaskToggles, muted)
			}
			line := settingsWarnForKey(t, buf.String(), "notify_task_toggles")
			if !strings.Contains(line, "reason=") || !strings.Contains(line, "kept=") {
				t.Fatalf("warning must carry the reason and the kept value: %s", line)
			}
		})
	}
}

func TestApplyRuntimeSettingsAppliesReadableNotifyTaskToggles(t *testing.T) {
	t.Run("object merges", func(t *testing.T) {
		rt := &config.RuntimeSettings{NotifyTaskToggles: map[string]bool{"low_balance": false}}
		buf := captureSettingsLogs(t, slog.LevelWarn)
		ApplyRuntimeSettings(&config.Config{}, rt, map[string]string{
			"notify_task_toggles": `{"token_expired":false,"daily_summary":true}`,
		})
		want := map[string]bool{"low_balance": false, "token_expired": false, "daily_summary": true}
		if !reflect.DeepEqual(rt.NotifyTaskToggles, want) {
			t.Fatalf("NotifyTaskToggles = %#v, want %#v", rt.NotifyTaskToggles, want)
		}
		settingsNoWarnForKey(t, buf.String(), "notify_task_toggles")
	})

	t.Run("json null means no per-type overrides", func(t *testing.T) {
		rt := &config.RuntimeSettings{}
		buf := captureSettingsLogs(t, slog.LevelWarn)
		ApplyRuntimeSettings(&config.Config{}, rt, map[string]string{"notify_task_toggles": `null`})
		if rt.NotifyTaskToggles == nil || len(rt.NotifyTaskToggles) != 0 {
			t.Fatalf("NotifyTaskToggles = %#v, want an empty non-nil map", rt.NotifyTaskToggles)
		}
		settingsNoWarnForKey(t, buf.String(), "notify_task_toggles")
	})

	t.Run("empty object clears every mute it names", func(t *testing.T) {
		rt := &config.RuntimeSettings{NotifyTaskToggles: map[string]bool{"low_balance": false}}
		buf := captureSettingsLogs(t, slog.LevelWarn)
		ApplyRuntimeSettings(&config.Config{}, rt, map[string]string{"notify_task_toggles": `{}`})
		if !reflect.DeepEqual(rt.NotifyTaskToggles, map[string]bool{"low_balance": false}) {
			t.Fatalf("NotifyTaskToggles = %#v, want the merge semantics to keep unmentioned keys", rt.NotifyTaskToggles)
		}
		settingsNoWarnForKey(t, buf.String(), "notify_task_toggles")
	})
}

// ---- The whole family: an unreadable row never wipes a configured value ----

// TestApplyRuntimeSettingsUnreadableRowsNeverWipeConfiguredValues is the
// family gate. Every settings key that stores a list, an object or an enum has
// a non-empty value the process resolved before hydration; for each of them an
// unreadable row must leave that value untouched and must be reported. Add a
// row here when a new key of that shape is hydrated.
func TestApplyRuntimeSettingsUnreadableRowsNeverWipeConfiguredValues(t *testing.T) {
	cases := []struct {
		key      string
		row      string
		baseline func(*config.Config, *config.RuntimeSettings)
		get      func(*config.Config, *config.RuntimeSettings) any
	}{
		{
			key:      "admin_ip_allowlist",
			row:      `{"oops":true}`,
			baseline: func(_ *config.Config, rt *config.RuntimeSettings) { rt.AdminIpAllowlist = []string{"203.0.113.7"} },
			get:      func(_ *config.Config, rt *config.RuntimeSettings) any { return rt.AdminIpAllowlist },
		},
		{
			key:      "global_blocked_brands",
			row:      `{"oops":true}`,
			baseline: func(_ *config.Config, rt *config.RuntimeSettings) { rt.GlobalBlockedBrands = []string{"acme"} },
			get:      func(_ *config.Config, rt *config.RuntimeSettings) any { return rt.GlobalBlockedBrands },
		},
		{
			key:      "global_allowed_models",
			row:      `{"oops":true}`,
			baseline: func(_ *config.Config, rt *config.RuntimeSettings) { rt.GlobalAllowedModels = []string{"gpt-4o"} },
			get:      func(_ *config.Config, rt *config.RuntimeSettings) any { return rt.GlobalAllowedModels },
		},
		{
			key:      "proxy_error_keywords",
			row:      `{"oops":true}`,
			baseline: func(_ *config.Config, rt *config.RuntimeSettings) { rt.ProxyErrorKeywords = []string{"rate limit"} },
			get:      func(_ *config.Config, rt *config.RuntimeSettings) any { return rt.ProxyErrorKeywords },
		},
		{
			key: "routing_weights",
			row: `{"BaseWeightFactor":`,
			baseline: func(_ *config.Config, rt *config.RuntimeSettings) {
				rt.RoutingWeights = config.RoutingWeights{BaseWeightFactor: 0.7}
			},
			get: func(_ *config.Config, rt *config.RuntimeSettings) any { return rt.RoutingWeights },
		},
		{
			key: "payload_rules",
			row: `[{"match":`,
			baseline: func(_ *config.Config, rt *config.RuntimeSettings) {
				rt.PayloadRules = []any{map[string]any{"match": "gpt-4o"}}
			},
			get: func(_ *config.Config, rt *config.RuntimeSettings) any { return rt.PayloadRules },
		},
		{
			key: "openai_service_tier_rules",
			row: `[{"tier":`,
			baseline: func(cfg *config.Config, _ *config.RuntimeSettings) {
				cfg.OpenAiServiceTierRules = map[string]any{"gpt-5": "priority"}
			},
			get: func(cfg *config.Config, _ *config.RuntimeSettings) any { return cfg.OpenAiServiceTierRules },
		},
		{
			key: "notify_task_toggles",
			row: `{"low_balance":"no"}`,
			baseline: func(_ *config.Config, rt *config.RuntimeSettings) {
				rt.NotifyTaskToggles = map[string]bool{"low_balance": false}
			},
			get: func(_ *config.Config, rt *config.RuntimeSettings) any { return rt.NotifyTaskToggles },
		},
		{
			key:      "checkin_schedule_mode",
			row:      `"every-5-minutes"`,
			baseline: func(_ *config.Config, rt *config.RuntimeSettings) { rt.CheckinScheduleMode = "window" },
			get:      func(_ *config.Config, rt *config.RuntimeSettings) any { return rt.CheckinScheduleMode },
		},
		{
			key:      "checkin_cron",
			row:      `"not a cron"`,
			baseline: func(_ *config.Config, rt *config.RuntimeSettings) { rt.CheckinCron = config.DefaultCheckinCron },
			get:      func(_ *config.Config, rt *config.RuntimeSettings) any { return rt.CheckinCron },
		},
		{
			key: "balance_refresh_cron",
			row: `"not a cron"`,
			baseline: func(_ *config.Config, rt *config.RuntimeSettings) {
				rt.BalanceRefreshCron = config.DefaultBalanceRefreshCron
			},
			get: func(_ *config.Config, rt *config.RuntimeSettings) any { return rt.BalanceRefreshCron },
		},
		{
			key:      "model_sync_cron",
			row:      `"not a cron"`,
			baseline: func(_ *config.Config, rt *config.RuntimeSettings) { rt.ModelSyncCron = config.DefaultModelSyncCron },
			get:      func(_ *config.Config, rt *config.RuntimeSettings) any { return rt.ModelSyncCron },
		},
		{
			key:      "log_cleanup_cron",
			row:      `"not a cron"`,
			baseline: func(_ *config.Config, rt *config.RuntimeSettings) { rt.LogCleanupCron = config.DefaultLogCleanupCron },
			get:      func(_ *config.Config, rt *config.RuntimeSettings) any { return rt.LogCleanupCron },
		},
	}

	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			cfg, rt := &config.Config{}, &config.RuntimeSettings{}
			tc.baseline(cfg, rt)
			want := tc.get(cfg, rt)

			buf := captureSettingsLogs(t, slog.LevelWarn)
			ApplyRuntimeSettings(cfg, rt, map[string]string{tc.key: tc.row})

			if got := tc.get(cfg, rt); !reflect.DeepEqual(got, want) {
				t.Fatalf("row %s for %s changed the resolved value to %#v, want it kept as %#v",
					tc.row, tc.key, got, want)
			}
			if got := tc.get(cfg, rt); got == nil {
				t.Fatalf("row %s for %s wiped the resolved value to nil", tc.row, tc.key)
			}
			settingsWarnForKey(t, buf.String(), tc.key)
		})
	}
}
