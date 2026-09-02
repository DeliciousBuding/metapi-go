package admin

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/app"
	"github.com/deliciousbuding/metapi-go/config"
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

// applyStringSettingDB handles the common "simple string field" pattern:
// normalize the body value, publish it into the runtime snapshot via
// UpdateRuntime (copy-on-write), and persist it. Returns the upsert error so
// callers can surface it as HTTP 500.
func applyStringSettingDB(db *sqlx.DB, body map[string]any, key string, dbKey string, set func(*config.RuntimeSettings, string)) error {
	if v, ok := body[key]; ok {
		val := normalizeString(v)
		config.UpdateRuntime(func(r *config.RuntimeSettings) { set(r, val) })
		if err := upsertSettingDB(db, dbKey, val); err != nil {
			return err
		}
	}
	return nil
}

// applyProxyAccessSettings applies the proxy token and system proxy URL.
func (h *settingsHandler) applyProxyAccessSettings(body map[string]any) *settingsApplyError {
	// Proxy token
	if v, ok := body["proxyToken"]; ok {
		token := normalizeString(v)
		if !strings.HasPrefix(token, "sk-") {
			return failSettings(http.StatusBadRequest, "downstream access token must start with sk-")
		}
		if len(token) < 6 {
			return failSettings(http.StatusBadRequest, "downstream access token must be at least 6 characters (including sk-)")
		}
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.ProxyToken = token })
		upsertSettingDB(h.db, "proxy_token", token)
	}

	// System proxy URL
	if err := applyStringSettingDB(h.db, body, "systemProxyUrl", "system_proxy_url", func(r *config.RuntimeSettings, v string) { r.SystemProxyUrl = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}

	return nil
}

// applyCheckinSettings applies the checkin schedule fields.
func (h *settingsHandler) applyCheckinSettings(body map[string]any) *settingsApplyError {
	// #1027: global check-in kill switch. Persisted to the checkin_enabled
	// setting and hot-applied to the running scheduler independently of the
	// schedule fields below.
	if v, ok := body["checkinEnabled"]; ok {
		enabled, err := toBoolStrict(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "checkinEnabled must be a boolean (true/false)")
		}
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.CheckinDisabled = !enabled })
		if err := upsertSettingDB(h.db, "checkin_enabled", enabled); err != nil {
			return failSettings(http.StatusInternalServerError, "failed to save checkinEnabled")
		}
		app.UpdateCheckinEnabled(enabled)
	}
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
				return failSettings(http.StatusBadRequest, "checkinIntervalHours must be a number")
			}
			if hours != float64(int(hours)) {
				return failSettings(http.StatusBadRequest, "check-in interval must be an integer number of hours between 1 and 24")
			}
			intervalHours := int(hours)
			patch.IntervalHours = &intervalHours
		}
		if _, err := applyCheckinScheduleSettings(h.db, config.Runtime(), patch); err != nil {
			return failSettings(http.StatusBadRequest, err.Error())
		}
	}
	if v, ok := body["checkinSchedule"]; ok {
		spec, err := decodeScheduleSpec(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "invalid checkinSchedule format")
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
		if _, err := applyCheckinScheduleSettings(h.db, config.Runtime(), patch); err != nil {
			return failSettings(http.StatusBadRequest, err.Error())
		}
	}
	return nil
}

