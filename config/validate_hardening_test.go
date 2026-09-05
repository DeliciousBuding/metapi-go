package config

import (
	"strings"
	"testing"
)

// validBaseConfig returns a static Config that is free of every existing
// Config.Validate warning/critical so the hardening tests can isolate a
// single rule by flipping one field and asserting exactly one new diagnostic
// appears. Runtime-mutable fields live on validBaseRuntime.
func validBaseConfig() *Config {
	return &Config{
		AccountCredentialSecret: "credential-secret-1234567890",
		ClaudeClientId:          "claude-client-id",
		CodexClientId:           "codex-client-id",
		GeminiCliClientId:       "gemini-cli-client-id",
		Port:                    4000,
		ListenHost:              "0.0.0.0",
		DbMaxOpenConns:          10,
		DbMaxIdleConns:          3,
		DbConnMaxLifetimeSec:    1800,
		DbConnMaxIdleTimeSec:    300,
		ProxyRateLimitRPM:       60,
		ProxyGlobalTokenRPM:     0,
		ProxyMaxChannelAttempts: 3,
		ProxyStickySessionTtlMs: 30 * 60 * 1000,
		TokenRouterCacheTtlMs:   1500,
	}
}

// validBaseRuntime is the RuntimeSettings twin of validBaseConfig: every
// runtime-mutable field set to a value that produces no diagnostics.
func validBaseRuntime() *RuntimeSettings {
	return &RuntimeSettings{
		AuthToken:                           "admin-token-1234",
		ProxyToken:                          "proxy-token-1234",
		DbType:                              "sqlite",
		CheckinScheduleMode:                 "cron",
		CheckinCron:                         "0 8 * * *",
		BalanceRefreshCron:                  "0 * * * *",
		ModelSyncCron:                       "0 4 * * *",
		LogCleanupCron:                      "0 6 * * *",
		CheckinIntervalHours:                6,
		CheckinWindowStart:                  "00:00",
		CheckinWindowEnd:                    "23:59",
		NotifyCooldownSec:                   300,
		ProxySessionChannelConcurrencyLimit: 2,
		ProxySessionChannelQueueWaitMs:      1500,
		TelegramApiBaseUrl:                  "https://api.telegram.org",
	}
}

// assertHasError fails the test unless Config.Validate returns a diagnostic
// whose message contains substr and whose severity matches wantCritical.
func assertHasError(t *testing.T, cfg *Config, substr string, wantCritical bool) {
	t.Helper()
	assertHasErrorIn(t, cfg.Validate(), substr, wantCritical)
}

// assertHasRuntimeError is the RuntimeSettings twin of assertHasError.
func assertHasRuntimeError(t *testing.T, rt *RuntimeSettings, substr string, wantCritical bool) {
	t.Helper()
	assertHasErrorIn(t, rt.Validate(), substr, wantCritical)
}

func assertHasErrorIn(t *testing.T, errs []error, substr string, wantCritical bool) {
	t.Helper()
	for _, err := range errs {
		if !strings.Contains(err.Error(), substr) {
			continue
		}
		if wantCritical && !IsCritical(err) {
			t.Fatalf("expected critical error for %q, got non-critical: %v", substr, err)
		}
		if !wantCritical && !IsWarning(err) {
			t.Fatalf("expected warning for %q, got critical: %v", substr, err)
		}
		return
	}
	t.Fatalf("Validate did not return an error containing %q", substr)
}

// assertNoErrorFor fails the test if Config.Validate returns any error
// mentioning substr.
func assertNoErrorFor(t *testing.T, cfg *Config, substr string) {
	t.Helper()
	assertNoErrorIn(t, cfg.Validate(), substr)
}

// assertNoRuntimeErrorFor is the RuntimeSettings twin of assertNoErrorFor.
func assertNoRuntimeErrorFor(t *testing.T, rt *RuntimeSettings, substr string) {
	t.Helper()
	assertNoErrorIn(t, rt.Validate(), substr)
}

func assertNoErrorIn(t *testing.T, errs []error, substr string) {
	t.Helper()
	for _, err := range errs {
		if strings.Contains(err.Error(), substr) {
			t.Fatalf("Validate unexpectedly returned error for %q: %v", substr, err)
		}
	}
}

func TestValidateNegativeProxyRateLimitRPMIsWarning(t *testing.T) {
	cfg := validBaseConfig()
	cfg.ProxyRateLimitRPM = -5
	assertHasError(t, cfg, "proxy_rate_limit_rpm", false)
}

