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
func LoadRuntimeSettings(cfg *config.Config) error {
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

	// Apply runtime overrides to config.
	ApplyRuntimeSettings(cfg, settingsMap)

	// Track whether log cleanup was explicitly configured via DB settings.
	cfg.LogCleanupConfigured = HasExplicitLogCleanupSettings(settingsMap)

	// Auto-enable log cleanup if retention > 0 and not previously configured.
	if !cfg.LogCleanupConfigured && cfg.LogCleanupRetentionDays > 0 {
		cfg.LogCleanupUsageLogsEnabled = true
		cfg.LogCleanupProgramLogsEnabled = true
		cfg.LogCleanupConfigured = true
		slog.Info("settings: auto-enabled log cleanup", "retention_days", cfg.LogCleanupRetentionDays)
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
func ApplyRuntimeSettings(cfg *config.Config, settingsMap map[string]string) {
	for key, rawValue := range settingsMap {
		value := strings.TrimSpace(rawValue)
		if value == "" {
			continue
		}

		switch key {
		// Auth
		case "auth_token":
			if v := parseOptionalString(value); v != "" {
				cfg.AuthToken = v
			}
		case "proxy_token":
			if v := parseOptionalString(value); v != "" {
				cfg.ProxyToken = v
			}
		case "account_credential_secret":
			if v := parseOptionalString(value); v != "" {
				cfg.AccountCredentialSecret = v
			}

		// Server
		case "port":
			cfg.Port = parseInt(value, cfg.Port)

		// Checkin schedule
		case "checkin_cron":
			if v := parseJSONSettingString(value); v != "" {
				cfg.CheckinCron = v
			}
		case "checkin_schedule_mode":
			switch strings.ToLower(parseJSONSettingString(value)) {
			case "cron", "interval", "window":
				cfg.CheckinScheduleMode = strings.ToLower(parseJSONSettingString(value))
			}
		case "checkin_interval_hours":
			hours := parseInt(value, cfg.CheckinIntervalHours)
			if hours >= 1 && hours <= 24 {
				cfg.CheckinIntervalHours = hours
			}
		// E1: random-window mode bounds (HH:mm, 24h).
		case "checkin_window_start":
			if v := parseJSONSettingString(value); v != "" {
				cfg.CheckinWindowStart = v
			}
		case "checkin_window_end":
			if v := parseJSONSettingString(value); v != "" {
				cfg.CheckinWindowEnd = v
			}

		// Notify
		case "webhook_url":
			cfg.WebhookUrl = value
		case "webhook_enabled":
			cfg.WebhookEnabled = parseBoolSetting(value, cfg.WebhookEnabled)
		case "bark_url":
			cfg.BarkUrl = value
		case "bark_enabled":
			cfg.BarkEnabled = parseBoolSetting(value, cfg.BarkEnabled)
		case "serverchan_key":
			cfg.ServerChanKey = value
		case "serverchan_enabled":
			cfg.ServerChanEnabled = parseBoolSetting(value, cfg.ServerChanEnabled)

		// Telegram
		case "telegram_enabled":
			cfg.TelegramEnabled = parseBoolSetting(value, cfg.TelegramEnabled)
		case "telegram_bot_token":
			cfg.TelegramBotToken = value
		case "telegram_chat_id":
			cfg.TelegramChatId = value

		// SMTP
		case "smtp_enabled":
			cfg.SmtpEnabled = parseBoolSetting(value, cfg.SmtpEnabled)
		case "smtp_host":
			cfg.SmtpHost = value
		case "smtp_port":
			cfg.SmtpPort = parseInt(value, cfg.SmtpPort)
		case "smtp_user":
			cfg.SmtpUser = value
		case "smtp_pass":
			cfg.SmtpPass = value
		case "smtp_from":
			cfg.SmtpFrom = value
		case "smtp_to":
			cfg.SmtpTo = value

		// Log cleanup
		case "log_cleanup.usage_logs_enabled":
			cfg.LogCleanupUsageLogsEnabled = parseBoolSetting(value, cfg.LogCleanupUsageLogsEnabled)
		case "log_cleanup.program_logs_enabled":
			cfg.LogCleanupProgramLogsEnabled = parseBoolSetting(value, cfg.LogCleanupProgramLogsEnabled)
		case "log_cleanup.retention_days":
			cfg.LogCleanupRetentionDays = config.MaxInt(1, parseInt(value, cfg.LogCleanupRetentionDays))

		// Proxy settings
		case "proxy_max_channel_attempts":
			cfg.ProxyMaxChannelAttempts = config.MaxInt(1, parseInt(value, cfg.ProxyMaxChannelAttempts))
		case "proxy_debug_trace_enabled":
			cfg.ProxyDebugTraceEnabled = parseBoolSetting(value, cfg.ProxyDebugTraceEnabled)

		// Model probe
		case "model_availability_probe_enabled":
			cfg.ModelAvailabilityProbeEnabled = parseBoolSetting(value, cfg.ModelAvailabilityProbeEnabled)

		// Codex
		case "codex_upstream_websocket_enabled":
			cfg.CodexUpstreamWebsocketEnabled = parseBoolSetting(value, cfg.CodexUpstreamWebsocketEnabled)

		// Generic JSON settings
		case "global_blocked_brands":
			if list, ok := parseStringListSetting(value); ok {
				cfg.GlobalBlockedBrands = list
			} else {
				slog.Warn("settings: ignoring invalid global_blocked_brands value")
			}
		case "global_allowed_models":
			// Non-destructive: invalid / unparseable values must not wipe a
			// previously configured allowlist (upstream / ).
			if list, ok := parseStringListSetting(value); ok {
				cfg.GlobalAllowedModels = list
			} else {
				slog.Warn("settings: ignoring invalid global_allowed_models value")
			}
		case "payload_rules":
			cfg.PayloadRules = config.ParseJsonValue(value)
		case "openai_service_tier_rules":
			cfg.OpenAiServiceTierRules = config.ParseJsonValue(value)

		// N7: prompt-cache ratio fallback overrides (0 = use code default).
		case "cache_ratio_default":
			cfg.CacheRatioDefault = parseFloatSetting(value, 0)
		case "cache_ratio_claude":
			cfg.CacheRatioClaude = parseFloatSetting(value, 0)

		// Feishu / DingTalk / WeCom / Ntfy dedicated channels.
		case "feishu_enabled":
			cfg.FeishuEnabled = parseBoolSetting(value, cfg.FeishuEnabled)
		case "feishu_webhook":
			cfg.FeishuWebhook = value
		case "feishu_secret":
			cfg.FeishuSecret = value
		case "dingtalk_enabled":
			cfg.DingtalkEnabled = parseBoolSetting(value, cfg.DingtalkEnabled)
		case "dingtalk_webhook":
			cfg.DingtalkWebhook = value
		case "dingtalk_secret":
			cfg.DingtalkSecret = value
		case "wecom_enabled":
			cfg.WecomEnabled = parseBoolSetting(value, cfg.WecomEnabled)
		case "wecom_webhook":
			cfg.WecomWebhook = value
		case "ntfy_enabled":
			cfg.NtfyEnabled = parseBoolSetting(value, cfg.NtfyEnabled)
		case "ntfy_url":
			cfg.NtfyUrl = value
		case "ntfy_topic":
			cfg.NtfyTopic = value
		case "ntfy_token":
			cfg.NtfyToken = value

		// per-alert-type mute toggles (JSON object).
		// Stored as {"token_expired":true,"low_balance":false,...}; missing
		// keys default to enabled (backward-compatible). nil/empty = all enabled.
		case "notify_task_toggles":
			if value != "" {
				toggles := map[string]bool{}
				if err := json.Unmarshal([]byte(value), &toggles); err == nil {
					if cfg.NotifyTaskToggles == nil {
						cfg.NotifyTaskToggles = map[string]bool{}
					}
					for k, v := range toggles {
						cfg.NotifyTaskToggles[k] = v
					}
				}
			}

		default:
			// Unknown setting — silently skip.
			// Future: system_proxy_url, routing weights, admin_ip_allowlist, etc.
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