// applyBalanceScheduleSettings applies the balance refresh cron/schedule.
func (h *settingsHandler) applyBalanceScheduleSettings(body map[string]any) *settingsApplyError {
	// #1027: global balance-refresh kill switch. Persisted to the
	// balance_refresh_enabled setting and hot-applied to the running
	// scheduler independently of the cron fields below.
	if v, ok := body["balanceRefreshEnabled"]; ok {
		enabled, err := toBoolStrict(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "balanceRefreshEnabled must be a boolean (true/false)")
		}
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.BalanceRefreshDisabled = !enabled })
		if err := upsertSettingDB(h.db, "balance_refresh_enabled", enabled); err != nil {
			return failSettings(http.StatusInternalServerError, "failed to save balanceRefreshEnabled")
		}
		if err := app.UpdateBalanceEnabled(enabled); err != nil {
			slog.Warn("settings: balance refresh enable hot update failed", "error", err)
		}
	}
	// Balance refresh cron
	if v, ok := body["balanceRefreshCron"]; ok {
		cron := normalizeString(v)
		if !scheduler.ValidateCronExpr(cron) {
			return failSettings(http.StatusBadRequest, "balanceRefreshCron is not a valid cron expression")
		}
		if err := persistDualSchedule(h.db, "balance_refresh_cron", cron, "balance_refresh_schedule_v2", scheduler.CronToSchedule(cron)); err != nil {
			return failSettings(http.StatusInternalServerError, "failed to save balance refresh schedule")
		}
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.BalanceRefreshCron = cron })
		if err := app.UpdateBalanceCron(cron); err != nil {
			slog.Warn("settings: balance refresh cron hot update failed", "error", err)
		}
	}
	if v, ok := body["balanceRefreshSchedule"]; ok {
		spec, err := decodeScheduleSpec(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "invalid balanceRefreshSchedule format")
		}
		if err := spec.Validate(); err != nil {
			return failSettings(http.StatusBadRequest, err.Error())
		}
		cron, err := scheduler.ScheduleToCron(spec)
		if err != nil {
			return failSettings(http.StatusBadRequest, err.Error())
		}
		if cron == "" {
			return failSettings(http.StatusBadRequest, "balanceRefreshSchedule cannot be converted to a cron expression")
		}
		if err := persistDualSchedule(h.db, "balance_refresh_cron", cron, "balance_refresh_schedule_v2", spec); err != nil {
			return failSettings(http.StatusInternalServerError, "failed to save balance refresh schedule")
		}
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.BalanceRefreshCron = cron })
		if err := app.UpdateBalanceCron(cron); err != nil {
			slog.Warn("settings: balance refresh cron hot update failed", "error", err)
		}
	}
	return nil
}

