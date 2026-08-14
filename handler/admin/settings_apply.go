package admin

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/app"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/scheduler"
	"github.com/jmoiron/sqlx"
)

// settingsApplyError carries an HTTP status and user-facing message for a
// runtime-settings apply failure; the handler maps it to writeError.
type settingsApplyError struct {
	status int
	msg    string
}

func (e *settingsApplyError) Error() string { return e.msg }

func failSettings(status int, msg string) *settingsApplyError {
	return &settingsApplyError{status: status, msg: msg}
}

// applyStringSetting handles the common "simple string field" pattern:
// normalize the body value, mutate the config target, and persist it.
// The upsert error is deliberately ignored, matching the historical handler.
func applyStringSetting(db *sqlx.DB, body map[string]any, key string, target *string, dbKey string) {
	if v, ok := body[key]; ok {
		*target = normalizeString(v)
		upsertSettingDB(db, dbKey, *target)
	}
}

// applyProxyAccessSettings applies the proxy token and system proxy URL.
func (h *settingsHandler) applyProxyAccessSettings(body map[string]any) *settingsApplyError {
	// Proxy token
	if v, ok := body["proxyToken"]; ok {
		token := normalizeString(v)
		if !strings.HasPrefix(token, "sk-") {
			return failSettings(http.StatusBadRequest, "下游访问令牌必须以 sk- 开头")
		}
		if len(token) < 6 {
			return failSettings(http.StatusBadRequest, "下游访问令牌至少 6 位（含 sk-）")
		}
		h.cfg.ProxyToken = token
		upsertSettingDB(h.db, "proxy_token", token)
	}

	// System proxy URL
	applyStringSetting(h.db, body, "systemProxyUrl", &h.cfg.SystemProxyUrl, "system_proxy_url")

	return nil
}

// applyCheckinSettings applies the checkin schedule fields.
func (h *settingsHandler) applyCheckinSettings(body map[string]any) *settingsApplyError {
	if hasAnyKey(body, "checkinCron", "checkinScheduleMode", "checkinIntervalHours") {
		patch := checkinSchedulePatch{}
		if v, ok := body["checkinCron"]; ok {
			cron := normalizeString(v)
			patch.Cron = &cron
		}
		if v, ok := body["checkinScheduleMode"]; ok {
			mode := normalizeString(v)
			patch.Mode = &mode
		}
		if v, ok := body["checkinIntervalHours"]; ok {
			hours, err := toFloat64Strict(v)
			if err != nil {
				return failSettings(http.StatusBadRequest, "checkinIntervalHours 必须是数字类型")
			}
			if hours != float64(int(hours)) {
				return failSettings(http.StatusBadRequest, "签到间隔必须是 1 到 24 的整数小时")
			}
			intervalHours := int(hours)
			patch.IntervalHours = &intervalHours
		}
		if _, err := applyCheckinScheduleSettings(h.db, h.cfg, patch); err != nil {
			return failSettings(http.StatusBadRequest, err.Error())
		}
	}
	if v, ok := body["checkinSchedule"]; ok {
		spec, err := decodeScheduleSpec(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "checkinSchedule 格式无效")
		}
		if err := spec.Validate(); err != nil {
			return failSettings(http.StatusBadRequest, err.Error())
		}
		patch := checkinSchedulePatch{Schedule: &spec}
		switch spec.Type {
		case "daily", "custom":
			mode := "cron"
			cron, err := scheduler.ScheduleToCron(spec)
			if err != nil {
				return failSettings(http.StatusBadRequest, err.Error())
			}
			patch.Mode = &mode
			patch.Cron = &cron
		case "interval":
			mode := "interval"
			patch.Mode = &mode
			patch.IntervalHours = &spec.EveryHours
			if cron, err := scheduler.ScheduleToCron(spec); err == nil && cron != "" {
				patch.Cron = &cron
			}
		case "window":
			mode := "window"
			patch.Mode = &mode
			patch.WindowStart = &spec.WindowStart
			patch.WindowEnd = &spec.WindowEnd
		}
		if _, err := applyCheckinScheduleSettings(h.db, h.cfg, patch); err != nil {
			return failSettings(http.StatusBadRequest, err.Error())
		}
	}
	return nil
}

