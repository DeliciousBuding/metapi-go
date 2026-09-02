package store

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/deliciousbuding/metapi-go/config"
)

// LoadRuntimeSettings reads the settings table and applies runtime overrides
// to the config. Mirrors TS runtimeSettingsHydration behavior.
func LoadRuntimeSettings(cfg *config.Config, rt *config.RuntimeSettings) error {
	db := GetDB()
	if db == nil {
		return fmt.Errorf("settings: database not initialized")
	}

	settingsStore := NewSettingsStore(db)
	all, err := settingsStore.GetAll()
	if err != nil {
		return err
	}

	settingsMap := toSettingsMap(all)
	if len(settingsMap) == 0 {
		slog.Info("settings: no runtime settings found in DB")
	} else {
		slog.Info("settings: loaded runtime settings", "count", len(settingsMap))
		// Apply runtime overrides to the static + runtime drafts.
		ApplyRuntimeSettings(cfg, rt, settingsMap)
	}

	// Which log-retention regime owns the log tables is decided by explicit
	// operator intent only: an admin-saved log-cleanup setting, or an env
	// toggle explicitly set to true (config.LogCleanupEnvEnabled). It used to
	// be inferred from "retention > 0", but config.Load floors retention at 1
	// day, so the inference fired as soon as the settings table held any row at
	// all: it silently flipped an explicit LOG_CLEANUP_*_ENABLED=false to true
	// and disabled the legacy proxy-log retention scheduler. Saving an
	// unrelated setting (the site name, say) must not switch the regime, and
	// an env-only deployment must not lose the regime it asked for either.
	//
	// The two sources are asymmetric on purpose: env can turn the regime ON but
	// cannot "claim" it while off, because claiming it disables the legacy
	// PROXY_LOG_RETENTION_DAYS pruner and would leave proxy_logs unpruned.
	// Values still come from the settings table when it has them (hydration ran
	// above); env only contributes the regime bit.
	dbExplicit := HasExplicitLogCleanupSettings(settingsMap)
	cfg.LogCleanupConfigured = dbExplicit || cfg.LogCleanupEnvEnabled

	slog.Info("settings: log retention regime",
		"regime", logRetentionRegime(cfg.LogCleanupConfigured),
		"configured", cfg.LogCleanupConfigured,
		"source", logRetentionRegimeSource(dbExplicit, cfg.LogCleanupEnvEnabled),
		"usage_logs_enabled", rt.LogCleanupUsageLogsEnabled,
		"program_logs_enabled", rt.LogCleanupProgramLogsEnabled,
		"retention_days", rt.LogCleanupRetentionDays)

	return nil
}

// logRetentionRegime names the scheduler that owns the log tables, for the
// single startup line above: "log_cleanup" prunes usage/program logs on
// LOG_CLEANUP_CRON, "legacy_fallback" leaves proxy_logs to the
// PROXY_LOG_RETENTION_DAYS pruner.
func logRetentionRegime(configured bool) string {
	if configured {
		return "log_cleanup"
	}
	return "legacy_fallback"
}

// logRetentionRegimeSource reports which kind of operator intent claimed the
// regime. Admin-saved settings win over the env toggle because they are also
// the source of the values, so the two never disagree about what is running.
func logRetentionRegimeSource(dbExplicit, envEnabled bool) string {
	switch {
	case dbExplicit:
		return "db_settings"
	case envEnabled:
		return "env_toggle"
	default:
		return "none"
	}
}

// toSettingsMap converts flat settings rows into a nested map structure.
// Keys containing dots (e.g. "log_cleanup.retention_days") are treated as
// namespaced keys; the caller interprets the namespace.
func toSettingsMap(rows map[string]string) map[string]string {
	return rows
}

// logCleanupExplicitKeys are the settings keys whose presence proves the
// operator configured log cleanup themselves, so startup must neither infer
// nor override anything for that subsystem.
//
// The underscore spellings are what the admin write path persists
// (handler/admin/settings_apply.go). The dotted spellings are deprecated
// read-only aliases: no release ever wrote them, they only matter for
// hand-edited or externally imported rows, and they can be dropped once such
// rows are extinct.
var logCleanupExplicitKeys = []string{
	"log_cleanup_usage_logs_enabled",
	"log_cleanup_program_logs_enabled",
	"log_cleanup_retention_days",
	"log_cleanup.enabled",
	"log_cleanup.usage_logs_enabled",
	"log_cleanup.program_logs_enabled",
	"log_cleanup.retention_days",
}

