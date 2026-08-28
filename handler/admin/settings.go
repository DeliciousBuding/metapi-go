package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/scheduler"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

// RegisterSettingsRoutes registers all /api/settings routes (runtime, brand-list, system-proxy/test).
func RegisterSettingsRoutes(r chi.Router, db *sqlx.DB, cfg *config.Config) {
	handler := &settingsHandler{db: db, cfg: cfg}

	r.Get("/api/settings/runtime", handler.getRuntime)
	r.Put("/api/settings/runtime", handler.updateRuntime)
	r.Get("/api/settings/brand-list", handler.brandList)
	r.Post("/api/settings/system-proxy/test", handler.testSystemProxy)
	r.Get("/api/settings/migration/preview", handler.previewSettingsMigration)
	r.Post("/api/settings/migration/apply", handler.applySettingsMigration)
}

type settingsHandler struct {
	db  *sqlx.DB
	cfg *config.Config
}

// GET /api/settings/runtime
func (h *settingsHandler) getRuntime(w http.ResponseWriter, r *http.Request) {
	cfg := h.cfg
	writeJSON(w, http.StatusOK, map[string]any{
		// Checkin
		"checkinCron":          cfg.CheckinCron,
		"checkinScheduleMode":  cfg.CheckinScheduleMode,
		"checkinIntervalHours": cfg.CheckinIntervalHours,
		"checkinWindowStart":   cfg.CheckinWindowStart,
		"checkinWindowEnd":     cfg.CheckinWindowEnd,
		"checkinSchedule":      scheduleSpecForCheckin(cfg),
		"checkinEnabled":       !cfg.CheckinDisabled,
		// Site & Branding
		"systemName":    cfg.SystemName,
		"logo":          cfg.Logo,
		"footer":        cfg.Footer,
		"about":         cfg.About,
		"serverAddress": cfg.ServerAddress,
		// Balance
		"balanceRefreshCron":     cfg.BalanceRefreshCron,
		"balanceRefreshSchedule": scheduler.CronToSchedule(cfg.BalanceRefreshCron),
		"balanceRefreshEnabled":  !cfg.BalanceRefreshDisabled,
		// Model sync (#1005) — plain cron, no v2 schedule mirror
		"modelSyncCron": cfg.ModelSyncCron,
		// Log cleanup
		"logCleanupCron":               cfg.LogCleanupCron,
		"logCleanupSchedule":           scheduler.CronToSchedule(cfg.LogCleanupCron),
		"logCleanupUsageLogsEnabled":   cfg.LogCleanupUsageLogsEnabled,
		"logCleanupProgramLogsEnabled": cfg.LogCleanupProgramLogsEnabled,
		"logCleanupRetentionDays":      cfg.LogCleanupRetentionDays,
		// Model probe
		"modelAvailabilityProbeEnabled": cfg.ModelAvailabilityProbeEnabled,
		// Codex
		"codexUpstreamWebsocketEnabled": cfg.CodexUpstreamWebsocketEnabled,
		// Responses
		"responsesCompactFallbackToResponsesEnabled": cfg.ResponsesCompactFallbackToResponsesEnabled,
		// Cross-protocol
		"disableCrossProtocolFallback": cfg.DisableCrossProtocolFallback,
		// Proxy session
		"proxySessionChannelConcurrencyLimit": cfg.ProxySessionChannelConcurrencyLimit,
		"proxySessionChannelQueueWaitMs":      cfg.ProxySessionChannelQueueWaitMs,
		// Debug trace
		"proxyDebugTraceEnabled":        cfg.ProxyDebugTraceEnabled,
		"proxyDebugCaptureHeaders":      cfg.ProxyDebugCaptureHeaders,
		"proxyDebugCaptureBodies":       cfg.ProxyDebugCaptureBodies,
		"proxyDebugCaptureStreamChunks": cfg.ProxyDebugCaptureStreamChunks,
		"proxyDebugTargetSessionId":     cfg.ProxyDebugTargetSessionId,
		"proxyDebugTargetClientKind":    cfg.ProxyDebugTargetClientKind,
		"proxyDebugTargetModel":         cfg.ProxyDebugTargetModel,
		"proxyDebugRetentionHours":      cfg.ProxyDebugRetentionHours,
		"proxyDebugMaxBodyBytes":        cfg.ProxyDebugMaxBodyBytes,
		// Routing
		"routingFallbackUnitCost":          cfg.RoutingFallbackUnitCost,
		"proxyFirstByteTimeoutSec":         cfg.ProxyFirstByteTimeoutSec,
		"tokenRouterFailureCooldownMaxSec": cfg.TokenRouterFailureCooldownMaxSec,
		"proxyRetryStatusRanges":           cfg.ProxyRetryStatusRanges,
		"proxyDisableStatusRanges":         cfg.ProxyDisableStatusRanges,
		"routingWeights": map[string]any{
			"baseWeightFactor": cfg.RoutingWeights.BaseWeightFactor,
			"valueScoreFactor": cfg.RoutingWeights.ValueScoreFactor,
			"costWeight":       cfg.RoutingWeights.CostWeight,
			"balanceWeight":    cfg.RoutingWeights.BalanceWeight,
			"usageWeight":      cfg.RoutingWeights.UsageWeight,
		},
		// Notify: Webhook
		"webhookUrl":     cfg.WebhookUrl,
		"webhookEnabled": cfg.WebhookEnabled,
		// Notify: Bark
		"barkUrl":     cfg.BarkUrl,
		"barkEnabled": cfg.BarkEnabled,
		// Notify: ServerChan
		"serverChanEnabled":   cfg.ServerChanEnabled,
		"serverChanKeyMasked": maskValue(cfg.ServerChanKey),
		// Notify: Telegram
		"telegramEnabled":         cfg.TelegramEnabled,
		"telegramApiBaseUrl":      cfg.TelegramApiBaseUrl,
		"telegramBotTokenMasked":  maskValue(cfg.TelegramBotToken),
		"telegramChatId":          cfg.TelegramChatId,
		"telegramUseSystemProxy":  cfg.TelegramUseSystemProxy,
		"telegramMessageThreadId": cfg.TelegramMessageThreadId,
		// Notify: SMTP
		"smtpEnabled":    cfg.SmtpEnabled,
		"smtpHost":       cfg.SmtpHost,
		"smtpPort":       cfg.SmtpPort,
		"smtpSecure":     cfg.SmtpSecure,
		"smtpUser":       cfg.SmtpUser,
		"smtpPassMasked": maskValue(cfg.SmtpPass),
		"smtpFrom":       cfg.SmtpFrom,
		"smtpTo":         cfg.SmtpTo,
		// Notify: Feishu / DingTalk / WeCom / Ntfy
		"feishuEnabled":        cfg.FeishuEnabled,
		"feishuWebhook":        cfg.FeishuWebhook,
		"feishuSecretMasked":   maskValue(cfg.FeishuSecret),
		"dingtalkEnabled":      cfg.DingtalkEnabled,
		"dingtalkWebhook":      cfg.DingtalkWebhook,
		"dingtalkSecretMasked": maskValue(cfg.DingtalkSecret),
		"wecomEnabled":         cfg.WecomEnabled,
		"wecomWebhook":         cfg.WecomWebhook,
		"ntfyEnabled":          cfg.NtfyEnabled,
		"ntfyUrl":              cfg.NtfyUrl,
		"ntfyTopic":            cfg.NtfyTopic,
		"ntfyTokenMasked":      maskValue(cfg.NtfyToken),
		"notifyTaskToggles":    cfg.NotifyTaskToggles,
		// Notify: cooldown
		"notifyCooldownSec": cfg.NotifyCooldownSec,
		// Admin
		"adminIpAllowlist": cfg.AdminIpAllowlist,
		"currentAdminIp":   extractClientIP(r),
		"serverTimeZone":   cfg.Tz,
		// System
		"systemProxyUrl":   cfg.SystemProxyUrl,
		"proxyTokenMasked": maskValue(cfg.ProxyToken),
		// Proxy
		"payloadRules":                 cfg.PayloadRules,
		"proxyErrorKeywords":           cfg.ProxyErrorKeywords,
		"proxyEmptyContentFailEnabled": cfg.ProxyEmptyContentFailEnabled,
		// Global filters (always JSON arrays; never null)
		"globalBlockedBrands": stringSliceOrEmpty(cfg.GlobalBlockedBrands),
		"globalAllowedModels": stringSliceOrEmpty(cfg.GlobalAllowedModels),
		// N7: effective prompt-cache ratio fallbacks (reflect overrides).
		"cacheRatioDefault": routing.DefaultCacheRatioForModel("gpt-4o"),
		"cacheRatioClaude":  routing.DefaultCacheRatioForModel("claude-3-5-sonnet"),
	})
}

