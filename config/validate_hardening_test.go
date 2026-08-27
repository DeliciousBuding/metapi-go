package config

import (
	"strings"
	"testing"
)

// validBaseConfig returns a Config that is free of every existing Validate
// warning/critical so the hardening tests can isolate a single rule by
// flipping one field and asserting exactly one new diagnostic appears.
func validBaseConfig() *Config {
	return &Config{
		AuthToken:                "admin-token-1234",
		ProxyToken:               "proxy-token-1234",
		AccountCredentialSecret:  "credential-secret-1234567890",
		ClaudeClientId:           "claude-client-id",
		CodexClientId:            "codex-client-id",
		GeminiCliClientId:        "gemini-cli-client-id",
		Port:                     4000,
		ListenHost:               "0.0.0.0",
		DbType:                   "sqlite",
		DbMaxOpenConns:           10,
		DbMaxIdleConns:           3,
		DbConnMaxLifetimeSec:     1800,
		DbConnMaxIdleTimeSec:     300,
		CheckinScheduleMode:      "cron",
		CheckinCron:              "0 8 * * *",
		BalanceRefreshCron:       "0 * * * *",
		ModelSyncCron:            "0 4 * * *",
		LogCleanupCron:           "0 6 * * *",
		CheckinIntervalHours:     6,
		CheckinWindowStart:       "00:00",
		CheckinWindowEnd:         "23:59",
		NotifyCooldownSec:        300,
		ProxyRateLimitRPM:        60,
		ProxyGlobalTokenRPM:      0,
		ProxyMaxChannelAttempts:  3,
		ProxySessionChannelConcurrencyLimit: 2,
		ProxyStickySessionEnabled:           false,
		ProxyStickySessionTtlMs:             30 * 60 * 1000,
		TokenRouterCacheTtlMs:               1500,
		ProxySessionChannelQueueWaitMs:      1500,
		ProxySessionChannelLeaseTtlMs:       90000,
		ProxySessionChannelLeaseKeepaliveMs: 15000,
		TelegramApiBaseUrl:                  "https://api.telegram.org",
	}
}