// HasExplicitLogCleanupSettings checks whether log cleanup was explicitly
// configured via the settings table. Mirrors TS behavior: checks for specific
// setting keys that indicate user-intended log cleanup config.
func HasExplicitLogCleanupSettings(settingsMap map[string]string) bool {
	for _, key := range logCleanupExplicitKeys {
		if _, ok := settingsMap[key]; ok {
			return true
		}
	}
	return false
}

// nonHydratedSettingKeys are settings that legitimately live in the settings
// table but are deliberately NOT folded into the startup snapshot by
// ApplyRuntimeSettings, each with the reason why. Two shapes belong here: keys
// whose consumer reads the settings table itself at use time (and re-validates
// what it reads), and keys whose value is consumed before hydration runs.
//
// Everything else the admin API can persist must have a case in
// ApplyRuntimeSettings. settings_rehydration_gate_test.go fails when it does
// not, because a persisted key that nothing reads back means the operator's
// change evaporates on the next restart.
var nonHydratedSettingKeys = map[string]string{
	// Resolved by store.EnsureRuntimeDatabase from the env-derived snapshot:
	// the database is already open (and is the very thing being read) by the
	// time LoadRuntimeSettings runs.
	"db_type": "consumed by store.EnsureRuntimeDatabase before hydration runs",
	"db_url":  "consumed by store.EnsureRuntimeDatabase before hydration runs",
	"db_ssl":  "consumed by store.EnsureRuntimeDatabase before hydration runs",
	// v2 ScheduleSpec mirrors of the cron keys, read by the schedule
	// migration service and the admin schedule endpoints.
	"checkin_schedule_v2":         "read by service/settingsmigration + handler/admin/checkin_schedule.go",
	"balance_refresh_schedule_v2": "read by service/settingsmigration",
	"log_cleanup_schedule_v2":     "read by service/settingsmigration",
	// Secret read at use time by its own handler; never published to the
	// runtime snapshot, where a stale copy would outlive rotation.
	"monitor_ldoh_cookie": "read at use time by handler/admin/monitor.go",
	// Backup destination + resume cursor, read at use time by the scheduler.
	"backup_webdav_config_v1": "read at use time by scheduler/backup_webdav.go",
	"backup_webdav_state_v1":  "read at use time by scheduler/backup_webdav.go",
	// Read at use time by the daily-summary scheduler.
	"daily_summary_cron": "read at use time by scheduler/daily_summary.go",
	// Removed setting (Wave 8 Lane D): it was stored but never rendered.
	// Legacy rows are intentionally ignored, and listed here so they do not
	// trip the unknown-key warning on every boot.
	"home_page_content": "setting removed; legacy rows intentionally ignored",
}

// unknownKeyLogLimit bounds the aggregated unknown-key warning so a database
// carrying many legacy rows cannot produce an unbounded log line.
const unknownKeyLogLimit = 20

// ApplyRuntimeSettings applies DB-backed settings overrides to the config.
// Each known key overrides the corresponding config field.

