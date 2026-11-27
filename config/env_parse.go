package config

import (
	"encoding/json"
	"log/slog"
	"math"
	"runtime"
	"strconv"
	"strings"
)

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

// Integer-valued settings accept fractional input and truncate toward zero.
func parseTruncatedInt(value string, fallback int) int {
	return int(math.Trunc(parseNumber(value, float64(fallback))))
}

// Timeouts and response size limits require a positive integer.
func parsePositiveInt(value string, def int) int {
	parsed := parseTruncatedInt(value, def)
	if parsed <= 0 {
		return def
	}
	return parsed
}

// parseConcurrencyLimit parses an optional concurrency cap where 0 is a
// meaningful value (unlimited) rather than a typo to clamp. Unset, invalid or
// negative values fall back to def so the documented default keeps working.
// Fractional values truncate, but a positive value below 1 becomes 1 rather
// than silently turning the cap into "unlimited".
func parseConcurrencyLimit(value string, def int) int {
	raw := parseNumber(value, float64(def))
	if raw < 0 {
		return def
	}
	if raw > 0 && raw < 1 {
		return 1
	}
	return int(math.Trunc(raw))
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

// §1.6 parseDbType: "" → "sqlite"; trim+lower → "mysql"/"postgres"/"postgresql" → "postgres"; else "sqlite".
func parseDbType(value string) string {
	if value == "" {
		return DefaultDbType
	}
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "mysql":
		return "mysql"
	case "postgres", "postgresql":
		return "postgres"
	default:
		return DefaultDbType
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
	return DefaultDbType
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
func (c *Config) PostgresSSLMode(dbSsl bool) string {
	if mode := normalizeDbSslMode(c.DbSslMode); mode != "" {
		return mode
	}
	if dbSsl {
		return "require"
	}
	return ""
}

// §1.7 NormalizeTokenRouterFailureCooldownMaxSec:
// !finite || <= 0 → (0, false); trunc → clamp[1, ceiling] → (int, true).
// Exported so the settings-table hydration path (store.ApplyRuntimeSettings)
// clamps a persisted cap exactly like the env path instead of growing a second
// copy of the rule.
func NormalizeTokenRouterFailureCooldownMaxSec(value float64) (int, bool) {
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

// MaxInt returns the larger of a and b. Exported for use by other packages.
func MaxInt(a, b int) int {
	return max(a, b)
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