// PUT /api/settings/runtime
func (h *settingsHandler) updateRuntime(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := decodeJSONRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	for _, apply := range []func(map[string]any) *settingsApplyError{
		h.applyProxyAccessSettings,
		h.applyCheckinSettings,
		h.applyBalanceScheduleSettings,
		h.applyModelSyncScheduleSettings,
		h.applyLogCleanupSettings,
		h.applyFeatureToggleSettings,
		h.applyProxySessionSettings,
		h.applyProxyDebugSettings,
		h.applyRoutingSettings,
		h.applyNotifySettings,
		h.applyFilterSettings,
		h.applySiteBrandingSettings,
	} {
		if err := apply(body); err != nil {
			writeError(w, err.status, err.msg)
			return
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	logSettingsEvent(h.db, "status", "Runtime settings updated", "Runtime settings updated", "info", now)

	writeJSON(w, http.StatusOK, map[string]any{
		"success":             true,
		"message":             "Runtime settings updated",
		"globalAllowedModels": stringSliceOrEmpty(h.cfg.GlobalAllowedModels),
		"globalBlockedBrands": stringSliceOrEmpty(h.cfg.GlobalBlockedBrands),
	})
}

// GET /api/settings/brand-list
func (h *settingsHandler) brandList(w http.ResponseWriter, r *http.Request) {
	// Canonical registered platforms + a few UI-facing product brands.
	seen := map[string]bool{}
	brands := make([]string, 0, 16)
	for _, a := range platform.ListAdapters() {
		name := strings.TrimSpace(a.PlatformName())
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		brands = append(brands, name)
	}
	// Keep operator-facing client brands that are not adapters.
	for _, extra := range []string{"lobechat", "openwebui"} {
		if !seen[extra] {
			brands = append(brands, extra)
			seen[extra] = true
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"brands": brands})
}

// POST /api/settings/system-proxy/test
func (h *settingsHandler) testSystemProxy(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ProxyUrl  *string `json:"proxyUrl"`
		TargetUrl *string `json:"targetUrl"`
	}
	if err := decodeJSONRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	proxyURL := h.cfg.SystemProxyUrl
	if body.ProxyUrl != nil {
		proxyURL = strings.TrimSpace(*body.ProxyUrl)
	}
	if proxyURL == "" {
		writeError(w, http.StatusBadRequest, "enter the system proxy address first")
		return
	}

	target := "https://www.gstatic.com/generate_204"
	if body.TargetUrl != nil && strings.TrimSpace(*body.TargetUrl) != "" {
		target = strings.TrimSpace(*body.TargetUrl)
		// Guard operator-supplied probe targets against non-http(s) and
		// cloud metadata / link-local SSRF first-hops.
		if service.IsForbiddenSiteTargetURL(target) || !service.IsValidHTTPURL(target) {
			writeError(w, http.StatusBadRequest, "Invalid targetUrl. Cloud metadata / link-local targets are not allowed; expected a valid http(s) URL.")
			return
		}
	}

	result := probeSystemProxy(r.Context(), proxyURL, target)
	writeJSON(w, http.StatusOK, result)
}

// probeSystemProxy performs a bounded HTTP GET via the given proxy URL.
// Injectable for tests via systemProxyProbeFn.
var systemProxyProbeFn = defaultSystemProxyProbe

func probeSystemProxy(ctx context.Context, proxyURL, target string) map[string]any {
	return systemProxyProbeFn(ctx, proxyURL, target)
}

func defaultSystemProxyProbe(ctx context.Context, proxyURL, target string) map[string]any {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return map[string]any{
			"success":   false,
			"proxyUrl":  proxyURL,
			"targetUrl": target,
			"reachable": false,
			"ok":        false,
			"message":   err.Error(),
		}
	}
	started := time.Now()
	resp, err := platform.DoWithProxy(ctx, req, &platform.ProxyConfig{ProxyURL: proxyURL})
	latency := time.Since(started).Milliseconds()
	if err != nil {
		return map[string]any{
			"success":   false,
			"proxyUrl":  proxyURL,
			"targetUrl": target,
			"reachable": false,
			"ok":        false,
			"latencyMs": latency,
			"message":   err.Error(),
		}
	}
	defer resp.Body.Close()
	ok := resp.StatusCode >= 200 && resp.StatusCode < 400
	return map[string]any{
		"success":    ok,
		"proxyUrl":   proxyURL,
		"targetUrl":  target,
		"reachable":  true,
		"ok":         ok,
		"statusCode": resp.StatusCode,
		"latencyMs":  latency,
	}
}

