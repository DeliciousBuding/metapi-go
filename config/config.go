package config

import (
	"encoding/json"
	"log/slog"
	"math"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

var (
	globalCfg *Config
	cfgMu     sync.RWMutex
)

// Set stores the global config singleton.
func Set(cfg *Config) {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	globalCfg = cfg
}

// Get returns the global config singleton.
// Panics if config has not been loaded.
func Get() *Config {
	cfgMu.RLock()
	defer cfgMu.RUnlock()
	if globalCfg == nil {
		panic("config.Get() called before config.Set() — load config first")
	}
	return globalCfg
}

// GetSafe returns the global config singleton, or nil when it has not been
// loaded yet. Prefer Get() in the composition root; GetSafe is for optional
// callbacks that may run before (or without) config initialization.
func GetSafe() *Config {
	cfgMu.RLock()
	defer cfgMu.RUnlock()
	return globalCfg
}

// CodexHeaderDefaults holds Codex-specific HTTP header defaults.
type CodexHeaderDefaults struct {
	UserAgent    string
	BetaFeatures string
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
	// Auth (4 fields)
	AuthToken               string
	ProxyToken              string
	AccountCredentialSecret string
	CodexClientId           string

	// OAuth Clients (4 fields)
	ClaudeClientId        string
	ClaudeClientSecret    string
	GeminiCliClientId     string
	GeminiCliClientSecret string

	// Server (7 fields)
	Port       int
	ListenHost string
	DataDir    string
	DbType     string
	DbUrl      string
	DbSsl      bool
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

	// Cron (5 fields)
	CheckinCron          string
	CheckinScheduleMode  string
	CheckinIntervalHours int
	// window mode — random HH:mm inside [start, end]
	// re-rolled per start/setting change (load spreading + anti-fingerprint).
	CheckinWindowStart string
	CheckinWindowEnd   string
	BalanceRefreshCron string
	// Global kill switches for the two always-on periodic jobs that touch
	// upstream accounts (check-in, balance refresh) — issue #1027. Defaults
	// preserve historical behavior (both jobs on); operators turn either job
	// off via env (CHECKIN_ENABLED / BALANCE_REFRESH_ENABLED) or the runtime
	// settings of the same camelCase names. Stored inverted (zero value =
	// enabled) so bare config literals keep the default-on behavior.
	CheckinDisabled        bool
	BalanceRefreshDisabled bool
	ModelSyncCron          string
	LogCleanupCron         string
	// Site & Branding (5 fields) - empty defaults keep the embedded frontend
	// branding and login-page copy unchanged. homePageContent was removed
	// (Wave 8 Lane D): the value was stored but never rendered anywhere.
	SystemName    string
	Logo          string
	Footer        string
	About         string
	ServerAddress string

	// Log Cleanup (4 fields)
	LogCleanupUsageLogsEnabled   bool
	LogCleanupProgramLogsEnabled bool
	LogCleanupRetentionDays      int
	LogCleanupConfigured         bool

	// Notify: Webhook (2 fields)
	WebhookUrl     string
	WebhookEnabled bool

	// Notify: Bark (2 fields)
	BarkUrl     string
	BarkEnabled bool

	// Notify: ServerChan (2 fields)
	ServerChanKey     string
	ServerChanEnabled bool

	// Notify: Telegram (6 fields)
	TelegramEnabled         bool
	TelegramApiBaseUrl      string
	TelegramBotToken        string
	TelegramChatId          string
	TelegramUseSystemProxy  bool
	TelegramMessageThreadId string

	// Notify: SMTP (8 fields)
	SmtpEnabled bool
	SmtpHost    string
	SmtpPort    int
	SmtpSecure  bool
	SmtpUser    string
	SmtpPass    string
	SmtpFrom    string
	SmtpTo      string

	// Notify: Feishu (3 fields) —
	FeishuEnabled bool
	FeishuWebhook string
	FeishuSecret  string

	// Notify: DingTalk (3 fields) —
	DingtalkEnabled bool
	DingtalkWebhook string
	DingtalkSecret  string

	// Notify: WeCom (2 fields) —
	WecomEnabled bool
	WecomWebhook string

	// Notify: Ntfy (4 fields) —
	NtfyEnabled bool
	NtfyUrl     string
	NtfyTopic   string
	NtfyToken   string

	// Notify: General (2 fields)
	NotifyCooldownSec int
	SystemProxyUrl    string

	// Resin sticky proxy pool (3 fields, env-only — no DDL for Tier 1).
	// RESIN_URL carries the base URL + token, e.g. http://resin.local:2260/my-token.
	// RESIN_PLATFORM_NAME is the Platform identity (falls back to site.Platform).
	// RESIN_ENABLED is the global opt-in (default false).
	ResinURL          string
	ResinPlatformName string
	ResinEnabled      bool

	// uTLS TLS fingerprint masking (1 field, env-only — no DDL for Tier 1).
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

	// ProxyRetryStatusRanges / ProxyDisableStatusRanges carry the
	// operator-tunable status-code range specs (routing.StatusRange policy,
	// competitor-study-2026-08 P1-2): retry decides which upstream statuses
	// count as retryable channel faults; disable decides which auto-disable
	// the failing channel. Runtime settings only (settings table +
	// PUT /api/settings/runtime, rehydrated at startup); blank keeps the
	// defaults, which reproduce the historical behavior exactly.
	ProxyRetryStatusRanges   string
	ProxyDisableStatusRanges string

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

	// UpdateCenterEnabled gates the residual update-center scheduler
	// (scheduler.UpdateCenterScheduler). The scheduler is a log-only no-op
	// today (no remote registry / version discovery), so it is disabled by
	// default to avoid a 15-min lease heartbeat with no product value.
	// METAPI_ENABLE_UPDATE_CENTER=true re-enables it for operators who want
	// the periodic log line while a real helper/registry client is wired.
	UpdateCenterEnabled bool

	// NotifyTaskToggles gates per-alert-type notifications.
	// Keys are alert task slugs ("token_expired", "low_balance", "proxy_all_failed").
	// Default nil = all enabled (backward-compatible). When a key is present and
	// false, SendNotification skips that task type so operators can mute, e.g.,
	// low_balance while still receiving token_expired alerts.
	NotifyTaskToggles map[string]bool
	// RedisURL enables optional shared admission counters.
	RedisURL string

	// Admin (3 fields)
	AdminIpAllowlist        []string
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
	// ProxyGlobalTokenRPM caps the global PROXY_TOKEN across all IPs. Parsed
	// from PROXY_GLOBAL_TOKEN_RPM (default 0 = unlimited). Safety net: even if
	// the token leaks, upstream spend is bounded.
	ProxyGlobalTokenRPM     int
	RoutingFallbackUnitCost float64
	// CacheRatioDefault / CacheRatioClaude override the prompt-cache ratio
	// fallbacks used when an upstream pricing row omits cache_ratio. 0/missing =
	// use the code defaults (DefaultCacheRatio=1.0, ClaudeCacheRatio=0.1).
	CacheRatioDefault float64
	CacheRatioClaude  float64
	// ProxyFirstByteTimeoutSec is the operator-facing first-byte / first-token
	// timeout in SECONDS (env PROXY_FIRST_BYTE_TIMEOUT_SEC). Internal dispatch
	// converts to milliseconds via proxy.FirstByteTimeoutMs (sec * 1000).
	// 0 disables observed first-byte timeout.
	ProxyFirstByteTimeoutSec int

	// Proxy: Token Router (2 fields)
	TokenRouterFailureCooldownMaxSec int
	TokenRouterCacheTtlMs            int

	// PricingCatalog (model data source registry) feeds the cold-start
	// catalog cost signal for cost-aware routing and hydrates the models
	// marketplace.
	//   PRICING_CATALOG_ENABLED      — default true
	//   PRICING_CATALOG_REFRESH_MIN  — auto-sync period in minutes (default 720 = 12h)
	//   PRICING_CATALOG_URL          — legacy single-source dataset URL; migrated
	//                                  into the registry as the top custom source
	PricingCatalogEnabled    bool
	PricingCatalogRefreshMin int
	PricingCatalogURL        string

	// Proxy: Channel (3 fields)
	ProxyMaxChannelAttempts   int
	ProxyStickySessionEnabled bool
	ProxyStickySessionTtlMs   int

	// Proxy: Session (4 fields)
	ProxySessionChannelConcurrencyLimit int
	ProxySessionChannelQueueWaitMs      int
	ProxySessionChannelLeaseTtlMs       int
	ProxySessionChannelLeaseKeepaliveMs int

	// Proxy: Misc (7 fields)
	CodexUpstreamWebsocketEnabled              bool
	ResponsesCompactFallbackToResponsesEnabled bool
	DisableCrossProtocolFallback               bool
	ProxyEmptyContentFailEnabled               bool
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
	ProxyErrorKeywords            []string
	GlobalBlockedBrands           []string
	GlobalAllowedModels           []string

	// Prompt Filter (OAuth account pool protection, #681).
	// PROMPT_FILTER_ENABLED gates a pre-upstream pattern-based safety filter
	// that blocks jailbreak/exfiltration prompts before they reach shared OAuth
	// accounts. Default false — opt-in only. PROMPT_FILTER_DENY_PATTERNS is a
	// comma-separated list of extra substring patterns appended to the seed list.
	PromptFilterEnabled      bool
	PromptFilterDenyPatterns []string

	// Proxy: Debug (9 fields)
	ProxyDebugTraceEnabled        bool
	ProxyDebugCaptureHeaders      bool
	ProxyDebugCaptureBodies       bool
	ProxyDebugCaptureStreamChunks bool
	ProxyDebugTargetSessionId     string
	ProxyDebugTargetClientKind    string
	ProxyDebugTargetModel         string
	ProxyDebugRetentionHours      int
	ProxyDebugMaxBodyBytes        int

	// Codex-specific (3 fields)
	CodexResponsesWebsocketBeta string
	CodexHeaderDefaults         CodexHeaderDefaults

	// Model Probe (4 fields)
	ModelAvailabilityProbeEnabled     bool
	ModelAvailabilityProbeIntervalMs  int
	ModelAvailabilityProbeTimeoutMs   int
	ModelAvailabilityProbeConcurrency int

	// RouteRebuildProbeFilterEnabled gates the route-rebuild probe filter (#625).
	// Default false keeps the legacy non-probe rebuild path byte-for-byte.
	RouteRebuildProbeFilterEnabled       bool
	RouteRebuildProbeFilterIncludeModels []string
	RouteRebuildProbeFilterExcludeModels []string

	// Retention (6 fields)
	ProxyLogRetentionDays                       int
	ProxyLogRetentionPruneIntervalMinutes       int
	ProxyFileRetentionDays                      int
	ProxyFileRetentionPruneIntervalMinutes      int
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

	// Routing Weights (5 fields)
	RoutingWeights RoutingWeights

	// Payload Rules (2 JSON fields)
	PayloadRules           any
	OpenAiServiceTierRules any
}

// ---------------------------------------------------------------------------
// §1 Parse functions — must match TS behavior byte-for-byte
// ---------------------------------------------------------------------------

// §1.1 parseBoolean: "" → fallback; trim+lower → "1"/"true"/"yes"/"on" → true; else false.
func parseBoolean(value string, fallback bool) bool {
	if value == "" {
		return fallback
	}
	normalized := strings.ToLower(strings.TrimSpace(value))
	return normalized == "1" || normalized == "true" || normalized == "yes" || normalized == "on"
}

// §1.2 parseNumber: "" → fallback; ParseFloat → NaN/Inf → fallback.
func parseNumber(value string, fallback float64) float64 {
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsInf(parsed, 0) || math.IsNaN(parsed) {
		return fallback
	}
	return parsed
}

// §1.2b parseTimeoutSec: integer-seconds timeout env var. "" / invalid /
// <=0 → def, so a misconfigured value never disables a timeout (same clamp
// pattern as LDOH_PROXY_TIMEOUT_SEC).
func parseTimeoutSec(value string, def int) int {
	parsed := int(math.Trunc(parseNumber(value, float64(def))))
	if parsed <= 0 {
		return def
	}
	return parsed
}

// §1.3 parseCsvList: "" → []; split by "," → trim each → filter len>0.
func parseCsvList(value string) []string {
	if value == "" {
		return []string{}
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if len(trimmed) > 0 {
			result = append(result, trimmed)
		}
	}
	return result
}

// §1.4 parseOptionalSecret: TrimSpace; "" stays "".
func parseOptionalSecret(value string) string {
	return strings.TrimSpace(value)
}

// §1.5 parseJsonValue: "" → nil; Unmarshal failure → nil (no panic).
func parseJsonValue(value string) any {
	if value == "" {
		return nil
	}
	var result any
	if err := json.Unmarshal([]byte(value), &result); err != nil {
		return nil
	}
	return result
}

// §1.6 parseDbType: "" → "sqlite"; trim+lower → "mysql"/"postgres"/"postgresql" → "postgres"; else "sqlite".
func parseDbType(value string) string {
	if value == "" {
		return "sqlite"
	}
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "mysql":
		return "mysql"
	case "postgres", "postgresql":
		return "postgres"
	default:
		return "sqlite"
	}
}

