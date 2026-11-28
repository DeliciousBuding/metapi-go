package admin

import (
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/schedulespec"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// RegisterSettingsRoutes registers all /api/settings routes (runtime, brand-list, system-proxy/test).
func RegisterSettingsRoutes(r chi.Router, db *store.DB, cfg *config.Config) {
	handler := &settingsHandler{db: db, cfg: cfg}

	r.Get("/api/settings/runtime", handler.getRuntime)
	r.Put("/api/settings/runtime", handler.updateRuntime)
	r.Get("/api/settings/brand-list", handler.brandList)
	r.Post("/api/settings/system-proxy/test", handler.testSystemProxy)
	r.Get("/api/settings/migration/preview", handler.previewSettingsMigration)
	r.Post("/api/settings/migration/apply", handler.applySettingsMigration)
}

type settingsHandler struct {
	db  *store.DB
	cfg *config.Config
}

// GET /api/settings/runtime
func (h *settingsHandler) getRuntime(w http.ResponseWriter, r *http.Request) {
	// One atomic snapshot for the whole response: the admin UI can never
	// observe a half-applied settings update.
	rt := config.Runtime()
	cfg := h.cfg
	writeJSON(w, http.StatusOK, map[string]any{
		// Checkin
		"checkinCron":          rt.CheckinCron,
		"checkinScheduleMode":  rt.CheckinScheduleMode,
		"checkinIntervalHours": rt.CheckinIntervalHours,
		"checkinWindowStart":   rt.CheckinWindowStart,
		"checkinWindowEnd":     rt.CheckinWindowEnd,
		"checkinSchedule":      scheduleSpecForCheckin(rt),
		"checkinEnabled":       !rt.CheckinDisabled,
		// Site & Branding
		"systemName":    rt.SystemName,
		"logo":          rt.Logo,
		"footer":        rt.Footer,
		"about":         rt.About,
		"serverAddress": rt.ServerAddress,
		// Balance
		"balanceRefreshCron":     rt.BalanceRefreshCron,
		"balanceRefreshSchedule": schedulespec.CronToSchedule(rt.BalanceRefreshCron),
		"balanceRefreshEnabled":  !rt.BalanceRefreshDisabled,
		// Model sync (#1005) — plain cron, no v2 schedule mirror
		"modelSyncCron":         rt.ModelSyncCron,
		"autoCreateModelRoutes": rt.AutoCreateModelRoutes,
		// Log cleanup
		"logCleanupCron":               rt.LogCleanupCron,
		"logCleanupSchedule":           schedulespec.CronToSchedule(rt.LogCleanupCron),
		"logCleanupUsageLogsEnabled":   rt.LogCleanupUsageLogsEnabled,
		"logCleanupProgramLogsEnabled": rt.LogCleanupProgramLogsEnabled,
		"logCleanupRetentionDays":      rt.LogCleanupRetentionDays,
		// Model probe
		"modelAvailabilityProbeEnabled": rt.ModelAvailabilityProbeEnabled,
		// Codex
		"codexUpstreamWebsocketEnabled": rt.CodexUpstreamWebsocketEnabled,
		// Cross-protocol
		"disableCrossProtocolFallback": rt.DisableCrossProtocolFallback,
		// Proxy session
		"proxySessionChannelConcurrencyLimit": rt.ProxySessionChannelConcurrencyLimit,
		"proxySessionChannelQueueWaitMs":      rt.ProxySessionChannelQueueWaitMs,
		// Routing
		"routingFallbackUnitCost":          rt.RoutingFallbackUnitCost,
		"proxyFirstByteTimeoutSec":         rt.ProxyFirstByteTimeoutSec,
		"tokenRouterFailureCooldownMaxSec": rt.TokenRouterFailureCooldownMaxSec,
		"proxyRetryStatusRanges":           rt.ProxyRetryStatusRanges,
		"proxyDisableStatusRanges":         rt.ProxyDisableStatusRanges,
		"routingWeights": map[string]any{
			"baseWeightFactor": rt.RoutingWeights.BaseWeightFactor,
			"valueScoreFactor": rt.RoutingWeights.ValueScoreFactor,
			"costWeight":       rt.RoutingWeights.CostWeight,
			"balanceWeight":    rt.RoutingWeights.BalanceWeight,
			"usageWeight":      rt.RoutingWeights.UsageWeight,
		},
		// Notify: Webhook
		"webhookUrl":     rt.WebhookUrl,
		"webhookEnabled": rt.WebhookEnabled,
		// Notify: Bark
		"barkUrl":     rt.BarkUrl,
		"barkEnabled": rt.BarkEnabled,
		// Notify: ServerChan
		"serverChanEnabled":   rt.ServerChanEnabled,
		"serverChanKeyMasked": maskValue(rt.ServerChanKey),
		// Notify: Telegram
		"telegramEnabled":         rt.TelegramEnabled,
		"telegramApiBaseUrl":      rt.TelegramApiBaseUrl,
		"telegramBotTokenMasked":  maskValue(rt.TelegramBotToken),
		"telegramChatId":          rt.TelegramChatId,
		"telegramUseSystemProxy":  rt.TelegramUseSystemProxy,
		"telegramMessageThreadId": rt.TelegramMessageThreadId,
		// Notify: SMTP
		"smtpEnabled":    rt.SmtpEnabled,
		"smtpHost":       rt.SmtpHost,
		"smtpPort":       rt.SmtpPort,
		"smtpSecure":     rt.SmtpSecure,
		"smtpUser":       rt.SmtpUser,
		"smtpPassMasked": maskValue(rt.SmtpPass),
		"smtpFrom":       rt.SmtpFrom,
		"smtpTo":         rt.SmtpTo,
		// Notify: Feishu / DingTalk / WeCom / Ntfy
		"feishuEnabled":        rt.FeishuEnabled,
		"feishuWebhook":        rt.FeishuWebhook,
		"feishuSecretMasked":   maskValue(rt.FeishuSecret),
		"dingtalkEnabled":      rt.DingtalkEnabled,
		"dingtalkWebhook":      rt.DingtalkWebhook,
		"dingtalkSecretMasked": maskValue(rt.DingtalkSecret),
		"wecomEnabled":         rt.WecomEnabled,
		"wecomWebhook":         rt.WecomWebhook,
		"ntfyEnabled":          rt.NtfyEnabled,
		"ntfyUrl":              rt.NtfyUrl,
		"ntfyTopic":            rt.NtfyTopic,
		"ntfyTokenMasked":      maskValue(rt.NtfyToken),
		"notifyTaskToggles":    rt.NotifyTaskToggles,
		// Notify: cooldown
		"notifyCooldownSec": rt.NotifyCooldownSec,
		// Admin
		"adminIpAllowlist": rt.AdminIpAllowlist,
		"currentAdminIp":   extractClientIP(r),
		"serverTimeZone":   cfg.Tz,
		// System
		"systemProxyUrl": rt.SystemProxyUrl,
		// Proxy
		"payloadRules":                 rt.PayloadRules,
		"proxyErrorKeywords":           rt.ProxyErrorKeywords,
		"proxyEmptyContentFailEnabled": rt.ProxyEmptyContentFailEnabled,
		// Global filters (always JSON arrays; never null)
		"globalBlockedBrands": stringSliceOrEmpty(rt.GlobalBlockedBrands),
		"globalAllowedModels": stringSliceOrEmpty(rt.GlobalAllowedModels),
		// Effective prompt-cache ratio fallbacks (reflect overrides).
		"cacheRatioDefault": routing.DefaultCacheRatioForModel("gpt-4o"),
		"cacheRatioClaude":  routing.DefaultCacheRatioForModel("claude-3-5-sonnet"),
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

func extractClientIP(r *http.Request) string {
	addr := r.RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx > 0 {
		return addr[:idx]
	}
	return addr
}