func extractClientIP(r *http.Request) string {
	addr := r.RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx > 0 {
		return addr[:idx]
	}
	return addr
}

func normalizeString(v any) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func hasAnyKey(body map[string]any, keys ...string) bool {
	for _, key := range keys {
		if _, ok := body[key]; ok {
			return true
		}
	}
	return false
}

func toBool(v any) bool {
	switch val := v.(type) {
	case bool:
		return val
	case string:
		val = strings.ToLower(strings.TrimSpace(val))
		return val == "1" || val == "true" || val == "yes" || val == "on"
	case float64:
		return val != 0
	case int:
		return val != 0
	default:
		return false
	}
}

// toFloat64Strict converts numeric JSON values and rejects non-numeric types
// such as strings, booleans, or objects. This prevents silent zero-coercion
// that hides client-side type errors.
func toFloat64Strict(v any) (float64, error) {
	switch val := v.(type) {
	case float64:
		return val, nil
	case float32:
		return float64(val), nil
	case int:
		return float64(val), nil
	case int64:
		return float64(val), nil
	case json.Number:
		n, err := val.Float64()
		if err != nil {
			return 0, fmt.Errorf("invalid numeric value: %s", val.String())
		}
		return n, nil
	default:
		return 0, fmt.Errorf("expected a number, got %T", v)
	}
}