// Mirrors TS runtime settings application logic. Settings stored in the
// settings table are JSON-encoded; this function parses and applies them.
// Numeric keys are clamped exactly like the env path in config.Load, so a
// value persisted through the admin API cannot bypass a floor the same value
// would hit when it arrives through env.
func ApplyRuntimeSettings(cfg *config.Config, rt *config.RuntimeSettings, settingsMap map[string]string) {
	var unknown []string
	for key, rawValue := range settingsMap {
		value := strings.TrimSpace(rawValue)
		if value == "" {
			// A NULL/absent row carries no operator intent. An explicit clear
			// is the two-character JSON string `""`, which the per-key cases
			// below decide on individually.
			continue
		}

		switch key {
		// Auth. These keep their non-empty guard on purpose: a blank persisted
		// value must never replace a credential that env (or an earlier
		// rotation) already configured, or a restart would silently downgrade
		// the deployment to the built-in default token. The branding keys
		// below are the opposite case and do accept an explicit blank.
		//
		// The values are JSON-encoded like every other setting (the admin
		// write path marshals them), so they must be decoded like every other
		// string setting: reading the raw row used to rehydrate the token with
		// its surrounding quotes, which made a token rotated through the admin
		// API stop authenticating after the next restart.
		case "auth_token":
			if v := parseJSONSettingString(value); v != "" {
				rt.AuthToken = v
			}
		case "proxy_token":
			if v := parseJSONSettingString(value); v != "" {
				rt.ProxyToken = v
			}
		case "account_credential_secret":
			if v := parseJSONSettingString(value); v != "" {
				cfg.AccountCredentialSecret = v
			}

		// Server
		case "port":
			cfg.Port = parseInt(value, cfg.Port)

		// Admin access control. An empty list means "every IP may reach the
		// admin API", so an unparseable value must not wipe a persisted
		// allowlist: that would silently re-open the control plane on restart.
		case "admin_ip_allowlist":
			if list, ok := parseStringListSetting(value); ok {
				rt.AdminIpAllowlist = list
			} else {
				slog.Warn("settings: ignoring invalid admin_ip_allowlist value")
			}

		// Proxy retry/disable status-code range policy (P1-2): blank keeps
		// the routing defaults (historical behavior).
		case "proxy_retry_status_ranges":
			rt.ProxyRetryStatusRanges = parseJSONSettingString(value)
		case "proxy_disable_status_ranges":
			rt.ProxyDisableStatusRanges = parseJSONSettingString(value)

		// Checkin schedule
		case "checkin_cron":
			// Validated like the env path (config.Validate rejects an
			// unparseable CHECKIN_CRON): an invalid expression must keep the
			// resolved default instead of becoming the scheduler's fallback.
			if v := parseJSONSettingString(value); config.ValidateCronExpr(v) {
				rt.CheckinCron = v
			} else {
				slog.Warn("settings: ignoring invalid checkin_cron value")
			}
		case "checkin_enabled":
			// Persisted as the positive switch, published inverted (zero value
			// = enabled), mirroring config.Load's CHECKIN_ENABLED handling.
			rt.CheckinDisabled = !parseBoolSetting(value, !rt.CheckinDisabled)
		case "checkin_schedule_mode":
			// config.Load resolves CHECKIN_SCHEDULE_MODE through the same
			// three-value enum, and RuntimeSettings.Validate reports anything
			// else as a critical boot error, so an unreadable row must keep the
			// resolved mode instead of poisoning the snapshot. Keeping it
			// silently left the row and GET /api/settings/runtime disagreeing
			// forever: scheduler/settings.go re-reads the row and falls back the
			// same way, so nothing ever surfaced the stale value. Reachable
			// through a backup import (the tables path validates only the cell
			// type), a hand edit, or a legacy row service/settingsmigration
			// leaves behind without validating the mode.
			mode := strings.ToLower(parseJSONSettingString(value))
			switch mode {
			case "cron", "interval", "window":
				rt.CheckinScheduleMode = mode
			default:
				reason := fmt.Sprintf("%q is not cron, interval or window", mode)
				if mode == "" {
					reason = "value carries no mode"
				}
				warnUnreadableSettingRow("checkin_schedule_mode", reason, rt.CheckinScheduleMode)
			}
		case "checkin_interval_hours":
			// config.Load resolves CHECKIN_INTERVAL_HOURS as
			// ClampInt(trunc(parseNumber(n)), 1, 24), so this clamps with the
			// same two-sided shape instead of guarding on the range. The guard
			// dropped an out-of-range row: a persisted 30 left the process on
			// the env-resolved 6, GET /api/settings/runtime echoed 6, and
			// nothing was logged, because the key counted as handled.
			rt.CheckinIntervalHours = config.ClampInt(
				parseIntSetting(value, float64(rt.CheckinIntervalHours), 1), 1, 24)
		// E1: random-window mode bounds (HH:mm, 24h).
		case "checkin_window_start":
			if v := parseJSONSettingString(value); v != "" {
				rt.CheckinWindowStart = v
			}
		case "checkin_window_end":
			if v := parseJSONSettingString(value); v != "" {
				rt.CheckinWindowEnd = v
			}

		// Periodic job schedules and kill switches. Each scheduler re-resolves
		// its own keys from the settings table when it starts; hydrating them
		// here keeps the published snapshot (and GET /api/settings/runtime)
		// truthful before that happens, and gives the schedulers a valid
		// fallback instead of an env value the operator already overrode.
		case "balance_refresh_cron":
			if v := parseJSONSettingString(value); config.ValidateCronExpr(v) {
				rt.BalanceRefreshCron = v
			} else {
				slog.Warn("settings: ignoring invalid balance_refresh_cron value")
			}
		case "balance_refresh_enabled":
			rt.BalanceRefreshDisabled = !parseBoolSetting(value, !rt.BalanceRefreshDisabled)
		case "model_sync_cron":
			if v := parseJSONSettingString(value); config.ValidateCronExpr(v) {
				rt.ModelSyncCron = v
			} else {
				slog.Warn("settings: ignoring invalid model_sync_cron value")
			}

		// Site & Branding. An explicit blank is honored: the embedded defaults
		// are empty strings and the admin write path persists whatever the
		// operator typed (including ""), so dropping empty values here made a
		// cleared site name/logo/footer/about/server address reappear as the
		// env value after a restart — while clearing a webhook URL in the same
		// form stuck. Both write paths now share one empty-value semantics.
		case "system_name":
			rt.SystemName = parseJSONSettingString(value)
		case "logo":
			rt.Logo = parseJSONSettingString(value)
		case "footer":
			rt.Footer = parseJSONSettingString(value)
		case "about":
			rt.About = parseJSONSettingString(value)
		case "server_address":
			rt.ServerAddress = parseJSONSettingString(value)

		// Notify
		case "webhook_url":
			rt.WebhookUrl = parseJSONSettingString(value)
		case "webhook_enabled":
			rt.WebhookEnabled = parseBoolSetting(value, rt.WebhookEnabled)
		case "bark_url":
			rt.BarkUrl = parseJSONSettingString(value)
		case "bark_enabled":
			rt.BarkEnabled = parseBoolSetting(value, rt.BarkEnabled)
		case "serverchan_key":
			rt.ServerChanKey = parseJSONSettingString(value)
		case "serverchan_enabled":
			rt.ServerChanEnabled = parseBoolSetting(value, rt.ServerChanEnabled)

		// Notify: General
		case "notify_cooldown_sec":
			rt.NotifyCooldownSec = parseInt(value, rt.NotifyCooldownSec)
		case "system_proxy_url":
			rt.SystemProxyUrl = parseJSONSettingString(value)

		// Telegram
		case "telegram_enabled":
			rt.TelegramEnabled = parseBoolSetting(value, rt.TelegramEnabled)
		case "telegram_bot_token":
			rt.TelegramBotToken = parseJSONSettingString(value)
		case "telegram_chat_id":
			rt.TelegramChatId = parseJSONSettingString(value)
		case "telegram_api_base_url":
			rt.TelegramApiBaseUrl = parseJSONSettingString(value)
		case "telegram_use_system_proxy":
			rt.TelegramUseSystemProxy = parseBoolSetting(value, rt.TelegramUseSystemProxy)
		case "telegram_message_thread_id":
			rt.TelegramMessageThreadId = parseJSONSettingString(value)

		// SMTP
		case "smtp_enabled":
			rt.SmtpEnabled = parseBoolSetting(value, rt.SmtpEnabled)
		case "smtp_host":
			rt.SmtpHost = parseJSONSettingString(value)
		case "smtp_port":
			rt.SmtpPort = parseInt(value, rt.SmtpPort)
		case "smtp_user":
			rt.SmtpUser = parseJSONSettingString(value)
		case "smtp_pass":
			rt.SmtpPass = parseJSONSettingString(value)
		case "smtp_from":
			rt.SmtpFrom = parseJSONSettingString(value)
		case "smtp_to":
			rt.SmtpTo = parseJSONSettingString(value)
		case "smtp_secure":
			rt.SmtpSecure = parseBoolSetting(value, rt.SmtpSecure)

		// Log cleanup. The admin write path persists the underscore keys; the
		// dotted spellings are deprecated read-only aliases (see
		// logCleanupExplicitKeys). Hydration used to read only the dotted
		// spellings, which nothing ever wrote, so this whole block was dead
		// code and persisted log-cleanup settings never reached the snapshot.
		case "log_cleanup_cron":
			if v := parseJSONSettingString(value); config.ValidateCronExpr(v) {
				rt.LogCleanupCron = v
			} else {
				slog.Warn("settings: ignoring invalid log_cleanup_cron value")
			}
		case "log_cleanup_usage_logs_enabled", "log_cleanup.usage_logs_enabled":
			rt.LogCleanupUsageLogsEnabled = parseBoolSetting(value, rt.LogCleanupUsageLogsEnabled)
		case "log_cleanup_program_logs_enabled", "log_cleanup.program_logs_enabled":
			rt.LogCleanupProgramLogsEnabled = parseBoolSetting(value, rt.LogCleanupProgramLogsEnabled)
		case "log_cleanup_retention_days", "log_cleanup.retention_days":
			// config.Load floors the env value at 1 day; the settings path
			// must not be able to bypass that floor.
			rt.LogCleanupRetentionDays = parseIntSetting(value, float64(rt.LogCleanupRetentionDays), 1)

		// Proxy settings
		case "proxy_max_channel_attempts":
			cfg.ProxyMaxChannelAttempts = config.MaxInt(1, parseInt(value, cfg.ProxyMaxChannelAttempts))
		case "proxy_first_byte_timeout_sec":
			// config.Load: max(0, trunc(n)); 0 disables the observed
			// first-byte timeout.
			rt.ProxyFirstByteTimeoutSec = parseIntSetting(value, float64(rt.ProxyFirstByteTimeoutSec), 0)
		case "proxy_empty_content_fail_enabled":
			rt.ProxyEmptyContentFailEnabled = parseBoolSetting(value, rt.ProxyEmptyContentFailEnabled)

		// Proxy: token router. Same normalization as config.Load §3.14 — a
		// non-finite or non-positive value keeps the env-resolved cap instead
		// of silently disabling failure cooldowns.
		case "token_router_failure_cooldown_max_sec":
			if v, ok := config.NormalizeTokenRouterFailureCooldownMaxSec(parseNumberSetting(value, 0)); ok {
				rt.TokenRouterFailureCooldownMaxSec = v
			} else {
				slog.Warn("settings: ignoring invalid token_router_failure_cooldown_max_sec value")
			}

		// Proxy: session channel limits (config.Load: max(0, trunc(n))).
		case "proxy_session_channel_concurrency_limit":
			rt.ProxySessionChannelConcurrencyLimit = parseIntSetting(value, float64(rt.ProxySessionChannelConcurrencyLimit), 0)
		case "proxy_session_channel_queue_wait_ms":
			rt.ProxySessionChannelQueueWaitMs = parseIntSetting(value, float64(rt.ProxySessionChannelQueueWaitMs), 0)

		// Proxy: fallback policy toggles.
		case "disable_cross_protocol_fallback":
			rt.DisableCrossProtocolFallback = parseBoolSetting(value, rt.DisableCrossProtocolFallback)
		case "responses_compact_fallback_to_responses_enabled":
			rt.ResponsesCompactFallbackToResponsesEnabled = parseBoolSetting(value, rt.ResponsesCompactFallbackToResponsesEnabled)

		// Routing cost model. config.Load floors the fallback unit cost at
		// 1e-6 so an unpriced model can never route as free.
		case "routing_fallback_unit_cost":
			rt.RoutingFallbackUnitCost = math.Max(1e-6, parseNumberSetting(value, rt.RoutingFallbackUnitCost))
		case "routing_weights":
			// Persisted as a whole config.RoutingWeights object. Decode into a
			// copy of the env-resolved weights so a partial or legacy row
			// cannot zero the components it does not mention.
			weights := rt.RoutingWeights
			if err := json.Unmarshal([]byte(value), &weights); err != nil {
				slog.Warn("settings: ignoring invalid routing_weights value")
			} else {
				rt.RoutingWeights = weights
			}
		// Failure-judgement keywords: an unparseable value must not wipe the
		// persisted list, or upstream failures would stop being detected.
		case "proxy_error_keywords":
			if list, ok := parseStringListSetting(value); ok {
				rt.ProxyErrorKeywords = list
			} else {
				slog.Warn("settings: ignoring invalid proxy_error_keywords value")
			}

		// Model probe
		case "model_availability_probe_enabled":
			rt.ModelAvailabilityProbeEnabled = parseBoolSetting(value, rt.ModelAvailabilityProbeEnabled)

		// Codex
		case "codex_upstream_websocket_enabled":
			rt.CodexUpstreamWebsocketEnabled = parseBoolSetting(value, rt.CodexUpstreamWebsocketEnabled)

		// Proxy: debug tracing. The master toggle was already hydrated but the
		// capture/target/retention knobs it depends on were not, so a restart
		// left tracing enabled with env-default capture rules.
		case "proxy_debug_trace_enabled":
			rt.ProxyDebugTraceEnabled = parseBoolSetting(value, rt.ProxyDebugTraceEnabled)
		case "proxy_debug_capture_headers":
			rt.ProxyDebugCaptureHeaders = parseBoolSetting(value, rt.ProxyDebugCaptureHeaders)
		case "proxy_debug_capture_bodies":
			rt.ProxyDebugCaptureBodies = parseBoolSetting(value, rt.ProxyDebugCaptureBodies)
		case "proxy_debug_capture_stream_chunks":
			rt.ProxyDebugCaptureStreamChunks = parseBoolSetting(value, rt.ProxyDebugCaptureStreamChunks)
		case "proxy_debug_target_session_id":
			rt.ProxyDebugTargetSessionId = parseJSONSettingString(value)
		case "proxy_debug_target_client_kind":
			rt.ProxyDebugTargetClientKind = parseJSONSettingString(value)
		case "proxy_debug_target_model":
			rt.ProxyDebugTargetModel = parseJSONSettingString(value)
		case "proxy_debug_retention_hours":
			// config.Load: max(1, trunc(n)).
			rt.ProxyDebugRetentionHours = parseIntSetting(value, float64(rt.ProxyDebugRetentionHours), 1)
		case "proxy_debug_max_body_bytes":
			// config.Load: max(1024, trunc(n)).
			rt.ProxyDebugMaxBodyBytes = parseIntSetting(value, float64(rt.ProxyDebugMaxBodyBytes), 1024)

		// Generic JSON settings
		case "global_blocked_brands":
			if list, ok := parseStringListSetting(value); ok {
				rt.GlobalBlockedBrands = list
			} else {
				slog.Warn("settings: ignoring invalid global_blocked_brands value")
			}
		case "global_allowed_models":
			// Non-destructive: invalid / unparseable values must not wipe a
			// previously configured allowlist (upstream / ).
			if list, ok := parseStringListSetting(value); ok {
				rt.GlobalAllowedModels = list
			} else {
				slog.Warn("settings: ignoring invalid global_allowed_models value")
			}
		// Non-destructive: a row hydration cannot read means the persisted
		// intent is unrecoverable, not that the operator asked for no rules.
		// Both branches used to assign the parse result directly, and
		// config.ParseJsonValue encodes failure as nil, so one bad row emptied
		// a configured rule set on the next restart with nothing in the log.
		// An explicit clear still works: the admin write path persists JSON
		// null or `[]` (upsertSettingDB normalizes a nil body value to an empty
		// slice), both of which parse.
		case "payload_rules":
			if rules, err := parseJSONValueSetting(value); err == nil {
				rt.PayloadRules = rules
			} else {
				warnUnreadableSettingRow("payload_rules", err.Error(),
					describeJSONSettingValue(rt.PayloadRules))
			}
		case "openai_service_tier_rules":
			if rules, err := parseJSONValueSetting(value); err == nil {
				cfg.OpenAiServiceTierRules = rules
			} else {
				warnUnreadableSettingRow("openai_service_tier_rules", err.Error(),
					describeJSONSettingValue(cfg.OpenAiServiceTierRules))
			}

		// N7: prompt-cache ratio fallback overrides (0 = use code default).
		case "cache_ratio_default":
			rt.CacheRatioDefault = parseFloatSetting(value, 0)
		case "cache_ratio_claude":
			rt.CacheRatioClaude = parseFloatSetting(value, 0)

		// Feishu / DingTalk / WeCom / Ntfy dedicated channels.
		case "feishu_enabled":
			rt.FeishuEnabled = parseBoolSetting(value, rt.FeishuEnabled)
		case "feishu_webhook":
			rt.FeishuWebhook = parseJSONSettingString(value)
		case "feishu_secret":
			rt.FeishuSecret = parseJSONSettingString(value)
		case "dingtalk_enabled":
			rt.DingtalkEnabled = parseBoolSetting(value, rt.DingtalkEnabled)
		case "dingtalk_webhook":
			rt.DingtalkWebhook = parseJSONSettingString(value)
		case "dingtalk_secret":
			rt.DingtalkSecret = parseJSONSettingString(value)
		case "wecom_enabled":
			rt.WecomEnabled = parseBoolSetting(value, rt.WecomEnabled)
		case "wecom_webhook":
			rt.WecomWebhook = parseJSONSettingString(value)
		case "ntfy_enabled":
			rt.NtfyEnabled = parseBoolSetting(value, rt.NtfyEnabled)
		case "ntfy_url":
			rt.NtfyUrl = parseJSONSettingString(value)
		case "ntfy_topic":
			rt.NtfyTopic = parseJSONSettingString(value)
		case "ntfy_token":
			rt.NtfyToken = parseJSONSettingString(value)

		// per-alert-type mute toggles (JSON object).
		// Stored as {"token_expired":true,"low_balance":false,...}; missing
		// keys default to enabled (backward-compatible). nil/empty = all enabled.
		case "notify_task_toggles":
			// service/notify gates each alert type on this map and treats a
			// missing key as enabled, so a row hydration cannot read must not
			// become "every alert type is unmuted": keep the resolved toggles
			// and say so. The admin write path marshals whatever the request
			// carried without checking that it is an object of booleans
			// (handler/admin/settings_apply.go), so a wrong-shaped row is
			// reachable without hand-editing the table. The loop above already
			// skipped empty cells, which carry no intent.
			if toggles, err := parseNotifyTaskTogglesSetting(value); err == nil {
				if rt.NotifyTaskToggles == nil {
					rt.NotifyTaskToggles = map[string]bool{}
				}
				for k, v := range toggles {
					rt.NotifyTaskToggles[k] = v
				}
			} else {
				warnUnreadableSettingRow("notify_task_toggles", err.Error(),
					fmt.Sprintf("%d toggles", len(rt.NotifyTaskToggles)))
			}

		default:
			// Keys listed in nonHydratedSettingKeys are resolved by their own
			// consumer; anything else that reached the table without a case
			// above is a round-trip defect worth one aggregated warning.
			if _, ok := nonHydratedSettingKeys[key]; !ok {
				unknown = append(unknown, key)
			}
		}
	}

	if len(unknown) > 0 {
		sort.Strings(unknown)
		slog.Warn("settings: persisted keys not applied at startup hydration",
			"count", len(unknown),
			"keys", strings.Join(truncateKeyList(unknown, unknownKeyLogLimit), ","))
	}
}