// applyBalanceScheduleSettings applies the balance refresh cron/schedule.
func (h *settingsHandler) applyBalanceScheduleSettings(body map[string]any) *settingsApplyError {
	// Balance refresh cron
	if v, ok := body["balanceRefreshCron"]; ok {
		cron := normalizeString(v)
		if !scheduler.ValidateCronExpr(cron) {
			return failSettings(http.StatusBadRequest, "balanceRefreshCron 不是有效的 cron 表达式")
		}
		if err := persistDualSchedule(h.db, "balance_refresh_cron", cron, "balance_refresh_schedule_v2", scheduler.CronToSchedule(cron)); err != nil {
			return failSettings(http.StatusInternalServerError, "保存余额刷新调度失败")
		}
		h.cfg.BalanceRefreshCron = cron
		if err := app.UpdateBalanceCron(cron); err != nil {
			slog.Warn("settings: balance refresh cron hot update failed", "error", err)
		}
	}
	if v, ok := body["balanceRefreshSchedule"]; ok {
		spec, err := decodeScheduleSpec(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "balanceRefreshSchedule 格式无效")
		}
		if err := spec.Validate(); err != nil {
			return failSettings(http.StatusBadRequest, err.Error())
		}
		cron, err := scheduler.ScheduleToCron(spec)
		if err != nil {
			return failSettings(http.StatusBadRequest, err.Error())
		}
		if cron == "" {
			return failSettings(http.StatusBadRequest, "balanceRefreshSchedule 无法转换为 cron 表达式")
		}
		if err := persistDualSchedule(h.db, "balance_refresh_cron", cron, "balance_refresh_schedule_v2", spec); err != nil {
			return failSettings(http.StatusInternalServerError, "保存余额刷新调度失败")
		}
		h.cfg.BalanceRefreshCron = cron
		if err := app.UpdateBalanceCron(cron); err != nil {
			slog.Warn("settings: balance refresh cron hot update failed", "error", err)
		}
	}
	return nil
}

// applyLogCleanupSettings applies the log cleanup fields and hot-reloads the
// cleanup task when the cron schedule changed.
func (h *settingsHandler) applyLogCleanupSettings(body map[string]any) *settingsApplyError {
	// Log cleanup settings
	logCleanupChanged := false
	if v, ok := body["logCleanupCron"]; ok {
		cron := normalizeString(v)
		if !scheduler.ValidateCronExpr(cron) {
			return failSettings(http.StatusBadRequest, "logCleanupCron 不是有效的 cron 表达式")
		}
		if err := persistDualSchedule(h.db, "log_cleanup_cron", cron, "log_cleanup_schedule_v2", scheduler.CronToSchedule(cron)); err != nil {
			return failSettings(http.StatusInternalServerError, "保存日志清理调度失败")
		}
		h.cfg.LogCleanupCron = cron
		logCleanupChanged = true
	}
	if v, ok := body["logCleanupUsageLogsEnabled"]; ok {
		enabled, err := toBoolStrict(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "logCleanupUsageLogsEnabled 必须是布尔类型 (true/false)")
		}
		h.cfg.LogCleanupUsageLogsEnabled = enabled
		upsertSettingDB(h.db, "log_cleanup_usage_logs_enabled", enabled)
	}
	if v, ok := body["logCleanupProgramLogsEnabled"]; ok {
		enabled, err := toBoolStrict(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "logCleanupProgramLogsEnabled 必须是布尔类型 (true/false)")
		}
		h.cfg.LogCleanupProgramLogsEnabled = enabled
		upsertSettingDB(h.db, "log_cleanup_program_logs_enabled", enabled)
	}
	if v, ok := body["logCleanupRetentionDays"]; ok {
		days, err := toFloat64Strict(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "logCleanupRetentionDays 必须是数字类型")
		}
		if days < 1 {
			return failSettings(http.StatusBadRequest, "日志清理保留天数必须是大于等于 1 的整数")
		}
		h.cfg.LogCleanupRetentionDays = int(days)
		upsertSettingDB(h.db, "log_cleanup_retention_days", int(days))
	}
	if v, ok := body["logCleanupSchedule"]; ok {
		spec, err := decodeScheduleSpec(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "logCleanupSchedule 格式无效")
		}
		if err := spec.Validate(); err != nil {
			return failSettings(http.StatusBadRequest, err.Error())
		}
		cron, err := scheduler.ScheduleToCron(spec)
		if err != nil {
			return failSettings(http.StatusBadRequest, err.Error())
		}
		if cron == "" {
			return failSettings(http.StatusBadRequest, "logCleanupSchedule 无法转换为 cron 表达式")
		}
		if err := persistDualSchedule(h.db, "log_cleanup_cron", cron, "log_cleanup_schedule_v2", spec); err != nil {
			return failSettings(http.StatusInternalServerError, "保存日志清理调度失败")
		}
		h.cfg.LogCleanupCron = cron
		logCleanupChanged = true
	}
	if logCleanupChanged {
		if err := app.UpdateLogCleanupSettings(h.cfg.LogCleanupCron, h.cfg.LogCleanupUsageLogsEnabled, h.cfg.LogCleanupProgramLogsEnabled, h.cfg.LogCleanupRetentionDays); err != nil {
			slog.Warn("settings: log cleanup hot update failed", "error", err)
		}
	}
	return nil
}

