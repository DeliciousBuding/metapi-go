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

	DefaultRequestBodyLimit = 20 * 1024 * 1024 // 20 MB

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

	TelegramApiBaseUrl = "https://api.telegram.org"
)