func TestValidateZeroProxyRateLimitRPMIsClean(t *testing.T) {
	cfg := validBaseConfig()
	cfg.ProxyRateLimitRPM = 0 // explicit disable must not warn.
	assertNoErrorFor(t, cfg, "proxy_rate_limit_rpm")
}

func TestValidateNegativeProxyGlobalTokenRPMIsWarning(t *testing.T) {
	cfg := validBaseConfig()
	cfg.ProxyGlobalTokenRPM = -1
	assertHasError(t, cfg, "proxy_global_token_rpm", false)
}

func TestValidateNegativeProxyMaxChannelAttemptsIsWarning(t *testing.T) {
	cfg := validBaseConfig()
	cfg.ProxyMaxChannelAttempts = -2
	assertHasError(t, cfg, "proxy_max_channel_attempts", false)
}

func TestValidateNegativeRateLimitRPMFromEnvIsWarning(t *testing.T) {
	// End-to-end: PROXY_RATE_LIMIT_RPM is no longer clamped at parse time, so
	// a negative env value reaches Validate and surfaces an operator warning.
	cfg, _ := Load(map[string]string{
		"AUTH_TOKEN":                "admin-token-1234",
		"PROXY_TOKEN":               "proxy-token-1234",
		"ACCOUNT_CREDENTIAL_SECRET": "credential-secret-1234567890",
		"CLAUDE_CLIENT_ID":          "claude-client-id",
		"CODEX_CLIENT_ID":           "codex-client-id",
		"GEMINI_CLI_CLIENT_ID":      "gemini-cli-client-id",
		"PROXY_RATE_LIMIT_RPM":      "-5",
	})
	assertHasError(t, cfg, "proxy_rate_limit_rpm", false)
}

func TestValidateConcurrencyLimitBelowOneIsWarning(t *testing.T) {
	rt := validBaseRuntime()
	rt.ProxySessionChannelConcurrencyLimit = 0
	assertHasRuntimeError(t, rt, "proxy_session_channel_concurrency_limit", false)
}

func TestValidateStickySessionTtlNonPositiveIsWarning(t *testing.T) {
	cfg := validBaseConfig()
	cfg.ProxyStickySessionTtlMs = 0
	assertHasError(t, cfg, "proxy_sticky_session_ttl_ms", false)
}

func TestValidateTokenRouterCacheTtlNonPositiveIsWarning(t *testing.T) {
	cfg := validBaseConfig()
	cfg.TokenRouterCacheTtlMs = 0
	assertHasError(t, cfg, "token_router_cache_ttl_ms", false)
}

func TestValidateQueueWaitMsNegativeIsWarning(t *testing.T) {
	rt := validBaseRuntime()
	rt.ProxySessionChannelQueueWaitMs = -100
	assertHasRuntimeError(t, rt, "proxy_session_channel_queue_wait_ms", false)
}

func TestValidateMalformedWebhookUrlIsWarning(t *testing.T) {
	rt := validBaseRuntime()
	rt.WebhookUrl = "://not-a-url"
	assertHasRuntimeError(t, rt, "webhook_url", false)
}

func TestValidateNonHttpSchemeWebhookUrlIsWarning(t *testing.T) {
	rt := validBaseRuntime()
	rt.FeishuWebhook = "ftp://example.com/hook"
	assertHasRuntimeError(t, rt, "feishu_webhook", false)
}

func TestValidateWebhookUrlMissingHostIsWarning(t *testing.T) {
	rt := validBaseRuntime()
	rt.BarkUrl = "https://"
	assertHasRuntimeError(t, rt, "bark_url", false)
}

func TestValidateValidWebhookUrlsAreClean(t *testing.T) {
	rt := validBaseRuntime()
	rt.WebhookUrl = "https://hooks.example.com/wh"
	rt.BarkUrl = "https://bark.example.com/key"
	rt.FeishuWebhook = "https://feishu.example.com/hook"
	rt.DingtalkWebhook = "https://oapi.dingtalk.com/hook"
	rt.WecomWebhook = "https://qyapi.weixin.example.com/hook"
	rt.NtfyUrl = "https://ntfy.example.com/topic"
	for _, field := range []string{
		"webhook_url", "bark_url", "feishu_webhook",
		"dingtalk_webhook", "wecom_webhook", "ntfy_url",
	} {
		assertNoRuntimeErrorFor(t, rt, field)
	}
	// ResinURL is static env-only config; it stays on Config.Validate.
	cfg := validBaseConfig()
	cfg.ResinURL = "http://resin.local:2260/my-token"
	assertNoErrorFor(t, cfg, "resin_url")
}

