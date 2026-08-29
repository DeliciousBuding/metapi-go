import re

# ============ settings.go: helpers + getRuntime + updateRuntime tail ============
src = open('handler/admin/settings.go', encoding='utf-8').read()

# getRuntime: read one snapshot
src = src.replace('''func (h *settingsHandler) getRuntime(w http.ResponseWriter, r *http.Request) {
	cfg := h.cfg''',
'''func (h *settingsHandler) getRuntime(w http.ResponseWriter, r *http.Request) {
	// One atomic snapshot for the whole response: the admin UI can never
	// observe a half-applied settings update.
	rt := config.Runtime()
	cfg := h.cfg''', 1)
# every runtime field read in getRuntime now comes from rt
for f in ['CheckinCron','CheckinScheduleMode','CheckinIntervalHours','CheckinWindowStart','CheckinWindowEnd',
          'CheckinDisabled','SystemName','Logo','Footer','About','ServerAddress','BalanceRefreshCron',
          'BalanceRefreshDisabled','ModelSyncCron','LogCleanupCron','LogCleanupUsageLogsEnabled',
          'LogCleanupProgramLogsEnabled','LogCleanupRetentionDays','ModelAvailabilityProbeEnabled',
          'CodexUpstreamWebsocketEnabled','ResponsesCompactFallbackToResponsesEnabled','DisableCrossProtocolFallback',
          'ProxySessionChannelConcurrencyLimit','ProxySessionChannelQueueWaitMs','ProxyDebugTraceEnabled',
          'ProxyDebugCaptureHeaders','ProxyDebugCaptureBodies','ProxyDebugCaptureStreamChunks',
          'ProxyDebugTargetSessionId','ProxyDebugTargetClientKind','ProxyDebugTargetModel',
          'ProxyDebugRetentionHours','ProxyDebugMaxBodyBytes','RoutingFallbackUnitCost',
          'ProxyFirstByteTimeoutSec','TokenRouterFailureCooldownMaxSec','ProxyRetryStatusRanges',
          'ProxyDisableStatusRanges','RoutingWeights','WebhookUrl','WebhookEnabled','BarkUrl','BarkEnabled',
          'ServerChanEnabled','ServerChanKey','TelegramEnabled','TelegramApiBaseUrl','TelegramBotToken',
          'TelegramChatId','TelegramUseSystemProxy','TelegramMessageThreadId','SmtpEnabled','SmtpHost',
          'SmtpPort','SmtpSecure','SmtpUser','SmtpPass','SmtpFrom','SmtpTo','FeishuEnabled','FeishuWebhook',
          'FeishuSecret','DingtalkEnabled','DingtalkWebhook','DingtalkSecret','WecomEnabled','WecomWebhook',
          'NtfyEnabled','NtfyUrl','NtfyTopic','NtfyToken','NotifyTaskToggles','NotifyCooldownSec',
          'AdminIpAllowlist','SystemProxyUrl','ProxyToken','PayloadRules','ProxyErrorKeywords',
          'ProxyEmptyContentFailEnabled','GlobalBlockedBrands','GlobalAllowedModels']:
    src = src.replace('cfg.'+f, 'rt.'+f)
src = src.replace('scheduleSpecForCheckin(cfg)', 'scheduleSpecForCheckin(rt)', 1)

# applyBoolSettingDB helper -> closure form
src = src.replace('''func applyBoolSettingDB(db *sqlx.DB, body map[string]any, key string, target *bool, dbKey string) error {
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
}''',
'''func applyBoolSettingDB(db *sqlx.DB, body map[string]any, key string, dbKey string, set func(*config.RuntimeSettings, bool)) error {
	if v, ok := body[key]; ok {
		val, err := toBoolStrict(v)
		if err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		config.UpdateRuntime(func(r *config.RuntimeSettings) { set(r, val) })
		if err := upsertSettingDB(db, dbKey, val); err != nil {
			return err
		}
	}
	return nil
}''', 1)

# updateRuntime tail response reads
src = src.replace('''		"globalAllowedModels": stringSliceOrEmpty(h.cfg.GlobalAllowedModels),
		"globalBlockedBrands": stringSliceOrEmpty(h.cfg.GlobalBlockedBrands),
	})
}''',
'''		"globalAllowedModels": stringSliceOrEmpty(config.Runtime().GlobalAllowedModels),
		"globalBlockedBrands": stringSliceOrEmpty(config.Runtime().GlobalBlockedBrands),
	})
}''', 1)

# brandList reads
src = src.replace('''		"globalAllowedModels": stringSliceOrEmpty(h.cfg.GlobalAllowedModels),
		"globalBlockedBrands": stringSliceOrEmpty(h.cfg.GlobalBlockedBrands),''',
'''		"globalAllowedModels": stringSliceOrEmpty(config.Runtime().GlobalAllowedModels),
		"globalBlockedBrands": stringSliceOrEmpty(config.Runtime().GlobalBlockedBrands),''')

# system-proxy test reads the runtime system proxy
src = src.replace('''	proxyURL := h.cfg.SystemProxyUrl''', '''	proxyURL := config.Runtime().SystemProxyUrl''', 1)
open('handler/admin/settings.go','w',encoding='utf-8').write(src)
print("settings.go updated")