// truncateKeyList caps a key list destined for a single log line, appending a
// "+N more" marker when it had to cut entries.
func truncateKeyList(keys []string, limit int) []string {
	if len(keys) <= limit {
		return keys
	}
	out := append([]string{}, keys[:limit]...)
	return append(out, fmt.Sprintf("(+%d more)", len(keys)-limit))
}

// parseStringListSetting decodes a settings-table value into a trimmed,
// de-duplicated string list. It accepts:
// - JSON arrays: ["a","b"]
// - JSON null → empty list
// - legacy double-encoded JSON arrays: "[\"a\",\"b\"]"
// - comma-separated plain strings: "a, b"

// ok=false means the value is present but unusable; callers must not overwrite
// the in-memory setting with an empty default in that case.
func parseStringListSetting(raw string) ([]string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false
	}

	var decoded any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		// Non-JSON legacy plain string (comma-separated).
		return normalizeStringList(raw), true
	}

	switch v := decoded.(type) {
	case nil:
		return []string{}, true
	case []any:
		return normalizeStringListAny(v), true
	case []string:
		return normalizeStringList(v...), true
	case string:
		// Double-encoded JSON array/string, or comma-separated plain string.
		inner := strings.TrimSpace(v)
		if inner == "" {
			return []string{}, true
		}
		var nested any
		if err := json.Unmarshal([]byte(inner), &nested); err == nil {
			switch nv := nested.(type) {
			case nil:
				return []string{}, true
			case []any:
				return normalizeStringListAny(nv), true
			case []string:
				return normalizeStringList(nv...), true
			case string:
				return normalizeStringList(nv), true
			default:
				return nil, false
			}
		}
		return normalizeStringList(inner), true
	default:
		return nil, false
	}
}

