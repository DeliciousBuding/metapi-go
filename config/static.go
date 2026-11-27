package config

import (
	"sync/atomic"
)

var staticCfg atomic.Pointer[Config]

// Set stores the global config singleton. The published Config is static:
// every field is frozen at boot before this call. Fields that may change at
// runtime live in RuntimeSettings (see SetRuntime/UpdateRuntime).
func Set(cfg *Config) {
	staticCfg.Store(cfg)
}

// Get returns the global config singleton.
// Panics if config has not been loaded.
func Get() *Config {
	cfg := staticCfg.Load()
	if cfg == nil {
		panic("config.Get() called before config.Set() — load config first")
	}
	return cfg
}

// GetSafe returns the global config singleton, or nil when it has not been
// loaded yet. Prefer Get() in the composition root; GetSafe is for optional
// callbacks that may run before (or without) config initialization.
func GetSafe() *Config {
	return staticCfg.Load()
}

// RoutingWeights holds the weight factors for the routing algorithm.
type RoutingWeights struct {
	BaseWeightFactor float64
	ValueScoreFactor float64
	CostWeight       float64
	BalanceWeight    float64
	UsageWeight      float64
}

// Config holds ALL configuration fields for the Metapi server.
// Field names use Go exported (PascalCase) but each maps 1:1 to a TS config field
// as documented in section 3.
type Config struct {
	AccountCredentialSecret string
	CodexClientId           string

	// OAuth Clients (3 fields)
	ClaudeClientId          string
	GeminiCliClientId       string
	GeminiCliClientSecret   string
	AntigravityClientSecret string

	// Server (7 fields)
	Port       int
	ListenHost string
	DataDir    string
	DbSslMode  string
	// PostgreSQL pool budget. Production operators must size MaxOpenConns no
	// higher than the database role CONNECTION LIMIT.
	// DbProfile is a convenience preset (shared-tiny|normal|dedicated);
	// explicit DB_MAX_* values always win when set.
	DbProfile            string
	DbMaxOpenConns       int
	DbMaxIdleConns       int
	DbConnMaxLifetimeSec int
	DbConnMaxIdleTimeSec int
	// DbApplicationName is injected into the PostgreSQL DSN as
	// application_name (default metapi-<hostname>) for pg_stat_activity.
	DbApplicationName string
	Tz                string

	// LogLevel controls the slog threshold applied at startup (env LOG_LEVEL).
	// Accepted values: debug, info, warn, error. Default "info". Raising the
	// threshold silences per-request / SSE / WS Debug-downgraded hot-path logs
	// that are already observable via Prometheus metrics, cutting log volume in
	// production without losing Warn/Error signal.
	LogLevel string

	// LogCleanupConfigured reports whether the log_cleanup regime owns the log
	// tables (true) or the legacy PROXY_LOG_RETENTION_DAYS pruner does (false).
	// Resolved during startup settings hydration, not by Load.
	LogCleanupConfigured bool

	// LogCleanupEnvEnabled records that an env toggle explicitly asked for the
	// log_cleanup regime: LOG_CLEANUP_USAGE_LOGS_ENABLED=true or
	// LOG_CLEANUP_PROGRAM_LOGS_ENABLED=true. Startup hydration ORs it into
	// LogCleanupConfigured so an env-driven deployment keeps its intent even
	// with an empty settings table.
	//
	// Deliberately one-directional: neither LOG_CLEANUP_RETENTION_DAYS nor
	// LOG_CLEANUP_CRON counts, and an explicit false does not count either.
	// Claiming the regime while both toggles are off would run the new scheduler
	// (which then skips for want of a target) and disable the legacy pruner, so
	// nothing would ever be pruned.
	LogCleanupEnvEnabled bool

	// Resin sticky proxy pool (3 fields, env-only — no schema change).
	// RESIN_URL carries the base URL + token, e.g. http://resin.local:2260/my-token.
	// RESIN_PLATFORM_NAME is the Platform identity (falls back to site.Platform).
	// RESIN_ENABLED is the global opt-in (default false).
	ResinURL          string
	ResinPlatformName string
	ResinEnabled      bool

	// uTLS TLS fingerprint masking (1 field, env-only — no schema change).
	// UTLS_ENABLED is the global opt-in (default false). When true, all
	// outbound platform requests use a uTLS Chrome-ClientHello transport
	// instead of Go's default crypto/tls ClientHello, masking the JA3/JA4
	// fingerprint that Cloudflare and similar WAFs use to block automated
	// traffic. Per-site use_utls overrides this flag (see service.UTLSEnabled).
	UTLSEnabled bool

	// Outbound proxy / upstream HTTP timeouts (5 fields, env-only — no DDL).
	// Integer seconds applied to every outbound site-proxy / upstream request
	// (platform.SiteProxy, DoWithProxy, pooled transports). Parsed from the
	// PROXY_*_TIMEOUT_SEC env vars; 0/negative/invalid falls back to the
	// Default* constants at Load time, which match the pre-#1009 hardcoded
	// values so unset deployments keep identical behavior.
	ProxyConnectTimeoutSec        int // dial / TCP connect
	ProxyTLSHandshakeTimeoutSec   int // TLS handshake
	ProxyResponseHeaderTimeoutSec int // wait for upstream response headers
	ProxyIdleConnTimeoutSec       int // idle keep-alive connection TTL
	ProxyRequestTimeoutSec        int // whole-request http.Client timeout
	// ProxyStreamIdleTimeoutSec bounds the gap between SSE chunks once a
	// stream is flowing: each relayed chunk resets the window, and expiry
	// aborts the stalled stream. Parsed from PROXY_STREAM_IDLE_TIMEOUT_SEC;
	// 0/negative/invalid falls back to DefaultProxyStreamIdleTimeoutSec.
	// Unlike the five transport timeouts above this applies to the relayed
	// body phase only — non-streaming responses never see it.
	ProxyStreamIdleTimeoutSec int
	// ProxyStreamWriteTimeoutSec bounds one downstream write+flush for every
	// relayed stream: SSE and native audio/video byte streams share it. The
	// relay arms it immediately before writing and clears it immediately after,
	// so it never limits total stream duration or the wait for the next
	// upstream chunk; a client that stops reading cannot pin a relay (and its
	// site slot) forever. Parsed from PROXY_STREAM_WRITE_TIMEOUT_SEC, with the
	// pre-generalization PROXY_SSE_WRITE_TIMEOUT_SEC accepted as an alias;
	// 0/negative/invalid falls back to DefaultProxyStreamWriteTimeoutSec.
	ProxyStreamWriteTimeoutSec int
	// ProxyDirectAccountMaxConcurrency caps concurrent proxy dispatches per
	// direct upstream account, keyed by upstream_credentials.id. Direct
	// channels have no site row to carry MaxConcurrency, so this is their
	// equivalent isolation knob. Parsed from
	// PROXY_DIRECT_ACCOUNT_MAX_CONCURRENCY; 0 (or any non-positive value)
	// means unlimited, which is the historical behavior. The cap is
	// process-local and read once at startup.
	ProxyDirectAccountMaxConcurrency int

	// LDOHBaseURL is the upstream LDOH dashboard URL proxied through
	// /monitor-proxy/ldoh/* (env-only — no DDL). Parsed from LDOH_BASE_URL;
	// defaults to DefaultLDOHBaseURL so operators can redirect the monitor
	// iframe at a self-hosted LDOH instance without a code change.
	LDOHBaseURL string
	// LDOHProxyTimeoutSec is the per-request timeout for the LDOH upstream
	// HTTP client used by the /monitor-proxy/ldoh/* admin surface (env-only —
	// no DDL). Parsed from LDOH_PROXY_TIMEOUT_SEC (default 30). Previously
	// hardcoded to 30s in handler/admin/monitor.go; making it configurable
	// lets operators raise the budget for slow LDOH instances or tighten it
	// to fail fast. 0/negative/invalid falls back to the default at Load time.
	LDOHProxyTimeoutSec int

	// RedisURL enables optional shared admission counters.
	RedisURL string

	AdminCorsAllowedOrigins []string
	TrustedProxyCidrs       []string
	// AdminRateLimitRPS / AdminRateLimitBurst control the per-IP token bucket
	// for /api/* admin routes. Parsed from ADMIN_RATE_LIMIT_RPS (default 100)
	// and ADMIN_RATE_LIMIT_BURST (default 200). A burst of 0 means the bucket
	// never refills above the RPS rate; values <= 0 disable the limiter.
	AdminRateLimitRPS   int
	AdminRateLimitBurst int
	// OAuthRateLimitRPS / OAuthRateLimitBurst control the stricter per-IP
	// token bucket applied to /api/oauth/* routes. Parsed from
	// OAUTH_RATE_LIMIT_RPS (default 10) and OAUTH_RATE_LIMIT_BURST (default 20).
	OAuthRateLimitRPS   int
	OAuthRateLimitBurst int
	// AuthRateLimitRPS / AuthRateLimitBurst control the stricter per-IP
	// token bucket applied to /api/auth/* routes (login is the only place
	// the master token is presented, so brute-force protection matters).
	// Parsed from AUTH_RATE_LIMIT_RPS (default 10) and AUTH_RATE_LIMIT_BURST
	// (default 20).
	AuthRateLimitRPS   int
	AuthRateLimitBurst int
	// AdminSessionTTLMinutes is the sliding lifetime of a server-side admin
	// UI session (#1034). Every authenticated request refreshes the expiry;
	// the raw master token never reaches the browser. Parsed from
	// ADMIN_SESSION_TTL_MINUTES (default 720 = 12h, matching the legacy
	// client-side session window). Values < 1 clamp to 1.
	AdminSessionTTLMinutes int
	// AdminSessionCookieSecure controls the Secure flag of the session
	// cookie: "auto" (default; Secure when the request is HTTPS), "true"
	// (always Secure -- use behind a TLS terminator), or "false" (never
	// Secure -- plain-HTTP dev only). Parsed from ADMIN_SESSION_COOKIE_SECURE.
	AdminSessionCookieSecure string

	// Proxy: Core (5 fields)
	RequestBodyLimit int
	// FileUploadLimitBytes is the higher body limit applied to multipart upload
	// routes (/v1/files, /v1/images/*) so large uploads are not rejected by the
	// general RequestBodyLimit. Parsed from FILE_UPLOAD_LIMIT_MB.
	FileUploadLimitBytes int
	// ProxyRateLimitRPM is the per-IP request-per-minute cap for /v1 proxy
	// routes. Parsed from PROXY_RATE_LIMIT_RPM (default 60; 0 = disabled).
	ProxyRateLimitRPM int
	// ProxyGlobalTokenRPM supplies the initial max_rpm for a migrated root key. Parsed
	// from PROXY_GLOBAL_TOKEN_RPM (default 0 = unlimited). Safety net: even if
	// the token leaks, upstream spend is bounded.
	ProxyGlobalTokenRPM int

	TokenRouterCacheTtlMs int

	// PricingCatalog (model data source registry) feeds the cold-start
	// catalog cost signal for cost-aware routing and hydrates the models
	// marketplace.
	//   PRICING_CATALOG_ENABLED      — default true
	//   PRICING_CATALOG_REFRESH_MIN  — auto-sync period in minutes (default 720 = 12h)
	//   PRICING_CATALOG_URL          — legacy single-source dataset URL; migrated
	//                                  into the registry as the top custom source
	PricingCatalogEnabled      bool
	PricingCatalogRefreshMin   int
	PricingCatalogURL          string
	PricingCatalogAllowPrivate bool

	// Proxy: Channel (2 fields)
	ProxyMaxChannelAttempts int
	ProxyStickySessionTtlMs int

	// ProxyMaxStreamResponseBytes caps the total bytes relayed for a single
	// SSE stream before a controlled termination (default 1 MB). Parsed once
	// at startup from PROXY_MAX_STREAM_RESPONSE_BYTES so the hot stream path
	// reads a struct field instead of calling os.Getenv per request.
	ProxyMaxStreamResponseBytes int
	// ProxyMaxBufferedResponseBytes caps the total bytes read for a single
	// non-streaming upstream response before a controlled termination
	// (default 20 MB). Parsed once at startup from
	// PROXY_MAX_BUFFERED_RESPONSE_BYTES so the buffered-response hot path
	// (upstream.go / upstream_stream.go / executor.go) reads a struct field
	// instead of re-parsing os.Getenv + strconv.ParseInt on every proxied
	// request. 0/negative/invalid falls back to the default at Load time.
	ProxyMaxBufferedResponseBytes int

	// Prompt Filter (OAuth account pool protection, #681).
	// PROMPT_FILTER_ENABLED gates a pre-upstream pattern-based safety filter
	// that blocks jailbreak/exfiltration prompts before they reach shared OAuth
	// accounts. Default false — opt-in only. PROMPT_FILTER_DENY_PATTERNS is a
	// comma-separated list of extra substring patterns appended to the seed list.
	PromptFilterEnabled      bool
	PromptFilterDenyPatterns []string

	// Codex-specific (2 fields)
	CodexResponsesWebsocketBeta string

	// Model Probe (3 fields; ModelAvailabilityProbeEnabled is runtime-mutable
	// and lives in RuntimeSettings)
	ModelAvailabilityProbeIntervalMs  int
	ModelAvailabilityProbeTimeoutMs   int
	ModelAvailabilityProbeConcurrency int

	// RouteRebuildProbeFilterEnabled gates the route-rebuild probe filter (#625).
	// Default false keeps the legacy non-probe rebuild path byte-for-byte.
	RouteRebuildProbeFilterEnabled       bool
	RouteRebuildProbeFilterIncludeModels []string
	RouteRebuildProbeFilterExcludeModels []string

	// Retention (4 fields)
	ProxyLogRetentionDays                       int
	ProxyLogRetentionPruneIntervalMinutes       int
	ProxyVideoTaskRetentionDays                 int
	ProxyVideoTaskRetentionPruneIntervalMinutes int

	// Proxy Log Batch Writer (3 fields). PROXY_LOG_ASYNC (default true) routes
	// proxy_logs INSERTs through a background batch writer that decouples the
	// hot proxy path from DB write latency / SQLite write-lock contention.
	// Set false for tests/e2e that need write-through visibility. BATCH_SIZE
	// (default 50) is the row count that triggers a flush; FLUSH_INTERVAL_MS
	// (default 1000) is the max time between flushes.
	ProxyLogAsync           bool
	ProxyLogBatchSize       int
	ProxyLogFlushIntervalMs int

	// Usage aggregation (1 field). USAGE_PROJECTION_INTERVAL_MS is the cadence
	// of the proxy_logs -> site_day_usage / site_hour_usage / model_day_usage
	// projection pass. Clamped to [1000, 3600000] ms.
	UsageProjectionIntervalMs int

	OpenAiServiceTierRules any
}
