# codex_ws_runtime.go line 524: runtime field via RuntimeSafe
src = open('handler/proxy/codex_ws_runtime.go', encoding='utf-8').read()
src = src.replace('''	cfg := safeConfigGet()
	if cfg == nil || !cfg.CodexUpstreamWebsocketEnabled {
		return false
	}''',
'''	if rt := config.RuntimeSafe(); rt == nil || !rt.CodexUpstreamWebsocketEnabled {
		return false
	}''', 1)
open('handler/proxy/codex_ws_runtime.go','w',encoding='utf-8').write(src)

# responses_ws.go line 775: runtime field via RuntimeSafe
src = open('handler/proxy/responses_ws.go', encoding='utf-8').read()
src = src.replace('''	runtimeCfg := safeConfigGet()
	if runtimeCfg == nil || !runtimeCfg.CodexUpstreamWebsocketEnabled {''',
'''	runtimeCfg := config.RuntimeSafe()
	if runtimeCfg == nil || !runtimeCfg.CodexUpstreamWebsocketEnabled {''', 1)
open('handler/proxy/responses_ws.go','w',encoding='utf-8').write(src)

# upstream_stream.go:204
src = open('handler/proxy/upstream_stream.go', encoding='utf-8').read()
src = src.replace('if cfg := config.GetSafe(); cfg != nil && cfg.ProxyEmptyContentFailEnabled {',
                  'if rt := config.RuntimeSafe(); rt != nil && rt.ProxyEmptyContentFailEnabled {', 1)
open('handler/proxy/upstream_stream.go','w',encoding='utf-8').write(src)

# upstream.go: alert + runtimeCfg split
src = open('handler/proxy/upstream.go', encoding='utf-8').read()
src = src.replace('alert.ReportProxyAllFailed(config.GetSafe(), db.DB,', 'alert.ReportProxyAllFailed(config.RuntimeSafe(), db.DB,', 1)
src = src.replace('''	runtimeCfg := config.Get()
	// Proxy selection: key proxy > account > site > system > direct
	// See proxy.KeyProxyPrecedence.
	proxyConfig := service.BuildPlatformProxyConfig(runtimeCfg, &selected.Account, &selected.Site)
	if ctx != nil && ctx.Auth != nil {
		proxyConfig = proxy.ApplyKeyProxyOverride(proxyConfig, ctx.Auth.ProxyURL)
	}
	firstByteTimeoutMs := int64(0)
	disableCrossProtocolFallback := false
	if runtimeCfg != nil {
		firstByteTimeoutMs = proxy.FirstByteTimeoutMs(runtimeCfg.ProxyFirstByteTimeoutSec)
		disableCrossProtocolFallback = runtimeCfg.DisableCrossProtocolFallback
	}''',
'''	staticCfg := config.Get()
	// Proxy selection: key proxy > account > site > system > direct
	// See proxy.KeyProxyPrecedence.
	proxyConfig := service.BuildPlatformProxyConfig(staticCfg, &selected.Account, &selected.Site)
	if ctx != nil && ctx.Auth != nil {
		proxyConfig = proxy.ApplyKeyProxyOverride(proxyConfig, ctx.Auth.ProxyURL)
	}
	// Runtime-mutable knobs come from the atomic settings snapshot so a
	// live settings change applies on the very next request.
	firstByteTimeoutMs := int64(0)
	disableCrossProtocolFallback := false
	if rt := config.RuntimeSafe(); rt != nil {
		firstByteTimeoutMs = proxy.FirstByteTimeoutMs(rt.ProxyFirstByteTimeoutSec)
		disableCrossProtocolFallback = rt.DisableCrossProtocolFallback
	}''', 1)
open('handler/proxy/upstream.go','w',encoding='utf-8').write(src)
print("handler/proxy fixed")