func normalizeStringListAny(items []any) []string {
	out := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		s, ok := item.(string)
		if !ok {
			continue
		}
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, exists := seen[s]; exists {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func normalizeStringList(parts ...string) []string {
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		for _, item := range strings.Split(part, ",") {
			s := strings.TrimSpace(item)
			if s == "" {
				continue
			}
			if _, exists := seen[s]; exists {
				continue
			}
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}

// parseNumberSetting decodes a JSON number setting. The admin write path
// marshals ints and floats with encoding/json, so values arrive as "30" or
// "0.25". Unlike parseFloatSetting it keeps negative numbers, letting each
// case apply the exact clamp config.Load uses for the matching env var instead
// of silently substituting the fallback. Unparseable or non-finite values keep
// the fallback.
func parseNumberSetting(raw string, fallback float64) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return fallback
	}
	return v
}

// parseIntSetting mirrors the "max(lo, trunc(n))" shape config.Load uses for
// its integer settings, so a number persisted through the admin API is clamped
// identically to the same number supplied through env.
func parseIntSetting(raw string, fallback float64, lo int) int {
	return config.MaxInt(lo, int(math.Trunc(parseNumberSetting(raw, fallback))))
}

// parseBoolSetting parses a boolean setting value.
func parseBoolSetting(value string, fallback bool) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return fallback
	}
	return normalized == "1" || normalized == "true" || normalized == "yes" || normalized == "on"
}

