package config

import (
	"log/slog"
	"math"
	"strings"
)

// Load reads environment variables from the given map and returns a fully
// populated Config. The caller (main.go) is responsible for loading.env files
// via godotenv before calling Load.
func Load(env map[string]string) (*Config, *RuntimeSettings) {
	// Helper to read from the env map; Go map access returns "" for missing keys.
	get := func(key string) string {
		return env[key]
	}

	cfg := &Config{}
	rt := &RuntimeSettings{}

	// ---- §3.1 Auth ----
	rt.AuthToken = firstNonEmpty(get("AUTH_TOKEN"), DefaultAuthToken)
	rt.ProxyToken = get("PROXY_TOKEN") // One-time legacy key import; never used for authentication.
	cfg.AccountCredentialSecret = firstNonEmpty(get("ACCOUNT_CREDENTIAL_SECRET"), rt.AuthToken)
	if cfg.AccountCredentialSecret == DefaultAuthToken {
		slog.Warn("config: AccountCredentialSecret is using the default value — this is insecure for production. Set ACCOUNT_CREDENTIAL_SECRET or AUTH_TOKEN environment variable.")
	}
	cfg.CodexClientId = firstNonEmpty(parseOptionalSecret(get("CODEX_CLIENT_ID")), DefaultCodexClientId)

	// ---- §3.2 OAuth Clients ----
	cfg.ClaudeClientId = firstNonEmpty(parseOptionalSecret(get("CLAUDE_CLIENT_ID")), DefaultClaudeClientId)
	cfg.GeminiCliClientId = firstNonEmpty(parseOptionalSecret(get("GEMINI_CLI_CLIENT_ID")), DefaultGeminiCliClientId)
	cfg.GeminiCliClientSecret = firstNonEmpty(parseOptionalSecret(get("GEMINI_CLI_CLIENT_SECRET")), DefaultGeminiCliClientSecret)
	cfg.AntigravityClientSecret = parseOptionalSecret(get("ANTIGRAVITY_CLIENT_SECRET"))

	// ---- §3.3 Server ----
	cfg.Port = parseTruncatedInt(get("PORT"), DefaultPort)
	cfg.ListenHost = parseListenHost(env)
	cfg.DataDir = firstNonEmpty(get("DATA_DIR"), DefaultDataDir)
	rt.DbUrl = strings.TrimSpace(firstNonEmpty(get("DB_URL"), get("DATABASE_URL")))
	rt.DbType = inferDbType(get("DB_TYPE"), rt.DbUrl)
	rt.DbSsl = parseBoolean(get("DB_SSL"), false)
	cfg.DbSslMode = normalizeDbSslMode(get("DB_SSLMODE"))
	cfg.DbProfile = normalizeDbProfile(firstNonEmpty(get("DB_PROFILE"), get("METAPI_DB_PROFILE"), DefaultDbProfile))
	openDefault, idleDefault := dbPoolDefaultsForProfile(cfg.DbProfile)
	cfg.DbMaxOpenConns = parseTruncatedInt(get("DB_MAX_OPEN_CONNS"), openDefault)
	cfg.DbMaxIdleConns = parseTruncatedInt(get("DB_MAX_IDLE_CONNS"), idleDefault)
	cfg.DbConnMaxLifetimeSec = parseTruncatedInt(get("DB_CONN_MAX_LIFETIME_SEC"), DefaultDbConnMaxLifetimeSec)
	cfg.DbConnMaxIdleTimeSec = parseTruncatedInt(get("DB_CONN_MAX_IDLE_TIME_SEC"), DefaultDbConnMaxIdleTimeSec)
	cfg.DbApplicationName = strings.TrimSpace(firstNonEmpty(get("DB_APPLICATION_NAME"), get("METAPI_DB_APPLICATION_NAME")))
	cfg.Tz = get("TZ")
	cfg.LogLevel = normalizeLogLevel(firstNonEmpty(get("LOG_LEVEL"), DefaultLogLevel))

	// ---- §3.4 Cron ----
	rt.CheckinCron = firstNonEmpty(get("CHECKIN_CRON"), DefaultCheckinCron)
	checkinMode := strings.ToLower(strings.TrimSpace(get("CHECKIN_SCHEDULE_MODE")))
	if checkinMode == "interval" {
		rt.CheckinScheduleMode = "interval"
	} else if checkinMode == "window" {
		rt.CheckinScheduleMode = "window"
	} else {
		rt.CheckinScheduleMode = "cron"
	}
	// E1: window bounds (HH:mm, 24h). Defaults: 00:00-23:59 = any time of day.
	rt.CheckinWindowStart = firstNonEmpty(get("CHECKIN_WINDOW_START"), "00:00")
	rt.CheckinWindowEnd = firstNonEmpty(get("CHECKIN_WINDOW_END"), "23:59")
	rt.CheckinIntervalHours = ClampInt(
		parseTruncatedInt(get("CHECKIN_INTERVAL_HOURS"), DefaultCheckinIntervalHours),
		1, 24,
	)
	rt.BalanceRefreshCron = firstNonEmpty(get("BALANCE_REFRESH_CRON"), DefaultBalanceRefreshCron)
	// #1027: global enable switches for the upstream-touching account jobs.
	// Absent/invalid env vars keep the historical defaults (both enabled).
	// Stored inverted: zero value = enabled (see Config field comment).
	rt.CheckinDisabled = !parseBoolean(get("CHECKIN_ENABLED"), true)
	rt.BalanceRefreshDisabled = !parseBoolean(get("BALANCE_REFRESH_ENABLED"), true)
	rt.ModelSyncCron = firstNonEmpty(get("MODEL_SYNC_CRON"), DefaultModelSyncCron)
	rt.LogCleanupCron = firstNonEmpty(get("LOG_CLEANUP_CRON"), DefaultLogCleanupCron)

	// ---- 3.4b Site & Branding ----
	rt.SystemName = firstNonEmpty(get("SYSTEM_NAME"), DefaultSystemName)
	rt.Logo = firstNonEmpty(get("LOGO"), DefaultLogo)
	rt.Footer = firstNonEmpty(get("FOOTER"), DefaultFooter)
	rt.About = firstNonEmpty(get("ABOUT"), DefaultAbout)
	rt.ServerAddress = firstNonEmpty(get("SERVER_ADDRESS"), DefaultServerAddress)

	// ---- §3.5 Log Cleanup ----
	rt.LogCleanupUsageLogsEnabled = parseBoolean(get("LOG_CLEANUP_USAGE_LOGS_ENABLED"), false)
	rt.LogCleanupProgramLogsEnabled = parseBoolean(get("LOG_CLEANUP_PROGRAM_LOGS_ENABLED"), false)
	rt.LogCleanupRetentionDays = max(1, parseTruncatedInt(get("LOG_CLEANUP_RETENTION_DAYS"), 30))
	// An env toggle set to true is an explicit ask for the log_cleanup regime;
	// retention and cron alone are not (see Config.LogCleanupEnvEnabled).
	cfg.LogCleanupEnvEnabled = rt.LogCleanupUsageLogsEnabled || rt.LogCleanupProgramLogsEnabled
	cfg.LogCleanupConfigured = false // resolved at hydration: explicit DB keys OR the env toggle above

	// ---- §3.6 Notify: Webhook ----
	rt.WebhookUrl = get("WEBHOOK_URL")
	rt.WebhookEnabled = parseBoolean(get("WEBHOOK_ENABLED"), true)

	// ---- §3.7 Notify: Bark ----
	rt.BarkUrl = get("BARK_URL")
	rt.BarkEnabled = parseBoolean(get("BARK_ENABLED"), true)

	// ---- §3.8 Notify: ServerChan ----
	rt.ServerChanKey = get("SERVERCHAN_KEY")
	rt.ServerChanEnabled = parseBoolean(get("SERVERCHAN_ENABLED"), true)

	// ---- §3.9 Notify: Telegram ----
	rt.TelegramEnabled = parseBoolean(get("TELEGRAM_ENABLED"), false)
	rt.TelegramApiBaseUrl = TelegramApiBaseUrl
	rt.TelegramBotToken = get("TELEGRAM_BOT_TOKEN")
	rt.TelegramChatId = get("TELEGRAM_CHAT_ID")
	rt.TelegramUseSystemProxy = parseBoolean(get("TELEGRAM_USE_SYSTEM_PROXY"), false)
	rt.TelegramMessageThreadId = strings.TrimSpace(get("TELEGRAM_MESSAGE_THREAD_ID"))

	// ---- §3.10 Notify: SMTP ----
	rt.SmtpEnabled = parseBoolean(get("SMTP_ENABLED"), false)
	rt.SmtpHost = get("SMTP_HOST")
	rt.SmtpPort = atoiOr(get("SMTP_PORT"), DefaultSmtpPort)
	rt.SmtpSecure = parseBoolean(get("SMTP_SECURE"), false)
	rt.SmtpUser = get("SMTP_USER")
	rt.SmtpPass = get("SMTP_PASS")
	rt.SmtpFrom = get("SMTP_FROM")
	rt.SmtpTo = get("SMTP_TO")
	rt.SmtpAllowCustomPort = parseBoolean(get("SMTP_ALLOW_CUSTOM_PORT"), false)

	// ---- §3.11 Notify: General ----
	rt.NotifyCooldownSec = max(0, parseTruncatedInt(get("NOTIFY_COOLDOWN_SEC"), DefaultNotifyCooldownSec))
	rt.SystemProxyUrl = get("SYSTEM_PROXY_URL")

	// ---- §3.11b Resin sticky proxy pool ----
	cfg.ResinURL = get("RESIN_URL")
	cfg.ResinPlatformName = get("RESIN_PLATFORM_NAME")
	cfg.ResinEnabled = parseBoolean(get("RESIN_ENABLED"), false)

	// ---- §3.11c uTLS TLS fingerprint masking ----
	cfg.UTLSEnabled = parseBoolean(get("UTLS_ENABLED"), false)

	// ---- §3.11e LDOH monitor proxy base URL ----
	cfg.LDOHBaseURL = strings.TrimSpace(firstNonEmpty(get("LDOH_BASE_URL"), DefaultLDOHBaseURL))
	// LDOH_PROXY_TIMEOUT_SEC: per-request timeout for the LDOH upstream HTTP
	// client. 0/negative/invalid falls back to the default (30s) so a
	// misconfigured value never produces an unbounded-context LDOH call.
	cfg.LDOHProxyTimeoutSec = parsePositiveInt(get("LDOH_PROXY_TIMEOUT_SEC"), DefaultLDOHProxyTimeoutSec)

	// ---- §3.11f Outbound proxy / upstream HTTP timeouts (#1009) ----
	// Integer seconds; 0/negative/invalid falls back to the default so a
	// misconfigured value never disables a timeout. Defaults match the
	// pre-#1009 hardcoded values in platform/site_proxy.go.
	cfg.ProxyConnectTimeoutSec = parsePositiveInt(get("PROXY_CONNECT_TIMEOUT_SEC"), DefaultProxyConnectTimeoutSec)
	cfg.ProxyTLSHandshakeTimeoutSec = parsePositiveInt(get("PROXY_TLS_HANDSHAKE_TIMEOUT_SEC"), DefaultProxyTLSHandshakeTimeoutSec)
	cfg.ProxyResponseHeaderTimeoutSec = parsePositiveInt(get("PROXY_RESPONSE_HEADER_TIMEOUT_SEC"), DefaultProxyResponseHeaderTimeoutSec)
	cfg.ProxyIdleConnTimeoutSec = parsePositiveInt(get("PROXY_IDLE_CONN_TIMEOUT_SEC"), DefaultProxyIdleConnTimeoutSec)
	cfg.ProxyRequestTimeoutSec = parsePositiveInt(get("PROXY_REQUEST_TIMEOUT_SEC"), DefaultProxyRequestTimeoutSec)
	cfg.ProxyStreamIdleTimeoutSec = parsePositiveInt(get("PROXY_STREAM_IDLE_TIMEOUT_SEC"), DefaultProxyStreamIdleTimeoutSec)
	// PROXY_STREAM_WRITE_TIMEOUT_SEC is the canonical name: the same per-write
	// window governs SSE and native media streams. PROXY_SSE_WRITE_TIMEOUT_SEC
	// remains the pre-generalization alias so existing deployments keep their
	// configured value; the canonical key wins when both are set.
	cfg.ProxyStreamWriteTimeoutSec = parsePositiveInt(
		firstNonEmpty(get("PROXY_STREAM_WRITE_TIMEOUT_SEC"), get("PROXY_SSE_WRITE_TIMEOUT_SEC")),
		DefaultProxyStreamWriteTimeoutSec,
	)
	cfg.ProxyDirectAccountMaxConcurrency = parseConcurrencyLimit(get("PROXY_DIRECT_ACCOUNT_MAX_CONCURRENCY"), DefaultProxyDirectAccountMaxConcurrency)

	// ---- §3.12 Notify: Feishu / DingTalk / WeCom / Ntfy ----
	rt.FeishuEnabled = parseBoolean(get("FEISHU_ENABLED"), false)
	rt.FeishuWebhook = get("FEISHU_WEBHOOK")
	rt.FeishuSecret = get("FEISHU_SECRET")
	rt.DingtalkEnabled = parseBoolean(get("DINGTALK_ENABLED"), false)
	rt.DingtalkWebhook = get("DINGTALK_WEBHOOK")
	rt.DingtalkSecret = get("DINGTALK_SECRET")
	rt.WecomEnabled = parseBoolean(get("WECOM_ENABLED"), false)
	rt.WecomWebhook = get("WECOM_WEBHOOK")
	rt.NtfyEnabled = parseBoolean(get("NTFY_ENABLED"), false)
	rt.NtfyUrl = get("NTFY_URL")
	rt.NtfyTopic = get("NTFY_TOPIC")
	rt.NtfyToken = get("NTFY_TOKEN")
	cfg.RedisURL = firstNonEmpty(get("REDIS_URL"), get("METAPI_REDIS_URL"))

	// ---- §3.12 Admin ----
	rt.AdminIpAllowlist = parseCsvList(get("ADMIN_IP_ALLOWLIST"))
	cfg.AdminCorsAllowedOrigins = parseCsvList(get("ADMIN_CORS_ALLOWED_ORIGINS"))
	cfg.TrustedProxyCidrs = parseCsvList(get("TRUSTED_PROXY_CIDRS"))
	// Admin/OAuth per-IP token-bucket rate limits. Defaults preserve the
	// original hardcoded values (Admin 100/200, OAuth 10/20) so existing
	// deployments are byte-for-byte compatible without env changes.
	cfg.AdminRateLimitRPS = max(0, parseTruncatedInt(get("ADMIN_RATE_LIMIT_RPS"), DefaultAdminRateLimitRPS))
	cfg.AdminRateLimitBurst = max(0, parseTruncatedInt(get("ADMIN_RATE_LIMIT_BURST"), DefaultAdminRateLimitBurst))
	cfg.OAuthRateLimitRPS = max(0, parseTruncatedInt(get("OAUTH_RATE_LIMIT_RPS"), DefaultOAuthRateLimitRPS))
	cfg.OAuthRateLimitBurst = max(0, parseTruncatedInt(get("OAUTH_RATE_LIMIT_BURST"), DefaultOAuthRateLimitBurst))
	// /api/auth/* brute-force cap (#1034): login is the only surface that
	// accepts the master token, so it gets the strict bucket regardless of
	// the general admin bucket size.
	cfg.AuthRateLimitRPS = max(0, parseTruncatedInt(get("AUTH_RATE_LIMIT_RPS"), DefaultAuthRateLimitRPS))
	cfg.AuthRateLimitBurst = max(0, parseTruncatedInt(get("AUTH_RATE_LIMIT_BURST"), DefaultAuthRateLimitBurst))
	// Server-side admin session (#1034): sliding TTL + cookie Secure policy.
	// Zero-config defaults must stay safe (12h sliding session, Secure
	// auto-adapting to the request protocol).
	cfg.AdminSessionTTLMinutes = max(1, parseTruncatedInt(get("ADMIN_SESSION_TTL_MINUTES"), DefaultAdminSessionTTLMinutes))
	cfg.AdminSessionCookieSecure = normalizeSessionCookieSecure(get("ADMIN_SESSION_COOKIE_SECURE"))

	// ---- §3.13 Proxy: Core ----
	// REQUEST_BODY_LIMIT_MB controls the general body limit (default 20 MB,
	// clamped to [1, 200]). FILE_UPLOAD_LIMIT_MB controls a separate higher
	// limit for multipart upload routes /v1/files and /v1/images/* (default
	// 100 MB, clamped to [1, 1000]).
	bodyLimitMB := ClampInt(parseTruncatedInt(get("REQUEST_BODY_LIMIT_MB"), DefaultRequestBodyLimitMB), 1, 200)
	cfg.RequestBodyLimit = bodyLimitMB * 1024 * 1024
	fileUploadLimitMB := ClampInt(parseTruncatedInt(get("FILE_UPLOAD_LIMIT_MB"), DefaultFileUploadLimitMB), 1, 1000)
	cfg.FileUploadLimitBytes = fileUploadLimitMB * 1024 * 1024

	// Per-IP rate limiting for /v1 proxy routes (default 60 RPM; 0 = disabled).
	// Negative values are NOT clamped here so config.Validate can warn the
	// operator — consumers (auth.ProxyRateLimit) already treat <=0 as disabled,
	// so the only observable effect of a negative is the startup warning.
	cfg.ProxyRateLimitRPM = parseTruncatedInt(get("PROXY_RATE_LIMIT_RPM"), DefaultProxyRateLimitRPM)
	// Legacy root key's initial managed RPM cap (default 0 = unlimited).
	// Negative left intact for Validate to warn on; <=0 disables at the limiter.
	cfg.ProxyGlobalTokenRPM = parseTruncatedInt(get("PROXY_GLOBAL_TOKEN_RPM"), 0)

	rt.RoutingFallbackUnitCost = math.Max(1e-6, parseNumber(get("ROUTING_FALLBACK_UNIT_COST"), 1))
	// Seconds; internal first-byte observation uses ms = sec * 1000.
	rt.ProxyFirstByteTimeoutSec = max(0, parseTruncatedInt(get("PROXY_FIRST_BYTE_TIMEOUT_SEC"), 0))

	// ---- §3.14 Proxy: Token Router ----
	tokenRouterParsed := parseNumber(get("TOKEN_ROUTER_FAILURE_COOLDOWN_MAX_SEC"), float64(TokenRouterFailureCooldownMaxSecCeiling))
	if val, ok := NormalizeTokenRouterFailureCooldownMaxSec(tokenRouterParsed); ok {
		rt.TokenRouterFailureCooldownMaxSec = val
	} else {
		rt.TokenRouterFailureCooldownMaxSec = TokenRouterFailureCooldownMaxSecCeiling
	}
	cfg.TokenRouterCacheTtlMs = max(100, parseTruncatedInt(get("TOKEN_ROUTER_CACHE_TTL_MS"), DefaultTokenRouterCacheTtlMs))

	// ---- §3.14b Pricing Catalog (models.dev official list prices) ----
	cfg.PricingCatalogEnabled = parseBoolean(get("PRICING_CATALOG_ENABLED"), DefaultPricingCatalogEnabled)
	cfg.PricingCatalogRefreshMin = max(0, parseTruncatedInt(get("PRICING_CATALOG_REFRESH_MIN"), DefaultPricingCatalogRefreshMin))
	cfg.PricingCatalogAllowPrivate = parseBoolean(get("PRICING_CATALOG_ALLOW_PRIVATE"), false)
	cfg.PricingCatalogURL = strings.TrimSpace(get("PRICING_CATALOG_URL"))
	if cfg.PricingCatalogURL == "" {
		cfg.PricingCatalogURL = DefaultPricingCatalogURL
	}

	// ---- §3.15 Proxy: Channel ----
	// Negative left intact so config.Validate can warn the operator; the
	// consumer (GetProxyMaxChannelAttempts) already clamps <=0 to 1, so the
	// only observable effect of a negative is the startup warning.
	cfg.ProxyMaxChannelAttempts = parseTruncatedInt(get("PROXY_MAX_CHANNEL_ATTEMPTS"), DefaultProxyMaxChannelAttempts)
	cfg.ProxyStickySessionTtlMs = max(30000, parseTruncatedInt(get("PROXY_STICKY_SESSION_TTL_MS"), DefaultProxyStickySessionTtlMs))

	// ---- §3.16 Proxy: Session ----
	rt.ProxySessionChannelConcurrencyLimit = max(0, parseTruncatedInt(get("PROXY_SESSION_CHANNEL_CONCURRENCY_LIMIT"), DefaultProxySessionChannelConcurrencyLimit))
	rt.ProxySessionChannelQueueWaitMs = max(0, parseTruncatedInt(get("PROXY_SESSION_CHANNEL_QUEUE_WAIT_MS"), DefaultProxySessionChannelQueueWaitMs))

	// ---- §3.17 Proxy: Misc ----
	rt.CodexUpstreamWebsocketEnabled = parseBoolean(get("CODEX_UPSTREAM_WEBSOCKET_ENABLED"), false)
	rt.DisableCrossProtocolFallback = parseBoolean(get("DISABLE_CROSS_PROTOCOL_FALLBACK"), false)
	rt.ProxyEmptyContentFailEnabled = parseBoolean(get("PROXY_EMPTY_CONTENT_FAIL"), false)
	// PROXY_MAX_STREAM_RESPONSE_BYTES caps a single SSE stream (default 1 MB).
	// 0/negative/invalid resolves to the default so the hot stream path never
	// has to re-parse env or guard against a zero limit. Read once here.
	cfg.ProxyMaxStreamResponseBytes = parsePositiveInt(get("PROXY_MAX_STREAM_RESPONSE_BYTES"), DefaultProxyMaxStreamResponseBytes)
	// PROXY_MAX_BUFFERED_RESPONSE_BYTES caps a single non-streaming upstream
	// response (default 20 MB). 0/negative/invalid resolves to the default so
	// the buffered-response hot path never has to re-parse env or guard against
	// a zero limit. Read once here — upstream.go / upstream_stream.go /
	// executor.go read the resolved value from the config singleton.
	cfg.ProxyMaxBufferedResponseBytes = parsePositiveInt(get("PROXY_MAX_BUFFERED_RESPONSE_BYTES"), int(DefaultProxyMaxBufferedResponseBytes))
	rt.ProxyErrorKeywords = parseCsvList(get("PROXY_ERROR_KEYWORDS"))
	rt.GlobalBlockedBrands = []string{}
	rt.GlobalAllowedModels = []string{}

	// ---- §3.17b Prompt Filter (OAuth pool protection, #681) ----
	cfg.PromptFilterEnabled = parseBoolean(get("PROMPT_FILTER_ENABLED"), false)
	cfg.PromptFilterDenyPatterns = parseCsvList(get("PROMPT_FILTER_DENY_PATTERNS"))

	// ---- §3.19 Codex-specific ----
	cfg.CodexResponsesWebsocketBeta = firstNonEmpty(
		parseOptionalSecret(get("CODEX_RESPONSES_WEBSOCKET_BETA")),
		"responses_websockets=2026-02-06",
	)

	// ---- §3.20 Model Probe ----
	rt.ModelAvailabilityProbeEnabled = parseBoolean(get("MODEL_AVAILABILITY_PROBE_ENABLED"), false)
	cfg.ModelAvailabilityProbeIntervalMs = max(60000, parseTruncatedInt(get("MODEL_AVAILABILITY_PROBE_INTERVAL_MS"), DefaultModelAvailabilityProbeIntervalMs))
	cfg.ModelAvailabilityProbeTimeoutMs = max(3000, parseTruncatedInt(get("MODEL_AVAILABILITY_PROBE_TIMEOUT_MS"), DefaultModelAvailabilityProbeTimeoutMs))
	cfg.ModelAvailabilityProbeConcurrency = ClampInt(
		parseTruncatedInt(get("MODEL_AVAILABILITY_PROBE_CONCURRENCY"), DefaultModelAvailabilityProbeConcurrency),
		1, 16,
	)

	cfg.RouteRebuildProbeFilterEnabled = parseBoolean(get("ROUTE_REBUILD_PROBE_FILTER_ENABLED"), false)
	cfg.RouteRebuildProbeFilterIncludeModels = parseCsvList(get("ROUTE_REBUILD_PROBE_FILTER_INCLUDE_MODELS"))
	cfg.RouteRebuildProbeFilterExcludeModels = parseCsvList(get("ROUTE_REBUILD_PROBE_FILTER_EXCLUDE_MODELS"))

	// ---- §3.21 Retention ----
	cfg.ProxyLogRetentionDays = max(0, parseTruncatedInt(get("PROXY_LOG_RETENTION_DAYS"), DefaultProxyLogRetentionDays))
	cfg.ProxyLogRetentionPruneIntervalMinutes = max(1, parseTruncatedInt(get("PROXY_LOG_RETENTION_PRUNE_INTERVAL_MINUTES"), DefaultProxyLogRetentionPruneIntervalMinutes))
	cfg.ProxyVideoTaskRetentionDays = max(0, parseTruncatedInt(get("PROXY_VIDEO_TASK_RETENTION_DAYS"), DefaultProxyVideoTaskRetentionDays))
	cfg.ProxyVideoTaskRetentionPruneIntervalMinutes = max(1, parseTruncatedInt(get("PROXY_VIDEO_TASK_RETENTION_PRUNE_INTERVAL_MINUTES"), DefaultProxyVideoTaskRetentionPruneIntervalMinutes))

	// ---- §3.21b Proxy Log Batch Writer ----
	// Default async=true so production gets the latency win automatically; the
	// clamp guards against malformed operator input. Batch size/interval floors
	// of 1 keep the writer functional even under tiny operator-chosen values.
	cfg.ProxyLogAsync = parseBoolean(get("PROXY_LOG_ASYNC"), DefaultProxyLogAsync)
	cfg.ProxyLogBatchSize = ClampInt(
		parseTruncatedInt(get("PROXY_LOG_BATCH_SIZE"), DefaultProxyLogBatchSize),
		1, 1000,
	)
	cfg.ProxyLogFlushIntervalMs = ClampInt(
		parseTruncatedInt(get("PROXY_LOG_FLUSH_INTERVAL_MS"), DefaultProxyLogFlushIntervalMs),
		1, 60000,
	)

	// ---- §3.21c Usage Aggregation Projection ----
	cfg.UsageProjectionIntervalMs = ClampInt(
		parseTruncatedInt(get("USAGE_PROJECTION_INTERVAL_MS"), DefaultUsageProjectionIntervalMs),
		1000, 3600000,
	)

	// ---- §3.22 Routing Weights ----
	rt.RoutingWeights = RoutingWeights{
		BaseWeightFactor: parseNumber(get("BASE_WEIGHT_FACTOR"), 0.5),
		ValueScoreFactor: parseNumber(get("VALUE_SCORE_FACTOR"), 0.5),
		CostWeight:       parseNumber(get("COST_WEIGHT"), 0.4),
		BalanceWeight:    parseNumber(get("BALANCE_WEIGHT"), 0.3),
		UsageWeight:      parseNumber(get("USAGE_WEIGHT"), 0.3),
	}

	// ---- §3.23 Payload Rules + Service Tier ----
	rt.PayloadRules = ParseJsonValue(firstNonEmpty(get("PAYLOAD_RULES_JSON"), get("PAYLOAD_RULES")))
	cfg.OpenAiServiceTierRules = ParseJsonValue(firstNonEmpty(get("OPENAI_SERVICE_TIER_RULES_JSON"), get("OPENAI_SERVICE_TIER_RULES")))

	return cfg, rt
}