// applyModelSyncScheduleSettings applies the periodic model-sync cron (#1005).
// Plain persistence (no v2 dual schedule mirror — that is legacy migration
// baggage the new setting never had): validate, persist model_sync_cron,
// update config, hot-reload the running scheduler.
func (h *settingsHandler) applyModelSyncScheduleSettings(body map[string]any) *settingsApplyError {
	if v, ok := body["modelSyncCron"]; ok {
		cron := normalizeString(v)
		if !scheduler.ValidateCronExpr(cron) {
			return failSettings(http.StatusBadRequest, "modelSyncCron is not a valid cron expression")
		}
		if err := upsertSettingDB(h.db, "model_sync_cron", cron); err != nil {
			return failSettings(http.StatusInternalServerError, "failed to save model sync schedule")
		}
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.ModelSyncCron = cron })
		if err := app.UpdateModelSyncCron(cron); err != nil {
			slog.Warn("settings: model sync cron hot update failed", "error", err)
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
			return failSettings(http.StatusBadRequest, "logCleanupCron is not a valid cron expression")
		}
		if err := persistDualSchedule(h.db, "log_cleanup_cron", cron, "log_cleanup_schedule_v2", scheduler.CronToSchedule(cron)); err != nil {
			return failSettings(http.StatusInternalServerError, "failed to save log cleanup schedule")
		}
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.LogCleanupCron = cron })
		logCleanupChanged = true
	}
	if v, ok := body["logCleanupUsageLogsEnabled"]; ok {
		enabled, err := toBoolStrict(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "logCleanupUsageLogsEnabled must be a boolean (true/false)")
		}
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.LogCleanupUsageLogsEnabled = enabled })
		upsertSettingDB(h.db, "log_cleanup_usage_logs_enabled", enabled)
	}
	if v, ok := body["logCleanupProgramLogsEnabled"]; ok {
		enabled, err := toBoolStrict(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "logCleanupProgramLogsEnabled must be a boolean (true/false)")
		}
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.LogCleanupProgramLogsEnabled = enabled })
		upsertSettingDB(h.db, "log_cleanup_program_logs_enabled", enabled)
	}
	if v, ok := body["logCleanupRetentionDays"]; ok {
		days, err := toFloat64Strict(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "logCleanupRetentionDays must be a number")
		}
		if days < 1 {
			return failSettings(http.StatusBadRequest, "log cleanup retention days must be an integer >= 1")
		}
		retentionDays := int(days)
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.LogCleanupRetentionDays = retentionDays })
		upsertSettingDB(h.db, "log_cleanup_retention_days", retentionDays)
	}
	if v, ok := body["logCleanupSchedule"]; ok {
		spec, err := decodeScheduleSpec(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "invalid logCleanupSchedule format")
		}
		if err := spec.Validate(); err != nil {
			return failSettings(http.StatusBadRequest, err.Error())
		}
		cron, err := scheduler.ScheduleToCron(spec)
		if err != nil {
			return failSettings(http.StatusBadRequest, err.Error())
		}
		if cron == "" {
			return failSettings(http.StatusBadRequest, "logCleanupSchedule cannot be converted to a cron expression")
		}
		if err := persistDualSchedule(h.db, "log_cleanup_cron", cron, "log_cleanup_schedule_v2", spec); err != nil {
			return failSettings(http.StatusInternalServerError, "failed to save log cleanup schedule")
		}
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.LogCleanupCron = cron })
		logCleanupChanged = true
	}
	if logCleanupChanged {
		// One snapshot for the whole hot-reload argument set.
		rt := config.Runtime()
		if err := app.UpdateLogCleanupSettings(rt.LogCleanupCron, rt.LogCleanupUsageLogsEnabled, rt.LogCleanupProgramLogsEnabled, rt.LogCleanupRetentionDays); err != nil {
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
			return failSettings(http.StatusBadRequest, "modelAvailabilityProbeEnabled must be a boolean (true/false)")
		}
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.ModelAvailabilityProbeEnabled = enabled })
		upsertSettingDB(h.db, "model_availability_probe_enabled", enabled)
		// #1027: hot-apply the toggle to the running ticker. Before this the
		// flag was only honored at scheduler Start, so toggling it off from
		// the settings page kept probing until a restart.
		if sched := scheduler.GetGlobalModelProbeScheduler(); sched != nil {
			if err := sched.SetEnabled(enabled); err != nil {
				slog.Warn("settings: model probe hot toggle failed", "error", err)
			}
		}
	}

	// Codex upstream websocket
	if v, ok := body["codexUpstreamWebsocketEnabled"]; ok {
		enabled, err := toBoolStrict(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "codexUpstreamWebsocketEnabled must be a boolean (true/false)")
		}
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.CodexUpstreamWebsocketEnabled = enabled })
		upsertSettingDB(h.db, "codex_upstream_websocket_enabled", enabled)
	}

	// Responses compact fallback
	if v, ok := body["responsesCompactFallbackToResponsesEnabled"]; ok {
		enabled, err := toBoolStrict(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "responsesCompactFallbackToResponsesEnabled must be a boolean (true/false)")
		}
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.ResponsesCompactFallbackToResponsesEnabled = enabled })
		upsertSettingDB(h.db, "responses_compact_fallback_to_responses_enabled", enabled)
	}

	// Cross protocol fallback
	if v, ok := body["disableCrossProtocolFallback"]; ok {
		enabled, err := toBoolStrict(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "disableCrossProtocolFallback must be a boolean (true/false)")
		}
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.DisableCrossProtocolFallback = enabled })
		upsertSettingDB(h.db, "disable_cross_protocol_fallback", enabled)
	}

	// Proxy empty content fail
	if err := applyBoolSettingDB(h.db, body, "proxyEmptyContentFailEnabled", "proxy_empty_content_fail_enabled", func(r *config.RuntimeSettings, v bool) { r.ProxyEmptyContentFailEnabled = v }); err != nil {
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
			return failSettings(http.StatusBadRequest, "proxySessionChannelConcurrencyLimit must be a number")
		}
		if n < 0 {
			return failSettings(http.StatusBadRequest, "session channel concurrency limit must be an integer >= 0")
		}
		limit := int(n)
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.ProxySessionChannelConcurrencyLimit = limit })
		upsertSettingDB(h.db, "proxy_session_channel_concurrency_limit", limit)
	}
	if v, ok := body["proxySessionChannelQueueWaitMs"]; ok {
		n, err := toFloat64Strict(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "proxySessionChannelQueueWaitMs must be a number")
		}
		if n < 0 {
			return failSettings(http.StatusBadRequest, "session channel queue wait time must be an integer number of milliseconds >= 0")
		}
		waitMs := int(n)
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.ProxySessionChannelQueueWaitMs = waitMs })
		upsertSettingDB(h.db, "proxy_session_channel_queue_wait_ms", waitMs)
	}
	return nil
}

