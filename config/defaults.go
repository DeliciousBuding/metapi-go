package config

// Default constants for MetAPI Go configuration.
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

	// Pricing catalog (models.dev official list prices) cold-start cost signal.
	DefaultPricingCatalogEnabled    = true
	DefaultPricingCatalogRefreshMin = 60
	DefaultPricingCatalogURL        = "https://models.dev/api.json"

	// Admin/OAuth per-IP token-bucket rate limits. These mirror the original
	// hardcoded router values so existing deployments are unchanged without
	// env overrides.
	DefaultAdminRateLimitRPS   = 100
	DefaultAdminRateLimitBurst = 200
	DefaultOAuthRateLimitRPS   = 10
	DefaultOAuthRateLimitBurst = 20

	TokenRouterFailureCooldownMaxSecCeiling = 30 * 24 * 60 * 60 // 30 days

	DefaultCheckinCron          = "0 8 * * *"
	DefaultCheckinIntervalHours = 6
	DefaultBalanceRefreshCron   = "0 * * * *"
	DefaultLogCleanupCron       = "0 6 * * *"

	// Site & Branding defaults. Empty means the embedded frontend branding and
	// login-page copy are used unchanged.
	DefaultSystemName      = ""
	DefaultLogo            = ""
	DefaultFooter          = ""
	DefaultAbout           = ""
	DefaultHomePageContent = ""
	DefaultServerAddress   = ""

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
	// Video task mappings: default 0 = retention disabled (no silent mass delete).
	DefaultProxyVideoTaskRetentionDays                 = 0
	DefaultProxyVideoTaskRetentionPruneIntervalMinutes = 60

	// Proxy log batch writer (async INSERT batching). Default async=true so
	// production gets the latency/lock-contention win automatically; tests and
	// e2e suites set PROXY_LOG_ASYNC=false for write-through visibility.
	DefaultProxyLogAsync             = true
	DefaultProxyLogBatchSize        = 50
	DefaultProxyLogFlushIntervalMs = 1000

	DefaultProxyDebugRetentionHours = 24
	DefaultProxyDebugMaxBodyBytes   = 262144

	// DefaultProxyMaxStreamResponseBytes caps the total bytes relayed for a
	// single SSE stream before a controlled termination (1 MB). Parsed from
	// PROXY_MAX_STREAM_RESPONSE_BYTES; 0/negative/invalid falls back to this
	// default. Read once at startup via config.Load — the stream handler reads
	// the resolved value from the config singleton, not os.Getenv per request.
	DefaultProxyMaxStreamResponseBytes = 1 << 20 // 1 MB

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
)