func TestValidateMalformedRedisUrlIsWarning(t *testing.T) {
	cfg := validBaseConfig()
	cfg.RedisURL = "://bad-redis"
	assertHasError(t, cfg, "redis_url", false)
}

func TestValidateRedisUrlWithRedisSchemeIsClean(t *testing.T) {
	cfg := validBaseConfig()
	cfg.RedisURL = "redis://localhost:6379/0"
	assertNoErrorFor(t, cfg, "redis_url")
}

func TestValidateSystemProxyUrlWithSocks5IsClean(t *testing.T) {
	// system_proxy_url allows non-http schemes (e.g. socks5), so only a
	// genuinely malformed / scheme-less value should warn.
	rt := validBaseRuntime()
	rt.SystemProxyUrl = "socks5://proxy.local:1080"
	assertNoRuntimeErrorFor(t, rt, "system_proxy_url")
}

func TestValidateSystemProxyUrlMissingSchemeIsWarning(t *testing.T) {
	rt := validBaseRuntime()
	rt.SystemProxyUrl = "proxy.local:1080"
	assertHasRuntimeError(t, rt, "system_proxy_url", false)
}

func TestValidateBadHhMmFormatIsWarning(t *testing.T) {
	rt := validBaseRuntime()
	rt.CheckinWindowStart = "9:00" // not 2-digit
	assertHasRuntimeError(t, rt, "checkin_window_start", false)
}

func TestValidateOutOfRangeHhMmIsWarning(t *testing.T) {
	rt := validBaseRuntime()
	rt.CheckinWindowEnd = "25:99"
	assertHasRuntimeError(t, rt, "checkin_window_end", false)
}

func TestValidateValidHhMmIsClean(t *testing.T) {
	rt := validBaseRuntime()
	rt.CheckinWindowStart = "08:30"
	rt.CheckinWindowEnd = "22:15"
	assertNoRuntimeErrorFor(t, rt, "checkin_window_start")
	assertNoRuntimeErrorFor(t, rt, "checkin_window_end")
}

func TestLoadProxyMaxStreamResponseBytesDefaultAndOverride(t *testing.T) {
	// Default when unset.
	cfg, _ := Load(map[string]string{})
	if cfg.ProxyMaxStreamResponseBytes != DefaultProxyMaxStreamResponseBytes {
		t.Fatalf("unset ProxyMaxStreamResponseBytes = %d, want %d",
			cfg.ProxyMaxStreamResponseBytes, DefaultProxyMaxStreamResponseBytes)
	}
	// Explicit override.
	cfg, _ = Load(map[string]string{"PROXY_MAX_STREAM_RESPONSE_BYTES": "2048"})
	if cfg.ProxyMaxStreamResponseBytes != 2048 {
		t.Fatalf("ProxyMaxStreamResponseBytes = %d, want 2048", cfg.ProxyMaxStreamResponseBytes)
	}
	// Non-positive falls back to the default.
	cfg, _ = Load(map[string]string{"PROXY_MAX_STREAM_RESPONSE_BYTES": "0"})
	if cfg.ProxyMaxStreamResponseBytes != DefaultProxyMaxStreamResponseBytes {
		t.Fatalf("zero ProxyMaxStreamResponseBytes = %d, want default %d",
			cfg.ProxyMaxStreamResponseBytes, DefaultProxyMaxStreamResponseBytes)
	}
	cfg, _ = Load(map[string]string{"PROXY_MAX_STREAM_RESPONSE_BYTES": "-10"})
	if cfg.ProxyMaxStreamResponseBytes != DefaultProxyMaxStreamResponseBytes {
		t.Fatalf("negative ProxyMaxStreamResponseBytes = %d, want default %d",
			cfg.ProxyMaxStreamResponseBytes, DefaultProxyMaxStreamResponseBytes)
	}
}

func TestValidateModelSyncCronInvalidIsWarning(t *testing.T) {
	rt := validBaseRuntime()
	rt.ModelSyncCron = "not a cron"
	assertHasRuntimeError(t, rt, "model_sync_cron", false)
}

func TestValidateModelSyncCronValidIsClean(t *testing.T) {
	rt := validBaseRuntime()
	rt.ModelSyncCron = "0 5 * * 1"
	assertNoRuntimeErrorFor(t, rt, "model_sync_cron")
}
