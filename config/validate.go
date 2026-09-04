package config

import (
	"fmt"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"github.com/robfig/cron/v3"
)

// Validate checks Config fields and returns all validation errors.
// Callers should treat these as a single report — log warnings for
// non-fatal issues and exit on critical ones before binding the port.
func (c *Config) Validate() []error {
	var errs []error

	// --- Critical: Port must be in [1, 65535] ---
	if c.Port < 1 || c.Port > 65535 {
		errs = append(errs, &configError{
			field:    "port",
			value:    fmt.Sprintf("%d", c.Port),
			msg:      "must be in [1, 65535]",
			critical: true,
		})
	}

	if !validDbSslMode(c.DbSslMode) {
		errs = append(errs, &configError{
			field:    "db_sslmode",
			value:    c.DbSslMode,
			msg:      "must be one of disable, allow, prefer, require, verify-ca, or verify-full",
			critical: true,
		})
	}
	if c.DbMaxOpenConns < 1 {
		errs = append(errs, &configError{
			field:    "db_max_open_conns",
			value:    fmt.Sprintf("%d", c.DbMaxOpenConns),
			msg:      "must be >= 1",
			critical: true,
		})
	}
	if c.DbMaxIdleConns < 0 || c.DbMaxIdleConns > c.DbMaxOpenConns {
		errs = append(errs, &configError{
			field:    "db_max_idle_conns",
			value:    fmt.Sprintf("%d", c.DbMaxIdleConns),
			msg:      "must be between 0 and db_max_open_conns",
			critical: true,
		})
	}
	if c.DbConnMaxLifetimeSec < 0 {
		errs = append(errs, &configError{
			field:    "db_conn_max_lifetime_sec",
			value:    fmt.Sprintf("%d", c.DbConnMaxLifetimeSec),
			msg:      "must be >= 0",
			critical: true,
		})
	}
	if c.DbConnMaxIdleTimeSec < 0 {
		errs = append(errs, &configError{
			field:    "db_conn_max_idle_time_sec",
			value:    fmt.Sprintf("%d", c.DbConnMaxIdleTimeSec),
			msg:      "must be >= 0",
			critical: true,
		})
	}

	// --- Critical: Admin CORS origins must be exact http(s) origins ---
	for _, origin := range c.AdminCorsAllowedOrigins {
		if !validateAdminCorsOrigin(origin) {
			errs = append(errs, &configError{
				field:    "admin_cors_allowed_origins",
				value:    origin,
				msg:      "must be exact http(s) origins without wildcards, paths, query strings, or fragments",
				critical: true,
			})
		}
	}

	// --- Critical: Trusted proxy CIDRs must be parseable IP prefixes ---
	for _, cidr := range c.TrustedProxyCidrs {
		if _, err := netip.ParsePrefix(strings.TrimSpace(cidr)); err != nil {
			errs = append(errs, &configError{
				field:    "trusted_proxy_cidrs",
				value:    cidr,
				msg:      "must be valid IP CIDR prefixes",
				critical: true,
			})
		}
	}

	// --- Warning: account_credential_secret fallback ---
	if c.AccountCredentialSecret == "" {
		errs = append(errs, &configError{
			field:    "account_credential_secret",
			value:    "(empty)",
			msg:      "UNSAFE: account credential encryption key not set — stored credentials are NOT encrypted",
			critical: false,
		})
	} else {
		// Length-based strength validation. Load() resolves an unset
		// ACCOUNT_CREDENTIAL_SECRET to AUTH_TOKEN, then to the default admin
		// token, so the value here is the final resolved secret. A short key
		// silently passes the empty check above, so enforce explicit floors:
		//   < 8 bytes  → critical (trivially brute-forceable)
		//   < 16 bytes → warning  (weak; .env.example recommends 32+ bytes)
		//
		// Length is not the whole story. The literal .env.example ships is 33
		// bytes, so it clears both floors and reads as a strong secret while
		// being public in this repository: buildCredentialKey derives the AES
		// key from SHA-256 of exactly this string, so anyone holding the
		// database can decrypt every stored upstream credential without
		// touching the server. Guarded separately, and matched against the
		// shared constant rather than a retyped literal.
		secretLen := len(c.AccountCredentialSecret)
		switch {
		case c.AccountCredentialSecret == EnvExampleAccountCredentialSecret:
			// A warning, not a critical error, on purpose — both escalations
			// were tried against the real code and each is its own trap:
			//   critical → hasCritical makes cmd/server exit(1), so a
			//     deployment already running on the placeholder refuses to
			//     boot after an upgrade;
			//   rotate   → nothing re-encrypts existing rows, so
			//     DecryptAccountPassword returns "" for every stored password
			//     and those accounts must be re-bound.
			// So the operator gets the fact plus the consequence, and chooses.
			errs = append(errs, &configError{
				field:    "account_credential_secret",
				value:    "(placeholder)",
				msg:      "UNSAFE: ACCOUNT_CREDENTIAL_SECRET is still the literal shipped in .env.example, which is public in this repository — anyone holding the database can derive the key and decrypt every stored upstream credential. Not a boot-stopping error on purpose: rotating the secret does not re-encrypt existing rows, so stored passwords stop decrypting and those accounts must be re-bound. Set a 32+ byte random secret before storing credentials; on an existing deployment, rotate and then re-bind the affected accounts.",
				critical: false,
			})
		case secretLen < 8:
			errs = append(errs, &configError{
				field:    "account_credential_secret",
				value:    fmt.Sprintf("%d bytes", secretLen),
				msg:      "UNSAFE: secret is shorter than 8 bytes — trivially brute-forceable; set ACCOUNT_CREDENTIAL_SECRET to a 32+ byte random secret (an unset secret falls back to AUTH_TOKEN)",
				critical: true,
			})
		case secretLen < 16:
			errs = append(errs, &configError{
				field:    "account_credential_secret",
				value:    fmt.Sprintf("%d bytes", secretLen),
				msg:      "weak: secret is shorter than 16 bytes — use a 32+ byte random ACCOUNT_CREDENTIAL_SECRET for production",
				critical: false,
			})
		}
	}

	// --- Warning: OAuth client IDs are placeholders ---
	if c.ClaudeClientId == "" || c.ClaudeClientId == DefaultClaudeClientId {
		errs = append(errs, &configError{
			field:    "claude_client_id",
			value:    "(placeholder)",
			msg:      "Claude OAuth login will fail — set CLAUDE_CLIENT_ID",
			critical: false,
		})
	}
	if c.CodexClientId == "" || c.CodexClientId == DefaultCodexClientId {
		errs = append(errs, &configError{
			field:    "codex_client_id",
			value:    "(placeholder)",
			msg:      "Codex OAuth login will fail — set CODEX_CLIENT_ID",
			critical: false,
		})
	}
	if c.GeminiCliClientId == "" || c.GeminiCliClientId == DefaultGeminiCliClientId {
		errs = append(errs, &configError{
			field:    "gemini_cli_client_id",
			value:    "(placeholder)",
			msg:      "Gemini CLI OAuth login will fail — set GEMINI_CLI_CLIENT_ID",
			critical: false,
		})
	}

	// --- Critical/Warning: Lease keepalive vs TTL sanity ---
	// keepalive > TTL is a genuine correctness bug: the lease expires before
	// the renewal heartbeat can land, so sessions silently drop. keepalive ==
	// TTL is borderline (no renewal headroom) and only warrants a warning.
	if c.ProxySessionChannelLeaseKeepaliveMs > c.ProxySessionChannelLeaseTtlMs {
		errs = append(errs, &configError{
			field:    "proxy_session_channel_lease_keepalive_ms",
			value:    fmt.Sprintf("%d > %d (ttl)", c.ProxySessionChannelLeaseKeepaliveMs, c.ProxySessionChannelLeaseTtlMs),
			msg:      "keepalive interval must be < lease TTL — leases expire before renewal",
			critical: true,
		})
	} else if c.ProxySessionChannelLeaseKeepaliveMs == c.ProxySessionChannelLeaseTtlMs && c.ProxySessionChannelLeaseTtlMs > 0 {
		errs = append(errs, &configError{
			field:    "proxy_session_channel_lease_keepalive_ms",
			value:    fmt.Sprintf("%d == %d (ttl)", c.ProxySessionChannelLeaseKeepaliveMs, c.ProxySessionChannelLeaseTtlMs),
			msg:      "keepalive equals TTL — set keepalive strictly less than TTL for renewal headroom",
			critical: false,
		})
	}

	// --- Warning: negative values silently disable limits ---
	// Consumers treat <=0 as disabled, so a negative configures "disabled"
	// without the operator realizing it. Recommend 0 for explicit disable.
	if c.ProxyRateLimitRPM < 0 {
		errs = append(errs, &configError{
			field:    "proxy_rate_limit_rpm",
			value:    fmt.Sprintf("%d", c.ProxyRateLimitRPM),
			msg:      "negative value silently disables the per-IP limiter — use 0 for explicit disable",
			critical: false,
		})
	}
	if c.ProxyGlobalTokenRPM < 0 {
		errs = append(errs, &configError{
			field:    "proxy_global_token_rpm",
			value:    fmt.Sprintf("%d", c.ProxyGlobalTokenRPM),
			msg:      "negative value silently disables the global token limiter — use 0 for explicit disable",
			critical: false,
		})
	}
	if c.ProxyMaxChannelAttempts < 0 {
		errs = append(errs, &configError{
			field:    "proxy_max_channel_attempts",
			value:    fmt.Sprintf("%d", c.ProxyMaxChannelAttempts),
			msg:      "negative value silently clamps attempts to 1 — use 0 to disable retries",
			critical: false,
		})
	}

	// --- Warning: range checks for proxy session / router knobs ---
	if c.ProxyStickySessionEnabled && c.ProxyStickySessionTtlMs <= 0 {
		errs = append(errs, &configError{
			field:    "proxy_sticky_session_ttl_ms",
			value:    fmt.Sprintf("%d", c.ProxyStickySessionTtlMs),
			msg:      "should be > 0 when sticky sessions are enabled",
			critical: false,
		})
	}
	if c.TokenRouterCacheTtlMs <= 0 {
		errs = append(errs, &configError{
			field:    "token_router_cache_ttl_ms",
			value:    fmt.Sprintf("%d", c.TokenRouterCacheTtlMs),
			msg:      "should be > 0 — a non-positive cache TTL disables token-router caching",
			critical: false,
		})
	}
	// --- Warning: resin URL must be well-formed http(s) ---
	if err := validateUrl("resin_url", c.ResinURL, true); err != nil {
		errs = append(errs, err)
	}
	// --- Warning: proxy / redis URLs must parse (scheme left open) ---
	for _, u := range []struct{ field, val string }{
		{"redis_url", c.RedisURL},
	} {
		if err := validateUrl(u.field, u.val, false); err != nil {
			errs = append(errs, err)
		}
	}

	return errs
}