// applyFeatureToggleSettings applies the boolean feature toggles.
func (h *settingsHandler) applyFeatureToggleSettings(body map[string]any) *settingsApplyError {
	// Model probe
	if v, ok := body["modelAvailabilityProbeEnabled"]; ok {
		enabled, err := toBoolStrict(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "modelAvailabilityProbeEnabled 必须是布尔类型 (true/false)")
		}
		h.cfg.ModelAvailabilityProbeEnabled = enabled
		upsertSettingDB(h.db, "model_availability_probe_enabled", enabled)
	}

	// Codex upstream websocket
	if v, ok := body["codexUpstreamWebsocketEnabled"]; ok {
		enabled, err := toBoolStrict(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "codexUpstreamWebsocketEnabled 必须是布尔类型 (true/false)")
		}
		h.cfg.CodexUpstreamWebsocketEnabled = enabled
		upsertSettingDB(h.db, "codex_upstream_websocket_enabled", enabled)
	}

	// Responses compact fallback
	if v, ok := body["responsesCompactFallbackToResponsesEnabled"]; ok {
		enabled, err := toBoolStrict(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "responsesCompactFallbackToResponsesEnabled 必须是布尔类型 (true/false)")
		}
		h.cfg.ResponsesCompactFallbackToResponsesEnabled = enabled
		upsertSettingDB(h.db, "responses_compact_fallback_to_responses_enabled", enabled)
	}

	// Cross protocol fallback
	if v, ok := body["disableCrossProtocolFallback"]; ok {
		enabled, err := toBoolStrict(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "disableCrossProtocolFallback 必须是布尔类型 (true/false)")
		}
		h.cfg.DisableCrossProtocolFallback = enabled
		upsertSettingDB(h.db, "disable_cross_protocol_fallback", enabled)
	}

	// Proxy empty content fail
	if err := applyBoolSettingDB(h.db, body, "proxyEmptyContentFailEnabled", &h.cfg.ProxyEmptyContentFailEnabled, "proxy_empty_content_fail_enabled"); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}

	return nil
}

// applyProxySessionSettings applies the proxy session channel limits.
func (h *settingsHandler) applyProxySessionSettings(body map[string]any) *settingsApplyError {
	// Proxy session settings
	if v, ok := body["proxySessionChannelConcurrencyLimit"]; ok {
		n, err := toFloat64Strict(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "proxySessionChannelConcurrencyLimit 必须是数字类型")
		}
		if n < 0 {
			return failSettings(http.StatusBadRequest, "会话通道并发上限必须是大于等于 0 的整数")
		}
		h.cfg.ProxySessionChannelConcurrencyLimit = int(n)
		upsertSettingDB(h.db, "proxy_session_channel_concurrency_limit", int(n))
	}
	if v, ok := body["proxySessionChannelQueueWaitMs"]; ok {
		n, err := toFloat64Strict(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "proxySessionChannelQueueWaitMs 必须是数字类型")
		}
		if n < 0 {
			return failSettings(http.StatusBadRequest, "会话通道排队等待时间必须是大于等于 0 的整数毫秒")
		}
		h.cfg.ProxySessionChannelQueueWaitMs = int(n)
		upsertSettingDB(h.db, "proxy_session_channel_queue_wait_ms", int(n))
	}
	return nil
}

