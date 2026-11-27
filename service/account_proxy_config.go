package service

import (
	"encoding/json"
	"strings"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/store"
)

// GetProxyURLFromExtraConfig reads proxyUrl from extraConfig.
func GetProxyURLFromExtraConfig(extraConfig *string) string {
	config := ParseExtraConfig(extraConfig)
	if config == nil {
		return ""
	}
	if url, ok := config["proxyUrl"].(string); ok {
		return url
	}
	return ""
}

// GetUseSystemProxyFromExtraConfig reads useSystemProxy from extraConfig.
func GetUseSystemProxyFromExtraConfig(extraConfig *string) bool {
	config := ParseExtraConfig(extraConfig)
	if config == nil {
		return false
	}
	if enabled, ok := config["useSystemProxy"].(bool); ok {
		return enabled
	}
	return false
}

// UTLSEnabled resolves whether uTLS TLS fingerprint masking is active for a
// site. Per-site UseUTLS overrides the global UTLS_ENABLED flag:
//   - site.UseUTLS != nil → explicit per-site decision (true/false)
//   - site.UseUTLS == nil → fall back to cfg.UTLSEnabled (global default)
//
// Mirrors the ResinEnabled resolution pattern so operators can globally opt in
// via UTLS_ENABLED=true and selectively disable specific sites with use_utls=false.
func UTLSEnabled(cfg *config.Config, site *store.Site) bool {
	if site != nil && site.UseUTLS != nil {
		return *site.UseUTLS
	}
	return cfg != nil && cfg.UTLSEnabled
}

// BuildPlatformProxyConfig resolves account and site proxy settings for platform calls.
//
// Precedence (highest to lowest):
//  1. key proxy (applied separately via proxy.ApplyKeyProxyOverride)
//  2. account extraConfig.proxyUrl (explicit per-account proxy)
//  3. site.ProxyURL (explicit per-site proxy)
//  4. Resin sticky forward proxy (when RESIN_ENABLED and identity available)
//  5. system proxy (account extraConfig.useSystemProxy or site.UseSystemProxy)
//  6. direct (no proxy)
//
// The Resin tier sits between explicit proxy URLs and the generic system-proxy
// opt-in so an operator who sets a per-account/per-site proxy still wins, but
// Resin takes priority over the shared SYSTEM_PROXY_URL. Resin requires a
// business identity: acc-{account.ID} when the account row exists. For
// pre-account flows (verify/login/create) with no account row yet, use
// BuildPlatformProxyConfigForToken which derives a deterministic temp identity.
func BuildPlatformProxyConfig(cfg *config.Config, account *store.Account, site *store.Site) *platform.ProxyConfig {
	if account == nil && site == nil {
		return nil
	}

	proxyCfg := &platform.ProxyConfig{}
	if site != nil {
		// wire site opt-in so ApplyCustomHeaders can force site-wins.
		proxyCfg.CustomHeadersOverrideRequest = site.CustomHeadersOverrideRequestHeaders
		if site.CustomHeaders != nil && strings.TrimSpace(*site.CustomHeaders) != "" {
			var headers map[string]string
			if err := json.Unmarshal([]byte(*site.CustomHeaders), &headers); err == nil && len(headers) > 0 {
				proxyCfg.CustomHeaders = filterPlatformCustomHeaders(headers)
			}
		}
		if site.CfClearance != nil {
			proxyCfg.ClearanceCookie = strings.TrimSpace(*site.CfClearance)
		}
		if site.BrowserUA != nil {
			proxyCfg.BrowserUA = strings.TrimSpace(*site.BrowserUA)
		}
	}

	// Resolve uTLS from site-level override or global UTLS_ENABLED flag. This
	// is independent of the proxy-precedence chain below — uTLS masking applies
	// regardless of whether a proxy URL is configured.
	proxyCfg.UseUTLS = UTLSEnabled(cfg, site)

	// 2. account explicit proxyUrl
	if account != nil {
		if accountProxyURL := strings.TrimSpace(GetProxyURLFromExtraConfig(account.ExtraConfig)); accountProxyURL != "" {
			proxyCfg.ProxyURL = accountProxyURL
			return normalizePlatformProxyConfig(proxyCfg)
		}
	}

	// 3. site explicit ProxyURL
	if site != nil {
		if site.ProxyURL != nil && strings.TrimSpace(*site.ProxyURL) != "" {
			proxyCfg.ProxyURL = strings.TrimSpace(*site.ProxyURL)
			return normalizePlatformProxyConfig(proxyCfg)
		}
	}

	// 4. Resin sticky forward proxy (only when no higher-precedence proxy set).
	//    Requires a business identity: acc-{account.ID} when account exists.
	if ResinEnabled(cfg, site) {
		platformName := ResolveResinPlatform(cfg, site)
		accountIdentity := AccountFor(account, site)
		if platformName != "" && accountIdentity != "" {
			if resinURL, ok := ForwardProxyURL(cfg, platformName, accountIdentity); ok {
				proxyCfg.ProxyURL = resinURL
				proxyCfg.ResinAccount = accountIdentity
				TouchResinLease(accountIdentity)
				return normalizePlatformProxyConfig(proxyCfg)
			}
		}
	}

	// 5. system proxy fallback (account opt-in, then site opt-in). The
	// runtime snapshot is read once so one request can never observe two
	// different system-proxy values mid-decision.
	systemProxy := ""
	if rt := config.RuntimeSafe(); rt != nil {
		systemProxy = strings.TrimSpace(rt.SystemProxyUrl)
	}
	if account != nil {
		if GetUseSystemProxyFromExtraConfig(account.ExtraConfig) && systemProxy != "" {
			proxyCfg.ProxyURL = systemProxy
			proxyCfg.UseSystemProxy = true
			return normalizePlatformProxyConfig(proxyCfg)
		}
	}

	if site != nil {
		if site.UseSystemProxy && systemProxy != "" {
			proxyCfg.ProxyURL = systemProxy
			proxyCfg.UseSystemProxy = true
			return normalizePlatformProxyConfig(proxyCfg)
		}
	}

	return normalizePlatformProxyConfig(proxyCfg)
}