// configError implements the error interface and carries metadata for
// callers that need to distinguish critical from non-fatal issues.
type configError struct {
	field    string
	value    string
	msg      string
	critical bool
}

func (e *configError) Error() string {
	severity := "warning"
	if e.critical {
		severity = "critical"
	}
	return fmt.Sprintf("config %s: %s=%s — %s", severity, e.field, e.value, e.msg)
}

// IsCritical returns true if the error represents a fatal config issue.
func IsCritical(err error) bool {
	if ce, ok := err.(*configError); ok {
		return ce.critical
	}
	return false
}

// IsWarning returns true if the error represents a non-fatal config warning.
// It is the complement of IsCritical for config errors produced by Validate.
func IsWarning(err error) bool {
	if ce, ok := err.(*configError); ok {
		return !ce.critical
	}
	return false
}

// validateUrl parses a URL string and returns a warning configError when the
// value is malformed. For webhook-style URLs (requireWebScheme=true) it also
// warns when the scheme is not http/https; otherwise it warns when the URL is
// missing a scheme or host. Empty values are treated as unset (no error).
func validateUrl(field, value string, requireWebScheme bool) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return &configError{
			field:    field,
			value:    value,
			msg:      "malformed URL — " + err.Error(),
			critical: false,
		}
	}
	if requireWebScheme {
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return &configError{
				field:    field,
				value:    value,
				msg:      "scheme must be http or https",
				critical: false,
			}
		}
	} else if parsed.Scheme == "" {
		return &configError{
			field:    field,
			value:    value,
			msg:      "URL is missing a scheme",
			critical: false,
		}
	}
	if parsed.Host == "" {
		return &configError{
			field:    field,
			value:    value,
			msg:      "URL is missing a host",
			critical: false,
		}
	}
	return nil
}