// assertHasError fails the test unless Validate returns a diagnostic whose
// message contains substr and whose severity matches wantCritical.
func assertHasError(t *testing.T, cfg *Config, substr string, wantCritical bool) {
	t.Helper()
	for _, err := range cfg.Validate() {
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

// assertNoErrorFor fails the test if Validate returns any error mentioning substr.
func assertNoErrorFor(t *testing.T, cfg *Config, substr string) {
	t.Helper()
	for _, err := range cfg.Validate() {
		if strings.Contains(err.Error(), substr) {
			t.Fatalf("Validate unexpectedly returned error for %q: %v", substr, err)
		}
	}
}

func TestValidateLeaseKeepaliveExceedsTtlIsCritical(t *testing.T) {
	cfg := validBaseConfig()
	cfg.ProxySessionChannelLeaseKeepaliveMs = 30000
	cfg.ProxySessionChannelLeaseTtlMs = 10000
	assertHasError(t, cfg, "proxy_session_channel_lease_keepalive_ms", true)
}

func TestValidateLeaseKeepaliveEqualsTtlIsWarning(t *testing.T) {
	cfg := validBaseConfig()
	cfg.ProxySessionChannelLeaseKeepaliveMs = 30000
	cfg.ProxySessionChannelLeaseTtlMs = 30000
	assertHasError(t, cfg, "proxy_session_channel_lease_keepalive_ms", false)
}

func TestValidateLeaseKeepaliveBelowTtlIsClean(t *testing.T) {
	cfg := validBaseConfig()
	// keepalive (15000) < ttl (90000): the healthy default path must not warn.
	assertNoErrorFor(t, cfg, "proxy_session_channel_lease_keepalive_ms")
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
	cfg := Load(map[string]string{
		"AUTH_TOKEN":               "admin-token-1234",
		"PROXY_TOKEN":              "proxy-token-1234",
		"ACCOUNT_CREDENTIAL_SECRET": "credential-secret-1234567890",
		"CLAUDE_CLIENT_ID":         "claude-client-id",
		"CODEX_CLIENT_ID":          "codex-client-id",
		"GEMINI_CLI_CLIENT_ID":     "gemini-cli-client-id",
		"PROXY_RATE_LIMIT_RPM":     "-5",
	})
	assertHasError(t, cfg, "proxy_rate_limit_rpm", false)
}

func TestValidateConcurrencyLimitBelowOneIsWarning(t *testing.T) {
	cfg := validBaseConfig()
	cfg.ProxySessionChannelConcurrencyLimit = 0
	assertHasError(t, cfg, "proxy_session_channel_concurrency_limit", false)
}

func TestValidateStickySessionTtlNonPositiveIsWarning(t *testing.T) {
	cfg := validBaseConfig()
	cfg.ProxyStickySessionEnabled = true
	cfg.ProxyStickySessionTtlMs = 0
	assertHasError(t, cfg, "proxy_sticky_session_ttl_ms", false)
}

func TestValidateStickySessionDisabledSkipsTtlCheck(t *testing.T) {
	cfg := validBaseConfig()
	cfg.ProxyStickySessionEnabled = false
	cfg.ProxyStickySessionTtlMs = 0 // disabled, so ttl is irrelevant
	assertNoErrorFor(t, cfg, "proxy_sticky_session_ttl_ms")
}

func TestValidateTokenRouterCacheTtlNonPositiveIsWarning(t *testing.T) {
	cfg := validBaseConfig()
	cfg.TokenRouterCacheTtlMs = 0
	assertHasError(t, cfg, "token_router_cache_ttl_ms", false)
}

func TestValidateQueueWaitMsNegativeIsWarning(t *testing.T) {
	cfg := validBaseConfig()
	cfg.ProxySessionChannelQueueWaitMs = -100
	assertHasError(t, cfg, "proxy_session_channel_queue_wait_ms", false)
}

func TestValidateMalformedWebhookUrlIsWarning(t *testing.T) {
	cfg := validBaseConfig()
	cfg.WebhookUrl = "://not-a-url"
	assertHasError(t, cfg, "webhook_url", false)
}

func TestValidateNonHttpSchemeWebhookUrlIsWarning(t *testing.T) {
	cfg := validBaseConfig()
	cfg.FeishuWebhook = "ftp://example.com/hook"
	assertHasError(t, cfg, "feishu_webhook", false)
}

func TestValidateWebhookUrlMissingHostIsWarning(t *testing.T) {
	cfg := validBaseConfig()
	cfg.BarkUrl = "https://"
	assertHasError(t, cfg, "bark_url", false)
}

func TestValidateValidWebhookUrlsAreClean(t *testing.T) {
	cfg := validBaseConfig()
	cfg.WebhookUrl = "https://hooks.example.com/wh"
	cfg.BarkUrl = "https://bark.example.com/key"
	cfg.FeishuWebhook = "https://feishu.example.com/hook"
	cfg.DingtalkWebhook = "https://oapi.dingtalk.com/hook"
	cfg.WecomWebhook = "https://qyapi.weixin.example.com/hook"
	cfg.NtfyUrl = "https://ntfy.example.com/topic"
	cfg.ResinURL = "http://resin.local:2260/my-token"
	for _, field := range []string{
		"webhook_url", "bark_url", "feishu_webhook",
		"dingtalk_webhook", "wecom_webhook", "ntfy_url", "resin_url",
	} {
		assertNoErrorFor(t, cfg, field)
	}
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
	cfg := validBaseConfig()
	cfg.SystemProxyUrl = "socks5://proxy.local:1080"
	assertNoErrorFor(t, cfg, "system_proxy_url")
}

func TestValidateSystemProxyUrlMissingSchemeIsWarning(t *testing.T) {
	cfg := validBaseConfig()
	cfg.SystemProxyUrl = "proxy.local:1080"
	assertHasError(t, cfg, "system_proxy_url", false)
}

func TestValidateBadHhMmFormatIsWarning(t *testing.T) {
	cfg := validBaseConfig()
	cfg.CheckinWindowStart = "9:00" // not 2-digit
	assertHasError(t, cfg, "checkin_window_start", false)
}

func TestValidateOutOfRangeHhMmIsWarning(t *testing.T) {
	cfg := validBaseConfig()
	cfg.CheckinWindowEnd = "25:99"
	assertHasError(t, cfg, "checkin_window_end", false)
}

func TestValidateValidHhMmIsClean(t *testing.T) {
	cfg := validBaseConfig()
	cfg.CheckinWindowStart = "08:30"
	cfg.CheckinWindowEnd = "22:15"
	assertNoErrorFor(t, cfg, "checkin_window_start")
	assertNoErrorFor(t, cfg, "checkin_window_end")
}

func TestLoadProxyMaxStreamResponseBytesDefaultAndOverride(t *testing.T) {
	// Default when unset.
	cfg := Load(map[string]string{})
	if cfg.ProxyMaxStreamResponseBytes != DefaultProxyMaxStreamResponseBytes {
		t.Fatalf("unset ProxyMaxStreamResponseBytes = %d, want %d",
			cfg.ProxyMaxStreamResponseBytes, DefaultProxyMaxStreamResponseBytes)
	}
	// Explicit override.
	cfg = Load(map[string]string{"PROXY_MAX_STREAM_RESPONSE_BYTES": "2048"})
	if cfg.ProxyMaxStreamResponseBytes != 2048 {
		t.Fatalf("ProxyMaxStreamResponseBytes = %d, want 2048", cfg.ProxyMaxStreamResponseBytes)
	}
	// Non-positive falls back to the default.
	cfg = Load(map[string]string{"PROXY_MAX_STREAM_RESPONSE_BYTES": "0"})
	if cfg.ProxyMaxStreamResponseBytes != DefaultProxyMaxStreamResponseBytes {
		t.Fatalf("zero ProxyMaxStreamResponseBytes = %d, want default %d",
			cfg.ProxyMaxStreamResponseBytes, DefaultProxyMaxStreamResponseBytes)
	}
	cfg = Load(map[string]string{"PROXY_MAX_STREAM_RESPONSE_BYTES": "-10"})
	if cfg.ProxyMaxStreamResponseBytes != DefaultProxyMaxStreamResponseBytes {
		t.Fatalf("negative ProxyMaxStreamResponseBytes = %d, want default %d",
			cfg.ProxyMaxStreamResponseBytes, DefaultProxyMaxStreamResponseBytes)
	}
}
func TestValidateModelSyncCronInvalidIsWarning(t *testing.T) {
	cfg := validBaseConfig()
	cfg.ModelSyncCron = "not a cron"
	assertHasError(t, cfg, "model_sync_cron", false)
}

func TestValidateModelSyncCronValidIsClean(t *testing.T) {
	cfg := validBaseConfig()
	cfg.ModelSyncCron = "0 5 * * 1"
	assertNoErrorFor(t, cfg, "model_sync_cron")
}
