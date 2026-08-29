import re
src = open('routing/router.go', encoding='utf-8').read()
src = src.replace('''	cacheTTLMs := int64(cfg.TokenRouterCacheTtlMs)
	cache := NewRouteCache(cacheTTLMs)
	SetGlobalCache(cache)

	configuredMaxSec := cfg.TokenRouterFailureCooldownMaxSec''',
'''	cacheTTLMs := int64(cfg.TokenRouterCacheTtlMs)
	cache := NewRouteCache(cacheTTLMs)
	SetGlobalCache(cache)

	// Tunables below are snapshotted at construction time (same semantics as
	// pre-C1: they are baked into the router and a settings change applies on
	// the next router rebuild / restart).
	rt := config.Runtime()
	configuredMaxSec := rt.TokenRouterFailureCooldownMaxSec''', 1)
src = src.replace('fallbackUnitCost := cfg.RoutingFallbackUnitCost', 'fallbackUnitCost := rt.RoutingFallbackUnitCost', 1)
for f in ['BaseWeightFactor','ValueScoreFactor','CostWeight','BalanceWeight','UsageWeight']:
    src = src.replace('cfg.RoutingWeights.'+f, 'rt.RoutingWeights.'+f)
open('routing/router.go','w',encoding='utf-8').write(src)
print("router.go updated")
