package config

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// REQUEST_BODY_LIMIT_MB parsing tests
// ---------------------------------------------------------------------------

func TestLoadRequestBodyLimitDefaultsTo20MB(t *testing.T) {
	cfg := Load(map[string]string{})
	if cfg.RequestBodyLimit != 20*1024*1024 {
		t.Fatalf("RequestBodyLimit = %d, want %d (20 MB default)", cfg.RequestBodyLimit, 20*1024*1024)
	}
}

func TestLoadRequestBodyLimitParsesCustomMB(t *testing.T) {
	cfg := Load(map[string]string{
		"REQUEST_BODY_LIMIT_MB": "50",
	})
	if cfg.RequestBodyLimit != 50*1024*1024 {
		t.Fatalf("RequestBodyLimit = %d, want %d (50 MB)", cfg.RequestBodyLimit, 50*1024*1024)
	}
}

func TestLoadRequestBodyLimitClampsToMin1MB(t *testing.T) {
	cfg := Load(map[string]string{
		"REQUEST_BODY_LIMIT_MB": "0",
	})
	if cfg.RequestBodyLimit != 1*1024*1024 {
		t.Fatalf("RequestBodyLimit = %d, want %d (clamped to 1 MB min)", cfg.RequestBodyLimit, 1*1024*1024)
	}
}

func TestLoadRequestBodyLimitClampsToMax200MB(t *testing.T) {
	cfg := Load(map[string]string{
		"REQUEST_BODY_LIMIT_MB": "999",
	})
	if cfg.RequestBodyLimit != 200*1024*1024 {
		t.Fatalf("RequestBodyLimit = %d, want %d (clamped to 200 MB max)", cfg.RequestBodyLimit, 200*1024*1024)
	}
}

func TestLoadRequestBodyLimitIgnoresInvalidValue(t *testing.T) {
	cfg := Load(map[string]string{
		"REQUEST_BODY_LIMIT_MB": "not-a-number",
	})
	if cfg.RequestBodyLimit != 20*1024*1024 {
		t.Fatalf("RequestBodyLimit = %d, want %d (default on invalid)", cfg.RequestBodyLimit, 20*1024*1024)
	}
}

// ---------------------------------------------------------------------------
// FILE_UPLOAD_LIMIT_MB parsing tests
// ---------------------------------------------------------------------------

func TestLoadFileUploadLimitDefaultsTo100MB(t *testing.T) {
	cfg := Load(map[string]string{})
	if cfg.FileUploadLimitBytes != 100*1024*1024 {
		t.Fatalf("FileUploadLimitBytes = %d, want %d (100 MB default)", cfg.FileUploadLimitBytes, 100*1024*1024)
	}
}

func TestLoadFileUploadLimitParsesCustomMB(t *testing.T) {
	cfg := Load(map[string]string{
		"FILE_UPLOAD_LIMIT_MB": "200",
	})
	if cfg.FileUploadLimitBytes != 200*1024*1024 {
		t.Fatalf("FileUploadLimitBytes = %d, want %d (200 MB)", cfg.FileUploadLimitBytes, 200*1024*1024)
	}
}

func TestLoadFileUploadLimitClampsToMin1MB(t *testing.T) {
	cfg := Load(map[string]string{
		"FILE_UPLOAD_LIMIT_MB": "0",
	})
	if cfg.FileUploadLimitBytes != 1*1024*1024 {
		t.Fatalf("FileUploadLimitBytes = %d, want %d (clamped to 1 MB min)", cfg.FileUploadLimitBytes, 1*1024*1024)
	}
}

func TestLoadFileUploadLimitClampsToMax1000MB(t *testing.T) {
	cfg := Load(map[string]string{
		"FILE_UPLOAD_LIMIT_MB": "99999",
	})
	if cfg.FileUploadLimitBytes != 1000*1024*1024 {
		t.Fatalf("FileUploadLimitBytes = %d, want %d (clamped to 1000 MB max)", cfg.FileUploadLimitBytes, 1000*1024*1024)
	}
}

// ---------------------------------------------------------------------------
// PROXY_RATE_LIMIT_RPM parsing tests
// ---------------------------------------------------------------------------

func TestLoadProxyRateLimitRPMDefaultsTo60(t *testing.T) {
	cfg := Load(map[string]string{})
	if cfg.ProxyRateLimitRPM != 60 {
		t.Fatalf("ProxyRateLimitRPM = %d, want 60 (default)", cfg.ProxyRateLimitRPM)
	}
}