// applyProxyDebugSettings applies the proxy debug trace settings.
func (h *settingsHandler) applyProxyDebugSettings(body map[string]any) *settingsApplyError {
	// Debug settings
	if err := applyBoolSettingDB(h.db, body, "proxyDebugTraceEnabled", "proxy_debug_trace_enabled", func(r *config.RuntimeSettings, v bool) { r.ProxyDebugTraceEnabled = v }); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}
	if err := applyBoolSettingDB(h.db, body, "proxyDebugCaptureHeaders", "proxy_debug_capture_headers", func(r *config.RuntimeSettings, v bool) { r.ProxyDebugCaptureHeaders = v }); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}
	if err := applyBoolSettingDB(h.db, body, "proxyDebugCaptureBodies", "proxy_debug_capture_bodies", func(r *config.RuntimeSettings, v bool) { r.ProxyDebugCaptureBodies = v }); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}
	if err := applyBoolSettingDB(h.db, body, "proxyDebugCaptureStreamChunks", "proxy_debug_capture_stream_chunks", func(r *config.RuntimeSettings, v bool) { r.ProxyDebugCaptureStreamChunks = v }); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}

	if err := applyStringSettingDB(h.db, body, "proxyDebugTargetSessionId", "proxy_debug_target_session_id", func(r *config.RuntimeSettings, v string) { r.ProxyDebugTargetSessionId = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "proxyDebugTargetClientKind", "proxy_debug_target_client_kind", func(r *config.RuntimeSettings, v string) { r.ProxyDebugTargetClientKind = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "proxyDebugTargetModel", "proxy_debug_target_model", func(r *config.RuntimeSettings, v string) { r.ProxyDebugTargetModel = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}

	if v, ok := body["proxyDebugRetentionHours"]; ok {
		n, err := toFloat64Strict(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "proxyDebugRetentionHours must be a number")
		}
		if n < 1 {
			return failSettings(http.StatusBadRequest, "proxy debug retention hours must be an integer >= 1")
		}
		hours := int(n)
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.ProxyDebugRetentionHours = hours })
		upsertSettingDB(h.db, "proxy_debug_retention_hours", hours)
	}
	if v, ok := body["proxyDebugMaxBodyBytes"]; ok {
		n, err := toFloat64Strict(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "proxyDebugMaxBodyBytes must be a number")
		}
		if n < 1024 {
			return failSettings(http.StatusBadRequest, "proxy debug capture size limit must be an integer >= 1024 bytes")
		}
		maxBytes := int(n)
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.ProxyDebugMaxBodyBytes = maxBytes })
		upsertSettingDB(h.db, "proxy_debug_max_body_bytes", maxBytes)
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
			return failSettings(http.StatusBadRequest, "routingFallbackUnitCost must be a number")
		}
		if n <= 0 {
			return failSettings(http.StatusBadRequest, "default unit cost for unpriced models must be a number greater than 0")
		}
		if n < 1e-6 {
			n = 1e-6
		}
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.RoutingFallbackUnitCost = n })
		upsertSettingDB(h.db, "routing_fallback_unit_cost", n)
	}
	if v, ok := body["proxyFirstByteTimeoutSec"]; ok {
		n, err := toFloat64Strict(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "proxyFirstByteTimeoutSec must be a number")
		}
		if n < 0 {
			return failSettings(http.StatusBadRequest, "first byte timeout must be a number >= 0 (seconds)")
		}
		timeoutSec := int(n)
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.ProxyFirstByteTimeoutSec = timeoutSec })
		upsertSettingDB(h.db, "proxy_first_byte_timeout_sec", timeoutSec)
	}
	if v, ok := body["tokenRouterFailureCooldownMaxSec"]; ok {
		n, err := toFloat64Strict(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "tokenRouterFailureCooldownMaxSec must be a number")
		}
		if n <= 0 {
			return failSettings(http.StatusBadRequest, "route failure cooldown cap must be a number > 0 (seconds)")
		}
		cooldownSec := int(n)
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.TokenRouterFailureCooldownMaxSec = cooldownSec })
		upsertSettingDB(h.db, "token_router_failure_cooldown_max_sec", cooldownSec)
	}
	// P1-2: operator-tunable status-code range policy. Empty specs are
	// allowed (they restore the routing defaults at lookup time); anything
	// non-empty must parse cleanly before it is persisted or applied.
	if v, ok := body["proxyRetryStatusRanges"]; ok {
		spec := normalizeString(v)
		if _, err := routing.ParseStatusRanges(spec); err != nil {
			return failSettings(http.StatusBadRequest, "proxyRetryStatusRanges: "+err.Error())
		}
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.ProxyRetryStatusRanges = spec })
		upsertSettingDB(h.db, "proxy_retry_status_ranges", spec)
	}
	if v, ok := body["proxyDisableStatusRanges"]; ok {
		spec := normalizeString(v)
		if _, err := routing.ParseStatusRanges(spec); err != nil {
			return failSettings(http.StatusBadRequest, "proxyDisableStatusRanges: "+err.Error())
		}
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.ProxyDisableStatusRanges = spec })
		upsertSettingDB(h.db, "proxy_disable_status_ranges", spec)
	}

	// Routing weights — validate every supplied component first, then publish
	// the merged weight set in ONE snapshot update so readers never observe a
	// half-applied weight vector.
	if v, ok := body["routingWeights"]; ok {
		if rw, ok2 := v.(map[string]any); ok2 {
			var baseWeightFactor, valueScoreFactor, costWeight, balanceWeight, usageWeight *float64
			if bf, ok3 := rw["baseWeightFactor"]; ok3 {
				val, err := toFloat64Strict(bf)
				if err != nil {
					return failSettings(http.StatusBadRequest, "routingWeights.baseWeightFactor must be a number")
				}
				baseWeightFactor = &val
			}
			if vsf, ok3 := rw["valueScoreFactor"]; ok3 {
				val, err := toFloat64Strict(vsf)
				if err != nil {
					return failSettings(http.StatusBadRequest, "routingWeights.valueScoreFactor must be a number")
				}
				valueScoreFactor = &val
			}
			if cw, ok3 := rw["costWeight"]; ok3 {
				val, err := toFloat64Strict(cw)
				if err != nil {
					return failSettings(http.StatusBadRequest, "routingWeights.costWeight must be a number")
				}
				costWeight = &val
			}
			if bw, ok3 := rw["balanceWeight"]; ok3 {
				val, err := toFloat64Strict(bw)
				if err != nil {
					return failSettings(http.StatusBadRequest, "routingWeights.balanceWeight must be a number")
				}
				balanceWeight = &val
			}
			if uw, ok3 := rw["usageWeight"]; ok3 {
				val, err := toFloat64Strict(uw)
				if err != nil {
					return failSettings(http.StatusBadRequest, "routingWeights.usageWeight must be a number")
				}
				usageWeight = &val
			}
			config.UpdateRuntime(func(r *config.RuntimeSettings) {
				if baseWeightFactor != nil {
					r.RoutingWeights.BaseWeightFactor = *baseWeightFactor
				}
				if valueScoreFactor != nil {
					r.RoutingWeights.ValueScoreFactor = *valueScoreFactor
				}
				if costWeight != nil {
					r.RoutingWeights.CostWeight = *costWeight
				}
				if balanceWeight != nil {
					r.RoutingWeights.BalanceWeight = *balanceWeight
				}
				if usageWeight != nil {
					r.RoutingWeights.UsageWeight = *usageWeight
				}
			})
			upsertSettingDB(h.db, "routing_weights", config.Runtime().RoutingWeights)
		}
	}

	// N7: prompt-cache ratio fallback overrides.
	if v, ok := body["cacheRatioDefault"]; ok {
		if val, err := toFloat64Strict(v); err == nil && val >= 0 {
			config.UpdateRuntime(func(r *config.RuntimeSettings) { r.CacheRatioDefault = val })
			upsertSettingDB(h.db, "cache_ratio_default", val)
		}
	}
	if v, ok := body["cacheRatioClaude"]; ok {
		if val, err := toFloat64Strict(v); err == nil && val >= 0 {
			config.UpdateRuntime(func(r *config.RuntimeSettings) { r.CacheRatioClaude = val })
			upsertSettingDB(h.db, "cache_ratio_claude", val)
		}
	}
	// Apply to routing runtime immediately (0 / non-positive falls back to code default).
	rt := config.Runtime()
	routing.SetCacheRatioDefaults(rt.CacheRatioDefault, 0, rt.CacheRatioClaude, 0)

	return nil
}

