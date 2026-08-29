import re, sys

MOVE = set("""AuthToken ProxyToken
DbType DbUrl DbSsl
CheckinCron CheckinScheduleMode CheckinIntervalHours CheckinWindowStart CheckinWindowEnd
BalanceRefreshCron CheckinDisabled BalanceRefreshDisabled ModelSyncCron LogCleanupCron
SystemName Logo Footer About ServerAddress
LogCleanupUsageLogsEnabled LogCleanupProgramLogsEnabled LogCleanupRetentionDays
WebhookUrl WebhookEnabled BarkUrl BarkEnabled ServerChanKey ServerChanEnabled
TelegramEnabled TelegramApiBaseUrl TelegramBotToken TelegramChatId TelegramUseSystemProxy TelegramMessageThreadId
SmtpEnabled SmtpHost SmtpPort SmtpSecure SmtpUser SmtpPass SmtpFrom SmtpTo
FeishuEnabled FeishuWebhook FeishuSecret DingtalkEnabled DingtalkWebhook DingtalkSecret
WecomEnabled WecomWebhook NtfyEnabled NtfyUrl NtfyTopic NtfyToken
NotifyCooldownSec NotifyTaskToggles SystemProxyUrl
AdminIpAllowlist
RoutingFallbackUnitCost CacheRatioDefault CacheRatioClaude ProxyFirstByteTimeoutSec
TokenRouterFailureCooldownMaxSec ProxyRetryStatusRanges ProxyDisableStatusRanges
ProxySessionChannelConcurrencyLimit ProxySessionChannelQueueWaitMs
CodexUpstreamWebsocketEnabled ResponsesCompactFallbackToResponsesEnabled
DisableCrossProtocolFallback ProxyEmptyContentFailEnabled
ProxyErrorKeywords GlobalBlockedBrands GlobalAllowedModels
ProxyDebugTraceEnabled ProxyDebugCaptureHeaders ProxyDebugCaptureBodies ProxyDebugCaptureStreamChunks
ProxyDebugTargetSessionId ProxyDebugTargetClientKind ProxyDebugTargetModel
ProxyDebugRetentionHours ProxyDebugMaxBodyBytes
RoutingWeights PayloadRules""".split())

src = open('config/config.go', encoding='utf-8').read()
lines = src.split('\n')

# --- locate Config struct body ---
start = next(i for i,l in enumerate(lines) if l.startswith('type Config struct {'))
depth = 0; end = None
for i in range(start, len(lines)):
    depth += lines[i].count('{') - lines[i].count('}')
    if depth == 0:
        end = i; break
assert end, "struct end not found"

body = lines[start+1:end]
keep, moved = [], []
pending_comments = []
for ln in body:
    s = ln.strip()
    if s.startswith('//'):
        pending_comments.append(ln)
        continue
    m = re.match(r'\s*([A-Za-z_][A-Za-z0-9_]*)\s+\S', ln)
    if m and m.group(1) in MOVE:
        moved.extend(pending_comments); moved.append(ln)
        pending_comments = []
    else:
        keep.extend(pending_comments); keep.append(ln)
        pending_comments = []
keep.extend(pending_comments)

new_config = lines[:start+1] + keep + lines[end:]
open('config/config.go','w',encoding='utf-8').write('\n'.join(new_config))

# --- split Load: token-rewrite cfg.X -> rt.X inside Load function ---
src2 = '\n'.join(new_config)
ls = src2.index('func Load(env map[string]string) *Config {')
# find matching closing brace of Load
depth=0; le=None
for i in range(ls, len(src2)):
    if src2[i]=='{': depth+=1
    elif src2[i]=='}':
        depth-=1
        if depth==0: le=i+1; break
load_body = src2[ls:le]
for f in sorted(MOVE, key=len, reverse=True):
    load_body = re.sub(r'\bcfg\.'+f+r'\b', 'rt.'+f, load_body)
load_body = load_body.replace('func Load(env map[string]string) *Config {',
    'func Load(env map[string]string) (*Config, *RuntimeSettings) {', 1)
load_body = load_body.replace('\tcfg := &Config{}\n',
    '\tcfg := &Config{}\n\trt := &RuntimeSettings{}\n', 1)
load_body = re.sub(r'\n\treturn cfg\n\}', '\n\treturn cfg, rt\n}', load_body)
src2 = src2[:ls] + load_body + src2[le:]
open('config/config.go','w',encoding='utf-8').write(src2)

# --- emit runtime struct body ---
open('config/_runtime_fields.txt','w',encoding='utf-8').write('\n'.join(moved)+'\n')
print("moved fields:", len([l for l in moved if not l.strip().startswith('//')]))