// parseInt parses an integer setting value, returning fallback on failure.
func parseInt(value string, fallback int) int {
	v, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return v
}

// parseFloatSetting parses a non-negative float setting; returns fallback on
// parse error, NaN, Inf, or negative. 0 is preserved (free cache read).
func parseFloatSetting(value string, fallback float64) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || !(v >= 0) || isNaNOrInf(v) {
		return fallback
	}
	return v
}

func isNaNOrInf(v float64) bool {
	return v != v || v > 1e308 || v < -1e308
}

func parseJSONSettingString(value string) string {
	value = strings.TrimSpace(value)
	var decoded string
	if err := json.Unmarshal([]byte(value), &decoded); err == nil {
		return strings.TrimSpace(decoded)
	}
	return strings.TrimSpace(value)
}

// parseJSONValueSetting decodes a settings row that holds arbitrary JSON. An
// empty cell, or one encoding/json cannot read, means the persisted intent is
// unrecoverable: the caller must keep the value it already resolved instead of
// assigning the failure result, because config.ParseJsonValue encodes that
// failure as nil and a direct assignment turned one unreadable row into an
// empty rule set. A readable JSON null IS intent ("no rules") and comes back as
// (nil, nil).
func parseJSONValueSetting(raw string) (any, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("value is empty")
	}
	var decoded any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return nil, fmt.Errorf("not valid JSON: %v", err)
	}
	return decoded, nil
}

