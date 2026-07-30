package app

import (
	"github.com/tokendancelab/metapi-go/config"
	"github.com/tokendancelab/metapi-go/routing"
)

// ApplyCacheRatioOverrides pushes the operator-configured cache-ratio fallback
// overrides (N7) from config into the routing package's runtime. Called at
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
}
