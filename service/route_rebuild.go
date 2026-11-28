package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/jmoiron/sqlx"
)

// RouteRebuildStats summarizes a synchronous rebuild pass.
// explicit_group routes do not materialize their own channels; they expand
// source routes at selection time via route_group_sources.
type RouteRebuildStats struct {
	RoutesCreated       int `json:"routesCreated"`
	UnsafeModelsSkipped int `json:"unsafeModelsSkipped"`
	RoutesConsidered    int `json:"routesConsidered"`
	PatternRoutes       int `json:"patternRoutes"`
	GroupRoutes         int `json:"groupRoutes"`
	ChannelsInserted    int `json:"channelsInserted"`
	ChannelsRemoved     int `json:"channelsRemoved"`
	ChannelsKept        int `json:"channelsKept"`
	// Changed covers newly created routes and automatic channel changes.
	Changed bool `json:"changed"`
}

// RebuildOptions configures optional rebuild behavior. The zero value keeps the
// legacy non-probe path byte-for-byte identical.
type RebuildOptions struct {
	CreateModelRoutes bool
	ProbeFilter       ProbeFilterConfig
}

var routeRebuildMu sync.Mutex

// RebuildRoutesBestEffort rebuilds pattern-route channels from current model
// availability and invalidates the token-router cache. Failures are logged and
// swallowed so account/site mutation paths stay best-effort.
func RebuildRoutesBestEffort(db *sqlx.DB) {
	if db == nil {
		slog.Debug("route rebuild skipped: database not available")
		return
	}
	stats, err := RebuildTokenRoutesFromAvailabilityWithOptions(context.Background(), db, rebuildOptionsFromConfig())
	if err != nil {
		slog.Warn("route rebuild best-effort failed", "error", err)
		return
	}
	slog.Info("route rebuild best-effort completed",
		"routesConsidered", stats.RoutesConsidered,
		"patternRoutes", stats.PatternRoutes,
		"groupRoutes", stats.GroupRoutes,
		"channelsInserted", stats.ChannelsInserted,
		"channelsRemoved", stats.ChannelsRemoved,
	)
}

// RebuildTokenRoutesFromAvailability is the non-probe rebuild entrypoint.
// Model route creation follows the opt-in runtime setting (default off); the
// in-process cache is always invalidated after the pass.
func RebuildTokenRoutesFromAvailability(ctx context.Context, db *sqlx.DB) (RouteRebuildStats, error) {
	opts := RebuildOptions{}
	if rt := config.RuntimeSafe(); rt != nil {
		opts.CreateModelRoutes = rt.AutoCreateModelRoutes
	}
	return RebuildTokenRoutesFromAvailabilityWithOptions(ctx, db, opts)
}

// RebuildTokenRoutesFromAvailabilityWithOptions repopulates automatic
// (non-manual) channels for every pattern/exact route from dual sources:
// 1. manual channels on exact-model routes whose model_pattern matches the target pattern
// 2. token_model_availability + model_availability rows whose model_name matches
//
// explicit_group routes are counted but not rewritten: their membership is
// route_group_sources, and channel expansion happens at select time. Manual
// override channels on pattern routes are never deleted.
//
// When opts.ProbeFilter.Enabled is true, desired channels are filtered by
// include/exclude model lists plus the latest connectivity probe result before
// any diff is written, and the cache is invalidated only when the model set
// actually changes. The zero-value RebuildOptions{} preserves legacy behavior.
//
// Concurrent rebuilds serialize on a process-wide mutex.
func RebuildTokenRoutesFromAvailabilityWithOptions(ctx context.Context, db *sqlx.DB, opts RebuildOptions) (RouteRebuildStats, error) {
	var stats RouteRebuildStats
	if db == nil {
		return stats, fmt.Errorf("route rebuild: db is nil")
	}
	if err := ctx.Err(); err != nil {
		return stats, err
	}

	routeRebuildMu.Lock()
	defer routeRebuildMu.Unlock()

	if opts.CreateModelRoutes {
		created, skipped, err := createUncoveredModelRoutes(ctx, db)
		if err != nil {
			return stats, err
		}
		stats.RoutesCreated, stats.UnsafeModelsSkipped = created, skipped
	}

	type routeRow struct {
		ID           int64  `db:"id"`
		ModelPattern string `db:"model_pattern"`
		RouteMode    string `db:"route_mode"`
		Enabled      bool   `db:"enabled"`
	}
	var routes []routeRow
	if err := db.SelectContext(ctx, &routes,
		`SELECT id, model_pattern, route_mode, enabled FROM token_routes ORDER BY id ASC`); err != nil {
		return stats, fmt.Errorf("load token_routes: %w", err)
	}

	stats.RoutesConsidered = len(routes)
	for _, route := range routes {
		if routing.IsExplicitGroupRoute(route.RouteMode) {
			stats.GroupRoutes++
			continue
		}
		stats.PatternRoutes++
		inserted, removed, kept, err := rebuildAutomaticRouteChannelsByModelPattern(ctx, db, route.ID, route.ModelPattern, opts)
		if err != nil {
			return stats, fmt.Errorf("rebuild route %d (%s): %w", route.ID, route.ModelPattern, err)
		}
		stats.ChannelsInserted += inserted
		stats.ChannelsRemoved += removed
		stats.ChannelsKept += kept
	}

	changed := stats.RoutesCreated + stats.ChannelsInserted + stats.ChannelsRemoved
	stats.Changed = changed > 0
	if opts.ProbeFilter.Enabled {
		if changed > 0 {
			routing.InvalidateCache()
		} else {
			slog.Info("route rebuild: model set unchanged, skipping cache invalidation")
		}
	} else {
		// Legacy non-probe path: always invalidate (identical to prior behavior).
		routing.InvalidateCache()
	}
	return stats, nil
}

// rebuildOptionsFromConfig resolves the probe-filter configuration from the
// global config singleton. A nil config yields the zero-value (legacy) options.
func rebuildOptionsFromConfig() RebuildOptions {
	opts := RebuildOptions{}
	if rt := config.RuntimeSafe(); rt != nil {
		opts.CreateModelRoutes = rt.AutoCreateModelRoutes
	}
	cfg := config.GetSafe()
	if cfg == nil {
		return opts
	}
	opts.ProbeFilter = ProbeFilterConfig{
		Enabled:       cfg.RouteRebuildProbeFilterEnabled,
		IncludeModels: cfg.RouteRebuildProbeFilterIncludeModels,
		ExcludeModels: cfg.RouteRebuildProbeFilterExcludeModels,
	}
	return opts
}
