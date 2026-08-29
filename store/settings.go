package store

import (
	"encoding/json"
	"fmt"
	"log/slog"
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

	if len(all) == 0 {
		slog.Info("settings: no runtime settings found in DB")
		return nil
	}

	settingsMap := toSettingsMap(all)
	slog.Info("settings: loaded runtime settings", "count", len(settingsMap))

	// Apply runtime overrides to the static + runtime drafts.
	ApplyRuntimeSettings(cfg, rt, settingsMap)

	// Track whether log cleanup was explicitly configured via DB settings.
	cfg.LogCleanupConfigured = HasExplicitLogCleanupSettings(settingsMap)

	// Auto-enable log cleanup if retention > 0 and not previously configured.
	if !cfg.LogCleanupConfigured && rt.LogCleanupRetentionDays > 0 {
		rt.LogCleanupUsageLogsEnabled = true
		rt.LogCleanupProgramLogsEnabled = true
		cfg.LogCleanupConfigured = true
		slog.Info("settings: auto-enabled log cleanup", "retention_days", rt.LogCleanupRetentionDays)
	}

	return nil
}

// toSettingsMap converts flat settings rows into a nested map structure.
// Keys containing dots (e.g. "log_cleanup.retention_days") are treated as
// namespaced keys; the caller interprets the namespace.
func toSettingsMap(rows map[string]string) map[string]string {
	return rows
}

// HasExplicitLogCleanupSettings checks whether log cleanup was explicitly
// configured via the settings table. Mirrors TS behavior: checks for
// specific setting keys that indicate user-intended log cleanup config.
func HasExplicitLogCleanupSettings(settingsMap map[string]string) bool {
	explicitKeys := []string{
		"log_cleanup.enabled",
		"log_cleanup.usage_logs_enabled",
		"log_cleanup.program_logs_enabled",
		"log_cleanup.retention_days",
	}
	for _, key := range explicitKeys {
		if _, ok := settingsMap[key]; ok {
			return true
		}
	}
	return false
}

// ApplyRuntimeSettings applies DB-backed settings overrides to the config.
// Each known key overrides the corresponding config field.

// Mirrors TS runtime settings application logic. Settings stored in the
// settings table are JSON-encoded; this function parses and applies them.
func ApplyRuntimeSettings(cfg *config.Config, rt *config.RuntimeSettings, settingsMap map[string]string) {
	for key, rawValue := range settingsMap {
		value := strings.TrimSpace(rawValue)
		if value == "" {
			continue
		}

		switch key {
		// Auth
		case "auth_token":
			if v := parseOptionalString(value); v != "" {
				rt.AuthToken = v
			}
		case "proxy_token":
			if v := parseOptionalString(value); v != "" {
				rt.ProxyToken = v
			}
		case "account_credential_secret":
			if v := parseOptionalString(value); v != "" {
				cfg.AccountCredentialSecret = v
			}

		// Server
		case "port":
			cfg.Port = parseInt(value, cfg.Port)

		// Proxy retry/disable status-code range policy (P1-2): blank keeps
		// the routing defaults (historical behavior).
		case "proxy_retry_status_ranges":
			rt.ProxyRetryStatusRanges = parseJSONSettingString(value)
		case "proxy_disable_status_ranges":
			rt.ProxyDisableStatusRanges = parseJSONSettingString(value)

		// Checkin schedule
		case "checkin_cron":
			if v := parseJSONSettingString(value); v != "" {
				rt.CheckinCron = v
			}
		case "checkin_schedule_mode":
			switch strings.ToLower(parseJSONSettingString(value)) {
			case "cron", "interval", "window":
				rt.CheckinScheduleMode = strings.ToLower(parseJSONSettingString(value))
			}
		case "checkin_interval_hours":
			hours := parseInt(value, rt.CheckinIntervalHours)
			if hours >= 1 && hours <= 24 {
				rt.CheckinIntervalHours = hours
			}
		// E1: random-window mode bounds (HH:mm, 24h).
		case "checkin_window_start":
			if v := parseJSONSettingString(value); v != "" {
				rt.CheckinWindowStart = v
			}
		case "checkin_window_end":
			if v := parseJSONSettingString(value); v != "" {
				rt.CheckinWindowEnd = v
			}

		// Site & Branding (empty values keep the embedded defaults)
		case "system_name":
			if v := parseJSONSettingString(value); v != "" {
				rt.SystemName = v
			}
		case "logo":
			if v := parseJSONSettingString(value); v != "" {
				rt.Logo = v
			}
		case "footer":
			if v := parseJSONSettingString(value); v != "" {
				rt.Footer = v
			}
		case "about":
			if v := parseJSONSettingString(value); v != "" {
				rt.About = v
			}
		// home_page_content was removed (Wave 8 Lane D): the setting was stored
		// but never rendered. Legacy rows are intentionally ignored here.
		case "server_address":
			if v := parseJSONSettingString(value); v != "" {
				rt.ServerAddress = v
			}
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

		// Log cleanup
		case "log_cleanup.usage_logs_enabled":
			rt.LogCleanupUsageLogsEnabled = parseBoolSetting(value, rt.LogCleanupUsageLogsEnabled)
		case "log_cleanup.program_logs_enabled":
			rt.LogCleanupProgramLogsEnabled = parseBoolSetting(value, rt.LogCleanupProgramLogsEnabled)
		case "log_cleanup.retention_days":
			rt.LogCleanupRetentionDays = config.MaxInt(1, parseInt(value, rt.LogCleanupRetentionDays))

		// Proxy settings
		case "proxy_max_channel_attempts":
			cfg.ProxyMaxChannelAttempts = config.MaxInt(1, parseInt(value, cfg.ProxyMaxChannelAttempts))
		case "proxy_debug_trace_enabled":
			rt.ProxyDebugTraceEnabled = parseBoolSetting(value, rt.ProxyDebugTraceEnabled)

		// Model probe
		case "model_availability_probe_enabled":
			cfg.ModelAvailabilityProbeEnabled = parseBoolSetting(value, cfg.ModelAvailabilityProbeEnabled)

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
		case "payload_rules":
			rt.PayloadRules = config.ParseJsonValue(value)
		case "openai_service_tier_rules":
			cfg.OpenAiServiceTierRules = config.ParseJsonValue(value)

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
			if value != "" {
				toggles := map[string]bool{}
				if err := json.Unmarshal([]byte(value), &toggles); err == nil {
					if rt.NotifyTaskToggles == nil {
						rt.NotifyTaskToggles = map[string]bool{}
					}
					for k, v := range toggles {
						rt.NotifyTaskToggles[k] = v
					}
				}
			}

		default:
			// Unknown setting — silently skip.
			// Future: routing weights, admin_ip_allowlist, etc.
		}
	}
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

// parseOptionalString returns the value if non-empty, empty string otherwise.
func parseOptionalString(value string) string {
	return strings.TrimSpace(value)
}

func parseJSONSettingString(value string) string {
	value = strings.TrimSpace(value)
	var decoded string
	if err := json.Unmarshal([]byte(value), &decoded); err == nil {
		return strings.TrimSpace(decoded)
	}
	return strings.TrimSpace(value)
}