// applyProxyDebugSettings applies the proxy debug trace settings.
func (h *settingsHandler) applyProxyDebugSettings(body map[string]any) *settingsApplyError {
	// Debug settings
	if err := applyBoolSettingDB(h.db, body, "proxyDebugTraceEnabled", &h.cfg.ProxyDebugTraceEnabled, "proxy_debug_trace_enabled"); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}
	if err := applyBoolSettingDB(h.db, body, "proxyDebugCaptureHeaders", &h.cfg.ProxyDebugCaptureHeaders, "proxy_debug_capture_headers"); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}
	if err := applyBoolSettingDB(h.db, body, "proxyDebugCaptureBodies", &h.cfg.ProxyDebugCaptureBodies, "proxy_debug_capture_bodies"); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}
	if err := applyBoolSettingDB(h.db, body, "proxyDebugCaptureStreamChunks", &h.cfg.ProxyDebugCaptureStreamChunks, "proxy_debug_capture_stream_chunks"); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}

	applyStringSetting(h.db, body, "proxyDebugTargetSessionId", &h.cfg.ProxyDebugTargetSessionId, "proxy_debug_target_session_id")
	applyStringSetting(h.db, body, "proxyDebugTargetClientKind", &h.cfg.ProxyDebugTargetClientKind, "proxy_debug_target_client_kind")
	applyStringSetting(h.db, body, "proxyDebugTargetModel", &h.cfg.ProxyDebugTargetModel, "proxy_debug_target_model")

	if v, ok := body["proxyDebugRetentionHours"]; ok {
		n, err := toFloat64Strict(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "proxyDebugRetentionHours 必须是数字类型")
		}
		if n < 1 {
			return failSettings(http.StatusBadRequest, "代理调试保留时长必须是大于等于 1 的整数小时")
		}
		h.cfg.ProxyDebugRetentionHours = int(n)
		upsertSettingDB(h.db, "proxy_debug_retention_hours", int(n))
	}
	if v, ok := body["proxyDebugMaxBodyBytes"]; ok {
		n, err := toFloat64Strict(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "proxyDebugMaxBodyBytes 必须是数字类型")
		}
		if n < 1024 {
			return failSettings(http.StatusBadRequest, "代理调试抓取体积上限必须是大于等于 1024 的整数字节")
		}
		h.cfg.ProxyDebugMaxBodyBytes = int(n)
		upsertSettingDB(h.db, "proxy_debug_max_body_bytes", int(n))
	}
	return nil
}