// toBoolStrict converts boolean JSON values and rejects non-boolean types
// such as strings, numbers, or objects. This prevents silent false-coercion
// that hides client-side type errors.
func toBoolStrict(v any) (bool, error) {
	if b, ok := v.(bool); ok {
		return b, nil
	}
	return false, fmt.Errorf("expected a boolean (true/false), got %T", v)
}

func applyBoolSettingDB(db *sqlx.DB, body map[string]any, key string, target *bool, dbKey string) error {
	if v, ok := body[key]; ok {
		val, err := toBoolStrict(v)
		if err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		*target = val
		if err := upsertSettingDB(db, dbKey, *target); err != nil {
			return err
		}
	}
	return nil
}

func upsertSettingDB(db *sqlx.DB, key string, value any) error {
	// Normalize nil string slices to empty arrays so we never persist JSON null
	// for list settings (null historically rehydrated as a wiped allowlist).
	if value == nil {
		value = []string{}
	}
	jsonValue, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("settings: marshal %q: %w", key, err)
	}
	query := db.Rebind(`INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`)
	if _, err := db.Exec(query, key, string(jsonValue)); err != nil {
		return fmt.Errorf("settings: upsert %q: %w", key, err)
	}
	return nil
}

// parseStringArraySetting validates an explicit JSON array setting.
// null / non-array values are rejected so accidental clients cannot wipe lists.
// Empty arrays are allowed and mean "clear / allow all" for the model whitelist.
func parseStringArraySetting(v any, field string) ([]string, error) {
	if v == nil {
		return nil, fmt.Errorf("%s must be an array of strings (use [] to clear)", field)
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array of strings", field)
	}
	out := make([]string, 0, len(arr))
	seen := make(map[string]struct{}, len(arr))
	for _, item := range arr {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("%s must be an array of strings", field)
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
	return out, nil
}

func stringSliceOrEmpty(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

func logSettingsEvent(db *sqlx.DB, eventType, title, message, level, createdAt string) {
	query := db.Rebind(`INSERT INTO events (type, title, message, level, related_type, created_at, "read")
		VALUES (?, ?, ?, ?, 'settings', ?, 0)`)
	if _, err := db.Exec(query, eventType, title, message, level, createdAt); err != nil {
		slog.Warn("settings: failed to log settings event", "type", eventType, "error", err)
	}
}

// decodeScheduleSpec converts a decoded JSON body value into a ScheduleSpec.
func decodeScheduleSpec(v any) (scheduler.ScheduleSpec, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return scheduler.ScheduleSpec{}, err
	}
	var spec scheduler.ScheduleSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return scheduler.ScheduleSpec{}, err
	}
	return spec, nil
}

// persistDualSchedule writes the legacy cron key and its v2 ScheduleSpec
// mirror in one transaction so the two never diverge.
func persistDualSchedule(db *sqlx.DB, legacyKey string, legacyValue any, v2Key string, spec scheduler.ScheduleSpec) error {
	tx, err := db.Beginx()
	if err != nil {
		return fmt.Errorf("settings: begin dual schedule write: %w", err)
	}
	defer tx.Rollback()
	if err := upsertSettingTx(db, tx, legacyKey, legacyValue); err != nil {
		return err
	}
	if cron, ok := legacyValue.(string); ok {
		spec.Cron = cron
	}
	if err := upsertSettingTx(db, tx, v2Key, spec); err != nil {
		return err
	}
	return tx.Commit()
}
