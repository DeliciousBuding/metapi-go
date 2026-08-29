src = open('app/cache_ratio_settings.go', encoding='utf-8').read()
src = src.replace('''// ApplyCacheRatioOverrides pushes the operator-configured cache-ratio fallback
// overrides from config into the routing package's runtime. Called at
// boot after LoadRuntimeSettings hydrates cfg, and again from the admin
// settings handler on save. 0/missing values fall back to the code defaults
// (routing.SetCacheRatioDefaults ignores non-positive values).
func ApplyCacheRatioOverrides(cfg *config.Config) {
	routing.SetCacheRatioDefaults(
		cfg.CacheRatioDefault,
		0, // cache_creation default override not exposed yet (code default 1.0)
		cfg.CacheRatioClaude,
		0, // claude cache_creation override not exposed yet (code default 1.25)
	)
}''',
'''// ApplyCacheRatioOverrides pushes the operator-configured cache-ratio
// fallback overrides from the runtime-settings snapshot into the routing
// package's runtime. Called at boot after the snapshot is published, and
// again from the admin settings handler on save. 0/missing values fall back
// to the code defaults (routing.SetCacheRatioDefaults ignores non-positive
// values).
func ApplyCacheRatioOverrides() {
	rt := config.Runtime()
	routing.SetCacheRatioDefaults(
		rt.CacheRatioDefault,
		0, // cache_creation default override not exposed yet (code default 1.0)
		rt.CacheRatioClaude,
		0, // claude cache_creation override not exposed yet (code default 1.25)
	)
}''', 1)
open('app/cache_ratio_settings.go','w',encoding='utf-8').write(src)

src = open('app/proxy_upstream.go', encoding='utf-8').read()
src = src.replace('if firstByteMs := proxy.FirstByteTimeoutMs(cfg.ProxyFirstByteTimeoutSec); firstByteMs > 0 {',
                  'if firstByteMs := proxy.FirstByteTimeoutMs(config.Runtime().ProxyFirstByteTimeoutSec); firstByteMs > 0 {', 1)
open('app/proxy_upstream.go','w',encoding='utf-8').write(src)
print("app fixed")