// applyRoutingSettings applies the routing cost/timeout/weight fields and
// hot-reloads the prompt-cache ratio defaults.
func (h *settingsHandler) applyRoutingSettings(body map[string]any) *settingsApplyError {
	// Routing
	if v, ok := body["routingFallbackUnitCost"]; ok {
		n, err := toFloat64Strict(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "routingFallbackUnitCost 必须是数字类型")
		}
		if n <= 0 {
			return failSettings(http.StatusBadRequest, "无价模型默认单价必须是大于 0 的数字")
		}
		if n < 1e-6 {
			n = 1e-6
		}
		h.cfg.RoutingFallbackUnitCost = n
		upsertSettingDB(h.db, "routing_fallback_unit_cost", n)
	}
	if v, ok := body["proxyFirstByteTimeoutSec"]; ok {
		n, err := toFloat64Strict(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "proxyFirstByteTimeoutSec 必须是数字类型")
		}
		if n < 0 {
			return failSettings(http.StatusBadRequest, "首字超时必须是大于等于 0 的数字（秒）")
		}
		h.cfg.ProxyFirstByteTimeoutSec = int(n)
		upsertSettingDB(h.db, "proxy_first_byte_timeout_sec", int(n))
	}
	if v, ok := body["tokenRouterFailureCooldownMaxSec"]; ok {
		n, err := toFloat64Strict(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "tokenRouterFailureCooldownMaxSec 必须是数字类型")
		}
		if n <= 0 {
			return failSettings(http.StatusBadRequest, "路由失败冷却上限必须是大于 0 的数字（秒）")
		}
		h.cfg.TokenRouterFailureCooldownMaxSec = int(n)
		upsertSettingDB(h.db, "token_router_failure_cooldown_max_sec", int(n))
	}

	// Routing weights
	if v, ok := body["routingWeights"]; ok {
		if rw, ok2 := v.(map[string]any); ok2 {
			if bf, ok3 := rw["baseWeightFactor"]; ok3 {
				val, err := toFloat64Strict(bf)
				if err != nil {
					return failSettings(http.StatusBadRequest, "routingWeights.baseWeightFactor 必须是数字类型")
				}
				h.cfg.RoutingWeights.BaseWeightFactor = val
			}
			if vsf, ok3 := rw["valueScoreFactor"]; ok3 {
				val, err := toFloat64Strict(vsf)
				if err != nil {
					return failSettings(http.StatusBadRequest, "routingWeights.valueScoreFactor 必须是数字类型")
				}
				h.cfg.RoutingWeights.ValueScoreFactor = val
			}
			if cw, ok3 := rw["costWeight"]; ok3 {
				val, err := toFloat64Strict(cw)
				if err != nil {
					return failSettings(http.StatusBadRequest, "routingWeights.costWeight 必须是数字类型")
				}
				h.cfg.RoutingWeights.CostWeight = val
			}
			if bw, ok3 := rw["balanceWeight"]; ok3 {
				val, err := toFloat64Strict(bw)
				if err != nil {
					return failSettings(http.StatusBadRequest, "routingWeights.balanceWeight 必须是数字类型")
				}
				h.cfg.RoutingWeights.BalanceWeight = val
			}
			if uw, ok3 := rw["usageWeight"]; ok3 {
				val, err := toFloat64Strict(uw)
				if err != nil {
					return failSettings(http.StatusBadRequest, "routingWeights.usageWeight 必须是数字类型")
				}
				h.cfg.RoutingWeights.UsageWeight = val
			}
			upsertSettingDB(h.db, "routing_weights", h.cfg.RoutingWeights)
		}
	}

	// N7: prompt-cache ratio fallback overrides.
	if v, ok := body["cacheRatioDefault"]; ok {
		if val, err := toFloat64Strict(v); err == nil && val >= 0 {
			h.cfg.CacheRatioDefault = val
			upsertSettingDB(h.db, "cache_ratio_default", val)
		}
	}
	if v, ok := body["cacheRatioClaude"]; ok {
		if val, err := toFloat64Strict(v); err == nil && val >= 0 {
			h.cfg.CacheRatioClaude = val
			upsertSettingDB(h.db, "cache_ratio_claude", val)
		}
	}
	// Apply to routing runtime immediately (0 / non-positive falls back to code default).
	routing.SetCacheRatioDefaults(h.cfg.CacheRatioDefault, 0, h.cfg.CacheRatioClaude, 0)

	return nil
}