func inferDbType(value string, dbURL string) string {
	if strings.TrimSpace(value) != "" {
		return parseDbType(value)
	}
	normalizedURL := strings.ToLower(strings.TrimSpace(dbURL))
	if strings.HasPrefix(normalizedURL, "postgres://") || strings.HasPrefix(normalizedURL, "postgresql://") {
		return "postgres"
	}
	return "sqlite"
}

func normalizeDbSslMode(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func validDbSslMode(value string) bool {
	switch normalizeDbSslMode(value) {
	case "", "disable", "allow", "prefer", "require", "verify-ca", "verify-full":
		return true
	default:
		return false
	}
}

// PostgresSSLMode returns the explicit DB_SSLMODE setting, or the legacy
// DB_SSL=true behavior mapped to sslmode=require.
func (c *Config) PostgresSSLMode() string {
	if mode := normalizeDbSslMode(c.DbSslMode); mode != "" {
		return mode
	}
	if c.DbSsl {
		return "require"
	}
	return ""
}

// §1.7 normalizeTokenRouterFailureCooldownMaxSec:
// !finite || <= 0 → (0, false); trunc → clamp[1, ceiling] → (int, true).
func normalizeTokenRouterFailureCooldownMaxSec(value float64) (int, bool) {
	if math.IsInf(value, 0) || math.IsNaN(value) || value <= 0 {
		return 0, false
	}
	truncated := math.Trunc(value)
	clamped := int(math.Max(1, math.Min(float64(TokenRouterFailureCooldownMaxSecCeiling), truncated)))
	return clamped, true
}

// §1.8 parseListenHost: explicit HOST always wins. Local Windows binaries
// default to loopback so changing go-build/go-run executable paths does not
// trigger recurring inbound firewall prompts. Server/container platforms keep
// the historical all-interface default.
func parseListenHost(env map[string]string) string {
	return parseListenHostForOS(env, runtime.GOOS)
}

func parseListenHostForOS(env map[string]string, goos string) string {
	host := env["HOST"]
	trimmed := strings.TrimSpace(host)
	if trimmed != "" {
		return trimmed
	}
	if goos == "windows" {
		return "127.0.0.1"
	}
	return "0.0.0.0"
}

// ---------------------------------------------------------------------------
// §3 Load — build Config from env map
// ---------------------------------------------------------------------------

// Load reads environment variables from the given map and returns a fully
// populated Config. The caller (main.go) is responsible for loading.env files
// via godotenv before calling Load.
func Load(env map[string]string) *Config {
	// Helper to read from the env map; Go map access returns "" for missing keys.
	get := func(key string) string {
		return env[key]
	}

	// Resolve ACCOUNT_CREDENTIAL_SECRET: env var → AUTH_TOKEN → default
	resolveAccountCredentialSecret := func() string {
		if v := get("ACCOUNT_CREDENTIAL_SECRET"); v != "" {
			return v
		}
		if v := get("AUTH_TOKEN"); v != "" {
			return v
		}
		return DefaultAuthToken
	}

	// Resolve PAYLOAD_RULES from legacy alias
	resolvePayloadRules := func() string {
		if v := get("PAYLOAD_RULES_JSON"); v != "" {
			return v
		}
		return get("PAYLOAD_RULES")
	}

	// Resolve OPENAI_SERVICE_TIER_RULES from legacy alias
	resolveOpenAiServiceTierRules := func() string {
		if v := get("OPENAI_SERVICE_TIER_RULES_JSON"); v != "" {
			return v
		}
		return get("OPENAI_SERVICE_TIER_RULES")
	}

	cfg := &Config{}

	// ---- §3.1 Auth ----
	cfg.AuthToken = firstNonEmpty(get("AUTH_TOKEN"), DefaultAuthToken)
	cfg.ProxyToken = firstNonEmpty(get("PROXY_TOKEN"), DefaultProxyToken)
	cfg.AccountCredentialSecret = resolveAccountCredentialSecret()
	if cfg.AccountCredentialSecret == DefaultAuthToken {
		slog.Warn("config: AccountCredentialSecret is using the default value — this is insecure for production. Set ACCOUNT_CREDENTIAL_SECRET or AUTH_TOKEN environment variable.")
	}
	cfg.CodexClientId = firstNonEmpty(parseOptionalSecret(get("CODEX_CLIENT_ID")), DefaultCodexClientId)

	// ---- §3.2 OAuth Clients ----
	cfg.ClaudeClientId = firstNonEmpty(parseOptionalSecret(get("CLAUDE_CLIENT_ID")), DefaultClaudeClientId)
	cfg.ClaudeClientSecret = parseOptionalSecret(get("CLAUDE_CLIENT_SECRET"))
	cfg.GeminiCliClientId = firstNonEmpty(parseOptionalSecret(get("GEMINI_CLI_CLIENT_ID")), DefaultGeminiCliClientId)
	cfg.GeminiCliClientSecret = firstNonEmpty(parseOptionalSecret(get("GEMINI_CLI_CLIENT_SECRET")), DefaultGeminiCliClientSecret)

	// ---- §3.3 Server ----
	cfg.Port = int(math.Trunc(parseNumber(get("PORT"), DefaultPort)))
	cfg.ListenHost = parseListenHost(env)
	cfg.DataDir = firstNonEmpty(get("DATA_DIR"), DefaultDataDir)
	cfg.DbUrl = strings.TrimSpace(firstNonEmpty(get("DB_URL"), get("DATABASE_URL")))
	cfg.DbType = inferDbType(get("DB_TYPE"), cfg.DbUrl)
	cfg.DbSsl = parseBoolean(get("DB_SSL"), false)
	cfg.DbSslMode = normalizeDbSslMode(get("DB_SSLMODE"))
	cfg.DbProfile = normalizeDbProfile(firstNonEmpty(get("DB_PROFILE"), get("METAPI_DB_PROFILE"), DefaultDbProfile))
	openDefault, idleDefault := dbPoolDefaultsForProfile(cfg.DbProfile)
	cfg.DbMaxOpenConns = int(math.Trunc(parseNumber(get("DB_MAX_OPEN_CONNS"), float64(openDefault))))
	cfg.DbMaxIdleConns = int(math.Trunc(parseNumber(get("DB_MAX_IDLE_CONNS"), float64(idleDefault))))
	cfg.DbConnMaxLifetimeSec = int(math.Trunc(parseNumber(get("DB_CONN_MAX_LIFETIME_SEC"), float64(DefaultDbConnMaxLifetimeSec))))
	cfg.DbConnMaxIdleTimeSec = int(math.Trunc(parseNumber(get("DB_CONN_MAX_IDLE_TIME_SEC"), float64(DefaultDbConnMaxIdleTimeSec))))
	cfg.DbApplicationName = strings.TrimSpace(firstNonEmpty(get("DB_APPLICATION_NAME"), get("METAPI_DB_APPLICATION_NAME")))
	cfg.Tz = get("TZ")
	cfg.LogLevel = normalizeLogLevel(firstNonEmpty(get("LOG_LEVEL"), DefaultLogLevel))

	// ---- §3.4 Cron ----
	cfg.CheckinCron = firstNonEmpty(get("CHECKIN_CRON"), DefaultCheckinCron)
	checkinMode := strings.ToLower(strings.TrimSpace(get("CHECKIN_SCHEDULE_MODE")))
	if checkinMode == "interval" {
		cfg.CheckinScheduleMode = "interval"
	} else if checkinMode == "window" {
		cfg.CheckinScheduleMode = "window"
	} else {
		cfg.CheckinScheduleMode = "cron"
	}
	// E1: window bounds (HH:mm, 24h). Defaults: 00:00-23:59 = any time of day.
	cfg.CheckinWindowStart = firstNonEmpty(get("CHECKIN_WINDOW_START"), "00:00")
	cfg.CheckinWindowEnd = firstNonEmpty(get("CHECKIN_WINDOW_END"), "23:59")
	cfg.CheckinIntervalHours = ClampInt(
		int(math.Trunc(parseNumber(get("CHECKIN_INTERVAL_HOURS"), DefaultCheckinIntervalHours))),
		1, 24,
	)
	cfg.BalanceRefreshCron = firstNonEmpty(get("BALANCE_REFRESH_CRON"), DefaultBalanceRefreshCron)
	// #1027: global enable switches for the upstream-touching account jobs.
	// Absent/invalid env vars keep the historical defaults (both enabled).
	// Stored inverted: zero value = enabled (see Config field comment).
	cfg.CheckinDisabled = !parseBoolean(get("CHECKIN_ENABLED"), true)
	cfg.BalanceRefreshDisabled = !parseBoolean(get("BALANCE_REFRESH_ENABLED"), true)
	cfg.ModelSyncCron = firstNonEmpty(get("MODEL_SYNC_CRON"), DefaultModelSyncCron)
	cfg.LogCleanupCron = firstNonEmpty(get("LOG_CLEANUP_CRON"), DefaultLogCleanupCron)

	// ---- 3.4b Site & Branding ----
	cfg.SystemName = firstNonEmpty(get("SYSTEM_NAME"), DefaultSystemName)
	cfg.Logo = firstNonEmpty(get("LOGO"), DefaultLogo)
	cfg.Footer = firstNonEmpty(get("FOOTER"), DefaultFooter)
	cfg.About = firstNonEmpty(get("ABOUT"), DefaultAbout)
	cfg.ServerAddress = firstNonEmpty(get("SERVER_ADDRESS"), DefaultServerAddress)

	// ---- §3.5 Log Cleanup ----
	cfg.LogCleanupUsageLogsEnabled = parseBoolean(get("LOG_CLEANUP_USAGE_LOGS_ENABLED"), false)
	cfg.LogCleanupProgramLogsEnabled = parseBoolean(get("LOG_CLEANUP_PROGRAM_LOGS_ENABLED"), false)
	cfg.LogCleanupRetentionDays = maxInt(1, int(math.Trunc(parseNumber(get("LOG_CLEANUP_RETENTION_DAYS"), 30))))
	cfg.LogCleanupConfigured = false // set later during runtime settings hydration

	// ---- §3.6 Notify: Webhook ----
	cfg.WebhookUrl = firstNonEmpty(get("WEBHOOK_URL"), "")
	cfg.WebhookEnabled = parseBoolean(get("WEBHOOK_ENABLED"), true)

	// ---- §3.7 Notify: Bark ----
	cfg.BarkUrl = firstNonEmpty(get("BARK_URL"), "")
	cfg.BarkEnabled = parseBoolean(get("BARK_ENABLED"), true)

	// ---- §3.8 Notify: ServerChan ----
	cfg.ServerChanKey = firstNonEmpty(get("SERVERCHAN_KEY"), "")
	cfg.ServerChanEnabled = parseBoolean(get("SERVERCHAN_ENABLED"), true)

	// ---- §3.9 Notify: Telegram ----
	cfg.TelegramEnabled = parseBoolean(get("TELEGRAM_ENABLED"), false)
	cfg.TelegramApiBaseUrl = TelegramApiBaseUrl
	cfg.TelegramBotToken = firstNonEmpty(get("TELEGRAM_BOT_TOKEN"), "")
	cfg.TelegramChatId = firstNonEmpty(get("TELEGRAM_CHAT_ID"), "")
	cfg.TelegramUseSystemProxy = parseBoolean(get("TELEGRAM_USE_SYSTEM_PROXY"), false)
	cfg.TelegramMessageThreadId = strings.TrimSpace(get("TELEGRAM_MESSAGE_THREAD_ID"))

	// ---- §3.10 Notify: SMTP ----
	cfg.SmtpEnabled = parseBoolean(get("SMTP_ENABLED"), false)
	cfg.SmtpHost = firstNonEmpty(get("SMTP_HOST"), "")
	cfg.SmtpPort = atoiOr(get("SMTP_PORT"), DefaultSmtpPort)
	cfg.SmtpSecure = parseBoolean(get("SMTP_SECURE"), false)
	cfg.SmtpUser = firstNonEmpty(get("SMTP_USER"), "")
	cfg.SmtpPass = firstNonEmpty(get("SMTP_PASS"), "")
	cfg.SmtpFrom = firstNonEmpty(get("SMTP_FROM"), "")
	cfg.SmtpTo = firstNonEmpty(get("SMTP_TO"), "")

	// ---- §3.11 Notify: General ----
	cfg.NotifyCooldownSec = maxInt(0, int(math.Trunc(parseNumber(get("NOTIFY_COOLDOWN_SEC"), DefaultNotifyCooldownSec))))
	cfg.SystemProxyUrl = firstNonEmpty(get("SYSTEM_PROXY_URL"), "")

	// ---- §3.11b Resin sticky proxy pool ----
	cfg.ResinURL = firstNonEmpty(get("RESIN_URL"), "")
	cfg.ResinPlatformName = firstNonEmpty(get("RESIN_PLATFORM_NAME"), "")
	cfg.ResinEnabled = parseBoolean(get("RESIN_ENABLED"), false)

	// ---- §3.11c uTLS TLS fingerprint masking ----
	cfg.UTLSEnabled = parseBoolean(get("UTLS_ENABLED"), false)

	// ---- §3.11e LDOH monitor proxy base URL ----
	cfg.LDOHBaseURL = strings.TrimSpace(firstNonEmpty(get("LDOH_BASE_URL"), DefaultLDOHBaseURL))
	// LDOH_PROXY_TIMEOUT_SEC: per-request timeout for the LDOH upstream HTTP
	// client. 0/negative/invalid falls back to the default (30s) so a
	// misconfigured value never produces an unbounded-context LDOH call.
	ldohTimeoutParsed := int(math.Trunc(parseNumber(get("LDOH_PROXY_TIMEOUT_SEC"), float64(DefaultLDOHProxyTimeoutSec))))
	if ldohTimeoutParsed <= 0 {
		ldohTimeoutParsed = DefaultLDOHProxyTimeoutSec
	}
	cfg.LDOHProxyTimeoutSec = ldohTimeoutParsed

	// ---- §3.11f Outbound proxy / upstream HTTP timeouts (#1009) ----
	// Integer seconds; 0/negative/invalid falls back to the default so a
	// misconfigured value never disables a timeout. Defaults match the
	// pre-#1009 hardcoded values in platform/site_proxy.go.
	cfg.ProxyConnectTimeoutSec = parseTimeoutSec(get("PROXY_CONNECT_TIMEOUT_SEC"), DefaultProxyConnectTimeoutSec)
	cfg.ProxyTLSHandshakeTimeoutSec = parseTimeoutSec(get("PROXY_TLS_HANDSHAKE_TIMEOUT_SEC"), DefaultProxyTLSHandshakeTimeoutSec)
	cfg.ProxyResponseHeaderTimeoutSec = parseTimeoutSec(get("PROXY_RESPONSE_HEADER_TIMEOUT_SEC"), DefaultProxyResponseHeaderTimeoutSec)
	cfg.ProxyIdleConnTimeoutSec = parseTimeoutSec(get("PROXY_IDLE_CONN_TIMEOUT_SEC"), DefaultProxyIdleConnTimeoutSec)
	cfg.ProxyRequestTimeoutSec = parseTimeoutSec(get("PROXY_REQUEST_TIMEOUT_SEC"), DefaultProxyRequestTimeoutSec)
	cfg.ProxyStreamIdleTimeoutSec = parseTimeoutSec(get("PROXY_STREAM_IDLE_TIMEOUT_SEC"), DefaultProxyStreamIdleTimeoutSec)

	// ---- §3.11d Update Center scheduler gate ----
	cfg.UpdateCenterEnabled = parseBoolean(get("METAPI_ENABLE_UPDATE_CENTER"), false)

	// ---- §3.12 Notify: Feishu / DingTalk / WeCom / Ntfy ----
	cfg.FeishuEnabled = parseBoolean(get("FEISHU_ENABLED"), false)
	cfg.FeishuWebhook = firstNonEmpty(get("FEISHU_WEBHOOK"), "")
	cfg.FeishuSecret = firstNonEmpty(get("FEISHU_SECRET"), "")
	cfg.DingtalkEnabled = parseBoolean(get("DINGTALK_ENABLED"), false)
	cfg.DingtalkWebhook = firstNonEmpty(get("DINGTALK_WEBHOOK"), "")
	cfg.DingtalkSecret = firstNonEmpty(get("DINGTALK_SECRET"), "")
	cfg.WecomEnabled = parseBoolean(get("WECOM_ENABLED"), false)
	cfg.WecomWebhook = firstNonEmpty(get("WECOM_WEBHOOK"), "")
	cfg.NtfyEnabled = parseBoolean(get("NTFY_ENABLED"), false)
	cfg.NtfyUrl = firstNonEmpty(get("NTFY_URL"), "")
	cfg.NtfyTopic = firstNonEmpty(get("NTFY_TOPIC"), "")
	cfg.NtfyToken = firstNonEmpty(get("NTFY_TOKEN"), "")
	cfg.RedisURL = firstNonEmpty(get("REDIS_URL"), get("METAPI_REDIS_URL"))

	// ---- §3.12 Admin ----
	cfg.AdminIpAllowlist = parseCsvList(get("ADMIN_IP_ALLOWLIST"))
	cfg.AdminCorsAllowedOrigins = parseCsvList(get("ADMIN_CORS_ALLOWED_ORIGINS"))
	cfg.TrustedProxyCidrs = parseCsvList(get("TRUSTED_PROXY_CIDRS"))
	// Admin/OAuth per-IP token-bucket rate limits. Defaults preserve the
	// original hardcoded values (Admin 100/200, OAuth 10/20) so existing
	// deployments are byte-for-byte compatible without env changes.
	cfg.AdminRateLimitRPS = maxInt(0, int(math.Trunc(parseNumber(get("ADMIN_RATE_LIMIT_RPS"), float64(DefaultAdminRateLimitRPS)))))
	cfg.AdminRateLimitBurst = maxInt(0, int(math.Trunc(parseNumber(get("ADMIN_RATE_LIMIT_BURST"), float64(DefaultAdminRateLimitBurst)))))
	cfg.OAuthRateLimitRPS = maxInt(0, int(math.Trunc(parseNumber(get("OAUTH_RATE_LIMIT_RPS"), float64(DefaultOAuthRateLimitRPS)))))
	cfg.OAuthRateLimitBurst = maxInt(0, int(math.Trunc(parseNumber(get("OAUTH_RATE_LIMIT_BURST"), float64(DefaultOAuthRateLimitBurst)))))
	// /api/auth/* brute-force cap (#1034): login is the only surface that
	// accepts the master token, so it gets the strict bucket regardless of
	// the general admin bucket size.
	cfg.AuthRateLimitRPS = maxInt(0, int(math.Trunc(parseNumber(get("AUTH_RATE_LIMIT_RPS"), float64(DefaultAuthRateLimitRPS)))))
	cfg.AuthRateLimitBurst = maxInt(0, int(math.Trunc(parseNumber(get("AUTH_RATE_LIMIT_BURST"), float64(DefaultAuthRateLimitBurst)))))
	// Server-side admin session (#1034): sliding TTL + cookie Secure policy.
	// Zero-config defaults must stay safe (12h sliding session, Secure
	// auto-adapting to the request protocol).
	cfg.AdminSessionTTLMinutes = maxInt(1, int(math.Trunc(parseNumber(get("ADMIN_SESSION_TTL_MINUTES"), float64(DefaultAdminSessionTTLMinutes)))))
	cfg.AdminSessionCookieSecure = normalizeSessionCookieSecure(get("ADMIN_SESSION_COOKIE_SECURE"))

	// ---- §3.13 Proxy: Core ----
	// REQUEST_BODY_LIMIT_MB controls the general body limit (default 20 MB,
	// clamped to [1, 200]). FILE_UPLOAD_LIMIT_MB controls a separate higher
	// limit for multipart upload routes /v1/files and /v1/images/* (default
	// 100 MB, clamped to [1, 1000]).
	bodyLimitMB := ClampInt(int(math.Trunc(parseNumber(get("REQUEST_BODY_LIMIT_MB"), float64(DefaultRequestBodyLimitMB)))), 1, 200)
	cfg.RequestBodyLimit = bodyLimitMB * 1024 * 1024
	fileUploadLimitMB := ClampInt(int(math.Trunc(parseNumber(get("FILE_UPLOAD_LIMIT_MB"), float64(DefaultFileUploadLimitMB)))), 1, 1000)
	cfg.FileUploadLimitBytes = fileUploadLimitMB * 1024 * 1024

	// Per-IP rate limiting for /v1 proxy routes (default 60 RPM; 0 = disabled).
	// Negative values are NOT clamped here so config.Validate can warn the
	// operator — consumers (auth.ProxyRateLimit) already treat <=0 as disabled,
	// so the only observable effect of a negative is the startup warning.
	cfg.ProxyRateLimitRPM = int(math.Trunc(parseNumber(get("PROXY_RATE_LIMIT_RPM"), float64(DefaultProxyRateLimitRPM))))
	// Global PROXY_TOKEN RPM cap across all IPs (default 0 = unlimited).
	// Negative left intact for Validate to warn on; <=0 disables at the limiter.
	cfg.ProxyGlobalTokenRPM = int(math.Trunc(parseNumber(get("PROXY_GLOBAL_TOKEN_RPM"), 0)))

	cfg.RoutingFallbackUnitCost = math.Max(1e-6, parseNumber(get("ROUTING_FALLBACK_UNIT_COST"), 1))
	// Seconds; internal first-byte observation uses ms = sec * 1000.
	cfg.ProxyFirstByteTimeoutSec = maxInt(0, int(math.Trunc(parseNumber(get("PROXY_FIRST_BYTE_TIMEOUT_SEC"), 0))))

	// ---- §3.14 Proxy: Token Router ----
	tokenRouterParsed := parseNumber(get("TOKEN_ROUTER_FAILURE_COOLDOWN_MAX_SEC"), float64(TokenRouterFailureCooldownMaxSecCeiling))
	if val, ok := normalizeTokenRouterFailureCooldownMaxSec(tokenRouterParsed); ok {
		cfg.TokenRouterFailureCooldownMaxSec = val
	} else {
		cfg.TokenRouterFailureCooldownMaxSec = TokenRouterFailureCooldownMaxSecCeiling
	}
	cfg.TokenRouterCacheTtlMs = maxInt(100, int(math.Trunc(parseNumber(get("TOKEN_ROUTER_CACHE_TTL_MS"), DefaultTokenRouterCacheTtlMs))))

	// ---- §3.14b Pricing Catalog (models.dev official list prices) ----
	cfg.PricingCatalogEnabled = parseBoolean(get("PRICING_CATALOG_ENABLED"), DefaultPricingCatalogEnabled)
	cfg.PricingCatalogRefreshMin = maxInt(0, int(math.Trunc(parseNumber(get("PRICING_CATALOG_REFRESH_MIN"), float64(DefaultPricingCatalogRefreshMin)))))
	cfg.PricingCatalogURL = strings.TrimSpace(get("PRICING_CATALOG_URL"))
	if cfg.PricingCatalogURL == "" {
		cfg.PricingCatalogURL = DefaultPricingCatalogURL
	}

	// ---- §3.15 Proxy: Channel ----
	// Negative left intact so config.Validate can warn the operator; the
	// consumer (GetProxyMaxChannelAttempts) already clamps <=0 to 1, so the
	// only observable effect of a negative is the startup warning.
	cfg.ProxyMaxChannelAttempts = int(math.Trunc(parseNumber(get("PROXY_MAX_CHANNEL_ATTEMPTS"), DefaultProxyMaxChannelAttempts)))
	cfg.ProxyStickySessionEnabled = parseBoolean(get("PROXY_STICKY_SESSION_ENABLED"), true)
	cfg.ProxyStickySessionTtlMs = maxInt(30000, int(math.Trunc(parseNumber(get("PROXY_STICKY_SESSION_TTL_MS"), float64(DefaultProxyStickySessionTtlMs)))))

	// ---- §3.16 Proxy: Session ----
	cfg.ProxySessionChannelConcurrencyLimit = maxInt(0, int(math.Trunc(parseNumber(get("PROXY_SESSION_CHANNEL_CONCURRENCY_LIMIT"), DefaultProxySessionChannelConcurrencyLimit))))
	cfg.ProxySessionChannelQueueWaitMs = maxInt(0, int(math.Trunc(parseNumber(get("PROXY_SESSION_CHANNEL_QUEUE_WAIT_MS"), DefaultProxySessionChannelQueueWaitMs))))
	cfg.ProxySessionChannelLeaseTtlMs = maxInt(5000, int(math.Trunc(parseNumber(get("PROXY_SESSION_CHANNEL_LEASE_TTL_MS"), DefaultProxySessionChannelLeaseTtlMs))))
	cfg.ProxySessionChannelLeaseKeepaliveMs = maxInt(1000, int(math.Trunc(parseNumber(get("PROXY_SESSION_CHANNEL_LEASE_KEEPALIVE_MS"), DefaultProxySessionChannelLeaseKeepaliveMs))))

	// ---- §3.17 Proxy: Misc ----
	cfg.CodexUpstreamWebsocketEnabled = parseBoolean(get("CODEX_UPSTREAM_WEBSOCKET_ENABLED"), false)
	cfg.ResponsesCompactFallbackToResponsesEnabled = parseBoolean(get("RESPONSES_COMPACT_FALLBACK_TO_RESPONSES_ENABLED"), false)
	cfg.DisableCrossProtocolFallback = parseBoolean(get("DISABLE_CROSS_PROTOCOL_FALLBACK"), false)
	cfg.ProxyEmptyContentFailEnabled = parseBoolean(get("PROXY_EMPTY_CONTENT_FAIL"), false)
	// PROXY_MAX_STREAM_RESPONSE_BYTES caps a single SSE stream (default 1 MB).
	// 0/negative/invalid resolves to the default so the hot stream path never
	// has to re-parse env or guard against a zero limit. Read once here.
	streamBytesParsed := int(math.Trunc(parseNumber(get("PROXY_MAX_STREAM_RESPONSE_BYTES"), float64(DefaultProxyMaxStreamResponseBytes))))
	if streamBytesParsed <= 0 {
		streamBytesParsed = DefaultProxyMaxStreamResponseBytes
	}
	cfg.ProxyMaxStreamResponseBytes = streamBytesParsed
	// PROXY_MAX_BUFFERED_RESPONSE_BYTES caps a single non-streaming upstream
	// response (default 20 MB). 0/negative/invalid resolves to the default so
	// the buffered-response hot path never has to re-parse env or guard against
	// a zero limit. Read once here — upstream.go / upstream_stream.go /
	// executor.go read the resolved value from the config singleton.
	bufferedBytesParsed := int(math.Trunc(parseNumber(get("PROXY_MAX_BUFFERED_RESPONSE_BYTES"), float64(DefaultProxyMaxBufferedResponseBytes))))
	if bufferedBytesParsed <= 0 {
		bufferedBytesParsed = int(DefaultProxyMaxBufferedResponseBytes)
	}
	cfg.ProxyMaxBufferedResponseBytes = bufferedBytesParsed
	cfg.ProxyErrorKeywords = parseCsvList(get("PROXY_ERROR_KEYWORDS"))
	cfg.GlobalBlockedBrands = []string{}
	cfg.GlobalAllowedModels = []string{}

	// ---- §3.17b Prompt Filter (OAuth pool protection, #681) ----
	cfg.PromptFilterEnabled = parseBoolean(get("PROMPT_FILTER_ENABLED"), false)
	cfg.PromptFilterDenyPatterns = parseCsvList(get("PROMPT_FILTER_DENY_PATTERNS"))

	// ---- §3.18 Proxy: Debug ----
	cfg.ProxyDebugTraceEnabled = parseBoolean(get("PROXY_DEBUG_TRACE_ENABLED"), false)
	cfg.ProxyDebugCaptureHeaders = parseBoolean(get("PROXY_DEBUG_CAPTURE_HEADERS"), true)
	cfg.ProxyDebugCaptureBodies = parseBoolean(get("PROXY_DEBUG_CAPTURE_BODIES"), false)
	cfg.ProxyDebugCaptureStreamChunks = parseBoolean(get("PROXY_DEBUG_CAPTURE_STREAM_CHUNKS"), false)
	cfg.ProxyDebugTargetSessionId = strings.TrimSpace(get("PROXY_DEBUG_TARGET_SESSION_ID"))
	cfg.ProxyDebugTargetClientKind = strings.TrimSpace(get("PROXY_DEBUG_TARGET_CLIENT_KIND"))
	cfg.ProxyDebugTargetModel = strings.TrimSpace(get("PROXY_DEBUG_TARGET_MODEL"))
	cfg.ProxyDebugRetentionHours = maxInt(1, int(math.Trunc(parseNumber(get("PROXY_DEBUG_RETENTION_HOURS"), DefaultProxyDebugRetentionHours))))
	cfg.ProxyDebugMaxBodyBytes = maxInt(1024, int(math.Trunc(parseNumber(get("PROXY_DEBUG_MAX_BODY_BYTES"), DefaultProxyDebugMaxBodyBytes))))

	// ---- §3.19 Codex-specific ----
	cfg.CodexResponsesWebsocketBeta = firstNonEmpty(
		parseOptionalSecret(get("CODEX_RESPONSES_WEBSOCKET_BETA")),
		"responses_websockets=2026-02-06",
	)
	cfg.CodexHeaderDefaults = CodexHeaderDefaults{
		UserAgent:    parseOptionalSecret(get("CODEX_HEADER_DEFAULTS_USER_AGENT")),
		BetaFeatures: parseOptionalSecret(get("CODEX_HEADER_DEFAULTS_BETA_FEATURES")),
	}

	// ---- §3.20 Model Probe ----
	cfg.ModelAvailabilityProbeEnabled = parseBoolean(get("MODEL_AVAILABILITY_PROBE_ENABLED"), false)
	cfg.ModelAvailabilityProbeIntervalMs = maxInt(60000, int(math.Trunc(parseNumber(get("MODEL_AVAILABILITY_PROBE_INTERVAL_MS"), float64(DefaultModelAvailabilityProbeIntervalMs)))))
	cfg.ModelAvailabilityProbeTimeoutMs = maxInt(3000, int(math.Trunc(parseNumber(get("MODEL_AVAILABILITY_PROBE_TIMEOUT_MS"), DefaultModelAvailabilityProbeTimeoutMs))))
	cfg.ModelAvailabilityProbeConcurrency = ClampInt(
		int(math.Trunc(parseNumber(get("MODEL_AVAILABILITY_PROBE_CONCURRENCY"), DefaultModelAvailabilityProbeConcurrency))),
		1, 16,
	)

	cfg.RouteRebuildProbeFilterEnabled = parseBoolean(get("ROUTE_REBUILD_PROBE_FILTER_ENABLED"), false)
	cfg.RouteRebuildProbeFilterIncludeModels = parseCsvList(get("ROUTE_REBUILD_PROBE_FILTER_INCLUDE_MODELS"))
	cfg.RouteRebuildProbeFilterExcludeModels = parseCsvList(get("ROUTE_REBUILD_PROBE_FILTER_EXCLUDE_MODELS"))

	// ---- §3.21 Retention ----
	cfg.ProxyLogRetentionDays = maxInt(0, int(math.Trunc(parseNumber(get("PROXY_LOG_RETENTION_DAYS"), DefaultProxyLogRetentionDays))))
	cfg.ProxyLogRetentionPruneIntervalMinutes = maxInt(1, int(math.Trunc(parseNumber(get("PROXY_LOG_RETENTION_PRUNE_INTERVAL_MINUTES"), float64(DefaultProxyLogRetentionPruneIntervalMinutes)))))
	cfg.ProxyFileRetentionDays = maxInt(0, int(math.Trunc(parseNumber(get("PROXY_FILE_RETENTION_DAYS"), DefaultProxyFileRetentionDays))))
	cfg.ProxyFileRetentionPruneIntervalMinutes = maxInt(1, int(math.Trunc(parseNumber(get("PROXY_FILE_RETENTION_PRUNE_INTERVAL_MINUTES"), float64(DefaultProxyFileRetentionPruneIntervalMinutes)))))
	cfg.ProxyVideoTaskRetentionDays = maxInt(0, int(math.Trunc(parseNumber(get("PROXY_VIDEO_TASK_RETENTION_DAYS"), float64(DefaultProxyVideoTaskRetentionDays)))))
	cfg.ProxyVideoTaskRetentionPruneIntervalMinutes = maxInt(1, int(math.Trunc(parseNumber(get("PROXY_VIDEO_TASK_RETENTION_PRUNE_INTERVAL_MINUTES"), float64(DefaultProxyVideoTaskRetentionPruneIntervalMinutes)))))

	// ---- §3.21b Proxy Log Batch Writer ----
	// Default async=true so production gets the latency win automatically; the
	// clamp guards against malformed operator input. Batch size/interval floors
	// of 1 keep the writer functional even under tiny operator-chosen values.
	cfg.ProxyLogAsync = parseBoolean(get("PROXY_LOG_ASYNC"), DefaultProxyLogAsync)
	cfg.ProxyLogBatchSize = ClampInt(
		int(math.Trunc(parseNumber(get("PROXY_LOG_BATCH_SIZE"), float64(DefaultProxyLogBatchSize)))),
		1, 1000,
	)
	cfg.ProxyLogFlushIntervalMs = ClampInt(
		int(math.Trunc(parseNumber(get("PROXY_LOG_FLUSH_INTERVAL_MS"), float64(DefaultProxyLogFlushIntervalMs)))),
		1, 60000,
	)

	// ---- §3.22 Routing Weights ----
	cfg.RoutingWeights = RoutingWeights{
		BaseWeightFactor: parseNumber(get("BASE_WEIGHT_FACTOR"), 0.5),
		ValueScoreFactor: parseNumber(get("VALUE_SCORE_FACTOR"), 0.5),
		CostWeight:       parseNumber(get("COST_WEIGHT"), 0.4),
		BalanceWeight:    parseNumber(get("BALANCE_WEIGHT"), 0.3),
		UsageWeight:      parseNumber(get("USAGE_WEIGHT"), 0.3),
	}

	// ---- §3.23 Payload Rules + Service Tier ----
	cfg.PayloadRules = parseJsonValue(resolvePayloadRules())
	cfg.OpenAiServiceTierRules = parseJsonValue(resolveOpenAiServiceTierRules())

	return cfg
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// firstNonEmpty returns the first non-empty argument, or "" if all are empty.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// normalizeLogLevel canonicalizes a LOG_LEVEL env value to one of
// debug|info|warn|error. Unknown/empty input falls back to DefaultLogLevel
// ("info") so an invalid operator value can never silence Warn/Error output.
func normalizeLogLevel(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return "debug"
	case "warn", "warning":
		return "warn"
	case "error":
		return "error"
	case "info", "":
		return "info"
	default:
		return "info"
	}
}

// SlogLevel maps a canonical log-level string to the matching slog.Level.
// Callers (e.g. cmd/server/main.go) use this at startup to configure the
// default slog handler threshold from config.LogLevel. Unknown values fall
// back to LevelInfo, matching normalizeLogLevel's "info" default.
func SlogLevel(logLevel string) slog.Level {
	switch normalizeLogLevel(logLevel) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// normalizeDbProfile maps env aliases to shared-tiny|normal|dedicated.
func normalizeDbProfile(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "shared-tiny", "shared_tiny", "tiny", "lite", "small":
		return "shared-tiny"
	case "dedicated", "large", "big":
		return "dedicated"
	case "normal", "default", "medium", "":
		return "normal"
	default:
		return "normal"
	}
}

// dbPoolDefaultsForProfile returns (maxOpen, maxIdle) for a normalized profile.
func dbPoolDefaultsForProfile(profile string) (int, int) {
	switch normalizeDbProfile(profile) {
	case "shared-tiny":
		return DefaultDbMaxOpenConnsSharedTiny, DefaultDbMaxIdleConnsSharedTiny
	case "dedicated":
		return DefaultDbMaxOpenConnsDedicated, DefaultDbMaxIdleConnsDedicated
	default:
		return DefaultDbMaxOpenConnsNormal, DefaultDbMaxIdleConnsNormal
	}
}

// ClampInt clamps v to [lo, hi].
func ClampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// maxInt returns the larger of a and b.
// normalizeSessionCookieSecure maps ADMIN_SESSION_COOKIE_SECURE to one of
// "auto", "true" or "false". Unset/unrecognized values fall back to the
// zero-config safe default "auto" (Secure follows the request protocol).
func normalizeSessionCookieSecure(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "yes":
		return "true"
	case "false", "0", "no":
		return "false"
	default:
		return DefaultAdminSessionCookieSecure
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// MaxInt returns the larger of a and b. Exported for use by other packages.
func MaxInt(a, b int) int {
	return maxInt(a, b)
}

// MaxInt64 returns the larger of a and b. Exported for use by other packages.
func MaxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// ParseJsonValue parses a JSON string into an any value.
// Returns nil on empty input or parse error.
func ParseJsonValue(value string) any {
	if value == "" {
		return nil
	}
	var result any
	if err := json.Unmarshal([]byte(value), &result); err != nil {
		return nil
	}
	return result
}

// atoiOr parses s as int, returning fallback on failure.
func atoiOr(s string, fallback int) int {
	if s == "" {
		return fallback
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return v
}
