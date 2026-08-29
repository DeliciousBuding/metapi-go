package config

import (
	"log/slog"
	"strings"
	"testing"
)

func TestLoadParsesAdminCorsAllowedOrigins(t *testing.T) {
	cfg, _ := Load(map[string]string{
		"ADMIN_CORS_ALLOWED_ORIGINS": " https://admin.example.com,https://ops.example.com ,, ",
	})

	want := []string{"https://admin.example.com", "https://ops.example.com"}
	if len(cfg.AdminCorsAllowedOrigins) != len(want) {
		t.Fatalf("AdminCorsAllowedOrigins length = %d, want %d: %#v", len(cfg.AdminCorsAllowedOrigins), len(want), cfg.AdminCorsAllowedOrigins)
	}
	for i := range want {
		if cfg.AdminCorsAllowedOrigins[i] != want[i] {
			t.Fatalf("AdminCorsAllowedOrigins[%d] = %q, want %q", i, cfg.AdminCorsAllowedOrigins[i], want[i])
		}
	}
}

func TestParseListenHostDefaultsByOS(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		goos string
		want string
	}{
		{name: "windows missing", env: map[string]string{}, goos: "windows", want: "127.0.0.1"},
		{name: "windows blank", env: map[string]string{"HOST": "  "}, goos: "windows", want: "127.0.0.1"},
		{name: "linux missing", env: map[string]string{}, goos: "linux", want: "0.0.0.0"},
		{name: "darwin blank", env: map[string]string{"HOST": "\t"}, goos: "darwin", want: "0.0.0.0"},
		{name: "explicit override", env: map[string]string{"HOST": " 192.0.2.10 "}, goos: "windows", want: "192.0.2.10"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseListenHostForOS(tt.env, tt.goos); got != tt.want {
				t.Fatalf("parseListenHostForOS(%v, %q) = %q, want %q", tt.env, tt.goos, got, tt.want)
			}
		})
	}
}

func TestLoadParsesTrustedProxyCidrs(t *testing.T) {
	cfg, _ := Load(map[string]string{
		"TRUSTED_PROXY_CIDRS": " 127.0.0.1/32,10.0.0.0/8 ,, ",
	})

	want := []string{"127.0.0.1/32", "10.0.0.0/8"}
	if len(cfg.TrustedProxyCidrs) != len(want) {
		t.Fatalf("TrustedProxyCidrs length = %d, want %d: %#v", len(cfg.TrustedProxyCidrs), len(want), cfg.TrustedProxyCidrs)
	}
	for i := range want {
		if cfg.TrustedProxyCidrs[i] != want[i] {
			t.Fatalf("TrustedProxyCidrs[%d] = %q, want %q", i, cfg.TrustedProxyCidrs[i], want[i])
		}
	}
}

func TestLoadInfersPostgresFromDatabaseURLAlias(t *testing.T) {
	_, rt := Load(map[string]string{
		"DATABASE_URL": "postgres://db.example.com:5432/metapi?sslmode=require",
	})

	if rt.DbType != "postgres" {
		t.Fatalf("DbType = %q, want postgres", rt.DbType)
	}
	if rt.DbUrl != "postgres://db.example.com:5432/metapi?sslmode=require" {
		t.Fatalf("DbUrl = %q, want DATABASE_URL value", rt.DbUrl)
	}
}

func TestLoadParsesResinConfig(t *testing.T) {
	cfg, _ := Load(map[string]string{
		"RESIN_URL":           "http://resin.local:2260/my-token",
		"RESIN_PLATFORM_NAME": "metapi",
		"RESIN_ENABLED":       "true",
	})

	if cfg.ResinURL != "http://resin.local:2260/my-token" {
		t.Fatalf("ResinURL = %q, want http://resin.local:2260/my-token", cfg.ResinURL)
	}
	if cfg.ResinPlatformName != "metapi" {
		t.Fatalf("ResinPlatformName = %q, want metapi", cfg.ResinPlatformName)
	}
	if !cfg.ResinEnabled {
		t.Fatal("ResinEnabled = false, want true")
	}
}

func TestLoadResinDefaultsOff(t *testing.T) {
	cfg, _ := Load(map[string]string{})

	if cfg.ResinURL != "" {
		t.Fatalf("ResinURL = %q, want empty default", cfg.ResinURL)
	}
	if cfg.ResinPlatformName != "" {
		t.Fatalf("ResinPlatformName = %q, want empty default", cfg.ResinPlatformName)
	}
	if cfg.ResinEnabled {
		t.Fatal("ResinEnabled = true, want false default")
	}
}