// applyNotifySettings applies the notification channel fields.
func (h *settingsHandler) applyNotifySettings(body map[string]any) *settingsApplyError {
	// Notify: Webhook
	if err := applyBoolSettingDB(h.db, body, "webhookEnabled", "webhook_enabled", func(r *config.RuntimeSettings, v bool) { r.WebhookEnabled = v }); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "webhookUrl", "webhook_url", func(r *config.RuntimeSettings, v string) { r.WebhookUrl = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}

	// Notify: Bark
	if err := applyBoolSettingDB(h.db, body, "barkEnabled", "bark_enabled", func(r *config.RuntimeSettings, v bool) { r.BarkEnabled = v }); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "barkUrl", "bark_url", func(r *config.RuntimeSettings, v string) { r.BarkUrl = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}

	// Notify: ServerChan
	if err := applyBoolSettingDB(h.db, body, "serverChanEnabled", "serverchan_enabled", func(r *config.RuntimeSettings, v bool) { r.ServerChanEnabled = v }); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "serverChanKey", "serverchan_key", func(r *config.RuntimeSettings, v string) { r.ServerChanKey = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}

	// Notify: Telegram
	if err := applyBoolSettingDB(h.db, body, "telegramEnabled", "telegram_enabled", func(r *config.RuntimeSettings, v bool) { r.TelegramEnabled = v }); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "telegramApiBaseUrl", "telegram_api_base_url", func(r *config.RuntimeSettings, v string) { r.TelegramApiBaseUrl = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "telegramBotToken", "telegram_bot_token", func(r *config.RuntimeSettings, v string) { r.TelegramBotToken = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "telegramChatId", "telegram_chat_id", func(r *config.RuntimeSettings, v string) { r.TelegramChatId = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}
	if err := applyBoolSettingDB(h.db, body, "telegramUseSystemProxy", "telegram_use_system_proxy", func(r *config.RuntimeSettings, v bool) { r.TelegramUseSystemProxy = v }); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "telegramMessageThreadId", "telegram_message_thread_id", func(r *config.RuntimeSettings, v string) { r.TelegramMessageThreadId = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}

	// Notify: SMTP
	if err := applyBoolSettingDB(h.db, body, "smtpEnabled", "smtp_enabled", func(r *config.RuntimeSettings, v bool) { r.SmtpEnabled = v }); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "smtpHost", "smtp_host", func(r *config.RuntimeSettings, v string) { r.SmtpHost = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}
	if v, ok := body["smtpPort"]; ok {
		n, err := toFloat64Strict(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "smtpPort must be a number")
		}
		port := int(n)
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.SmtpPort = port })
		upsertSettingDB(h.db, "smtp_port", port)
	}
	if err := applyBoolSettingDB(h.db, body, "smtpSecure", "smtp_secure", func(r *config.RuntimeSettings, v bool) { r.SmtpSecure = v }); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "smtpUser", "smtp_user", func(r *config.RuntimeSettings, v string) { r.SmtpUser = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "smtpPass", "smtp_pass", func(r *config.RuntimeSettings, v string) { r.SmtpPass = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "smtpFrom", "smtp_from", func(r *config.RuntimeSettings, v string) { r.SmtpFrom = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "smtpTo", "smtp_to", func(r *config.RuntimeSettings, v string) { r.SmtpTo = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}

	// Notify: Feishu / DingTalk / WeCom / Ntfy
	if err := applyBoolSettingDB(h.db, body, "feishuEnabled", "feishu_enabled", func(r *config.RuntimeSettings, v bool) { r.FeishuEnabled = v }); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "feishuWebhook", "feishu_webhook", func(r *config.RuntimeSettings, v string) { r.FeishuWebhook = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "feishuSecret", "feishu_secret", func(r *config.RuntimeSettings, v string) { r.FeishuSecret = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}
	if err := applyBoolSettingDB(h.db, body, "dingtalkEnabled", "dingtalk_enabled", func(r *config.RuntimeSettings, v bool) { r.DingtalkEnabled = v }); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "dingtalkWebhook", "dingtalk_webhook", func(r *config.RuntimeSettings, v string) { r.DingtalkWebhook = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "dingtalkSecret", "dingtalk_secret", func(r *config.RuntimeSettings, v string) { r.DingtalkSecret = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}
	if err := applyBoolSettingDB(h.db, body, "wecomEnabled", "wecom_enabled", func(r *config.RuntimeSettings, v bool) { r.WecomEnabled = v }); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "wecomWebhook", "wecom_webhook", func(r *config.RuntimeSettings, v string) { r.WecomWebhook = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}
	if err := applyBoolSettingDB(h.db, body, "ntfyEnabled", "ntfy_enabled", func(r *config.RuntimeSettings, v bool) { r.NtfyEnabled = v }); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "ntfyUrl", "ntfy_url", func(r *config.RuntimeSettings, v string) { r.NtfyUrl = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "ntfyTopic", "ntfy_topic", func(r *config.RuntimeSettings, v string) { r.NtfyTopic = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "ntfyToken", "ntfy_token", func(r *config.RuntimeSettings, v string) { r.NtfyToken = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}

	// Per-alert-type mute toggles (JSON object). The map is replaced
	// wholesale on the draft — published snapshots share map references, so
	// in-place mutation is forbidden by the RuntimeSettings contract.
	if v, ok := body["notifyTaskToggles"]; ok {
		if raw, mErr := json.Marshal(v); mErr == nil {
			upsertSettingDB(h.db, "notify_task_toggles", v)
			// Re-hydrate from the marshaled value so runtime matches persisted.
			fresh := map[string]bool{}
			if uErr := json.Unmarshal(raw, &fresh); uErr == nil {
				config.UpdateRuntime(func(r *config.RuntimeSettings) { r.NotifyTaskToggles = fresh })
			}
		}
	}

	// Notify cooldown
	if v, ok := body["notifyCooldownSec"]; ok {
		n, err := toFloat64Strict(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "notifyCooldownSec must be a number")
		}
		if n < 0 {
			return failSettings(http.StatusBadRequest, "alert cooldown must be a number >= 0 (seconds)")
		}
		cooldownSec := int(n)
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.NotifyCooldownSec = cooldownSec })
		upsertSettingDB(h.db, "notify_cooldown_sec", cooldownSec)
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
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.AdminIpAllowlist = list })
		// A failed persist must not look like success: the allowlist would be
		// live until the next restart and then silently revert to the env
		// value, re-opening the admin API to every IP.
		if err := upsertSettingDB(h.db, "admin_ip_allowlist", list); err != nil {
			return failSettings(http.StatusInternalServerError, "failed to save admin IP allowlist")
		}
	}

	// Global blocked brands
	if v, ok := body["globalBlockedBrands"]; ok {
		brands, err := parseStringArraySetting(v, "globalBlockedBrands")
		if err != nil {
			return failSettings(http.StatusBadRequest, err.Error())
		}
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.GlobalBlockedBrands = brands })
		if err := upsertSettingDB(h.db, "global_blocked_brands", brands); err != nil {
			return failSettings(http.StatusInternalServerError, "failed to save brand blocking")
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
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.GlobalAllowedModels = models })
		if err := upsertSettingDB(h.db, "global_allowed_models", models); err != nil {
			return failSettings(http.StatusInternalServerError, "failed to save model allowlist")
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
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.ProxyErrorKeywords = keywords })
		// Same reasoning as the allowlist above: the two neighbouring filters
		// in this function already report persist failures.
		if err := upsertSettingDB(h.db, "proxy_error_keywords", keywords); err != nil {
			return failSettings(http.StatusInternalServerError, "failed to save proxy error keywords")
		}
	}

	// Payload rules
	if v, ok := body["payloadRules"]; ok {
		rules := v
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.PayloadRules = rules })
		upsertSettingDB(h.db, "payload_rules", v)
	}

	return nil
}

// applySiteBrandingSettings applies the site branding fields.
func (h *settingsHandler) applySiteBrandingSettings(body map[string]any) *settingsApplyError {
	// Site & Branding
	if err := applyStringSettingDB(h.db, body, "systemName", "system_name", func(r *config.RuntimeSettings, v string) { r.SystemName = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "logo", "logo", func(r *config.RuntimeSettings, v string) { r.Logo = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "footer", "footer", func(r *config.RuntimeSettings, v string) { r.Footer = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "about", "about", func(r *config.RuntimeSettings, v string) { r.About = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "serverAddress", "server_address", func(r *config.RuntimeSettings, v string) { r.ServerAddress = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}

	return nil
}