// validateHhMm validates a 24h HH:mm window bound (2 digits : 2 digits with
// hours 00-23 and minutes 00-59). Empty values are treated as unset.
func validateHhMm(field, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if len(value) != 5 || value[2] != ':' {
		return &configError{
			field:    field,
			value:    value,
			msg:      "must be HH:mm (2 digits : 2 digits)",
			critical: false,
		}
	}
	hh, errH := strconv.Atoi(value[:2])
	mm, errM := strconv.Atoi(value[3:])
	if errH != nil || errM != nil || hh < 0 || hh > 23 || mm < 0 || mm > 59 {
		return &configError{
			field:    field,
			value:    value,
			msg:      "must be HH:mm with hours 00-23 and minutes 00-59",
			critical: false,
		}
	}
	return nil
}

// ValidateCronExpr checks if a cron expression is parseable. Auto-normalizes
// 5-field expressions (minute hour dom month dow) to 6-field (second...)
// for compatibility with cron.WithSeconds(), matching the scheduler behavior.
// This is the canonical cron validation used by both config.Validate and
// scheduler.ValidateCronExpr.
func ValidateCronExpr(expr string) bool {
	if strings.TrimSpace(expr) == "" {
		return false
	}
	fields := strings.Fields(expr)
	if len(fields) == 5 {
		expr = "0 " + expr
	}
	parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	_, err := parser.Parse(expr)
	return err == nil
}

