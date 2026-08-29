package app

import (
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/routing"
)

// ApplyCacheRatioOverrides pushes the operator-configured cache-ratio
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
}
