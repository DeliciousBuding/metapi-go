package config

import (
	"fmt"
	"net/netip"
	"strings"
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
			msg:      "negative value imports the legacy key without an RPM limit — use 0 for unlimited",
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
	// Unconditional now that PROXY_STICKY_SESSION_ENABLED is gone: the Codex WS
	// runtime always applies ProxyStickySessionTtlMs, so a non-positive TTL was
	// never "disabled", it was just clamped away by Load.
	if c.ProxyStickySessionTtlMs <= 0 {
		errs = append(errs, &configError{
			field:    "proxy_sticky_session_ttl_ms",
			value:    fmt.Sprintf("%d", c.ProxyStickySessionTtlMs),
			msg:      "should be > 0",
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