// parseNotifyTaskTogglesSetting decodes the per-alert-type mute map. Only a
// JSON object of booleans — or null, meaning "no per-type overrides" — carries
// a readable intent; an array, a bare scalar or a non-boolean flag is a row the
// caller must refuse to apply.
func parseNotifyTaskTogglesSetting(raw string) (map[string]bool, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("value is empty")
	}
	var decoded any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return nil, fmt.Errorf("not valid JSON: %v", err)
	}
	switch v := decoded.(type) {
	case nil:
		return map[string]bool{}, nil
	case map[string]any:
		toggles := make(map[string]bool, len(v))
		for name, flag := range v {
			enabled, ok := flag.(bool)
			if !ok {
				return nil, fmt.Errorf("toggle %q is %s, want a boolean",
					name, describeJSONSettingValue(flag))
			}
			toggles[name] = enabled
		}
		return toggles, nil
	default:
		return nil, fmt.Errorf("JSON %s, want an object of booleans", describeJSONSettingValue(decoded))
	}
}

// warnUnreadableSettingRow logs the one line an operator needs when hydration
// refuses a persisted row: which key, why, and which value the process keeps.
// A refusal the log does not mention leaves the settings table and
// GET /api/settings/runtime disagreeing with nothing to explain the gap.
func warnUnreadableSettingRow(key, reason string, kept any) {
	slog.Warn("settings: persisted value not applied, keeping the resolved value",
		"key", key, "reason", reason, "kept", kept)
}

// describeJSONSettingValue names the shape of a resolved JSON setting so a
// warning can say what the process keeps without echoing the whole blob.
func describeJSONSettingValue(v any) string {
	switch value := v.(type) {
	case nil:
		return "null"
	case map[string]any:
		return fmt.Sprintf("object with %d keys", len(value))
	case []any:
		return fmt.Sprintf("array with %d items", len(value))
	case string:
		return "string"
	case bool:
		return fmt.Sprintf("bool %t", value)
	case float64:
		return "number"
	default:
		return fmt.Sprintf("%T", v)
	}
}
