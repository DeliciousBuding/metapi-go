package config

// Default constants for Metapi Go configuration.
// These mirror the original TypeScript runtime defaults.

const (
	DefaultAuthToken             = "change-me-admin-token"
	DefaultProxyToken            = "change-me-proxy-sk-token"
	DefaultCodexClientId         = "CODEX_CLIENT_ID_PLACEHOLDER"
	DefaultClaudeClientId        = "CLAUDE_CLIENT_ID_PLACEHOLDER"
	DefaultGeminiCliClientId     = "GEMINI_CLI_CLIENT_ID_PLACEHOLDER"
	DefaultGeminiCliClientSecret = "GEMINI_CLI_CLIENT_SECRET_PLACEHOLDER"

	DefaultPort    = 4000
	DefaultDataDir = "./data"
	DefaultDbType  = "sqlite"

	// DefaultLogLevel is the slog threshold applied at startup when LOG_LEVEL
	// is unset or invalid. "info" preserves the pre-config behavior so the
	// hot-path Debug downgrades stay silent unless an operator opts in.
	DefaultLogLevel = "info"

	// DbProfile selects PostgreSQL pool presets. Explicit DB_MAX_* env vars
	// always override the profile numbers. Dedicated/large-DB users can set
	// DB_PROFILE=dedicated or raise DB_MAX_OPEN_CONNS freely.
	//
	// shared-tiny — multi-tenant / role LIMIT 1–3 (e.g. shared managed PostgreSQL B1ms-class)
	// normal — default; small-to-medium single service
	// dedicated — large exclusive PostgreSQL (legacy 20/5)
	DefaultDbProfile = "normal"

	DefaultDbMaxOpenConnsSharedTiny = 2
	DefaultDbMaxIdleConnsSharedTiny = 1
	DefaultDbMaxOpenConnsNormal     = 10
	DefaultDbMaxIdleConnsNormal     = 3
	DefaultDbMaxOpenConnsDedicated  = 20
	DefaultDbMaxIdleConnsDedicated  = 5

	DefaultDbConnMaxLifetimeSec = 1800
	DefaultDbConnMaxIdleTimeSec = 300

	// RequestBodyLimit / FileUploadLimit. Parsed from MB env vars in Load:
	//   REQUEST_BODY_LIMIT_MB  (default 20, clamped [1, 200])
	//   FILE_UPLOAD_LIMIT_MB   (default 100, clamped [1, 1000])
	// DefaultRequestBodyLimit is kept as the byte equivalent of 20 MB for
	// backward-compatible struct literals in tests and callers that don't
	// run Load().
	DefaultRequestBodyLimitMB = 20
	DefaultFileUploadLimitMB  = 100
	DefaultRequestBodyLimit   = DefaultRequestBodyLimitMB * 1024 * 1024 // 20 MB

	// Per-IP rate limiting for /v1 proxy routes. Default 60 RPM per IP.
	// 0 disables the per-IP limiter entirely.
	DefaultProxyRateLimitRPM = 60

	// Pricing catalog (model data source registry: llm-metadata primary +
	// models.dev fallback by default) cold-start cost signal + marketplace
	// hydration. Refresh cadence is 12h — the primary upstream dataset is
	// rebuilt daily, so the old 60min cadence wasted bandwidth.
	// PRICING_CATALOG_URL is honored as a legacy single-source override and
	// migrated into the registry as the top-priority custom source.
	DefaultPricingCatalogEnabled    = true
	DefaultPricingCatalogRefreshMin = 720
	DefaultPricingCatalogURL        = "https://models.dev/api.json"

	// Admin/OAuth per-IP token-bucket rate limits. These mirror the original
	// hardcoded router values so existing deployments are unchanged without
	// env overrides.
	DefaultAdminRateLimitRPS   = 100
	DefaultAdminRateLimitBurst = 200
	DefaultOAuthRateLimitRPS   = 10
	DefaultOAuthRateLimitBurst = 20
	// /api/auth/* (login/logout/session/ws-ticket) gets the same strict
	// defaults as OAuth: it is the only surface that accepts the master
	// token (#1034 session model).
	DefaultAuthRateLimitRPS   = 10
	DefaultAuthRateLimitBurst = 20

	// Admin UI session defaults (#1034). 720 minutes = 12h sliding TTL,
	// matching the legacy client-side session window; the credential now
	// lives server-side (HttpOnly cookie) instead of localStorage.
	DefaultAdminSessionTTLMinutes = 720
	// "auto" sets the Secure flag based on the request protocol so local
	// plain-HTTP dev works while HTTPS deployments stay protected.
	DefaultAdminSessionCookieSecure = "auto"

	TokenRouterFailureCooldownMaxSecCeiling = 30 * 24 * 60 * 60 // 30 days

	DefaultCheckinCron          = "0 8 * * *"
	DefaultCheckinIntervalHours = 6
	DefaultBalanceRefreshCron   = "0 * * * *"
	// DefaultModelSyncCron drives the periodic upstream model-list sync
	// (MODEL_SYNC_CRON env / settings key model_sync_cron). Daily 04:00 by
	// default; operators can widen to weekly etc. via env or the settings API.
	DefaultModelSyncCron  = "0 4 * * *"
	DefaultLogCleanupCron = "0 6 * * *"

	// Site & Branding defaults. Empty means the embedded frontend branding and
	// login-page copy are used unchanged.
	DefaultSystemName    = ""
	DefaultLogo          = ""
	DefaultFooter        = ""
	DefaultAbout         = ""
	DefaultServerAddress = ""

	DefaultNotifyCooldownSec = 300
	DefaultSmtpPort          = 587

	DefaultTokenRouterCacheTtlMs               = 1500
	DefaultProxyMaxChannelAttempts             = 3
	DefaultProxyStickySessionTtlMs             = 30 * 60 * 1000 // 30 minutes
	DefaultProxySessionChannelConcurrencyLimit = 2
	DefaultProxySessionChannelQueueWaitMs      = 1500
	DefaultProxySessionChannelLeaseTtlMs       = 90000
	DefaultProxySessionChannelLeaseKeepaliveMs = 15000

	DefaultModelAvailabilityProbeIntervalMs  = 30 * 60 * 1000 // 30 minutes
	DefaultModelAvailabilityProbeTimeoutMs   = 15000
	DefaultModelAvailabilityProbeConcurrency = 1

	DefaultProxyLogRetentionDays                  = 30
	DefaultProxyLogRetentionPruneIntervalMinutes  = 30
	DefaultProxyFileRetentionDays                 = 30
	DefaultProxyFileRetentionPruneIntervalMinutes = 60
	// Video task mappings are short-lived id rewrites, so they retire faster
	// than proxy logs/files: 7 days. The same knob bounds the process-local
	// rewrite cache in handler/proxy. <=0 remains an explicit operator opt-out
	// (retention disabled), it is just no longer the default.
	DefaultProxyVideoTaskRetentionDays                 = 7
	DefaultProxyVideoTaskRetentionPruneIntervalMinutes = 60

	// Proxy log batch writer (async INSERT batching). Default async=true so
	// production gets the latency/lock-contention win automatically; tests and
	// e2e suites set PROXY_LOG_ASYNC=false for write-through visibility.
	DefaultProxyLogAsync           = true
	DefaultProxyLogBatchSize       = 50
	DefaultProxyLogFlushIntervalMs = 1000

	DefaultProxyDebugRetentionHours = 24
	DefaultProxyDebugMaxBodyBytes   = 262144

	// DefaultProxyMaxStreamResponseBytes caps the total bytes relayed for a
	// single SSE stream before a controlled termination (64 MB). Parsed from
	// PROXY_MAX_STREAM_RESPONSE_BYTES; 0/negative/invalid falls back to this
	// default. Read once at startup via config.Load — the stream handler reads
	// the resolved value from the config singleton, not os.Getenv per request.
	DefaultProxyMaxStreamResponseBytes = 64 << 20 // 64 MB

	// DefaultProxyMaxBufferedResponseBytes caps the total bytes read for a
	// single non-streaming upstream response before a controlled termination
	// (20 MB). Parsed from PROXY_MAX_BUFFERED_RESPONSE_BYTES; 0/negative/invalid
	// falls back to this default. Read once at startup via config.Load so the
	// buffered-response hot path reads a struct field instead of re-parsing
	// os.Getenv + strconv.ParseInt on every proxied request.
	DefaultProxyMaxBufferedResponseBytes int64 = 20 << 20 // 20 MB

	TelegramApiBaseUrl = "https://api.telegram.org"

	// DefaultLDOHBaseURL is the upstream LDOH (LdoHub) dashboard URL proxied
	// by the /monitor-proxy/ldoh/* admin surface. Parsed from LDOH_BASE_URL
	// so operators can point the monitor iframe at a self-hosted LDOH
	// instance without rebuilding the binary.
	DefaultLDOHBaseURL = "https://ldoh.105117.xyz"

	// DefaultLDOHProxyTimeoutSec is the per-request timeout for the LDOH
	// upstream HTTP client used by the /monitor-proxy/ldoh/* admin surface.
	// Parsed from LDOH_PROXY_TIMEOUT_SEC; 0/negative/invalid falls back to
	// this default. Matches the previous hardcoded 30s so existing
	// deployments are unchanged without an env override.
	DefaultLDOHProxyTimeoutSec = 30

	// Default outbound proxy / upstream HTTP timeouts in seconds (#1009).
	// Parsed from the PROXY_*_TIMEOUT_SEC env vars; 0/negative/invalid falls
	// back to these defaults at Load time. Values match the timeouts that were
	// hardcoded in platform/site_proxy.go before #1009, so deployments without
	// env overrides keep identical behavior.
	DefaultProxyConnectTimeoutSec        = 2  // TCP dial (connect) timeout
	DefaultProxyTLSHandshakeTimeoutSec   = 10 // TLS handshake timeout
	DefaultProxyResponseHeaderTimeoutSec = 30 // wait for upstream response headers
	DefaultProxyIdleConnTimeoutSec       = 90 // idle keep-alive connection TTL
	DefaultProxyRequestTimeoutSec        = 30 // whole-request http.Client timeout
	// DefaultProxyStreamIdleTimeoutSec bounds the gap between SSE chunks on a
	// flowing stream (each relayed chunk resets the window). 300 aligns with
	// new-api's STREAMING_TIMEOUT; the timeout distinguishes a stalled stream
	// from a long-but-healthy one, which the whole-request timeout cannot.
	DefaultProxyStreamIdleTimeoutSec = 300
)