// BuildPlatformProxyConfigForToken is the pre-account variant of
// BuildPlatformProxyConfig for verify-token / login / create flows where no
// Account row exists yet. It derives a deterministic temp Resin identity from
// (siteID, rawToken) so all pre-account requests for the same token share an
// egress IP. Nothing carries that assignment over to the stable acc-{id}
// identity used once the Account row exists: the forward-proxy cache and the
// lease tracker are both keyed by identity, and the tracker is
// observability-only — resin.go is explicit that there is no TCP lease to
// release, so there is nothing to hand over.
//
// Precedence matches BuildPlatformProxyConfig with the temp identity filling
// the resin slot: site.ProxyURL > resin(temp) > system > direct.
func BuildPlatformProxyConfigForToken(cfg *config.Config, site *store.Site, rawToken string) *platform.ProxyConfig {
	// When the site has an explicit ProxyURL, that beats resin — delegate
	// entirely so headers/cookies/UA are assembled by the shared path.
	if site != nil && site.ProxyURL != nil && strings.TrimSpace(*site.ProxyURL) != "" {
		return BuildPlatformProxyConfig(cfg, nil, site)
	}

	// No explicit site.ProxyURL: assemble the base config (headers, cookies,
	// UA, and possibly the system proxy via site.UseSystemProxy).
	proxyCfg := BuildPlatformProxyConfig(cfg, nil, site)
	if proxyCfg == nil {
		proxyCfg = &platform.ProxyConfig{}
	}

	if !ResinEnabled(cfg, site) || site == nil {
		return normalizePlatformProxyConfig(proxyCfg)
	}

	platformName := ResolveResinPlatform(cfg, site)
	tempIdentity := TempAccountFor(site.ID, rawToken)
	if platformName == "" || tempIdentity == "" {
		return normalizePlatformProxyConfig(proxyCfg)
	}
	resinURL, ok := ForwardProxyURL(cfg, platformName, tempIdentity)
	if !ok {
		return normalizePlatformProxyConfig(proxyCfg)
	}
	// Resin wins over the system proxy per precedence, so override whatever
	// ProxyURL the inner call set (or fill the empty direct case).
	proxyCfg.ProxyURL = resinURL
	proxyCfg.UseSystemProxy = false
	proxyCfg.ResinAccount = tempIdentity
	TouchResinLease(tempIdentity)
	return normalizePlatformProxyConfig(proxyCfg)
}

// BuildPlatformProxyConfigForTokenWithProxyURL is the pre-account proxy
// builder for unsaved account-form values. A non-empty request override must
// win over the saved site proxy, Resin, and system proxy for the verification
// or create request; empty input keeps the normal site-level precedence.
func BuildPlatformProxyConfigForTokenWithProxyURL(cfg *config.Config, site *store.Site, rawToken, proxyURL string) *platform.ProxyConfig {
	proxyURL = strings.TrimSpace(proxyURL)
	if proxyURL == "" {
		return BuildPlatformProxyConfigForToken(cfg, site, rawToken)
	}

	// Build the site-derived headers/UA/cookie/uTLS settings without resolving
	// Resin or the saved site proxy, then apply the explicit unsaved override.
	proxyCfg := BuildPlatformProxyConfig(cfg, nil, site)
	if proxyCfg == nil {
		proxyCfg = &platform.ProxyConfig{}
	}
	proxyCfg.ProxyURL = proxyURL
	proxyCfg.UseSystemProxy = false
	proxyCfg.ResinAccount = ""
	return normalizePlatformProxyConfig(proxyCfg)
}

func normalizePlatformProxyConfig(proxyCfg *platform.ProxyConfig) *platform.ProxyConfig {
	if proxyCfg == nil {
		return nil
	}
	if proxyCfg.ProxyURL == "" && !proxyCfg.UseSystemProxy && len(proxyCfg.CustomHeaders) == 0 && !proxyCfg.InsecureSkipTLS &&
		proxyCfg.ClearanceCookie == "" && proxyCfg.BrowserUA == "" && !proxyCfg.UseUTLS {
		return nil
	}
	return proxyCfg
}

func filterPlatformCustomHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	filtered := make(map[string]string, len(headers))
	for k, v := range headers {
		if isReservedPlatformCustomHeader(k) {
			continue
		}
		filtered[k] = v
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

// isReservedPlatformCustomHeader is the assembly-side filter for site
// custom_headers: what a site may not inject into the platform proxy config.
// It is deliberately narrower than platform.IsDeniedCustomHeader (it also
// reserves content-type and new-api-user, which the data plane owns) but the
// security-critical set must stay in step with it — see
// TestReservedPlatformCustomHeadersStayInStepWithPlatformDenyList.
func isReservedPlatformCustomHeader(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	// accept-encoding decides whether the response body stays readable for
	// usage accounting, content-failure judgement and health checks, so it is
	// not site-configurable (same rule as platform.IsDeniedCustomHeader).
	case "authorization", "cookie", "new-api-user", "content-type", "content-length", "host", "accept-encoding":
		return true
	default:
		return false
	}
}
