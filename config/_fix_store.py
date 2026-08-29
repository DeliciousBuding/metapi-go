import re

MOVE = set("AuthToken ProxyToken DbType DbUrl DbSsl CheckinCron CheckinScheduleMode CheckinIntervalHours CheckinWindowStart CheckinWindowEnd BalanceRefreshCron CheckinDisabled BalanceRefreshDisabled ModelSyncCron LogCleanupCron SystemName Logo Footer About ServerAddress LogCleanupUsageLogsEnabled LogCleanupProgramLogsEnabled LogCleanupRetentionDays WebhookUrl WebhookEnabled BarkUrl BarkEnabled ServerChanKey ServerChanEnabled TelegramEnabled TelegramApiBaseUrl TelegramBotToken TelegramChatId TelegramUseSystemProxy TelegramMessageThreadId SmtpEnabled SmtpHost SmtpPort SmtpSecure SmtpUser SmtpPass SmtpFrom SmtpTo FeishuEnabled FeishuWebhook FeishuSecret DingtalkEnabled DingtalkWebhook DingtalkSecret WecomEnabled WecomWebhook NtfyEnabled NtfyUrl NtfyTopic NtfyToken NotifyCooldownSec NotifyTaskToggles SystemProxyUrl AdminIpAllowlist RoutingFallbackUnitCost CacheRatioDefault CacheRatioClaude ProxyFirstByteTimeoutSec TokenRouterFailureCooldownMaxSec ProxyRetryStatusRanges ProxyDisableStatusRanges ProxySessionChannelConcurrencyLimit ProxySessionChannelQueueWaitMs CodexUpstreamWebsocketEnabled ResponsesCompactFallbackToResponsesEnabled DisableCrossProtocolFallback ProxyEmptyContentFailEnabled ProxyErrorKeywords GlobalBlockedBrands GlobalAllowedModels ProxyDebugTraceEnabled ProxyDebugCaptureHeaders ProxyDebugCaptureBodies ProxyDebugCaptureStreamChunks ProxyDebugTargetSessionId ProxyDebugTargetClientKind ProxyDebugTargetModel ProxyDebugRetentionHours ProxyDebugMaxBodyBytes RoutingWeights PayloadRules".split())

# --- store/settings.go: hydrate runtime fields into rt draft ---
src = open('store/settings.go', encoding='utf-8').read()
for f in sorted(MOVE, key=len, reverse=True):
    src = re.sub(r'\bcfg\.' + f + r'\b', 'rt.' + f, src)
src = src.replace('func LoadRuntimeSettings(cfg *config.Config) error {',
                  'func LoadRuntimeSettings(cfg *config.Config, rt *config.RuntimeSettings) error {', 1)
src = src.replace('\t// Apply runtime overrides to config.\n\tApplyRuntimeSettings(cfg, settingsMap)',
                  '\t// Apply runtime overrides to the static + runtime drafts.\n\tApplyRuntimeSettings(cfg, rt, settingsMap)', 1)
src = src.replace('func ApplyRuntimeSettings(cfg *config.Config, settingsMap map[string]string) {',
                  'func ApplyRuntimeSettings(cfg *config.Config, rt *config.RuntimeSettings, settingsMap map[string]string) {', 1)
# LogCleanup auto-enable uses rt retention but cfg.LogCleanupConfigured stays static
open('store/settings.go','w',encoding='utf-8').write(src)

# --- store/bootstrap.go ---
src = open('store/bootstrap.go', encoding='utf-8').read()
src = src.replace('func EnsureRuntimeDatabase(cfg *config.Config) error {',
                  'func EnsureRuntimeDatabase(cfg *config.Config, rt *config.RuntimeSettings) error {', 1)
src = src.replace('\tdialect := cfg.DbType\n\tdsn := cfg.DbUrl',
                  '\tdialect := rt.DbType\n\tdsn := rt.DbUrl', 1)
src = src.replace('cfg.PostgresSSLMode()', 'cfg.PostgresSSLMode(rt.DbSsl)', 1)
open('store/bootstrap.go','w',encoding='utf-8').write(src)
print("store files updated")