func TestLoadPromptFilterDefaultsOff(t *testing.T) {
	cfg, _ := Load(map[string]string{})
	if cfg.PromptFilterEnabled {
		t.Fatal("PromptFilterEnabled = true, want false default")
	}
	if len(cfg.PromptFilterDenyPatterns) != 0 {
		t.Fatalf("PromptFilterDenyPatterns = %v, want empty default", cfg.PromptFilterDenyPatterns)
	}
}

func TestLoadPromptFilterParsesEnabledAndPatterns(t *testing.T) {
	cfg, _ := Load(map[string]string{
		"PROMPT_FILTER_ENABLED":       "true",
		"PROMPT_FILTER_DENY_PATTERNS": " forbidden phrase , another-bad ,",
	})
	if !cfg.PromptFilterEnabled {
		t.Fatal("PromptFilterEnabled = false, want true")
	}
	want := []string{"forbidden phrase", "another-bad"}
	if len(cfg.PromptFilterDenyPatterns) != len(want) {
		t.Fatalf("PromptFilterDenyPatterns = %v, want %v", cfg.PromptFilterDenyPatterns, want)
	}
	for i := range want {
		if cfg.PromptFilterDenyPatterns[i] != want[i] {
			t.Fatalf("PromptFilterDenyPatterns[%d] = %q, want %q", i, cfg.PromptFilterDenyPatterns[i], want[i])
		}
	}
}

func TestLoadParsesUTLSEnabled(t *testing.T) {
	cfg, _ := Load(map[string]string{
		"UTLS_ENABLED": "true",
	})
	if !cfg.UTLSEnabled {
		t.Fatal("UTLSEnabled = false, want true")
	}
}

func TestLoadUTLSDefaultsOff(t *testing.T) {
	cfg, _ := Load(map[string]string{})
	if cfg.UTLSEnabled {
		t.Fatal("UTLSEnabled = true, want false default")
	}
}

func TestLoadUpdateCenterDefaultsOff(t *testing.T) {
	cfg, _ := Load(map[string]string{})
	if cfg.UpdateCenterEnabled {
		t.Fatal("UpdateCenterEnabled = true, want false default")
	}
}

func TestLoadParsesUpdateCenterEnabled(t *testing.T) {
	cfg, _ := Load(map[string]string{
		"METAPI_ENABLE_UPDATE_CENTER": "true",
	})
	if !cfg.UpdateCenterEnabled {
		t.Fatal("UpdateCenterEnabled = false, want true")
	}
}

func TestLoadPrefersDBURLOverDatabaseURLAlias(t *testing.T) {
	_, rt := Load(map[string]string{
		"DB_URL":       "sqlite://local.db",
		"DATABASE_URL": "postgres://db.example.com:5432/metapi",
	})

	if rt.DbType != "sqlite" {
		t.Fatalf("DbType = %q, want sqlite", rt.DbType)
	}
	if rt.DbUrl != "sqlite://local.db" {
		t.Fatalf("DbUrl = %q, want DB_URL value", rt.DbUrl)
	}
}

func TestLoadParsesPostgresSSLMode(t *testing.T) {
	cfg, rt := Load(map[string]string{
		"DB_SSL":     "true",
		"DB_SSLMODE": "verify-full",
	})

	if cfg.DbSslMode != "verify-full" {
		t.Fatalf("DbSslMode = %q, want verify-full", cfg.DbSslMode)
	}
	if got := cfg.PostgresSSLMode(rt.DbSsl); got != "verify-full" {
		t.Fatalf("PostgresSSLMode() = %q, want verify-full", got)
	}
}

func TestPostgresSSLModePreservesLegacyDBSSL(t *testing.T) {
	cfg, rt := Load(map[string]string{
		"DB_SSL": "true",
	})

	if got := cfg.PostgresSSLMode(rt.DbSsl); got != "require" {
		t.Fatalf("PostgresSSLMode() = %q, want require", got)
	}
}

func TestLoadParsesPostgresPoolBudget(t *testing.T) {
	cfg, _ := Load(map[string]string{
		"DB_MAX_OPEN_CONNS":         "2",
		"DB_MAX_IDLE_CONNS":         "1",
		"DB_CONN_MAX_LIFETIME_SEC":  "1800",
		"DB_CONN_MAX_IDLE_TIME_SEC": "300",
	})

	if cfg.DbMaxOpenConns != 2 || cfg.DbMaxIdleConns != 1 {
		t.Fatalf("pool = %d/%d, want 2/1", cfg.DbMaxOpenConns, cfg.DbMaxIdleConns)
	}
	if cfg.DbConnMaxLifetimeSec != 1800 || cfg.DbConnMaxIdleTimeSec != 300 {
		t.Fatalf(
			"lifetime/idle = %d/%d, want 1800/300",
			cfg.DbConnMaxLifetimeSec,
			cfg.DbConnMaxIdleTimeSec,
		)
	}
}