// applyNotifySettings applies the notification channel fields.
func (h *settingsHandler) applyNotifySettings(body map[string]any) *settingsApplyError {
	// Notify: Webhook
	if err := applyBoolSettingDB(h.db, body, "webhookEnabled", &h.cfg.WebhookEnabled, "webhook_enabled"); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}
	applyStringSetting(h.db, body, "webhookUrl", &h.cfg.WebhookUrl, "webhook_url")

	// Notify: Bark
	if err := applyBoolSettingDB(h.db, body, "barkEnabled", &h.cfg.BarkEnabled, "bark_enabled"); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}
	applyStringSetting(h.db, body, "barkUrl", &h.cfg.BarkUrl, "bark_url")

	// Notify: ServerChan
	if err := applyBoolSettingDB(h.db, body, "serverChanEnabled", &h.cfg.ServerChanEnabled, "serverchan_enabled"); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}
	applyStringSetting(h.db, body, "serverChanKey", &h.cfg.ServerChanKey, "serverchan_key")

	// Notify: Telegram
	if err := applyBoolSettingDB(h.db, body, "telegramEnabled", &h.cfg.TelegramEnabled, "telegram_enabled"); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}
	applyStringSetting(h.db, body, "telegramApiBaseUrl", &h.cfg.TelegramApiBaseUrl, "telegram_api_base_url")
	applyStringSetting(h.db, body, "telegramBotToken", &h.cfg.TelegramBotToken, "telegram_bot_token")
	applyStringSetting(h.db, body, "telegramChatId", &h.cfg.TelegramChatId, "telegram_chat_id")
	if err := applyBoolSettingDB(h.db, body, "telegramUseSystemProxy", &h.cfg.TelegramUseSystemProxy, "telegram_use_system_proxy"); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}
	applyStringSetting(h.db, body, "telegramMessageThreadId", &h.cfg.TelegramMessageThreadId, "telegram_message_thread_id")

	// Notify: SMTP
	if err := applyBoolSettingDB(h.db, body, "smtpEnabled", &h.cfg.SmtpEnabled, "smtp_enabled"); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}
	applyStringSetting(h.db, body, "smtpHost", &h.cfg.SmtpHost, "smtp_host")
	if v, ok := body["smtpPort"]; ok {
		n, err := toFloat64Strict(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "smtpPort 必须是数字类型")
		}
		h.cfg.SmtpPort = int(n)
		upsertSettingDB(h.db, "smtp_port", h.cfg.SmtpPort)
	}
	if err := applyBoolSettingDB(h.db, body, "smtpSecure", &h.cfg.SmtpSecure, "smtp_secure"); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}
	applyStringSetting(h.db, body, "smtpUser", &h.cfg.SmtpUser, "smtp_user")
	applyStringSetting(h.db, body, "smtpPass", &h.cfg.SmtpPass, "smtp_pass")
	applyStringSetting(h.db, body, "smtpFrom", &h.cfg.SmtpFrom, "smtp_from")
	applyStringSetting(h.db, body, "smtpTo", &h.cfg.SmtpTo, "smtp_to")

	// Notify: Feishu / DingTalk / WeCom / Ntfy
	if err := applyBoolSettingDB(h.db, body, "feishuEnabled", &h.cfg.FeishuEnabled, "feishu_enabled"); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}
	applyStringSetting(h.db, body, "feishuWebhook", &h.cfg.FeishuWebhook, "feishu_webhook")
	applyStringSetting(h.db, body, "feishuSecret", &h.cfg.FeishuSecret, "feishu_secret")
	if err := applyBoolSettingDB(h.db, body, "dingtalkEnabled", &h.cfg.DingtalkEnabled, "dingtalk_enabled"); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}
	applyStringSetting(h.db, body, "dingtalkWebhook", &h.cfg.DingtalkWebhook, "dingtalk_webhook")
	applyStringSetting(h.db, body, "dingtalkSecret", &h.cfg.DingtalkSecret, "dingtalk_secret")
	if err := applyBoolSettingDB(h.db, body, "wecomEnabled", &h.cfg.WecomEnabled, "wecom_enabled"); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}
	applyStringSetting(h.db, body, "wecomWebhook", &h.cfg.WecomWebhook, "wecom_webhook")
	if err := applyBoolSettingDB(h.db, body, "ntfyEnabled", &h.cfg.NtfyEnabled, "ntfy_enabled"); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}
	applyStringSetting(h.db, body, "ntfyUrl", &h.cfg.NtfyUrl, "ntfy_url")
	applyStringSetting(h.db, body, "ntfyTopic", &h.cfg.NtfyTopic, "ntfy_topic")
	applyStringSetting(h.db, body, "ntfyToken", &h.cfg.NtfyToken, "ntfy_token")

	// Per-alert-type mute toggles (JSON object).
	if v, ok := body["notifyTaskToggles"]; ok {
		if h.cfg.NotifyTaskToggles == nil {
			h.cfg.NotifyTaskToggles = map[string]bool{}
		}
		if raw, mErr := json.Marshal(v); mErr == nil {
			upsertSettingDB(h.db, "notify_task_toggles", v)
			// Re-hydrate from the marshaled value so runtime matches persisted.
			fresh := map[string]bool{}
			if uErr := json.Unmarshal(raw, &fresh); uErr == nil {
				h.cfg.NotifyTaskToggles = fresh
			}
		}
	}

	// Notify cooldown
	if v, ok := body["notifyCooldownSec"]; ok {
		n, err := toFloat64Strict(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "notifyCooldownSec 必须是数字类型")
		}
		if n < 0 {
			return failSettings(http.StatusBadRequest, "告警冷静期必须是大于等于 0 的数字（秒）")
		}
		h.cfg.NotifyCooldownSec = int(n)
		upsertSettingDB(h.db, "notify_cooldown_sec", int(n))
	}

	return nil
}

