package service

import (
	"sync"

	"github.com/deliciousbuding/metapi-go/routing"
)

// siteProxyCacheInvalidators are optional hooks (e.g. admin accounts snapshot cache).
// Registered from other packages to avoid import cycles.
var (
	siteProxyCacheInvalidatorsMu sync.RWMutex
	siteProxyCacheInvalidators   []func()
)

// RegisterSiteProxyCacheInvalidator appends a hook invoked by InvalidateSiteProxyCache.
// Safe for concurrent registration; hooks should be idempotent and non-blocking.
func RegisterSiteProxyCacheInvalidator(fn func()) {
	if fn == nil {
		return
	}
	siteProxyCacheInvalidatorsMu.Lock()
	siteProxyCacheInvalidators = append(siteProxyCacheInvalidators, fn)
	siteProxyCacheInvalidatorsMu.Unlock()
}

// InvalidateSiteProxyCache refreshes process-local caches that depend on site/proxy config.
// Always invalidates the token-router route cache; then runs registered hooks
// (admin accounts snapshot, future proxy config caches).
func InvalidateSiteProxyCache() {
	routing.InvalidateCache()
	siteProxyCacheInvalidatorsMu.RLock()
	hooks := append([]func(){}, siteProxyCacheInvalidators...)
	siteProxyCacheInvalidatorsMu.RUnlock()
	for _, fn := range hooks {
		func() {
			defer func() {
				if r := recover(); r != nil {
					// never let a hook panic abort site mutations
					_ = r
				}
			}()
			fn()
		}()
	}
}

// InvalidateTokenRouterCache signals that cached token-router state should be invalidated.
func InvalidateTokenRouterCache() {
	routing.InvalidateCache()
}

// InvalidateSiteCaches invalidates both site proxy and token router caches.
func InvalidateSiteCaches() {
	InvalidateSiteProxyCache()
	InvalidateTokenRouterCache()
}