func TestValidateRejectsPostgresPoolAboveOpenBudget(t *testing.T) {
	cfg, _ := Load(map[string]string{
		"AUTH_TOKEN":                "admin-token",
		"PROXY_TOKEN":               "proxy-token",
		"ACCOUNT_CREDENTIAL_SECRET": "credential-secret",
		"CLAUDE_CLIENT_ID":          "claude-client",
		"CODEX_CLIENT_ID":           "codex-client",
		"GEMINI_CLI_CLIENT_ID":      "gemini-client",
		"DB_MAX_OPEN_CONNS":         "2",
		"DB_MAX_IDLE_CONNS":         "3",
	})

	for _, err := range cfg.Validate() {
		if strings.Contains(err.Error(), "db_max_idle_conns") && IsCritical(err) {
			return
		}
	}
	t.Fatal("Validate did not return critical db_max_idle_conns error")
}

func TestValidateRejectsInvalidPostgresSSLMode(t *testing.T) {
	cfg, _ := Load(map[string]string{
		"AUTH_TOKEN":                "admin-token",
		"PROXY_TOKEN":               "proxy-token",
		"ACCOUNT_CREDENTIAL_SECRET": "credential-secret",
		"CLAUDE_CLIENT_ID":          "claude-client",
		"CODEX_CLIENT_ID":           "codex-client",
		"GEMINI_CLI_CLIENT_ID":      "gemini-client",
		"DB_SSLMODE":                "verify-maybe",
	})

	var found bool
	for _, err := range cfg.Validate() {
		if strings.Contains(err.Error(), "db_sslmode") && IsCritical(err) {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("Validate did not return critical db_sslmode error")
	}
}

func TestValidateRejectsDefaultTokensAsCritical(t *testing.T) {
	_, rt := Load(map[string]string{
		"ACCOUNT_CREDENTIAL_SECRET": "credential-secret",
		"CLAUDE_CLIENT_ID":          "claude-client",
		"CODEX_CLIENT_ID":           "codex-client",
		"GEMINI_CLI_CLIENT_ID":      "gemini-client",
	})

	for _, field := range []string{"auth_token", "proxy_token"} {
		t.Run(field, func(t *testing.T) {
			var found bool
			for _, err := range rt.Validate() {
				if strings.Contains(err.Error(), field) && IsCritical(err) {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("Validate did not return critical %s error for default token", field)
			}
		})
	}
}

func TestValidateAcceptsExplicitNonDefaultTokens(t *testing.T) {
	_, rt := Load(map[string]string{
		"AUTH_TOKEN":                "admin-token",
		"PROXY_TOKEN":               "proxy-token",
		"ACCOUNT_CREDENTIAL_SECRET": "credential-secret",
		"CLAUDE_CLIENT_ID":          "claude-client",
		"CODEX_CLIENT_ID":           "codex-client",
		"GEMINI_CLI_CLIENT_ID":      "gemini-client",
	})

	for _, err := range rt.Validate() {
		if strings.Contains(err.Error(), "auth_token") || strings.Contains(err.Error(), "proxy_token") {
			t.Fatalf("Validate returned token error for explicit non-default tokens: %v", err)
		}
	}
}

func TestValidateRejectsUnsafeAdminCorsOrigins(t *testing.T) {
	for _, origin := range []string{"*", "https://*.example.com", "https://admin.example.com/path", "javascript:alert(1)"} {
		t.Run(origin, func(t *testing.T) {
			cfg, _ := Load(map[string]string{
				"AUTH_TOKEN":                 "admin-token",
				"PROXY_TOKEN":                "proxy-token",
				"ACCOUNT_CREDENTIAL_SECRET":  "credential-secret",
				"CLAUDE_CLIENT_ID":           "claude-client",
				"CODEX_CLIENT_ID":            "codex-client",
				"GEMINI_CLI_CLIENT_ID":       "gemini-client",
				"ADMIN_CORS_ALLOWED_ORIGINS": origin,
			})

			var found bool
			for _, err := range cfg.Validate() {
				if strings.Contains(err.Error(), "admin_cors_allowed_origins") && IsCritical(err) {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("Validate did not return critical admin CORS error for %q", origin)
			}
		})
	}
}

func TestValidateAcceptsExactAdminCorsOrigins(t *testing.T) {
	cfg, _ := Load(map[string]string{
		"AUTH_TOKEN":                 "admin-token",
		"PROXY_TOKEN":                "proxy-token",
		"ACCOUNT_CREDENTIAL_SECRET":  "credential-secret",
		"CLAUDE_CLIENT_ID":           "claude-client",
		"CODEX_CLIENT_ID":            "codex-client",
		"GEMINI_CLI_CLIENT_ID":       "gemini-client",
		"ADMIN_CORS_ALLOWED_ORIGINS": "https://admin.example.com,http://localhost:5173",
	})

	for _, err := range cfg.Validate() {
		if strings.Contains(err.Error(), "admin_cors_allowed_origins") {
			t.Fatalf("Validate returned admin CORS error for exact origins: %v", err)
		}
	}
}

func TestValidateRejectsInvalidTrustedProxyCidrs(t *testing.T) {
	cfg, _ := Load(map[string]string{
		"AUTH_TOKEN":                "admin-token",
		"PROXY_TOKEN":               "proxy-token",
		"ACCOUNT_CREDENTIAL_SECRET": "credential-secret",
		"CLAUDE_CLIENT_ID":          "claude-client",
		"CODEX_CLIENT_ID":           "codex-client",
		"GEMINI_CLI_CLIENT_ID":      "gemini-client",
		"TRUSTED_PROXY_CIDRS":       "127.0.0.1/32,not-a-cidr",
	})

	var found bool
	for _, err := range cfg.Validate() {
		if strings.Contains(err.Error(), "trusted_proxy_cidrs") && IsCritical(err) {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("Validate did not return critical trusted proxy CIDR error")
	}
}

func TestValidateCronExprAcceptsFiveField(t *testing.T) {
	// 5-field cron (no seconds) should pass — validateCronExpr auto-normalises.
	for _, expr := range []string{"0 8 * * *", "0 * * * *", "0 6 * * *", "30 2 * * 1-5"} {
		if !ValidateCronExpr(expr) {
			t.Fatalf("validateCronExpr(%q) = false, want true for 5-field expression", expr)
		}
	}
}

func TestValidateCronExprAcceptsSixField(t *testing.T) {
	for _, expr := range []string{"0 0 8 * * *", "15 0 * * * *"} {
		if !ValidateCronExpr(expr) {
			t.Fatalf("validateCronExpr(%q) = false, want true for 6-field expression", expr)
		}
	}
}

func TestValidateCronExprRejectsInvalid(t *testing.T) {
	for _, expr := range []string{"", "not a cron", "* * * * * * *", "100 8 * * *"} {
		if ValidateCronExpr(expr) {
			t.Fatalf("validateCronExpr(%q) = true, want false", expr)
		}
	}
}

func TestLoadDbProfileDefaults(t *testing.T) {
	// Default profile is normal (10/3).
	cfg, _ := Load(map[string]string{})
	if cfg.DbProfile != "normal" {
		t.Fatalf("DbProfile = %q, want normal", cfg.DbProfile)
	}
	if cfg.DbMaxOpenConns != DefaultDbMaxOpenConnsNormal || cfg.DbMaxIdleConns != DefaultDbMaxIdleConnsNormal {
		t.Fatalf("normal defaults = %d/%d, want %d/%d", cfg.DbMaxOpenConns, cfg.DbMaxIdleConns, DefaultDbMaxOpenConnsNormal, DefaultDbMaxIdleConnsNormal)
	}

	tiny, _ := Load(map[string]string{"DB_PROFILE": "shared-tiny"})
	if tiny.DbProfile != "shared-tiny" {
		t.Fatalf("DbProfile = %q, want shared-tiny", tiny.DbProfile)
	}
	if tiny.DbMaxOpenConns != DefaultDbMaxOpenConnsSharedTiny || tiny.DbMaxIdleConns != DefaultDbMaxIdleConnsSharedTiny {
		t.Fatalf("shared-tiny defaults = %d/%d, want %d/%d", tiny.DbMaxOpenConns, tiny.DbMaxIdleConns, DefaultDbMaxOpenConnsSharedTiny, DefaultDbMaxIdleConnsSharedTiny)
	}

	dedicated, _ := Load(map[string]string{"METAPI_DB_PROFILE": "dedicated"})
	if dedicated.DbProfile != "dedicated" {
		t.Fatalf("DbProfile = %q, want dedicated", dedicated.DbProfile)
	}
	if dedicated.DbMaxOpenConns != DefaultDbMaxOpenConnsDedicated || dedicated.DbMaxIdleConns != DefaultDbMaxIdleConnsDedicated {
		t.Fatalf("dedicated defaults = %d/%d, want %d/%d", dedicated.DbMaxOpenConns, dedicated.DbMaxIdleConns, DefaultDbMaxOpenConnsDedicated, DefaultDbMaxIdleConnsDedicated)
	}
}

func TestLoadExplicitPoolOverridesProfile(t *testing.T) {
	cfg, _ := Load(map[string]string{
		"DB_PROFILE":        "shared-tiny",
		"DB_MAX_OPEN_CONNS": "50",
		"DB_MAX_IDLE_CONNS": "10",
	})
	if cfg.DbMaxOpenConns != 50 || cfg.DbMaxIdleConns != 10 {
		t.Fatalf("explicit override = %d/%d, want 50/10", cfg.DbMaxOpenConns, cfg.DbMaxIdleConns)
	}
}

func TestLoadParsesBrandingEnv(t *testing.T) {
	_, rt := Load(map[string]string{
		"SYSTEM_NAME":       "My Gateway",
		"LOGO":              "https://example.com/logo.png",
		"FOOTER":            "Powered by Metapi",
		"ABOUT":             "About page copy",
		"HOME_PAGE_CONTENT": "Welcome",
		"SERVER_ADDRESS":    "https://gw.example.com",
	})
	if rt.SystemName != "My Gateway" {
		t.Fatalf("SystemName = %q", rt.SystemName)
	}
	if rt.Logo != "https://example.com/logo.png" {
		t.Fatalf("Logo = %q", rt.Logo)
	}
	if rt.Footer != "Powered by Metapi" {
		t.Fatalf("Footer = %q", rt.Footer)
	}
	if rt.About != "About page copy" {
		t.Fatalf("About = %q", rt.About)
	}
	if rt.ServerAddress != "https://gw.example.com" {
		t.Fatalf("ServerAddress = %q", rt.ServerAddress)
	}
}

func TestLoadBrandingDefaultsEmpty(t *testing.T) {
	_, rt := Load(map[string]string{})
	if rt.SystemName != "" || rt.Logo != "" || rt.Footer != "" || rt.About != "" || rt.ServerAddress != "" {
		t.Fatalf("branding defaults not empty: %+v", rt)
	}
}

func TestValidateAcceptsWindowScheduleMode(t *testing.T) {
	// Checkin schedule mode lives on the runtime snapshot now; static pool
	// fields are irrelevant to this assertion.
	rt := &RuntimeSettings{
		DbType:              "sqlite",
		CheckinScheduleMode: "window",
		AuthToken:           "not-default-admin",
		ProxyToken:          "sk-not-default-proxy",
	}
	for _, err := range rt.Validate() {
		if configErrorField(err) == "checkin_schedule_mode" && configErrorCritical(err) {
			t.Fatalf("window mode rejected as critical: %v", err)
		}
	}
}

func TestValidateRejectsBogusScheduleMode(t *testing.T) {
	rt := &RuntimeSettings{
		DbType:              "sqlite",
		CheckinScheduleMode: "bogus",
		AuthToken:           "not-default-admin",
		ProxyToken:          "sk-not-default-proxy",
	}
	found := false
	for _, err := range rt.Validate() {
		if configErrorField(err) == "checkin_schedule_mode" {
			found = true
			if !configErrorCritical(err) {
				t.Fatalf("bogus mode not critical: %v", err)
			}
		}
	}
	if !found {
		t.Fatal("no checkin_schedule_mode validation error")
	}
}

func configErrorField(err error) string {
	if ce, ok := err.(*configError); ok {
		return ce.field
	}
	return ""
}

func configErrorCritical(err error) bool {
	if ce, ok := err.(*configError); ok {
		return ce.critical
	}
	return false
}

func TestValidateCriticalOnShortCredentialSecret(t *testing.T) {
	// 7-byte secret (< 8) must be a critical error. Short keys were silently
	// accepted before the length floor was added.
	cfg, _ := Load(map[string]string{
		"AUTH_TOKEN":                "admin-token-not-default",
		"PROXY_TOKEN":               "proxy-token-not-default",
		"ACCOUNT_CREDENTIAL_SECRET": "tiny123", // 7 bytes
		"CLAUDE_CLIENT_ID":          "claude-client",
		"CODEX_CLIENT_ID":           "codex-client",
		"GEMINI_CLI_CLIENT_ID":      "gemini-client",
	})

	var found bool
	for _, err := range cfg.Validate() {
		if configErrorField(err) == "account_credential_secret" && configErrorCritical(err) {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("Validate did not return critical error for < 8 byte credential secret")
	}
}

func TestValidateWarnsOnWeakCredentialSecret(t *testing.T) {
	// 14-byte secret (>= 8, < 16) must be a non-critical warning.
	cfg, _ := Load(map[string]string{
		"AUTH_TOKEN":                "admin-token-not-default",
		"PROXY_TOKEN":               "proxy-token-not-default",
		"ACCOUNT_CREDENTIAL_SECRET": "mediumsecret12", // 14 bytes
		"CLAUDE_CLIENT_ID":          "claude-client",
		"CODEX_CLIENT_ID":           "codex-client",
		"GEMINI_CLI_CLIENT_ID":      "gemini-client",
	})

	var foundWarning bool
	for _, err := range cfg.Validate() {
		if configErrorField(err) != "account_credential_secret" {
			continue
		}
		if configErrorCritical(err) {
			t.Fatalf("14-byte secret flagged critical, want warning: %v", err)
		}
		foundWarning = true
	}
	if !foundWarning {
		t.Fatal("Validate did not return weak-secret warning for 14 byte credential secret")
	}
}

func TestValidateAcceptsStrongCredentialSecret(t *testing.T) {
	// 17-byte secret (>= 16) must produce no credential_secret length error.
	cfg, _ := Load(map[string]string{
		"AUTH_TOKEN":                "admin-token-not-default",
		"PROXY_TOKEN":               "proxy-token-not-default",
		"ACCOUNT_CREDENTIAL_SECRET": "credential-secret", // 17 bytes
		"CLAUDE_CLIENT_ID":          "claude-client",
		"CODEX_CLIENT_ID":           "codex-client",
		"GEMINI_CLI_CLIENT_ID":      "gemini-client",
	})

	for _, err := range cfg.Validate() {
		if configErrorField(err) == "account_credential_secret" {
			t.Fatalf("Validate returned credential_secret error for 17 byte secret: %v", err)
		}
	}
}

func TestLoadLogLevelDefaultsToInfo(t *testing.T) {
	cfg, _ := Load(map[string]string{})
	if cfg.LogLevel != DefaultLogLevel {
		t.Fatalf("LogLevel = %q, want %q", cfg.LogLevel, DefaultLogLevel)
	}
	if cfg.LogLevel != "info" {
		t.Fatalf("LogLevel = %q, want info", cfg.LogLevel)
	}
}

func TestLoadLogLevelParsesKnownLevels(t *testing.T) {
	cases := map[string]string{
		"debug":   "debug",
		"DEBUG":   "debug",
		" info ":  "info",
		"warn":    "warn",
		"warning": "warn",
		"Warn":    "warn",
		"error":   "error",
		"ERROR":   "error",
	}
	for input, want := range cases {
		t.Run(input, func(t *testing.T) {
			cfg, _ := Load(map[string]string{"LOG_LEVEL": input})
			if cfg.LogLevel != want {
				t.Fatalf("LOG_LEVEL=%q → LogLevel = %q, want %q", input, cfg.LogLevel, want)
			}
		})
	}
}

func TestLoadLogLevelFallsBackOnInvalid(t *testing.T) {
	for _, input := range []string{"verbose", "trace", "123", "off"} {
		t.Run(input, func(t *testing.T) {
			cfg, _ := Load(map[string]string{"LOG_LEVEL": input})
			if cfg.LogLevel != "info" {
				t.Fatalf("LOG_LEVEL=%q → LogLevel = %q, want info (fallback)", input, cfg.LogLevel)
			}
		})
	}
}

func TestSlogLevelMapsCanonicalLevels(t *testing.T) {
	cases := map[string]slog.Level{
		"debug": slog.LevelDebug,
		"info":  slog.LevelInfo,
		"warn":  slog.LevelWarn,
		"error": slog.LevelError,
	}
	for input, want := range cases {
		if got := SlogLevel(input); got != want {
			t.Fatalf("SlogLevel(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestSlogLevelFallsBackToInfo(t *testing.T) {
	if got := SlogLevel("bogus"); got != slog.LevelInfo {
		t.Fatalf("SlogLevel(\"bogus\") = %v, want LevelInfo", got)
	}
	if got := SlogLevel(""); got != slog.LevelInfo {
		t.Fatalf("SlogLevel(\"\") = %v, want LevelInfo", got)
	}
}

func TestLoadAdminOAuthRateLimitDefaults(t *testing.T) {
	cfg, _ := Load(map[string]string{})

	if cfg.AdminRateLimitRPS != DefaultAdminRateLimitRPS {
		t.Fatalf("AdminRateLimitRPS = %d, want %d", cfg.AdminRateLimitRPS, DefaultAdminRateLimitRPS)
	}
	if cfg.AdminRateLimitBurst != DefaultAdminRateLimitBurst {
		t.Fatalf("AdminRateLimitBurst = %d, want %d", cfg.AdminRateLimitBurst, DefaultAdminRateLimitBurst)
	}
	if cfg.OAuthRateLimitRPS != DefaultOAuthRateLimitRPS {
		t.Fatalf("OAuthRateLimitRPS = %d, want %d", cfg.OAuthRateLimitRPS, DefaultOAuthRateLimitRPS)
	}
	if cfg.OAuthRateLimitBurst != DefaultOAuthRateLimitBurst {
		t.Fatalf("OAuthRateLimitBurst = %d, want %d", cfg.OAuthRateLimitBurst, DefaultOAuthRateLimitBurst)
	}
}

func TestLoadAdminOAuthRateLimitFromEnv(t *testing.T) {
	cfg, _ := Load(map[string]string{
		"ADMIN_RATE_LIMIT_RPS":   "200",
		"ADMIN_RATE_LIMIT_BURST": "400",
		"OAUTH_RATE_LIMIT_RPS":   "25",
		"OAUTH_RATE_LIMIT_BURST": "50",
	})

	if cfg.AdminRateLimitRPS != 200 {
		t.Fatalf("AdminRateLimitRPS = %d, want 200", cfg.AdminRateLimitRPS)
	}
	if cfg.AdminRateLimitBurst != 400 {
		t.Fatalf("AdminRateLimitBurst = %d, want 400", cfg.AdminRateLimitBurst)
	}
	if cfg.OAuthRateLimitRPS != 25 {
		t.Fatalf("OAuthRateLimitRPS = %d, want 25", cfg.OAuthRateLimitRPS)
	}
	if cfg.OAuthRateLimitBurst != 50 {
		t.Fatalf("OAuthRateLimitBurst = %d, want 50", cfg.OAuthRateLimitBurst)
	}
}

func TestLoadAdminOAuthRateLimitClampsNegativeToZero(t *testing.T) {
	cfg, _ := Load(map[string]string{
		"ADMIN_RATE_LIMIT_RPS":   "-5",
		"ADMIN_RATE_LIMIT_BURST": "-10",
		"OAUTH_RATE_LIMIT_RPS":   "-1",
		"OAUTH_RATE_LIMIT_BURST": "-2",
	})

	if cfg.AdminRateLimitRPS != 0 {
		t.Fatalf("AdminRateLimitRPS = %d, want 0 (clamped)", cfg.AdminRateLimitRPS)
	}
	if cfg.AdminRateLimitBurst != 0 {
		t.Fatalf("AdminRateLimitBurst = %d, want 0 (clamped)", cfg.AdminRateLimitBurst)
	}
	if cfg.OAuthRateLimitRPS != 0 {
		t.Fatalf("OAuthRateLimitRPS = %d, want 0 (clamped)", cfg.OAuthRateLimitRPS)
	}
	if cfg.OAuthRateLimitBurst != 0 {
		t.Fatalf("OAuthRateLimitBurst = %d, want 0 (clamped)", cfg.OAuthRateLimitBurst)
	}
}

func TestLoadAdminOAuthRateLimitFallsBackOnInvalid(t *testing.T) {
	cfg, _ := Load(map[string]string{
		"ADMIN_RATE_LIMIT_RPS":   "not-a-number",
		"ADMIN_RATE_LIMIT_BURST": "still-not-a-number",
		"OAUTH_RATE_LIMIT_RPS":   "garbage",
		"OAUTH_RATE_LIMIT_BURST": "oops",
	})

	if cfg.AdminRateLimitRPS != DefaultAdminRateLimitRPS {
		t.Fatalf("AdminRateLimitRPS = %d, want default %d on invalid input", cfg.AdminRateLimitRPS, DefaultAdminRateLimitRPS)
	}
	if cfg.AdminRateLimitBurst != DefaultAdminRateLimitBurst {
		t.Fatalf("AdminRateLimitBurst = %d, want default %d on invalid input", cfg.AdminRateLimitBurst, DefaultAdminRateLimitBurst)
	}
	if cfg.OAuthRateLimitRPS != DefaultOAuthRateLimitRPS {
		t.Fatalf("OAuthRateLimitRPS = %d, want default %d on invalid input", cfg.OAuthRateLimitRPS, DefaultOAuthRateLimitRPS)
	}
	if cfg.OAuthRateLimitBurst != DefaultOAuthRateLimitBurst {
		t.Fatalf("OAuthRateLimitBurst = %d, want default %d on invalid input", cfg.OAuthRateLimitBurst, DefaultOAuthRateLimitBurst)
	}
}

func TestLoadProxyTimeoutsDefaults(t *testing.T) {
	cfg, _ := Load(map[string]string{})
	// Pin the defaults: they must equal the timeouts that were hardcoded in
	// platform/site_proxy.go before #1009 so unset deployments behave
	// identically.
	tests := []struct {
		name string
		got  int
		want int
	}{
		{"ProxyConnectTimeoutSec", cfg.ProxyConnectTimeoutSec, 2},
		{"ProxyTLSHandshakeTimeoutSec", cfg.ProxyTLSHandshakeTimeoutSec, 10},
		{"ProxyResponseHeaderTimeoutSec", cfg.ProxyResponseHeaderTimeoutSec, 30},
		{"ProxyIdleConnTimeoutSec", cfg.ProxyIdleConnTimeoutSec, 90},
		{"ProxyRequestTimeoutSec", cfg.ProxyRequestTimeoutSec, 30},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Fatalf("%s = %d on empty env, want %d (pre-#1009 hardcoded default)", tt.name, tt.got, tt.want)
		}
	}
}

func TestLoadParsesProxyTimeouts(t *testing.T) {
	cfg, _ := Load(map[string]string{
		"PROXY_CONNECT_TIMEOUT_SEC":         "7",
		"PROXY_TLS_HANDSHAKE_TIMEOUT_SEC":   "11",
		"PROXY_RESPONSE_HEADER_TIMEOUT_SEC": "45.9", // truncated to 45, same as LDOH
		"PROXY_IDLE_CONN_TIMEOUT_SEC":       "120",
		"PROXY_REQUEST_TIMEOUT_SEC":         "60",
	})
	if cfg.ProxyConnectTimeoutSec != 7 {
		t.Fatalf("ProxyConnectTimeoutSec = %d, want 7", cfg.ProxyConnectTimeoutSec)
	}
	if cfg.ProxyTLSHandshakeTimeoutSec != 11 {
		t.Fatalf("ProxyTLSHandshakeTimeoutSec = %d, want 11", cfg.ProxyTLSHandshakeTimeoutSec)
	}
	if cfg.ProxyResponseHeaderTimeoutSec != 45 {
		t.Fatalf("ProxyResponseHeaderTimeoutSec = %d, want 45 (truncated)", cfg.ProxyResponseHeaderTimeoutSec)
	}
	if cfg.ProxyIdleConnTimeoutSec != 120 {
		t.Fatalf("ProxyIdleConnTimeoutSec = %d, want 120", cfg.ProxyIdleConnTimeoutSec)
	}
	if cfg.ProxyRequestTimeoutSec != 60 {
		t.Fatalf("ProxyRequestTimeoutSec = %d, want 60", cfg.ProxyRequestTimeoutSec)
	}
}

func TestLoadProxyTimeoutsFallBackOnInvalid(t *testing.T) {
	cfg, _ := Load(map[string]string{
		"PROXY_CONNECT_TIMEOUT_SEC":         "0",
		"PROXY_TLS_HANDSHAKE_TIMEOUT_SEC":   "-5",
		"PROXY_RESPONSE_HEADER_TIMEOUT_SEC": "soon",
		"PROXY_IDLE_CONN_TIMEOUT_SEC":       "  ",
		// PROXY_REQUEST_TIMEOUT_SEC intentionally unset.
	})
	if cfg.ProxyConnectTimeoutSec != DefaultProxyConnectTimeoutSec {
		t.Fatalf("ProxyConnectTimeoutSec = %d on zero, want default %d", cfg.ProxyConnectTimeoutSec, DefaultProxyConnectTimeoutSec)
	}
	if cfg.ProxyTLSHandshakeTimeoutSec != DefaultProxyTLSHandshakeTimeoutSec {
		t.Fatalf("ProxyTLSHandshakeTimeoutSec = %d on negative, want default %d", cfg.ProxyTLSHandshakeTimeoutSec, DefaultProxyTLSHandshakeTimeoutSec)
	}
	if cfg.ProxyResponseHeaderTimeoutSec != DefaultProxyResponseHeaderTimeoutSec {
		t.Fatalf("ProxyResponseHeaderTimeoutSec = %d on invalid, want default %d", cfg.ProxyResponseHeaderTimeoutSec, DefaultProxyResponseHeaderTimeoutSec)
	}
	if cfg.ProxyIdleConnTimeoutSec != DefaultProxyIdleConnTimeoutSec {
		t.Fatalf("ProxyIdleConnTimeoutSec = %d on blank, want default %d", cfg.ProxyIdleConnTimeoutSec, DefaultProxyIdleConnTimeoutSec)
	}
	if cfg.ProxyRequestTimeoutSec != DefaultProxyRequestTimeoutSec {
		t.Fatalf("ProxyRequestTimeoutSec = %d on unset, want default %d", cfg.ProxyRequestTimeoutSec, DefaultProxyRequestTimeoutSec)
	}
}

func TestLoadModelSyncCronDefault(t *testing.T) {
	_, rt := Load(map[string]string{})
	if rt.ModelSyncCron != DefaultModelSyncCron {
		t.Fatalf("ModelSyncCron = %q, want default %q", rt.ModelSyncCron, DefaultModelSyncCron)
	}
}

func TestLoadParsesModelSyncCron(t *testing.T) {
	_, rt := Load(map[string]string{
		"MODEL_SYNC_CRON": "0 5 * * 1",
	})
	if rt.ModelSyncCron != "0 5 * * 1" {
		t.Fatalf("ModelSyncCron = %q", rt.ModelSyncCron)
	}
}
