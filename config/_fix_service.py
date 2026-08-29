import re

# --- service/account_service.go ---
src = open('service/account_service.go', encoding='utf-8').read()
old = '''	// 5. system proxy fallback (account opt-in, then site opt-in)
	if account != nil {
		if GetUseSystemProxyFromExtraConfig(account.ExtraConfig) && cfg != nil && strings.TrimSpace(cfg.SystemProxyUrl) != "" {
			proxyCfg.ProxyURL = strings.TrimSpace(cfg.SystemProxyUrl)
			proxyCfg.UseSystemProxy = true
			return normalizePlatformProxyConfig(proxyCfg)
		}
	}

	if site != nil {
		if site.UseSystemProxy && cfg != nil && strings.TrimSpace(cfg.SystemProxyUrl) != "" {
			proxyCfg.ProxyURL = strings.TrimSpace(cfg.SystemProxyUrl)
			proxyCfg.UseSystemProxy = true
			return normalizePlatformProxyConfig(proxyCfg)
		}
	}'''
new = '''	// 5. system proxy fallback (account opt-in, then site opt-in). The
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
	}'''
assert old in src
src = src.replace(old, new, 1)
open('service/account_service.go','w',encoding='utf-8').write(src)

# --- service/oauth/flow.go ---
src = open('service/oauth/flow.go', encoding='utf-8').read()
src = src.replace('systemProxy := strings.TrimSpace(config.Get().SystemProxyUrl)',
                  'systemProxy := strings.TrimSpace(config.Runtime().SystemProxyUrl)', 1)
old = '''func resolveOauthProviderProxyUrl(provider string) *string {
	cfg := config.Get()
	switch provider {
	case "codex":
		// Codex provider uses system proxy if configured.
		if cfg.SystemProxyUrl != "" {
			return &cfg.SystemProxyUrl
		}
	case "claude":
		if cfg.SystemProxyUrl != "" {
			return &cfg.SystemProxyUrl
		}
	case "gemini-cli":
		if cfg.SystemProxyUrl != "" {
			return &cfg.SystemProxyUrl
		}
	case "antigravity":
		if cfg.SystemProxyUrl != "" {
			return &cfg.SystemProxyUrl
		}
	}
	return nil
}'''
new = '''func resolveOauthProviderProxyUrl(provider string) *string {
	rt := config.Runtime()
	switch provider {
	case "codex":
		// Codex provider uses system proxy if configured.
		if rt.SystemProxyUrl != "" {
			return &rt.SystemProxyUrl
		}
	case "claude":
		if rt.SystemProxyUrl != "" {
			return &rt.SystemProxyUrl
		}
	case "gemini-cli":
		if rt.SystemProxyUrl != "" {
			return &rt.SystemProxyUrl
		}
	case "antigravity":
		if rt.SystemProxyUrl != "" {
			return &rt.SystemProxyUrl
		}
	}
	return nil
}'''
assert old in src
src = src.replace(old, new, 1)
open('service/oauth/flow.go','w',encoding='utf-8').write(src)
print("service fixes applied")
