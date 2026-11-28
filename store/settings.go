package store

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"

	"github.com/deliciousbuding/metapi-go/config"
)

// ApplyRuntimeSettings hydrates the runtime and static drafts from JSON-encoded
// settings rows using the same numeric limits as config.Load.
func ApplyRuntimeSettings(cfg *config.Config, rt *config.RuntimeSettings, settingsMap map[string]string) {
	var unknown []string
	for key, rawValue := range settingsMap {
		value := strings.TrimSpace(rawValue)
		if value == "" {
			// A blank cell is absent. A JSON empty string is an explicit clear.
			continue
		}

		switch key {
		// A blank credential must not clear an env-provided or rotated token.
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

		// Proxy retry/disable status-code range policy: blank keeps
		// the routing defaults (historical behavior).
		case "proxy_retry_status_ranges":
			rt.ProxyRetryStatusRanges = parseJSONSettingString(value)
		case "proxy_disable_status_ranges":
			rt.ProxyDisableStatusRanges = parseJSONSettingString(value)

		// Checkin schedule
		case "checkin_cron":
			if v, ok := validatedCronSetting(key, parseJSONSettingString(value)); ok {
				rt.CheckinCron = v
			}
		case "checkin_enabled":
			// Persisted as the positive switch, published inverted (zero value
			// = enabled), mirroring config.Load's CHECKIN_ENABLED handling.
			rt.CheckinDisabled = !parseBoolSetting(value, !rt.CheckinDisabled)
		case "checkin_schedule_mode":
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
			// Keep the same inclusive range as CHECKIN_INTERVAL_HOURS.
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

		// Publish the periodic schedules before the schedulers start.
		case "balance_refresh_cron":
			if v, ok := validatedCronSetting(key, parseJSONSettingString(value)); ok {
				rt.BalanceRefreshCron = v
			}
		case "balance_refresh_enabled":
			rt.BalanceRefreshDisabled = !parseBoolSetting(value, !rt.BalanceRefreshDisabled)
		case "auto_create_model_routes":
			rt.AutoCreateModelRoutes = parseBoolSetting(value, rt.AutoCreateModelRoutes)
		case "model_sync_cron":
			if v, ok := validatedCronSetting(key, parseJSONSettingString(value)); ok {
				rt.ModelSyncCron = v
			}

		// Branding accepts explicit clears, including over env-provided values.
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

		// Keep the dotted read aliases for older log-cleanup rows.
		case "log_cleanup_cron":
			if v, ok := validatedCronSetting(key, parseJSONSettingString(value)); ok {
				rt.LogCleanupCron = v
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
		// Malformed rules keep the resolved value. JSON null or [] explicitly clears it.
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

		// Prompt-cache ratio fallback overrides (0 = use code default).
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
			// Merge only readable overrides. A missing alert type defaults to enabled.
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