func TestLoadProxyRateLimitRPMDisabledWhenZero(t *testing.T) {
	cfg := Load(map[string]string{
		"PROXY_RATE_LIMIT_RPM": "0",
	})
	if cfg.ProxyRateLimitRPM != 0 {
		t.Fatalf("ProxyRateLimitRPM = %d, want 0 (disabled)", cfg.ProxyRateLimitRPM)
	}
}

func TestLoadProxyRateLimitRPMParsesCustomValue(t *testing.T) {
	cfg := Load(map[string]string{
		"PROXY_RATE_LIMIT_RPM": "120",
	})
	if cfg.ProxyRateLimitRPM != 120 {
		t.Fatalf("ProxyRateLimitRPM = %d, want 120", cfg.ProxyRateLimitRPM)
	}
}

func TestLoadProxyRateLimitRPMIgnoresInvalidValue(t *testing.T) {
	cfg := Load(map[string]string{
		"PROXY_RATE_LIMIT_RPM": "abc",
	})
	if cfg.ProxyRateLimitRPM != 60 {
		t.Fatalf("ProxyRateLimitRPM = %d, want 60 (default on invalid)", cfg.ProxyRateLimitRPM)
	}
}

// Negative RPM values are intentionally left intact by Load so config.Validate
// can surface a startup warning ("use 0 for explicit disable"). The limiter
// consumer (auth.ProxyRateLimit) still treats <=0 as disabled, so the only
// observable effect of a negative is the warning — no silent disable.
func TestLoadProxyRateLimitRPMPreservesNegativeForValidation(t *testing.T) {
	cfg := Load(map[string]string{
		"PROXY_RATE_LIMIT_RPM": "-5",
	})
	if cfg.ProxyRateLimitRPM != -5 {
		t.Fatalf("ProxyRateLimitRPM = %d, want -5 (preserved for Validate warning)", cfg.ProxyRateLimitRPM)
	}
	var warned bool
	for _, err := range cfg.Validate() {
		if IsWarning(err) && strings.Contains(err.Error(), "proxy_rate_limit_rpm") {
			warned = true
			break
		}
	}
	if !warned {
		t.Fatal("Validate did not warn on negative PROXY_RATE_LIMIT_RPM")
	}
}

// ---------------------------------------------------------------------------
// PROXY_GLOBAL_TOKEN_RPM parsing tests
// ---------------------------------------------------------------------------

func TestLoadProxyGlobalTokenRPMDefaultsToZero(t *testing.T) {
	cfg := Load(map[string]string{})
	if cfg.ProxyGlobalTokenRPM != 0 {
		t.Fatalf("ProxyGlobalTokenRPM = %d, want 0 (default = unlimited)", cfg.ProxyGlobalTokenRPM)
	}
}

func TestLoadProxyGlobalTokenRPMParsesCustomValue(t *testing.T) {
	cfg := Load(map[string]string{
		"PROXY_GLOBAL_TOKEN_RPM": "300",
	})
	if cfg.ProxyGlobalTokenRPM != 300 {
		t.Fatalf("ProxyGlobalTokenRPM = %d, want 300", cfg.ProxyGlobalTokenRPM)
	}
}

// Negative values are preserved (not clamped) so Validate can warn; the global
// token limiter still treats <=0 as unlimited, matching 0 = explicit disable.
func TestLoadProxyGlobalTokenRPMPreservesNegativeForValidation(t *testing.T) {
	cfg := Load(map[string]string{
		"PROXY_GLOBAL_TOKEN_RPM": "-10",
	})
	if cfg.ProxyGlobalTokenRPM != -10 {
		t.Fatalf("ProxyGlobalTokenRPM = %d, want -10 (preserved for Validate warning)", cfg.ProxyGlobalTokenRPM)
	}
	var warned bool
	for _, err := range cfg.Validate() {
		if IsWarning(err) && strings.Contains(err.Error(), "proxy_global_token_rpm") {
			warned = true
			break
		}
	}
	if !warned {
		t.Fatal("Validate did not warn on negative PROXY_GLOBAL_TOKEN_RPM")
	}
}

func TestLoadProxyGlobalTokenRPMIgnoresInvalidValue(t *testing.T) {
	cfg := Load(map[string]string{
		"PROXY_GLOBAL_TOKEN_RPM": "nope",
	})
	if cfg.ProxyGlobalTokenRPM != 0 {
		t.Fatalf("ProxyGlobalTokenRPM = %d, want 0 (default on invalid)", cfg.ProxyGlobalTokenRPM)
	}
}
