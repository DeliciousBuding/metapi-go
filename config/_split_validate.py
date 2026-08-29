import re
src = open('config/validate.go', encoding='utf-8').read()

def cut(start_marker, end_marker, text, include_start=True, include_end=False):
    """Cut text from start_marker to end_marker, return (cut_text, remaining)."""
    s = text.index(start_marker)
    e = text.index(end_marker, s)
    if include_end:
        e += len(end_marker)
    return text[s:e], text[:s] + text[e:]

moved_parts = []

# 1. CheckinScheduleMode critical
part, src = cut('\t// --- Critical: CheckinScheduleMode', '\t// --- Critical: DBType', src)
moved_parts.append(part)

# 2. DBType critical (DbSslMode check stays)
part, src = cut('\t// --- Critical: DBType', '\tif !validDbSslMode', src)
moved_parts.append(part)

# 3. Four cron warnings
part, src = cut('\t// --- Warning: Cron expressions', '\t// --- Warning: NotifyCooldownSec', src)
moved_parts.append(part)

# 4. NotifyCooldownSec
part, src = cut('\t// --- Warning: NotifyCooldownSec', '\t// --- Warning: ProxyFirstByteTimeoutSec', src)
moved_parts.append(part)

# 5. ProxyFirstByteTimeoutSec
part, src = cut('\t// --- Warning: ProxyFirstByteTimeoutSec', '\t// --- Warning: TokenRouterFailureCooldownMaxSec', src)
moved_parts.append(part)

# 6. TokenRouterFailureCooldownMaxSec
part, src = cut('\t// --- Warning: TokenRouterFailureCooldownMaxSec', '\t// --- Critical: Admin CORS origins', src)
moved_parts.append(part)

# 7. CheckinIntervalHours
part, src = cut('\t// --- Warning: CheckinIntervalHours', '\t// --- Warning: RoutingWeights', src)
moved_parts.append(part)

# 8. RoutingWeights
part, src = cut('\t// --- Warning: RoutingWeights', '\t// --- Critical: Default AUTH_TOKEN', src)
moved_parts.append(part)

# 9. AuthToken/ProxyToken defaults
part, src = cut('\t// --- Critical: Default AUTH_TOKEN', '\t// --- Warning: account_credential_secret', src)
moved_parts.append(part)

# 10+11. ProxySessionChannelConcurrencyLimit (between "range checks" header and sticky)
part, src = cut('\tif c.ProxySessionChannelConcurrencyLimit < 1 {', '\tif c.ProxyStickySessionEnabled', src)
moved_parts.append(part)

# ProxySessionChannelQueueWaitMs (after TokenRouterCacheTtlMs block)
part, src = cut('\tif c.ProxySessionChannelQueueWaitMs < 0 {', '\t// --- Warning: webhook / service URLs', src)
moved_parts.append(part)

# 12. webhook URLs — split runtime URLs from resin_url
webhook_block, src = cut('\t// --- Warning: webhook / service URLs', '\t// --- Warning: proxy / redis URLs', src)
runtime_urls = '''\twebhookUrls := []struct{ field, val string }{
\t\t{"webhook_url", r.WebhookUrl},
\t\t{"bark_url", r.BarkUrl},
\t\t{"feishu_webhook", r.FeishuWebhook},
\t\t{"dingtalk_webhook", r.DingtalkWebhook},
\t\t{"wecom_webhook", r.WecomWebhook},
\t\t{"ntfy_url", r.NtfyUrl},
\t\t{"telegram_api_base_url", r.TelegramApiBaseUrl},
\t}
\tfor _, u := range webhookUrls {
\t\tif err := validateUrl(u.field, u.val, true); err != nil {
\t\t\terrs = append(errs, err)
\t\t}
\t}
'''
moved_parts.append('\t// --- Warning: notify webhook URLs must be well-formed http(s) ---\n' + runtime_urls)
# resin_url stays in Config.Validate
src = src.replace('\t// --- Warning: proxy / redis URLs', '\t// --- Warning: resin URL must be well-formed http(s) ---\n\tif err := validateUrl("resin_url", c.ResinURL, true); err != nil {\n\t\terrs = append(errs, err)\n\t}\n\t// --- Warning: proxy / redis URLs', 1)

# 13. system_proxy_url — split from redis_url
old_open = '''\tfor _, u := range []struct{ field, val string }{
\t\t{"system_proxy_url", c.SystemProxyUrl},
\t\t{"redis_url", c.RedisURL},
\t} {
\t\tif err := validateUrl(u.field, u.val, false); err != nil {
\t\t\terrs = append(errs, err)
\t\t}
\t}'''
new_static = '''\tfor _, u := range []struct{ field, val string }{
\t\t{"redis_url", c.RedisURL},
\t} {
\t\tif err := validateUrl(u.field, u.val, false); err != nil {
\t\t\terrs = append(errs, err)
\t\t}
\t}'''
assert old_open in src
src = src.replace(old_open, new_static, 1)
moved_parts.append('''\t// --- Warning: system proxy URL must parse (scheme left open) ---
\tif err := validateUrl("system_proxy_url", r.SystemProxyUrl, false); err != nil {
\t\terrs = append(errs, err)
\t}

''')

# 14. checkin window HH:mm (runs to end of Validate: "return errs")
part, src = cut('\t// --- Warning: checkin window bounds', '\treturn errs\n}', src)
moved_parts.append(part)

# Rename c. -> r. in moved parts
moved = ''.join(moved_parts)
moved = moved.replace('c.CheckinScheduleMode', 'r.CheckinScheduleMode')
moved = moved.replace('c.DbType', 'r.DbType')
moved = moved.replace('c.CheckinCron', 'r.CheckinCron')
moved = moved.replace('c.BalanceRefreshCron', 'r.BalanceRefreshCron')
moved = moved.replace('c.ModelSyncCron', 'r.ModelSyncCron')
moved = moved.replace('c.LogCleanupCron', 'r.LogCleanupCron')
moved = moved.replace('c.NotifyCooldownSec', 'r.NotifyCooldownSec')
moved = moved.replace('c.ProxyFirstByteTimeoutSec', 'r.ProxyFirstByteTimeoutSec')
moved = moved.replace('c.TokenRouterFailureCooldownMaxSec', 'r.TokenRouterFailureCooldownMaxSec')
moved = moved.replace('c.CheckinIntervalHours', 'r.CheckinIntervalHours')
moved = moved.replace('c.RoutingWeights', 'r.RoutingWeights')
moved = moved.replace('c.AuthToken', 'r.AuthToken')
moved = moved.replace('c.ProxyToken', 'r.ProxyToken')
moved = moved.replace('c.ProxySessionChannelConcurrencyLimit', 'r.ProxySessionChannelConcurrencyLimit')
moved = moved.replace('c.ProxySessionChannelQueueWaitMs', 'r.ProxySessionChannelQueueWaitMs')
moved = moved.replace('c.CheckinWindowStart', 'r.CheckinWindowStart')
moved = moved.replace('c.CheckinWindowEnd', 'r.CheckinWindowEnd')

method = '''
// Validate checks the runtime-mutable settings and returns all validation
// errors. Boot-time call path mirrors Config.Validate: warnings for
// non-fatal issues, exit on critical ones before binding the port.
func (r *RuntimeSettings) Validate() []error {
\tvar errs []error

''' + moved + '''\treturn errs
}
'''
src = src + method
open('config/validate.go','w',encoding='utf-8').write(src)
print("validate split done")
