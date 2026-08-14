package service

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/store"
)

// TestUTLSEnabledGating verifies the global UTLS_ENABLED flag gates uTLS
// masking when no per-site override is present.
func TestUTLSEnabledGating(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		cfg  *config.Config
		site *store.Site
		want bool
	}{
		{"nil config", nil, nil, false},
		{"disabled flag", &config.Config{UTLSEnabled: false}, nil, false},
		{"enabled flag nil site", &config.Config{UTLSEnabled: true}, nil, true},
		{"disabled flag nil site", &config.Config{UTLSEnabled: false}, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := UTLSEnabled(tc.cfg, tc.site); got != tc.want {
				t.Fatalf("UTLSEnabled = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestUTLSEnabledPerSiteOverride verifies per-site use_utls overrides the
// global UTLS_ENABLED flag in both directions (opt-in and opt-out).
func TestUTLSEnabledPerSiteOverride(t *testing.T) {
	t.Parallel()
	enabled := true
	disabled := false
	cases := []struct {
		name string
		cfg  *config.Config
		site *store.Site
		want bool
	}{
		{
			name: "global on, site nil override → inherit global",
			cfg:  &config.Config{UTLSEnabled: true},
			site: &store.Site{ID: 1, UseUTLS: nil},
			want: true,
		},
		{
			name: "global on, site explicit true → still true",
			cfg:  &config.Config{UTLSEnabled: true},
			site: &store.Site{ID: 1, UseUTLS: &enabled},
			want: true,
		},
		{
			name: "global on, site explicit false → false (per-site opt-out)",
			cfg:  &config.Config{UTLSEnabled: true},
			site: &store.Site{ID: 1, UseUTLS: &disabled},
			want: false,
		},
		{
			name: "global off, site explicit true → true (per-site opt-in)",
			cfg:  &config.Config{UTLSEnabled: false},
			site: &store.Site{ID: 1, UseUTLS: &enabled},
			want: true,
		},
		{
			name: "global off, site nil override → inherit global (off)",
			cfg:  &config.Config{UTLSEnabled: false},
			site: &store.Site{ID: 1, UseUTLS: nil},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := UTLSEnabled(tc.cfg, tc.site); got != tc.want {
				t.Fatalf("UTLSEnabled = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestBuildPlatformProxyConfig_SetsUseUTLS verifies that BuildPlatformProxyConfig
// propagates the resolved uTLS flag to the ProxyConfig, and that
// normalizePlatformProxyConfig does not strip a UseUTLS-only config to nil.
func TestBuildPlatformProxyConfig_SetsUseUTLS(t *testing.T) {
	enabled := true
	site := &store.Site{
		ID:       1,
		Name:     "test",
		URL:      "https://example.com",
		Platform: "openai",
		UseUTLS:  &enabled,
	}
	cfg := &config.Config{UTLSEnabled: false}

	result := BuildPlatformProxyConfig(cfg, nil, site)
	if result == nil {
		t.Fatal("BuildPlatformProxyConfig returned nil for site with UseUTLS=true")
	}
	if !result.UseUTLS {
		t.Fatal("ProxyConfig.UseUTLS = false, want true (site override)")
	}
}

// TestBuildPlatformProxyConfig_UTLSGlobalOn verifies the global UTLS_ENABLED
// flag alone (no per-site override) produces a non-nil ProxyConfig with
// UseUTLS=true, even when no proxy URL or headers are configured.
func TestBuildPlatformProxyConfig_UTLSGlobalOn(t *testing.T) {
	site := &store.Site{
		ID:       1,
		Name:     "test",
		URL:      "https://example.com",
		Platform: "openai",
	}
	cfg := &config.Config{UTLSEnabled: true}

	result := BuildPlatformProxyConfig(cfg, nil, site)
	if result == nil {
		t.Fatal("BuildPlatformProxyConfig returned nil for global UTLS on")
	}
	if !result.UseUTLS {
		t.Fatal("ProxyConfig.UseUTLS = false, want true (global on)")
	}
}

// TestBuildPlatformProxyConfig_UTLSGlobalOff verifies that when both global
// and per-site uTLS are off, the ProxyConfig is nil (no utls, no proxy).
func TestBuildPlatformProxyConfig_UTLSGlobalOff(t *testing.T) {
	site := &store.Site{
		ID:       1,
		Name:     "test",
		URL:      "https://example.com",
		Platform: "openai",
	}
	cfg := &config.Config{UTLSEnabled: false}

	result := BuildPlatformProxyConfig(cfg, nil, site)
	if result != nil {
		t.Fatalf("BuildPlatformProxyConfig = %+v, want nil (no utls, no proxy)", result)
	}
}

// TestBuildPlatformProxyConfigForToken_PropagatesUTLS verifies the pre-account
// variant also carries the resolved UseUTLS flag from the inner
// BuildPlatformProxyConfig call.
func TestBuildPlatformProxyConfigForToken_PropagatesUTLS(t *testing.T) {
	enabled := true
	site := &store.Site{
		ID:       1,
		Name:     "test",
		URL:      "https://example.com",
		Platform: "openai",
		UseUTLS:  &enabled,
	}
	cfg := &config.Config{UTLSEnabled: false}

	result := BuildPlatformProxyConfigForToken(cfg, site, "raw-token")
	if result == nil {
		t.Fatal("BuildPlatformProxyConfigForToken returned nil for site with UseUTLS=true")
	}
	if !result.UseUTLS {
		t.Fatal("ProxyConfig.UseUTLS = false, want true (site override)")
	}
}

// TestNormalizePlatformProxyConfig_KeepsUTLS verifies that a ProxyConfig with
// only UseUTLS=true (no proxy URL, headers, or cookies) is NOT stripped to nil
// by normalizePlatformProxyConfig — otherwise global uTLS opt-in with no other
// proxy settings would be silently dropped.
func TestNormalizePlatformProxyConfig_KeepsUTLS(t *testing.T) {
	cfg := &platform.ProxyConfig{UseUTLS: true}
	result := normalizePlatformProxyConfig(cfg)
	if result == nil {
		t.Fatal("normalizePlatformProxyConfig stripped UseUTLS-only config to nil")
	}
	if !result.UseUTLS {
		t.Fatal("UseUTLS = false after normalize, want true")
	}
}

// TestNormalizePlatformProxyConfig_NilForEmpty verifies that a completely
// empty ProxyConfig (UseUTLS=false, no proxy, no headers) is still nilled.
func TestNormalizePlatformProxyConfig_NilForEmpty(t *testing.T) {
	cfg := &platform.ProxyConfig{}
	result := normalizePlatformProxyConfig(cfg)
	if result != nil {
		t.Fatal("normalizePlatformProxyConfig should return nil for empty config")
	}
}
