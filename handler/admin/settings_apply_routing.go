package admin

import (
	"log/slog"
	"net/http"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/scheduler"
	"github.com/deliciousbuding/metapi-go/service"
)

// applyProxyAccessSettings applies the proxy token and system proxy URL.
func (h *settingsHandler) applyProxyAccessSettings(body map[string]any) *settingsApplyError {
	if _, ok := body["proxyToken"]; ok {
		return failSettings(http.StatusGone, "Root downstream keys have moved to API keys. Manage access tokens at /downstream-keys.")
	}

	// System proxy URL: same scheme/host validation as the per-site, per-account
	// and per-key fields. Empty disables the system proxy and stays valid.
	if v, ok := body["systemProxyUrl"]; ok {
		if err := service.ValidateProxyURL(normalizeString(v)); err != nil {
			return failSettings(http.StatusBadRequest, invalidProxyURLMessage)
		}
	}
	if err := applyStringSettingDB(h.db, body, "systemProxyUrl", "system_proxy_url", func(r *config.RuntimeSettings, v string) { r.SystemProxyUrl = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
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
	// Operator-tunable status-code range policy. Empty specs are
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

	// Prompt-cache ratio fallback overrides.
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