// applyFilterSettings applies the list/rule filters.
func (h *settingsHandler) applyFilterSettings(body map[string]any) *settingsApplyError {
	// Admin IP allowlist
	if v, ok := body["adminIpAllowlist"]; ok {
		var list []string
		switch val := v.(type) {
		case []any:
			for _, item := range val {
				if s, ok2 := item.(string); ok2 {
					list = append(list, strings.TrimSpace(s))
				}
			}
		case string:
			list = strings.Split(val, ",")
			for i := range list {
				list[i] = strings.TrimSpace(list[i])
			}
		}
		h.cfg.AdminIpAllowlist = list
		upsertSettingDB(h.db, "admin_ip_allowlist", list)
	}

	// Global blocked brands
	if v, ok := body["globalBlockedBrands"]; ok {
		brands, err := parseStringArraySetting(v, "globalBlockedBrands")
		if err != nil {
			return failSettings(http.StatusBadRequest, err.Error())
		}
		h.cfg.GlobalBlockedBrands = brands
		if err := upsertSettingDB(h.db, "global_blocked_brands", brands); err != nil {
			return failSettings(http.StatusInternalServerError, "保存品牌屏蔽失败")
		}
	}

	// Global allowed models
	// Only an explicit array (including []) may mutate this setting. null /
	// wrong types used to silently wipe the allowlist (upstream).
	if v, ok := body["globalAllowedModels"]; ok {
		models, err := parseStringArraySetting(v, "globalAllowedModels")
		if err != nil {
			return failSettings(http.StatusBadRequest, err.Error())
		}
		h.cfg.GlobalAllowedModels = models
		if err := upsertSettingDB(h.db, "global_allowed_models", models); err != nil {
			return failSettings(http.StatusInternalServerError, "保存模型白名单失败")
		}
	}

	// Proxy error keywords
	if v, ok := body["proxyErrorKeywords"]; ok {
		var keywords []string
		switch val := v.(type) {
		case []any:
			for _, item := range val {
				if s, ok2 := item.(string); ok2 {
					keywords = append(keywords, strings.TrimSpace(s))
				}
			}
		case string:
			for _, kw := range strings.Split(val, ",") {
				kw = strings.TrimSpace(kw)
				if kw != "" {
					keywords = append(keywords, kw)
				}
			}
		}
		h.cfg.ProxyErrorKeywords = keywords
		upsertSettingDB(h.db, "proxy_error_keywords", keywords)
	}

	// Payload rules
	if v, ok := body["payloadRules"]; ok {
		h.cfg.PayloadRules = v
		upsertSettingDB(h.db, "payload_rules", v)
	}

	return nil
}

// applySiteBrandingSettings applies the site branding fields.
func (h *settingsHandler) applySiteBrandingSettings(body map[string]any) *settingsApplyError {
	// Site & Branding
	applyStringSetting(h.db, body, "systemName", &h.cfg.SystemName, "system_name")
	applyStringSetting(h.db, body, "logo", &h.cfg.Logo, "logo")
	applyStringSetting(h.db, body, "footer", &h.cfg.Footer, "footer")
	applyStringSetting(h.db, body, "about", &h.cfg.About, "about")
	applyStringSetting(h.db, body, "homePageContent", &h.cfg.HomePageContent, "home_page_content")
	applyStringSetting(h.db, body, "serverAddress", &h.cfg.ServerAddress, "server_address")

	return nil
}