func validateAdminCorsOrigin(origin string) bool {
	origin = strings.TrimSpace(origin)
	if origin == "" || strings.Contains(origin, "*") {
		return false
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	if parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	return parsed.Path == "" || parsed.Path == "/"
}

// Validate checks the runtime-mutable settings and returns all validation
// errors. Boot-time call path mirrors Config.Validate: warnings for
// non-fatal issues, exit on critical ones before binding the port.
func (r *RuntimeSettings) Validate() []error {
	var errs []error

	// --- Critical: CheckinScheduleMode must be "cron" or "interval" ---
	mode := strings.TrimSpace(strings.ToLower(r.CheckinScheduleMode))
	if mode != "cron" && mode != "interval" && mode != "window" {
		errs = append(errs, &configError{
			field:    "checkin_schedule_mode",
			value:    r.CheckinScheduleMode,
			msg:      "must be 'cron', 'interval', or 'window'",
			critical: true,
		})
	}

	// --- Critical: DBType must be "sqlite" or "postgres" ---
	dbType := strings.TrimSpace(strings.ToLower(r.DbType))
	if dbType != "sqlite" && dbType != "postgres" {
		errs = append(errs, &configError{
			field:    "db_type",
			value:    r.DbType,
			msg:      "must be 'sqlite' or 'postgres'",
			critical: true,
		})
	}
	// --- Warning: Cron expressions must be parseable ---
	if !ValidateCronExpr(r.CheckinCron) {
		errs = append(errs, &configError{
			field:    "checkin_cron",
			value:    r.CheckinCron,
			msg:      "invalid cron expression",
			critical: false,
		})
	}
	if !ValidateCronExpr(r.BalanceRefreshCron) {
		errs = append(errs, &configError{
			field:    "balance_refresh_cron",
			value:    r.BalanceRefreshCron,
			msg:      "invalid cron expression",
			critical: false,
		})
	}
	if !ValidateCronExpr(r.ModelSyncCron) {
		errs = append(errs, &configError{
			field:    "model_sync_cron",
			value:    r.ModelSyncCron,
			msg:      "invalid cron expression",
			critical: false,
		})
	}
	if !ValidateCronExpr(r.LogCleanupCron) {
		errs = append(errs, &configError{
			field:    "log_cleanup_cron",
			value:    r.LogCleanupCron,
			msg:      "invalid cron expression",
			critical: false,
		})
	}

	// --- Warning: NotifyCooldownSec >= 0 ---
	if r.NotifyCooldownSec < 0 {
		errs = append(errs, &configError{
			field:    "notify_cooldown_sec",
			value:    fmt.Sprintf("%d", r.NotifyCooldownSec),
			msg:      "must be >= 0",
			critical: false,
		})
	}

	// --- Warning: ProxyFirstByteTimeoutSec >= 0 ---
	if r.ProxyFirstByteTimeoutSec < 0 {
		errs = append(errs, &configError{
			field:    "proxy_first_byte_timeout_sec",
			value:    fmt.Sprintf("%d", r.ProxyFirstByteTimeoutSec),
			msg:      "must be >= 0",
			critical: false,
		})
	}

	// --- Warning: TokenRouterFailureCooldownMaxSec >= 0 ---
	if r.TokenRouterFailureCooldownMaxSec < 0 {
		errs = append(errs, &configError{
			field:    "token_router_failure_cooldown_max_sec",
			value:    fmt.Sprintf("%d", r.TokenRouterFailureCooldownMaxSec),
			msg:      "must be >= 0",
			critical: false,
		})
	}

	// --- Warning: CheckinIntervalHours in [1, 24] ---
	if r.CheckinIntervalHours < 1 || r.CheckinIntervalHours > 24 {
		errs = append(errs, &configError{
			field:    "checkin_interval_hours",
			value:    fmt.Sprintf("%d", r.CheckinIntervalHours),
			msg:      "must be in [1, 24]",
			critical: false,
		})
	}

	// --- Warning: RoutingWeights all >= 0 ---
	rw := r.RoutingWeights
	if rw.BaseWeightFactor < 0 {
		errs = append(errs, &configError{
			field:    "base_weight_factor",
			value:    fmt.Sprintf("%f", rw.BaseWeightFactor),
			msg:      "must be >= 0",
			critical: false,
		})
	}
	if rw.ValueScoreFactor < 0 {
		errs = append(errs, &configError{
			field:    "value_score_factor",
			value:    fmt.Sprintf("%f", rw.ValueScoreFactor),
			msg:      "must be >= 0",
			critical: false,
		})
	}
	if rw.CostWeight < 0 {
		errs = append(errs, &configError{
			field:    "cost_weight",
			value:    fmt.Sprintf("%f", rw.CostWeight),
			msg:      "must be >= 0",
			critical: false,
		})
	}
	if rw.BalanceWeight < 0 {
		errs = append(errs, &configError{
			field:    "balance_weight",
			value:    fmt.Sprintf("%f", rw.BalanceWeight),
			msg:      "must be >= 0",
			critical: false,
		})
	}
	if rw.UsageWeight < 0 {
		errs = append(errs, &configError{
			field:    "usage_weight",
			value:    fmt.Sprintf("%f", rw.UsageWeight),
			msg:      "must be >= 0",
			critical: false,
		})
	}

	// --- Critical: Default AUTH_TOKEN / PROXY_TOKEN ---
	if r.AuthToken == DefaultAuthToken {
		errs = append(errs, &configError{
			field:    "auth_token",
			value:    "(default)",
			msg:      "UNSAFE: using default admin token — set AUTH_TOKEN",
			critical: true,
		})
	}
	if r.ProxyToken == DefaultProxyToken {
		errs = append(errs, &configError{
			field:    "proxy_token",
			value:    "(default)",
			msg:      "UNSAFE: using default proxy token — set PROXY_TOKEN",
			critical: true,
		})
	}

	if r.ProxySessionChannelConcurrencyLimit < 1 {
		errs = append(errs, &configError{
			field:    "proxy_session_channel_concurrency_limit",
			value:    fmt.Sprintf("%d", r.ProxySessionChannelConcurrencyLimit),
			msg:      "should be >= 1 — 0 disables per-channel concurrency control",
			critical: false,
		})
	}
	if r.ProxySessionChannelQueueWaitMs < 0 {
		errs = append(errs, &configError{
			field:    "proxy_session_channel_queue_wait_ms",
			value:    fmt.Sprintf("%d", r.ProxySessionChannelQueueWaitMs),
			msg:      "should be >= 0 — negative values are treated as 0 (no queue wait)",
			critical: false,
		})
	}

	// --- Warning: notify webhook URLs must be well-formed http(s) ---
	webhookUrls := []struct{ field, val string }{
		{"webhook_url", r.WebhookUrl},
		{"bark_url", r.BarkUrl},
		{"feishu_webhook", r.FeishuWebhook},
		{"dingtalk_webhook", r.DingtalkWebhook},
		{"wecom_webhook", r.WecomWebhook},
		{"ntfy_url", r.NtfyUrl},
		{"telegram_api_base_url", r.TelegramApiBaseUrl},
	}
	for _, u := range webhookUrls {
		if err := validateUrl(u.field, u.val, true); err != nil {
			errs = append(errs, err)
		}
	}
	// --- Warning: system proxy URL must parse (scheme left open) ---
	if err := validateUrl("system_proxy_url", r.SystemProxyUrl, false); err != nil {
		errs = append(errs, err)
	}

	// --- Warning: checkin window bounds must be HH:mm ---
	if err := validateHhMm("checkin_window_start", r.CheckinWindowStart); err != nil {
		errs = append(errs, err)
	}
	if err := validateHhMm("checkin_window_end", r.CheckinWindowEnd); err != nil {
		errs = append(errs, err)
	}

	return errs
}
